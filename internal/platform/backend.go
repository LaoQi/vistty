package platform

type Backend interface {
	CreateSurface(width, height int) (Surface, error)
	CreateSurfaceFor(out Output) (Surface, error)
	CreateInputSource() (InputSource, error)
	ListOutputs() ([]Output, error)
	Run(func())
	Done() <-chan struct{}
	Stop() error
	Close() error
}
