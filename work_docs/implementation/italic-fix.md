# 斜体渲染修复 实施方案

## 概述

### 背景
当前 italic 实现存在三条独立路径，问题各异：

| 路径 | 文件 | 方向 | 抗锯齿 | 超出边界 |
|------|------|------|--------|---------|
| CPU 位移（Wayland/DRM dumb） | `render/draw.go:101` | ✗ 反了（`\`） | ✗ 整数位移 | ✗ 丢弃+被覆盖 |
| GPU shader（DRM GBM） | `gpu/shader.go:36` | ✓ 正确（`/`） | ✗ GL_NEAREST | ✗ quad 裁剪 |
| 字体层 | `font/face.go:71` | — | — | 不支持 italic 合成 |

### 方案
**方案 B：font 层预生成斜体字形**（slope=0.25）
- font 包新增 `ShearGlyph` 工具：对正常字形 bitmap 做 shear 变换（顶部向右、双线性插值抗锯齿）
- render 层 `getGlyph(r, italic)` 区分 italic，缓存 key = `(rune, italic)`
- 移除 `blendGlyphItalic` 和 shader italic 分支，CPU/GPU 统一走正常 `BlendGlyph`/atlas 路径

### 预期效果
- 修正 Wayland 倾斜方向（`\` → `/`）
- 三后端斜体效果一致（font 层统一生成）
- 抗锯齿（双线性插值，仅影响 italic 字形，正常字形不受影响）
- 溢出 1-2px 可接受（单 pass 渲染被右 cell 背景覆盖，与 alacritty/kitty 一致）

### 关键约束
- CGO_ENABLED=0
- slope=0.25（顶部偏移 0.25×字形高 ≈ 4px@16pt）
- italic 字形独立 atlas 缓存，ResetAtlas/字号变更需同步清空
- OSD/光标不使用 italic

## 实施阶段

### 阶段 1: font 层 shear 工具
- **状态**: 已审计
- **目标**: 实现 `ShearGlyph` 函数，将正常字形 bitmap 转换为斜体字形 bitmap（顶部向右、双线性插值抗锯齿）
- **实施内容**:
  1. 新建 `internal/font/shear.go`，导出 `ShearGlyph(g *Glyph, slope float64) *Glyph`
     - 算法：顶部向右偏移 `slope*(h-1)`，底部不动；对每个目标像素反查源 `sx = ox - (h-1-oy)*slope`，双线性插值采样
     - 输出：新 bitmap 宽 `w + ceil(slope*(h-1))`，`XOffset = g.XOffset`（底部对齐），`YOffset/Advance` 不变
     - 方向：顶部右移 = `/` 形 = 标准 italic
     - slope 为 0 时直接返回原 glyph（避免无谓拷贝）
  2. 新建 `internal/font/shear_test.go`：
     - 验证方向（顶部右移，底部不动）
     - 验证宽度（`w + ceil(slope*(h-1))`）
     - 验证抗锯齿（亚像素混合产生非 0/255 中间值）
     - 验证 slope=0 返回原 glyph
     - 验证空字形/1px 字形边界情况
- **验证标准**:
  - `go build ./internal/font/` 通过
  - `go test ./internal/font/` 通过
  - ShearGlyph 对 'A' 字形生成顶部右移的斜体字形
- **审计结果**: （初始为空）

### 阶段 2: render + gpu 接线
- **状态**: 已审计
- **目标**: 将 font 层 ShearGlyph 接入渲染管线，移除旧的 italic 位移实现
- **实施内容**:
  1. `internal/render/compositor.go`：
     - 新增 `italicAtlas *font.Atlas` 字段（构造函数和 SetFace 中初始化）
     - `getGlyph(r rune, italic bool)`：italic 时从 italicAtlas 取；miss 则取 normal 字形 → `font.ShearGlyph(g, 0.25)` → 存入 italicAtlas
     - CPU 路径 `:229`：`getGlyph(cell.Rune, cell.Attr&screen.AttrItalic != 0)`
     - GPU 路径 `:418`：同上
     - 光标 `:289`：`getGlyph(cursorRune, false)`
     - `OverlayGlyph`/`OverlayUploadGlyph`：`getGlyph(r, false)`
     - CPU 路径 `:238-242`：移除 `blendGlyphItalic` 分支，统一 `BlendGlyph`（Bold +1 偏移保留）
     - GPU 路径 `:413-415`：移除 `inst.AttrFlags += 4`（italic bit 不再需要）
     - `SetFace` 中重建 `italicAtlas`
  2. `internal/platform/gpu.go`：
     - `UploadGlyph(r rune, bitmap []byte, w, h int, italic bool)` 加 italic 参数
  3. `internal/platform/gpu/renderer.go`：
     - atlasCache key 由 `map[rune]atlasEntry` 改为 `map[glyphKey]atlasEntry`，`glyphKey=struct{Rune rune; Italic bool}`
     - `UploadGlyph` 实现加 italic 参数并作为 key 一部分
  4. `internal/platform/gbm/surface.go:226`：
     - `UploadGlyph` 转发加 italic 参数
  5. `internal/platform/gpu/shader.go`：
     - 删除 `:34-38` 的 `hasItalic`/`glyphCoord.x += ...`
  6. `internal/render/draw.go`：
     - 移除 `blendGlyphItalic` 函数
- **验证标准**:
  - `go build ./...` 通过
  - `go vet ./...` 通过
  - 现有测试无回归（测试签名更新后）
- **审计结果**: （初始为空）

### 阶段 3: 测试更新 + 全量验证 + 清理
- **状态**: 已审计
- **目标**: 更新受 italic 改动影响的测试，全量回归验证
- **实施内容**:（阶段2 subagent 已同步完成测试签名更新，本阶段做最终残留检查与回归）
  1. `internal/render/compositor_test.go` — AttrFlags 期望 7→3，fake UploadGlyph 签名已更新（阶段2完成）
  2. `internal/platform/gbm/*_test.go` — 15 处 UploadGlyph 调用已补 italic（阶段2完成）
  3. `internal/perf/replay/` — 无 GPURenderer 实现，无需改动
  4. `internal/platform/gpu/renderer_test.go` — 仅 shader 字符串检查，无 UploadGlyph 调用，无需改动
- **验证标准**:
  - `go build ./... && go vet ./... && go test ./...` 全部通过 ✓
  - 无残留 `blendGlyphItalic` 引用 ✓
  - 无残留 shader italic 分支（`hasItalic`/`glyphCoord.x +=`）✓
  - 无残留 `AttrFlags += 4` / `bit2=italic` ✓
  - 所有 UploadGlyph 调用点已传 italic 参数 ✓
- **审计结果**: 通过。三后端 italic 现统一走 font 层 ShearGlyph(0.25) 预生成斜体字形，CPU/GPU 路径一致，方向修正为标准 / 形，双线性插值抗锯齿。

## 变更记录
| 时间 | 阶段 | 操作 | 备注 |
|------|------|------|------|
| 2026-07-02 | 文档 | 创建实施方案 | 方案B + slope 0.25 |
| 2026-07-02 | 阶段1 | subagent 实施 shear.go + 测试 | 审计通过 |
| 2026-07-02 | 阶段2 | subagent 实施 render+gpu 接线 | 审计通过（gofmt 修复缩进） |
| 2026-07-02 | 阶段3 | 最终残留检查 + 全量回归 | 审计通过，全部通过 |

## 完成总结

斜体渲染修复已完成。原三条 italic 路径（CPU 整数位移方向反 + GPU shader 位移 + 字体层不支持）统一为：
**font 层 `ShearGlyph(g, 0.25)` 预生成斜体字形 → CPU/GPU 统一走正常 BlendGlyph/atlas 路径**。

- 方向：修正为标准 `/` 形（顶部向右）
- 抗锯齿：双线性插值，仅影响 italic 字形
- 一致性：CPU/Wayland、DRM dumb、DRM GBM 三后端效果一致
- 架构：移除 `blendGlyphItalic` 与 shader italic 分支，italicAtlas 独立缓存
| 2026-07-02 | 阶段1 | subagent 实施完成 | shear.go + shear_test.go |
| 2026-07-02 | 阶段1 | 审计通过 | build/vet/test 全通过，6测试PASS |
