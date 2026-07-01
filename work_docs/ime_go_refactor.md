# IME Go 层职能精简 — 纯查询引擎

## 背景

上一轮优化将翻页/选择移到 Lua 层，但 Go 层仍持有交互状态（`buf`/`active`/`cands`）和按键处理逻辑（`ProcessKey`）。Go 层职责应仅为词库查询 + 拼音格式化，交互状态和按键决策全部由 Lua 管理。

## 设计决策

| 决策项 | 选择 | 理由 |
|--------|------|------|
| Go 层职能 | 纯查询引擎 | 词库查询是 Go 层唯一需要做的事；交互决策属于 UI 层 |
| InputMethod 接口 | `Name()+Lookup(input)+FormatPreedit(input)` | 无状态方法，Go 层不持有 buf/active/cands |
| Pinyin 结构 | 仅保留 dict | 无状态，每次 Lookup 重新计算 |
| Registry 职能 | 注册+查询路由 | 移除 Activate/Deactivate/Active/ProcessKey |
| LuaIMM | 简化为 Lookup+FormatPreedit 两个钩子 | 与 Go 原生 Pinyin 接口一致 |
| Lua 层状态管理 | ime_buf/ime_active 自行维护 | Go 层无交互状态 |
| Lua API | 移除 process_key/activate/deactivate/active/reset | 新增 lookup(input)/format_preedit(input) |

## 新接口

### ime/ime.go

```go
type Candidate struct {
    Word string
    Code string
}

type InputMethod interface {
    Name() string
    Lookup(input string) []Candidate
    FormatPreedit(input string) string
}

type Registry struct {
    methods map[string]InputMethod
}
```

移除：`KeyEvent`、`Response`、`Activate/Deactivate/IsActive/ProcessKey/Preedit/Candidates/Reset`

### ime/registry.go

```go
func (r *Registry) Register(m InputMethod)
func (r *Registry) Lookup(name, input string) []Candidate
func (r *Registry) FormatPreedit(name, input string) string
func (r *Registry) List() []string
func (r *Registry) ClearLuaMethods()
```

移除：`Activate/Deactivate/Active/ProcessKey`

### ime/pinyin/pinyin.go

```go
type Pinyin struct {
    dict map[string][]dictEntry
}

func New() *Pinyin
func (p *Pinyin) Name() string
func (p *Pinyin) Lookup(input string) []ime.Candidate
func (p *Pinyin) FormatPreedit(input string) string
```

移除：`buf/cands/active/codeXxx/modCtrl/ProcessKey/Reset/Preedit()/Candidates()/firstCandidateWord()/pageCandidates()`
Lookup 实现基于原 `lookup()` 逻辑，但无状态（每次重新计算）
FormatPreedit 实现基于原 `formatPreedit()` 逻辑

### ime/lua_adapter.go

```go
type LuaIMMHooks struct {
    Lookup         func(input string) []Candidate
    FormatPreedit  func(input string) string
}

type LuaIMM struct {
    name           string
    lookup         func(input string) []Candidate
    formatPreedit  func(input string) string
}
```

移除：`OnActivate/OnDeactivate/ProcessKey/Preedit/Candidates/OnReset/active/onActivate/onDeactivate/processKey/preedit/candidates/onReset`

### Lua API 变更 (api_ime.go)

| 原 API | 新 API | 说明 |
|--------|--------|------|
| `vistty.ime.process_key(ev)` | 移除 | Lua 直接处理按键 |
| `vistty.ime.preedit()` | `vistty.ime.format_preedit(input)` | 传入拼音串 |
| `vistty.ime.candidates()` | `vistty.ime.lookup(input)` | 传入拼音串 |
| `vistty.ime.activate(name)` | 移除 | Lua 管理激活状态 |
| `vistty.ime.deactivate()` | 移除 | Lua 管理 |
| `vistty.ime.active()` | 移除 | Lua 管理 |
| `vistty.ime.reset()` | 移除 | Go 无状态 |
| `vistty.ime.register(name, hooks)` | 保留，hooks 简化 | hooks 仅需 lookup+format_preedit |
| `vistty.ime.list()` | 保留 | 列出可用输入法 |

### ime.lua 状态管理

```lua
local ime_active = false
local ime_active_name = ""
local ime_buf = ""

-- Ctrl+Space 切换
vistty.input.on_key(function(ev)
    if ime_active then
        ime_active = false
        ime_buf = ""
        ime_active_name = ""
    else
        ime_active = true
        ime_buf = ""
        ime_active_name = "pinyin"
    end
    return true
end)

-- 按键处理（IME 激活时）
vistty.input.on_key(function(ev)
    if not ime_active then return end
    if ime_buf == "" and ev.code ~= letters then return end
    
    if isLowerLetter(ev) then
        ime_buf += ev.rune
        ime_page = 0
        -- consumed=true
    elseif ev.code == vistty.keys.BACKSPACE then
        ime_buf = removeLastChar(ime_buf)
        ime_page = 0
    elseif ev.code == vistty.keys.ESC then
        ime_buf = ""
        ime_page = 0
        -- consumed=true
    elseif ev.code == vistty.keys.ENTER then
        vistty.term.send(ime_buf)
        ime_buf = ""
        ime_active = false
        -- consumed=true
    elseif ev.code == vistty.keys.SPACE then
        local cands = vistty.ime.lookup(ime_buf)
        if #cands > 0 then
            vistty.term.send(cands[1].word)
        else
            vistty.term.send(ime_buf)
        end
        ime_buf = ""
        ime_active = false
        -- consumed=true
    -- 数字键/翻页键...
    end
end)

-- 渲染
vistty.ui.on_render(function(ctx)
    if ime_active and ime_buf ~= "" then
        local pre = vistty.ime.format_preedit(ime_buf)
        local cands = vistty.ime.lookup(ime_buf)
        -- 渲染 preedit + 候选词
    end
end)
```

## 修改清单

| # | 文件 | 变更类型 | 说明 |
|---|------|----------|------|
| 1 | `ime/ime.go` | 重构 | 移除 KeyEvent/Response，简化 InputMethod 接口 |
| 2 | `ime/registry.go` | 重构 | 移除 Activate/Deactivate/Active/ProcessKey，新增 Lookup/FormatPreedit |
| 3 | `ime/lua_adapter.go` | 重构 | 简化为 Lookup+FormatPreedit 两个钩子 |
| 4 | `ime/pinyin/pinyin.go` | 重构 | 移除 buf/cands/active/ProcessKey，改为 Lookup+FormatPreedit 无状态方法 |
| 5 | `ime/pinyin/pinyin_test.go` | 重构 | 移除 ProcessKey 测试，新增 Lookup/FormatPreedit 测试 |
| 6 | `ime/ime_test.go` | 重构 | 简化 mockIM，适配新接口 |
| 7 | `internal/plugins/api_ime.go` | 重构 | 移除旧 API，新增 lookup/format_preedit，简化 register |
| 8 | `internal/plugins/manager.go` | 修改 | 移除 registry.active 管理，移除 registry.ProcessKey 路由 |
| 9 | `examples/ime.lua` | 修改 | 自行管理 ime_active/ime_buf 状态 |
| 10 | `examples/init.lua` | 修改 | 移除 vistty.ime.activate/deactivate/active/process_key/reset 调用 |

## 验证计划

1. `go build ./...` — 编译通过
2. `go vet ./...` — 静态分析
3. `go test ./ime/...` — 测试通过
4. `go test ./internal/plugins/...` — 插件测试通过
