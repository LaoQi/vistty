package internal

import (
	"fmt"
	"reflect"
	"runtime"
)

type Version struct {
	VersionMajor    int32
	VersionMinor    int32
	VersionPatch    int32
	_               uint32
	NameLen        uint64
	NamePtr        uint64
	DateLen        uint64
	DatePtr        uint64
	DescLen        uint64
	DescPtr        uint64
}

type Capability struct {
	Capability uint64
	Value      uint64
}

type Resources struct {
	FbIDPtr        uint64
	CrtcIDPtr      uint64
	ConnectorIDPtr uint64
	EncoderIDPtr   uint64
	CountFbs       uint32
	CountCrtcs     uint32
	CountConnectors uint32
	CountEncoders  uint32
	MinWidth       uint32
	MaxWidth       uint32
	MinHeight      uint32
	MaxHeight      uint32
}

type ModeInfo struct {
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
	Name       [32]byte
}

type Connector struct {
	EncodersPtr     uint64
	ModesPtr        uint64
	PropsPtr        uint64
	PropValuesPtr   uint64
	CountModes      uint32
	CountProps      uint32
	CountEncoders   uint32
	EncoderID       uint32
	ConnectorID     uint32
	ConnectorType   uint32
	ConnectorTypeID uint32
	Connection      uint32
	MMWidth         uint32
	MMHeight        uint32
	Subpixel        uint32
	Pad             uint32
}

type Encoder struct {
	EncoderID      uint32
	EncoderType    uint32
	CrtcID         uint32
	PossibleCrtcs  uint32
	PossibleClones uint32
}

type Crtc struct {
	SetConnectorsPtr uint64
	CountConnectors  uint32
	CrtcID           uint32
	FbID             uint32
	X                uint32
	Y                uint32
	GammaSize        uint32
	ModeValid        uint32
	Mode             ModeInfo
}

type FB struct {
	FbID   uint32
	Width  uint32
	Height uint32
	Pitch  uint32
	BPP    uint32
	Depth  uint32
	Handle uint32
}

type CreateDumb struct {
	Height uint32
	Width  uint32
	BPP    uint32
	Flags  uint32
	Handle uint32
	Pitch  uint32
	Size   uint64
}

type MapDumb struct {
	Handle uint32
	Pad    uint32
	Offset uint64
}

type DestroyDumb struct {
	Handle uint32
}

type PageFlip struct {
	CrtcID    uint32
	FbID      uint32
	Flags     uint32
	Reserved  uint32
	UserData  uint64
}

func init() {
	if runtime.GOARCH != "amd64" {
		panic("drm/internal: only linux/amd64 is supported")
	}

	mustSize := func(typ reflect.Type, expected uintptr) {
		if typ.Size() != expected {
			panic("drm/internal: " + typ.Name() + " size mismatch: got " + strconv(typ.Size()) + ", expected " + strconv(expected))
		}
	}

	mustOffset := func(typ reflect.Type, field string, expected uintptr) {
		actual, ok := typ.FieldByName(field)
		if !ok {
			panic("drm/internal: " + typ.Name() + " has no field " + field)
		}
		if actual.Offset != expected {
			panic("drm/internal: " + typ.Name() + "." + field + " offset mismatch: got " + strconv(actual.Offset) + ", expected " + strconv(expected))
		}
	}

	mustSize(reflect.TypeOf(Version{}), 64)
	mustOffset(reflect.TypeOf(Version{}), "VersionMajor", 0)
	mustOffset(reflect.TypeOf(Version{}), "VersionMinor", 4)
	mustOffset(reflect.TypeOf(Version{}), "VersionPatch", 8)
	mustOffset(reflect.TypeOf(Version{}), "NameLen", 16)
	mustOffset(reflect.TypeOf(Version{}), "NamePtr", 24)
	mustOffset(reflect.TypeOf(Version{}), "DateLen", 32)
	mustOffset(reflect.TypeOf(Version{}), "DatePtr", 40)
	mustOffset(reflect.TypeOf(Version{}), "DescLen", 48)
	mustOffset(reflect.TypeOf(Version{}), "DescPtr", 56)

	mustSize(reflect.TypeOf(Capability{}), 16)
	mustOffset(reflect.TypeOf(Capability{}), "Capability", 0)
	mustOffset(reflect.TypeOf(Capability{}), "Value", 8)

	mustSize(reflect.TypeOf(Resources{}), 64)
	mustOffset(reflect.TypeOf(Resources{}), "FbIDPtr", 0)
	mustOffset(reflect.TypeOf(Resources{}), "CrtcIDPtr", 8)
	mustOffset(reflect.TypeOf(Resources{}), "ConnectorIDPtr", 16)
	mustOffset(reflect.TypeOf(Resources{}), "EncoderIDPtr", 24)
	mustOffset(reflect.TypeOf(Resources{}), "CountFbs", 32)
	mustOffset(reflect.TypeOf(Resources{}), "CountCrtcs", 36)
	mustOffset(reflect.TypeOf(Resources{}), "CountConnectors", 40)
	mustOffset(reflect.TypeOf(Resources{}), "CountEncoders", 44)
	mustOffset(reflect.TypeOf(Resources{}), "MinWidth", 48)
	mustOffset(reflect.TypeOf(Resources{}), "MaxWidth", 52)
	mustOffset(reflect.TypeOf(Resources{}), "MinHeight", 56)
	mustOffset(reflect.TypeOf(Resources{}), "MaxHeight", 60)

	mustSize(reflect.TypeOf(ModeInfo{}), 68)
	mustOffset(reflect.TypeOf(ModeInfo{}), "Clock", 0)
	mustOffset(reflect.TypeOf(ModeInfo{}), "HDisplay", 4)
	mustOffset(reflect.TypeOf(ModeInfo{}), "VRefresh", 24)
	mustOffset(reflect.TypeOf(ModeInfo{}), "Flags", 28)
	mustOffset(reflect.TypeOf(ModeInfo{}), "Type", 32)
	mustOffset(reflect.TypeOf(ModeInfo{}), "Name", 36)

	mustSize(reflect.TypeOf(Crtc{}), 104)
	mustOffset(reflect.TypeOf(Crtc{}), "SetConnectorsPtr", 0)
	mustOffset(reflect.TypeOf(Crtc{}), "CountConnectors", 8)
	mustOffset(reflect.TypeOf(Crtc{}), "CrtcID", 12)
	mustOffset(reflect.TypeOf(Crtc{}), "FbID", 16)
	mustOffset(reflect.TypeOf(Crtc{}), "ModeValid", 32)
	mustOffset(reflect.TypeOf(Crtc{}), "Mode", 36)

	mustSize(reflect.TypeOf(Connector{}), 80)
	mustOffset(reflect.TypeOf(Connector{}), "EncodersPtr", 0)
	mustOffset(reflect.TypeOf(Connector{}), "ModesPtr", 8)
	mustOffset(reflect.TypeOf(Connector{}), "CountModes", 32)
	mustOffset(reflect.TypeOf(Connector{}), "ConnectorID", 48)
	mustOffset(reflect.TypeOf(Connector{}), "Connection", 60)

	mustSize(reflect.TypeOf(Encoder{}), 20)
	mustOffset(reflect.TypeOf(Encoder{}), "EncoderID", 0)
	mustOffset(reflect.TypeOf(Encoder{}), "CrtcID", 8)
	mustOffset(reflect.TypeOf(Encoder{}), "PossibleCrtcs", 12)

	mustSize(reflect.TypeOf(FB{}), 28)
	mustOffset(reflect.TypeOf(FB{}), "FbID", 0)
	mustOffset(reflect.TypeOf(FB{}), "Handle", 24)

	mustSize(reflect.TypeOf(CreateDumb{}), 32)
	mustOffset(reflect.TypeOf(CreateDumb{}), "Handle", 16)
	mustOffset(reflect.TypeOf(CreateDumb{}), "Pitch", 20)
	mustOffset(reflect.TypeOf(CreateDumb{}), "Size", 24)

	mustSize(reflect.TypeOf(MapDumb{}), 16)
	mustOffset(reflect.TypeOf(MapDumb{}), "Offset", 8)

	mustSize(reflect.TypeOf(DestroyDumb{}), 4)

	mustSize(reflect.TypeOf(PageFlip{}), 24)
	mustOffset(reflect.TypeOf(PageFlip{}), "UserData", 16)
}

func strconv(v uintptr) string {
	return fmt.Sprintf("%d", v)
}
