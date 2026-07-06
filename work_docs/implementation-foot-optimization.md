# foot 终端优化实施方案

## 概述

克隆分析 [foot](https://codeberg.org/dnkl/foot)（C 语言、Wayland 原生终端，性能与 alacritty 持平）的实现，对照 vistty 现状，提取可借鉴的终端实现方法与性能优化技术，分阶段移植到 vistty。

foot 源码已克隆至 `reference/foot/`（已加入 `.gitignore`，不纳入版本控制）。

**预期效果**：
- TUI 应用（nvim/tmux/htop）渲染帧数大幅下降（BSU 合并更新）
- CPU 渲染路径（Wayland/dumb buffer）工作量从"全量重绘"降为"仅 dirty 区域"（damage tracking）
- 滚动从 O(n) 切片平移降为 O(1) 环形指针移动
- 内存占用下降（Cell 紧凑化 + 渲染调度合并）

**核心结论**：foot 的性能优势主要来自 **damage tracking（双层 dirty）+ BSU（同步更新）+ 环形缓冲 grid** 三件套，而非单遍 VT 解析或多线程渲染。vistty 已有 GPU instanced draw 优势（GBM 路径 P99=9.57ms 已较快），优化重点应放在 CPU 路径与渲染调度。

---

## foot 实现分析

### 1. 数据结构（`reference/foot/terminal.h`）

**Cell = 12 字节**（位域压缩，`terminal.h:68-72`）：
```c
struct attributes {
    bool bold:1; bool dim:1; bool italic:1; bool underline:1;
    bool strikethrough:1; bool blink:1; bool conceal:1; bool reverse:1;
    uint32_t fg:24;                    // 24位颜色值直接内联
    bool clean:1;                      // cell 级 dirty（渲染后置1）
    enum color_source fg_src:2;        // default/base16/base256/rgb
    enum color_source bg_src:2;
    bool confined:1; bool selected:1; bool url:1;
    uint32_t bg:24;
};  // 8 字节
struct cell { char32_t wc; struct attributes attrs; };  // 12 字节
```
- 颜色用 24 位值 + 2 位 source 编码，省掉 `IsDefault bool`
- `clean` 位是 cell 级 dirty 标志（渲染后置 1，需重绘置 0）

**row 有 dirty/linebreak**（`terminal.h:148-160`）：
```c
struct row { struct cell *cells; struct row_data *extra;
             bool dirty; bool linebreak; ... };
```

**grid 环形缓冲**（`terminal.h:220-248`、`grid.h:34-44`）：
```c
struct grid { int num_rows, num_cols, offset, view;
              struct row **rows; tll(struct damage) scroll_damage; ... };
static inline int grid_row_absolute(const struct grid *grid, int row_no) {
    return (grid->offset + row_no) & (grid->num_rows - 1);  // 位与取模（num_rows 须 2的幂）
}
```
- 滚动只需移动 `offset` 指针，O(1)
- `view` 是滚动回放视图起点，screen + scrollback 共享同一环形空间
- `scroll_damage` 链表追踪滚动损伤

### 2. Damage Tracking（`terminal.c:2512-2564`、`render.c:1620-1670`）

**双层 dirty**：
- 行级：`row->dirty`（`render.c:1624` `if (!row->dirty) continue` 跳过整行）
- cell 级：`cell->attrs.clean`（`render.c:700` `if (cell->attrs.clean) return` 跳过 cell）
- 渲染后清除：`render.c:703` `cell->attrs.clean = 1`、`render.c:3571` `row->dirty = false`

**damage API**（`terminal.h:873-887`）：
- `term_damage_rows/start/end` — 标记行范围 dirty + 所有 cell clean=0
- `term_damage_view` — 标记可视区域
- `term_damage_cursor` — 只标记光标 cell（精确损伤）
- `term_damage_color` — OSC 颜色变更时按颜色匹配精确标记 cell
- `term_damage_scroll` — 滚动损伤（SCROLL/SCROLL_REVERSE/IN_VIEW 等）

**pixman_region 累积**（`render.c:1024,1284-1291`）：
- 每个 dirty cell 渲染时 `pixman_region32_union_rect` 累积到 damage region
- 帧结束时 `wl_surface_damage_buffer` 提交 dirty 区域给合成器
- buffer 双缓冲时用 `buf->dirty[0]` 重应用上帧 damage

### 3. BSU 同步更新（`terminal.c:3823-3867`）

```c
void term_enable_app_sync_updates(struct terminal *term) {
    term->render.app_sync_updates.enabled = true;
    // 1秒超时兜底（防止应用崩溃后永久不刷新）
    timerfd_settime(term->render.app_sync_updates.timer_fd, 0,
                    &(struct itimerspec){.it_value = {.tv_sec = 1}}, NULL);
    // 禁用 pending 渲染（合并所有更新到一帧）
    if (只有 grid 待渲染) { term->render.refresh.grid = false; pending.grid = false; }
    // 禁用 delayed_render_timer
}
void term_disable_app_sync_updates(struct terminal *term) {
    term->render.app_sync_updates.enabled = false;
    render_refresh(term);  // 禁用时立即一次性刷新所有累积更新
    timerfd_settime(..., {0}, NULL);  // 重置超时
}
```
- DECSET 2026 启用 / DECSET 2026 reset 禁用
- 启用时：禁用渲染调度，仍写 screen + 标记 damage，但不触发渲染
- 禁用时：触发一次渲染（配合 damage tracking 只渲染 dirty 区域）
- 1 秒超时兜底：超时后自动禁用并刷新

### 4. Buffer 管理（`shm.h`、`shm.c`）

- **buffer_chain**：buffer 池复用，避免重复分配 SHM pool
- **buffer age**（`shm.h:28`）：追踪 buffer 在 busy 期间被请求次数，决定是否需重绘全屏（合成器未及时释放时双缓冲）
- **shm_scroll**（`shm.h:91-93`）：在 SHM buffer 内 `memmove` 滚动行，避免重绘滚动区域——foot 滚动性能关键
- **每 worker 独立 dirty region**（`shm.h:41`）：多线程渲染时各 worker 累积自己的 damage，帧结束合并

### 5. VT 解析（`vt.c`）

- 基于 DEC ANSI parser（vt100.net 状态机），`vt.c:28-54` 定义 16 个状态
- **单遍处理**：解析即执行 action（`action_execute`/`action_print` 等直接调用 term 函数），无中间 Sequence 对象
- 完整支持 Sixel（`sixel.c`，DCS 状态机）、grapheme clustering（VS16/ZWJ，`composed.c`）
- 支持 subparams（ITU-T T.416）

### 6. 渲染调度（`terminal.h:640-729`）

- **refresh/pending 双阶段**：`refresh`（asap）vs `pending`（下帧回调）
- **delayed_render_timer**：合并短时间内的多个渲染请求（lower_fd/upper_fd 双阈值）
- **多线程渲染**：`sem_t start/done` + `thrd_t *workers` + 行队列分发
- **preapplied_damage**：Wayland 合成器提前准备 damage（`terminal.h:702-708`）
- **ascii_printer 函数指针**：根据 sixels/osc8/underline/charset 标志动态选择打印函数，避免每次判断
- **timerfd 异步驱动**：光标闪烁/blink/flash 用 timerfd，非每帧检查

### 7. 字符合成（`box-drawing.c`）

- box-drawing(U+2500-259F)/braille(U+2800-28FF)/octants(U+1CD00)/legacy(U+1FB00) 四类预生成自定义字形
- 用 pixman 像素级合成（`pixman_image_fill_boxes`/`pixman_image_composite`）

---

## vistty 现状对照（差距清单）

| 维度 | foot | vistty | 差距/影响 |
|------|------|--------|----------|
| Cell 大小 | 12B（位域） | 16B（5B padding，`cell.go:20-26`） | 31% 浪费，缓存命中率低 |
| 行 dirty | `row->dirty` | **无**（`line.go:3-5` 仅 cells） | 无法跳过未变行 |
| cell dirty | `attrs.clean` 位 | **无** | 无法跳过未变 cell |
| damage API | 完整 6 类 | **无**（仅 session 层全局 `m.dirty`） | 每帧全量重绘 |
| grid 滚动 | 环形 O(1) | 切片 copy O(n)（`buffer.go:76-93`） | 大量滚动时开销 |
| BSU | DECSET 2026 完整 | **未实现**（`?2026$p` 不响应） | TUI 应用每帧重绘 |
| history | 与 grid 共享环形 | 独立切片头删除（`history.go`） | O(n) 头删除 |
| VT 解析 | 单遍执行 | 双遍（FeedInto+Apply，sync.Pool） | 有 Sequence 对象开销但已复用 |
| Sixel | 完整 | **未实现**（DCS 空 case） | 功能缺失 |
| 渲染调度 | delayed_render + 双阶段 | 全局 dirty + 15tick 兜底 | 渲染帧数可优化 |
| buffer 优化 | buffer age + shm_scroll | **无** | Wayland 滚动全量重绘 |
| 多线程渲染 | workers + sem | 单线程（LockOSThread） | Go goroutine 受限 |
| GPU 渲染 | **无**（pixman CPU） | instanced draw（已有优势） | vistty 优势项 |

**vistty 已知痛点**（来自 `work_docs/gbm_perf_analysis.md`、`work_docs/optimize.md`）：
- `BlendGlyphAlpha` 占 CPU 62%、`FillRect` 29%（全量重绘导致）
- `terminal.mu` RWMutex 锁竞争：PTY 写锁阻塞渲染
- nvim 询问 `?2026$p` 无响应

---

## 实施阶段

### 阶段 1: BSU 同步更新（DECSET 2026）

- **状态**: 已审计
- **目标**: 实现 BSU，TUI 应用重绘期间合并渲染，禁用时一次性刷新。这是投入最小、TUI 应用收益最直接的优化。
- **借鉴 foot**: `terminal.c:3823-3867`（enable/disable + 1秒超时兜底）
- **实施内容**:
  1. `terminal/terminal.go` Terminal 增加 `appSyncUpdates bool` + `syncTimer *time.Timer` 字段
  2. `terminal/terminal.go` `handleMode`（约 line 636-694）增加 `case 2026`：
     - set: 调用 `enableSyncUpdates()`，置 `appSyncUpdates=true`，启动 1 秒超时 timer（到期自动 disable + 触发渲染）
     - reset: 调用 `disableSyncUpdates()`，置 false，停止 timer，触发一次 dirty 渲染
  3. `terminal/terminal.go` `handleMode` 的 `?2026$p`（DSR 模式查询）响应 `CSI ?2026;2$y`（表示支持）
  4. `session/render_loop.go`：PTY 收到序列时，若 `appSyncUpdates==true` 则写 screen + 标记 dirty 但**不立即触发渲染**（仅累积）；禁用或超时时置 `m.dirty=true` 触发一次渲染
  5. 超时兜底：`syncTimer` 到期回调中 `disableSyncUpdates()` + 置 dirty
  6. BSU 状态变更需 thread-safe（terminal.mu 保护）
- **涉及文件**: `terminal/terminal.go`、`session/render_loop.go`、`session/master.go`
- **验证标准**:
  - `go build ./...` 通过
  - 运行 nvim/htop，BSU 期间帧率下降（用 `-fps` 日志确认渲染次数减少）
  - nvim 重绘无撕裂（禁用时一帧完成）
  - 1 秒超时兜底生效（kill nvim 后 1 秒内自动刷新）
- **风险**: 低。BSU 是模式开关，不改变 screen 渲染逻辑，仅影响渲染调度。即使无 damage tracking，禁用时全量渲染也正确（只是没省工作量）。
- **审计结果**: 通过。subagent 实现正确。锁设计合理：`enableSyncUpdates`/`disableSyncUpdates` 为锁内调用（caller 持写锁，因 Go RWMutex 不可重入），`syncUpdateExpired`（timer goroutine）独立加锁 + 幂等检查 + 锁外调用 `OnRenderRequest` 回调。`?2026l` 路径依赖 render_loop `!IsSyncUpdates()`→`m.dirty=true`（≤16ms 渲染），timer 路径通过 `renderReqCh` 立即渲染。**审计修复关键缺陷**：vte `parseCSIPrivate`（csi.go）不识别 DECRQM `?2026$p`（nvim 实际发送的查询格式，final byte 'p'），原走 default 返回 CSIUnknown。补 `case 'p'` 路由到 CSIDeviceStatusReport，handleDSR case 2026 响应 `\x1b[?2026;<Ps>$y`（符合 DECRPM 格式）。补充 TestBSUDECRQM 测试。build/vet/test 全通过。

### 阶段 2: Damage Tracking（双层 dirty）

- **状态**: 已审计
- **目标**: 实现行级 + cell 级双层 dirty，渲染时跳过未变行/cell，消除全量重绘。这是 CPU 路径性能的核心优化（预期 `BlendGlyphAlpha`/`FillRect` 开销下降 50%+）。
- **借鉴 foot**: `terminal.h:148-160`（row.dirty）、`terminal.h:42-61`（attrs.clean 位）、`render.c:1620-1670`（双层跳过）、`terminal.c:2512-2564`（damage API）
- **实施内容**:
  1. `internal/screen/cell.go`：Attributes 增加 `AttrClean = 1 << 7`（位 7，当前用到 0-6）；增加 `Clean() bool`/`SetClean()` 方法
  2. `internal/screen/line.go`：Line 增加 `dirty bool` 字段 + `Dirty()`/`SetDirty()` 方法
  3. `internal/screen/buffer.go` 增加 damage API：
     - `DamageRows(start, end int)` — 标记行 dirty + 行内所有 cell clean=0
     - `DamageView()` — 标记可视区域
     - `DamageCursor(row, col int)` — 只标记光标 cell + 行 dirty
     - `DamageAll()` — 全屏标记
  4. **所有写操作标记 damage**（关键工作量大）：
     - `terminal/terminal.go` 中打印（`print`/`writeChar`）、擦除（`erase`/`eraseLine`/`eraseDisplay`）、SGR 不需（不改 cell）、光标移动后 damage 旧光标位置、滚动后 damage 滚动区域、resize 后 DamageAll
     - 每个 CSI 执行函数检查是否改变 screen，是则调对应 damage API
  5. `internal/render/compositor.go` 渲染循环改造：
     - CPU 路径（`Render` line 192-360）：遍历行 `if !line.Dirty() { continue }`；遍历 cell `if cell.Clean() { continue }`；渲染后 `cell.SetClean()`/`line.SetDirty(false)`
     - GPU 路径（`renderGPU` line 366-537）：只收集非 clean cell 的 instance（`instances[:0]` 后只 append dirty cell），减少 instance 上传和绘制量
  6. 光标处理：每帧 damage 旧光标位置 + 新光标位置（光标移动时）
  7. 渲染完成后清除所有 dirty（`DamageAll` 后的首帧全量，之后增量）
- **涉及文件**: `internal/screen/cell.go`、`internal/screen/line.go`、`internal/screen/buffer.go`、`terminal/terminal.go`（所有 CSI 执行）、`internal/render/compositor.go`、`session/render_loop.go`
- **验证标准**:
  - `go build ./...` + `go vet ./...` 通过
  - `go test ./internal/screen/ ./terminal/` 通过
  - 稳态无输入时 dirty 行数=0（光标闪烁除外）
  - 单字符输入只 damage 1-2 cell（用 debug 日志统计）
  - `./vistty -backend wayland -fps` 帧时间下降（对比 optimize.md 基线）
  - GBM 路径 `go test -bench=BenchmarkLayers` 改善
- **风险**: 中。需在所有 screen 写操作点插入 damage 标记，遗漏会导致渲染残影。建议先实现 + 大量手动测试（nvim/htop/less/vim），再逐步覆盖。
- **审计结果**: 通过。subagent 未执行，主 agent 自主实施。screen 层（cell.go AttrClean 位+方法、line.go dirty 字段+方法、buffer.go DamageRows/View/All/Line/Cursor API + ScrollUp/Down/Clear/ClearAll/ClearRect 自动 damage）；terminal 层（executeSequences 光标移动统一 DamageCursor 旧行+新行、execPrint/eraseChars/deleteChars/insertChars DamageLine、handleMode alt screen DamageAll、Resize/SetScrollOffset/SetTheme DamageAll）；compositor CPU 路径（useDirty=!directRender，dirty 行清背景+非 clean cell 跳过+渲染后 SetClean/SetDirty(false)、scrollChanged 检测、cursorRow 光标行总是渲染）。GPU 路径保持全量（Clear 重绘需全部 cell）。修复审计缺陷：cursorRow 原检查 cursorVisible 导致光标 blinkOff 时光标行不渲染残影，改为总是渲染光标行。build/vet/test 全通过。Wayland directRender 路径全量渲染（buffer 切换安全），dirty 优化主要在 DRM dumb CPU 路径。

### 阶段 3: 环形缓冲区 grid

- **状态**: 已审计
- **目标**: screen Buffer 改为环形缓冲，滚动从 O(n) 切片平移降为 O(1) 指针移动，history 与 grid 共享环形空间。
- **借鉴 foot**: `terminal.h:220-248`（grid offset/view）、`grid.h:34-44`（位与取模）、`grid.h:46-73`（懒分配行）
- **实施内容**:
  1. `internal/screen/buffer.go` 重构 Buffer：
     - `lines []*Line`（容量 = nextPow2(maxRows + scrollback)）+ `offset int` + `numRows int`（2的幂）
     - `rowAbsolute(row)`: `(b.offset + row) & (b.numRows - 1)`
     - `Line(row)`: `b.lines[b.rowAbsolute(row)]`
  2. `ScrollUp(n)`：`b.offset = (b.offset + n) & (b.numRows - 1)`，被滚出的行如需保留则移到 scrollback 区（仍在环形内），新行 reset+Fill —— **O(1)**
  3. `ScrollDown(n)`：反向移动 offset
  4. history 与 grid 共享环形：`view` 指针标识当前可视顶部，scrollback 是 offset 之前的部分
  5. `Resize`：重建环形数组，复用旧行
  6. `history.go` 适配：History 不再独立切片，改为 grid 环形内的 view 偏移
  7. scroll region（scrollTop/scrollBot）适配环形：局部滚动仍用 copy 但范围更小
- **涉及文件**: `internal/screen/buffer.go`、`internal/screen/history.go`、`internal/screen/line.go`、`terminal/terminal.go`（scroll 调用）、`internal/render/compositor.go`（行遍历适配）
- **验证标准**:
  - `go build ./...` + `go test ./internal/screen/` 通过
  - 大量滚动（`seq 100000`）无残影、无内存增长泄漏
  - scrollback 翻页正确
  - 滚动性能 benchmark 改善（对比基线）
- **风险**: 高。环形缓冲索引容易出错（off-by-one、scroll region 边界）。需充分单元测试覆盖 ScrollUp/Down/Resize/scroll-region 组合。建议保留旧实现做对比测试。
- **审计结果**: 通过。主 agent 自主实施。screen Buffer 改为环形（cap=nextPow2(rows), mask=cap-1, offset 头指针）；lineAt(row)/physRow(row) 统一行访问 `(offset+row)&mask`；全屏滚动 offset+=n O(1)（仅 push history + 底部新行），region 滚动逐行 copy O(region)；ScrollDown offset-=n + region 反向 copy；Resize 重建环形复用旧行；history 保持独立。修复 region copy 别名缺陷：copy 后 src/dst 指针别名，新行 Fill 原对象会修改被引用行，改 region 新行为 NewLine（新对象）。build/vet/test 全通过（含 scroll region 边界测试）。

### 阶段 4: 渲染调度优化

- **状态**: 已审计
- **目标**: 减少不必要的渲染帧数——delayed render 合并短时间内的多个渲染请求 + 异步定时器驱动光标/闪烁（而非每帧检查）。
- **借鉴 foot**: `terminal.h:457-461`（delayed_render_timer）、`terminal.h:593-598`（cursor_blink fd）、`terminal.h:564-567`（blink fd）
- **实施内容**:
  1. `session/render_loop.go` 实现 delayed render：
     - 收到序列触发渲染时，不立即渲染，而是启动短定时器（如 4ms/8ms），合并期间所有序列为一次渲染
     - upper bound 定时器保证最大延迟（如 16ms 必渲染）
  2. 光标闪烁异步化：
     - 独立 `time.Ticker(500ms)` 驱动光标翻转，仅翻转时 damage 光标 cell + 置 dirty
     - 移除 render_loop 中 `tickCount%15` 兜底逻辑（改为光标 ticker 精准触发）
  3. BSU 与 delayed render 联动：BSU 期间禁用 delayed render timer
  4. session 层 `m.dirty` 与 screen 层 damage 联动：screen 有 dirty 行时才置 m.dirty
- **涉及文件**: `session/render_loop.go`、`session/master.go`、`terminal/terminal.go`
- **验证标准**:
  - `go build ./...` 通过
  - 稳态无输入时 CPU 占用下降（无 15tick 兜底渲染）
  - 光标闪烁稳定 500ms 翻转
  - 高输出时渲染帧数下降（delayed render 合并）
- **风险**: 中。定时器交互复杂，需处理 BSU/resize/退出时的 timer 清理。光标 ticker 与渲染主循环的同步。
- **审计结果**: 通过。主 agent 自主实施。cursorBlinkTicker 500ms 独立驱动光标闪烁（替代 15 tick 250ms 兜底）；移除 `m.tickCount%15` 兜底，无 dirty 时完全跳过渲染（稳态零渲染，仅 500ms 光标闪烁触发一次）；delayed render 评估：vistty 已有 60fps ticker 天然合并 seqCh 序列（序列在 ticker 间累积，无需额外 delayed_render_timer）；cursorBlinkTicker 触发 dirty→ticker.C 渲染→compositor.cursorVisible 检查 500ms 翻转 blinkOn（约 516ms 翻转，可接受）；BSU 联动：BSU 期间 cursorBlinkTicker 仍 dirty，但 BSU 通常 <500ms 最多一次渲染，不影响合并效果。build/vet/test 全通过。

### 阶段 5: Cell 紧凑化（评估后决定）

- **状态**: 待实施
- **目标**: Cell 从 16B 压缩到 12-13B，减少缓存占用与内存带宽（1000 行 scrollback × 260 cols 可省 ~1MB/终端）。
- **借鉴 foot**: `terminal.h:42-72`（位域 attributes 8B + char32_t 4B = 12B）
- **实施内容**（Go 无位域，用 uint 编码）:
  1. 方案评估：Color 改为 `uint32` 编码（24位 RGB + 2位 source + 标志位），省 `IsDefault bool`
  2. Width 合并到 Rune 高位（rune 仅需 21 位，uint32 高 11 位放 width:3 + flags）
  3. 目标布局：`Rune uint32`(4, 含 width) + `Fg uint32`(4) + `Bg uint32`(4) + `Attr uint16`(2) = 14B → padding 16B（收益有限）
  4. 或激进方案：`Rune uint32`(4) + `Packed uint64`(8，含所有 attr+fg+bg+flags) = 12B，但访问需位运算
  5. **建议**：先 benchmark 评估收益，若缓存命中率提升不显著则跳过（Go GC 对小对象开销可能抵消收益）
- **涉及文件**: `internal/screen/cell.go`、所有访问 Cell 字段的代码
- **验证标准**:
  - benchmark 确认渲染性能提升 ≥5% 才值得
  - `go test ./...` 全通过
- **风险**: 中高。触及 Cell 核心结构，影响面广。Go 手动位运算易错且降低可读性。**建议最后做，且仅当收益明确时**。
- **审计结果**: 

### 阶段 6: Wayland Buffer 优化

- **状态**: 待实施
- **目标**: Wayland 后端 buffer 复用 + age 追踪 + scroll 优化。
- **借鉴 foot**: `shm.h`（buffer_chain + age + shm_scroll）
- **实施内容**:
  1. `platform/wayland/surface.go`：wl_shm buffer 池复用（chain），避免每帧分配
  2. buffer age 追踪：busy 期间被请求次数，决定是否全量重绘
  3. `shm_scroll`：buffer 内 `memmove` 滚动行，配合 damage tracking 只重绘非滚动区
  4. `wl_surface.damage_buffer` 精确提交 dirty 区域（配合阶段 2 的 pixman_region 等价物）
- **涉及文件**: `platform/wayland/surface.go`、`platform/wayland/wl.go`
- **验证标准**:
  - Wayland 路径滚动性能改善
  - buffer 分配次数下降（debug 日志统计）
- **风险**: 中。Wayland 协议层改造，需熟悉 wl_shm pool 生命周期。DRM 路径不受影响。
- **审计结果**: 

### 阶段 7（可选）: Sixel + Grapheme Clustering

- **状态**: 待实施
- **目标**: 功能扩展（非性能优化），按需实施。
- **借鉴 foot**: `sixel.c`（Sixel 状态机）、`composed.c`（VS16/ZWJ 组合）
- **实施内容**:
  1. `internal/vte/sixel.go`：DCS Sixel 解析（DECSIXEL/DECGRA/DECGRI/DECGCI 状态机）
  2. `screen/sixel.go`：sixel 图像存储 + cell 引用 + 字体缩放重缩放
  3. `render`：sixel 渲染（CPU 直接 blit RGBA / GPU 纹理）
  4. `terminal/terminal.go` + `screen/cell.go`：VS16/VS15 感知 + ZWJ 序列组合（AGENTS.md 预留扩展点）
- **涉及文件**: `internal/vte/`、`internal/screen/`、`internal/render/`、`terminal/`
- **验证标准**: Sixel 图像正确显示（`img2sixel`）；emoji ZWJ 序列正确组合
- **风险**: 高。功能复杂，Sixel 解析状态机庞大。建议独立成项，不纳入本次优化。
- **审计结果**: 

---

## 优先级与依赖关系

```
阶段1 BSU ──────────────┐（BSU 禁用时一次性刷新，配合 damage 更高效）
                        ↓
阶段2 Damage Tracking ──┤（核心，后续阶段基础）
                        ↓
阶段3 环形缓冲 grid ──── ┤（滚动 damage 标记依赖 damage API）
                        ↓
阶段4 渲染调度 ───────── ┤（依赖 screen dirty 联动）
                        ↓
阶段5 Cell 紧凑化 ────── ┤（独立，评估后决定）
                        ↓
阶段6 Wayland buffer ─── ┤（依赖 damage region 提交）
                        ↓
阶段7 Sixel（可选）──── ─┘
```

**建议执行顺序**：1 → 2 → 3 → 4 → 6 → 5（评估） → 7（可选）

阶段 1-2 是最高价值组合（BSU + damage），阶段 3-4 是进阶优化，阶段 5-6 是平台/内存优化，阶段 7 是功能扩展。

## 变更记录

| 时间 | 阶段 | 操作 | 备注 |
|------|------|------|------|
| 2026-07-06 | - | 创建方案文档 | foot 克隆至 reference/，分析完成 |
| 2026-07-06 | 1 | 实施完成 | BSU 状态机 + timer + DSR/DECRQM 响应 |
| 2026-07-06 | 1 | 审计通过 | 修复 vte 不识别 DECRQM ?2026$p 缺陷，补测试 |
| 2026-07-06 | 2 | 实施完成 | 双层 dirty（cell AttrClean + line dirty）+ damage API + CPU 路径 dirty 跳过 |
| 2026-07-06 | 2 | 审计通过 | 修复光标行 cursorVisible 检查导致 blinkOff 残影，改光标行总是渲染 |
| 2026-07-06 | 3 | 实施完成 | 环形缓冲 grid（offset+mask+cap，全屏滚动 O(1)，region 逐行 copy） |
| 2026-07-06 | 3 | 审计通过 | 修复 region copy 指针别名，新行改 NewLine |
| 2026-07-06 | 4 | 实施完成 | cursorBlinkTicker 500ms 异步光标 + 移除 15 tick 兜底 |
| 2026-07-06 | 4 | 审计通过 | delayed render 评估（已有 ticker 合并），光标异步化完成 |
