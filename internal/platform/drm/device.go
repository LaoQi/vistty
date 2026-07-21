package drm

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func OpenCard(idx int) (*os.File, error) {
	return os.OpenFile(fmt.Sprintf("/dev/dri/card%d", idx), os.O_RDWR|syscall.O_CLOEXEC, 0)
}

func OpenRender(idx int) (*os.File, error) {
	return os.OpenFile(fmt.Sprintf("/dev/dri/renderD%d", 128+idx), os.O_RDWR|syscall.O_CLOEXEC, 0)
}

func ListDevices() []string {
	matches, _ := filepath.Glob("/dev/dri/card*")
	return matches
}
