# Vistty Master/Slave 多屏架构改造

## 决策汇总

| 维度 | 决策 |
|------|------|
| 架构 | master/slave，同进程 goroutine |
| 模式 | `-mode mirror\|independent` 启动参数，无运行时切换 |
| 镜像渲染 | master 集中渲染，按主屏 winsize，其他屏裁剪黑边 |
| 独立渲染 | 每 slave 独立 compositor+faceCache，串行渲染 |
| font 归属 | mirror 全局共享；independent 每 slave 独立 |
| 主屏 | `-primary <名称\|索引>`，默认第一个 connected |
| 设备范围 | 单卡多 connector |
| GBM | P1，强制 Atomic Modesetting |
| tabs | 仅预留接口（Slave.terms[]/active 字段就位），后期实现 |
| scrollOffset | 暂留 Terminal，后期重做为 string 历史检索 |
| 后台 terminal winsize | 冻结最后值 |
| 模式切换 | 无热键，仅启动参数 |
| 缩放热键 | Ctrl→Mod(Win)，Mod+=/-/0 |
| 焦点切换 | Mod+1..9 / Mod+Tab（independent） |

## 架构总览

```
cmd/vistty -mode mirror|independent -primary <name|idx>
    └── master.New(backend, opts)
          ├─ 枚举 outputs → 主屏标记 → 每 output 一个 Slave
          ├─ Terminal 池（active 标志 + 绑定分配）
          ├─ input 路由（Mod+num/Tab 焦点；Mod+=/-/0 缩放）
          ├─ 渲染主循环（LockOSThread）
          │    mirror:    master 持 compositor+faceCache → 渲染1次 → 裁剪分发各 slave
          │    independent: 各 slave 自持 compositor+faceCache → 串行渲染 → commitAll
          └─ commitAll()（dumb: per-surface Swap；GBM: AtomicCommit）

Slave {
    output, surface, backBuf
    terms    []*Terminal   // 绑定列表（mirror:共享单T；independent:独占T；tabs预留:多T）
    active   int           // 焦点 T 索引（tabs 预留，当前恒 0）
    // 独立模式独有：
    compositor *Compositor, faceCache *FaceCache, scrollOffset int
}

Terminal（简化后，纯逻辑会话）{
    screen(mainBuf+altBuf), cursor, parser, pty, ptyCmd
    seqCh, eofCh, mu, active, done
    终端状态: curFg/curBg/curAttr/saved/autoWrap/charset/tabStops/...
    scrollOffset(暂留), cols/rows
    方法: FeedBytes/Apply/WriteToPTY/SetPtySize/Close — 无 Render/Swap/Run/HandleKey
}
```

## 热键方案

| 快捷键 | 功能 | 适用模式 | 改动 |
|--------|------|---------|------|
| Mod+=/-/0 | 放大/缩小/重置字体 | 两者 | 替换 Ctrl（terminal.go:1190） |
| Mod+1..9 | 焦点切到第 N 屏 | independent | 新增 |
| Mod+Tab | 轮转焦点 | independent | 新增 |

底层 `ModSuper`（input.go:16）已就绪，仅需 master input_route 加分支；keymap.go:37 补 126:ModSuper（右Win）。

## 文件改造清单

### 新增
| 文件 | 职责 |
|------|------|
| `platform/output.go` | Output 接口（ID/ConnectorID/CrtcID/Mode/Size/Name） |
| `master/master.go` | 枚举+主屏标记+Terminal池+焦点+input路由+模式(font归属) |
| `master/render_loop.go` | 统一主循环: 镜像裁剪分发/独立串行 + commitAll |
| `slave/slave.go` | Slave: output+surface+backBuf+terms[]+active+(独立)compositor/faceCache |

### 改造
| 文件 | 改动 |
|------|------|
| `terminal/terminal.go` | 剥离 surface/compositor/input/backend/font/Run/handleKey/handleScale/handleResize/fps；保留 PTY+screen+parser+状态+CSI执行+FeedBytes/Apply；增 active/mu/cols/rows |
| `terminal/options.go` | 增 Primary string + Mode string |
| `platform/drm/display.go` | findDisplay→findOutputs()；DisplayInfo 增 Name |
| `platform/drm/backend.go` | 单字段→outputs+surfaces map；CreateSurfaceFor；eventLoop 按 ev.CrtcID 路由 |
| `platform/drm/surface.go` | 增 OutputID() |
| `platform/keymap.go:37` | 补 126:ModSuper（右Win） |
| `cmd/vistty/main.go` | 增 -primary/-mode flag；terminal.New→master.New |

### GBM（P1）
| 文件 | 职责 |
|------|------|
| `platform/drm/internal/gbm/` | purego dlopen libgbm.so+libEGL.so |
| `platform/drm/internal/atomic.go/property.go/plane.go` | 填充骨架 |
| `platform/drm/gbm_device.go/gbm_surface.go/atomic_commit.go` | GBM 后端 |

## 关键设计要点

1. **eventLoop CrtcID 路由**（P0a 硬前提）：backend.go:147-149 现丢弃 ev.CrtcID，多屏 flip 串扰。改 surfaces[ev.CrtcID].notifyFlip()。
2. **Terminal 并发保护**：pty-read goroutine 写 screen（apply 持 mu 写锁），master/View 渲染读（RLock）。镜像共享单 T 时多 slave 读同一 T，只读无竞争。
3. **镜像裁剪**：master 渲染主屏 backBuf，各 slave 按 min(主屏cols,本屏cols)×min(主屏rows,本屏rows) 拷左上区域，超出填背景色。
4. **scrollOffset 暂留 Terminal**：当前保留现有机制，镜像多屏共享会互相干扰滚动（已知暂时缺陷）。后期重做时移除。
5. **Wayland 适配**：ListOutputs() 返回单虚拟输出，退化为单 slave，镜像/独立无差异。

## 阶段追踪

### P0a：接口 + dumb buffer 多屏路由 ✅
- [x] platform/output.go — Output 接口
- [x] platform/backend.go — ListOutputs/CreateSurfaceFor 接口
- [x] platform/surface.go — 增 OutputID()
- [x] platform/drm/display.go — findDisplay→findOutputs()；DisplayInfo 增 Name + 字段 unexported 消冲突
- [x] platform/drm/backend.go — outputs[]+surfaces map；CreateSurfaceFor；eventLoop CrtcID 路由修复
- [x] platform/drm/surface.go — OutputID() 实现 + connectorID 字段
- [x] platform/keymap.go — 补 126:ModSuper
- [x] platform/wayland/ — ListOutputs/CreateSurfaceFor/waylandOutput 适配
- [x] go build ./... 通过
- [x] go vet ./... 通过
- [x] go test ./... 通过（9 包 ok）
- **状态**: ✅ 完成

### P0a 审计记录
- eventLoop CrtcID 路由（backend.go:186-192）：优先 `b.surfaces[ev.CrtcID].notifyFlip()`，fallback `b.surface` —— 多屏正确性硬前提已修复
- findOutputs（display.go:60）：返回所有 connected 输出，preferred mode 选择（modeTypePreferred），21 种 connector type name 映射
- DisplayInfo 字段改 unexported（connID/crtcID/mode/savedCrtc/name）消除 field/method 同名冲突，实现 platform.Output 接口
- DRMSurface 增 connectorID 字段，newDRMSurface 签名同步更新，CreateSurface/CreateSurfaceFor 调用处已更新
- Wayland waylandOutput 单虚拟输出适配，WaylandSurface.OutputID() 返回 0
- 测试 mock（terminal_test/harness/compositor_test）均已补新接口方法
- 保留旧 CreateSurface(w,h) 兼容现有 terminal.go 单屏路径

### P0b：Terminal 简化 + Slave/Master + 镜像模式 ✅
- [x] terminal/terminal.go — 剥离 12 字段(compositor/surface/input/backend/face/faceCache/fontData/initialFontSize/scaleReqCh/resizeCh/fpsLogging/wg)；新增 mu/active/cols/rows；New(opts,cols,rows) 不依赖 backend；Apply/FeedBytes 持 mu 写锁；Screen/Cursor/HandleKey/PtyWrite 等导出
- [x] terminal/options.go — 增 Primary/Mode 字段
- [x] slave/slave.go — Slave 结构(output+surface+backBuf+terms[]+activeIdx)
- [x] master/master.go — 枚举+主屏匹配+font+slaves+Terminal池+compositor+input
- [x] master/render_loop.go — 镜像集中渲染裁剪分发(blitToSlave fillBlack+min尺寸拷贝)
- [x] master 缩放热键 Mod+=/-/0（替换 Ctrl）
- [x] cmd/vistty/main.go — -primary/-mode flag；master.New
- [x] terminal/render_harness.go 适配 New 签名
- [x] terminal/*_test.go 适配（feedBytes→FeedBytes 90处）
- [x] master/master_test.go 迁移集成测试
- [x] go build ./... 通过
- [x] go vet ./... 通过
- [x] go test ./... 通过（10 包 ok 含新 master 包）
- **状态**: ✅ 完成

### P0b 审计记录
- Terminal 简化正确：剥离渲染/IO/主循环/字体职责，保留纯逻辑会话(PTY+screen+parser+状态+CSI执行)
- master.New 流程：ListOutputs→主屏匹配(名称或索引)→font→CreateSurfaceFor 每 output→Terminal.New(opts,cols,rows)→绑定所有 slave→compositor 绑主屏→input
- 镜像渲染裁剪：compositor.Render(主屏 Swap 内含)→blitToSlave 非主屏(fillBlack+min(主屏,本屏)尺寸拷左上)→Swap
- **审计修复**：handleResize/handleScale 漏调 SetPtySize → 已补 ft.SetPtySize(rows,cols)（resize/缩放后同步 PTY winsize）
- ModSuper 缩放热键拦截正确，Ctrl+C/D/Z 保留在 Terminal.HandleKey
- 两阶段关闭正确（signalClose→wg.Wait→backend.Stop→input.Close→cleanup）
- slave.backBuf 预留给 P0c 独立模式

### P0c：独立模式 + 焦点路由 + tabs 预留 ✅
- [x] slave/slave.go — 独立模式字段 compositor/faceCache/face + InitIndependent + Close 分路径
- [x] master/master.go — New 按 opts.Mode 分支 initMirror/initIndependent + signalClose 遍历所有 terms + renderReqCh
- [x] master/render_loop.go — renderIndependent 串行渲染跳过 !Active() + handleKey Mod+1..9/Tab 焦点切换 + setFocus renderReqCh
- [x] Slave.terms[]/activeIdx 字段就位（tabs 预留）
- [x] Terminal.Active() 渲染跳过 + 后台 winsize 冻结（active 恒 true，PTY 继续读但跳过渲染）
- [x] 多 seqCh fan-in（N goroutine → unifiedSeqCh，mirror nil channel 不触发）
- [x] go build ./... 通过
- [x] go vet ./... 通过
- [x] go test ./... 通过（10 包 ok 含 master）
- **状态**: ✅ 完成

### P0c 审计记录
- 独立模式渲染（renderIndependent）：遍历所有 slave，跳过 t==nil 和 !t.Active()，各 slave 用自己的 Compositor.Render（含 Swap），串行渲染正确
- 焦点路由：Mod+1..9（evdev code 2-10 → idx 0-8）、Mod+Tab（code 15 轮转）；setFocus 更新 focusIdx + 投递 renderReqCh 触发主线程立即渲染（避免 inputLoop 并发渲染）
- tabs 预留字段就位：Slave.terms []*Terminal + activeIdx int，ActiveTerm 返回 terms[activeIdx]，当前恒 0
- Terminal.Active() 访问器（terminal.go:146），active 恒 true，后台跳过渲染机制就绪
- 多 seqCh fan-in：independent 模式 N goroutine 合并 SeqCh→unifiedSeqCh(cap=16) + EofCh/Done→termExitCh(cap=1)；mirror 模式 nil channel select 永不触发，走原 mirrorSeqCh 路径
- slave.InitIndependent：创建 faceCache+face+compositor 绑定 slave 自身 surface；Close 按 independent/mirror 分路径（避免重复关 surface）
- signalClose 正确遍历所有 m.terms 调 SignalClose
- **审计修复**：handleResizeIndependent/handleScaleIndependent 漏调 SetPtySize → 已补 ft.SetPtySize(rows,cols)（4 处 SetPtySize 全部就位：mirror resize/scale + independent resize/scale）

### P1a：DRM Atomic/Property/Plane ioctl 封装 ✅
- [x] platform/drm/internal/atomic.go — AtomicCommit + HasAtomic + AtomicFlag 常量 + AtomicObject/AtomicProp 高层 API
- [x] platform/drm/internal/property.go — GetObjectProperties/GetProperty/CreateBlob/DestroyBlob/GetBlob + ModeObject 常量
- [x] platform/drm/internal/plane.go — GetPlaneResources/GetPlane + PlaneResult
- [x] platform/drm/internal/codes.go — 9 个新 ioctl 码（SET_CLIENT_CAP/GETPROPERTY/GETPROPBLOB/GETPLANERESOURCES/GETPLANE/OBJ_GETPROPERTIES/ATOMIC/CREATEPROPBLOB/DESTROYPROPBLOB）
- [x] platform/drm/internal/types.go — 8 个新结构体 + init() 大小/偏移校验
- [x] platform/drm/internal/cap.go — DRM_CLIENT_CAP_ATOMIC + SetClientCap + DRM_CLIENT_CAP_UNIVERSAL_PLANES
- [x] go build ./... 通过
- [x] go vet ./... 通过
- [x] go test ./... 通过
- **状态**: ✅ 完成

### P1a 审计记录
- 结构体布局全部匹配内核 UAPI（drm_mode_atomic 56B/drm_mode_get_plane_res 16B/drm_mode_get_plane 32B/drm_mode_get_property 64B/drm_mode_obj_get_properties 32B/drm_mode_create_blob 16B/drm_mode_get_blob 16B/drm_mode_destroy_blob 4B）
- ioctl 码经内核头文件校验（0xBC/0xB5/0xB6/0xB9/0xAA/0xAC/0xBD/0xBE/0x0d 均正确）
- Atomic 标志位正确（PageFlipEvent=0x01/PageFlipAsync=0x02/TestOnly=0x0100/NonBlock=0x0200/AllowModeset=0x0400）
- HasAtomic 通过 SetClientCap(DRM_CLIENT_CAP_ATOMIC=3) 探测（内核唯一方式，无 DRM_CAP_ATOMIC）
- 两步 ioctl 模式正确（GetObjectProperties/GetPlaneResources/GetPlane/GetProperty 先查 count 再查数据）
- AtomicCommit 高层 API：AtomicObject{ID,Props[]} → 内部构建 objs/count_props/props/values 四并行数组 + runtime.KeepAlive 防 GC 回收
- 空切片安全处理（len==0 时不取 &slice[0]）
- 修正了任务描述中多处与内核 UAPI 不符的错误（ioctl 码/结构体字段/标志位/CAP 探测方式）

### P1b：GBM + EGL purego dlopen + GBMSurface + AtomicCommitor ✅
- [x] platform/drm/internal/gbm/asm_amd64.s — 汇编 trampoline ccall0-ccall6（SysV AMD64 ABI + 16B 栈对齐）
- [x] platform/drm/internal/gbm/dlfcn.go — 自研 dlopen/dlsym（/proc/self/maps 定位 libc → ELF 头解析 → PT_DYNAMIC → GNU hash/SysV hash 符号查找）
- [x] platform/drm/internal/gbm/loader.go — LibHandle 封装 + OpenLib/Sym/Close
- [x] platform/drm/internal/gbm/gbm.go — GBMLoader（create_device/surface_create/lock_front_buffer/release_buffer/bo_get_handle/bo_get_stride 等）
- [x] platform/drm/internal/gbm/egl.go — EGLLoader（GetDisplay/GetPlatformDisplay/Initialize/ChooseConfig/CreateContext/CreateWindowSurface/SwapBuffers 等）
- [x] platform/drm/gbm_device.go — GBMDevice（SetClientCap→LoadGBM+LoadEGL→gbm_create_device→eglGetDisplay→eglInitialize→eglChooseConfig→eglCreateContext）
- [x] platform/drm/gbm_surface.go — GBMSurface implements Surface（Swap: eglSwapBuffers→lock_front_buffer→AddFB→CommitSingle→wait flipCh→release old BO）
- [x] platform/drm/atomic_commit.go — AtomicCommitor（属性ID缓存+primary plane发现+CommitSingle单CRTC/Commit多CRTC同步）
- [x] platform/drm/backend.go — GBM 可选初始化（HasAtomic→NewGBMDevice 成功 useGBM=true，失败回退 dumb buffer）
- [x] platform/drm/internal/mode.go — +CreateModeBlob + PlaneTypePrimary/Cursor 常量
- [x] go build ./... 通过
- [x] go vet ./... 通过（8 个 unsafe.Pointer 警告，ELF 内存解析必要操作）
- [x] go test ./... 通过
- **状态**: ✅ 完成

### P1b 审计记录
- 自研 dlopen 实现正确：不依赖 syscall.SYS_DLOPEN（dlopen 非系统调用），通过 /proc/self/maps 定位 libc → ELF 头/PT_LOAD/PT_DYNAMIC → GNU hash（含 SysV hash 回退）符号查找
- 汇编 trampoline 正确：ccall0-ccall6 设置 RDI/RSI/RDX/RCX/R8/R9 + SUBQ $8 保证 16 字节栈对齐（SysV ABI 要求 C 函数入口 SP%16==8）
- GBMDevice 初始化序列正确：SetClientCap(ATOMIC)+SetClientCap(UNIVERSAL_PLANES)→LoadGBM+LoadEGL→gbm_create_device→eglGetPlatformDisplay→eglInitialize→eglBindAPI→eglChooseConfig→eglCreateContext
- AtomicCommitor 正确：primary plane 发现（PossibleCrtcs & (1<<crtcIndex)）+ 属性ID缓存（CRTC: ACTIVE/MODE_ID，Plane: FB_ID/CRTC_ID/SRC_*/CRTC_*，Connector: CRTC_ID）+ SRC_W/H <<16（16.16 定点数）+ CommitSingle（modeset/pageflip 分支）+ Commit（多 CRTC 同步批提交）
- GBMSurface.Swap 流程正确：eglSwapBuffers→lock_front_buffer→AddFB→CommitSingle→wait flipCh→release old BO/FB
- backend.go GBM 回退正确：HasAtomic 成功→NewGBMDevice 成功→useGBM=true；任意步骤失败→静默回退 dumb buffer（现有路径不变）
- eventLoop 按 ev.CrtcID 路由 GBM surfaces
- GBMSurface.Data() 返回 nil（GPU 纹理无 CPU mmap），现有 CPU 渲染管线不兼容（GL 渲染管线为后续工作）
- go vet 8 个 unsafe.Pointer 警告（dlfcn.go ELF 内存解析的必要操作，无法避免）

## 审计记录

（每阶段完成后追加审计结论）
