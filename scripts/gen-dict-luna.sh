#!/usr/bin/env bash
# 重建 pinyin/data/dict.bin 词库（rime-luna-pinyin 官方明月拼音）。
#
# 数据源（rime/rime-luna-pinyin，GPLv3）：
#   luna_pinyin.dict.yaml — 明月拼音主词典（单字 + 词组，自带注音，约 7 万条）
#
# 权重策略：-order-weight 按文件收录顺序倒序赋权（后出现权重高）。
#
# 用法：./scripts/gen-dict-luna.sh
set -euo pipefail

cd "$(dirname "$0")/.."

TMPDIR=$(mktemp -d -t luna-pinyin-XXXXXX)
trap 'rm -rf "$TMPDIR"' EXIT

echo "==> 克隆 rime-luna-pinyin（depth=1）到 $TMPDIR"
git clone --depth 1 https://github.com/rime/rime-luna-pinyin "$TMPDIR/rime-luna-pinyin"

DICT="$TMPDIR/rime-luna-pinyin"
OUT="pinyin/data/dict.bin"
OUT_GZ="pinyin/data/dict.bin.gz"

echo "==> 生成 $OUT"
go run ./cmd/gen-dict \
    -o "$OUT" \
    -order-weight \
    "$DICT/luna_pinyin.dict.yaml"

echo "==> gzip 压缩 → $OUT_GZ"
gzip -9 -c "$OUT" > "$OUT_GZ"
rm "$OUT"

echo "==> 完成"
ls -la "$OUT_GZ"
