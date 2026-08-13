# work_docs — 开发过程文档

按文档性质分类存放，共 5 类：

| 目录 | 说明 |
|------|------|
| `implementation/` | 各功能点的实施记录（方案 + 阶段 + 审计结果 + 变更记录） |
| `design/` | 架构设计方案（含已实施的方案，标注状态） |
| `analysis/` | 性能 / 内存 / 可行性 / 线程模型分析报告 |
| `archive/` | 已过时或被取代的文档（头部标注原因与去向） |
| `test-data/` | 用于在终端中验证渲染效果的测试数据 |

> 完整功能实现清单见 `implementation/changelog.md`（从 AGENTS.md 迁出）。
> 最新架构与模块说明以根目录 `AGENTS.md` 为准。

## implementation/ — 实施记录

| 文档 | 内容 | 状态 |
|------|------|------|
| `changelog.md` | 已实现功能概要（全量 changelog） | 持续更新 |
| `bugfix-2026-07.md` | 缺陷修复实施方案（P0/P1/P2，7 阶段） | 已完成 |
| `emoji.md` | Emoji 彩色渲染（自研 CBDT 解析，零依赖） | 已完成 |
| `fallback-font.md` | 双字体 Fallback 链（Sarasa + NerdFont） | 已完成 |
| `font-patch.md` | 字体补丁系统（vfp 自定义格式 + append-only + TTF 重组装 + ChainFace） | 已完成 |
| `foot-optimization.md` | foot 终端优化（BSU / damage / 环形 grid） | 阶段1-4 完成，5-7 待实施 |
| `italic-fix.md` | 斜体渲染修复（font 层 ShearGlyph） | 已完成 |
| `pinyin-memory.md` | 拼音字典内存优化（94.2MB→23.7MB） | 已完成 |
| `theme.md` | 终端配色主题系统 | 已完成 |
| `ui-refactor.md` | UI 层重构（TabBar/StatusBar/FloatingOverlay/InputTarget） | 已完成 |
| `wayland-csd.md` | Wayland CSD 自绘装饰 | 已完成 |

## design/ — 设计方案

| 文档 | 内容 | 状态 |
|------|------|------|
| `plugins.md` | 插件系统设计方案（gopher-lua + vistty.* API） | 已实施 |
| `input.md` | 输入设备热插拔设计（inotify + capabilities） | 已实施 |
| `plan.md` | GBMSurface 异步 Mailbox 提交架构（三缓冲） | 已实施 |
| `font-chain-custom-font.md` | 自定义主字体时禁用默认字体查找链（patch+nerd） | 已实施 |

## analysis/ — 分析报告

| 文档 | 内容 |
|------|------|
| `gbm_perf_analysis.md` | GBM 模式 CPU 开销热点分析与优化计划 |
| `luavm.md` | Lua VM 执行线程模型分析 |
| `memory_analysis.md` | 内存评估与布局分析（词库部分已更新为优化后数据） |
| `nvidia-gbm-feasibility.md` | nvidia-drm GBM 可行性评估（结论：不可行，方向停止） |
| `optimize.md` | 渲染热点分析与优化（多轮优化记录） |
| `sixel-evaluation.md` | Sixel 图形支持技术评估（仅探索，不实施） |

## archive/ — 已过时文档

| 文档 | 取代者 |
|------|--------|
| `emoji.md` | `implementation/emoji.md`（早期 go-text/typesetting 方案被自研 CBDT 取代） |
| `ime.md` | 当前 pinyin 顶层包架构（无 ime/ 包） |
| `ime_candidate_optimize.md` | 当前 ime.lua 自适应分页实现 |
| `ime_go_refactor.md` | 当前 pinyin 顶层包架构（重构更进一步） |
| `progress.md` | `implementation/changelog.md` 与 AGENTS.md |
| `todos.md` | `implementation/foot-optimization.md`（阶段5-7 待办） |

## test-data/ — 渲染测试数据

| 文档 | 内容 |
|------|------|
| `emoji-test.md` | Emoji 彩色渲染测试字符表（P0+P1 单 rune） |
| `font-test.md` | 字体显示测试（主字体 / fallback / 合成字形覆盖） |
