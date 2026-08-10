# Vistty 实施进度

> **状态：已过时（早期阶段进度，已被 AGENTS.md 取代）**
> 本文档仅记录早期阶段 1-3（底层/中间/上层模块）的进度。当前项目已完成远超此范围的功能，完整实现清单见 AGENTS.md "已实现功能概要" 及 `implementation/changelog.md`。
> 本文档保留作历史记录，勿据此判断项目状态。

## 阶段1：底层模块（已完成）

| 模块 | 状态 | 文件数 | 测试 |
|------|------|--------|------|
| internal/platform/drm/internal | ✅ 完成 | 14 | ✅ 通过（vt_test） |
| internal/platform (接口定义) | ✅ 完成 | 3 | N/A |
| internal/vte (转义序列解析器) | ✅ 完成 | 9 | ✅ 通过 |
| internal/screen (终端缓冲区) | ✅ 完成 | 12 | ✅ 通过 |

## 阶段2：中间层模块（已完成）

| 模块 | 状态 | 文件数 | 测试 |
|------|------|--------|------|
| internal/font (字体管理) | ✅ 完成 | 3 | ⚠️ 缺测试文件 |
| internal/render (渲染合成) | ✅ 完成 | 4 | ✅ 通过（draw_test） |
| internal/platform/drm (DRM后端) | ✅ 完成 | 6 | ✅ 通过（vt_test） |

## 审计与修复

- 阶段1审计已完成，修复了以下问题：
  - event.go 误关 fd（改用 syscall.Read）
  - types.go strconv 函数错误（改用 fmt.Sprintf）
  - ModeInfo 结构体布局校验值修正
  - VTE UTF-8 解码支持
  - Screen 全角字符续接机制
  - Screen DirtyRegions bug 修复
  - Screen selection IsEmpty 修复
  - Screen Line.Resize 残留数据修复

## 阶段3：上层模块（已完成）

| 模块 | 状态 | 文件数 | 测试 |
|------|------|--------|------|
| internal/platform/wayland (Wayland后端) | ✅ 完成 | 4 | N/A（需Wayland环境） |
| terminal (胶水层) | ✅ 完成 | 2 | N/A |
| cmd/vistty (入口) | ✅ 完成 | 1 | N/A |

## 审计与修复

- 阶段1审计已完成，修复了以下问题：
  - event.go 误关 fd（改用 syscall.Read）
  - types.go strconv 函数错误（改用 fmt.Sprintf）
  - ModeInfo 结构体布局校验值修正
  - VTE UTF-8 解码支持
  - Screen 全角字符续接机制
  - Screen DirtyRegions bug 修复
  - Screen selection IsEmpty 修复
  - Screen Line.Resize 残留数据修复

构建：`go build ./...` ✅
静态分析：`go vet ./...` ✅

## Wayland 后端说明

- 双缓冲：2 个 wl_shm pool/buffer 对，memfd_create + mmap
- XDG Shell：xdg_wm_base + xdg_surface + xdg_toplevel
- 键盘：wl_keyboard + 简化 XKB keymap 解析（US布局回退）
- 鼠标：wl_pointer（enter/motion/button）
- 使用方式：`vistty -backend wayland`

## 待确认事项
- font 包缺少测试文件
