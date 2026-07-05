# 双字体 Fallback 链 实施方案

## 概述

为终端模拟器 vistty 增加字体 fallback 机制：主字体（Sarasa Fixed SC subset）缺失某字形时，自动回退到第二字体（NerdFont subset，含 Powerline + Nerd 图标）查找。解决内置 Sarasa 子集缺失 Powerline（U+E0A0-E0D4）和 Nerd Font 图标（PUA）的问题，支持 powerline 状态栏与 starship/oh-my-posh 等工具。

**整体状态：已完成（3 阶段全部审计通过，最终回归测试通过）**

**核心设计**：`FallbackFace` 实现 `font.Face` 接口 → compositor/GPU/OSD 零改动，改动集中在 font 包 + 配置层 + session 层。

**Fallback 查找顺序**：
```
FallbackFace.Glyph(r):
  ① primary.Glyph(r)        [Sarasa，含 synthBlockElement 块字符合成]
     hit → 返回
  ② fallback.Glyph(r)       [NerdFont subset：Powerline/Nerd图标]
     hit → 调 YOffset 对齐 primary baseline → 返回
  ③ nil
```

**子集范围**：Powerline (U+E0A0-E0D4) + Nerd PUA 图标 (U+E000-F8FF)，约 1.1MB。

## 实施阶段

### 阶段 1: font 包核心
- **状态**: 已审计
- **目标**: 实现 FallbackFace 类型、FallbackFaceCache、embed NerdFont subset，compositor 无感知
- **实施内容**:
  1. 生成 NerdFont subset 字体文件：
     ```bash
     pyftsubset /usr/share/fonts/TTF/NotoMonoNerdFontMono-Regular.ttf \
       --unicodes=U+E000-F8FF \
       --notdef-outline \
       --output-file=internal/font/assets/NerdFontFallback.ttf
     ```
     （U+E000-F8FF 整个 PUA 含 Powerline E0A0-E0D4 + Nerd 图标，约 1.1MB）
  2. `internal/font/embedded.go`：新增 `//go:embed assets/NerdFontFallback.ttf` + `embeddedFallbackFont []byte` + `EmbeddedFallbackFontData() []byte`
  3. `internal/font/face.go`：新增 `FallbackFace` 类型：
     ```go
     type FallbackFace struct {
         primary  *OpenTypeFace
         fallback *OpenTypeFace  // 可为 nil
     }
     func NewFallbackFace(primary, fallback *OpenTypeFace) *FallbackFace
     func (f *FallbackFace) Metrics() Metrics              // 返回 primary.Metrics()
     func (f *FallbackFace) Glyph(r rune) (*Glyph, error)  // primary→fallback→nil，fallback hit 调 YOffset
     func (f *FallbackFace) Close() error                  // 关闭两者（fallback 非 nil 时）
     ```
     YOffset 对齐公式（fallback glyph 按 primary baseline 定位）：
     ```
     adjustedYOffset = originalYOffset + (fallbackAscent - primaryAscent)
     ```
     原理：compositor 渲染 `gy = py + primaryAscent + glyph.YOffset`；fallback glyph 原 YOffset 相对 fallback baseline，需平移 `(fallbackAscent - primaryAscent)` 对齐 primary baseline。
     注意：synthBlockElement 保留在 OpenTypeFace.Glyph 内部（块字符硬边合成，NerdFont subset 不含块字符，合成优先）
  4. `internal/font/cache.go`：
     - 新增接口 `FaceCacheProvider`（统一 *FaceCache 与 *FallbackFaceCache，供 slave 持有接口类型）：
       ```go
       type FaceCacheProvider interface {
           GetFace(size float64) (Face, error)
           Close() error
       }
       ```
     - `*FaceCache` 新增适配方法 `GetFace`（**不改原 Get 签名**，向后兼容，阶段 1 自包含在 font 包内）：
       ```go
       func (c *FaceCache) GetFace(size float64) (Face, error) {
           f, err := c.Get(size)
           if err != nil { return nil, err }
           return f, nil  // *OpenTypeFace → Face 接口
       }
       ```
     - 新增 `FallbackFaceCache`（实现 FaceCacheProvider）：
       ```go
       type FallbackFaceCache struct {
           mu       sync.Mutex
           primary  *opentype.Font
           fallback *opentype.Font  // 可为 nil
           dpi      float64
           faces    map[float64]*FallbackFace
       }
       func NewFallbackFaceCache(primaryData, fallbackData []byte, dpi float64) (*FallbackFaceCache, error)
       func (c *FallbackFaceCache) GetFace(size float64) (Face, error)  // 返回 *FallbackFace
       func (c *FallbackFaceCache) Close() error
       ```
     注意：原 `*FaceCache.Get`（返回 *OpenTypeFace）保持不变，现有调用点（slave/render_harness）阶段 1 不受影响，阶段 3 改用 GetFace
  5. `internal/font/fallback_face_test.go`：测试用例：
     - primary hit → 返回 primary glyph（如 'o'）
     - primary miss + fallback hit → 返回 fallback glyph（如 Powerline U+E0B0）
     - primary miss + fallback miss → nil（如生僻字）
     - baseline 对齐：fallback glyph YOffset = original + (fallbackAscent - primaryAscent)
     - fallback 为 nil 时退化为 primary-only
     - Metrics 返回 primary
- **验证标准**: `go build ./internal/font/...` 通过；`go test ./internal/font/` 通过；FallbackFace 实现 Face 接口（编译期保证）
- **审计结果**: 通过。FallbackFace 缓存 metrics 避免热路径开销，YOffset 对齐公式正确（g2.YOffset += fallbackAscent - primaryAscent），Close 双关闭+错误保留；FallbackFaceCache fallback 创建失败回滚防泄漏；GetFace 适配不改原 Get 向后兼容；10 个测试覆盖 primary/fallback/both-miss/nil/metrics/baseline/cache/接口断言；build/vet/test 全通过；synthBlockElement 保持不动

### 阶段 2: 配置层
- **状态**: 实施中
- **目标**: 暴露 fallback 字体路径配置，支持 init.lua 自定义/禁用
- **实施内容**:
  1. `terminal/options.go`：`Options` 新增 `FallbackFontPath string` 字段；`DefaultOptions` 设默认 `""`
  2. `internal/plugins/config.go`：
     - `RunConfig` 新增 `FallbackFontPath string`
     - `DefaultRunConfig` 默认 `""`
     - `readConfig` 新增 `cfg.FallbackFontPath = getString(pm.L, ct, "fallback_font", cfg.FallbackFontPath)`
  3. `cmd/vistty/main.go`：`opts.FallbackFontPath = runCfg.FallbackFontPath`
  4. init.lua 用法：`vistty.config.fallback_font = "/path/to/font.ttf"`（自定义）或 `""`（禁用）；不设则用内置 NerdFont subset
- **验证标准**: `go build ./...` 通过；`go vet ./...` 通过
- **审计结果**: 通过。FallbackFontPath 字段/默认值/readConfig/传递模式与 FontPath 完全一致；getString 读 "fallback_font"；build/vet 通过

### 阶段 3: session 集成
- **状态**: 实施中
- **目标**: master/slave 加载 fallback 字体数据，InitIndependent 用 FallbackFaceCache，zoom 路径兼容
- **实施内容**:
  1. `session/master.go`：
     - `Master` 新增 `fallbackFontData []byte` 字段
     - `NewMaster` 中读取 `opts.FallbackFontPath`：非空则 `os.ReadFile`；为空则用 `font.EmbeddedFallbackFontData()`
     - 传 `m.fallbackFontData` 给 `InitIndependent`
  2. `session/slave.go`：
     - `Slave.faceCache` 字段类型改为 `font.FaceCacheProvider`（接口）
     - `InitIndependent(fontData, fallbackFontData []byte, fontSize float64)`：fallbackFontData 非 nil 时用 `font.NewFallbackFaceCache(fontData, fallbackFontData, 72)`；nil 时用 `font.NewFaceCache(fontData, 72)`
     - `FaceCache()` 方法返回类型改为 `font.FaceCacheProvider`
  3. `session/render_loop.go`：`handleScaleIndependent` 中 `s.FaceCache().Get(newSize)` 自动适配接口（返回 Face），后续 SetFace 不变
  4. `terminal/render_harness.go`：`RenderHarness.faceCache` 类型改为 `font.FaceCacheProvider`；`NewRenderHarness` 加载 fallback（用 `font.EmbeddedFallbackFontData()` 或 opts）同 slave 模式
- **验证标准**: `go build ./...` 通过；`go test ./...` 无回归（除预先存在的 TestP6ExampleInitLua）；`go vet ./...` 通过
- **审计结果**: 通过。master.go fallbackFontData 加载与 fontData 一致；slave.go 按 fallbackFontData 非空选 NewFallbackFaceCache 否则 NewFaceCache，GetFace 返回 Face；render_loop.go GetFace 替换；render_harness.go 同步；InitIndependent 唯一调用点(master.go:177)已更新；ui.NewOSD 已接受 font.Face；build/vet/test 通过

## 变更记录
| 时间 | 阶段 | 操作 | 备注 |
|------|------|------|------|
| 2026-07-04 | - | 创建实施方案 | 用户确认双字体 fallback + Nerd 图标 1.1MB |
| 2026-07-04 | 阶段1 | 实施完成 | FallbackFace/FallbackFaceCache/embed/10测试，1.05MB subset |
| 2026-07-04 | 阶段1 | 审计通过 | 代码审查+build/vet/test 验证通过 |
| 2026-07-04 | 阶段2 | 实施完成 | options/config/main 加 FallbackFontPath |
| 2026-07-04 | 阶段2 | 审计通过 | 模式与 FontPath 一致，build/vet 通过 |
| 2026-07-04 | 阶段3 | 实施完成 | master/slave/render_loop/render_harness 集成 fallback |
| 2026-07-04 | 阶段3 | 审计通过 | 全部调用点更新，build/vet/test 通过 |
| 2026-07-04 | 整体 | 已完成 | 最终回归通过（仅预先存在 TestP6ExampleInitLua 失败） |
