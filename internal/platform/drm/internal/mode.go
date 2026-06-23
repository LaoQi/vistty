package internal

import (
	"unsafe"
)

const (
	Connected    uint8 = 1
	Disconnected uint8 = 2
	UnknownConn  uint8 = 3
)

type ModeInfoPublic struct {
	Clock      uint32
	HDisplay   uint16
	HSyncStart uint16
	HSyncEnd   uint16
	HTotal     uint16
	HSkew      uint16
	VDisplay   uint16
	VSyncStart uint16
	VSyncEnd   uint16
	VTotal     uint16
	VScan      uint16
	VRefresh   uint32
	Flags      uint32
	Type       uint32
	Name       string
}

type ResourcesPublic struct {
	FbIDs        []uint32
	CrtcIDs      []uint32
	ConnectorIDs []uint32
	EncoderIDs   []uint32
	MinWidth     uint32
	MaxWidth     uint32
	MinHeight    uint32
	MaxHeight    uint32
}

type ConnectorResult struct {
	ID            uint32
	ConnectorType uint32
	TypeID        uint32
	Connection    uint32
	MMWidth       uint32
	MMHeight      uint32
	Subpixel      uint32
	EncoderID     uint32
	EncoderIDs    []uint32
	Modes         []ModeInfoPublic
}

type EncoderResult struct {
	ID             uint32
	EncoderType    uint32
	CrtcID         uint32
	PossibleCrtcs  uint32
	PossibleClones uint32
}

type CrtcResult struct {
	ID         uint32
	FbID       uint32
	X          uint32
	Y          uint32
	GammaSize  uint32
	ModeValid  bool
	Mode       ModeInfoPublic
}

func GetResources(fd int) (*ResourcesPublic, error) {
	var res Resources
	if err := drmIoctl(fd, DRM_IOCTL_MODE_GETRESOURCES, unsafe.Pointer(&res), "DRM_IOCTL_MODE_GETRESOURCES"); err != nil {
		return nil, err
	}

	fbIDs := make([]uint32, res.CountFbs)
	crtcIDs := make([]uint32, res.CountCrtcs)
	connIDs := make([]uint32, res.CountConnectors)
	encIDs := make([]uint32, res.CountEncoders)

	res.FbIDPtr = uint64(uintptr(unsafe.Pointer(&fbIDs[0])))
	res.CrtcIDPtr = uint64(uintptr(unsafe.Pointer(&crtcIDs[0])))
	res.ConnectorIDPtr = uint64(uintptr(unsafe.Pointer(&connIDs[0])))
	res.EncoderIDPtr = uint64(uintptr(unsafe.Pointer(&encIDs[0])))

	if err := drmIoctl(fd, DRM_IOCTL_MODE_GETRESOURCES, unsafe.Pointer(&res), "DRM_IOCTL_MODE_GETRESOURCES"); err != nil {
		return nil, err
	}

	return &ResourcesPublic{
		FbIDs:        fbIDs,
		CrtcIDs:      crtcIDs,
		ConnectorIDs: connIDs,
		EncoderIDs:   encIDs,
		MinWidth:     res.MinWidth,
		MaxWidth:     res.MaxWidth,
		MinHeight:    res.MinHeight,
		MaxHeight:    res.MaxHeight,
	}, nil
}

func GetConnector(fd int, id uint32) (*ConnectorResult, error) {
	var c Connector
	c.ConnectorID = id

	if err := drmIoctl(fd, DRM_IOCTL_MODE_GETCONNECTOR, unsafe.Pointer(&c), "DRM_IOCTL_MODE_GETCONNECTOR"); err != nil {
		return nil, err
	}

	encIDs := make([]uint32, c.CountEncoders)
	modes := make([]ModeInfo, c.CountModes)
	props := make([]uint32, c.CountProps)
	propVals := make([]uint64, c.CountProps)

	c.EncodersPtr = uint64(uintptr(unsafe.Pointer(&encIDs[0])))
	c.ModesPtr = uint64(uintptr(unsafe.Pointer(&modes[0])))
	c.PropsPtr = uint64(uintptr(unsafe.Pointer(&props[0])))
	c.PropValuesPtr = uint64(uintptr(unsafe.Pointer(&propVals[0])))
	c.CountProps = 0

	if err := drmIoctl(fd, DRM_IOCTL_MODE_GETCONNECTOR, unsafe.Pointer(&c), "DRM_IOCTL_MODE_GETCONNECTOR"); err != nil {
		return nil, err
	}

	pubModes := make([]ModeInfoPublic, len(modes))
	for i, m := range modes {
		pubModes[i] = modeInfoToPublic(&m)
	}

	return &ConnectorResult{
		ID:            c.ConnectorID,
		ConnectorType: c.ConnectorType,
		TypeID:        c.ConnectorTypeID,
		Connection:    c.Connection,
		MMWidth:       c.MMWidth,
		MMHeight:      c.MMHeight,
		Subpixel:      c.Subpixel,
		EncoderID:     c.EncoderID,
		EncoderIDs:    encIDs,
		Modes:         pubModes,
	}, nil
}

func GetEncoder(fd int, id uint32) (*EncoderResult, error) {
	var e Encoder
	e.EncoderID = id

	if err := drmIoctl(fd, DRM_IOCTL_MODE_GETENCODER, unsafe.Pointer(&e), "DRM_IOCTL_MODE_GETENCODER"); err != nil {
		return nil, err
	}

	return &EncoderResult{
		ID:             e.EncoderID,
		EncoderType:    e.EncoderType,
		CrtcID:         e.CrtcID,
		PossibleCrtcs:  e.PossibleCrtcs,
		PossibleClones: e.PossibleClones,
	}, nil
}

func GetCrtc(fd int, id uint32) (*CrtcResult, error) {
	var c Crtc
	c.CrtcID = id

	if err := drmIoctl(fd, DRM_IOCTL_MODE_GETCRTC, unsafe.Pointer(&c), "DRM_IOCTL_MODE_GETCRTC"); err != nil {
		return nil, err
	}

	return &CrtcResult{
		ID:        c.CrtcID,
		FbID:      c.FbID,
		X:         c.X,
		Y:         c.Y,
		GammaSize: c.GammaSize,
		ModeValid: c.ModeValid != 0,
		Mode:      modeInfoToPublic(&c.Mode),
	}, nil
}

func SetCrtc(fd int, crtcID, fbID uint32, x, y uint32, mode *ModeInfoPublic, connectors []uint32) error {
	var c Crtc
	c.CrtcID = crtcID
	c.FbID = fbID
	c.X = x
	c.Y = y
	c.SetConnectorsPtr = 0
	c.CountConnectors = 0

	if len(connectors) > 0 {
		c.SetConnectorsPtr = uint64(uintptr(unsafe.Pointer(&connectors[0])))
		c.CountConnectors = uint32(len(connectors))
	}

	if mode != nil {
		c.ModeValid = 1
		c.Mode = publicToModeInfo(mode)
	}

	return drmIoctl(fd, DRM_IOCTL_MODE_SETCRTC, unsafe.Pointer(&c), "DRM_IOCTL_MODE_SETCRTC")
}

func modeInfoToPublic(m *ModeInfo) ModeInfoPublic {
	n := m.Name[:]
	end := 0
	for end < len(n) && n[end] != 0 {
		end++
	}
	return ModeInfoPublic{
		Clock:      m.Clock,
		HDisplay:   m.HDisplay,
		HSyncStart: m.HSyncStart,
		HSyncEnd:   m.HSyncEnd,
		HTotal:     m.HTotal,
		HSkew:      m.HSkew,
		VDisplay:   m.VDisplay,
		VSyncStart: m.VSyncStart,
		VSyncEnd:   m.VSyncEnd,
		VTotal:     m.VTotal,
		VScan:      m.VScan,
		VRefresh:   m.VRefresh,
		Flags:      m.Flags,
		Type:       m.Type,
		Name:       string(n[:end]),
	}
}

func publicToModeInfo(p *ModeInfoPublic) ModeInfo {
	var m ModeInfo
	m.Clock = p.Clock
	m.HDisplay = p.HDisplay
	m.HSyncStart = p.HSyncStart
	m.HSyncEnd = p.HSyncEnd
	m.HTotal = p.HTotal
	m.HSkew = p.HSkew
	m.VDisplay = p.VDisplay
	m.VSyncStart = p.VSyncStart
	m.VSyncEnd = p.VSyncEnd
	m.VTotal = p.VTotal
	m.VScan = p.VScan
	m.VRefresh = p.VRefresh
	m.Flags = p.Flags
	m.Type = p.Type
	copy(m.Name[:], p.Name)
	return m
}
