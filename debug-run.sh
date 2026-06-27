#!/usr/bin/env bash
# Vistty 调试执行脚本
# 默认：开启 VISTTY_DEBUG，日志仅输出到带时间戳的文件（无 stderr）
#
# 用法：
#   ./debug-run.sh [debug|perf] [--log-dir DIR] -- [vistty 参数]...
#
# 模式：
#   debug   普通调试（默认），go run 快速启动，仅记录调试日志
#   perf    性能分析，go build 编译后运行，额外生成 cpu/mem/trace profile
#
# 选项：
#   --log-dir DIR   日志目录（默认 /tmp）
#   -h, --help      显示此帮助
#
# 示例：
#   ./debug-run.sh -- -backend wayland
#   ./debug-run.sh perf -- -backend drm -fps
#   ./debug-run.sh debug --log-dir ~/logs -- -backend drm -tty 2
set -euo pipefail

MODE="debug"
LOG_DIR="/tmp"
VISTTY_ARGS=()

while [[ $# -gt 0 ]]; do
    case "$1" in
        debug|perf)    MODE="$1"; shift ;;
        --log-dir)     LOG_DIR="${2:?--log-dir 需要参数}"; shift 2 ;;
        -h|--help)     sed -n '2,/^set -euo/{/^set -euo/d;p}' "$0" | sed 's/^# \?//'; exit 0 ;;
        --)            shift; VISTTY_ARGS=("$@"); break ;;
        *)             VISTTY_ARGS=("$@"); break ;;
    esac
done

cd "$(dirname "$0")"

TS="$(date +%Y%m%d-%H%M%S)"
mkdir -p "$LOG_DIR"
LOG_FILE="${LOG_DIR%/}/vistty-${MODE}-${TS}.log"

export VISTTY_DEBUG=1
export VISTTY_DEBUG_FILE="$LOG_FILE"
export VISTTY_DEBUG_STDERR=0

EXTRA_ARGS=()
if [[ "$MODE" == "perf" ]]; then
    EXTRA_ARGS+=(
        -cpuprofile "${LOG_DIR%/}/vistty-cpu-${TS}.prof"
        -memprofile "${LOG_DIR%/}/vistty-mem-${TS}.prof"
        -trace      "${LOG_DIR%/}/vistty-trace-${TS}.out"
    )
fi

echo "==> mode: $MODE"
echo "==> log:  $LOG_FILE"
[[ "$MODE" == "perf" ]] && echo "==> prof: ${LOG_DIR%/}/vistty-{cpu,mem,trace}-${TS}.*"
echo "==> args: ${VISTTY_ARGS[*]:-（无）}"
echo

if [[ "$MODE" == "perf" ]]; then
    BIN="${LOG_DIR%/}/vistty-${TS}"
    go build -o "$BIN" ./cmd/vistty
    exec "$BIN" "${EXTRA_ARGS[@]}" "${VISTTY_ARGS[@]}"
else
    exec go run ./cmd/vistty "${EXTRA_ARGS[@]}" "${VISTTY_ARGS[@]}"
fi
