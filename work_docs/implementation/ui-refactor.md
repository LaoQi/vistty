# UI 层重构实施方案

## 概述
将单体 OSD 拆分为 TabBar + StatusBar 两个独立 Overlay，提取 PanelPrimitive 公共包，Compositor 升级多 Overlay 支持；引入 FloatingOverlay 浮层体系支持 Dialog/Toast；重构输入路由为 InputTarget 焦点栈 + IME 输出解耦。

## 实施阶段

### 阶段 1: 精简 — 拆 OSD + 公共包 + 多 Overlay
- **状态**: 已审计
- **目标**: 将单体 OSD 拆为 TabBar + StatusBar，提取 panel 公共包，Compositor 支持多 Overlay
- **审计结果**: go build/vet/test 全部通过，OSD 成功拆分，左/右/上面板预留代码已清除
- **实施内容**:
  1. 新建 `internal/panel/types.go` — PanelPrimitive 公共定义
  2. 新建 `internal/ui/tabbar.go` — TabBar struct 实现 render.Overlay
  3. 新建 `internal/ui/statusbar.go` — StatusBar struct 实现 render.Overlay
  4. 修改 `internal/ui/theme.go` — 拆为 TabBarTheme + StatusBarTheme
  5. 删除 `internal/ui/osd.go`
  6. 修改 `internal/render/compositor.go` — overlay Overlay → overlays []Overlay + AddOverlay/RemoveOverlay
  7. 修改 `internal/render/overlay.go` — 无变化（Overlay 接口不变）
  8. 修改 `session/slave.go` — osd → tabBar + statusBar
  9. 修改 `session/render_loop.go` — 适配多 Overlay + PanelPrimitive 引用
  10. 修改 `session/master.go` — ApplyTheme 适配新 theme 结构
  11. 修改 `internal/plugins/` — OSDTheme → 拆分 theme，PanelPrimitive → panel 包
  12. 更新测试
- **验证标准**: go build ./... 通过，go vet ./... 通过，go test ./... 通过

### 阶段 2: GPU alpha 基础
- **状态**: 已审计
- **目标**: CellInstance 支持 BgA，shader 支持 alpha 输出
- **审计结果**: go build/vet/test 全部通过，HasBg→BgA 重命名完成，shader 输出 max(v_bgA, alpha) 替代硬编码 1.0
- **实施内容**:
  1. 修改 `internal/platform/gpu.go` — CellInstance: HasBg → BgA float32
  2. 修改 `internal/platform/gpu/shader.go` — fragment shader 输出真实 alpha
  3. 修改 `internal/platform/gpu/renderer.go` — DrawInstances 支持 blend 参数
  4. 修改所有使用 HasBg 的代码 → BgA
- **验证标准**: go build 通过，现有 GPU 渲染行为不变（BgA=1.0 等价旧 HasBg=1.0）

### 阶段 3: FloatingOverlay + Compositor Pass 2
- **状态**: 已审计
- **目标**: 定义 FloatingOverlay 接口，Compositor 支持浮层渲染 Pass 2
- **审计结果**: go build/vet/test 全部通过，FloatingOverlay 接口定义，CPU/GPU Pass 2 实现，GL_BLEND 常量+DrawInstancesBlended 方法实现
- **实施内容**:
  1. 修改 `internal/render/overlay.go` — 新增 FloatingOverlay 接口
  2. 修改 `internal/render/compositor.go` — floatingOverlays + Pass 2 渲染
  3. CPU 路径浮层渲染（alpha blend 到 backBuf）
  4. GPU 路径浮层渲染（启用 GL_BLEND 的独立 DrawInstances）
- **验证标准**: go build 通过，现有边栏渲染不受影响

### 阶段 4: InputTarget + 焦点栈 + IME 输出解耦
- **状态**: 已审计
- **目标**: 输入路由从硬编码 terminal 改为 InputTarget 焦点栈，IME 输出解耦
- **审计结果**: go build/vet/test 全部通过，InputTarget 接口定义，焦点栈 Push/Pop/CurrentInputTarget，handleKey 使用焦点栈，vistty.input.commit() Lua API，Terminal.CommitText 实现
- **实施内容**:
  1. 新建 `session/input.go` — InputTarget 接口 + FocusStack
  2. 修改 `session/master.go` — 焦点栈管理
  3. 修改 `session/render_loop.go` — handleKey 使用焦点栈
  4. 修改 `terminal/terminal.go` — 实现 InputTarget (CommitText)
  5. 新建 `internal/plugins/api_input.go` — vistty.input.commit()
  6. 修改 `internal/plugins/context.go` — PluginContext 新增 CommitText
- **验证标准**: go build 通过，现有输入行为不变

### 阶段 5: Dialog/Toast 组件
- **状态**: 已审计
- **目标**: 实现 Dialog 和 Toast 浮层组件
- **审计结果**: go build/vet/test 全部通过，InputField/Toast/Dialog 实现 FloatingOverlay + InputTarget
- **实施内容**:
  1. 新建 `internal/ui/dialog.go` — Dialog 浮层
  2. 新建 `internal/ui/toast.go` — Toast 浮层
  3. 新建 `internal/ui/inputfield.go` — 文本输入框
  4. Dialog/Toast 实现 FloatingOverlay + InputTarget
  5. session 层浮层生命周期管理
- **验证标准**: go build 通过

### 阶段 6: Lua API + ime.lua 适配
- **状态**: 已审计
- **目标**: 暴露 Dialog/Toast/commit API 到 Lua，适配 ime.lua
- **审计结果**: go build/vet/test 全部通过，vistty.ui.toast/dialog/close_dialog API，vistty.input.commit() 替代 vistty.term.send()
- **实施内容**:
  1. 新建 `internal/plugins/api_dialog.go` — vistty.ui.dialog()
  2. 新建 `internal/plugins/api_toast.go` — vistty.ui.toast()
  3. 修改 `examples/ime.lua` — vistty.term.send → vistty.input.commit
  4. 修改 `examples/statusbar.lua` — 适配新 API
- **验证标准**: go build 通过，Lua 脚本功能正常

## 变更记录
| 时间 | 阶段 | 操作 | 备注 |
|------|------|------|------|
| 2026-07-25 | P1 | 拆 OSD → TabBar + StatusBar，panel 公共包，Compositor 多 Overlay | 全部测试通过 |
| 2026-07-25 | P2 | CellInstance HasBg → BgA，shader alpha 输出，DrawInstancesBlended | 全部测试通过 |
| 2026-07-25 | P3 | FloatingOverlay 接口，Compositor Pass 2，GL_BLEND 支持 | 全部测试通过 |
| 2026-07-25 | P4 | InputTarget 接口，焦点栈，vistty.input.commit()，IME 输出解耦 | 全部测试通过 |
| 2026-07-25 | P5 | InputField/Toast/Dialog 组件实现 | 全部测试通过 |
| 2026-07-25 | P6 | vistty.ui.toast/dialog API，ime.lua 适配 commit() | 全部测试通过 |
