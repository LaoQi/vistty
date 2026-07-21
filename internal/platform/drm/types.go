package drm

import (
	"fmt"
	"reflect"
	"runtime"
)

type Version struct {
	VersionMajor int32
	VersionMinor int32
	VersionPatch int32
	_            uint32
	NameLen      uint64
	NamePtr      uint64
	DateLen      uint64
	DatePtr      uint64
	DescLen      uint64
	DescPtr      uint64
}

type Capability struct {
	Capability uint64
	Value      uint64
}

type Resources struct {
	FbIDPtr         uint64
	CrtcIDPtr       uint64
	ConnectorIDPtr  uint64
	EncoderIDPtr    uint64
	CountFbs        uint32
	CountCrtcs      uint32
	CountConnectors uint32
	CountEncoders   uint32
	MinWidth        uint32
	MaxWidth        uint32
	MinHeight       uint32
	MaxHeight       uint32
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

type FB2 struct {
	FbID        uint32
	Width       uint32
	Height      uint32
	PixelFormat uint32
	Flags       uint32
	Handles     [4]uint32
	Pitches     [4]uint32
	Offsets     [4]uint32
	_           uint32
	Modifier    [4]uint64
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
	CrtcID   uint32
	FbID     uint32
	Flags    uint32
	Reserved uint32
	UserData uint64
}

type AtomicReq struct {
	Flags         uint32
	CountObjs     uint32
	ObjsPtr       uint64
	CountPropsPtr uint64
	PropsPtr      uint64
	PropValuesPtr uint64
	Reserved      uint64
	UserData      uint64
}

type PlaneRes struct {
	PlaneIDPtr  uint64
	CountPlanes uint32
	_           uint32
}

type Plane struct {
	PlaneID          uint32
	CrtcID           uint32
	FbID             uint32
	PossibleCrtcs    uint32
	GammaSize        uint32
	CountFormatTypes uint32
	FormatTypePtr    uint64
}

type PropertyRes struct {
	ValuesPtr      uint64
	EnumBlobPtr    uint64
	PropID         uint32
	Flags          uint32
	Name           [32]byte
	CountValues    uint32
	CountEnumBlobs uint32
}

type ObjProperties struct {
	PropsPtr      uint64
	PropValuesPtr uint64
	CountProps    uint32
	ObjID         uint32
	ObjType       uint32
	_             uint32
}

type GetBlobReq struct {
	BlobID uint32
	Length uint32
	Data   uint64
}

type CreateBlobReq struct {
	Data   uint64
	Length uint32
	BlobID uint32
}

type DestroyBlobReq struct {
	BlobID uint32
}

type PrimeHandle struct {
	Handle uint32
	FD     uint32
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

	mustSize(reflect.TypeOf(FB2{}), 104)
	mustOffset(reflect.TypeOf(FB2{}), "FbID", 0)
	mustOffset(reflect.TypeOf(FB2{}), "Width", 4)
	mustOffset(reflect.TypeOf(FB2{}), "Height", 8)
	mustOffset(reflect.TypeOf(FB2{}), "PixelFormat", 12)
	mustOffset(reflect.TypeOf(FB2{}), "Flags", 16)
	mustOffset(reflect.TypeOf(FB2{}), "Handles", 20)
	mustOffset(reflect.TypeOf(FB2{}), "Pitches", 36)
	mustOffset(reflect.TypeOf(FB2{}), "Offsets", 52)
	mustOffset(reflect.TypeOf(FB2{}), "Modifier", 72)

	mustSize(reflect.TypeOf(CreateDumb{}), 32)
	mustOffset(reflect.TypeOf(CreateDumb{}), "Handle", 16)
	mustOffset(reflect.TypeOf(CreateDumb{}), "Pitch", 20)
	mustOffset(reflect.TypeOf(CreateDumb{}), "Size", 24)

	mustSize(reflect.TypeOf(MapDumb{}), 16)
	mustOffset(reflect.TypeOf(MapDumb{}), "Offset", 8)

	mustSize(reflect.TypeOf(DestroyDumb{}), 4)

	mustSize(reflect.TypeOf(PageFlip{}), 24)
	mustOffset(reflect.TypeOf(PageFlip{}), "UserData", 16)

	mustSize(reflect.TypeOf(AtomicReq{}), 56)
	mustOffset(reflect.TypeOf(AtomicReq{}), "Flags", 0)
	mustOffset(reflect.TypeOf(AtomicReq{}), "CountObjs", 4)
	mustOffset(reflect.TypeOf(AtomicReq{}), "ObjsPtr", 8)
	mustOffset(reflect.TypeOf(AtomicReq{}), "CountPropsPtr", 16)
	mustOffset(reflect.TypeOf(AtomicReq{}), "PropsPtr", 24)
	mustOffset(reflect.TypeOf(AtomicReq{}), "PropValuesPtr", 32)
	mustOffset(reflect.TypeOf(AtomicReq{}), "Reserved", 40)
	mustOffset(reflect.TypeOf(AtomicReq{}), "UserData", 48)

	mustSize(reflect.TypeOf(PlaneRes{}), 16)
	mustOffset(reflect.TypeOf(PlaneRes{}), "PlaneIDPtr", 0)
	mustOffset(reflect.TypeOf(PlaneRes{}), "CountPlanes", 8)

	mustSize(reflect.TypeOf(Plane{}), 32)
	mustOffset(reflect.TypeOf(Plane{}), "PlaneID", 0)
	mustOffset(reflect.TypeOf(Plane{}), "CrtcID", 4)
	mustOffset(reflect.TypeOf(Plane{}), "FbID", 8)
	mustOffset(reflect.TypeOf(Plane{}), "PossibleCrtcs", 12)
	mustOffset(reflect.TypeOf(Plane{}), "GammaSize", 16)
	mustOffset(reflect.TypeOf(Plane{}), "CountFormatTypes", 20)
	mustOffset(reflect.TypeOf(Plane{}), "FormatTypePtr", 24)

	mustSize(reflect.TypeOf(PropertyRes{}), 64)
	mustOffset(reflect.TypeOf(PropertyRes{}), "ValuesPtr", 0)
	mustOffset(reflect.TypeOf(PropertyRes{}), "EnumBlobPtr", 8)
	mustOffset(reflect.TypeOf(PropertyRes{}), "PropID", 16)
	mustOffset(reflect.TypeOf(PropertyRes{}), "Flags", 20)
	mustOffset(reflect.TypeOf(PropertyRes{}), "Name", 24)
	mustOffset(reflect.TypeOf(PropertyRes{}), "CountValues", 56)
	mustOffset(reflect.TypeOf(PropertyRes{}), "CountEnumBlobs", 60)

	mustSize(reflect.TypeOf(ObjProperties{}), 32)
	mustOffset(reflect.TypeOf(ObjProperties{}), "PropsPtr", 0)
	mustOffset(reflect.TypeOf(ObjProperties{}), "PropValuesPtr", 8)
	mustOffset(reflect.TypeOf(ObjProperties{}), "CountProps", 16)
	mustOffset(reflect.TypeOf(ObjProperties{}), "ObjID", 20)
	mustOffset(reflect.TypeOf(ObjProperties{}), "ObjType", 24)

	mustSize(reflect.TypeOf(GetBlobReq{}), 16)
	mustOffset(reflect.TypeOf(GetBlobReq{}), "BlobID", 0)
	mustOffset(reflect.TypeOf(GetBlobReq{}), "Length", 4)
	mustOffset(reflect.TypeOf(GetBlobReq{}), "Data", 8)

	mustSize(reflect.TypeOf(CreateBlobReq{}), 16)
	mustOffset(reflect.TypeOf(CreateBlobReq{}), "Data", 0)
	mustOffset(reflect.TypeOf(CreateBlobReq{}), "Length", 8)
	mustOffset(reflect.TypeOf(CreateBlobReq{}), "BlobID", 12)

	mustSize(reflect.TypeOf(DestroyBlobReq{}), 4)
	mustOffset(reflect.TypeOf(DestroyBlobReq{}), "BlobID", 0)

	mustSize(reflect.TypeOf(PrimeHandle{}), 8)
	mustOffset(reflect.TypeOf(PrimeHandle{}), "Handle", 0)
	mustOffset(reflect.TypeOf(PrimeHandle{}), "FD", 4)
}

func strconv(v uintptr) string {
	return fmt.Sprintf("%d", v)
}
