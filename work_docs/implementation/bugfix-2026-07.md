# 缺陷修复实施方案（2026-07）

## 概述

基于 `work_docs/fix.md` 代码审查报告，分 7 个阶段实施全部已确认缺陷修复。
共覆盖 P0 高危 8 项、P1 中危 28 项、P2 低危 36 项，以及工程改进（gofmt CI、回归测试、锁契约文档化）。

## 项目约束（所有阶段必须遵守）

- CGO_ENABLED=0，仅 linux/amd64，模块 `github.com/LaoQi/vistty`
- 依赖方向不可违反：`drm` 不依赖 `gbm`；`render` 不依赖 `ui`；`plugins` 不依赖 `session`
- Lua LState 线程契约：所有 gopher-lua PCall 只在主渲染线程执行
- 每阶段完成后必须运行：`go build ./... && go vet ./... && go test ./...`
- 不提交 git，除非用户明确要求

## 实施阶段

### 阶段 1: 可用性阻塞修复（P0-3 / P0-4）
- **状态**: 已审计
- **目标**: 修复 init.lua 钩子死锁与 Wayland 退出悬挂，恢复基本可用性
- **实施内容**:

  **P0-3 on_activate / init.lua 钩子调用阻塞型 API 死锁**
  - 位置：`cmd/vistty/main.go:201`（Activate 在 Run 前），`session/master.go:472-479`（NewTab 阻塞发送），`master.go:150,153`（scaleReqCh/tabReqCh cap=1）
  - 修复：将 PluginContext 投递方法（NewTab/CloseCurrentTab/SwitchTab/SetScale/ZoomIn/ZoomOut）改为非阻塞 `select { case ch <- req: default: }`，满时丢弃 + debug.Warningf 日志
  - 同时将 `pm.Activate(m)` 移到 `m.Run()` 内部第一帧前执行（通过新增回调或 Run 内首行调用），双保险

  **P0-4 WaylandBackend.Stop 无法唤醒阻塞 recvmsg，退出悬挂**
  - 位置：`internal/platform/wayland/backend.go:241-248`（Stop），`internal/platform/wayland/wl.go:127-145`（dispatch 阻塞 Recvmsg）
  - 修复：`conn.close()` 中先 `unix.Shutdown(fd, unix.SHUT_RDWR)` 再 `unix.Close(fd)`，使阻塞 recvmsg 立即返回
- **验证标准**:
  - `go build ./... && go vet ./... && go test ./...` 全通过
  - init.lua 中 `vistty.on_activate(function() vistty.tab.new(); vistty.tab.new(); vistty.zoom.in() end)` 启动不挂死
  - Wayland 后端关闭窗口 1 秒内退出（代码审查确认 Shutdown 路径）
- **审计结果**: 通过。
  - 8 个 PluginContext 方法改非阻塞（master.go），保留 `<-m.done` 逃逸分支
  - Activate 移入 Run()（render_loop.go），ApplyTheme 保持在前（main.go 删除 Activate 调用）
  - Reload 顺序一致化（manager.go：ApplyTheme 先于 Activate）
  - wl.go close() 用 sync.Once 幂等 + Shutdown 唤醒 recvmsg（conn 始终指针使用，sync.Once 安全）
  - **审计改进**：tabReqCh/scaleReqCh 容量 1->8，使 on_activate 多请求可排队处理（非阻塞仅作溢出兜底），满足验证标准"标签/缩放最终生效"
  - 构建/vet 清洁；测试除预存在 TestP6ExampleInitLua（stash 验证与本次无关）外全通过

### 阶段 2: 终端语义 - scrollback/region（P0-5 / P0-7）
- **状态**: 已审计
- **目标**: 修复 DECSTBM 无参数重置失效与 scrollback 污染
- **实施内容**:

  **P0-5 ESC[r 无法重置滚动区域**
  - 位置：`internal/vte/csi.go:124-133`（case 'r'），`terminal/terminal.go:617-622`，`internal/screen/buffer.go:201`（top>bot guard）
  - 修复：csi.go 保留参数原样（缺省 bottom=0）；terminal 层处理：`bot <= 0` 解释为 rows（即 `SetScrollRegion(top-1, rows-1)`）；top 缺省（`ESC[r`）为整屏 `SetScrollRegion(0, rows-1)`
  - SetScrollRegion 内部 rows 边界 clamp（已有 bot>=rows clamp，确认 top clamp 完整）

  **P0-7 ScrollDown / 区域 ScrollUp 错误推入 history**
  - 位置：`internal/screen/buffer.go:159-165`（ScrollDown 全屏分支 Push），`buffer.go:132-138`（ScrollUp 区域分支 Push）
  - 修复：ScrollDown 两个分支（全屏 + 区域）都不 Push history（SD 语义：内容下移，底部行被挤出丢失，不进 scrollback）；ScrollUp 仅全屏分支（scrollTop==0 && scrollBot==rows-1）Push history，区域分支不 Push
- **验证标准**:
  - 新增 buffer 单测：SetScrollRegion(5,10) -> 发 ESC[r -> 断言 region 恢复 (0, rows-1)；发 ESC[3r -> 断言 region 为 (2, rows-1)
  - 新增 buffer 单测：全屏 ScrollUp(1) -> history +1；设 region 后区域 ScrollUp(1) -> history 不变；ScrollDown(1) -> history 不变
  - `go build ./... && go vet ./... && go test ./...` 全通过
- **审计结果**: 通过。
  - csi.go：NParams 改为 seq.NParams（不再硬编码 2）
  - terminal.go：top 缺省 1 / bot 缺省 rows，覆盖 ESC[r / ESC[0;0r / ESC[3r / ESC[3;10r 四场景
  - buffer.go：ScrollUp 全屏分支保留 Push，区域分支删除 Push；ScrollDown 全屏+区域两分支均删除 Push
  - 新增 8 测试（terminal_csi_test.go 4 + buffer_test.go 4）全通过，现有 scroll 测试无回归
  - 构建/vet 清洁；仅预存在 TestP6ExampleInitLua 失败

### 阶段 3: 终端语义 - IL/DL + dirty 渲染（P0-6 / P0-1 / P1-8）
- **状态**: 已审计
- **目标**: 修复 IL/DL 滚动范围、CPU dirty 路径光标行擦除、execPrint 每字符 DamageLine
- **实施内容**:

  **P0-6 IL/DL 忽略光标行，滚动整个区域**
  - 位置：`terminal/terminal.go:599-604`
  - 修复：buffer 新增 `InsertLines(row, n)` / `DeleteLines(row, n)`：以 `[row, scrollBot]` 为临时区域执行行移动（复用环形缓冲 region 滚动逻辑），不触碰 history；terminal CSI L/M 改调新方法（光标在 region 外时 no-op）

  **P0-1 CPU dirty 渲染路径光标移动导致整行内容被擦除**
  - 位置：`internal/render/compositor.go:271-288`，`internal/screen/buffer.go:358-370`，`terminal/terminal.go:425-428`
  - 修复方案(a)：compositor 不清整行，改为逐 cell 处理--非 Clean 的 cell 先 cell 级背景 FillRect 再绘字形，Clean cell 跳过；移除 dirty 行整行 FillRect（line 275）
  - 保留 DamageCursor 细粒度（无需退化为 DamageLine）

  **P1-8 execPrint 每字符 DamageLine 导致 O(cols²)**
  - 位置：`terminal/terminal.go:478`，`internal/screen/buffer.go:341-356`
  - 修复：execPrint 只对触碰 cell（col 及宽字符 col+1）SetDirty + `line.SetDirty(true)`；批量序列执行后统一 damage（与 P0-1 方案 a 协同）
- **验证标准**:
  - 新增 buffer 单测：构造 10 行内容，光标在第 3 行，IL(2) -> 断言 1-2 行不动，3-8 下移 2 行，9-10 被挤出且 history 无新增
  - 新增 compositor 像素级测试：fake Surface（DirectRender=false）写入文本 -> 移动光标 -> Render -> 断言旧光标行非光标列像素仍为字形而非纯背景色
  - benchmark：`go test -bench . ./terminal/ -benchmem` 对比 cat 场景
  - `go build ./... && go vet ./... && go test ./...` 全通过
- **审计结果**: 通过。
  - buffer.go：新增 InsertLines/DeleteLines（复用 ScrollDown/ScrollUp region 移位模式，向下移位避免别名，NewLine 避免 Fill 覆盖，不进 history）；DamageCell 提取，DamageCursor 改委托
  - terminal.go：CSI L/M 改调 InsertLines/DeleteLines(cursor.Row)；execPrint 用 DamageCell(row, writtenCol) 替代 DamageLine（宽字符额外标 col+1），writtenCol 在自增前保存
  - compositor.go：删除 dirty 路径整行 FillRect；per-cell bg 条件改 `useDirty || (bg!=defBg)`，dirty 路径逐 cell 清 bg，Clean cell 保留上一帧像素
  - 新增 15 测试（buffer 7 + compositor 3 + terminal 5）全通过；DamageCursor 调用方经 wrapper 兼容
  - subagent 修正：InsertLines/DeleteLines Fill 阶段始终 NewLine（避免指针别名覆盖 shift 结果）；compositor 测试光标移到 row 1 使 row 0 Clean cell 真正被跳过
  - 构建/vet 清洁；仅预存在 TestP6ExampleInitLua 失败

### 阶段 4: Wayland surface 生命周期（P0-2 / P1-6 / P1-20 / P1-21 / P1-22）
- **状态**: 已审计
- **目标**: 修复 resize use-after-munmap、buffer release 跟踪、resize 失败泄漏、8KB 缓冲截断、fd CLOEXEC
- **实施内容**:

  **P0-2 resize 与渲染 use-after-munmap 竞争**
  - 位置：`internal/platform/wayland/surface.go:246-289`，`compositor.go:220-221`，`surface.go:136-141`
  - 修复：resize 两阶段--dispatch 线程 onConfigure 只记录 pending 尺寸并投递 ResizeEvent；实际 buffer 替换延迟到渲染线程处理 ResizeEvent 时执行；旧 buffer 销毁等 wl_buffer.release

  **P1-6 未跟踪 wl_buffer.release**
  - 位置：`surface.go:160-181`（Swap），`268-281`（resize 销毁）
  - 修复：为 wlBuffer 注册 release 事件，每块 buffer 维护 released 标志；Swap 选已 release 的 buffer，都未 release 时新建第三块；resize 销毁前确认已 release

  **P1-20 resize 失败路径状态不一致 + 新 buffer 泄漏**
  - 位置：`surface.go:246-266`
  - 修复：先创建全部新 buffer，全部成功后才提交尺寸字段；失败时完整销毁已创建的新 buffer

  **P1-21 wl.go dispatch 固定 8KB 缓冲，超大消息误判连接关闭**
  - 位置：`internal/platform/wayland/wl.go:66`（inBuf 8192），`127-145`
  - 修复：缓冲按需增长（append 风格），检查 MSG_CTRUNC

  **P1-22 Wayland socket fd 未设 CLOEXEC**
  - 位置：`internal/platform/wayland/wl.go:52`
  - 修复：`unix.Socket(AF_UNIX, SOCK_STREAM|SOCK_CLOEXEC, 0)`；审计 DRM evdev/tty 打开路径补 O_CLOEXEC
- **验证标准**:
  - `go build ./... && go vet ./... && go test -race ./internal/platform/wayland/ ./internal/render/ ./...` 全通过
  - 代码审查确认两阶段 resize + release 跟踪 + 失败路径对称
  - 实机：Wayland 后端快速拖动窗口 resize 30 秒不崩溃（如环境允许）
- **审计结果**: 通过。
  - surface.go：两阶段 resize（onConfigure 记 pending + Data() 持锁 applyResizeLocked）；P1-20 先创建 newBufs 全成功才提交尺寸+替换+销毁旧，失败完整销毁已创建；P1-6 shmBuf.released *bool 共享指针 + onRelease 回调（dispatch 线程持 s.mu 写）+ Swap 检查未释放跳帧；提取 destroyShmBuf 公共函数
  - wl.go：wlBuffer.onRelease 字段 + createBuffer handler（opcode 0 触发）；dispatch 动态扩容 inBuf（size>cap 倍增）+ MSG_CTRUNC 检查 + size<8 校验；SOCK_CLOEXEC
  - drm：device.go OpenCard/OpenRender、backend.go 两处 OpenFile、vt.go syscall.Open 全部 +O_CLOEXEC（evdev 库内部无法修改，已跳过）
  - **锁序核查**：dispatch 在 c.mu.Unlock() 后调 onEvent（:182-184），onRelease 取 s.mu 不持 c.mu；applyResizeLocked 持 s.mu 调 writeMsg 取 c.mu。锁序 s.mu->c.mu 单向，无反转
  - 新增 3 测试（TestConnDispatchGrowth/TestWlBufferReleaseEvent/TestWlBufferReleaseNoCallbackNoPanic）全通过
  - 构建/vet 清洁；仅预存在 TestP6ExampleInitLua 失败

### 阶段 5: GBM/DRM/VT 资源（P0-8 / P1-23 / P1-24 / P1-26）
- **状态**: 已审计
- **目标**: 修复 GBM flip 超时泄漏、VT 切回 5 秒卡顿、CRTC 恢复不带 connector、BGRA 纹理参数非法
- **实施内容**:

  **P0-8 GBM flip 超时后 committed 帧被覆盖，BO+FB 泄漏**
  - 位置：`internal/platform/gbm/surface.go:398`（committed 覆盖），`433-443`（超时分支）
  - 修复：Swap 覆盖 s.committed 前检查非空，按 onFlipComplete 释放路径（RmFB + SurfaceReleaseBuffer + scanout 记账调整）释放旧帧；持 commitMu 顺序一致

  **P1-23 VT 切回后首个 Swap 固定等待 5 秒**
  - 位置：`internal/platform/drm/surface.go:119-143`，`gbm/surface.go:410-444`，`drm/backend.go:72-79`
  - 修复：OnDeactivate 时遍历 surface 清 flipPending（或向 flipCh 投递伪完成信号）；与 P0-8 committed 释放协同

  **P1-24 退出时 CRTC 恢复不带 connector，错误被吞**
  - 位置：`internal/platform/drm/backend.go:223-232`
  - 修复：恢复时带原 connector ID（DisplayInfo.connID）；失败记 debug 日志

  **P1-26 initGL BGRA 纹理上传参数非法，未查错误**
  - 位置：`internal/platform/gbm/surface.go:192-196`
  - 修复：hasBGRA 时 internalformat 也用 GL_BGRA_EXT；补 GetError 检查并记日志
- **验证标准**:
  - `go build ./... && go vet ./... && go test ./...` 全通过
  - 代码审查确认释放路径对称、VT 切换 flipPending 清理、CRTC 恢复带 connector
  - 实机（如环境允许）：`scripts/gbm-bench.sh` 长跑后 GEM 句柄不增长
- **审计结果**: 通过。
  - gbm/surface.go：P0-8 waitForFlipComplete 超时分支模拟 onFlipComplete 轮转（releaseBO=scanout; scanout=committed; committed=nil）；P1-23 SetActive(false) flipPending 时同轮转；P1-26 internalFmt 与 uploadFmt hasBGRA 时同用 GL_BGRA_EXT + GetError 前后检查
  - drm/surface.go：P1-23 SetActive(false) flipPending 时清零 + 排空 flipCh
  - drm/backend.go：P1-24 SetCrtc 带 []uint32{out.connID} + 失败 debug.Warningf
  - gbmProvider.SetActive 已遍历 surfaces 传播（device.go:256-262），无需额外改动
  - 新增 6 测试（gbm 3 + drm 3）全通过
  - 构建/vet 清洁；仅预存在 TestP6ExampleInitLua 失败

### 阶段 6a: P1 剩余 - 终端/parser/渲染（11 项）
- **状态**: 已审计
- **目标**: parser 硬化、热路径分配、PtyWrite 写队列、emoji 肤色、RIS/saved cursor
- **实施内容**: P1-1/2/3/4/5/7/9/11/14/15/16
- **验证标准**: `go build ./... && go vet ./... && go test ./...` 全通过 + 新增单测
- **审计结果**: 通过。
  - parser.go：P1-2 Params [16]->[32]int（全项目 [16]int 统一改）；P1-3 OSC/DCS data 64KB 限制；P1-4 curParam 65535 钳制；额外修复 feedGround ESC 时清 intermed（防 stale intermed 污染后续 ESC）
  - csi.go：P1-5 删除 ?25 特判统一走 handleMode
  - terminal.go：P1-1 HandleKey 滚动改 SetScrollOffset + render_loop handleKey 后设 dirty；P1-7 debug.Enabled() 包裹；P1-9 seqPool cap 256 + cap>2048 丢弃；P1-11 writeCh(64)+ptyWriteLoop 异步写队列+nil 回退（测试兼容）；P1-15 fullReset 补全 cursor/modes/title/syncUpdates；P1-16 Resize+restoreCursor clamp saved cursor
  - runeutil：P1-14 isEmojiModifier 增 Fitzpatrick 0x1F3FB-FF + RuneWidth 显式返回 0（绕过 x/text/width EastAsianWide 分类）
  - **审计修正**：移除 vte 对 internal/debug 的违规依赖（AGENTS.md "vte -> 无内部依赖"），截断改为静默
  - 新增测试（parser 3 + csi/terminal 2 + runeutil + terminal_state）全通过
  - 构建/vet 清洁；仅预存在 TestP6ExampleInitLua 失败

### 阶段 6b: P1 剩余 - 平台/goroutine（9 项）
- **状态**: 已审计
- **目标**: goroutine 泄漏、DRM input 竞态、inotify 溢出、eventLoop 退避、库句柄泄漏、ioctl hotplug、channel 关闭检查、除零、切片越界
- **实施内容**: P1-10/12/13/17/18/19/25/27/28
- **验证标准**: `go build ./... && go vet ./... && go test ./...` 全通过 + `-race` 平台测试
- **审计结果**: 通过。
  - master.go：P1-10 relay 增加 `case <-t.Done(): return` + SeqCh `, ok` 检查；P1-13 metrics guard
  - render_loop.go：P1-13 metrics guard；P1-28 KeyEvents/MouseEvents `, ok` 检查
  - compositor.go：P1-12 copyAllToSurface bounds clamp（同 stride + 逐行）；P1-13 NewCompositor Width guard
  - drm/input.go：P1-17 ready channel + fd 字段全加锁 + closeInotifyFdLocked/closeExitFdLocked（先置 -1 再 close）；P1-18 IN_Q_OVERFLOW 移到 name 前缀检查前
  - drm/backend.go：P1-19 eventLoop 错误退避 10ms
  - drm/mode.go/plane.go/property.go：P1-27 for 循环重试（count 增长 continue，缩小 [:count] 截断）
  - gbm/gbm.go + gl/egl.go + gl/gles.go：P1-25 Loader Close()（Dlclose，幂等）
  - gbm/device.go：P1-25 所有失败路径按依赖序 Close loader 回滚
  - **审计修正**：补 `defer i.closeInotifyFdLocked()` 修复 EpollCtl 失败路径 inotifyFd 泄漏（subagent 移除显式清理后未补 defer）
  - 新增 4 测试（relay 退出 + copyAll 越界 + zero metrics）全通过
  - 构建/vet 清洁；仅预存在 TestP6ExampleInitLua 失败

### 阶段 7: P2 + gofmt + 回归测试
- **状态**: 已审计
- **目标**: 清理低危健壮性问题、统一 gofmt、补强锁契约文档
- **实施内容**:
  - 全量 `gofmt -w .`（已执行，`gofmt -l .` 输出空）
  - P2-1 删除 Compositor.frameCount 死字段（仅自增无读取）
  - P2-3 feedUTF8 非法续字节发 ReplacementChar 后 `p.dispatch(b)` 重处理（避免吞 ASCII/ESC）
  - P2-4 FillRect/FillRectBlend 内部 clamp 到行边界（`row*stride+stride`），防止宽字符光标渗入下一行
  - P2-5 handleMode 区分 csi.Private：ANSI 模式（case 4 IRM/insertMode）走 `!csi.Private` 分支；DEC 私有模式走原有 switch；csi.go 补 `case 'h'/'l'` 非私有路径（原仅 parseCSIPrivate 处理）
  - P2-7 SetEraseCell 过滤 attr，strip AttrBold/Dim/Italic/Underline/Blink/Reverse/CrossedOut/Clean（仅保留颜色）
  - P2-9 startPty 先过滤已有 TERM=/COLORTERM= 再追加（避免重复环境变量）
  - P2-10 case 47/1047 复位分支补 `t.scrollOffset = 0`（与 1049 一致）
  - P2-11 删除 slave.go ResizeTerms 中重复 SetPtySize 调用（terminal.Resize 已调）；terminal.go 删除未使用的 SetPtySize 导出方法
  - P2-14 Buffer 字段 `cap` 改名 `capacity`（避免遮蔽内建函数）；NewBuffer 局部变量同步改名
  - P2-22 drm/input.go 文件头注释声明"仅支持键盘（EV_KEY），鼠标未实现"
  - P2-23 wlSeat/wlKeyboard/wlPointer 增加 version 字段；bindSeat 保存 `min(version,5)`；release 前检查 `version >= 5`（seat）/ `version >= 3`（keyboard/pointer），低版本跳过 release 请求
  - P2-27 Compositor 增加 `gpuDisabled bool` 永久禁用标志；BeginFrame 失败后置 `gpu=nil; gpuDisabled=true`，下一帧检查跳过重新赋值；同时删除 P2-1 的 frameCount 两次自增
  - P2-29 isValidSide 拒绝 "top"（仅接受 bottom/left/right），enable("top",n) 返回 error
  - P2-30 api_keybind bind_keys 改安全断言 `num, ok := v.(lua.LNumber); if !ok { L.RaiseError(...) }`
  - P2-32 GBMSurface.Close 从 GBMDevice.surfaces map `delete(s.crtcID)`；Close 补 nil-safe eglLoader 检查
  - 锁契约文档化：Terminal 类型定义处注释"主线程单线程"假设（无锁 accessor 列表）；Compositor.Render 注释 RLock 下写 dirty 位的单线程串行假设；GBMSurface.Close 注释 commitMu 锁契约
  - 错误日志（P2-19）：vt.go VT_RELDISP/VT_ACTIVATE ioctl 失败补 debug.Warningf；SetTextMode Close 失败补 warning；wl.go wl_surface.attach/damage/commit writeMsg 失败补 debug.Warningf
- **验证标准**:
  - `gofmt -l .` 输出为空
  - `go build ./... && go vet ./... && go test ./...` 全通过
- **审计结果**: 通过。
  - 新增 11 测试（vte parser 2 + terminal mode 5 + render 2 + plugins 2 + wayland 1 + gbm 1）全通过
  - 构建/vet 清洁；gofmt 清洁
  - P2-5 审计修正：发现 csi.go ParseCSI 非私有路径缺少 `case 'h'/'l'`（原仅 parseCSIPrivate 处理 SM/RM），ESC[4h 被误判 CSIUnknown；补 case 后 ANSI IRM 模式 4 正确路由到 handleMode 的 `!csi.Private` 分支
  - P2-32 审计修正：Close 中 MakeCurrent 原无条件调用 nil eglLoader 崩溃，补 `eglLoader != nil && (glInitDone || eglContext != 0)` 守卫

## 变更记录
| 时间 | 阶段 | 操作 | 备注 |
|------|------|------|------|
| 2026-07-21 | 文档 | 创建实施计划 | 基于 fix.md 分 7 阶段 |
| 2026-07-21 | 阶段1 | 实施完成并审计通过 | P0-3/P0-4；审计改进 channel 容量 1->8 |
| 2026-07-21 | 阶段2 | 实施完成并审计通过 | P0-5/P0-7；8 新测试通过 |
| 2026-07-21 | 阶段3 | 实施完成并审计通过 | P0-6/P0-1/P1-8；15 新测试通过 |
| 2026-07-21 | 阶段4 | 实施完成并审计通过 | P0-2/P1-6/P1-20/P1-21/P1-22；3 新测试通过；锁序核查无反转 |
| 2026-07-21 | 阶段5 | 实施完成并审计通过 | P0-8/P1-23/P1-24/P1-26；6 新测试通过 |
| 2026-07-21 | 阶段6a | 实施完成并审计通过 | 11 项终端/parser；审计移除 vte->debug 违规依赖 |
| 2026-07-21 | 阶段6b | 实施完成并审计通过 | 9 项平台/goroutine；审计补 inotifyFd defer 泄漏修复 |
| 2026-07-21 | 阶段7 | 实施完成并审计通过 | 16 项 P2 + 锁契约文档 + 错误日志；11 新测试通过；审计修正 csi.go 缺 h/l case |
| 2026-07-21 | 全部 | 实施完成 | gofmt 清洁 + 全量测试零失败（含原预存在 TestP6ExampleInitLua 修复）|

## 完成总结

全部 7 阶段（含 6a/6b 拆分共 8 个实施批次）审计通过，覆盖 fix.md 全部 P0（8 项）+ P1（28 项）+ P2 有影响项（16 项）+ 工程改进（gofmt、锁契约文档、错误日志、回归测试）。

### 最终验证
- `gofmt -l .`：空
- `go build ./...`：通过
- `go vet ./...`：通过
- `go test ./...`：全部通过，零失败

### 新增测试约 55 个
buffer 11 + compositor 6 + terminal 14 + vte 5 + runeutil 3 + wayland 3 + gbm 6 + drm 3 + session 1 + plugins 2 + render 1

### 主 agent 审计修正（3 处）
1. 阶段1：tabReqCh/scaleReqCh 容量 1->8（支持 on_activate 多请求排队）
2. 阶段6a：移除 vte 对 internal/debug 的违规依赖（AGENTS.md 依赖方向）
3. 阶段6b：补 `defer i.closeInotifyFdLocked()` 修复 EpollCtl 失败路径 inotifyFd 泄漏

### 未实施遗留（低优先级，可按需排期）
- P2-2/6/8/12/13/15/16/17/18/20/21/24/25/26/28/31/33/34/35/36：纯代码质量项（命名/微优化/冗余注释）
- evdev 库内部未设 O_CLOEXEC（无法不修改库修复）
- DRM 鼠标事件（EV_REL/EV_ABS）未实现（P2-22 已文档声明）
