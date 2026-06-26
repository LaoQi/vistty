package platform

// CellInstance 是 GPU instanced draw 的单 cell 渲染数据。
type CellInstance struct {
	X, Y        float32 // cell 左上角像素位置
	GlyphU0, V0 float32 // 字形在 atlas 纹理中的 UV 左上
	GlyphU1, V1 float32 // 字形在 atlas 纹理中的 UV 右下
	GlyphW     float32 // 字形像素宽
	GlyphH     float32 // 字形像素高
	FgR, FgG    float32 // 前景色
	FgB         float32
	BgR, BgG    float32 // 背景色
	BgB         float32
	HasBg       float32 // 1.0=非默认背景, 0.0=默认
	CellW       float32 // cell 宽度像素（1 或 2 倍 metrics.Width）
	AttrFlags   float32 // bit0=underline, bit1=crossedOut
}

// GPURenderer 是 Surface 可选实现的 GPU 渲染接口。
// Compositor 检测 Surface 是否实现此接口，是则走 GPU instanced draw 路径。
type GPURenderer interface {
	// UploadGlyph 将字形 alpha 位图上传到 GPU atlas 纹理，返回 UV 坐标。
	// 若 rune 已在 atlas 中，直接返回缓存 UV。
	UploadGlyph(r rune, bitmap []byte, w, h int) (u0, v0, u1, v1 float32, ok bool)
	// DrawInstances 用 instanced draw 渲染所有 cell。
	DrawInstances(instances []CellInstance, screenW, screenH int, bgColor [3]float32) error
}
