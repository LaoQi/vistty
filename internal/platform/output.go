package platform

type Output interface {
	ID() uint32
	ConnectorID() uint32
	CrtcID() uint32
	Name() string
	Size() (int, int)
}
