#!/usr/bin/env bash
# quick-update.sh — 快速更新本地配置和可执行文件
#
# 功能：
#   1. 编译 vistty 并安装到 /usr/local/bin/vistty（需 sudo 时自动申请）
#   2. 同步 examples/ 配置到 ~/.config/vistty/，保留本地 init.lua 中的 vistty.config 表
#
# 用法：
#   ./scripts/quick-update.sh [选项]
#
# 选项：
#   --bin-only     仅更新可执行文件，不同步配置
#   --cfg-only     仅同步配置，不编译安装
#   -h, --help     显示此帮助
set -euo pipefail

cd "$(dirname "$0")/.."

BIN_ONLY=false
CFG_ONLY=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --bin-only)  BIN_ONLY=true; shift ;;
        --cfg-only)  CFG_ONLY=true; shift ;;
        -h|--help)   sed -n '2,/^set -euo/{/^set -euo/d;p}' "$0" | sed 's/^# \?//'; exit 0 ;;
        *)           echo "未知选项: $1" >&2; exit 1 ;;
    esac
done

REPO_ROOT="$(pwd)"
EXAMPLES_DIR="${REPO_ROOT}/examples"
CONFIG_DIR="${HOME}/.config/vistty"

install_bin() {
    echo "==> 编译 vistty ..."
    bash scripts/build.sh /tmp/vistty-build-$$
    echo "==> 安装到 /usr/local/bin/vistty ..."
    local need_sudo=false
    if [[ ! -w /usr/local/bin ]]; then
        need_sudo=true
    elif [[ -e /usr/local/bin/vistty && ! -w /usr/local/bin/vistty ]]; then
        need_sudo=true
    fi

    if ${need_sudo}; then
        sudo mv /tmp/vistty-build-$$ /usr/local/bin/vistty
        sudo chmod 755 /usr/local/bin/vistty
    else
        mv /tmp/vistty-build-$$ /usr/local/bin/vistty
        chmod 755 /usr/local/bin/vistty
    fi
    echo "    /usr/local/bin/vistty 已更新"
}

sync_config() {
    echo "==> 同步配置到 ${CONFIG_DIR} ..."
    mkdir -p "${CONFIG_DIR}/themes"

    for src in "${EXAMPLES_DIR}"/*.lua; do
        base="$(basename "${src}")"
        dst="${CONFIG_DIR}/${base}"

        if [[ "${base}" == "init.lua" ]]; then
            if [[ ! -f "${dst}" ]]; then
                cp "${src}" "${dst}"
                echo "    init.lua — 首次复制"
            else
                merge_init_lua "${src}" "${dst}"
            fi
        else
            if ! diff -q "${src}" "${dst}" &>/dev/null; then
                cp "${src}" "${dst}"
                echo "    ${base} — 已更新"
            else
                echo "    ${base} — 无变化"
            fi
        fi
    done

    for src in "${EXAMPLES_DIR}/themes/"*.lua; do
        [[ -f "${src}" ]] || continue
        base="$(basename "${src}")"
        dst="${CONFIG_DIR}/themes/${base}"

        if [[ "${base}" == "custom.lua" ]] && [[ -f "${dst}" ]]; then
            echo "    themes/${base} — 跳过（保留本地自定义）"
            continue
        fi

        if ! diff -q "${src}" "${dst}" &>/dev/null; then
            cp "${src}" "${dst}"
            echo "    themes/${base} — 已更新"
        else
            echo "    themes/${base} — 无变化"
        fi
    done
}

find_block_end() {
    local file="$1" start_line="$2"
    local brace=0 open=false line_n=0
    while IFS= read -r line; do
        line_n=$((line_n + 1))
        [[ "${line}" =~ \{ ]] && { brace=$((brace + $(echo "$line" | grep -o '{' | wc -l))); open=true; }
        [[ "${line}" =~ \} ]] && brace=$((brace - $(echo "$line" | grep -o '}' | wc -l)))
        ${open} && [[ ${brace} -le 0 ]] && { echo $((start_line + line_n - 1)); return; }
    done < <(tail -n +${start_line} "${file}")
    echo $((start_line + line_n - 1))
}

merge_init_lua() {
    local src="$1" dst="$2"
    local tmp="${dst}.tmp.$$"

    local dst_start dst_end src_start src_end
    dst_start=$(grep -n 'vistty\.config\s*=' "${dst}" | head -1 | cut -d: -f1)
    if [[ -z "${dst_start}" ]]; then
        cp "${src}" "${dst}"
        echo "    init.lua — 未找到本地 vistty.config，完整覆盖"
        return
    fi
    dst_end=$(find_block_end "${dst}" "${dst_start}")

    src_start=$(grep -n 'vistty\.config\s*=' "${src}" | head -1 | cut -d: -f1)
    if [[ -z "${src_start}" ]]; then
        echo "    init.lua — 上游无 vistty.config，跳过"
        return
    fi
    src_end=$(find_block_end "${src}" "${src_start}")

    {
        head -n $((dst_start - 1)) "${dst}"
        tail -n +${dst_start} "${dst}" | head -n $((dst_end - dst_start + 1))
        tail -n +$((src_end + 1)) "${src}"
    } > "${tmp}"

    if ! diff -q "${tmp}" "${dst}" &>/dev/null; then
        mv "${tmp}" "${dst}"
        echo "    init.lua — 已合并（保留本地 vistty.config，更新其余逻辑）"
    else
        rm -f "${tmp}"
        echo "    init.lua — 无变化"
    fi
}

if ${BIN_ONLY}; then
    install_bin
elif ${CFG_ONLY}; then
    sync_config
else
    install_bin
    echo
    sync_config
fi

echo "==> 完成"
