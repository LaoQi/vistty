#!/usr/bin/env bash
# Vistty GBM 实机性能测试
#
# 在空闲 TTY 上以 root 启动 vistty(drm-gbm)，子进程为 htop，
# 运行指定时长后自动退出，收集 CPU/内存/FPS/锁竞争数据。
#
# 前提：
#   1. 目标 TTY 无其他进程（如 X11/Wayland 合成器）
#   2. 当前用户在 video/input 组
#   3. sudo 可用（TTY 设备 /dev/ttyN 仅 root 可写）
#
# 用法：
#   sudo ./scripts/gbm-bench.sh                    # 默认 tty2, 90秒
#   sudo ./scripts/gbm-bench.sh -t 3               # 指定 tty3
#   sudo ./scripts/gbm-bench.sh -d 120             # 运行120秒
#   sudo ./scripts/gbm-bench.sh -t 2 -d 60         # tty2 运行60秒
#
# 产出（/tmp/vistty-bench-<TS>/）：
#   vistty          编译后的二进制
#   cpu.prof        CPU profile（全程采样）
#   mem.prof        堆内存快照（退出时）
#   mutex.prof      互斥锁 profile（退出时）
#   fps.log         每帧耗时（stderr）
#   debug.log       调试日志
#   monitor.log     外部资源监控（每2秒采样 RSS/CPU/线程数）
#   report.txt      汇总报告
#
set -euo pipefail

TTY_NUM=2
DURATION=90
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
TS="$(date +%Y%m%d-%H%M%S)"
OUT_DIR="/tmp/vistty-bench-${TS}"

while [[ $# -gt 0 ]]; do
    case "$1" in
        -t) TTY_NUM="${2:?}"; shift 2 ;;
        -d) DURATION="${2:?}"; shift 2 ;;
        -h|--help)
            sed -n '2,/^set -euo/{/^set -euo/d;p}' "$0" | sed 's/^# \?//'
            exit 0 ;;
        *) echo "未知参数: $1" >&2; exit 1 ;;
    esac
done

if [[ $EUID -ne 0 ]]; then
    echo "错误：需要 root 权限（TTY 设备 /dev/tty${TTY_NUM} 仅 root 可写）" >&2
    echo "用法：sudo $0 -t ${TTY_NUM} -d ${DURATION}" >&2
    exit 1
fi

TTY_DEV="/dev/tty${TTY_NUM}"
if [[ ! -c "$TTY_DEV" ]]; then
    echo "错误：${TTY_DEV} 不存在或不是字符设备" >&2
    exit 1
fi

echo "========================================"
echo " Vistty GBM 实机性能测试"
echo "========================================"
echo " TTY:       ${TTY_DEV}"
echo " 时长:      ${DURATION}s"
echo " 输出目录:  ${OUT_DIR}"
echo " 项目目录:  ${PROJECT_DIR}"
echo "========================================"
echo

mkdir -p "$OUT_DIR"

# ---- 1. 编译 ----
echo "[1/5] 编译 vistty..."
cd "$PROJECT_DIR"
GOPROXY=https://goproxy.cn,direct go build -o "${OUT_DIR}/vistty" ./cmd/vistty
echo "      二进制: ${OUT_DIR}/vistty ($(du -h "${OUT_DIR}/vistty" | cut -f1))"

# ---- 2. 检查 htop ----
if ! command -v htop &>/dev/null; then
    echo "错误：htop 未安装" >&2
    exit 1
fi
echo "[2/5] htop: $(htop --version 2>&1 | head -1)"

# ---- 3. 检查 DRM 设备 ----
echo "[3/5] DRM 设备检查..."
DRM_CARD=""
for card in /dev/dri/card*; do
    if [[ -c "$card" ]]; then
        DRM_CARD="$card"
        echo "      发现: $card"
    fi
done
if [[ -z "$DRM_CARD" ]]; then
    echo "错误：未找到 /dev/dri/card* 设备" >&2
    exit 1
fi

# ---- 4. 启动 vistty + 监控 ----
echo "[4/5] 启动 vistty (drm-gbm, htop, ${DURATION}s)..."

VISTTY_BIN="${OUT_DIR}/vistty"
VISTTY_PID=""
MONITOR_PID=""

cleanup() {
    echo
    echo "[清理] 停止监控..."
    if [[ -n "$MONITOR_PID" ]] && kill -0 "$MONITOR_PID" 2>/dev/null; then
        kill "$MONITOR_PID" 2>/dev/null || true
        wait "$MONITOR_PID" 2>/dev/null || true
    fi
    echo "[清理] 发送 SIGTERM 到 vistty..."
    if [[ -n "$VISTTY_PID" ]] && kill -0 "$VISTTY_PID" 2>/dev/null; then
        kill -TERM "$VISTTY_PID" 2>/dev/null || true
        # 等待最多 10 秒让 vistty 正常关闭（恢复 CRTC + KD_TEXT）
        for i in $(seq 1 20); do
            if ! kill -0 "$VISTTY_PID" 2>/dev/null; then
                break
            fi
            sleep 0.5
        done
        if kill -0 "$VISTTY_PID" 2>/dev/null; then
            echo "[清理] vistty 未退出，发送 SIGKILL..."
            kill -KILL "$VISTTY_PID" 2>/dev/null || true
        fi
    fi
    # 确保 TTY 恢复文本模式（vistty 正常退出时会恢复，这里兜底）
    if command -v kbd_mode &>/dev/null; then
        kbd_mode -s -C "$TTY_DEV" 2>/dev/null || true
    fi
    echo "[清理] 完成"
}
trap cleanup EXIT

# 资源监控：每 2 秒采样 vistty 进程的 RSS/CPU/线程数
monitor_vistty() {
    local pid="$1"
    local outfile="$2"
    echo "timestamp_rss_kb_cpu_pct_threads" > "$outfile"
    while kill -0 "$pid" 2>/dev/null; do
        if [[ -f "/proc/${pid}/stat" ]]; then
            # stat 字段: pid comm state ppid pgrp session tty_nr tpgid flags ...
            # 我们需要: utime(14) stime(15) rss(24) num_threads(20)
            read -r _ comm state _ _ _ _ _ _ _ _ _ _ utime stime cutime cstime _ _ num_threads _ rss _ < "/proc/${pid}/stat" 2>/dev/null || break
            # 获取总 CPU 时间（jiffies）
            total_jiffies=$((utime + stime))
            echo "$(date +%s) ${rss} 0 ${num_threads}" >> "$outfile"
        fi
        sleep 2
    done
}

# 启动 vistty
# -backend drm-gbm: GPU 加速模式
# -tty: 绑定 TTY
# -config: htop 专用 init.lua
# -cpuprofile/-memprofile/-mutexprofile: profiling
# -fps: 帧时间日志
# VISTTY_DEBUG: 调试日志
export VISTTY_DEBUG=1
export VISTTY_DEBUG_FILE="${OUT_DIR}/debug.log"
export VISTTY_DEBUG_STDERR=0

"$VISTTY_BIN" \
    -backend drm-gbm \
    -tty "$TTY_NUM" \
    -config "${PROJECT_DIR}/scripts/htop-init.lua" \
    -cpuprofile "${OUT_DIR}/cpu.prof" \
    -memprofile "${OUT_DIR}/mem.prof" \
    -mutexprofile "${OUT_DIR}/mutex.prof" \
    -fps \
    2>"${OUT_DIR}/fps.log" &
VISTTY_PID=$!

echo "      PID: ${VISTTY_PID}"

# 等待 vistty 启动（最多 5 秒）
echo "      等待启动..."
for i in $(seq 1 10); do
    if ! kill -0 "$VISTTY_PID" 2>/dev/null; then
        echo "错误：vistty 启动后立即退出" >&2
        echo "------ debug.log (最后 30 行) ------" >&2
        tail -30 "${OUT_DIR}/debug.log" 2>/dev/null >&2 || true
        exit 1
    fi
    if [[ -f "${OUT_DIR}/debug.log" ]] && grep -q "GPU: instanced draw ready" "${OUT_DIR}/debug.log" 2>/dev/null; then
        echo "      GPU instanced draw 已就绪"
        break
    fi
    sleep 0.5
done

# 启动资源监控
monitor_vistty "$VISTTY_PID" "${OUT_DIR}/monitor.log" &
MONITOR_PID=$!

# ---- 5. 运行指定时长 ----
echo "[5/5] 运行 ${DURATION}s..."
echo "      （在 ${TTY_DEV} 上观察 htop 渲染，Ctrl+Alt+F${TTY_NUM} 切换查看）"
echo

# 等待指定时长或 vistty 提前退出
SECONDS=0
while [[ $SECONDS -lt $DURATION ]]; do
    if ! kill -0 "$VISTTY_PID" 2>/dev/null; then
        echo "警告：vistty 在 ${SECONDS}s 时提前退出"
        break
    fi
    sleep 1
done

ELAPSED=$SECONDS
echo
echo "运行结束（${ELAPSED}s）"

# 正常关闭 vistty（SIGTERM 触发两阶段关闭，恢复 CRTC + KD_TEXT）
echo "发送 SIGTERM..."
kill -TERM "$VISTTY_PID" 2>/dev/null || true

# 等待 vistty 退出（最多 10 秒）
for i in $(seq 1 20); do
    if ! kill -0 "$VISTTY_PID" 2>/dev/null; then
        break
    fi
    sleep 0.5
done

if kill -0 "$VISTTY_PID" 2>/dev/null; then
    echo "警告：vistty 未正常退出，发送 SIGKILL"
    kill -KILL "$VISTTY_PID" 2>/dev/null || true
fi

wait "$VISTTY_PID" 2>/dev/null || true
VISTTY_EXIT=$?
echo "vistty 退出码: ${VISTTY_EXIT}"

# 等待监控进程结束
wait "$MONITOR_PID" 2>/dev/null || true

# ---- 生成报告 ----
echo
echo "========================================"
echo " 生成报告..."
echo "========================================"

REPORT="${OUT_DIR}/report.txt"

{
    echo "=== Vistty GBM 实机性能测试报告 ==="
    echo "日期:     $(date '+%Y-%m-%d %H:%M:%S')"
    echo "TTY:      ${TTY_DEV}"
    echo "运行时长: ${ELAPSED}s (目标 ${DURATION}s)"
    echo "退出码:   ${VISTTY_EXIT}"
    echo "输出目录: ${OUT_DIR}"
    echo

    echo "--- 基本状态 ---"
    if [[ $VISTTY_EXIT -eq 0 ]]; then
        echo "正常退出: 是"
    else
        echo "正常退出: 否（退出码 ${VISTTY_EXIT}）"
    fi
    if [[ $ELAPSED -ge $DURATION ]]; then
        echo "运行完整: 是（${ELAPSED}s >= ${DURATION}s）"
    else
        echo "运行完整: 否（${ELAPSED}s < ${DURATION}s，提前退出）"
    fi
    echo

    echo "--- GPU 初始化 ---"
    if grep -q "GPU: instanced draw ready" "${OUT_DIR}/debug.log" 2>/dev/null; then
        grep "GPU: instanced draw ready" "${OUT_DIR}/debug.log" | tail -1
        echo "GPU 路径: 成功"
    elif grep -q "GPU instanced draw init failed" "${OUT_DIR}/debug.log" 2>/dev/null; then
        grep "GPU instanced draw init failed" "${OUT_DIR}/debug.log" | tail -1
        echo "GPU 路径: 失败（回退 CPU）"
    else
        echo "GPU 路径: 未知（未找到相关日志）"
    fi
    echo

    echo "--- VAO 状态 ---"
    if grep -q "HasVAO" "${OUT_DIR}/debug.log" 2>/dev/null; then
        grep "HasVAO" "${OUT_DIR}/debug.log" | tail -1
    else
        echo "（日志中无 VAO 相关信息）"
    fi
    echo

    echo "--- FPS 帧时间统计 ---"
    if [[ -f "${OUT_DIR}/fps.log" ]] && grep -q "^frame:" "${OUT_DIR}/fps.log"; then
        TOTAL_FRAMES=$(grep -c "^frame:" "${OUT_DIR}/fps.log")
        # 提取帧时间数值（格式: "frame: 16.234ms" 或 "frame: 16.234µs" 或 "frame: 1.234s"）
        # 转换为微秒统一计算
        FPS_DATA="${OUT_DIR}/fps_us.tmp"
        > "$FPS_DATA"
        while IFS= read -r line; do
            val="${line#frame: }"
            val="${val% }"
            if [[ "$val" == *ms ]]; then
                us=$(echo "${val%ms}" | awk '{printf "%.0f", $1 * 1000}')
            elif [[ "$val" == *µs ]] || [[ "$val" == *us ]]; then
                us=$(echo "${val%[µu]s}" | awk '{printf "%.0f", $1}')
            elif [[ "$val" == *s ]]; then
                us=$(echo "${val%s}" | awk '{printf "%.0f", $1 * 1000000}')
            else
                continue
            fi
            echo "$us" >> "$FPS_DATA"
        done < <(grep "^frame:" "${OUT_DIR}/fps.log")

        if [[ -s "$FPS_DATA" ]]; then
            AVG_US=$(awk '{sum+=$1; n++} END {if(n>0) printf "%.0f", sum/n; else print 0}' "$FPS_DATA")
            MIN_US=$(awk 'BEGIN{m=1e18} {if($1<m) m=$1} END {printf "%.0f", m}' "$FPS_DATA")
            MAX_US=$(awk 'BEGIN{m=0} {if($1>m) m=$1} END {printf "%.0f", m}' "$FPS_DATA")
            P50_US=$(sort -n "$FPS_DATA" | awk '{a[NR]=$1} END {printf "%.0f", a[int(NR*0.5)]}')
            P95_US=$(sort -n "$FPS_DATA" | awk '{a[NR]=$1} END {printf "%.0f", a[int(NR*0.95)]}')
            P99_US=$(sort -n "$FPS_DATA" | awk '{a[NR]=$1} END {printf "%.0f", a[int(NR*0.99)]}')

            # 超过 16ms 的帧数（丢帧）
            DROPPED=$(awk -v threshold=16000 '$1 > threshold {n++} END {print n+0}' "$FPS_DATA")

            AVG_MS=$(awk "BEGIN {printf \"%.2f\", ${AVG_US}/1000}")
            MIN_MS=$(awk "BEGIN {printf \"%.2f\", ${MIN_US}/1000}")
            MAX_MS=$(awk "BEGIN {printf \"%.2f\", ${MAX_US}/1000}")
            P50_MS=$(awk "BEGIN {printf \"%.2f\", ${P50_US}/1000}")
            P95_MS=$(awk "BEGIN {printf \"%.2f\", ${P95_US}/1000}")
            P99_MS=$(awk "BEGIN {printf \"%.2f\", ${P99_US}/1000}")

            echo "总帧数:     ${TOTAL_FRAMES}"
            echo "平均帧时间: ${AVG_MS}ms"
            echo "最小帧时间: ${MIN_MS}ms"
            echo "最大帧时间: ${MAX_MS}ms"
            echo "P50:        ${P50_MS}ms"
            echo "P95:        ${P95_MS}ms"
            echo "P99:        ${P99_MS}ms"
            echo "丢帧(>16ms): ${DROPPED} (${DROPPED}00/${TOTAL_FRAMES} = $(awk "BEGIN {printf \"%.1f\", ${DROPPED}*100/${TOTAL_FRAMES}")%)"
            if [[ $AVG_US -gt 0 ]]; then
                AVG_FPS=$(awk "BEGIN {printf \"%.1f\", 1000000/${AVG_US}}")
                echo "平均 FPS:   ${AVG_FPS}"
            fi
        fi
        rm -f "$FPS_DATA"
    else
        echo "（无 FPS 数据）"
    fi
    echo

    echo "--- 资源监控统计 ---"
    if [[ -f "${OUT_DIR}/monitor.log" ]] && [[ $(wc -l < "${OUT_DIR}/monitor.log") -gt 1 ]]; then
        # 跳过标题行，分析 RSS/线程数
        RSS_COL=$(awk 'NR>1 {print $2}' "${OUT_DIR}/monitor.log")
        THREAD_COL=$(awk 'NR>1 {print $4}' "${OUT_DIR}/monitor.log")
        if [[ -n "$RSS_COL" ]]; then
            AVG_RSS=$(echo "$RSS_COL" | awk '{sum+=$1; n++} END {if(n>0) printf "%.0f", sum/n; else print 0}')
            MAX_RSS=$(echo "$RSS_COL" | awk 'BEGIN{m=0} {if($1>m) m=$1} END {printf "%.0f", m}')
            AVG_RSS_MB=$(awk "BEGIN {printf \"%.1f\", ${AVG_RSS}/1024}")
            MAX_RSS_MB=$(awk "BEGIN {printf \"%.1f\", ${MAX_RSS}/1024}")
            echo "平均 RSS:   ${AVG_RSS_MB}MB"
            echo "峰值 RSS:   ${MAX_RSS_MB}MB"
        fi
        if [[ -n "$THREAD_COL" ]]; then
            AVG_THREADS=$(echo "$THREAD_COL" | awk '{sum+=$1; n++} END {if(n>0) printf "%.0f", sum/n; else print 0}')
            MAX_THREADS=$(echo "$THREAD_COL" | awk 'BEGIN{m=0} {if($1>m) m=$1} END {printf "%.0f", m}')
            echo "平均线程数: ${AVG_THREADS}"
            echo "峰值线程数: ${MAX_THREADS}"
        fi
    else
        echo "（无监控数据）"
    fi
    echo

    echo "--- Profile 文件 ---"
    for f in cpu.prof mem.prof mutex.prof; do
        if [[ -f "${OUT_DIR}/${f}" ]]; then
            SIZE=$(du -h "${OUT_DIR}/${f}" | cut -f1)
            echo "${f}: ${SIZE}"
        else
            echo "${f}: 未生成"
        fi
    done
    echo

    echo "--- 调试日志摘要 ---"
    if [[ -f "${OUT_DIR}/debug.log" ]]; then
        echo "日志大小: $(du -h "${OUT_DIR}/debug.log" | cut -f1)"
        echo "错误/警告:"
        grep -i "error\|warning\|failed\|fallback" "${OUT_DIR}/debug.log" 2>/dev/null | tail -20 || echo "（无）"
        echo
        echo "GBM Swap 统计:"
        SWAP_COUNT=$(grep -c "GBM Swap:" "${OUT_DIR}/debug.log" 2>/dev/null || echo 0)
        echo "Swap 调用次数: ${SWAP_COUNT}"
        FLIP_TIMEOUT=$(grep -c "flip 超时" "${OUT_DIR}/debug.log" 2>/dev/null || echo 0)
        echo "Flip 超时次数: ${FLIP_TIMEOUT}"
    else
        echo "（无调试日志）"
    fi
    echo

    echo "--- 卡死检测 ---"
    if [[ $ELAPSED -lt $DURATION ]] && [[ $VISTTY_EXIT -ne 0 ]]; then
        echo "结果: 异常退出（vistty 在 ${ELAPSED}s 时退出，目标 ${DURATION}s）"
    elif [[ $FLIP_TIMEOUT -gt 0 ]] 2>/dev/null; then
        echo "结果: 存在 flip 超时（可能卡顿）"
    elif grep -q "render error" "${OUT_DIR}/debug.log" 2>/dev/null; then
        echo "结果: 存在渲染错误"
    else
        echo "结果: 未检测到卡死"
    fi
    echo

    echo "--- Profile 分析命令 ---"
    echo "CPU 热点:    go tool pprof -top ${OUT_DIR}/cpu.prof"
    echo "CPU 调用图:  go tool pprof -web  ${OUT_DIR}/cpu.prof"
    echo "内存分配:    go tool pprof -top ${OUT_DIR}/mem.prof"
    echo "锁竞争:     go tool pprof -top ${OUT_DIR}/mutex.prof"
    echo "交互分析:    go tool pprof ${OUT_DIR}/cpu.prof"

} > "$REPORT"

cat "$REPORT"

echo
echo "========================================"
echo " 测试完成"
echo " 报告: ${REPORT}"
echo "========================================"
