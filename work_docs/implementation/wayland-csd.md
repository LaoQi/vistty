# Wayland CSD 自绘装饰 实施方案

## 概述
在现有 OSD 标签栏上融合 CSD 装饰元素（关闭/最大化/最小化按钮），不新增独立栏。当合成器不支持 SSD 或回退到 CSD 时，标签栏右侧绘制 `_` `□` `×` 按钮，标签栏空白区域支持拖拽移动窗口。

## 实施阶段

### 阶段 1: Surface 接口 + DecoMode 实现
- **状态**: 已审计
- **目标**: Surface 接口新增 DecoMode() 方法，DRM/Wayland Surface 分别实现
- **实施内容**:
  1. `platform/surface.go` 新增 `DecoMode() uint32` 到 Surface 接口
  2. `platform/wayland/surface.go` 的 `DecoMode()` 已存在，确认满足接口
  3. `platform/drm/surface.go` 新增 `DecoMode() uint32` 返回 2（SSD）
  4. GBM surface 如有也需实现
- **验证标准**: `go build ./...` 通过，无编译错误
- **审计结果**: 通过。Surface 接口新增 DecoMode() uint32；DRM/GBM 返回 2(SSD)；Wayland 已有实现；perf/replay fakeSurface 返回 0。go build 通过。

### 阶段 2: wl.go 新增 xdg_toplevel move/resize/setMinSize/setMaxSize
- **状态**: 已审计
- **目标**: Wayland 协议层新增窗口移动/缩放/尺寸限制请求
- **实施内容**:
  1. `wlXdgToplevel` 新增 `move(seatID, serial)` — opcode 4
  2. `wlXdgToplevel` 新增 `resize(seatID, serial, edge)` — opcode 5
  3. `wlXdgToplevel` 新增 `setMinSize(w, h)` — opcode 6
  4. `wlXdgToplevel` 新增 `setMaxSize(w, h)` — opcode 7
- **验证标准**: `go build ./...` 通过
- **审计结果**: 通过。wlXdgToplevel 新增 move(5)/resize(6)/setMaxSize(7)/setMinSize(8) 4个请求方法。go build 通过。

### 阶段 3: OSD CSD 按钮渲染
- **状态**: 实施中
- **目标**: OSD 在 CSD 模式下标签栏右侧绘制按钮，提供按钮区域查询
- **实施内容**:
  1. OSD 新增 `csdMode bool` 字段 + `SetCSDMode(bool)` 方法
  2. `layoutTabs()` CSD 模式下标签截断到 `width - 3*cellW`，右侧预留按钮区
  3. `RenderCPU()` CSD 模式下绘制 3 个按钮（背景 + 符号字形）
  4. `RenderGPU()` 同步实现 CSD 按钮的 CellInstance
  5. 新增 `CsdButtonRects(width int) [3]image.Rectangle` 返回按钮像素矩形
  6. 按钮符号：`─`（最小化）、`□`（最大化）、`✕`（关闭）
  7. 关闭按钮背景色区分（hover 暂不实现，静态红色背景）
- **验证标准**: `go build ./...` 通过
- **审计结果**: 通过。OSD 新增 csdMode/SetCSDMode/CsdEnabled/csdButtonsWidth/layoutCsdButtons/CsdButtonRects/HitTestTabBar；layoutTabs 新增 csdWidth 参数；RenderCPU/RenderGPU 插入 CSD 按钮渲染。修正 HitTestTabBar 非 CSD 模式也返回 TabBarArea。go build + go test 通过。

### 阶段 4: Slave DecoMode 感知 + 注入 OSD
- **状态**: 实施中
- **目标**: Slave 感知 Surface 的 DecoMode 并注入 OSD
- **实施内容**:
  1. `Slave` 新增 `SetCSDMode(csd bool)` 方法
  2. `InitIndependent` 后检查 `surface.DecoMode()`，设置 `osd.csdMode`
  3. `CheckInsetsChanged` 中检测 DecoMode 变化触发 dirty
- **验证标准**: `go build ./...` 通过
- **审计结果**: 通过。Slave 新增 prevCsdMode/CsdMode()；InitIndependent 初始化 CSD；CheckInsetsChanged 检测 DecoMode 变化并同步 osd。go build 通过。

### 阶段 5: Input serial 暴露 + MouseEvent 扩展
- **状态**: 实施中
- **目标**: WaylandInput 暴露 serial 和 seat 引用，MouseEvent 携带 serial
- **实施内容**:
  1. `platform/input.go` MouseEvent 新增 `Serial uint32` 字段
  2. `WaylandInput` 新增 `lastSerial uint32`，onButton/onMotion 时更新
  3. `WaylandInput` 新增 `Seat() *wlSeat` 方法
  4. `WaylandInput` 新增 `LastSerial() uint32` 方法
  5. `WaylandSurface` 新增 `StartMove(seat *wlSeat, serial uint32)` 方法
  6. `WaylandSurface` 新增 `StartResize(seat *wlSeat, serial uint32, edge uint32)` 方法
- **验证标准**: `go build ./...` 通过
- **审计结果**: 通过。MouseEvent 新增 Serial uint32；WaylandInput 记录 lastSerial 并暴露 Seat()/LastSerial()；WaylandSurface 新增 StartMove/StartResize。go build 通过。

### 阶段 6: 鼠标事件处理
- **状态**: 实施中
- **目标**: 主循环处理鼠标事件，实现 CSD 按钮点击和窗口拖拽
- **实施内容**:
  1. `Master` 新增 `mouseEvCh chan platform.MouseEvent`
  2. `inputLoop` 同时监听 `input.MouseEvents()`，转发到 `mouseEvCh`
  3. 主循环 `select` 新增 `mouseEvCh` 分支
  4. `handleMouse(ev)` 逻辑：
     - CSD 模式下检查鼠标是否在标签栏区域
     - CSD 按钮点击：× → signalClose()，□ → 最大化/还原，─ → 暂不实现（无标准协议）
     - 标签栏空白区域左键按下 → surface.StartMove()
     - 标签点击 → 切换标签
  5. DRM 后端鼠标事件透传（无 CSD 逻辑）
- **验证标准**: `go build ./...` 通过
- **审计结果**: 通过。platform 新增 WindowMover 接口；WaylandSurface StartMove/StartResize 改为从 backend.seat 获取 seat；Master 新增 mouseEvCh + handleMouse（CSD 关闭按钮+拖拽移动）；修复 3 个 test fakeSurface 缺失 DecoMode；handleMouse 增加 CsdMode 检查防止 SSD 误触。go build + go vet + go test 全部通过。

### 阶段 7: 构建验证 + vet + 测试
- **状态**: 已完成
- **目标**: 全面验证无回归
- **实施内容**:
  1. `go build ./...`
  2. `go vet ./...`
  3. `go test ./...`
  4. 修复所有问题
- **验证标准**: 全部通过
- **审计结果**:

## 变更记录
| 时间 | 阶段 | 操作 | 备注 |
|------|------|------|------|
| 2026-07-02 | - | 创建实施方案文档 | |
| 2026-07-02 | P1 | 实施完成 | Surface 接口 + DecoMode |
| 2026-07-02 | P2 | 实施完成 | wl.go move/resize/setMinSize/setMaxSize |
| 2026-07-02 | P3 | 实施完成 | OSD CSD 按钮渲染 |
| 2026-07-02 | P4 | 实施完成 | Slave DecoMode 感知 |
| 2026-07-02 | P5 | 实施完成 | Input serial + MouseEvent |
| 2026-07-02 | P6 | 实施完成 | 鼠标事件处理 |
| 2026-07-02 | P7 | 审计通过 | go build + go vet + go test 全部通过 |
