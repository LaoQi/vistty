#!/usr/bin/env bash
# 重建 font/assets/NerdFontFallback.ttf（PUA 图标 + Dingbats/Greek/Misc Symbols 补充）。
#
# 数据源：
#   NotoMonoNerdFontMono-Regular.ttf — PUA 图标（U+E000-F8FF，含 Powerline U+E0A0-E0D4 + Nerd 图标）
#   DejaVuSansMono.ttf               — Dingbats/Greek/Misc Symbols（补 ✕ π 等主字体缺失字形）
#
# 合并方式：pyftsubset 分别提取两源子集 → fonttools Merger 合并为单文件。
#   PUA + Dingbats(U+2700-27BF) + Greek(U+0370-03FF) + Misc Symbols(U+2600-26FF)
#
# 产出：font/assets/NerdFontFallback.ttf（约 1.17MB，3909 字形）
#
# 用法：./scripts/gen-font-subset.sh
#   可通过环境变量覆盖源字体路径：
#     NERD_FONT_SRC=... SYMBOL_FONT_SRC=... ./scripts/gen-font-subset.sh
set -euo pipefail

cd "$(dirname "$0")/.."

NERD_FONT_SRC="${NERD_FONT_SRC:-/usr/share/fonts/TTF/NotoMonoNerdFontMono-Regular.ttf}"
SYMBOL_FONT_SRC="${SYMBOL_FONT_SRC:-/usr/share/fonts/TTF/DejaVuSansMono.ttf}"
OUT="font/assets/NerdFontFallback.ttf"

TMPDIR=$(mktemp -d -t font-subset-XXXXXX)
trap 'rm -rf "$TMPDIR"' EXIT

# 依赖检查
command -v pyftsubset >/dev/null || { echo "错误：需要 pyftsubset（pip install fonttools）"; exit 1; }
python3 -c "import fontTools.merge" 2>/dev/null || { echo "错误：需要 fontTools（pip install fonttools）"; exit 1; }

echo "==> 检查源字体"
for f in "$NERD_FONT_SRC" "$SYMBOL_FONT_SRC"; do
    [ -f "$f" ] || { echo "错误：源字体不存在 $f"; exit 1; }
done

echo "==> 提取 NerdFont PUA 子集（U+E000-F8FF）"
pyftsubset "$NERD_FONT_SRC" \
    --unicodes=U+E000-F8FF \
    --notdef-outline \
    --output-file="$TMPDIR/pua.ttf"

echo "==> 提取符号子集（Dingbats + Greek + Misc Symbols）"
pyftsubset "$SYMBOL_FONT_SRC" \
    --unicodes=U+2700-27BF,U+0370-03FF,U+2600-26FF \
    --notdef-outline \
    --output-file="$TMPDIR/symbols.ttf"

echo "==> 合并 PUA + 符号 → $OUT"
PUA="$TMPDIR/pua.ttf" SYM="$TMPDIR/symbols.ttf" OUTFILE="$OUT" python3 -c "
import os
from fontTools.merge import Merger
Merger().merge([os.environ['PUA'], os.environ['SYM']]).save(os.environ['OUTFILE'])
"

echo "==> 完成"
ls -la "$OUT"
python3 -c "
from fontTools.ttLib import TTFont
cmap = TTFont('$OUT', lazy=True).getBestCmap()
print(f'字形数: {len(cmap)}')
checks = [(0xE0A0,'Powerline'),(0xE0B0,'Powerline三角'),(0x2715,'✕'),(0x03C0,'π'),(0x2600,'☀')]
for cp,nm in checks:
    print(f'  U+{cp:04X} {nm}: {\"Y\" if cp in cmap else \"N\"}')
"
