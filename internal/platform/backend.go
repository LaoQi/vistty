package platform

type Backend interface {
	CreateSurface(width, height int) (Surface, error)
	CreateInputSource() (InputSource, error)
	Run(func())
	Done() <-chan struct{}
	Stop() error
	Close() error
}
