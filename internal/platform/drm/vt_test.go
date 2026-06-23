package drm

import (
	"reflect"
	"testing"
	"unsafe"
)

func TestVTConstants(t *testing.T) {
	if ioctlKdSetmode != 0x4B3A {
		t.Errorf("ioctlKdSetmode = 0x%x, want 0x4B3A", ioctlKdSetmode)
	}
	if ioctlKdGetmode != 0x4B3B {
		t.Errorf("ioctlKdGetmode = 0x%x, want 0x4B3B", ioctlKdGetmode)
	}
	if ioctlVtGetstate != 0x5603 {
		t.Errorf("ioctlVtGetstate = 0x%x, want 0x5603", ioctlVtGetstate)
	}
	if ioctlVtSetmode != 0x5602 {
		t.Errorf("ioctlVtSetmode = 0x%x, want 0x5602", ioctlVtSetmode)
	}
	if ioctlVtReldis != 0x5605 {
		t.Errorf("ioctlVtReldis = 0x%x, want 0x5605", ioctlVtReldis)
	}
	if ioctlVtAcqdis != 0x5606 {
		t.Errorf("ioctlVtAcqdis = 0x%x, want 0x5606", ioctlVtAcqdis)
	}

	if kdGraphics != 0x03 {
		t.Errorf("kdGraphics = 0x%x, want 0x03", kdGraphics)
	}
	if kdText != 0x00 {
		t.Errorf("kdText = 0x%x, want 0x00", kdText)
	}

	if vtAuto != 0x00 {
		t.Errorf("vtAuto = 0x%x, want 0x00", vtAuto)
	}
	if vtProcess != 0x01 {
		t.Errorf("vtProcess = 0x%x, want 0x01", vtProcess)
	}
}

func TestVTModeSize(t *testing.T) {
	typ := reflect.TypeOf(vtMode{})
	sz := typ.Size()
	if sz != 8 {
		t.Errorf("vtMode size = %d, want 8", sz)
	}
}

func TestVTModeLayout(t *testing.T) {
	typ := reflect.TypeOf(vtMode{})

	checkOffset(t, typ, "Mode", 0)
	checkOffset(t, typ, "Waitv", 1)
	checkOffset(t, typ, "Relsig", 2)
	checkOffset(t, typ, "Acqsig", 4)
	checkOffset(t, typ, "Frsig", 6)
}

func checkOffset(t *testing.T, typ reflect.Type, field string, expected uintptr) {
	t.Helper()
	f, ok := typ.FieldByName(field)
	if !ok {
		t.Errorf("vtMode has no field %s", field)
		return
	}
	if f.Offset != expected {
		t.Errorf("vtMode.%s offset = %d, want %d", field, f.Offset, expected)
	}
}

func TestVTModeIsUnsafeCompatible(t *testing.T) {
	var vm vtMode
	p := unsafe.Pointer(&vm)
	modePtr := (*int8)(p)
	*modePtr = vtProcess
	if vm.Mode != vtProcess {
		t.Errorf("vtMode.Mode not at offset 0")
	}
}
