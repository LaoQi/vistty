# 字体显示测试文档

用于验证 vistty 字体渲染。覆盖三类字形来源：主字体、fallback 字体、合成字形。

## 字体架构

| 来源 | 字体 | 体积 | 覆盖职责 |
|------|------|------|----------|
| 主字体 | Sarasa Fixed SC subset | 6.7MB | CJK（20992字全）、Box Drawing、箭头、数学、几何、CJK 标点、半宽全角 |
| Fallback | NerdFont PUA subset | 1.05MB | Powerline（U+E0A0-E0D4）+ Nerd Font 图标（PUA 3500个） |
| 合成 | synthBlockElement | - | Block Elements（U+2580-259F，硬边几何） |

查找顺序：`primary.Glyph(r)` → miss → `fallback.Glyph(r)` → miss → `synthBlockElement(r)`（仅块字符）→ nil

## 覆盖概览（cmap 实测）

| Unicode 块 | 范围 | 来源 | 覆盖 |
|------------|------|------|------|
| Basic Latin | U+0020-007E | Sarasa | 全 |
| Latin-1 Supplement | U+00A0-00FF | Sarasa | 全 |
| General Punctuation | U+2000-206F | Sarasa | 84/112 |
| Arrows | U+2190-21FF | Sarasa | 全 |
| Math Operators | U+2200-22FF | Sarasa | 全 |
| Misc Technical | U+2300-23FF | Sarasa | 217/256 |
| Box Drawing | U+2500-257F | Sarasa | 全 |
| Block Elements | U+2580-259F | 合成 | 全（32/32） |
| Geometric Shapes | U+25A0-25FF | Sarasa | 全 |
| Misc Symbols | U+2600-26FF | Sarasa | 108/256（稀疏） |
| CJK Sym & Punct | U+3000-303F | Sarasa | 全 |
| CJK Unified | U+4E00-9FFF | Sarasa | 全（20992） |
| Half/Fullwidth | U+FF00-FFEF | Sarasa | 225/240 |
| Powerline | U+E0A0-E0D4 | NerdFont | 全（38） |
| Nerd Font PUA | U+E000-F8FF | NerdFont | 3500 |

## 测试字符表

### 1. 基本拉丁与拉丁补充（Sarasa 原生）
```
ABCdef1230 !@#$%^&*()_+-={}[]|\:;"'<>,.?/~`
ÀÁÂÃÄÅ Æ Ç ÈÉÊË ÌÍÎÏ Ñ ÒÓÔÕÖ Ø ÙÚÛÜ ß àáâãäå
```
**验证**：字形清晰、等宽对齐、无缺字。

### 2. CJK 统一汉字（Sarasa 原生）
```
终端模拟器字体显示测试
中文拼音输入法 转义序列解析
禅 镜 龘 靐 靇 𪚥（末字 CJK Ext B 可能缺失）
```
**验证**：双宽对齐、字形完整。`𪚥`（U+2A6A5，CJK Ext B）预期缺失（子集不含 Ext B）。

### 3. CJK 标点（Sarasa 原生）
```
「」『』【】（）《》〈〉〔〕〖〗〘〙〚〛
、。·●○◆◇■□★☆※→←↑↓
```
**验证**：全角标点双宽、居中对齐。

### 4. Box Drawing（Sarasa 原生）
```
┌──────────────────────────────┐
│  Box Drawing 边框测试         │
├──────────────┬───────────────┤
│  左上 ╔═╗    │  右下 ╚═╝     │
│  竖 │  竖    │  交叉 ┼       │
└──────────────┴───────────────┘
```
**验证**：线条连续无断裂、转角衔接、双宽 CJK 与单宽边框对齐。

### 5. Block Elements（合成 fallback）
```
█▀▀▀█ █▀▀▀█ █▀▀▀█
█   █ █   █ █   █
█▄▄▄█ █▄▄▄█ █▄▄▄█

完整块: █   上半: ▀   下半: ▄
左半: ▌   右半: ▐
渐变: ▏▎▍▌▋▊▉█
阴影: ░▒▓█
```
**验证**：块字符填满 cell、硬边无抗锯齿、▀▄ 上下半精确占 cell 一半。渐变 8 级宽度递增。

### 6. 几何形状（Sarasa 原生）
```
●○◆◇■□▲△▼▽★☆►◄▲▼
◉◎◈◇◆■□▲△▼▽◐◑◒◓
```
**验证**：单宽居中、无 tofu（豆腐框）。

### 7. 箭头与数学运算符（Sarasa 原生）
```
→ ← ↑ ↓ ↔ ↕ ⇒ ⇐ ⇑ ⇓ ⇔
≤ ≥ ≠ ≈ ± × ÷ ∞ √ ∑ ∏ ∫ ∂ ∇
∈ ∉ ⊂ ⊃ ∪ ∩ ∀ ∃ ∅ ∴ ∵
```
**验证**：箭头方向正确、数学符号清晰。

### 8. Powerline 字形（NerdFont fallback）
```
分支:  
左分隔:  
右分隔:  
左三角:  
右三角:  
渐变: 
```
**验证**：三角形/斜切分隔符正常显示（非 tofu）。若显示为空格或 tofu，说明 fallback 未生效。

### 9. Powerline 状态栏示例
```
\033[48;2;92;156;245m \033[48;2;40;40;40m main \033[48;2;10;10;10m\033[38;2;40;40;40m\033[0m\033[38;2;92;156;245m\033[48;2;10;10;10m src/\033[0m
```
渲染效果（应显示蓝底分支 + 灰底路径 + 三角过渡）：
```
  main  src/  
```
**验证**：三角过渡平滑、背景色衔接、无空隙。

### 10. Nerd Font 图标（NerdFont fallback）
NerdFont PUA 图标码位不固定（3500个），建议用脚本扫描实机存在的图标：
```bash
python3 -c "
for cp in range(0xE000, 0xF8FF+1):
    print(chr(cp), end='')
" | head -c 2000
```
或在 vistty 内运行：
```bash
printf '%b' "$(python3 -c "print(''.join(chr(c) for c in range(0xE000,0xF900)))")"
```
**验证**：大部分 PUA 字符应显示为图标（非 tofu）。少量 NerdFont 自身未含的码位会缺失（正常）。

### 11. 半宽全角形式（Sarasa 原生）
```
０１２３４５６７８９
ＡＢＣＤＥａｂｃｄｅ
！＃＄％＆
```
**验证**：全角字符双宽、与 CJK 等宽对齐。

### 12. Bold / Italic / Underline 属性
```
\033[1mBold 粗体\033[0m  \033[3mItalic 斜体\033[0m  \033[4mUnderline 下划线\033[0m
\033[1m█▀▄\033[0m  \033[3m█▀▄\033[0m  \033[4m█▀▄\033[0m
```
**验证**：bold 块字符边缘无错位、italic 块字符倾斜合理（合成字形经 ShearGlyph）、underline 不与块字符重叠。

### 13. Truecolor 背景与块字符组合
```
\033[38;2;238;238;238m\033[48;2;10;10;10m█▀▀█ █▀▀█ █▀▀█ █▀▀▄\033[0m
```
**验证**：truecolor fg/bg 正确应用到块字符（块字符 fg 着色、bg 透出）。

## 快速验证脚本

在 vistty 终端内运行（需 shell）：
```bash
cat << 'TESTEOF'
=== 1. ASCII ===
ABCdef1230 !@#$%^&*()

=== 2. CJK ===
终端模拟器字体测试  中文拼音

=== 3. Box Drawing ===
┌────────┬────────┐
│  左    │  右    │
└────────┴────────┘

=== 4. Block Elements (合成) ===
█▀▀▀█
█   █
█▄▄▄█
渐变: ▏▎▍▌▋▊▉█  阴影: ░▒▓█

=== 5. Powerline (fallback) ===
分支:   右三角:   左三角:  

=== 6. 几何/箭头 ===
●○◆◇■□  →←↑↓ ⇒⇐

=== 7. 数学 ===
≤ ≥ ≠ ± × ÷ ∞ √ ∑

=== 8. 半角全角 ===
ＡＢＣ１２３
TESTEOF
```

## 验证要点

1. **无 tofu（豆腐框 □）**：所有列出的字符应显示实际字形，缺失字符会显示为空（compositor 跳过）或默认框。
2. **等宽对齐**：CJK 双宽字符与两个 ASCII 宽度一致；Box Drawing 边框与 CJK 文字对齐。
3. **块字符硬边**：█▀▄ 应为像素精确硬边（合成），无抗锯齿模糊。
4. **Powerline 三角**： 显示为三角形/斜切，非空白。
5. **bold 块字符**：bold 属性下 █ 不应右溢出或左留白。
6. **italic 块字符**：斜体 █ 倾斜后仍填满 cell 区域。
7. **truecolor**：SGR 48;2;r;g;b 背景正确填充块字符 cell。

## 已知缺失（预期，非 bug）

| 字符类 | 范围 | 原因 | 影响 |
|--------|------|------|------|
| CJK Ext A/B | U+3400-4DBF, U+20000+ | 子集不含罕见字 | 生僻字不显示 |
| 日文假名 | U+3040-30FF | Sarasa SC 是简中版 | 日文不显示 |
| 韩文 | U+AC00-D7AF | 子集不含 | 韩文不显示 |
| Braille | U+2800-28FF | 子集不含 | 盲文不显示 |
| 部分Misc Symbols | U+2600-26FF 稀疏 | 子集部分裁剪 | 部分符号缺失 |

## 排查指南

- **Powerline 显示为空格**：检查 fallback 是否加载（`fallback_font` 配置或内置 NerdFont）；确认字符码位在 E0A0-E0D4。
- **块字符不显示**：检查 synthBlockElement（face.go）是否生效；块字符应总由合成覆盖。
- **CJK 显示为 tofu**：主字体加载失败；检查 FontPath 或内置 Sarasa。
- **bold 块字符错位**：compositor bold 偏移 +1px 与块字符 cell 宽度的交互。
- **Nerd 图标部分缺失**：NerdFont subset 只含 NerdFont 实际存在的 3500 个 PUA 码位，非全部 6400 个 PUA 码位都有字形。
