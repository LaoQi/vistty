package drm

import (
	"regexp"
	"strings"
	"testing"
	"unsafe"

	"github.com/LaoQi/vistty/internal/platform"
)

// ============================================================
// L1: packGlyph 纯函数测试（shelf packing + UV 计算）
// 定位子因 5（UV 半纹素 inset 把字形采样挤没）与 shelf 推进逻辑。
// ============================================================

func TestPackGlyphFirstPlacement(t *testing.T) {
	const aw, ah = 2048, 2048
	px, py, nsx, nsy, nsh, u0, v0, u1, v1, reset, ok := packGlyph(0, 0, 0, aw, ah, 7, 16)
	if !ok {
		t.Fatal("expected ok for first placement")
	}
	if reset {
		t.Error("first placement should not reset")
	}
	if px != 0 || py != 0 {
		t.Errorf("first placement at (0,0), got (%d,%d)", px, py)
	}
	if nsx != 7 || nsy != 0 || nsh != 16 {
		t.Errorf("shelf state (7,0,16), got (%d,%d,%d)", nsx, nsy, nsh)
	}
	// 0.5 纹素 inset
	if u0 != 0.5/aw || v0 != 0.5/ah {
		t.Errorf("u0/v0 inset wrong: u0=%v v0=%v", u0, v0)
	}
	if u1 != 6.5/aw || v1 != 15.5/ah {
		t.Errorf("u1/v1 inset wrong: u1=%v v1=%v", u1, v1)
	}
}

func TestPackGlyphHorizontalWrap(t *testing.T) {
	const aw, ah = 16, 64
	// 第一个 7×16 放 (0,0), shelfX→7
	px, _, nsx, _, nsh, _, _, _, _, _, ok := packGlyph(0, 0, 0, aw, ah, 7, 16)
	if !ok || px != 0 || nsx != 7 || nsh != 16 {
		t.Fatalf("first: px=%d nsx=%d nsh=%d ok=%v", px, nsx, nsh, ok)
	}
	// 第二个 shelfX=7, 7+7=14<=16 → 放 (7,0), shelfX→14
	px, _, nsx, _, _, _, _, _, _, _, ok = packGlyph(7, 0, 16, aw, ah, 7, 16)
	if !ok || px != 7 || nsx != 14 {
		t.Fatalf("second: px=%d nsx=%d ok=%v", px, nsx, ok)
	}
	// 第三个 shelfX=14, 14+7=21>16 → 换行: px=0, py=0+16=16
	px, py, nsx, nsy, _, _, _, _, _, _, ok := packGlyph(14, 0, 16, aw, ah, 7, 16)
	if !ok || px != 0 || py != 16 || nsx != 7 || nsy != 16 {
		t.Fatalf("wrap: px=%d py=%d nsx=%d nsy=%d ok=%v", px, py, nsx, nsy, ok)
	}
}

func TestPackGlyphResetWhenFull(t *testing.T) {
	const aw, ah = 8, 8
	// 4×4 字形，atlas 8×8 可放 4 个（2行2列），第 5 个触发 reset
	state := [3]int{0, 0, 0} // shelfX, shelfY, shelfH
	for i := 0; i < 4; i++ {
		px, py, nsx, nsy, nsh, _, _, _, _, reset, ok := packGlyph(state[0], state[1], state[2], aw, ah, 4, 4)
		if !ok || reset {
			t.Fatalf("placement %d: ok=%v reset=%v", i, ok, reset)
		}
		state[0], state[1], state[2] = nsx, nsy, nsh
		_ = px
		_ = py
	}
	// 第 5 个：shelf 已满 → reset
	px, py, _, _, _, _, _, _, _, reset, ok := packGlyph(state[0], state[1], state[2], aw, ah, 4, 4)
	if !ok {
		t.Fatal("expected ok even when reset (w/h fit)")
	}
	if !reset {
		t.Fatal("expected reset when atlas full")
	}
	if px != 0 || py != 0 {
		t.Errorf("after reset placement at (0,0), got (%d,%d)", px, py)
	}
}

func TestPackGlyphInvalidSize(t *testing.T) {
	cases := []struct{ w, h int }{
		{0, 16}, {7, 0}, {-1, 16}, {7, -1},
		{2049, 16}, // w > atlasW
		{7, 2049},  // h > atlasH
	}
	for _, c := range cases {
		_, _, _, _, _, _, _, _, _, _, ok := packGlyph(0, 0, 0, 2048, 2048, c.w, c.h)
		if ok {
			t.Errorf("w=%d h=%d should fail", c.w, c.h)
		}
	}
}

func TestPackGlyphWidthEqualsAtlas(t *testing.T) {
	// 边界：w == atlasW 应能放置（不换行）
	px, _, _, _, _, u0, _, u1, _, _, ok := packGlyph(0, 0, 0, 8, 8, 8, 4)
	if !ok || px != 0 {
		t.Fatalf("w==atlasW: px=%d ok=%v", px, ok)
	}
	if u0 != 0.5/8 || u1 != 7.5/8 {
		t.Errorf("inset wrong: u0=%v u1=%v", u0, u1)
	}
}

func TestPackGlyphShelfHeightTracksMax(t *testing.T) {
	// 先放 7×16（shelfH=16），再放 7×20（同行，shelfH→20）
	_, _, nsx, _, nsh, _, _, _, _, _, ok := packGlyph(0, 0, 0, 64, 64, 7, 16)
	if !ok || nsh != 16 {
		t.Fatalf("first: nsh=%d", nsh)
	}
	_, _, _, _, nsh, _, _, _, _, _, ok = packGlyph(nsx, 0, 16, 64, 64, 7, 20)
	if !ok || nsh != 20 {
		t.Errorf("second: nsh=%d (want 20)", nsh)
	}
}

func TestPackGlyphUVInRangeAndOrdered(t *testing.T) {
	_, _, _, _, _, u0, v0, u1, v1, _, ok := packGlyph(100, 200, 0, 2048, 2048, 7, 16)
	if !ok {
		t.Fatal("expected ok")
	}
	if !(u0 >= 0 && u0 <= 1 && u1 >= 0 && u1 <= 1 && v0 >= 0 && v0 <= 1 && v1 >= 0 && v1 <= 1) {
		t.Errorf("UV out of [0,1]: u0=%v v0=%v u1=%v v1=%v", u0, v0, u1, v1)
	}
	if !(u0 < u1 && v0 < v1) {
		t.Errorf("UV not ordered: u0=%v u1=%v v0=%v v1=%v", u0, u1, v0, v1)
	}
}

// ============================================================
// L3: shader 契约静态校验 + CellInstance 内存布局契约
// 定位子因 3（v_inGlyph 除零）与故障点 5（attribute offset 错位）。
// ============================================================

func TestShaderVertexLocationSet(t *testing.T) {
	// 解析 gpuVertexSrc 中所有 layout(location=N) in 变量
	re := regexp.MustCompile(`layout\s*\(\s*location\s*=\s*(\d+)\s*\)\s+in\s+\w+\s+(\w+)\s*;`)
	matches := re.FindAllStringSubmatch(gpuVertexSrc, -1)
	if len(matches) == 0 {
		t.Fatal("no layout(location=) declarations found in vertex shader")
	}
	seen := map[int]string{}
	for _, m := range matches {
		var loc int
		for _, ch := range m[1] {
			loc = loc*10 + int(ch-'0')
		}
		seen[loc] = m[2]
	}
	// 期望 location 0..10 全部存在
	for i := 0; i <= 10; i++ {
		if _, ok := seen[i]; !ok {
			t.Errorf("missing layout(location=%d)", i)
		}
	}
	// 不应有多余 location
	for loc := range seen {
		if loc < 0 || loc > 10 {
			t.Errorf("unexpected location=%d (%s)", loc, seen[loc])
		}
	}
}

func TestShaderVertexInstanceAttrNames(t *testing.T) {
	// 关键 instance attribute 的变量名校验（与 DrawInstances 注释对应）
	expect := map[int]string{
		2:  "i_cellPos",
		3:  "i_cellSize",
		4:  "i_glyphOff",
		5:  "i_glyphSize",
		6:  "i_glyphUV",
		7:  "i_fg",
		8:  "i_bg",
		9:  "i_hasBg",
		10: "i_attrFlags",
	}
	re := regexp.MustCompile(`layout\s*\(\s*location\s*=\s*(\d+)\s*\)\s+in\s+\w+\s+(\w+)\s*;`)
	for _, m := range re.FindAllStringSubmatch(gpuVertexSrc, -1) {
		var loc int
		for _, ch := range m[1] {
			loc = loc*10 + int(ch-'0')
		}
		if want, ok := expect[loc]; ok && m[2] != want {
			t.Errorf("location=%d name=%q want %q", loc, m[2], want)
		}
	}
}

func TestShaderFragmentSamplesRedChannel(t *testing.T) {
	// fragment 必须采样 atlas 的 R 通道（UploadGlyph 把 alpha 放 R 通道）
	if !strings.Contains(gpuFragmentSrc, "texture(u_atlas") {
		t.Error("fragment does not sample u_atlas")
	}
	if !strings.Contains(gpuFragmentSrc, ".r") {
		t.Error("fragment does not sample .r channel — mismatch with UploadGlyph (alpha in R)")
	}
}

func TestShaderVInGlyphDividesByGlyphSize(t *testing.T) {
	// v_inGlyph 计算 glyphCoord = (... - i_glyphOff) / i_glyphSize
	// i_glyphSize==0 时除零 → 需由 compositor 保证 GlyphW/H>0（见 L2 测试）
	if !strings.Contains(gpuVertexSrc, "/ i_glyphSize") {
		t.Error("vertex shader v_inGlyph does not divide by i_glyphSize (除零风险点未找到，shader 可能已变更)")
	}
}

// TestCellInstanceLayoutContract 断言 CellInstance 字段 offset 与
// DrawInstances 中 VertexAttribPointer 的硬编码 offset 严格一致。
// 若有人改了 CellInstance 字段顺序而忘改 DrawInstances，此测试会失败。
func TestCellInstanceLayoutContract(t *testing.T) {
	var ci platform.CellInstance

	// DrawInstances 中 instance attribute 的硬编码配置（index, size, offset）
	type attrCfg struct {
		index     uint32
		size      int32
		codeOff   uintptr // DrawInstances VertexAttribPointer 第6参数
		fieldOff  uintptr // unsafe.Offsetof 实际字段
		fieldName string
	}
	quadStride := int32(unsafe.Sizeof(ci))
	if quadStride != 80 {
		t.Errorf("CellInstance size=%d, want 80 (DrawInstances stride 依赖此值)", quadStride)
	}

	cfgs := []attrCfg{
		{2, 2, 0, unsafe.Offsetof(ci.X), "X"},
		{3, 2, 8, unsafe.Offsetof(ci.CellW), "CellW"},
		{4, 2, 16, unsafe.Offsetof(ci.GlyphOffX), "GlyphOffX"},
		{5, 2, 24, unsafe.Offsetof(ci.GlyphW), "GlyphW"},
		{6, 4, 32, unsafe.Offsetof(ci.GlyphU0), "GlyphU0"},
		{7, 3, 48, unsafe.Offsetof(ci.FgR), "FgR"},
		{8, 3, 60, unsafe.Offsetof(ci.BgR), "BgR"},
		{9, 1, 72, unsafe.Offsetof(ci.HasBg), "HasBg"},
		{10, 1, 76, unsafe.Offsetof(ci.AttrFlags), "AttrFlags"},
	}
	for _, c := range cfgs {
		if c.codeOff != c.fieldOff {
			t.Errorf("attr %d (%s): DrawInstances offset=%d but field offset=%d — 布局错位将导致采样错误数据",
				c.index, c.fieldName, c.codeOff, c.fieldOff)
		}
		// size 对应的连续字段应落在同一 attribute 范围内（offset + size*4 <= stride）
		end := c.fieldOff + uintptr(c.size)*4
		if end > uintptr(quadStride) {
			t.Errorf("attr %d (%s): offset+%d*4=%d exceeds stride %d", c.index, c.fieldName, c.size, end, quadStride)
		}
	}
}
