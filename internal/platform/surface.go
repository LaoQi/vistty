package platform

type Surface interface {
	Size() (width, height int)
	Data() []byte
	Stride() int
	Swap() error
	Close() error
}
