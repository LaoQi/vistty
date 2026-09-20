# Bell（终端响铃）支持 — 技术探索评估

> **状态：仅技术探索，不实施。** 本文档为后续实施时的参考资料。
>
> **评估日期：2026-09-21**

## 一、背景

BEL（`0x07`，`\a`）是最古老的终端控制字符：默认响铃（或视觉闪烁），并被大量程序用作「需要你注意」的信号——`readline` 补全失败、`vim`/`less` 报错或滚到边界、`sudo` 密码提示、`printf '\a'` 之类的显式提醒。

现代终端对其有三种常见表现（tmux/iTerm2/foot 语义）：

| 形式 | 说明 |
|------|------|
| audible bell | PC speaker（`KDMKTONE`）或播放音频文件 |
| visual bell / flash | 全屏反色闪烁一瞬 |
| bell mark | 非当前窗口/标签上打标记（tab 高亮、标题加 `*`），切回时清除 |
| 无 | 直接丢弃（许多现代终端的默认） |

并配套「bell-action」策略决定哪些终端的响铃可见（tmux：`any` / `none` / `current` / `other`）。

## 二、现有架构分析

### 2.1 解析链路已完备，消费端静默丢弃（关键结论）

`vte` 已把 BEL 规范化为 `ActionExecute{Command: 0x07}`，`terminal.execControl` 中对应分支是**空 case**：

```go
// terminal/terminal.go:559
case vte.ControlBEL:
```

各上下文现状（已按代码路径实测）：

| 上下文 | 代码位置 | 产出 | 语义是否正确 |
|--------|---------|------|------------|
| Ground 态 `\a` | `internal/vte/parser.go:119-131`（`b < 0x20` → ActionExecute） | `ActionExecute 0x07` → `ControlBEL` | ✅ 应响铃 |
| CSI 参数态中的 `\a` | `internal/vte/parser.go:267-271` | 同上 | ✅ 与 xterm 一致（C0 在 CSI 中立即执行） |
| CSI 中间态中的 `\a` | `internal/vte/parser.go:315-320` | 同上 | ✅ |
| ESC / CSI Entry 中间态中的 `\a` | `internal/vte/parser.go:206-209`、`internal/vte/parser.go:223-226` | 同上 | ✅ |
| OSC 终止符 `\a`（`ESC ]0;t\a`） | `internal/vte/parser.go:341-346` | 仅终止 OSC，**不**产 Execute | ✅ 终止符，不算响铃 |
| DCS 终止符 `\a` | `internal/vte/parser.go:361-366` | 同上 | ✅ |

**结论：检测层零改动，bell 支持完全落在消费层（terminal 回调）+ 表现层（session/ui/render）。** 这是本次评估最重要的结论——不存在类似 Sixel 的 64KB payload 那种结构性障碍（对比 `sixel-evaluation.md` §2.1）。

### 2.2 现有可复用机制

| 机制 | 位置 | 对 bell 的复用价值 |
|------|------|------------------|
| 终端事件回调（`OnTitle`/`OnDefaultColor`/`OnCursorColor`） | `terminal/options.go:10-25`、`terminal/terminal.go:1505-1543` | bell 回调照抄此模式，主线程契约一致 |
| 主循环 ticker + dirty 跳帧 | `session/render_loop.go:68-70`、`112-176`、`159` | 视觉 bell 的过期/相位刷新挂在这里 |
| 过期浮层（`ExpirableOverlay`） | `internal/render/overlay.go:39-45`、`session/master.go:840-861` | 若要 Toast 式提示可直接复用 |
| 墙钟纯函数相位（光标闪烁） | `internal/render/compositor.go:233-258` | 闪烁相位照此实现，避免累积状态抖动 |
| lifecycle Lua 钩子框架 | `internal/plugins/api_lifecycle.go:22-36`、`internal/plugins/manager.go:126-190` | `vistty.on_bell` 是第 13 个钩子，模式现成 |
| Tab 标题/主题/布局 | `internal/ui/tabbar.go:12-15,74-95,218-319`、`session/slave.go:160-172` | tab 标记式视觉 bell |

## 三、语义边界

1. **必须节流**：失控脚本可每秒产出数万 BEL（`yes $'\a'`）。xterm/VTE 均有最小间隔限制。建议单终端最小间隔 100ms，超限丢弃并计数（可选 debug 日志）。
2. **OSC/DCS 终止符不响铃**（现状已正确，须加测试锁定该行为）。
3. **与 `\e[?5h`（DECSCNM 反显）无关**：那是独立的显示模式，见 §4 P3。
4. **焦点策略**：多 tab / 多屏下必须定义「谁响铃」。建议沿用 tmux 的三档：`focus`（仅当前焦点终端，等同 xterm 语义）/ `any`（任何终端；非焦点终端以 tab 标记提示）/ `none`（仅触发 Lua 钩子）。
5. **清除时机**（`any` 模式）：切换到该 tab，或该终端收到键盘输入时，清除其 bell 标记。

## 四、方案分档

### P0 — 核心通路（无表现形式，纯基础设施）

* `terminal.Options`（`terminal/options.go:10-25`）增 `OnBell func()` + `BellMinInterval time.Duration`（默认 100ms，测试可注入 0）。
* `Terminal.SetOnBell(f func())`；`execControl` 的 `case vte.ControlBEL:`（`terminal/terminal.go:559`）改为 `t.notifyBell()`。
* `notifyBell()`：单终端节流（`lastBell` + 时钟），未超限才回调。回调发生在 `Apply()` 内、由主循环串行调用（`session/render_loop.go:107`），与 `OnTitle` 同一契约——**可直接操作 UI 状态**。
* 可测试性：把时钟抽为 `t.now func() time.Time`（默认 `time.Now`），测试注入假时钟。
* 估算：~40 行实现 + ~60 行测试。

> P0 单独落地即有价值（`OnBell == nil` 时是 no-op，零行为变化），插件可在钩子里做任意事（见 P2b）。

### P1 — 视觉 bell

**P1a：Tab 标记（推荐默认，成本最低）**

* `ui.Tab`（`internal/ui/tabbar.go:12-15`）增 `Bell bool`；`TabBarTheme`（`74-95`）增 `BellBg/BellFg`（默认取反色或 `ActiveBg` 加亮）。
* `TabBar.layoutTabs`（`internal/ui/tabbar.go:218-319`）对 `Bell && i != active` 的标签使用 bell 配色，或标题前缀 `*`（`truncateTabTitle` 已处理列宽截断）。
* `Terminal` 增 `MarkBell()/BellMarked()/ClearBellMark()`：状态放终端对象上，生命周期天然跟随，避免 slave 切片增删导致索引错位。
* `session/master.go:210-222` 路由 + `slave.UpdateTabs`（`session/slave.go:160-172`）读取标记。
* 估算：~120 行 + ~80 行测试。

**P1b：全屏反色闪烁（可选，成本最高）**

* `Compositor` 增 `bellUntil time.Time` + `SetBellFlash(d time.Duration)`；`Render()`（`internal/render/compositor.go:261-478`）在 cell 绘制完成后、Pass2 浮层之前施加反色。
* CPU 路径：逐像素 `XOR 0x00FFFFFF`（BGRA，不动 alpha）。1920×1080 ≈ 2M 像素、~8MB/帧内存流量；dumb buffer（`DirectRender()==false`，设备内存）需实测，预估 3-10ms/帧。
* GPU 路径：GLES 无 `glLogicOp`，需新增全屏 quad + `glBlendFunc(GL_ONE_MINUS_DST_COLOR, GL_ZERO)` 实现 `dst' = 1 - dst`。当前 `gpu.Renderer` 只有 instanced glyph/cell 路径（`internal/platform/gpu/renderer.go:394-411` 的 `DrawInstancesBlended`），需新增 `FillScreenInvert()` 并自理 blend 状态保存/恢复——**这是唯一有状态机风险的点**（Pass2 依赖 `SRC_ALPHA/ONE_MINUS_SRC_ALPHA`）。
* 渲染驱动：需要独立小 ticker（如 100ms）在闪烁期内强制 `m.dirty = true`（现有 `cursorBlinkTicker` 500ms 粒度太粗，`session/render_loop.go:159`）；相位用墙钟纯函数（照 `internal/render/compositor.go:241-258` 写法），`deadline` 到期自动停止并强制一次全量重绘。
* 估算：~250-350 行 + 像素级测试（可复用 `internal/render/compositor_cpu_test.go` 的黄金帧模式）。

### P2 — 音频 bell

**P2a：PC speaker（内置、可选、能力探测）**

```go
const kdmktone = 0x4B30 // /usr/include/linux/kd.h:26；x/sys/unix 未导出该常量
fd, err := unix.Open("/dev/tty0", unix.O_WRONLY|unix.O_NOCTTY, 0)
unix.IoctlSetInt(fd, kdmktone, (ms<<16)|hz)
```

* **权限现实**：本机 `/dev/tty0` 为 `crw------- root root`；典型用户仅在 `video/audio/input` 组，**默认打不开**，需 root 或加入 `tty` 组。注意这与 DRM 后端不等价（DRM 只需 `video` 组）。
* **硬件现实**：现代笔记本多无 PC 蜂鸣器（或 `pcspkr` 模块被屏蔽），ioctl 成功也可能无声。
* 结论：作为**可选能力**实现（打开失败静默降级 + debug 日志），不能作为 bell 主力。
* 估算：新增 `internal/bell/pcspeaker.go` ~70 行 + 探测单例 + 测试（以失败路径为主）。

**P2b：Lua 钩子 + 非阻塞外部播放器（推荐主力）**

* `vistty.on_bell(fn)` → `fn(tab_idx, focused)`，纯属插件框架既有模式（`internal/plugins/api_lifecycle.go` + `internal/plugins/manager.go` 的 `bellHooks` 字段 + `FireBell` + `Reload` 保存/恢复）。
* Go 侧提供**非阻塞**播放：`vistty.bell.play(path)`（内部 `exec.Command("paplay"/"pw-play"/"aplay").Start()` + 自动探测可用命令），或更通用的 `vistty.bell.spawn(cmd, ...args)`。
  * 必须非阻塞：钩子运行在主渲染线程，`os.execute` 会卡帧；文档示例也要显式提醒（写 `&` 或走 `vistty.bell.play`）。
* 估算：钩子 ~40 行、`play` ~60 行 + 测试。

**明确不做**：自研 ALSA / PulseAudio PCM 输出。ALSA 需要 `SNDRV_PCM_IOCTL_*` 一整套 ioctl 封装（数百行且硬件相关），PulseAudio/PipeWire 是 socket 协议，成本同样远超 bell 本身价值。

### P3 — 相关扩展（按需）

* `DECSCNM`（`\e[?5h/l`）反显模式：`handleMode`（`terminal/terminal.go:756-832`）当前未处理 `case 5`（`insertMode`/`cursorKeysApp`/`autoWrap` 等已处理）。实现后可复用 P1b 的反色原语，且是独立价值项：`vim` 的 `visualbell`、`screen`/`tmux` 都依赖它。
* OSC 777 / OSC 9 桌面通知：与 bell 是同类「注意我」语义，可一并规划（`execOSC` 当前只处理颜色/标题/剪贴板类序列，`terminal/terminal.go:834-900`）。

## 五、配置与 Lua API 设计（建议）

```lua
vistty.config.bell                 = "none" -- none | tab | flash | tab+flash | audio | both
vistty.config.bell_action          = "any"  -- focus | any | none（视觉/音频触发范围）
vistty.config.bell_speaker         = false  -- 是否尝试 PC speaker（需 /dev/tty0 权限）
vistty.config.bell_flash_ms        = 200    -- flash 单次时长
vistty.config.bell_min_interval_ms = 100    -- 单终端最小间隔（防刷屏）
```

```lua
vistty.on_bell(function(tab_idx, focused)
	if focused then
		vistty.bell.play("/usr/share/sounds/freedesktop/stereo/bell.oga")
	end
end)
```

* 字段落在 `plugins.RunConfig`（`internal/plugins/config.go:15-27`）+ `readConfig`（`internal/plugins/config.go:54`）+ `cmd/vistty/main.go:79-86` 的 `opts` 装配，与 `scrollback`/`primary` 同路径（3 处机械改动，低风险）。
* **默认值建议 `bell = "none"`**：与当前行为完全一致，避免升级后突然打扰；示例 lua 里给 `tab` 推荐值。若要「开箱可见」，`bell = "tab"` + `bell_action = "any"` 干扰也很低（仅标签上多一个标记）。

## 六、关键技术细节与风险

| 项 | 说明 | 风险 |
|----|------|------|
| 渲染线程契约 | `Apply()` 只在主循环调用（`session/render_loop.go:107`），故 `OnBell` 回调在主线程 | 低；需在注释中固化该契约，未来若并发 `Apply` 需改为 pending 队列（参照 `titlePending`） |
| 节流位置 | 单终端节流放 terminal 层，全局/焦点决策放 session 层，避免双重节流语义混乱 | 低 |
| 多屏路由 | flash 只作用于发声终端所在 `Slave` 的 `Compositor`；音频全局 | 低；需 `Master` 反查 term→slave（遍历 `slaves[].terms` 即可） |
| GPU 反色 blend | `GL_ONE_MINUS_DST_COLOR` 不能污染 Pass2 的 `SRC_ALPHA` 状态 | **中**；需显式保存/恢复 + `gbm-bench.sh` 实测无回归 |
| dumb buffer 反色成本 | 设备内存 mmap 写入慢于堆内存 | 中；需实测，超标则降级为「只闪 tab / 只闪边框」 |
| PC speaker 权限 | `/dev/tty0` 需 root 或 `tty` 组，且多数硬件无声 | 中；靠能力探测 + 静默降级 |
| `os.execute` 阻塞 | 钩子在主渲染线程，播放必须非阻塞 | 中；靠 `vistty.bell.play` 规避 + 文档提醒 |
| 与 `vim`/`less` 交互 | 二者视觉 bell 常配 DECSCNM；未实现 P3 时表现为纯 BEL | 低 |

## 七、测试计划

* `internal/vte`：断言 `\a` 在 ground / CSI 中产 `ActionExecute 0x07`；在 OSC/DCS 中作终止符且**不**产 Execute（锁定语义防回归）。
* `terminal`：`"a\ab"` → OnBell 恰 1 次；假时钟下间隔 10ms 连发 3 次 → 仅 1 次回调；`\e]0;t\a` 标题序列 → 0 次；alt screen 中 `\a` → 1 次。
* `session`：`bell_action` 三档 × 焦点/非焦点终端的路由断言（参照 `session/master_test.go` 风格）。
* `render`：反色原语表驱动 + 边界（BGRA / stride 非 4 倍数）+ 黄金帧（闪烁前后逐字节）；tab 标记配色像素断言。
* 手工：`printf '\a'`；`vim`（`:set belloff=` / `vb`）；`htop` 中无效键；`yes $'\a' | head -c 2000` 类刷屏压测观察节流。

## 八、工作量与推荐路线

| 阶段 | 内容 | 估算（含测试） | 依赖 |
|------|------|---------------|------|
| P0 | terminal 回调 + 节流 | ~100 行 / 0.5 天 | — |
| P2b | `vistty.on_bell` + `vistty.bell.play` | ~100 行 / 0.5 天 | P0 |
| P1a | tab 标记 + 配置项 | ~200 行 / 1 天 | P0 |
| P3 | DECSCNM `\e[?5h` | ~150 行 / 0.5 天 | 反色原语 |
| P1b | 全屏反色闪烁（CPU + GPU） | ~300 行 / 1.5 天 | P0 |
| P2a | PC speaker | ~100 行 / 0.5 天 | P0 |

**推荐路线：P0 → P2b（钩子，立刻可用且零打扰）→ P1a（tab 标记）→ P3（DECSCNM，顺带产出反色原语）→ P1b（flash）→ P2a（PC speaker）。**

理由：P0+P2b+P1a 已覆盖绝大多数实际需求（可见提示 + 可自定义声音），且全部是低风险改动；P1b 的反色原语先在 P3 落地更稳（DECSCNM 是显式模式切换，不需要 ticker/相位/过期机制），再复用到 flash；PC speaker 收益最低（权限 + 硬件双重限制），放最后。

**默认建议保持 `bell = "none"`，由用户显式开启**，与项目「无侵入、可配置」的一贯风格一致。

## 九、关键代码位置索引

| 位置 | 说明 |
|------|------|
| `internal/vte/parser.go:119-131` | `feedGround()` — ground 态 C0 → `ActionExecute`（BEL 入口） |
| `internal/vte/parser.go:206-209`、`223-226` | ESC / CSI Entry 中间态 C0 → `ActionExecute` |
| `internal/vte/parser.go:267-271`、`315-320` | CSI 参数态 / 中间态 C0 → `ActionExecute` |
| `internal/vte/parser.go:341-346` | `feedOSCString()` — OSC 的 BEL 终止符（**不**产 Execute，须保持） |
| `internal/vte/parser.go:361-366` | `feedDCSString()` — DCS 的 BEL 终止符（同上） |
| `internal/vte/control.go:25-26` | `ParseControl()` — `0x07` → `ControlBEL` |
| `terminal/terminal.go:539-566` | `execControl()` — **`case vte.ControlBEL:` 空 case（改动点）** |
| `terminal/options.go:10-25` | `Options` — 增 `OnBell` / `BellMinInterval` |
| `terminal/terminal.go:1505-1543` | `SetOnDefaultColor/SetOnCursorColor/SetOnTitle` — 回调注册模式参照 |
| `terminal/terminal.go:756-832` | `handleMode()` — P3 DECSCNM 增 `case 5`（当前无） |
| `terminal/terminal.go:834-900` | `execOSC()` — P3 OSC 777/9 通知的接入点 |
| `session/master.go:210-222` | `bindTerminalCallbacks()` — 注册 `SetOnBell` 的位置 |
| `session/master.go:840-861` | `cleanupOverlays()` — 过期浮层清理（Toast 式提示复用点） |
| `session/slave.go:160-172` | `UpdateTabs()` — tab 标记同步点 |
| `session/render_loop.go:107` | `msg.term.Apply()` — 主线程调用点（回调契约来源） |
| `session/render_loop.go:68-70,112-176` | ticker + dirty 跳帧主循环（视觉 bell 的驱动位点） |
| `session/render_loop.go:159` | `case <-cursorBlinkTicker.C` — 相位刷新先例 |
| `internal/render/compositor.go:233-258` | `NoteActivity()` / `cursorVisible()` — 墙钟纯函数相位写法 |
| `internal/render/compositor.go:261-478` | `Render()` — CPU 合成路径（flash 反色注入点） |
| `internal/render/compositor.go:482-676` | `renderGPU()` — GPU 合成路径（flash 反色注入点） |
| `internal/render/overlay.go:30-45` | `FloatingOverlay` / `ExpirableOverlay` 接口 |
| `internal/platform/gpu/renderer.go:394-411` | `DrawInstancesBlended()` — blend 状态管理参照 / `FillScreenInvert` 新增点 |
| `internal/ui/tabbar.go:12-15` | `Tab` 结构 — 增 `Bell bool` |
| `internal/ui/tabbar.go:74-95` | `TabBarTheme` + 默认值 — 增 bell 配色 |
| `internal/ui/tabbar.go:218-319` | `layoutTabs()` — bell 标记渲染 |
| `internal/ui/tabbar.go:320-428` | `RenderCPU()`（320）/ `RenderGPU()`（356）— tab 渲染双路径 |
| `internal/plugins/api_lifecycle.go:22-36` | lifecycle 钩子注册表（增 `on_bell`） |
| `internal/plugins/manager.go:126-190` | `fireHooks()` + `Fire*` 系列（增 `FireBell`） |
| `internal/plugins/config.go:15-27,54` | `RunConfig` + `readConfig()` — 新增 bell 配置字段 |
| `cmd/vistty/main.go:79-86` | `opts` 装配（RunConfig → `terminal.Options`） |
| `/usr/include/linux/kd.h:25-26` | `KIOCSOUND` 0x4B2F / `KDMKTONE` 0x4B30（x/sys/unix 未导出） |

## 十、参考

- **xterm `charproc.c` / `xterm.h`** — BEL 处理、`bellPercent`/`bellSuppressTime`（节流先例）、`visualBell`/`bellOnActivity` 选项
- **tmux `options-table.c`（`bell-action` / `visual-bell` / `bell-on-alert`）** — bell-action 三档策略与「非当前窗口标记」语义来源
- **foot `terminal.c`（`term_bell()`）** — 极简终端对 bell 的处理（回调给上层 + 配置化）
- **iTerm2 / WezTerm 文档** — tab 上 bell 标记（`*` / 高亮）的交互惯例
- **golang.org/x/sys/unix** — `IoctlSetInt` / `Open`；`KDMKTONE` 需自定义常量（未导出）
- **Linux `Documentation/admin-guide/`+`include/uapi/linux/kd.h`** — `KDMKTONE` 参数格式（低 16 位频率 Hz、高 16 位时长 ms）与 `/dev/ttyN` 权限要求
- **本项目** `work_docs/analysis/sixel-evaluation.md` — 同类型「仅探索不实施」评估文档的写法参照
