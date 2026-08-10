# 终端配色主题 实施方案

## 概述

为终端引入可配置配色主题系统，覆盖终端 cell（defFg/defBg/16色ANSI调色板）、光标色、OSD 标签栏/CSD 按钮颜色。全部通过 plugin 的 Lua 层配置，支持启动时静态配置与运行时动态切换。

**当前现状**：
- 默认前景/背景色硬编码在 3 处（`terminal.go:104-105` New、`terminal.go:791-792` fullReset、`compositor.go:60-63` NewCompositor），需手动同步
- 16 色 ANSI 调色板（`ansiColor()` terminal.go:1151-1174）为局部硬编码数组，不可配置
- OSD 颜色（`osd.go:23-35`）8 个包级变量硬编码，无 setter
- 光标色复用 defFg（CPU 路径 compositor.go:339），GPU 路径仅反转 cell fg/bg（compositor.go:501-509），无 OSC 12 解析
- 无任何 theme/palette 抽象系统

**决策**：
- 范围：终端 defFg/defBg/cursorColor/16色调色板 + OSD 8 色 + 光标色
- 动态切换：`vistty.theme.apply(table)` 运行时切换，通过 channel 投递主线程串行应用
- 预设主题：放 Lua 侧 `examples/themes/*.lua`（与 init.lua 同目录），通过 `require` 加载，Go 不内置预设
- OSC 冲突：主题切换覆盖 OSC 10/11/12 运行时修改（重置为 theme 值），程序可后续再次 OSC 修改
- Fallback：Lua 未配置时 Go 层用 `DefaultTheme`/`DefaultOSDTheme` 兜底；部分配置时字段级 fallback
- 顺便修复：`Compositor.defColor` 加锁（已存在的潜在 data race：OSC 回调在 seqRelay goroutine，渲染在 main goroutine）

## Fallback 设计

### 三层兜底链

```
Lua 未配置 vistty.config.theme
  → readConfig: RunConfig.Theme = nil
    → cmd/vistty: Options.Theme = nil
      → terminal.New(): opts.Theme == nil → 使用 terminal.DefaultTheme
        → compositor: Terminal.New() 末尾推送覆盖 NewCompositor 初始默认值
      → ui.NewOSD(): osdTheme == 零值 → 使用 ui.DefaultOSDTheme
```

### 字段级 fallback

用户在 Lua 中可能只配置部分字段（例如只改 fg/bg，不改 palette）。解析时对每个字段单独兜底：fg/bg/cursor 缺失用 DefaultTheme 对应字段；palette 缺失或不足 16 项整体用 DefaultTheme.Palette；osd 子表缺失或部分字段缺失用 DefaultOSDTheme 对应字段。

### 默认值来源

| 默认值 | 来源 | 目标 |
|--------|------|------|
| `terminal.DefaultTheme.DefFg` | `terminal.go:104` `{204,204,204}` | 提取 |
| `terminal.DefaultTheme.DefBg` | `terminal.go:105` `{0,0,0}` | 提取 |
| `terminal.DefaultTheme.CursorColor` | `= DefFg`（当前光标复用 defFg） | 新建 |
| `terminal.DefaultTheme.Palette[16]` | `terminal.go:1151-1174` ansiColor 局部数组 | 提取为包级变量 |
| `ui.DefaultOSDTheme` | `osd.go:23-35` 8 个包级变量 | 提取 |

### 行为保证

- 完全未配置主题：行为与当前完全一致（默认值就是现有硬编码值），零回归风险
- 部分配置：缺失字段用默认值补全，不会出现全黑/全零破坏性配色
- `vistty.theme.get()` 未配置时返回 DefaultTheme 对应 table
- `vistty.theme.default()` 显式返回默认主题 table，可 `apply(default())` 重置

## 实施阶段

### 阶段 1: terminal.Theme + Terminal 改造 + OSC 12 + Compositor cursor
- **状态**: 已审计
- **目标**: 在 terminal 层引入 Theme 结构体，消除硬编码默认色/调色板，支持 OSC 12 光标色，Compositor 渲染光标用专用 cursorColor
- **实施内容**:
  - 新增 `terminal/theme.go`：
    - `Theme` 结构体：`DefFg, DefBg, CursorColor screen.Color` + `Palette [16]screen.Color`
    - `DefaultTheme` 包级变量：提取自 terminal.go:104-105 的 defFg/defBg + terminal.go:1151-1174 的 ansiColor 16 色数组；CursorColor = DefFg
  - 修改 `terminal/options.go:10-21`：
    - Options 新增 `Theme *Theme` 字段
    - Options 新增 `OnCursorColor func(screen.Color)` 回调
  - 修改 `terminal/terminal.go`：
    - Terminal struct 新增 `theme *Theme` 字段 + `cursorColor screen.Color` 字段
    - `New()`: opts.Theme == nil 时用 `&DefaultTheme`（拷贝避免共享 mutation）；defFg/defBg/cursorColor 初始化自 theme（替代 terminal.go:104-105 硬编码）
    - `New()` 末尾立即调 `OnDefaultColor(defFg,defBg)` + `OnCursorColor(cursorColor)` 推送初始值（消除 compositor 硬编码依赖）
    - `ansiColor()`（terminal.go:1151-1174）改为 `(t *Terminal) ansiColor(idx)` 方法，读 `t.theme.Palette[idx]`；包级 `ansiColor` 删除
    - `color256()`（terminal.go:1176-1205）改为方法 `(t *Terminal) color256(idx)`，内部 `t.ansiColor`
    - `applySGR`（terminal.go:959-1014）调用改为 `t.ansiColor`/`t.color256`
    - `fullReset()`（terminal.go:791-794）用 `t.theme` 重置 defFg/defBg/cursorColor（替代硬编码）
    - 新增 `SetTheme(theme *Theme)` 方法：更新 t.theme + defFg/defBg/cursorColor + 触发 OnDefaultColor + OnCursorColor 回调
    - 新增 `SetOnCursorColor(f func(screen.Color))` setter
    - `execOSC`（terminal.go:689-695）新增 `case vte.OSCCursorColor:` 分发
    - 新增 `handleOSCCursorColor(data string)`：解析颜色，设 cursorColor，调 OnCursorColor 回调；查询模式 `?` 响应 `\x1b]12;rgb:...`
  - 修改 `internal/vte/osc.go`：
    - OSCCommand 枚举新增 `OSCCursorColor`（在 OSCBgColor 之后）
    - switch（osc.go:51-70）新增 `case 12: return OSCSequence{Command: OSCCursorColor, ...}`
  - 修改 `internal/render/compositor.go`：
    - `defaultColor` struct（compositor.go:12-15）新增 `cursor screen.Color` 字段
    - `NewCompositor`（compositor.go:60-63）初始值 cursor = fg（与当前行为一致）
    - 新增 `SetCursorColor(c screen.Color)` 方法（加 mu 锁，与 SetDefaultColors 一并加锁）
    - `SetDefaultColors`（compositor.go:571-574）加 mu 锁
    - CPU 光标路径（compositor.go:339-340）：颜色改用 `c.defColor.cursor`（替代 resolveFg(IsDefault)）
    - GPU 光标路径（compositor.go:501-509）：改为用 defCursor 设 cell 背景 HasBg=1，保留原 fg 字形（与 CPU 行为一致），不再交换 fg/bg
    - resolveFg/resolveBg（compositor.go:576-588）读取 defColor 加 mu 锁
  - 修改 `terminal/terminal_osc_test.go`：新增 OSC 12 测试用例
  - 修改 `terminal/color256_test.go`：适配 color256 改为方法（构造 Terminal 调用）
- **验证标准**:
  - `go build ./...` 成功
  - `go vet ./...` 无错误
  - `go test ./terminal/ ./internal/vte/ ./internal/render/` 无回归
  - OSC 12 设置/查询测试通过
  - ansiColor 改为方法后 16 色值与原硬编码一致
- **审计结果**: 通过。新增 terminal/theme.go（DefaultTheme Palette 与原 ansiColor 完全一致）；options.go 新增 Theme/OnCursorColor 字段；vte/osc.go 新增 OSCCursorColor + case 12；terminal.go 改造完整（New 拷贝 theme 避免共享 mutation + 末尾推送初始回调消除 compositor 硬编码依赖 + ansiColor/color256 改方法读 t.theme.Palette + fullReset 从 theme 重置 + handleOSCCursorColor 实现 + SetTheme/SetOnCursorColor）。compositor.go 采用快照方案（Render/renderGPU 开头加锁拷贝 dc，resolveFg/resolveBg 改包级函数接收 dc，热路径无锁；SetDefaultColors/SetCursorColor 加锁保证写入原子性）。审计中发现 SetTheme 未加 t.mu 锁（与 seqRelay goroutine 的 Apply 并发访问 t.theme 有 data race），已修复：SetTheme 加 t.mu 保护字段写入，回调在锁外调用（避免持有 t.mu 时调 compositor.mu）。GPU 光标测试 TestRenderGPUCursorSwap 更新为 TestRenderGPUCursor 验证新行为。go build/vet/test 全绿（仅预存在的 TestP6ExampleInitLua 失败，master 分支也失败，与本次无关）。

### 阶段 2: ui.OSDTheme + osd.go 改造
- **状态**: 已审计
- **目标**: OSD 颜色从包级硬编码变量改为 OSD struct 持有的可配置 OSDTheme，支持 SetTheme 运行时变更
- **实施内容**:
  - 新增 `ui/theme.go`：OSDTheme 结构体（8 个 [3]uint8 字段）+ DefaultOSDTheme（值提取自 osd.go:23-35 原包级变量）
  - 修改 `internal/ui/osd.go`：删除 8 个包级颜色变量；OSD struct 新增 theme OSDTheme 字段；NewOSD 追加 osdTheme 参数（零值 OSDTheme{} 兜底 DefaultOSDTheme）；layoutTabs/layoutCsdButtons 改为 *OSD 方法读 o.theme.*；4 处调用点改为 o.layoutTabs/o.layoutCsdButtons；新增 SetTheme 方法
  - 修改 `internal/ui/osd_test.go`：NewOSD 调用追加 OSDTheme{} 参数（测试零值兜底）；layoutTabs 调用改为构造 OSD 实例后调方法；断言读 DefaultOSDTheme.*
  - 修改 `session/slave.go:85`：NewOSD 调用传 ui.DefaultOSDTheme
- **验证标准**:
  - `go build ./...` 成功
  - `go vet ./...` 无错误
  - `go test ./internal/ui/` 无回归
  - OSD 渲染颜色与改造前一致（默认值不变）
- **审计结果**: 通过。theme.go DefaultOSDTheme 与原包级变量逐字段核对一致。osd.go 包级变量已删除（grep 确认无残留），layoutTabs/layoutCsdButtons 方法体内所有颜色引用改为 o.theme.*（ActiveBg/ActiveFg/InactiveBg/InactiveFg/BarBg/CsdBtnBg/CsdCloseBg/CsdBtnFg 全覆盖）。NewOSD 零值兜底逻辑正确（OSDTheme{} 比较）。4 处调用点（RenderCPU×2 + RenderGPU×2）改为方法调用。slave.go 传 DefaultOSDTheme。osd_test.go 全部适配（11 处 NewOSD 调用 + layoutTabs 方法调用 + 断言改读 DefaultOSDTheme）。go build/vet/test 全绿（ui 10/10 通过，session 通过）。注意：零值兜底用 OSDTheme{} 比较，全黑主题（理论上无意义）会被误判兜底，可接受。

### 阶段 3: plugins 层 + readConfig + vistty.theme API
- **状态**: 已审计
- **目标**: 提供 Lua 配置入口（静态 vistty.config.theme + 动态 vistty.theme.* API），解析 Lua table 为 terminal.Theme + ui.OSDTheme，通过 PluginContext 接口下发
- **实施内容**:
  - 修改 `internal/plugins/context.go`：PluginContext 接口新增 `ApplyTheme(term terminal.Theme, osd ui.OSDTheme)` 方法，import 新增 internal/ui
  - 修改 `internal/plugins/config.go`：RunConfig 新增 TermTheme *terminal.Theme + OSDTheme *ui.OSDTheme 字段；readConfig 新增 vistty.config.theme 解析；新增 parseLuaTheme/luaColorToScreenColor/luaColorToArray3 辅助函数（复用 api_ui.go parseColor）；import 新增 terminal/internal/screen/internal/ui
  - 新增 `internal/plugins/api_theme.go`：registerTheme 注册 vistty.theme.apply/get/default；themeToLuaTable 互逆转换；colorToHex/array3ToHex 辅助函数
  - 修改 `internal/plugins/manager.go`：PluginManager 新增 currentTheme/currentOSDTheme 字段；import 新增 internal/ui/terminal
  - 修改 `internal/plugins/api_misc.go`：registerAPIs 增加 registerTheme 调用
  - 修改 `session/master.go`：实现 ApplyTheme（遍历 terms 调 SetTheme + 遍历 slaves 调 osd.SetTheme + dirty）；bindTerminalCallbacks 增加 OnCursorColor 桥接
  - 修改 `cmd/vistty/main.go`：Activate 后若 runCfg.TermTheme != nil 调 m.ApplyTheme 应用初始主题
  - 修改 4 个测试 fakeCtx（p2/p3/p4/p6）补 ApplyTheme 空实现
- **验证标准**:
  - `go build ./...` 成功
  - `go vet ./...` 无错误
  - `go test ./internal/plugins/` 无回归（除预存在 TestP6ExampleInitLua）
  - Lua 手测：vistty.theme.apply({fg="#ff0000",bg="#000000"}) 不报错
  - 字段级 fallback：部分配置不产生全零颜色
- **审计结果**: 通过。api_theme.go registerTheme 实现 apply/get/default 三 API，themeToLuaTable 与 parseLuaTheme 互逆。config.go parseLuaTheme 字段级 fallback 正确（先用 DefaultTheme/DefaultOSDTheme 填充，再逐字段覆盖存在的值）。审计中发现 2 个问题已修复：(1) parseLuaTheme OSD 字段级 fallback bug——当 osd 子表存在但某字段缺失时 luaColorToArray3(L, lua.LNil) 返回全零覆盖默认值，改为先检查 L.GetField 返回值 != lua.LNil 再覆盖；(2) bindTerminalCallbacks 缺少 OnCursorColor 桥接——terminal.SetTheme/OSC 12 触发 OnCursorColor 回调但回调为 nil 导致光标色不同步到 compositor，已增加 SetOnCursorColor 桥接到 slaveComp.SetCursorColor。subagent 提前实现了阶段4的 master.go ApplyTheme + main.go 初始主题应用（本属于阶段4），纳入本阶段审计。ApplyTheme 直接调用（非 channel 投递）是安全的——vistty.theme.apply 在 Lua 钩子中调用（主线程 select），main.go 初始应用在 Run 前主线程，均不跨 goroutine。go build/vet 通过，仅预存在 TestP6ExampleInitLua 失败。

### 阶段 4: session 层 ApplyTheme + 预设主题 + examples + 文档
- **状态**: 已审计
- **目标**: 打通 session 层主题应用链路，创建预设主题文件，更新示例与文档
- **实施内容**:
  - 修改 `session/master.go`（阶段3 subagent 提前实现）：Master 实现 ApplyTheme（遍历 terms 调 SetTheme + 遍历 slaves 调 osd.SetTheme + dirty）；bindTerminalCallbacks 增加 OnCursorColor 桥接（阶段3审计修复）；编译期断言适配
  - 修改 `cmd/vistty/main.go`（阶段3 subagent 提前实现）：Activate 后若 runCfg.TermTheme != nil 调 m.ApplyTheme 应用初始主题
  - 新增 7 个预设主题文件 `examples/themes/*.lua`：dracula/solarized_dark/solarized_light/gruvbox/monokai/nord/one_dark，每个返回完整主题 table（fg/bg/cursor/palette[16]/osd[8色]）
  - 修改 `examples/init.lua`：增加主题配置示例（pcall require 降级 + vistty.config.theme 赋值 + 动态切换注释）
  - 修改 `AGENTS.md`：预留扩展点移除"主题/配色"（已实现）；已实现功能概要增加主题系统描述；包目录结构增加 terminal/theme.go、ui/theme.go、examples/themes/ 目录、api_theme.go
- **验证标准**:
  - `go build ./...` 成功
  - `go vet ./...` 无错误
  - `go test ./...` 无回归（仅预存在 TestP6ExampleInitLua fontsize 失败）
  - 端到端：`go run ./cmd/vistty -backend wayland` 启动后主题生效
  - 预设主题 require 加载成功
  - 动态切换：vistty.theme.apply() 立即生效全屏重绘
  - 未配置主题时行为与改造前完全一致
- **审计结果**: 通过。7 个预设主题文件颜色值正确（标准配色方案）。examples/init.lua 用 pcall(require) 优雅降级（themes/ 不在同目录时 theme=nil，Go 层 DefaultTheme 兜底）。ApplyTheme 直接调用（非 channel 投递）安全——vistty.theme.apply 在主线程 Lua 钩子中调用，main.go 初始应用在 Run 前主线程。bindTerminalCallbacks OnCursorColor 桥接确保光标色同步。全量 go build/vet/test 通过（仅预存在 TestP6ExampleInitLua fontsize want 14 got 24 失败，master 分支也有，与主题无关）。注意：themes/ 目录与 init.lua 同目录（examples/themes/），用户安装时复制到 ~/.config/vistty/themes/，require 通过 package.path 自动加载。

## 变更记录
| 时间 | 阶段 | 操作 | 备注 |
|------|------|------|------|
| 2026-07-05 | - | 撰写实施文档 | 初始方案，4 阶段 |
| 2026-07-05 | 阶段1 | subagent 实施 + 审计 | terminal.Theme + OSC 12 + Compositor cursor；修复 SetTheme 缺少 t.mu 锁 |
| 2026-07-05 | 阶段2 | subagent 实施 + 审计 | ui.OSDTheme + osd.go 改造；8 包级变量→OSD struct theme 字段 |
| 2026-07-05 | 阶段3 | subagent 实施 + 审计 | plugins 层 vistty.theme API；修复 parseLuaTheme OSD 字段 fallback bug + bindTerminalCallbacks 缺 OnCursorColor 桥接；subagent 提前实现 master.go ApplyTheme + main.go 初始应用 |
| 2026-07-05 | 阶段4 | 主 agent 直接实施 + 审计 | 7 个预设主题 + examples/init.lua + AGENTS.md；pcall 降级处理 themes/ require |
