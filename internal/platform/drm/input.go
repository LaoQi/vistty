package drm

// DRM 后端仅支持键盘输入（EV_KEY），鼠标事件（EV_REL/EV_ABS）未实现。
// render_loop.go 中 MouseEvents 处理在 DRM 后端不可达（mouseCh 永不产生事件）。

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/LaoQi/vistty/internal/debug"
	"github.com/LaoQi/vistty/internal/platform"
	"github.com/holoplot/go-evdev"
	"golang.org/x/sys/unix"
)

type deviceEntry struct {
	dev  *evdev.InputDevice
	path string
	done chan struct{}
}

type DRMInput struct {
	keyCh     chan platform.KeyEvent
	mouseCh   chan platform.MouseEvent
	devices   map[string]*deviceEntry
	done      chan struct{}
	closeOnce sync.Once
	mods      platform.Modifiers
	mu        sync.Mutex
	inotifyFd int
	watchDone chan struct{}
	exitFd    int
	ready     chan struct{}
}

func newDRMInput() (*DRMInput, error) {
	i := &DRMInput{
		keyCh:     make(chan platform.KeyEvent, 64),
		mouseCh:   make(chan platform.MouseEvent, 16),
		devices:   make(map[string]*deviceEntry),
		done:      make(chan struct{}),
		inotifyFd: -1,
		watchDone: make(chan struct{}),
		ready:     make(chan struct{}),
	}

	paths, err := evdev.ListDevicePaths()
	if err != nil {
		return nil, fmt.Errorf("list evdev devices: %w", err)
	}

	opened := 0
	for _, p := range paths {
		if err := i.openDevice(p.Path); err != nil {
			if !strings.Contains(err.Error(), "no EV_KEY capability") {
				debug.Warningf("input: open %s failed: %v", p.Path, err)
			} else {
				debug.Debugf("input: skipping %s: %v", p.Path, err)
			}
			continue
		}
		opened++
	}

	if opened == 0 {
		debug.Errorf("input: no input device opened (found %d candidates). Ensure user is in 'input' group or run with sudo: usermod -aG input $USER", len(paths))
	}

	go i.watchLoop()

	return i, nil
}

func (i *DRMInput) openDevice(path string) error {
	dev, err := evdev.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}

	hasKey := false
	for _, t := range dev.CapableTypes() {
		if t == evdev.EV_KEY {
			hasKey = true
			break
		}
	}
	if !hasKey {
		dev.Close()
		return fmt.Errorf("device %s has no EV_KEY capability", path)
	}

	if err := dev.Grab(); err != nil {
		dev.Close()
		return fmt.Errorf("grab %s: %w", path, err)
	}

	e := &deviceEntry{
		dev:  dev,
		path: path,
		done: make(chan struct{}),
	}

	i.mu.Lock()
	i.devices[path] = e
	i.mu.Unlock()

	go i.readLoop(e)

	debug.Debugf("input device connected: %s", path)
	return nil
}

func (i *DRMInput) closeDevice(path string) {
	i.mu.Lock()
	e, ok := i.devices[path]
	if !ok {
		i.mu.Unlock()
		return
	}
	delete(i.devices, path)
	remaining := len(i.devices)
	i.mu.Unlock()

	close(e.done)
	e.dev.Ungrab()
	e.dev.Close()

	if remaining == 0 {
		i.mu.Lock()
		i.mods = 0
		i.mu.Unlock()
	}
}

func (i *DRMInput) handleDeviceLost(e *deviceEntry) {
	i.mu.Lock()
	if cur, ok := i.devices[e.path]; !ok || cur != e {
		i.mu.Unlock()
		return
	}
	delete(i.devices, e.path)
	remaining := len(i.devices)
	i.mu.Unlock()

	e.dev.Ungrab()
	e.dev.Close()

	if remaining == 0 {
		i.mu.Lock()
		i.mods = 0
		i.mu.Unlock()
	}

	debug.Debugf("input device disconnected: %s", e.path)
}

func (i *DRMInput) readLoop(e *deviceEntry) {
	defer i.handleDeviceLost(e)

	for {
		ev, err := e.dev.ReadOne()
		if err != nil {
			select {
			case <-i.done:
				return
			case <-e.done:
				return
			default:
				return
			}
		}

		if ev.Type != evdev.EV_KEY {
			continue
		}
		if ev.Value == 2 {
			continue
		}

		code := uint16(ev.Code)

		i.mu.Lock()
		if mod, ok := platform.LookupModifier(uint32(code)); ok {
			if ev.Value != 0 {
				i.mods |= mod
			} else {
				i.mods &^= mod
			}
			mods := i.mods
			i.mu.Unlock()

			select {
			case i.keyCh <- platform.KeyEvent{
				Rune:  0,
				Code:  code,
				Mods:  mods,
				State: platform.KeyState(ev.Value != 0),
			}:
			case <-i.done:
				return
			case <-e.done:
				return
			}
			continue
		}
		mods := i.mods
		i.mu.Unlock()

		r := platform.FallbackKeyRune(uint32(code), mods)

		select {
		case i.keyCh <- platform.KeyEvent{
			Rune:  r,
			Code:  code,
			Mods:  mods,
			State: platform.KeyState(ev.Value != 0),
		}:
		case <-i.done:
			return
		case <-e.done:
			return
		}
	}
}

func (i *DRMInput) openDeviceWithRetry(path string) {
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(100 * time.Millisecond)
		}
		if err := i.openDevice(path); err != nil {
			if attempt == 2 {
				debug.Warningf("failed to open device %s after 3 attempts: %v", path, err)
			}
			continue
		}
		return
	}
}

func (i *DRMInput) rescanDevices() {
	paths, err := evdev.ListDevicePaths()
	if err != nil {
		debug.Warningf("rescan devices failed: %v", err)
		return
	}

	current := make(map[string]bool, len(paths))
	for _, p := range paths {
		current[p.Path] = true
	}

	i.mu.Lock()
	existing := make(map[string]bool, len(i.devices))
	for path := range i.devices {
		existing[path] = true
	}
	i.mu.Unlock()

	for path := range current {
		if !existing[path] {
			i.openDeviceWithRetry(path)
		}
	}

	for path := range existing {
		if !current[path] {
			i.closeDevice(path)
		}
	}
}

func (i *DRMInput) closeInotifyFdLocked() {
	i.mu.Lock()
	fd := i.inotifyFd
	i.inotifyFd = -1
	i.mu.Unlock()
	if fd >= 0 {
		unix.Close(fd)
	}
}

func (i *DRMInput) closeExitFdLocked() {
	i.mu.Lock()
	fd := i.exitFd
	i.exitFd = -1
	i.mu.Unlock()
	if fd >= 0 {
		unix.Close(fd)
	}
}

func (i *DRMInput) watchLoop() {
	defer close(i.watchDone)

	fd, err := unix.InotifyInit1(unix.IN_NONBLOCK | unix.IN_CLOEXEC)
	if err != nil {
		debug.Warningf("inotify init failed: %v", err)
		return
	}
	i.mu.Lock()
	i.inotifyFd = fd
	i.mu.Unlock()
	defer i.closeInotifyFdLocked()

	wd, err := unix.InotifyAddWatch(fd, "/dev/input", unix.IN_CREATE|unix.IN_DELETE|unix.IN_MOVED_TO)
	if err != nil {
		debug.Warningf("inotify add watch failed: %v", err)
		return
	}
	_ = wd

	epollFd, err := unix.EpollCreate1(unix.EPOLL_CLOEXEC)
	if err != nil {
		debug.Warningf("epoll create failed: %v", err)
		return
	}
	defer unix.Close(epollFd)

	exitFd, err := unix.Eventfd(0, unix.EFD_NONBLOCK|unix.EFD_CLOEXEC)
	if err != nil {
		debug.Warningf("eventfd create failed: %v", err)
		i.closeInotifyFdLocked()
		return
	}
	i.mu.Lock()
	i.exitFd = exitFd
	i.mu.Unlock()
	defer i.closeExitFdLocked()

	inotifyEvent := &unix.EpollEvent{
		Events: unix.EPOLLIN,
		Fd:     int32(fd),
	}
	exitEvent := &unix.EpollEvent{
		Events: unix.EPOLLIN,
		Fd:     int32(exitFd),
	}

	if err := unix.EpollCtl(epollFd, unix.EPOLL_CTL_ADD, fd, inotifyEvent); err != nil {
		debug.Warningf("epoll add inotify failed: %v", err)
		return
	}
	if err := unix.EpollCtl(epollFd, unix.EPOLL_CTL_ADD, exitFd, exitEvent); err != nil {
		debug.Warningf("epoll add eventfd failed: %v", err)
		return
	}

	close(i.ready)

	events := make([]unix.EpollEvent, 8)
	buf := make([]byte, 4096)

	for {
		n, err := unix.EpollWait(epollFd, events, -1)
		if err != nil {
			if err == unix.EINTR {
				continue
			}
			return
		}

		for j := 0; j < n; j++ {
			if int(events[j].Fd) == exitFd {
				return
			}

			if int(events[j].Fd) == fd {
				nr, readErr := unix.Read(fd, buf)
				if readErr != nil {
					continue
				}

				var offset uint32
				for offset < uint32(nr) {
					if offset+uint32(unsafe.Sizeof(unix.InotifyEvent{})) > uint32(nr) {
						break
					}
					hdr := (*unix.InotifyEvent)(unsafe.Pointer(&buf[offset]))
					nameLen := hdr.Len
					offset += uint32(unsafe.Sizeof(unix.InotifyEvent{}))

					var name string
					if nameLen > 0 {
						end := offset + nameLen
						if end > uint32(nr) {
							break
						}
						name = strings.TrimRight(string(buf[offset:end]), "\x00")
						offset = end
					}

					if hdr.Mask&unix.IN_Q_OVERFLOW != 0 {
						i.rescanDevices()
						continue
					}

					if !strings.HasPrefix(name, "event") {
						continue
					}

					fullPath := "/dev/input/" + name

					if hdr.Mask&(unix.IN_CREATE|unix.IN_MOVED_TO) != 0 {
						i.openDeviceWithRetry(fullPath)
					}
					if hdr.Mask&unix.IN_DELETE != 0 {
						i.closeDevice(fullPath)
					}
				}
			}
		}
	}
}

func (i *DRMInput) KeyEvents() <-chan platform.KeyEvent {
	return i.keyCh
}

func (i *DRMInput) MouseEvents() <-chan platform.MouseEvent {
	return i.mouseCh
}

func (i *DRMInput) Close() error {
	i.closeOnce.Do(func() {
		close(i.done)

		select {
		case <-i.ready:
			i.mu.Lock()
			exitFd := i.exitFd
			i.mu.Unlock()
			if exitFd >= 0 {
				buf := make([]byte, 8)
				buf[0] = 1
				_, _ = unix.Write(exitFd, buf)
			}
		case <-i.watchDone:
		}

		<-i.watchDone

		i.mu.Lock()
		inotifyFd := i.inotifyFd
		i.inotifyFd = -1
		devs := i.devices
		i.devices = nil
		i.mu.Unlock()

		if inotifyFd >= 0 {
			unix.Close(inotifyFd)
		}

		for _, e := range devs {
			close(e.done)
			e.dev.Ungrab()
			e.dev.Close()
		}
	})
	return nil
}

var _ platform.InputSource = (*DRMInput)(nil)
