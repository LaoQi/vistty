package drm

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"unsafe"
)

const (
	ioctlTiocGwinsz = 0x5413
	ioctlKdSetmode  = 0x4B3A
	ioctlKdGetmode  = 0x4B3B
	ioctlVtGetstate = 0x5603
	ioctlVtSetmode  = 0x5602
	ioctlVtReldis   = 0x5605
	ioctlVtAcqdis   = 0x5606

	kdGraphics = 0x03
	kdText     = 0x00

	vtAuto   = 0x00
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

func newVTManager(callbacks VTCallbacks) (*VTManager, error) {
	ttyFd, err := syscallOpenTty()
	if err != nil {
		return nil, err
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
				vtIoctl(v.ttyFd, ioctlVtReldis, 0)
			case syscall.SIGUSR1:
				vtIoctl(v.ttyFd, ioctlVtAcqdis, 0)
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
		v.SetTextMode()
		syscall.Close(v.ttyFd)
	})
	return nil
}

func syscallOpenTty() (int, error) {
	fd, err := syscall.Open("/dev/tty", syscall.O_RDWR, 0)
	if err != nil {
		return 0, fmt.Errorf("open /dev/tty: %w", err)
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
