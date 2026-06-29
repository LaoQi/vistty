# Vistty 中文输入法（IME）实现方案

## 目标

在现有插件系统基础上，实现一个可用的中文拼音输入法，并预留多输入法扩展能力。

## 设计决策

| 决策项 | 选择 | 理由 |
|--------|------|------|
| 架构层 | Go 层词库与状态机 + Lua 插件前端胶水 | 40 万词条查询性能靠 Go map；Lua 只做按键转发与 UI 绘制 |
| 输入法抽象 | 抽出 `InputMethod` 接口，预留多输入法 | 拼音/五笔/双拼均可实现同一接口 |
| 注册机制 | Go 原生注册 + Lua 侧 `vistty.ime.register` | 既支持高性能 Go 实现，也支持纯 Lua 实现新输入法 |
| 输入方案 | 全拼 | 最通用 |
| 词库来源 | rime-ice（GPLv3）8105 单字 + ext/tencent 精选高频词 | 质量高、长期维护 |
| 词库分发 | `go:embed` 内嵌 dict.bin | 即装即用，零配置 |
| 候选词 UI | 底部单行面板 | 复用现有 `vistty.ui.enable("bottom", 1)` |
| 选词交互 | 纯键盘（1-9 选词，Space/-=/方向键翻页） | 契合终端场景 |
| License | vistty 升级 GPLv2 → GPLv2+（"version 2 or later"） | 兼容 GPLv3 词库数据 |

## 架构

```
github.com/LaoQi/vistty/
├── ime/                          # 顶层包
│   ├── ime.go                    # InputMethod 接口 + KeyEvent/Candidate/Response 类型 + Registry
│   ├── registry.go               # Registry：注册/激活/路由
│   ├── lua_adapter.go            # LuaIMM：把 Lua 注册的回调包装成 InputMethod
│   ├── ime_test.go               # Registry + 接口契约测试
│   └── pinyin/                   # 拼音（Go 原生实现）
│       ├── pinyin.go             # 实现 InputMethod + 状态机
│       ├── syllable.go           # 全拼音节表 + DP 切分
│       ├── dict.go               # go:embed 词库加载
│       ├── data/dict.bin         # 预处理词库
│       ├── data/LICENSE          # 词库来源许可（rime-ice GPLv3）
│       └── pinyin_test.go        # 切分 + 查询 + 状态机测试
├── cmd/gen-dict/main.go          # 词库预处理工具
├── internal/plugins/
│   ├── api_ime.go                # vistty.ime.* 绑定（含 register）
│   └── (manager.go / api_misc.go 小幅改动)
└── examples/
    ├── ime_pinyin.lua            # 前端胶水：Ctrl+Space 切换 + on_key 路由 + on_render
    └── init.lua                  # 末尾 dofile ime_pinyin.lua
```

## 核心接口

### `ime/ime.go`

```go
// KeyEvent 从 platform.KeyEvent 转译（解耦 platform 依赖）
type KeyEvent struct {
    Rune  rune
    Code  uint16
    Mods  uint8   // platform.Modifiers 位值
    State bool    // true=press
}

// Candidate 候选词
type Candidate struct {
    Word string  // 候选文本
    Code string  // 编码显示（如拼音 "ni hao"）
}

// Response ProcessKey 的返回
type Response struct {
    Consumed   bool        // 拦截该键不传终端
    Commit     string      // 本次提交文本（送 PTY，一次性）
    Preedit    string      // 预编辑串显示
    Candidates []Candidate // 当前候选列表
}

// InputMethod 输入法接口。Go 原生与 Lua 注册都实现此接口。
type InputMethod interface {
    Name() string
    Activate()
    Deactivate()
    IsActive() bool
    ProcessKey(ev KeyEvent) Response
    Preedit() string
    Candidates() []Candidate
    Reset()
}

// Registry 多输入法注册中心
type Registry struct {
    methods map[string]InputMethod
    active  InputMethod
}

func NewRegistry() *Registry
func (r *Registry) Register(m InputMethod)
func (r *Registry) Activate(name string) bool
func (r *Registry) Deactivate()
func (r *Registry) Active() InputMethod     // nil=未激活
func (r *Registry) List() []string
func (r *Registry) ProcessKey(ev KeyEvent) Response // 路由到 active，无激活返回空 Response
```

**关键点**：
- `InputMethod` 接口粒度大，状态机在实现内部。Go 的 `pinyin.Pinyin` 持有 `pinyinBuf`/`candidates`/`page` 等状态。
- Lua 注册的输入法通过 `LuaIMM`（`lua_adapter.go`）包装，把 Lua 回调函数适配成接口。
- `Registry.ProcessKey` 无激活时返回空 Response（Consumed=false），Lua 层据此放行。

## Lua API（`api_ime.go`）

```lua
-- 注册 Lua 实现的输入法（双拼/五笔等可纯 Lua 实现）
vistty.ime.register("myime", {
    on_activate = function() end,
    on_deactivate = function() end,
    process_key = function(ev)
        -- ev = {rune=, code=, mods=, state=}
        -- return {consumed=, commit=, preedit=, candidates={{word=,code=},...}}
    end,
    preedit = function() return "" end,
    candidates = function() return {} end,
    reset = function() end,
})

-- 通用控制
vistty.ime.list()                  -- → {"pinyin"}
vistty.ime.activate("pinyin")
vistty.ime.deactivate()
vistty.ime.active()                -- → "pinyin" / nil
vistty.ime.process_key(ev)         -- → Response（路由到 active 输入法）
vistty.ime.preedit()               -- → string
vistty.ime.candidates()            -- → {{word=,code=},...}
vistty.ime.reset()
```

`LuaIMM`（Go 端 adapter）持有这些 Lua 回调函数引用，实现 `InputMethod` 接口时调对应 Lua 函数。Lua 注册的输入法与 Go 原生 pinyin 在 Registry 中地位平等。

## 拼音实现（`ime/pinyin/`）

### 状态机（`pinyin.go`）

```
状态：buf (string), cands ([]Candidate), page (int)

ProcessKey(ev):
  非 a-z 字母键：
    Backspace → 删 buf 末字符；buf 空→Reset；consumed=true
    1-9 → 选候选 page*9+n；commit word；consumed=true
    Space → 选首候选或提交原文 buf；consumed=true
    Enter → 提交 buf 原文（直通英文）；consumed=true
    Esc → Reset；consumed=true
    -/=/←/→ → 翻页；consumed=true
    其他键 → 若 buf 非空先 commit 首候选，再 consumed=false（放行）
  a-z：buf += rune；查询 candidates；consumed=true
```

### 拼音切分（`syllable.go`）

硬编码约 400 个音节表（`a/ai/an/ang/ao/ba/bai/.../zuo`），DP 求所有合法切分：
- `nihao` → `[["ni","hao"]]`
- `xian` → `[["xi","an"], ["xian"]]`（多切分合并去重按权重排序）

### 词库（`dict.go` + `data/dict.bin`）

**来源**：rime-ice `cn_dicts/8105.dict.yaml`（单字全量 8615 条）+ 从 `ext.dict.yaml`/`tencent.dict.yaml` 精选权重 Top 2-3 万高频词。

**预处理格式**（`cmd/gen-dict/main.go`）：
```
[4B: 条目数][4B: key 池大小][4B: word 池大小]
[key 池: 排序拼音串, \0 分隔]
[word 池: UTF-8 词, 2B 长度前缀]
[条目 × N]: [4B key 偏移][2B word 偏移][2B word len][4B 权重]
```
运行时 `map[string][]Candidate` 加载，查询 O(1)。

## 底部面板布局（单行）

`vistty.ui.enable("bottom", 1)`（1 行）：

```
│ ni'hao_  1你好 2你号 3拟好 4尼豪 5逆号                  │
```

- preedit 青色 + 末尾 `_` 光标占位
- 候选词白色，序号灰色，两项间空格分隔
- 激活但无输入时显示 `中` 状态符
- 未激活不绘制（透明）

## Lua 前端（集成于 `examples/init.lua`）

拼音输入法逻辑直接内联在 `examples/init.lua` 中，无需独立文件：

```lua
-- Ctrl+Space 切换拼音
vistty.input.on_key(function(ev)
    if ev.state ~= vistty.state.PRESS then return end
    if (ev.mods % (vistty.mods.CTRL*2)) >= vistty.mods.CTRL
       and ev.code == vistty.keys.SPACE then
        if vistty.ime.active() then
            vistty.ime.deactivate()
        else
            vistty.ime.activate("pinyin")
        end
        return true
    end
end)

-- 按键路由到 active 输入法
vistty.input.on_key(function(ev)
    if ev.state ~= vistty.state.PRESS then return end
    if not vistty.ime.active() then return end
    local r = vistty.ime.process_key(ev)
    if r.commit and r.commit ~= "" then
        vistty.term.send(r.commit)
    end
    return r.consumed
end)

-- 底部单行面板：IME 激活时渲染 preedit + 候选词，否则渲染时钟与标签数
vistty.ui.enable("bottom", 1)
vistty.ui.on_render(function(ctx)
    local w, h = ctx:size()
    ctx:rect(0, 0, w, h, {bg=vistty.colors.DARKGRAY})
    if vistty.ime.active() then
        local pre = vistty.ime.preedit()
        if pre == "" then
            ctx:text(2, 0, "中", {fg=vistty.colors.CYAN})
        else
            ctx:text(0, 0, pre .. "_", {fg=vistty.colors.CYAN})
            local cands = vistty.ime.candidates()
            local x = #pre + 2
            for i, c in ipairs(cands) do
                local idx = tostring(i)
                ctx:text(x, 0, idx, {fg=vistty.colors.GRAY})
                ctx:text(x + #idx, 0, c.word, {fg=vistty.colors.WHITE})
                x = x + #idx + #c.word + 1
            end
        end
    else
        ctx:text(2, 0, os.date("%H:%M:%S"), {fg="#64C8FF"})
        ctx:text(w - 10, 0, "tabs:" .. vistty.tab.count(), {fg=vistty.colors.GOLD})
    end
    return true
end)
```

## 修改清单

| 文件 | 改动类型 | 说明 |
|------|---------|------|
| `ime/ime.go` | 新增 | InputMethod 接口 + 类型 + Registry |
| `ime/registry.go` | 新增 | Registry 实现 |
| `ime/lua_adapter.go` | 新增 | LuaIMM 包装 |
| `ime/ime_test.go` | 新增 | Registry + 接口契约测试 |
| `ime/pinyin/pinyin.go` | 新增 | 状态机 + 实现 InputMethod |
| `ime/pinyin/syllable.go` | 新增 | 音节表 + DP 切分 |
| `ime/pinyin/dict.go` | 新增 | go:embed 加载 |
| `ime/pinyin/data/dict.bin` | 新增 | 预处理词库 |
| `ime/pinyin/data/LICENSE` | 新增 | rime-ice GPLv3 许可 |
| `ime/pinyin/pinyin_test.go` | 新增 | 切分 + 查询 + 状态机测试 |
| `cmd/gen-dict/main.go` | 新增 | 词库预处理工具 |
| `internal/plugins/api_ime.go` | 新增 | vistty.ime.* 绑定（含 register） |
| `internal/plugins/api_misc.go` | 修改 | registerAPIs 加 registerIME |
| `internal/plugins/manager.go` | 修改 | PluginManager 加 registry + 初始化注册 pinyin |
| `examples/init.lua` | 修改 | 内联拼音输入法逻辑（Ctrl+Space 切换 + on_key 路由 + 底部面板） |
| `LICENSE` | 修改 | 头部升级 GPLv2+（"version 2 or later"） |
| `AGENTS.md` | 修改 | 补充 ime 模块说明 |

## 关键约束

- CGO_ENABLED=0：词库纯 Go 解析
- gopher-lua 非线程安全：所有 Lua 调用经主线程，LuaIMM 调用假定主线程上下文
- `manager.go:121` release 跳过：IME 状态机只依赖 press，无影响
- pinyin 初始化在 NewPluginManager（主线程前），Registry 随 PluginManager 生命周期

## 验证计划

1. `go test ./ime/...` — Registry + 拼音切分 + 查询 + 状态机
2. `go vet ./... && go build ./...`
3. `go run ./cmd/gen-dict` — 生成 dict.bin
4. 手动：`go run ./cmd/vistty -backend wayland`，Ctrl+Space 激活，输入 `nihao` → 选词
