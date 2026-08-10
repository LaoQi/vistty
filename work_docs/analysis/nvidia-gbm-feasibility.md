# nvidia-drm GBM 可行性评估

> 日期：2026-07-22
> 状态：评估完成，结论为不可行，方向停止
> 环境：nvidia-drm 610.43.03, kernel 7.1.4-arch1-1, DRM atomic supported

## 问题

nvidia 环境下 `drm-gbm` 后端 atomic commit 返回 EINVAL，无法启动。

错误日志关键行：
```
DRM_IOCTL_MODE_ATOMIC: invalid argument
atomic modeset commit (after disable): DRM_IOCTL_MODE_ATOMIC: invalid argument
DRM_IOCTL_MODE_SETCRTC: invalid argument
```

## 根因

**nvidia-drm 的 GBM surface BO 的 GEM handle 不能用于 atomic commit。**

验证矩阵：

| BO 来源 | AddFB | Atomic Commit | 说明 |
|---------|-------|---------------|------|
| `CreateDumbBuffer` | OK | **OK** | 标准 dumb buffer，CPU 渲染 |
| `gbm_bo_create` (非 surface) | OK | **OK** | GBM 独立 BO，可用于 atomic |
| `gbm_surface_lock_front_buffer` | OK | **EINVAL** | GBM surface BO，AddFB 不报错但 atomic 拒绝 |

关键发现：
- nvidia-drm 支持 `DRM_CLIENT_CAP_ATOMIC`（`HasAtomic: true`）
- `CAP_ADDFB2_MODIFIERS: 1`
- GBM surface BO modifier 为 LINEAR (0x0)，不是 modifier 问题
- `PrimeFDToHandle` 对 surface BO 的 FD 也返回 EINVAL — BO 不可跨进程导入
- nvidia-drm 在 `AddFB` 时不做完整 scanout 验证，FB 创建成功但 atomic 时被拒绝
- disable-then-enable 重试也失败，legacy `SetCrtc` 也失败

## 环境探测

### nvidia-drm 驱动能力

```
Driver: nvidia-drm v0.0.0 (用户空间 libdrm 报告)
Kernel module: nvidia_drm 610.43.03
Kernel cmdline: modeset=1 nvidia_drm (确认 nvidia-drm.modeset=1)

CRTCs: 4 (200, 392, 584, 776)
Connectors: 3 (HDMI-1, eDP-1, DP-1)
Planes: 12 (4× PRIMARY, 4× CURSOR, 4× OVERLAY)

HasAtomic: true
CAP_DUMB_BUFFER: 1
CAP_PRIME: 0x1 (DRM_PRIME_CAP_IMPORT)
CAP_ASYNC_PAGE_FLIP: 1
CAP_ADDFB2_MODIFIERS: 1
```

### GBM BO 属性

```
# gbm_bo_create（非 surface）
BO: format=0x34325258 stride=10240 handle=2 modifier=0x0 planeCount=1

# gbm_surface_lock_front_buffer（EGL SwapBuffers 后）
Surface BO: format=0x34325258 stride=10240 handle=1 modifier=0x0 planeCount=1
```

两者 modifier 均为 LINEAR，区别仅在 handle 来源。

### EGL 扩展

```
EGL version: 1.5

关键扩展：
EGL_MESA_image_dma_buf_export: YES
EGL_EXT_image_dma_buf_import: YES
EGL_EXT_image_dma_buf_import_modifiers: YES
EGL_EXT_output_base: YES
EGL_EXT_output_drm: YES
EGL_NV_output_drm_flip_event: YES
EGL_KHR_stream: YES
EGL_KHR_stream_attrib: YES
EGL_NV_stream_attrib: YES
EGL_NV_stream_consumer_eglimage: YES
EGL_WL_wayland_eglstream: YES

不可用：
EGL_MESA_drm_image: NO
eglCreateImageKHR(EGL_NATIVE_PIXMAP_KHR, gbm_bo): 返回 0x0（失败）
eglCreateImageKHR(EGL_GL_TEXTURE_2D_KHR, tex): 返回 0x0（失败）
```

### GLES 扩展

```
GL_EXT_memory_object: YES
GL_EXT_memory_object_fd: YES
GL_OES_EGL_image: YES
GL_OES_EGL_image_external: YES
```

## 方案评估

### A. Dumb buffer + atomic（当前 drm 后端）

- **可行性：已工作**
- GPU 加速：无
- 复杂度：零（现有实现）
- 适用：所有 DRM 平台
- 备注：CPU 渲染，性能受限

### B. EGLStream + EGL Output（nvidia 原生路径）

- **可行性：理论可行，未验证**
- GPU 加速：有（零拷贝）
- 复杂度：高（~500 行自研 EGLStream consumer + output 层）
- 适用：**仅 nvidia**
- 备注：
  - EGLStream 是 nvidia 专有扩展，Mesa（Intel/AMD/Ampere）不实现
  - 对非 nvidia 平台无任何收益
  - nvidia + DRM 直出场景本身很边缘（桌面环境下通常不是 DRM master）
  - 需实现 `EGL_EXT_output_base` + `EGL_KHR_stream` + `EGL_NV_output_drm_flip_event` 全链路
  - 维护成本高，nvidia-only 分支

### C. glReadPixels → dumb → atomic

- **可行性：已验证 OK**
- GPU 加速：渲染有，提交有拷贝
- 复杂度：中
- 适用：所有 DRM 平台
- 性能开销：
  - glReadPixels 2560×1440 RGBA: ~14MB/帧
  - RGBA→BGRA 转换 + mmap write: ~14MB/帧
  - 60fps 下 ~1.6GB/s 额外内存带宽
- 备注：过渡方案，不推荐长期使用

### D. 等 nvidia 修复 GBM surface BO atomic 支持

- **可行性：未知**
- 备注：nvidia-drm 550+ 已改进 GBM 支持，610 可能仍有问题
- 需要 nvidia 在内核驱动中修复 GBM surface BO 的 scanout 约束检查

### E. EGLImage → dma_buf export → PRIME import → AddFB2 → atomic

- **可行性：受阻**
- 原因：nvidia EGL 的 `eglCreateImageKHR` 对 `EGL_NATIVE_PIXMAP_KHR`（GBM BO）和 `EGL_GL_TEXTURE_2D_KHR` 均返回 NULL
- `EGL_MESA_image_dma_buf_export` 扩展虽然存在，但无法创建 EGLImage 来导出
- 备注：此路径在 Mesa 平台上可能有效，但 nvidia 不支持

## 结论

**nvidia-drm 下 GBM 模式不可行，方向停止。**

原因：
1. 根因是 nvidia-drm 内核驱动对 GBM surface BO 的 scanout 约束，不是用户空间代码能解决的
2. EGLStream 是唯一零拷贝路径，但仅 nvidia 受益，投入大，场景窄
3. nvidia + DRM 直出场景本身很边缘（通常在 Wayland/X11 下运行，不是 DRM master）
4. Dumb buffer fallback 覆盖所有平台，已工作

**推荐优先级：A（dumb buffer，当前状态）> 不做 > C > B**

## 代码变更

本次评估未修改任何项目代码。工作区已有的 `IsNvidiaDRM` 移除和 nvidia 特殊处理移除是独立变更。

## 保留的测试

无。探测工具 `cmd/drm-probe/` 已清理。
