# IME 候选词自适应显示优化

## 背景

当前中文输入法候选词显示存在以下问题：

1. **CJK 位置计算 Bug**：Lua 层用 `#str`（字节长度）计算位置，CJK 字符 3 字节但显示宽度 2 cell，导致位置偏移
2. **无显示区域宽度限制**：候选词多或长时直接溢出面板右边界
3. **固定 pageSize=9**：不考虑候选词实际显示宽度，9 个长词可能远超面板宽度
4. **OSD 渲染层无越界裁剪**：超出面板区域的文本直接画到帧缓冲外

## 设计决策

| 决策项 | 选择 | 理由 |
|--------|------|------|
| 分页逻辑位置 | Lua 层 | 显示宽度与面板宽度相关，属于 UI 层关注点；Go 层只管词库查询 |
| 数字键选择 | Lua 层直接 send | 无需 Go 层 SelectCandidate 回传，减少跨层交互 |
| 候选词最大数 | 256 | 原先 36（9×4 页）太少，256 足够覆盖高频词 |
| 显示宽度 API | `vistty.display_width(str)` | 暴露 Go 层 `runeutil.StringWidth` 给 Lua，修复 CJK 位置计算 |
| IME 模块化 | `require("ime")` 独立文件 | IME 逻辑复杂，独立模块便于维护和复用 |
| 页码重置时机 | on_key 检测 preedit 变化 | 输入新字母时重置页码，翻页时不重置 |
| 渲染裁剪 | OSD 层 clip 区域 | 防御性措施，即使 Lua 层已裁剪，渲染层也应保护 |

## 架构变更

### Go 层精简

```
Before:
  Pinyin.ProcessKey → 字母/Backspace/Enter/Esc/Space/1-9/翻页键
  Pinyin.Candidates() → 当前页候选词（最多9个）
  Pinyin.page / pageSize / pageCandidates()

After:
  Pinyin.ProcessKey → 字母/Backspace/Enter/Esc/Space（仅核心输入）
  Pinyin.Candidates() → 全部候选词（最多256个）
  移除 page/pageSize/pageCandidates/翻页键/数字键处理
```

### Lua 层接管

```
ime.lua 模块:
  ├── page_slice(cands, page, avail_w)  — 基于显示宽度自适应分页
  ├── total_pages(cands, avail_w)       — 计算总页数
  ├── setup_key_handler()               — 数字键选择 + 翻页
  └── setup_render_handler()            — 候选词面板渲染 + 页码指示
```

### 数据流

```
用户按键 → on_key 钩子链:
  1. Ctrl+Space 切换 IME
  2. process_key 路由到 Go 层（字母/Backspace/Esc/Enter/Space）
  3. ime.lua on_key 处理（数字键选择 + 翻页键）
     ↓
每帧 on_render:
  1. ime.lua 获取全部候选词
  2. page_slice 基于面板宽度计算当前页候选词
  3. 用 display_width 精确计算位置渲染
  4. 超出面板宽度时截断显示
  5. OSD 层 clip 区域防御性裁剪
```

## 修改清单

| # | 文件 | 变更类型 | 说明 |
|---|------|----------|------|
| 1 | `internal/runeutil/runeutil.go` | 修改 | 新增 `StringWidth(s string) int` |
| 2 | `ime/pinyin/pinyin.go` | 重构 | 移除翻页/数字键；`Candidates()`返回全部；`maxCandidates=256` |
| 3 | `ime/pinyin/pinyin_test.go` | 修改 | 移除翻页测试，修改Reset测试，新增全量测试 |
| 4 | `internal/plugins/api_misc.go` | 修改 | 新增 `vistty.display_width(str)` Lua API |
| 5 | `internal/plugins/manager.go` | 修改 | 设置 `package.path` 包含 init.lua 所在目录 |
| 6 | `internal/ui/osd.go` | 修改 | 渲染越界裁剪（clip 区域） |
| 7 | `examples/ime.lua` | 新建 | IME 模块：翻页+选择+自适应宽度 |
| 8 | `examples/init.lua` | 修改 | 移除 IME 逻辑，`require("ime")` 引入 |

## 自适应分页算法

`page_slice(cands, page, avail_w)` 核心逻辑：

```
1. 从候选词列表开头遍历，按 page 逐页跳过
2. 每页逐个累加候选词显示宽度（序号 + 词 + 间隔）
3. 超出 avail_w 且已放至少 1 个时换页
4. 每页上限 9 个（数字键限制）
5. 到达目标页时收集候选词返回
```

## 验证计划

1. `go build ./...` — 编译通过
2. `go vet ./...` — 静态分析
3. `go test ./...` — 全量测试（含 pinyin 测试修改）
4. 手动验证：`go run ./cmd/vistty -backend wayland`
   - 输入拼音，候选词不超出面板边界
   - CJK 候选词位置正确
   - 数字键选词正常
   - 翻页正常
   - 页码指示显示正确
