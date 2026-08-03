package platform

type ResizeEvent struct {
	Width    int
	Height   int
	OutputID uint32
}

type Surface interface {
	Size() (width, height int)
	Data() []byte
	Stride() int
	Swap() error
	Close() error
	ResizeEvents() <-chan ResizeEvent
	OutputID() uint32
	DirectRender() bool
	DecoMode() uint32
}

type WindowMover interface {
	StartMove(serial uint32)
	StartResize(serial uint32, edge uint32)
}

// Screenshotter 可选接口，由 GPU 渲染路径的 Surface 实现（GBM）。
// CPU 路径（DRM dumb / Wayland）无需实现，调用方直接读
// render.Compositor.BackBuf()。
//
// GPU instanced draw 路径下像素只存在于 GL backbuffer，cpuBuf 无有效
// 数据，必须在 Swap 内 eglSwapBuffers 之前（EGLContext 已 current）
// 通过 glReadPixels 回读。
type Screenshotter interface {
	// RequestScreenshot 请求下一次 Swap 时回读帧缓冲。
	// 须在渲染主线程调用（与 Swap 同线程）。
	RequestScreenshot()
	// ReadScreenshot 取回最近一次回读的像素，统一输出 BGRA32
	// （内部已完成 Y 翻转与 R/B 交换，alpha 置 255）。
	// 尚无回读数据时 ok=false。
	ReadScreenshot() (data []byte, stride, width, height int, ok bool)
}
