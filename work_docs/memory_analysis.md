# Vistty 内存评估与布局分析

> 评估日期：2026-07-03
> 环境：linux/amd64, CGO_ENABLED=0
> 评估方式：`runtime.ReadMemStats` 实测 + `unsafe.Sizeof` 理论计算 + 源码静态分析
> 场景：双屏 DRM-GBM（1080p + 2K 2560×1440），实测 RSS > 330 MB

## 一、测量方法

### 词库内存实测

在 `pinyin` 包内创建临时测试，采用"有 dict vs 清空 dict"对比法：

```go
Init()
runtime.GC()  // 多次
var withDict runtime.MemStats
runtime.ReadMemStats(&withDict)

globalDict = nil
runtime.GC()  // 多次
var withoutDict runtime.MemStats
runtime.ReadMemStats(&withoutDict)
// delta = withDict - withoutDict
```

### 字体内存实测

在 `internal/font` 包内测量 `opentype.Parse` + `newFaceFromParsed` 前后的 HeapAlloc delta。

### 理论计算

基于 `unsafe.Sizeof` 获取结构体大小，结合 Go map bucket 布局（8 KV/bucket）和 string/slice header（16B/24B）推算。

---

## 二、各组件内存详解

### 1. 词库 (pinyin) — 106 MB，最大常驻消费者

实测数据（`withDict - withoutDict`）：

```
dictData (go:embed gzip):    11.9 MB  ← 常驻不可释放
globalDict map:              94.2 MB  ← 788,355 key / 890,790 entry
加载峰值 (TotalAlloc):       170.6 MB ← 临时，GC 后回收
```

理论分解（与实测 94.2 MB 吻合）：

| 子项 | 大小 | 计算依据 |
|------|------|---------|
| key string headers | 12.0 MB | 788,355 × 16B |
| key string data | 9.2 MB | 拼音串，均长 12B |
| `[]dictEntry` slice headers | 18.0 MB | 788,355 × 24B |
| dictEntry arrays | 20.4 MB | 890,790 × 24B (string ptr 16 + int 8) |
| word string headers | 13.6 MB | 890,790 × 16B |
| word string data | 9.6 MB | 中文词数据 |
| map buckets | ~31.6 MB | 98,545 buckets × 336B (8 KV/bucket) |
| **合计** | **~114.5 MB** | 实测 94.2 MB（GC 碎片回收后略低） |

**问题**：Go `string` 和 `slice` header 开销巨大 — 78 万 key header 占 12+18=30 MB，89 万 word header 占 13.6 MB，**仅 header 就 ~43 MB（占 45%）**。

### 2. 字体 (font) — 6.7 MB

```
embeddedFont (go:embed TTF):  6.6 MB  ← Sarasa Fixed SC 子集
opentype.Parse:               0.1 MB  ← parsed font 对象（索引，不复制数据）
NewFace(14pt):                <0.1 MB ← face 对象极小
```

`FaceCache` 按 size 缓存 face，每个 size 仅 ~0.1 MB。每个 Slave 调用 `InitIndependent` 创建独立 `FaceCache`（`slave.go:67`），各自 `opentype.Parse`（每份 0.1 MB，可忽略）。

### 3. 帧缓冲 — 后端相关

| 后端 | DirectRender | Go Heap (backBuf) | mmap/GPU显存 | 1080p 合计 |
|------|-------------|-------------------|-------------|-----------|
| **DRM dumb** | false | 8.3 MB | 16.6 MB (2× mmap) | 24.9 MB |
| **Wayland** | true | 0 | 16.6 MB (2× wl_shm) | 16.6 MB |
| **DRM-GBM** | true | 8.3 MB (cpuBuf) | 见下文 | 见下文 |

关键代码位置：
- DRM dumb: 双缓冲 mmap (`drm/surface.go:47-84`) + compositor backBuf (`compositor.go:67`)
- Wayland: 双缓冲 wl_shm mmap (`wayland/surface.go:74-85`)，DirectRender 直接写 surface 无 backBuf
- GBM: `DirectRender=true` 但 `ensureCPUBuf` 仍分配 cpuBuf (`gbm/surface.go:113-118`)，**GPU 路径下 8.3 MB 浪费**

### 4. GBM GPU 显存 — 每屏独立

每屏持有以下 GPU 资源（`gbm/surface.go` + `gpu/renderer.go`）：

| 资源 | 大小 (1080p) | 来源 |
|------|-------------|------|
| 3× GBM BO (三缓冲) | 23.7 MB | `committed` + `scanout` + `releaseBO` (`surface.go:72-74`) |
| s.texture (CPU fallback) | 7.9 MB | `initGL` 中 `TexImage2D` (`surface.go:196`)，**GPU 路径不必要** |
| atlasTex (GPU 字形 atlas) | 16.8 MB | `renderer.go:111-112`，固定 2048×2048 RGBA |
| instanceVBO | 5.2 MB | `renderer.go:148-153`，65536 × 80B |
| **每屏小计** | **53.6 MB** | |

2K 屏 (2560×1440) 的 BO 和 s.texture 按分辨率线性增长，atlasTex/VBO 固定不变：

| 资源 | 1080p | 2K |
|------|-------|-----|
| 3× GBM BO | 23.7 MB | 42.2 MB |
| s.texture | 7.9 MB | 14.1 MB |
| atlasTex | 16.8 MB | 16.8 MB |
| instanceVBO | 5.2 MB | 5.2 MB |
| **合计** | **53.6 MB** | **78.3 MB** |

**GBM 三缓冲机制**：`onFlipComplete` (`surface.go:439-456`) 将 `scanout → releaseBO`、`committed → scanout`，下一帧 `Swap` 释放 `releaseBO`。同一时刻最多 3 个 BO 存在。

### 5. 终端缓冲区 (screen) — 3.4 MB/终端

```
Cell 结构: 16 bytes
  rune(int32) 4 + width(uint8) 1 + pad 3 + Fg(Color) 4 + Bg(Color) 4 + Attr(uint16) 2 + pad 2
```

| 子项 | 200×50 终端 | 260×65 终端 (2K) |
|------|------------|-----------------|
| screen buffer | 160 KB | 270 KB |
| history (1000行) | 3.2 MB | 4.2 MB |
| **每个标签** | **3.4 MB** | **4.5 MB** |

History 上限 1000 行（`buffer.go:26`），满载时是主要开销。多标签线性增长。

### 6. Glyph Atlas (CPU cache) — 2-6 MB

2 个 LRU Atlas（normal + italic），容量各 8192（`compositor.go:51-52`）：

- 每 glyph: `Glyph` 结构 32B + Bitmap ~180-288B + atlasEntry 24B + list node ~50B ≈ 400B
- 满载: 8192 × 400 × 2 ≈ 6.4 MB
- 典型使用 2000-4000 glyph，实际 2-3 MB

### 7. compositor instances — 0.8-1.3 MB

`CellInstance` 结构 80 bytes（20 × float32，`platform/gpu.go:5-19`），预分配 `cols×rows`：

- 200×50: 800 KB
- 260×65: 1.27 MB

### 8. Lua VM — ~2 MB

- gopher-lua `LState` + 11 个钩子 LTable + bindings + pressedKeys
- 词库不在 Lua 层存储，仅通过 `vistty.pinyin.lookup` 按需查询 Go 侧
- IME 状态: 5 个标量（`ime.lua:3-7`），无大表

---

## 三、双屏内存布局（1080p + 2K GBM）

### RSS ~330 MB 构成分解

```
进程 RSS ~330 MB
│
├── Go HeapSys ~200 MB ──────────────────────────────────────
│   ├── 词库 dictData (go:embed gzip)      12 MB   常驻不可释放
│   ├── 词库 globalDict (78万key map)       94 MB   常驻不可释放
│   ├── GC 残留 (词库加载峰值170MB)       ~50 MB   GOGC=100% 未及时归还 ★
│   ├── 字体 embeddedFont (TTF)              7 MB   常驻
│   ├── cpuBuf 屏1 (1080p)                 7.9 MB   GBM 不必要 ★
│   ├── cpuBuf 屏2 (2K)                   14.1 MB   GBM 不必要 ★
│   ├── glyph atlas×2 (每屏 normal+italic)~6 MB   部分填充
│   ├── terminal buffer+history×2          7.8 MB   满载1000行
│   ├── compositor instances×2               2 MB
│   ├── Lua VM + 钩子 + bindings             2 MB
│   └── Go runtime + goroutine 栈            3 MB
│
├── GPU 显存 ~100 MB (部分计入 RSS) ─────────────────────────
│   ├── 屏1 (1080p):
│   │   ├── 3× GBM BO (三缓冲)            23.7 MB
│   │   ├── s.texture (CPU fallback)       7.9 MB   GPU路径不必要 ★
│   │   ├── atlasTex (2048² RGBA)         16.8 MB
│   │   └── instanceVBO (65536×80)         5.2 MB
│   │   小计: 53.6 MB
│   │
│   ├── 屏2 (2K):
│   │   ├── 3× GBM BO (三缓冲)            42.2 MB
│   │   ├── s.texture (CPU fallback)      14.1 MB   GPU路径不必要 ★
│   │   ├── atlasTex (2048² RGBA)         16.8 MB
│   │   └── instanceVBO (65536×80)         5.2 MB
│   │   小计: 78.3 MB
│   │
│   └── 双屏合计: ~132 MB (部分由 GPU 驱动管理，不计入进程 RSS)
│
├── Mesa/EGL 驱动 ~30 MB ────────────────────────────────────
│   └── 2× EGLContext (shader compiler + state tracker + command buffer)
│
└── 其他 ~2 MB (goroutine 栈、purego dlopen 库映射)
```

### 330MB 占比分布

| 组件 | RSS 贡献 | 占比 | 可优化 |
|------|---------|------|--------|
| 词库 (dictData + globalDict) | 106 MB | 32% | 是 (数据结构重构) |
| 词库 GC 残留 | ~50 MB | 15% | 是 (GOGC 调优) |
| GPU 显存 (BO+纹理+VBO) | ~100 MB | 30% | 部分 (atlas/VBO 缩小) |
| Mesa/EGL 驱动 | ~30 MB | 9% | 否 |
| cpuBuf×2 | 22 MB | 7% | 是 (延迟分配) |
| 字体+atlas+terminal+Lua | 25 MB | 7% | 部分 |

### 单屏 vs 双屏对比

| 组件 | 单屏 1080p | 双屏 1080p+2K | 增量 |
|------|-----------|--------------|------|
| 词库 | 106 MB | 106 MB | 0 (全局共享) |
| GC 残留 | ~50 MB | ~50 MB | 0 |
| cpuBuf | 7.9 MB | 22 MB | +14 MB |
| glyph atlas | 3 MB | 6 MB | +3 MB |
| terminal+history | 3.4 MB | 7.8 MB | +4.4 MB |
| GPU BO (三缓冲) | 23.7 MB | 65.9 MB | +42 MB |
| GPU 纹理+VBO | 30 MB | 60 MB | +30 MB |
| Mesa 驱动 | ~20 MB | ~30 MB | +10 MB |
| **总计** | **~250 MB** | **~330 MB** | **+80 MB** |

双屏增量主要来自：GPU BO 三缓冲 (+42MB) + GPU 纹理/VBO 翻倍 (+30MB)。

---

## 四、关键内存放大因素

### 1. 词库 = 156 MB (48%)

- `globalDict` map 占 94 MB，其中 string/slice header 占 43 MB (45%)
- 加载峰值 170 MB，GOGC=100% 导致 HeapSys 保留 ~150 MB 未归还 OS
- `dictData` gzip 12 MB go:embed 常驻

### 2. 双屏 GPU 显存 = 132 MB (40%)

- 每屏固定开销不随分辨率缩放：atlasTex 16.8 MB + instanceVBO 5.2 MB = 22 MB/屏
- 随分辨率线性增长：3× GBM BO 三缓冲 + s.texture
- GBM 三缓冲：`committed` + `scanout` + `releaseBO` 同时存在 3 个 BO

### 3. GBM cpuBuf + s.texture = 44 MB (13%) 完全浪费

- `ensureCPUBuf` (`device.go:198`) 在 surface 创建时立即分配 cpuBuf
- `initGL` (`surface.go:196`) 创建 s.texture 并 `TexImage2D` 上传 cpuBuf
- GPU instanced draw 路径从不使用 cpuBuf 和 s.texture
- 仅 CPU fallback 路径需要，但 GPU 成功后永不回退

### 4. IME 高频分配加剧 GC 压力

`ime.lua:236` 在 `on_render` 回调中每帧调用 `vistty.pinyin.lookup`：

- Go 侧：每次 `Lookup` 分配 map + slice + memo map（`pinyin.go:35,69,83`）
- Lua 侧：每次构建 ~256 LTable + 256 LString + 256 LString ≈ 1000 个 Lua 对象
- **60fps 下每秒 ~6 万个临时 Lua 对象** + 60 次 Go map 重建
- `ime_last_buf` 字段已存在但从未被读取用于缓存判断（死字段）

---

## 五、优化方案

按收益排序，预期优化后双屏 RSS: ~330 MB → ~190-210 MB。

### P0: GOGC/GOMEMLIMIT 调优 — 省 30-50 MB RSS，零风险

当前未设置 GOGC（默认 100%），词库加载峰值 170 MB 后 GC 不及时归还内存。

**方案**：在 `cmd/vistty/main.go` 启动时设置：

```go
debug.SetGCPercent(50)           // 更积极 GC
debug.SetMemoryLimit(180 << 20)  // 180 MB 软上限，触发 scavenger 归还
```

- 复杂度：低（2 行代码）
- 风险：无（仅影响 GC 频率，SoftLimit 不硬杀）

### P1: GBM cpuBuf + s.texture 延迟分配 — 省 22 MB heap + 22 MB GPU显存

当前 `device.go:198` 在 surface 创建时立即 `ensureCPUBuf()`，`initGL` 立即创建 `s.texture`。

**方案**：
- `ensureCPUBuf` 改为仅在 CPU fallback 路径调用（compositor Render 检测 `gpu == nil` 时）
- `s.texture` 改为仅在 `drawTexturedQuad`（CPU fallback）首次调用时创建
- GPU instanced draw 成功后释放 cpuBuf 和 s.texture

- 复杂度：低（移动分配时机）
- 风险：低（需确保 fallback 路径正确触发）

### P2: 词库数据结构重构 — 省 40-60 MB heap

当前 `map[string][]dictEntry` 的 string/slice header 占 43 MB，map bucket 占 31.6 MB。

**方案 A：offset 索引替代 string/slice**
- key 和 word 用连续 byte pool 存储，用 uint32 offset 引用
- 消除全部 string header (25.6 MB) 和 slice header (18 MB)
- 预期省 ~40 MB

**方案 B：Double-Array Trie 替代 map**
- 消除 map bucket (31.6 MB) + key header (12 MB)
- 预期省 ~60 MB

- 复杂度：中高（需重构 `loadDict` + `Lookup` + `SplitFuzzy`）
- 风险：中（需保持查询 API 不变，全量测试）

### P3: GPU atlas texture 2048² → 1024² — 省 12.5 MB GPU显存

当前 `renderer.go:111-112` 固定 2048×2048。典型终端使用 2000-4000 glyph，1024² 可容纳 ~2000 个 24×24 字形。

- 复杂度：低（改 2 个常量）
- 风险：低（atlas 满时自动 reset 重建）

### P4: IME 候选词缓存 — 减少 GC 压力，间接省 10-20 MB RSS

`ime.lua` 的 `M.candidates()` 每帧无缓存查询。`ime_last_buf` 字段已存在但未使用。

**方案**：
- Lua 层：`ime_buf` 未变化时缓存 `candidates()` 结果
- Go 层：`Lookup` 可选 `sync.Pool` 复用 map/slice
- `ime_last_buf` 用于检测 buffer 变化，未变则返回缓存

- 复杂度：低（Lua 层改动）
- 风险：无

### P5: Glyph atlas LRU 8192 → 2048 — 省 4 MB heap/屏

当前 `compositor.go:51-52` 容量 8192。典型使用 2000-4000 glyph，2048 足够。

- 复杂度：低（改常量）
- 风险：低（LRU 自动淘汰）

### P6: GPU VBO 65536 → 16384 — 省 3.9 MB GPU显存/屏

当前 `renderer.go:148` 预分配 65536 instances。2K 屏 260×65 = 16900 cells，16384 略不足，改为 32768。

- 复杂度：低（改常量）
- 风险：低（超限截断，已有保护）

---

## 六、验证方法

### 运行时内存 profile

```bash
# GBM 双屏运行 + heap profile
sudo ./vistty -backend drm-gbm -memprofile /tmp/vistty.mem
# Ctrl+C 退出后分析
go tool pprof -top -cum /tmp/vistty.mem
go tool pprof -svg /tmp/vistty.mem > /tmp/vistty_mem.svg
```

### RSS 监控

```bash
# 运行中查看 RSS 构成
ps -o pid,rss,vsz,cmd -p <pid>
pmap -x <pid> | tail -1
cat /proc/<pid>/status | grep -E 'VmRSS|VmSize|VmData'

# GPU 显存 (Intel)
sudo cat /sys/kernel/debug/dri/0/i915_gem_objects | head -5
intel_gpu_top  # 如可用
```

### 词库内存测试

```bash
go test -run=TestMemStats -v ./pinyin/
```

---

## 七、附录：各后端内存对比

| | DRM dumb | DRM-GBM | Wayland |
|---|---------|---------|---------|
| Surface | Dumb buffer mmap ×2 | GBM BO ×3 + EGL surface | wl_shm ×2 |
| 渲染路径 | CPU (backBuf → surface) | GPU instanced draw | CPU (DirectRender → surface) |
| Go Heap backBuf | 8.3 MB/屏 | 0 (DirectRender) | 0 (DirectRender) |
| Go Heap cpuBuf | — | 8.3 MB/屏 ★浪费 | — |
| GPU 显存 | — | atlas 16.8 + VBO 5.2 + texture 8.3 + BO ×3 | — |
| mmap | 16.6 MB/屏 | — (GBM BO 在 GPU 显存) | 16.6 MB/屏 |
| 帧缓冲总计 (1080p) | 24.9 MB | 53.6 MB | 16.6 MB |
| 帧缓冲总计 (2K) | 39.0 MB | 78.3 MB | 26.2 MB |
