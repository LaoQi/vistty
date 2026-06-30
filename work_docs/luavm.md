# Lua VM 执行线程分析

> 分析日期：2026-06-30
> 范围：Lua VM (gopher-lua) 的执行线程模型，是否影响渲染线程

## 1. 核心结论

**Lua VM 与渲染主线程同线程串行执行**，通过 `runtime.LockOSThread()`（render_loop.go:26）锁定在主 OS 线程。所有可能触发 Lua 的事件通过 channel 汇聚到主循环的 `select` 单点消费，规避了 gopher-lua VM 非线程安全的风险。

代价：Lua 钩子是**同步阻塞**调用方。若 Lua 脚本出现死循环、`os.execute`、超长计算，会直接卡住渲染帧。`PluginManager` 内部**无任何 mutex**，完全依赖"主线程串行"这一不变式。

## 2. VM 创建与持有

- **创建位置**：`cmd/vistty/main.go:45` `pm := plugins.NewPluginManager(configPath)`
- **VM 实例**：`lua.NewState()`（manager.go:37），由 `PluginManager.L` 字段持有（manager.go:24），全进程唯一，无 per-goroutine VM、无 VM pool
- **创建时机**：main goroutine，`Run()` 启动前
- **init.lua 执行**：`pm.Load()` → `L.DoFile`（manager.go:63），仍在 main goroutine
- **Activate 注入 ctx**：`pm.Activate(m)`（main.go:188-189），Run 前完成

```go
// internal/plugins/manager.go:23-34
type PluginManager struct {
    L        *lua.LState      // 唯一 VM 实例
    ctx      PluginContext
    initPath string
    registry    *ime.Registry
    keyHooks    *lua.LTable
    renderHooks *lua.LTable
    panels      map[string]int
    bindings    []keyBinding
    pressedKeys map[uint16]bool
    active      bool
}
```

## 3. 调用入口与触发线程

### 3.1 调用入口清单

| 入口方法 | 文件:行号 | 触发线程 | 是否执行 Lua |
|---|---|---|---|
| `pm.Load()` → `L.DoFile` | manager.go:63 | main goroutine（Run 前） | 是（执行 init.lua 全文） |
| `pm.Reload()` → `L.DoFile` | manager.go:74,86 | 主渲染线程 | 是（重新执行 init.lua） |
| `pm.OnKey(ev)` | manager.go:121 | 主渲染线程 | 是（PCall bindings + keyHooks） |
| `pm.OnRender(side,w,h)` | manager.go:249 | 主渲染线程 | 是（PCall renderHooks） |
| `pm.EnabledPanels()` | manager.go:102 | 主渲染线程 | **否**（仅读 map） |
| `pm.SetPanel(side,n)` | manager.go:110 | 主渲染线程 | **否**（仅写 map） |
| `pm.Close()` → `L.Close()` | manager.go:95 | main goroutine（defer） | 否（仅销毁 VM） |
| IME 钩子闭包 | api_ime.go:179-263 | 主渲染线程（在 OnKey/OnRender 栈内） | 是（PCall） |

### 3.2 生产代码调用点

生产代码中只有 **3 处**调用 PM 方法，全部在 `session/render_loop.go`：

```
render_loop.go:103   consumed, out := m.plugins.OnKey(ev)         // 主线程 select 的 keyEvCh 分支
render_loop.go:162   panels := m.plugins.EnabledPanels()         // renderPlugins() 内
render_loop.go:200   dirty, prims := m.plugins.OnRender(side,w,h)// renderPlugins() 内
```

`master.go:522/530/539` 的 `SetPanel/Reload/EnablePanel` 是 `PluginContext` 接口实现，但当前 Lua 路径**并未通过 ctx 调用**（`vistty.ui.enable/disable` 直接改 `pm.panels`，见 api_ui.go:26；`vistty.reload` 直接调 `pm.Reload()`，见 api_misc.go:182）。注释明确要求"须在主渲染线程调用"（master.go:534）。

### 3.3 IME ProcessKey 路径

IME 的按键路由**不是独立的 Go→Lua 调用**，而是嵌套在 `OnKey` 的 Lua 执行栈内：

```
主线程 OnKey (render_loop.go:103)
  └─ L.PCall(keyHooks[1])                        // 执行 init.lua 注册的 on_key 闭包 (manager.go:181)
       └─ Lua 调用 vistty.ime.process_key(ev)    // api_ime.go:105
            └─ pm.registry.ProcessKey(ev)         // registry.go:45 (Go)
                 └─ 若 active 是 LuaIMM:
                    └─ makeProcessKeyHook 闭包   // api_ime.go:247
                         └─ L.PCall(processKey fn) // api_ime.go:251 ← 嵌套 PCall
```

即 **Lua 主动调用 Go，Go 再回调 Lua** 的嵌套模式，但全程在同一个 `OnKey` 的 `L.PCall` 栈帧内，**同一线程、同一调用栈**，无并发。

拼音 IME 是 Go 原生实现（`ime/pinyin/pinyin.go:88`），其 `ProcessKey` 不回调 Lua。

## 4. 钩子机制

### 4.1 钩子注册（暂存）

钩子注册发生在 init.lua 执行期间（main goroutine，Run 前），把 Lua 函数引用存入 PM 字段：

| Lua API | Go 实现 | 存储位置 |
|---|---|---|
| `vistty.input.bind(code, fn)` | api_keybind.go:16 | `pm.bindings` (slice) |
| `vistty.input.bind_keys(tbl,fn)` | api_keybind.go:23 | `pm.bindings` (slice) |
| `vistty.input.on_key(fn)` | api_misc.go:190 | `pm.keyHooks` (LTable) |
| `vistty.ui.on_render(fn)` | api_ui.go:41 | `pm.renderHooks` (LTable) |
| `vistty.ime.register(name, hooks)` | api_ime.go:20 | `pm.registry`（闭包捕获 `*lua.LFunction`） |

注意：`api_ime.go` 的 `makeProcessKeyHook` 等闭包**捕获了 `pm.L`**（api_ime.go:249-250 `pm.L.Push(fn)`），所以 IME 钩子最终也通过同一个 `LState` 执行。

### 4.2 钩子激活机制

- **激活标志**：`pm.active` 在 `Activate()` (manager.go:71) 时置 true。`OnKey`/`OnRender` 开头都检查 `if !pm.active` (manager.go:132,250) 提前返回。
- **暂存→激活**：文档里说的"暂存"是指 init.lua 期间注册到 `keyHooks/renderHooks/bindings`（此时 `active=false`，不会被触发）；`Activate(ctx)` 之后才进入可触发状态。**没有两阶段队列、没有 pending buffer**，激活只是置 `active=true`。

### 4.3 同步 vs 异步

**全部同步阻塞执行**。证据：

- `OnKey` 中遍历 `pm.bindings` (manager.go:133-164) 直接 `pm.L.PCall(1, 1, nil)` (manager.go:140,154)；
- `OnKey` 中 `keyHooks.ForEach` (manager.go:171) 内 `pm.L.PCall(1, 1, nil)` (manager.go:181)；
- `OnRender` 中 `renderHooks.ForEach` (manager.go:258) 内 `pm.L.PCall(1, 1, nil)` (manager.go:265)。

没有任何 `go func()`、没有 channel 把钩子投递到其他线程。**钩子执行时间 = 调用方阻塞时间**。

### 4.4 Reload 行为

`pm.Reload()` (manager.go:74-93) 重建 `keyHooks/renderHooks/panels/bindings`，调 `registry.ClearLuaMethods()` (manager.go:82)，然后重新 `registerAPIs` + `Load`。期间 `active=false`，避免重载过程中被 `OnKey/OnRender` 重入。Reload 由 `vistty.reload()` Lua 函数触发（api_misc.go:181-186），即**在主线程 Lua 栈内递归调用**——因为已在主线程，重入安全。

## 5. 渲染主循环

### 5.1 主循环结构

`session/render_loop.go:25-148` `Master.Run()`：

```go
func (m *Master) Run() error {
    runtime.LockOSThread()        // render_loop.go:26 ← 锁定 OS 线程
    defer runtime.UnlockOSThread()
    ...
    go func() { m.backend.Run(func() {}) ... }()  // render_loop.go:37 backend-loop goroutine
    ...
    m.wg.Add(2)
    go m.inputLoop()     // render_loop.go:56  input goroutine
    go m.signalLoop()    // render_loop.go:57  signal goroutine
    ...
    for {
        select {
        case msg := <-usc:          // render_loop.go:65  seq-relay（PTY 序列）
        case <-ticker.C:            // render_loop.go:69  60fps 渲染 tick → renderFrame()
        case ev := <-resizeCh:      // render_loop.go:99  resize
        case ev := <-m.keyEvCh:    // render_loop.go:101 按键 → OnKey + handleKey
        case req := <-m.scaleReqCh: // render_loop.go:110 缩放
        case <-m.renderReqCh:      // render_loop.go:112 渲染请求
        case req := <-m.tabReqCh:   // render_loop.go:124 标签操作
        case exited := <-tec:      // render_loop.go:126 终端退出
        case <-m.done:             // render_loop.go:128
        case <-m.backend.Done():   // render_loop.go:130
        }
    }
}
```

**确认主循环 LockOSThread** (render_loop.go:26)。所有 `select` 分支体都在这个被锁定的 OS 线程上执行。

### 5.2 主循环中调用 Lua 的步骤

只有 **3 个分支**会触发 Lua，全部在主线程同步执行：

1. **`case <-ticker.C` (render_loop.go:69)** → `renderFrame()` (render_loop.go:85) → `renderPlugins()` (render_loop.go:151→158)：
   - `renderPlugins` 遍历所有 slave，对每个的 `bottom/left/right` 三个面板调用 `m.plugins.OnRender(side, w, h)` (render_loop.go:200)。
   - 即 **每帧最多 `len(slaves) × 3` 次 `OnRender`** → 每次 `OnRender` 内 `renderHooks.ForEach` PCall。
   - `OnRender` 开头 `side=="top"` 提前返回 (manager.go:253-255)，故顶部不调 Lua。

2. **`case ev := <-m.keyEvCh` (render_loop.go:101)** → `m.plugins.OnKey(ev)` (render_loop.go:103)：
   - 若 `consumed` 则 `continue` (render_loop.go:104-106)，不调 `handleKey`；
   - 否则 `handleKey(ev)` (render_loop.go:109) → `terminal.HandleKey` (terminal.go:1007)，**HandleKey 不调用 Lua**，只做 `WriteKeyEscape`/`PtyWrite` (terminal.go:1029,1050)。

3. **`case req := <-m.scaleReqCh` (render_loop.go:110)** → `handleScale` (render_loop.go:300) → 末尾 `m.renderFrame()` (render_loop.go:346)，又走 `renderPlugins` → `OnRender`。

其余分支（`usc`/`resizeCh`/`tabReqCh`/`tec`/`done`/`backend.Done`）**不调用 Lua**。

### 5.3 Lua 阻塞渲染的可行性

**会**。证据链：

- `renderFrame` → `renderPlugins` → `OnRender` → `renderHooks.ForEach` → `pm.L.PCall` (manager.go:265) 同步阻塞。
- `OnKey` → `bindings` 遍历 `pm.L.PCall` (manager.go:140,154) + `keyHooks.ForEach` `pm.L.PCall` (manager.go:181) 同步阻塞。

若 Lua 脚本中存在：
- 死循环 / `while true do end` → 永久卡住主线程，渲染停止，输入停止响应。
- `os.execute("sleep 5")`（gopher-lua 标准库默认含 `os`）→ 卡 5 秒。
- 候选词面板每帧 `vistty.ime.candidates()` 在大词库下遍历 → 帧耗时增加。

`OnRender` 是每帧调用，**Lua 渲染钩子的耗时直接计入帧时间**。这是当前架构最敏感的性能点。

## 6. 输入路径

### 6.1 inputLoop goroutine（独立，不触碰 Lua）

`render_loop.go:352-401` `inputLoop()`：

```go
for {
    select {
    case ev := <-m.input.KeyEvents():   // render_loop.go:377
        ...
        m.dispatchKey(ev)               // render_loop.go:380,387
    case <-delayCh: ... m.dispatchKey(repeatEv)  // render_loop.go:394
    case <-rateCh:  ... m.dispatchKey(repeatEv)  // render_loop.go:396
    case <-m.done: return
    }
}
```

`dispatchKey` (render_loop.go:406-411) **只做一件事**：把按键投递到 channel：

```go
func (m *Master) dispatchKey(ev platform.KeyEvent) {
    select {
    case m.keyEvCh <- ev:    // render_loop.go:408
    case <-m.done:
    }
}
```

注释明确说明设计意图 (render_loop.go:403-405)：

> 由 inputLoop goroutine 调用，将按键事件投递到 keyEvCh，由主线程 select 消费，**保证 plugins.OnKey（gopher-lua VM 非线程安全）与 handleKey 在主线程串行执行**。

所以 inputLoop **从不直接调用 Lua**。它只负责按键 repeat 逻辑并把事件塞进 `keyEvCh`（buffer 64，master.go:136）。

### 6.2 handleKey 不调用 Lua

`render_loop.go:413-420` `handleKey` → `ft.HandleKey(ev)` (render_loop.go:418) → `terminal.HandleKey` (terminal.go:1007-1052)，只做滚动偏移维护 + `WriteKeyEscape` + `PtyWrite`，**全程无 Lua 调用**。

### 6.3 IME Registry.ProcessKey 不独立触发

如 §3.3 所述，`registry.ProcessKey` (registry.go:45-50) 只在 `vistty.ime.process_key` Lua 函数内被调用 (api_ime.go:112)，而该 Lua 函数只在 `on_key` 钩子内被 Lua 调用（见 examples/init.lua:69-77）。因此 ProcessKey **始终在主线程 `OnKey` 的 Lua 栈内**，不构成独立调用点。

## 7. Mutex 使用与锁竞争

### 7.1 PluginManager 无 mutex

确认：`manager.go:23-34` 的 `PluginManager` 结构体**没有任何 `sync.Mutex`/`sync.RWMutex` 字段**。全文件搜索 `sync.` 在 `internal/plugins/` 下**零命中**。

PM 的所有可变状态——`bindings`、`panels`、`keyHooks`、`renderHooks`、`pressedKeys`、`active`、`ctx`——**全部无锁访问**。这依赖一个隐式不变式：

> **所有 PM 方法的调用必须发生在主渲染线程。**

### 7.2 该不变式当前是否成立？

**成立**。逐条核对：

| PM 字段写入处 | 触发线程 | 证据 |
|---|---|---|
| `bindings` append | main goroutine（init.lua 执行）| api_keybind.go:19，在 `L.DoFile` 栈内 |
| `panels` 写 | 主线程（on_render 钩子内）| api_ui.go:26,36，在 `L.PCall` 栈内 |
| `keyHooks/renderHooks` Append | main/主线程 | api_misc.go:192, api_ui.go:43 |
| `pressedKeys` 写 | 主线程 | manager.go:126（OnKey 内） |
| `active`/`ctx` 写 | main goroutine | manager.go:70-71（Activate 在 Run 前） |
| `keyHooks/renderHooks` 重建（Reload）| 主线程 | manager.go:75-76（vistty.reload 在主线程 Lua 栈） |

读取侧（OnKey/OnRender/EnabledPanels）也都在主线程（render_loop.go:103,162,200）。

**因此当前不存在数据竞争，也不存在锁竞争**——因为根本没有锁。

### 7.3 潜在风险点

虽然当前安全，但有几个**脆弱点**：

1. **不变式未被代码强制**：`EnablePanel/DisablePanel/ReloadPlugins`（master.go:518,526,535）是 `PluginContext` 接口实现，暴露给 Lua 通过 `pm.ctx` 调用。若未来有 Lua 脚本（或任何代码路径）从非主线程调用它们，会破坏不变式且无锁保护。目前 Lua 路径绕过了它们（直接改 `pm.panels`/调 `pm.Reload()`），所以暂时安全。

2. **PTY 写阻塞会卡主线程**：`vistty.term.send` (api_term.go:28) → `t.PtyWrite` (terminal.go:1054) → `t.hostWriter.Write` (terminal.go:1058)，`hostWriter` 是 PTY master `*os.File` (terminal.go:95)。当 shell 未读取且 PTY buffer 满时，`write()` 系统调用会阻塞主线程。`PtyWrite` **不持有 `terminal.mu`**（terminal.go:1054-1061 无 Lock），所以不会和 pty-read 路径的 `Apply`（terminal.go:117 持 `mu.Lock`）死锁，但会阻塞渲染。IME commit 路径（init.lua:73-75 `vistty.term.send(r.commit)`）在 OnKey 栈内，若 commit 大量文本可能短暂卡顿。

3. **Lua 标准库未裁剪**：`lua.NewState()` (manager.go:37) 使用默认配置，gopher-lua 默认开放 `os`/`io` 等。`os.execute`/`io.read` 等会阻塞主线程。

4. **每帧 OnRender × 面板数**：多屏 + 多面板时，每帧 `len(slaves) × 3` 次 `OnRender`（render_loop.go:194-205 循环），每次都 PCall 全部 `renderHooks`。示例 init.lua 的 on_render 含 `os.date` 和字符串拼接，单次成本可接受，但若用户脚本复杂，帧预算（16.6ms@60fps）会被压缩。

## 8. 完整的 Goroutine × Lua 调用矩阵

| Goroutine | 是否调用 Lua | 调用点 | 说明 |
|---|---|---|---|
| **main**（Run 前）| 是 | `pm.Load()`→`L.DoFile` (manager.go:63)；`registerAPIs` 注册闭包 (manager.go:48) | 初始化阶段，单线程 |
| **主渲染线程**（Run，LockOSThread）| **是** | `OnKey` (render_loop.go:103)；`OnRender` (render_loop.go:200)；`EnabledPanels` (render_loop.go:162，无 L)；`Reload`（via vistty.reload，api_misc.go:182） | select 单点消费，串行 |
| **backend-loop** (render_loop.go:37) | 否 | `m.backend.Run()` | 仅 Wayland dispatch / DRM 空操作 |
| **inputLoop** (render_loop.go:56) | **否** | `dispatchKey`→`keyEvCh` channel (render_loop.go:408) | 只投递事件，不碰 VM |
| **signalLoop** (render_loop.go:57) | 否 | `signalClose()` (render_loop.go:444) | 只 `close(m.done)` |
| **pty-read** (master.go:202) | 否 | `PtyReadLoop` (terminal.go:295) → `seqCh` | 只读 PTY + 解析，序列经 seq-relay 投主线程 |
| **seq-relay** (master.go:206) | 否 | `t.SeqCh()`→`m.seqRelay` (master.go:212) | 中转 channel |
| **exit-watch** (master.go:221) | 否 | `t.EofCh()`→`m.exitCh` (master.go:230) | 中转 channel |
| **signal**（SIGINT 等）| 否 | `signalClose()` | 只 `close(m.done)` |
| **drm-event**（仅 DRM）| 否 | Page Flip 回调 | 不碰 VM |
| **vt-signal**（仅 DRM，SIGUSR1/2）| 否 | VT 切换 | 不碰 VM |
| **input-watch**（仅 DRM，inotify）| 否 | 热插拔 | 不碰 VM |

**结论：唯一执行 Lua 的 goroutine 是主渲染线程（以及 Run 前的 main goroutine，二者是同一线程因为 LockOSThread 在 Run 入口）。**

## 9. 是否影响渲染线程？最终判断

**直接影响：是，但属于设计预期内的"同线程串行"，非意外的并发干扰。**

- **无并发数据竞争**：VM 只在一个线程执行，无 mutex 需求，无锁竞争。
- **同步阻塞影响帧时间**：每帧 `OnRender`（render_loop.go:200）+ 按键时 `OnKey`（render_loop.go:103）的 Lua 执行时间直接计入主线程时间预算。Lua 钩子耗时 ↑ → 帧率 ↓。
- **无跨线程唤醒/阻塞**：inputLoop/pty-read 等通过 channel 解耦，不会因为等 Lua 而阻塞（它们只往 channel 塞数据，不读结果）。
- **最大风险是 Lua 脚本本身**：死循环、`os.execute`、超长计算会卡死渲染。这是"开放沙箱"架构的固有代价（examples/init.lua:110 注释 "完全开放沙箱，dofile 可用"）。

## 10. 独立 Lua 线程方案评估

### 10.1 核心障碍：钩子返回值的同步依赖

独立线程的最大难点不在 VM 安全性，而在**返回值被主线程立即消费**：

- `OnKey` → `(consumed, out)`：主线程据此决定是否 `handleKey`（render_loop.go:104-109），`out` 可能改写 rune/code/mods。**必须同步等结果**。
- `OnRender` → `(dirty, primitives)`：主线程当帧用 `primitives` 填充 OSD（render_loop.go:204）。**必须当帧可用**。

若简单异步化，会破坏按键语义（无法判断是否消费）和面板渲染语义（无图元）。

### 10.2 Lua 回调 Go API 的线程安全

只要 Lua 跑在独立线程，`vistty.*` API 就在该线程执行，当前这些 API 访问的共享状态**全部无锁**：

| API | 访问的共享状态 | 当前安全性来源 |
|---|---|---|
| `vistty.term.send` | `terminal.PtyWrite` → PTY master fd | 主线程串行 |
| `vistty.ui.enable/disable` | `pm.panels` map | 主线程串行 |
| `vistty.ui.on_render` 注册 | `pm.renderHooks` LTable | 主线程串行 |
| `vistty.ime.process_key` | `pm.registry` + IME 状态 | 主线程串行 |
| `vistty.tab.*` | `pm.ctx`(Master) 的标签状态 | 主线程串行 |

独立线程后必须为这些加锁，其中：
- `pm.panels`/`pm.registry` 加 mutex 简单
- **`terminal.PtyWrite` 跨线程写 PTY 需谨慎**（可能与 pty-read 的 Apply 在不同线程，需确保 terminal.mu 覆盖）
- **`pm.ctx` (Master) 的标签操作**跨线程最复杂，当前 Master 大量状态无锁

### 10.3 三种独立线程思路的利弊

#### 思路 A：完全独立线程 + 跨线程同步 RPC

Lua VM 跑在独立 goroutine，主线程通过 req/resp channel 调用并阻塞等待结果。

- **问题**：主线程仍需等结果，阻塞点从"Lua 执行"变成"等 channel 往返"，**多了 goroutine 调度与 channel 唤醒开销**，正常情况帧时间反而更差。
- **唯一收益**：Lua 死循环可加超时检测，主线程可降级继续渲染。
- **复杂度**：高——需处理 `vistty.*` 全部 API 的跨线程安全。
- **判断**：收益不抵开销，不推荐。

#### 思路 B：OnRender 异步 + OnKey 同步

Lua 线程持续跑渲染钩子，结果异步投递，主线程用**上一帧结果**（延迟 1 帧）；OnKey 保持同步跨线程调用（按键频率低，可接受微秒级往返）。

- **优点**：每帧 `len(slaves)×3` 次 OnRender 不再阻塞主线程，**帧率提升明显**；OnKey 同步保证按键语义不破坏。
- **缺点**：
  - 面板 UI 延迟一帧（约 16ms，可接受）
  - 仍需解决 Lua 回调 Go API 的线程安全（见 10.2）
  - 需处理 OnRender 与 OnKey 在 Lua 线程的串行化（用同一 channel 排队，避免 VM 并发）
- **复杂度**：中
- **判断**：若 profiling 显示 OnRender 占帧时间显著，此方案收益最高。

#### 思路 C：不独立线程，加防护

- 用 gopher-lua 的 `debug.sethook` 设置指令计数上限，防 Lua 死循环
- 裁剪 `os`/`io` 标准库（定制 `lua.OpenPackage`），防 `os.execute`/`io.read` 等阻塞系统调用
- **优点**：改动极小，解决"Lua 卡死渲染"的最大风险，不改线程模型
- **缺点**：不能降低正常钩子的帧时间开销
- **复杂度**：低

### 10.4 评估总结

| 维度 | 思路 A | 思路 B | 思路 C |
|---|---|---|---|
| 帧时间改善 | 负面（多往返开销） | 显著（OnRender 解耦） | 无 |
| 防卡死能力 | 有（超时降级） | 部分（仅 OnRender 解耦） | 有（指令计数+库裁剪） |
| 改动量 | 大（全 API 加锁） | 中（部分加锁+异步管线） | 极小 |
| 按键语义破坏 | 无（同步） | 无（OnKey 同步） | 无 |
| 面板 UI 延迟 | 无 | 1 帧 | 无 |

**核心判断**：

1. 当前架构的"同线程串行"是**有意为之的保守设计**，非缺陷。它用零锁换取了简单性，代价是 Lua 钩子耗时直接计入帧预算。
2. 真正的风险不在性能（示例 init.lua 的 on_render 成本可控），而在**Lua 脚本可卡死渲染**——这是优先要解决的问题。
3. 思路 C 用最低成本解决最大风险，思路 B 在性能确实不足时才有必要。
4. **任何独立线程方案的前提**：必须先完成 `vistty.*` API 的线程安全改造，其中 Master 标签状态的跨线程化是最大工作量。

## 11. 关键文件:行号索引

**PluginManager 结构与 VM 创建**
- `internal/plugins/manager.go:23-34` — PluginManager 结构（无 mutex）
- `internal/plugins/manager.go:36-50` — NewPluginManager + `lua.NewState()`
- `cmd/vistty/main.go:45-46,188-189` — PM 创建/Load/Activate（main goroutine，Run 前）

**主渲染循环**
- `session/render_loop.go:25-27` — `Run()` + `runtime.LockOSThread()`
- `session/render_loop.go:63-134` — select 主循环
- `session/render_loop.go:101-109` — keyEvCh 分支：`OnKey` + `handleKey`
- `session/render_loop.go:150-153` — `renderFrame`→`renderPlugins`+`renderIndependent`
- `session/render_loop.go:158-207` — `renderPlugins`（每帧调 `OnRender`）
- `session/render_loop.go:200` — `m.plugins.OnRender(side, w, h)` 调用点
- `session/render_loop.go:162` — `m.plugins.EnabledPanels()` 调用点
- `session/render_loop.go:103` — `m.plugins.OnKey(ev)` 调用点

**输入路径（解耦关键）**
- `session/render_loop.go:352-401` — `inputLoop` goroutine（不调 Lua）
- `session/render_loop.go:403-411` — `dispatchKey` 注释 + channel 投递（设计意图明示）
- `session/render_loop.go:413-420` — `handleKey`（不调 Lua）
- `terminal/terminal.go:1007-1052` — `HandleKey`（不调 Lua）
- `terminal/terminal.go:1054-1061` — `PtyWrite`（不持锁，但 PTY 写可能阻塞）

**钩子执行（全部同步 PCall）**
- `internal/plugins/manager.go:121-222` — `OnKey`（bindings PCall + keyHooks.ForEach PCall）
- `internal/plugins/manager.go:249-279` — `OnRender`（renderHooks.ForEach PCall）
- `internal/plugins/manager.go:74-93` — `Reload`（重建 + 重新 DoFile）
- `internal/plugins/api_misc.go:181-186` — `vistty.reload` → `pm.Reload()`
- `internal/plugins/api_ime.go:179-263` — IME 钩子闭包（捕获 `pm.L`，PCall）
- `internal/plugins/api_ime.go:105-115` — `vistty.ime.process_key` → `registry.ProcessKey`

**PTY 路径（不碰 Lua）**
- `session/master.go:200-234` — `startTerminalGoroutines`（pty-read/seq-relay/exit-watch）
- `terminal/terminal.go:295-330` — `PtyReadLoop`
- `terminal/terminal.go:116-127` — `Apply`/`FeedBytes`（持 `mu.Lock`，主线程消费）

**PluginContext 接口（预留扩展点，当前 Lua 未走此路径）**
- `internal/plugins/context.go:18-36` — 接口定义
- `session/master.go:516-540` — Master 实现（EnablePanel/DisablePanel/ReloadPlugins，注释要求主线程调用）
