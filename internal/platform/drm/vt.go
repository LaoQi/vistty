package drm

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"unsafe"

	"github.com/LaoQi/vistty/internal/debug"
)

const (
	ioctlTiocGwinsz = 0x5413
	ioctlTiocSctty  = 0x540E
	ioctlKdSetmode  = 0x4B3A
	ioctlKdGetmode  = 0x4B3B
	ioctlVtGetstate = 0x5603
	ioctlVtSetmode  = 0x5602
	ioctlVtReldis   = 0x5605
	ioctlVtAcqdis   = 0x5606

	kdGraphics = 0x03
	kdText     = 0x00

	vtAuto    = 0x00
	vtProcess = 0x01
)

type vtMode struct {
	Mode   int8
	Waitv  int8
	Relsig int16
	Acqsig int16
	Frsig  int16
}

type VTCallbacks struct {
	OnActivate   func()
	OnDeactivate func()
}

type VTManager struct {
	ttyFd     int
	savedMode int8
	sigCh     chan os.Signal
	done      chan struct{}
	callbacks VTCallbacks
	closeOnce sync.Once
	wg        sync.WaitGroup
}

func newVTManager(callbacks VTCallbacks, ttyPath string) (*VTManager, error) {
	ttyFd, err := syscallOpenTty(ttyPath)
	if err != nil {
		debug.Warningf("vt manager: %v; running without VT switch support", err)
		return nil, nil
	}

	v := &VTManager{
		ttyFd:     ttyFd,
		callbacks: callbacks,
		sigCh:     make(chan os.Signal, 2),
		done:      make(chan struct{}),
	}

	var curMode int8
	if err := vtIoctl(ttyFd, ioctlKdGetmode, uintptr(unsafe.Pointer(&curMode))); err == nil {
		v.savedMode = curMode
	}

	signal.Notify(v.sigCh, syscall.SIGUSR1, syscall.SIGUSR2)

	v.wg.Add(1)
	go v.signalLoop()

	return v, nil
}

func (v *VTManager) signalLoop() {
	defer v.wg.Done()
	for {
		select {
		case <-v.done:
			return
		case sig := <-v.sigCh:
			switch sig {
			case syscall.SIGUSR2:
				v.callbacks.OnDeactivate()
				if err := vtIoctl(v.ttyFd, ioctlVtReldis, 0); err != nil {
					debug.Warningf("vt: VT_RELDISP ioctl failed: %v", err)
				}
			case syscall.SIGUSR1:
				if err := vtIoctl(v.ttyFd, ioctlVtAcqdis, 0); err != nil {
					debug.Warningf("vt: VT_ACTIVATE ioctl failed: %v", err)
				}
				v.callbacks.OnActivate()
			}
		}
	}
}

func (v *VTManager) SetGraphicsMode() error {
	if err := vtIoctl(v.ttyFd, ioctlKdSetmode, uintptr(kdGraphics)); err != nil {
		return fmt.Errorf("set kd graphics: %w", err)
	}

	vm := vtMode{
		Mode:   vtProcess,
		Relsig: int16(syscall.SIGUSR2),
		Acqsig: int16(syscall.SIGUSR1),
	}
	if err := vtIoctl(v.ttyFd, ioctlVtSetmode, uintptr(unsafe.Pointer(&vm))); err != nil {
		return fmt.Errorf("set vt process mode: %w", err)
	}

	return nil
}

func (v *VTManager) SetTextMode() error {
	vm := vtMode{Mode: vtAuto}
	if err := vtIoctl(v.ttyFd, ioctlVtSetmode, uintptr(unsafe.Pointer(&vm))); err != nil {
		return fmt.Errorf("set vt auto mode: %w", err)
	}
	if err := vtIoctl(v.ttyFd, ioctlKdSetmode, uintptr(kdText)); err != nil {
		return fmt.Errorf("set kd text mode: %w", err)
	}
	return nil
}

func (v *VTManager) Close() error {
	v.closeOnce.Do(func() {
		close(v.done)
		signal.Stop(v.sigCh)
		v.wg.Wait()
		if err := v.SetTextMode(); err != nil {
			debug.Warningf("vt: SetTextMode on close failed: %v", err)
		}
		syscall.Close(v.ttyFd)
	})
	return nil
}

func syscallOpenTty(ttyPath string) (int, error) {
	target := ttyPath
	if target == "" {
		target = "/dev/tty"
	} else {
		if _, _, errno := syscall.Syscall(syscall.SYS_SETSID, 0, 0, 0); errno != 0 && errno != syscall.EPERM {
			return 0, fmt.Errorf("setsid: %w", errno)
		}
	}

	fd, err := syscall.Open(target, syscall.O_RDWR|syscall.O_CLOEXEC, 0)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", target, err)
	}

	if ttyPath != "" {
		if err := vtIoctl(fd, ioctlTiocSctty, 0); err != nil {
			syscall.Close(fd)
			return 0, fmt.Errorf("set controlling tty %s: %w", target, err)
		}
	}

	return fd, nil
}

func vtIoctl(fd int, req uintptr, arg uintptr) error {
	for {
		_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), req, arg)
		if errno == syscall.EINTR {
			continue
		}
		if errno != 0 {
			return errno
		}
		return nil
	}
}
