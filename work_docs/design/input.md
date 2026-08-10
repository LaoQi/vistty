# 输入设备热插拔设计

## 问题

当前 `DRMInput` 在 `newDRMInput()` 中一次性扫描 `/dev/input/event*`，为每个设备启动 `readLoop` goroutine。当设备断开时，`readLoop` 因 `ReadOne()` 返回错误而**静默退出**，无任何通知；新设备插入后也无人发现。所有设备断开时，`keyCh` 无写入者，`inputLoop` 永久阻塞，应用看起来"冻死"。

Wayland 后端未处理 `wl_seat.capabilities` 变化，键盘/指针重新出现时无法恢复。

## 设计目标

1. **DRM 后端**：自动检测输入设备的热插拔，断开时清理、重连时恢复
2. **Wayland 后端**：处理 `wl_seat.capabilities` 变化（键盘/指针重新出现）
3. **Session 层**：无需替换 `InputSource`，对上层透明
4. **CGO_ENABLED=0**：纯 Go 实现，用 inotify 监听 `/dev/input`

## 核心设计：DRM 后端热插拔

### 方案：inotify + 设备生命周期管理

在 `DRMInput` 内部增加一个 `watchLoop` goroutine，通过 inotify 监听 `/dev/input` 目录的 `IN_CREATE`/`IN_DELETE` 事件，实现设备的动态增删。

```
┌─────────────────────────────────────────────────────┐
│                    DRMInput                          │
│                                                     │
│  keyCh (chan KeyEvent)  ←── 所有 readLoop 共享写入   │
│  mouseCh (chan MouseEvent)                          │
│  done (chan struct{})                               │
│                                                     │
│  mu sync.Mutex                                      │
│  devices map[string]*deviceEntry  ←── 路径→设备映射  │
│  mods   Modifiers                                   │
│                                                     │
│  goroutines:                                        │
│    watchLoop()  ←── inotify 监听 /dev/input          │
│    readLoop(dev) × N  ←── 每设备一个                 │
└─────────────────────────────────────────────────────┘
```

### 数据结构变更

```go
// internal/platform/drm/input.go

type deviceEntry struct {
    dev  *evdev.InputDevice
    path string
    done chan struct{}   // 单设备关闭信号
}

type DRMInput struct {
    keyCh   chan platform.KeyEvent
    mouseCh chan platform.MouseEvent
    done    chan struct{}
    closeOnce sync.Once

    mu      sync.Mutex
    devices map[string]*deviceEntry   // path → entry
    mods    platform.Modifiers

    inotifyFd  int
    watchDone  chan struct{}
}
```

### 设备打开/关闭流程

**打开设备** (`openDevice`):
1. `evdev.Open(path)` → 检查 `EV_KEY` 能力 → `Grab()`
2. 创建 `deviceEntry{dev, path, make(chan struct{})}`
3. `mu.Lock()` → `devices[path] = entry` → `mu.Unlock()`
4. `go readLoop(entry)`

**关闭设备** (`closeDevice`):
1. `mu.Lock()` → 从 `devices` 删除 entry → `mu.Unlock()`
2. `close(entry.done)` → 通知 readLoop 退出
3. `dev.Ungrab()` → `dev.Close()`
4. **重置 modifier 状态**：遍历剩余设备，若全部键盘已断开则 `mods = 0`

### readLoop 改造

```go
func (i *DRMInput) readLoop(e *deviceEntry) {
    defer i.handleDeviceLost(e)  // 退出时自动清理

    for {
        ev, err := e.dev.ReadOne()
        if err != nil {
            select {
            case <-i.done:      // 全局关闭
                return
            case <-e.done:      // 单设备关闭
                return
            default:
                return          // 设备断开，触发 handleDeviceLost
            }
        }
        // ... 原有事件处理逻辑不变 ...
    }
}
```

**`handleDeviceLost`** — 设备丢失时的清理：
1. `mu.Lock()` → 从 `devices` map 中删除该 entry → `mu.Unlock()`
2. `dev.Ungrab()` + `dev.Close()`（忽略错误，fd 可能已失效）
3. 检查剩余设备数，若为 0 则重置 `mods = 0`
4. `debug.Debugf("input device disconnected: %s", e.path)`

### watchLoop — inotify 监听

```go
func (i *DRMInput) watchLoop() {
    defer close(i.watchDone)

    // 创建 inotify 实例
    i.inotifyFd, _ = unix.InotifyInit1(unix.IN_NONBLOCK | unix.IN_CLOEXEC)
    unix.InotifyAddWatch(i.inotifyFd, "/dev/input",
        unix.IN_CREATE|unix.IN_DELETE|unix.IN_MOVED_TO)

    // 用 epoll 监听 inotify fd + done 信号
    epollFd, _ := unix.EpollCreate1(0)
    // 添加 inotifyFd (EPOLLIN) + eventfd(done)
    // ...

    for {
        events, err := unix.EpollWait(epollFd, ...)
        // 处理 inotify 事件：
        //   IN_CREATE/IN_MOVED_TO → openDevice(path)
        //   IN_DELETE → closeDevice(path)
        //   done → return
    }
}
```

**关键细节**：
- `IN_CREATE` 事件到达时，设备节点可能还未就绪（udev 规则尚未应用权限），需要**短暂延迟后重试打开**（最多 3 次，间隔 100ms）
- inotify 事件只提供文件名（如 `event5`），需拼接为 `/dev/input/event5`
- 使用 `IN_NONBLOCK` + epoll，避免 inotify 读取阻塞
- 用 epoll 同时监听 inotify fd 和 `done` 信号（通过 eventfd 或 pipe），实现干净退出
- 只关注 `event*` 文件名模式，忽略 `mice`、`mouse*` 等

### Close() 改造

```go
func (i *DRMInput) Close() error {
    i.closeOnce.Do(func() {
        close(i.done)                    // 通知所有 readLoop 退出
        <-i.watchDone                    // 等待 watchLoop 退出
        if i.inotifyFd >= 0 {
            unix.Close(i.inotifyFd)
        }
        i.mu.Lock()
        for _, e := range i.devices {
            close(e.done)
            e.dev.Ungrab()
            e.dev.Close()
        }
        i.mu.Unlock()
    })
    return nil
}
```

## Wayland 后端改造

Wayland 的热插拔由合成器管理，通过 `wl_seat.capabilities` 事件通知。当前代码在 `bindSeat` 中注册了 `onCapabilities` 回调但未在 `WaylandInput` 中处理。

### 改造方案

1. 在 `WaylandInput` 中增加 `seat *wlSeat` 字段引用
2. 增加 `handleCapabilities(cap uint32)` 方法
3. 当 `WL_SEAT_CAPABILITY_KEYBOARD` 标志出现时，若 `keyboard == nil` 则重新 `seat.getKeyboard()` + 注册回调
4. 当该标志消失时，`keyboard.release()` + 置 nil + 重置 mods
5. 同理处理 `WL_SEAT_CAPABILITY_POINTER`
6. 在 `bindSeat` 的 `onCapabilities` 回调中，将事件转发给 `WaylandInput`

```go
func (i *WaylandInput) handleCapabilities(cap uint32) {
    const (
        WL_SEAT_CAPABILITY_POINTER  = 1
        WL_SEAT_CAPABILITY_KEYBOARD = 2
    )
    if cap&WL_SEAT_CAPABILITY_KEYBOARD != 0 && i.keyboard == nil {
        i.keyboard = i.seat.getKeyboard()
        // 注册 onKeymap/onModifiers/onKey 回调
    } else if cap&WL_SEAT_CAPABILITY_KEYBOARD == 0 && i.keyboard != nil {
        i.keyboard.release()
        i.keyboard = nil
        i.mu.Lock()
        i.mods = 0
        i.mu.Unlock()
    }
    // pointer 同理
}
```

## Session 层

Session 层**无需修改**。因为：
- `InputSource` 接口不变（`KeyEvents()/MouseEvents()/Close()`）
- `DRMInput` 内部管理设备增删，`keyCh` 始终是同一个 channel
- `inputLoop` 只读 `keyCh`，不关心有多少设备在写入
- 设备全部断开时 `keyCh` 无人写入，`inputLoop` 阻塞在 select 上，但不会死锁（`done` channel 仍可退出）

## 修改文件清单

| 文件 | 变更 |
|------|------|
| `internal/platform/drm/input.go` | 重构：增加 `deviceEntry`、`devices map`、`watchLoop`、`openDevice`/`closeDevice`/`handleDeviceLost`；改造 `readLoop`/`Close` |
| `internal/platform/wayland/input.go` | 增加 `seat` 字段引用、`handleCapabilities` 方法；改造 `newWaylandInput` |
| `internal/platform/wayland/backend.go` | `bindSeat` 的 `onCapabilities` 回调转发给 `WaylandInput` |
| `internal/platform/input.go` | 无变更 |
| `internal/platform/backend.go` | 无变更 |
| `session/` | 无变更 |

## 风险与边界情况

1. **权限问题**：inotify 监听 `/dev/input` 需要读权限（`input` 组），与打开 evdev 设备权限一致，无额外要求
2. **设备名复用**：USB 设备拔插后可能获得不同的 eventN 编号，inotify 的 `IN_DELETE`+`IN_CREATE` 能正确处理
3. **Grab 竞争**：新设备打开后立即 Grab，若其他进程（如 X11）也在监听，可能 Grab 失败 → 跳过该设备，下次 inotify 事件不会重试（需注意）
4. **快速拔插**：短时间内 `IN_DELETE`+`IN_CREATE` 可能合并，watchLoop 需处理"先删后建"的序列
5. **inotify 溢出**：`IN_Q_OVERFLOW` 事件时，应做一次全量重新扫描（`evdev.ListDevicePaths()` 对比 `devices` map，补新增、删已失）
6. **Wayland seat 重建**：极少数情况下合成器可能销毁并重建 `wl_seat`，需在 registry global remove 事件中处理（当前未实现，可后续补充）

## 实现优先级

1. **P0**：DRM 后端 `watchLoop` + 设备生命周期管理（解决核心问题）
2. **P1**：Wayland 后端 `capabilities` 变化处理
3. **P2**：inotify 溢出时的全量重扫描
4. **P3**：inputLoop 健康监控（debug 日志）
