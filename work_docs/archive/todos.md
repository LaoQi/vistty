# 待办事项（历史快照）

> **状态：历史快照**。前 7 项已完成。foot 优化剩余阶段（阶段5-7）的完整方案与进度见 `implementation/foot-optimization.md`。此文件作为早期待办清单存档。

- [x] 字体支持emoji
- [x] 支持输入设备变更
- [x] 支持输入法
- [x] 优化GBM下内存
- [x] 多屏丢失
- [x] 面板占用提醒终端resize
- [x] 多颜色主题支持

# foot 优化方案剩余阶段（见 implementation-foot-optimization.md）
- [ ] 阶段5: Cell 紧凑化（16B→12B 位域压缩，Go 对齐规则需评估）
- [ ] 阶段6: Wayland Buffer 优化（buffer_chain 复用 + buffer age + shm_scroll）
- [ ] 阶段7: Sixel + Grapheme Clustering（功能扩展，DCS Sixel 状态机 + VS16/ZWJ）
