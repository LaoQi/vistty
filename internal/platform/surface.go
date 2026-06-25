package platform

type ResizeEvent struct {
	Width  int
	Height int
}

type Surface interface {
	Size() (width, height int)
	Data() []byte
	Stride() int
	Swap() error
	Close() error
	ResizeEvents() <-chan ResizeEvent
	OutputID() uint32
}
