package gpu

// GPU instanced draw shaders (GLES 3.00)。vertex 计算 cell 像素位置 + 字形
// 子区域坐标;fragment 用 atlas alpha 混合前景/背景，并绘制 underline/crossedOut。
const gpuVertexSrc = `#version 300 es
layout(location=0) in vec2 a_quadPos;   // 0..1 unit quad
layout(location=1) in vec2 a_quadTex;   // 0..1 unit texcoord
layout(location=2) in vec2 i_cellPos;   // cell pixel position
layout(location=3) in vec2 i_cellSize;  // cellW, cellH (quad size)
layout(location=4) in vec2 i_glyphOff;  // glyph offset in cell
layout(location=5) in vec2 i_glyphSize; // glyph draw size
layout(location=6) in vec4 i_glyphUV;   // atlas UV (u0,v0,u1,v1)
layout(location=7) in vec3 i_fg;
layout(location=8) in vec3 i_bg;
layout(location=9) in float i_hasBg;
layout(location=10) in float i_attrFlags;
uniform vec2 u_resolution;
out vec2 v_tex;
out vec2 v_cellUV;
out vec3 v_fg;
out vec3 v_bg;
out float v_hasBg;
out float v_attrFlags;
out vec2 v_glyphCoord;
out float v_hasGlyph;
out vec2 v_cellSize;
void main() {
    vec2 cellPixelPos = a_quadPos * i_cellSize + i_cellPos;
    vec2 ndc = cellPixelPos / u_resolution * 2.0 - 1.0;
    ndc.y = -ndc.y;
    gl_Position = vec4(ndc, 0.0, 1.0);
    // 字形在 cell 内的子区域坐标 (0..1 if in glyph, else out of range)
    vec2 glyphCoord = (a_quadPos * i_cellSize - i_glyphOff) / i_glyphSize;
    v_tex = mix(i_glyphUV.xy, i_glyphUV.zw, glyphCoord);
    v_cellUV = a_quadPos;
    v_glyphCoord = glyphCoord;
    v_cellSize = i_cellSize;
    v_fg = i_fg;
    v_bg = i_bg;
    v_hasBg = i_hasBg;
    v_attrFlags = i_attrFlags;
    // UV 退化 (u0>=u1 或 v0>=v1) 表示无字形（空字符 UV=0），避免采样 atlas (0,0) 残留
    v_hasGlyph = sign(max(i_glyphUV.z - i_glyphUV.x, 0.0)) * sign(max(i_glyphUV.w - i_glyphUV.y, 0.0));
}
`

const gpuFragmentSrc = `#version 300 es
precision mediump float;
in vec2 v_tex;
in vec2 v_cellUV;
in vec3 v_fg;
in vec3 v_bg;
in float v_hasBg;
in float v_attrFlags;
in vec2 v_glyphCoord;
in float v_hasGlyph;
in vec2 v_cellSize;
uniform sampler2D u_atlas;
uniform vec3 u_defBg;
out vec4 fragColor;
void main() {
    float alpha = 0.0;
    float inGlyph = step(0.0, v_glyphCoord.x) * step(v_glyphCoord.x, 1.0)
                   * step(0.0, v_glyphCoord.y) * step(v_glyphCoord.y, 1.0);
    if (inGlyph > 0.5 && v_hasGlyph > 0.5) {
        alpha = texture(u_atlas, v_tex).r;
    }
    vec3 bg = mix(u_defBg, v_bg, v_hasBg);
    vec3 color = mix(bg, v_fg, alpha);
    // underline (bit 0)：cell 底部精确 1px
    float hasUL = mod(floor(v_attrFlags), 2.0);
    float ulThreshold = 1.0 - 1.0 / v_cellSize.y;
    if (hasUL > 0.5 && v_cellUV.y > ulThreshold) {
        color = v_fg;
        alpha = 1.0;
    }
    // crossed out (bit 1)：cell 中线精确 1px
    float hasCO = floor(v_attrFlags / 2.0);
    float coHalf = 0.5 / v_cellSize.y;
    if (hasCO > 0.5 && v_cellUV.y > 0.5 - coHalf && v_cellUV.y < 0.5 + coHalf) {
        color = v_fg;
        alpha = 1.0;
    }
    fragColor = vec4(color, 1.0);
}
`
