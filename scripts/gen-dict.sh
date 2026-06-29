#!/usr/bin/env bash
# 重建 ime/pinyin/data/dict.bin 词库（rime-ice 全量数据）。
#
# 数据源（iDvel/rime-ice，GPLv3）：
#   8105.dict.yaml    — 《通用规范汉字表》单字全量
#   base.dict.yaml    — 基础词库（含两字常用词组）
#   ext.dict.yaml     — 扩展词库（三字及以上）
#   tencent.dict.yaml — 腾讯词向量（三字及以上，无注音，用 8105 注音）
#
# 用法：./scripts/gen-dict.sh
set -euo pipefail

cd "$(dirname "$0")/.."

TMPDIR=$(mktemp -d -t rime-ice-XXXXXX)
trap 'rm -rf "$TMPDIR"' EXIT

echo "==> 克隆 rime-ice（depth=1）到 $TMPDIR"
git clone --depth 1 https://github.com/iDvel/rime-ice "$TMPDIR/rime-ice"

DICTS="$TMPDIR/rime-ice/cn_dicts"
OUT="ime/pinyin/data/dict.bin"
OUT_GZ="ime/pinyin/data/dict.bin.gz"

echo "==> 生成 $OUT"
go run ./cmd/gen-dict \
    -o "$OUT" \
    -annotate "$DICTS/8105.dict.yaml" \
    "$DICTS/8105.dict.yaml" \
    "$DICTS/base.dict.yaml" \
    "$DICTS/ext.dict.yaml" \
    "$DICTS/tencent.dict.yaml"

echo "==> gzip 压缩 → $OUT_GZ"
gzip -9 -c "$OUT" > "$OUT_GZ"
rm "$OUT"

echo "==> 完成"
ls -la "$OUT_GZ"
