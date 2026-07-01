#!/usr/bin/env bash
# Vistty GBM 实机测试 — 屏幕显示检测辅助脚本
#
# 在 vistty 运行期间，从另一个 TTY/SSH 会话执行此脚本，
# 检测 vistty 进程状态、帧率、DRM 输出状态等。
#
# 用法：
#   ./scripts/gbm-check.sh <vistty-pid> [输出目录]
#
# 检测项：
#   1. 进程存活 + 状态（是否卡在 D 状态）
#   2. FPS 日志是否持续更新（帧在推进）
#   3. DRM CRTC 是否 active
#   4. /proc/pid 状态（线程数、内存、fd 数）
#   5. 是否有 flip 超时
#
set -euo pipefail

PID="${1:?用法: $0 <vistty-pid> [输出目录]}"
OUT_DIR="${2:-/tmp/vistty-bench-check}"

mkdir -p "$OUT_DIR"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m'

pass() { echo -e "${GREEN}[PASS]${NC} $1"; }
fail() { echo -e "${RED}[FAIL]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
info() { echo -e "       $1"; }

echo "=== Vistty GBM 屏幕显示检测 ==="
echo "PID: ${PID}"
echo "时间: $(date '+%Y-%m-%d %H:%M:%S')"
echo

# 1. 进程存活
echo "--- 1. 进程状态 ---"
if kill -0 "$PID" 2>/dev/null; then
    pass "进程存活"
else
    fail "进程不存在（PID ${PID}）"
    exit 1
fi

if [[ -f "/proc/${PID}/stat" ]]; then
    STAT=$(cat "/proc/${PID}/stat" 2>/dev/null)
    STATE=$(echo "$STAT" | awk '{print $3}')
    case "$STATE" in
        S) pass "进程状态: S (sleeping)" ;;
        R) pass "进程状态: R (running)" ;;
        D) fail "进程状态: D (uninterruptible sleep — 可能卡死在内核调用)" ;;
        Z) fail "进程状态: Z (zombie)" ;;
        T) warn "进程状态: T (stopped)" ;;
        *) warn "进程状态: ${STATE} (未知)" ;;
    esac

    THREADS=$(echo "$STAT" | awk '{print $20}')
    RSS=$(echo "$STAT" | awk '{print $24}')
    RSS_MB=$((RSS * 4 / 1024))
    info "线程数: ${THREADS}, RSS: ${RSS_MB}MB"
fi

# 检查所有线程状态
D_COUNT=0
if [[ -d "/proc/${PID}/task" ]]; then
    for tid_dir in /proc/${PID}/task/*/; do
        tid=$(basename "$tid_dir")
        tstate=$(awk '{print $3}' "${tid_dir}/stat" 2>/dev/null || echo "?")
        if [[ "$tstate" == "D" ]]; then
            D_COUNT=$((D_COUNT + 1))
            tname=$(awk '{print $2}' "${tid_dir}/stat" 2>/dev/null | tr -d '()')
            warn "线程 ${tid} (${tname}) 处于 D 状态"
        fi
    done
fi
if [[ $D_COUNT -eq 0 ]]; then
    pass "无线程处于 D 状态"
else
    fail "${D_COUNT} 个线程处于 D 状态（可能卡死）"
fi
echo

# 2. FPS 日志更新检测
echo "--- 2. 帧推进检测 ---"
FPS_LOG="${OUT_DIR}/../fps.log"
# 尝试多个可能的位置
for candidate in "${OUT_DIR}/fps.log" "${OUT_DIR}/../fps.log" "/tmp/vistty-bench-"*/fps.log; do
    if [[ -f "$candidate" ]]; then
        FPS_LOG="$candidate"
        break
    fi
done

if [[ -f "$FPS_LOG" ]]; then
    LINES_BEFORE=$(wc -l < "$FPS_LOG")
    sleep 3
    LINES_AFTER=$(wc -l < "$FPS_LOG")
    NEW_LINES=$((LINES_AFTER - LINES_BEFORE))
    if [[ $NEW_LINES -gt 0 ]]; then
        pass "帧在推进（3秒内新增 ${NEW_LINES} 帧）"
        # 最近 10 帧的平均帧时间
        RECENT=$(tail -10 "$FPS_LOG" | grep "^frame:" | sed 's/frame: //' | sed 's/ms$//')
        if [[ -n "$RECENT" ]]; then
            AVG=$(echo "$RECENT" | awk '{sum+=$1; n++} END {printf "%.2f", sum/n}')
            info "最近 10 帧平均: ${AVG}ms"
        fi
    else
        fail "帧未推进（3秒内无新帧 — 可能卡死或 dirty 跳帧）"
    fi
else
    warn "未找到 fps.log（需 -fps 参数启动 vistty）"
fi
echo

# 3. DRM CRTC 状态
echo "--- 3. DRM 输出状态 ---"
if command -v drminfo &>/dev/null; then
    drminfo 2>/dev/null | head -30 || warn "drminfo 执行失败"
elif [[ -f /sys/class/drm/card1/status ]]; then
    for conn in /sys/class/drm/card1-*; do
        if [[ -f "${conn}/status" ]]; then
            name=$(basename "$conn")
            status=$(cat "${conn}/status" 2>/dev/null)
            info "${name}: ${status}"
        fi
    done
else
    info "（无可用的 DRM 状态检查工具）"
fi
echo

# 4. FD 数量（检测 fd 泄漏）
echo "--- 4. 文件描述符 ---"
if [[ -d "/proc/${PID}/fd" ]]; then
    FD_COUNT=$(ls "/proc/${PID}/fd" 2>/dev/null | wc -l)
    info "打开的 fd 数: ${FD_COUNT}"
    if [[ $FD_COUNT -gt 200 ]]; then
        warn "fd 数量偏多（>200），可能存在泄漏"
    else
        pass "fd 数量正常"
    fi
fi
echo

# 5. 调试日志中的错误/超时
echo "--- 5. 错误检测 ---"
DEBUG_LOG=""
for candidate in "${OUT_DIR}/debug.log" "${OUT_DIR}/../debug.log" "/tmp/vistty-bench-"*/debug.log; do
    if [[ -f "$candidate" ]]; then
        DEBUG_LOG="$candidate"
        break
    fi
done

if [[ -f "$DEBUG_LOG" ]]; then
    FLIP_TIMEOUTS=$(grep -c "flip 超时" "$DEBUG_LOG" 2>/dev/null || echo 0)
    RENDER_ERRORS=$(grep -c "render error" "$DEBUG_LOG" 2>/dev/null || echo 0)
    EGL_ERRORS=$(grep -c "eglMakeCurrent.*failed\|eglSwapBuffers.*failed" "$DEBUG_LOG" 2>/dev/null || echo 0)

    if [[ $FLIP_TIMEOUTS -eq 0 ]]; then
        pass "无 flip 超时"
    else
        fail "${FLIP_TIMEOUTS} 次 flip 超时"
    fi

    if [[ $RENDER_ERRORS -eq 0 ]]; then
        pass "无渲染错误"
    else
        fail "${RENDER_ERRORS} 次渲染错误"
    fi

    if [[ $EGL_ERRORS -eq 0 ]]; then
        pass "无 EGL 错误"
    else
        fail "${EGL_ERRORS} 次 EGL 错误"
    fi
else
    warn "未找到 debug.log"
fi
echo

echo "=== 检测完成 ==="
