# Vistty 插件系统设计方案

## 一、总体架构

通过 gopher-lua（纯 Go，CGO=0 兼容）嵌入 Lua VM，以单入口脚本 `init.lua`（默认 `~/.config/vistty/init.lua`，`-config` flag 指定）取代原 JSONC 配置文件，成为唯一配置源和插件注册点。

```
cmd/vistty/main.go
    └── -config <init.lua 路径> → PluginManager (internal/plugins/)
                            │
                            ├── gopher-lua VM（纯 Go，仅主线程访问）
                            ├── 脚本加载：DoFile(init.lua) 单次执行
                            └── PluginContext 接口（依赖倒置，session.Master 实现）
                                    │
            ┌───────────────────────┼────────────────────────┐
            ▼                       ▼                        ▼
    输入拦截（inputLoop→主线程）   终端/标签/屏幕控制      面板渲染（OSD 聚合）
```

### 核心约束

- **Lua VM 主线程独占**：gopher-lua 非线程安全，所有 Lua 调用经 channel 序列化到主渲染线程。按键延迟最多一帧。
- **单次执行**：init.lua 只 DoFile 一次，配置和钩子注册在同一遍完成；钩子回调延迟到 Master 就绪后由 Go 事件触发。
- **完全开放沙箱**：保留 io/os/loadfile/dofile，插件可模块化、读写文件、执行命令。
- **依赖倒置**：`plugins` 包定义 `PluginContext` 接口，`session.Master` 实现，避免 `plugins→session` 循环依赖。
- **OSD 聚合器**：Compositor 零改动，OSD 内部聚合插件图元，复用现有 FillRect/BlendGlyph/GlyphProvider/GPUGlyphUploader。

## 二、init.lua 执行时序

```
main.go
  ├─ 1. 解析启动类 flag（-backend/-config/-tty/-fps/-profiling/-list-outputs/-record）
  ├─ 2. PluginManager.New(initPath)  -- 创建 VM + 注入全部 vistty.* 命名空间（钩子暂存）
  ├─ 3. pm.Load()  -- DoFile(init.lua) 一次性执行：
  │     ├─ 设置 vistty.config 表
  │     ├─ vistty.input.on_key(fn)  → 暂存到 keyHooks
  │     └─ vistty.ui.on_render(fn) → 暂存到 renderHooks
  ├─ 4. Go 从 Lua state 读 vistty.config → RunConfig
  ├─ 5. 按 RunConfig 创建后端（-backend flag 覆盖 config.backend）
  ├─ 6. session.NewMaster(backend, opts, uiCfg, keybinds)  -- 全部从 RunConfig 构造
  ├─ 7. pm.Activate(m)  -- 绑定 Master，激活钩子（vistty.term.* 等现在可调用）
  └─ 8. m.Run()  -- 主循环
        ├─ 按键: inputLoop→keyEvCh→主线程→pm.OnKey(ev)→handleKey
        └─ 渲染: 主线程→OSD.RenderCPU/GPU→pm.OnRender(side)绘制插件面板
```

init.lua 顶层代码在步骤 3 执行：设置 config + 注册钩子（钩子暂存）。步骤 7 Activate 后，钩子回调由 Go 事件触发执行。init.lua 顶层不应调用 `vistty.term.send()` 等（Master 未就绪，no-op），只注册钩子。

## 三、Lua API（分层命名空间）

### 3.1 常量表

```lua
vistty.keys = {
    -- 修饰键 scancode（用于识别单独按下事件）
    LEFT_CTRL=29, RIGHT_CTRL=97, LEFT_ALT=56, RIGHT_ALT=100,
    LEFT_SHIFT=42, RIGHT_SHIFT=54, LEFT_SUPER=125, RIGHT_SUPER=126,
    -- 系统键
    CAPSLOCK=58, NUMLOCK=69, SCROLLLOCK=70,
    SYSRQ=99, PAUSE=119, PRINT=210, MENU=139,
    -- 转义键（ESCAPE 别名 ESC）
    ESCAPE=1, ESC=1,
    -- 数字键（主键盘区，NUM1..NUM9, NUM0）
    NUM1=2, NUM2=3, NUM3=4, NUM4=5, NUM5=6,
    NUM6=7, NUM7=8, NUM8=9, NUM9=10, NUM0=11,
    -- 符号键（Linux input-event-codes.h 标准命名）
    MINUS=12, EQUAL=13, BACKSPACE=14, TAB=15,
    LEFTBRACE=26, RIGHTBRACE=27, ENTER=28, RETURN=28,
    SEMICOLON=39, APOSTROPHE=40, GRAVE=41, BACKSLASH=43,
    COMMA=51, DOT=52, SLASH=53, SPACE=57,
    -- 字母键（QWERTY 布局，值为 evdev scancode）
    Q=16, W=17, E=18, R=19, T=20, Y=21, U=22, I=23, O=24, P=25,
    A=30, S=31, D=32, F=33, G=34, H=35, J=36, K=37, L=38,
    Z=44, X=45, C=46, V=47, B=48, N=49, M=50,
    -- 功能键
    F1=59, F2=60, F3=61, F4=62, F5=63, F6=64,
    F7=65, F8=66, F9=67, F10=68, F11=87, F12=88,
    -- 方向 / 编辑键
    UP=103, DOWN=108, LEFT=105, RIGHT=106,
    HOME=102, END=107, PAGE_UP=104, PAGE_DOWN=109,
    INSERT=110, DELETE=111,
    -- 小键盘
    KP7=71, KP8=72, KP9=73,
    KP4=75, KP5=76, KP6=77, KPPLUS=78,
    KP1=79, KP2=80, KP3=81, KP0=82, KPDOT=83,
    KPENTER=96, KPSLASH=98, KPASTERISK=55, KPMINUS=74, KPEQUAL=117,
}

vistty.mods = {
    CTRL=1, ALT=2, SHIFT=4, SUPER=8,  -- = platform.Modifiers 位值
}

vistty.state = {
    PRESS="press", RELEASE="release",  -- 与 ev.state 字符串字段直接比较
}

vistty.colors = {
    BLACK="#000000", WHITE="#FFFFFF",
    RED="#FF0000", GREEN="#00FF00", BLUE="#0000FF",
    YELLOW="#FFFF00", CYAN="#00FFFF", MAGENTA="#FF00FF",
    ORANGE="#FF8800", PURPLE="#8844CC", PINK="#FF8888",
    LIME="#88FF00", TEAL="#008888", NAVY="#000088",
    MAROON="#880000", OLIVE="#888800", GRAY="#888888",
    DARKGRAY="#444444", LIGHTGRAY="#CCCCCC",
    BROWN="#884400", SILVER="#C0C0C0", GOLD="#FFD700",
    CORAL="#FF7F50", SALMON="#FA8072", KHAKI="#F0E68C",
    IVORY="#FFFFF0", INDIGO="#4B0082",
    -- 别名
    GREY="#888888", DARKGREY="#444444", LIGHTGREY="#CCCCCC",
}
```

常量表与 Go 侧对齐：`vistty.keys` 的 code 值 = evdev scancode = `KeyEvent.code`，`vistty.mods` = `platform.Modifiers` 位值（ModCtrl=1<<0=1, ModAlt=2, ModShift=4, ModSuper=8），`vistty.state` = `newKeyEventTable` 中 state 字段的字符串值，零转换成本。推荐用 `ev.state == vistty.state.PRESS` 取代字符串字面量比较。

### 3.2 vistty.config（配置表，init 阶段设置）

```lua
vistty.config = {
    backend   = "auto",        -- "auto"|"drm-gbm"|"drm"|"wayland"
    shell     = "/bin/bash",
    font      = "",            -- 空 = 内置 Sarasa
    fontsize  = 14,
    primary   = "",            -- 输出名或索引，空 = 第一个
    error_log = "",            -- 空 = 默认 XDG 路径
    keybindings = {
        zoom_in     = {key="=",     mod="super"},
        zoom_out    = {key="-",     mod="super"},
        zoom_reset  = {key="0",     mod="super"},
        new_tab     = {key="t",     mod="super"},
        close_tab   = {key="w",     mod="super"},
        prev_tab    = {key="Left",  mod="super"},
        next_tab    = {key="Right", mod="super"},
        next_screen = {key="Tab",   mod="super"},
        switch_n    = {key="1-9",   mod="super"},
    },
}
```

> **keybindings.mod 字段**：每个键绑定可显式指定 `mod`（`"super"/"ctrl"/"alt"/"shift"`）。未指定时回退到 `ModSuper`。已移除全局 `mod_key` 配置项——各绑定应显式声明 mod。
>
> **OSD**：顶部标签栏默认开启，无需配置。底/左/右边面板完全由插件系统 `vistty.ui.enable / on_render` 驱动。已移除 `osd` 配置项。

### 3.3 vistty.input（输入拦截）

```lua
vistty.input.on_key(function(ev)
    -- ev = {rune=number, code=number, mods=number, state="press"|"release"}
    --   rune  : Unicode 码点（可打印字符有值，功能键为 0）
    --   code  : evdev scancode（功能键有值，修饰键按下时也有值）
    --   mods  : 修饰键位掩码（CTRL|ALT|SHIFT|SUPER 组合）
    --   state : "press" 或 "release"
    --
    -- 返回值：
    --   true / "consume"      → 消费，不进终端/快捷键
    --   false / nil           → 继续传递
    --   {rune=, code=, mods=} → 改写后的事件（继续传递）

    -- 示例 1：吞掉 Ctrl+Space
    if ev.mods & vistty.mods.CTRL ~= 0 and ev.code == vistty.keys.SPACE then
        return true
    end

    -- 示例 2：F1 插入自定义命令
    if ev.code == vistty.keys.F1 and ev.state == vistty.state.PRESS then
        vistty.term.send("git status\n")
        return true
    end
end)
```

### 3.4 vistty.term（终端控制）

```lua
-- send: 发送 UTF-8 字符串到焦点终端 PTY
vistty.term.send("ls -la\n")            -- 字符串
vistty.term.send("你好")                 -- UTF-8 多字节
vistty.term.send("\x03")                -- Ctrl+C 控制码
vistty.term.send("\x1b[31mred\x1b[0m")  -- 转义序列
vistty.term.send("echo ", arg, "\n")    -- 多参数拼接

-- send_key: 发送功能键转义序列（等价于 Go 侧 WriteKeyEscape）
-- code 必须是 vistty.keys 中的功能键值；mods 支持位组合
-- 不支持可打印字符（code 为可打印字符 scancode 时静默忽略）
vistty.term.send_key(vistty.keys.UP, 0)                          -- \x1b[A
vistty.term.send_key(vistty.keys.UP, vistty.mods.ALT)            -- \x1b\x1b[A
vistty.term.send_key(vistty.keys.ENTER, 0)                      -- \r
vistty.term.send_key(vistty.keys.PAGE_UP, vistty.mods.SHIFT)     -- \x1b[5~
vistty.term.send_key(vistty.keys.DELETE, vistty.mods.CTRL|vistty.mods.ALT)  -- \x1b\x1b[3~

-- 屏幕访问
vistty.term.scroll(n)           -- 绝对滚动偏移
vistty.term.scroll_by(delta)    -- 相对滚动
vistty.term.cols()              -- → number
vistty.term.rows()              -- → number
vistty.term.title()             -- → string
vistty.term.read_screen()       -- → 2D table [row][col]={rune=,fg=,bg=,attr=}
vistty.term.resize(cols, rows)
```

**send vs send_key 分工**：
- `send(s)` 发原始字节/UTF-8 字符串（含控制码如 `\x03`），UTF-8 透传。
- `send_key(code, mods)` 发功能键转义序列（走 `WriteKeyEscape`）。仅 switch case 内的 code 有效（Up/Down/Left/Right/Home/End/PgUp/PgDn/Insert/Delete/Backspace/Tab/Enter），其余静默忽略。mods 中 Alt 位会加 `\x1b` 前缀，Ctrl/Shift 对功能键转义无影响但透传无害。不支持可打印字符。

### 3.5 vistty.tab（标签管理）

```lua
vistty.tab.new()
vistty.tab.close()
vistty.tab.next()
vistty.tab.prev()
vistty.tab.list()   -- → {{title="bash",active=true}, {title="vim",active=false}}
vistty.tab.count()  -- → number
```

### 3.6 vistty.screen（多屏切换）

```lua
vistty.screen.next()
vistty.screen.switch(idx)      -- 切到第 idx 屏
vistty.screen.count()          -- → number
vistty.screen.focused_idx()    -- → number
```

### 3.7 vistty.zoom（缩放）

```lua
vistty.zoom.in() / vistty.zoom.out() / vistty.zoom.reset()
```

### 3.8 vistty.ui（四周面板，仅文本+矩形）

```lua
vistty.ui.enable("bottom", 1)   -- 声明占用底边 1 行（影响 Insets）
vistty.ui.disable("bottom")

vistty.ui.on_render(function(ctx)
    local w, h = ctx:size()
    -- 颜色参数支持：字符串 "#RRGGBB" / "#RRGGBBAA" / vistty.colors.* / table {r,g,b} / {r,g,b,a}
    ctx:rect(0, 0, w, h, {bg=vistty.colors.DARKGRAY})
    ctx:text(2, 0, os.date("%H:%M"), {fg=vistty.colors.YELLOW})
    ctx:text(w-20, 0, "CPU: 23%", {fg="#00FF00FF"})       -- 不透明绿
    ctx:rect(0, 0, w, 1, {bg="#FF000080"})                  -- 半透明红背景
    return true
end)
-- ctx:text(x, y, str, opts)  opts={fg=<color>, bg=<color>, bold=bool}
-- ctx:rect(x, y, w, h, opts) opts={bg=<color>}
-- <color> = "#RGB" | "#RGBA" | "#RRGGBB" | "#RRGGBBAA" | vistty.colors.NAME | {r,g,b} | {r,g,b,a}
-- ctx:size() → width, height  当前面板的 cell 尺寸（列宽×行高单位）
```

### 3.9 其他

```lua
vistty.log("debug message")      -- 写入 vistty 调试日志
vistty.reload()                  -- 重新加载所有插件（清空钩子 + 重新执行 init.lua + Activate）
```

## 四、Go 侧实现

### 4.1 文件结构

```
internal/plugins/
├── manager.go        # PluginManager: VM + Load/Activate/Reload + 配置读取
├── context.go        # PluginContext 接口（Master 实现）
├── config.go         # RunConfig + 从 Lua state 读 vistty.config
├── api_input.go      # vistty.input.on_key + OnKey 分发
├── api_ui.go         # vistty.ui.* + Primitive 收集
├── api_term.go       # vistty.term.send/send_key/scroll/...
├── api_tab.go        # vistty.tab.*
├── api_screen.go     # vistty.screen.*
├── api_zoom.go       # vistty.zoom.*
├── api_misc.go       # vistty.log/reload/keys/mods 常量
└── manager_test.go
```

### 4.2 PluginManager 核心方法

```go
type PluginManager struct {
    L           *lua.LState
    ctx         PluginContext    // Activate 前为 nil
    initPath    string
    keyHooks    *lua.LTable      // 暂存 on_key 函数
    renderHooks *lua.LTable      // 暂存 on_render 函数
    panels      map[string]int   // side → 行数（enable/disable 声明）
    active      bool             // Activate 后 true
}

func NewPluginManager(initPath string) *PluginManager
func (pm *PluginManager) Load() (*RunConfig, error)      // DoFile init.lua + 读 config
func (pm *PluginManager) Activate(ctx PluginContext)      // 绑定 Master，激活钩子
func (pm *PluginManager) Reload() error                   // 清空钩子 + 重新 Load + Activate
func (pm *PluginManager) OnKey(ev platform.KeyEvent) (consumed bool, out platform.KeyEvent)
func (pm *PluginManager) OnRender(side string, width, height int) (dirty bool, primitives []Primitive)
func (pm *PluginManager) EnabledPanels() map[string]int  // OSD 读取
func (pm *PluginManager) Close()
```

### 4.3 PluginContext 接口（`internal/plugins/context.go`）

```go
type PluginContext interface {
    FocusTerm() *terminal.Terminal
    Terms() []*terminal.Terminal
    NewTab() error; CloseCurrentTab(); NextTab(); PrevTab()
    TabList() []TabInfo
    NextScreen(); SwitchScreen(idx int); ScreenCount() int; FocusScreenIdx() int
    ZoomIn(); ZoomOut(); ZoomReset()
    EnablePanel(side string, lines int); DisablePanel(side string)
    ReloadPlugins() error
}

type TabInfo struct {
    Title  string
    Active bool
}
```

### 4.4 RunConfig（`internal/plugins/config.go`）

```go
type RunConfig struct {
    Backend, Shell, FontPath, Primary, ErrorLog string
    FontSize    float64
    Keybindings KeybindTable
}
```

从 Lua `vistty.config` 表读取，`toKeybindTable` 将各键绑定的 `mod` 字段经 `platform.ParseModKey` 转换为 `Modifiers`，未指定 mod 时回退到 `ModSuper`。顶部标签栏默认开启，无需配置；底/左/右边面板由 `vistty.ui.enable` 驱动。

### 4.5 四个注入点

| 注入点 | 文件 | 改动 |
|--------|------|------|
| 配置加载 | `cmd/vistty/main.go` | 删除 config.Load，改 `pm.Load()` 返回 RunConfig |
| 按键拦截 | `session/render_loop.go:306` | inputLoop→keyEvCh→主线程→pm.OnKey→handleKey |
| 面板渲染 | `internal/ui/osd.go` | OSD 持有插件图元，RenderCPU/GPU 末尾绘制；Insets 合并 |
| 入口组装 | `cmd/vistty/main.go:213` | NewMaster 后 `pm.Activate(m)` |

### 4.6 Master 新增导出方法

```
FocusTerm/Terms/NewTab/CloseCurrentTab/NextTab/PrevTab/TabList
NextScreen/SwitchScreen/ScreenCount/FocusScreenIdx
ZoomIn/ZoomOut/ZoomReset
ReloadPlugins/EnablePanel/DisablePanel
```

内部走 channel 投递主线程（renderReqCh/tabReqCh/scaleReqCh），与现有快捷键机制一致，保证并发安全。

### 4.7 OSD 聚合机制

`ui.OSD` 新增字段：
```go
panels     map[string]*PanelData  // side → 插件图元缓存
panelLines map[string]int         // side → 插件声明行数
```

- `SetPanel(side, data)` 由 PluginManager.OnRender 后调用
- `RenderCPU/GPU` 末尾遍历 panels 绘制图元（复用 FillRect/BlendGlyph）
- `Insets()` 合并 panelLines 与 cfg 取 max

### 4.8 按键链路改造

```
[原] inputLoop → handleKey(ev) [inputLoop goroutine]
[新] inputLoop → keyEvCh → 主 select → pm.OnKey(ev) → handleKey(ev) [主线程]
```

- `inputLoop`（render_loop.go:256）收到按键后 `select { case m.keyEvCh <- ev: case <-m.done: }`
- 主 select（render_loop.go:60）新增 `case ev := <-m.keyEvCh:` 分支
- 分支内先 `if m.plugins != nil { ... }` 再调 `handleKey`
- 重复键处理（delay/rate ticker）仍在 inputLoop，生成的重复事件也走 keyEvCh

## 五、命令行 flag 调整

| 保留 | 删除 |
|------|------|
| `-config`（语义改为 init.lua 路径） | `-shell/-font/-fontsize/-primary` |
| `-backend/-tty/-fps` | `-errorlog` |
| `-cpuprofile/-memprofile/-mutexprofile/-trace` | `-gen-config` |
| `-list-outputs/-record` | |

## 六、删除 internal/config 包

完全删除 `internal/config/`（config.go/config_test.go），配置加载移入 `internal/plugins/manager.go`：
- `DefaultInitPath()` 返回 `~/.config/vistty/init.lua`（XDG_CONFIG_HOME，复用原 config.DefaultPath 逻辑）

## 七、依赖

`go.mod` 新增 `github.com/yuin/gopher-lua v1.1.1`（纯 Go，CGO=0 兼容）。

## 八、实施阶段

| 阶段 | 内容 | 文件 |
|------|------|------|
| **P1** | PluginManager + VM + 钩子暂存 + Activate/Reload + vistty.config 读取 + RunConfig + 常量表(keys/mods) + log/reload | `internal/plugins/{manager,context,config,api_misc}.go` |
| **P2** | Master 导出方法 + vistty.term(send/send_key/scroll/...)/tab/screen/zoom.* | `session/master.go`、`internal/plugins/api_{term,tab,screen,zoom}.go` |
| **P3** | inputLoop→keyEvCh + 主 select 分支 + vistty.input.on_key + OnKey 分发 | `session/render_loop.go`、`internal/plugins/api_input.go` |
| **P4** | OSD 聚合 + vistty.ui.* + Primitive 收集 + RenderCPU/GPU 绘制 | `internal/ui/osd.go`、`internal/plugins/api_ui.go` |
| **P5** | 删除 internal/config + main.go 改用 pm.Load() + flag 调整 + examples/init.lua | `cmd/vistty/main.go`、删除 `internal/config/` |
| **P6** | 测试 + 示例插件 | `internal/plugins/manager_test.go` |

## 九、关键设计决策汇总

1. **单次执行**：init.lua 只 DoFile 一次，配置和钩子注册在同一遍完成。
2. **完全开放沙箱**：io/os/dofile 保留，插件可模块化、读写文件、执行命令。
3. **手动重载**：`vistty.reload()` 清空钩子 + 重新执行 init.lua + Activate。
4. **-config 复用**：flag 语义从 config.jsonc 改为 init.lua。
5. **config 包删除**：JSONC 解析全移除，配置加载移入 plugins 包。
6. **常量表对齐 Go**：`vistty.keys` = evdev scancode，`vistty.mods` = platform.Modifiers 位值，零转换。
7. **send/send_key 分工**：send 发 UTF-8 字符串/控制码，send_key 发功能键转义（仅 WriteKeyEscape switch case 的 code，不支持可打印字符，mods 支持位组合）。
8. **OSD 聚合器**：Compositor 零改动，OSD 内部聚合插件图元。
9. **主线程 VM**：所有 Lua 调用经 keyEvCh/主 select 序列化到主线程。
10. **仅文本+矩形面板**：ctx:text/ctx:rect，复用 FillRect/BlendGlyph。
11. **鼠标事件不随插件系统实现**：留待后续。

## 十、init.lua 完整示例

```lua
-- ~/.config/vistty/init.lua

vistty.config = {
    backend  = "auto",
    shell    = "/bin/bash",
    font     = "",
    fontsize = 14,
    primary  = "",
    error_log = "",
    keybindings = {
        zoom_in     = {key="=",     mod="super"},
        zoom_out    = {key="-",     mod="super"},
        zoom_reset  = {key="0",     mod="super"},
        new_tab     = {key="t",     mod="super"},
        close_tab   = {key="w",     mod="super"},
        prev_tab    = {key="Left",  mod="super"},
        next_tab    = {key="Right", mod="super"},
        next_screen = {key="Tab",   mod="super"},
        switch_n    = {key="1-9",   mod="super"},
    },
}

-- 输入拦截示例
vistty.input.on_key(function(ev)
    if (ev.mods % (vistty.mods.CTRL * 2)) >= vistty.mods.CTRL and ev.code == vistty.keys.SPACE then
        vistty.term.send("\x1b[6~")  -- 自定义 Ctrl+Space = PageDown
        return true
    end
end)

-- 底部状态栏插件
vistty.ui.enable("bottom", 1)
vistty.ui.on_render(function(ctx)
    local w, h = ctx:size()
    ctx:rect(0, 0, w, h, {bg=vistty.colors.GRAY})
    ctx:text(2, 0, os.date("%H:%M:%S"), {fg=vistty.colors.SKY})
    ctx:text(w-10, 0, "tabs:"..vistty.tab.count(), {fg=vistty.colors.GOLD})
    -- 半透明矩形示例：#RRGGBBAA
    ctx:rect(10, 0, 5, 1, {bg="#FF000080"})
    return true
end)

-- 模块化加载（完全开放沙箱，dofile 可用）
-- dofile(os.getenv("HOME").."/.config/vistty/plugins/extra.lua")
```
