package drm

import (
	"encoding/binary"
	"io"
	"syscall"
)

const (
	EventVBlank       = 0x01
	EventFlipComplete = 0x02
)

type Event struct {
	Type     uint8
	Sequence uint32
	TVSec    uint32
	TVUsec   uint32
	CrtcID   uint32
}

const eventBufSize = 4096

// EventReader 是有状态的 DRM 事件读取器。
//
// 一次 syscall.Read 可能从内核返回多个拼接的事件（尤其多屏同时 flip
// 完成时），EventReader 缓存残差字节并逐个解析，确保事件不丢失。
// 无状态的 ReadEvent 会丢弃首个事件之后的字节，导致多屏渲染卡死。
type EventReader struct {
	fd    int
	buf   []byte
	valid int
}

func NewEventReader(fd int) *EventReader {
	return &EventReader{fd: fd, buf: make([]byte, eventBufSize)}
}

// ReadEvent 返回下一个完整事件。
// 若缓存中已有完整事件则直接解析；否则从 fd 读取补充。
// fd 关闭（EOF）或出错时返回相应 error。
func (er *EventReader) ReadEvent() (*Event, error) {
	for {
		if ev, ok := er.tryParse(); ok {
			return ev, nil
		}
		if er.valid >= len(er.buf) {
			return nil, io.ErrShortBuffer
		}
		n, err := readFromFd(er.fd, er.buf[er.valid:])
		if n > 0 {
			er.valid += n
		}
		if err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, io.EOF
		}
	}
}

// tryParse 尝试从缓存头部解析一个完整事件，成功则消费并返回。
func (er *EventReader) tryParse() (*Event, bool) {
	if er.valid < 8 {
		return nil, false
	}
	evType := binary.LittleEndian.Uint32(er.buf[0:4])
	evLen := binary.LittleEndian.Uint32(er.buf[4:8])
	if evLen < 8 || uint32(er.valid) < evLen {
		return nil, false
	}
	ev := parseEvent(er.buf[:evLen], evType, evLen)
	copy(er.buf, er.buf[evLen:er.valid])
	er.valid -= int(evLen)
	return ev, true
}

// parseEvent 解析单个事件字节切片。
func parseEvent(b []byte, evType, evLen uint32) *Event {
	ev := &Event{Type: uint8(evType)}
	if evType == EventVBlank || evType == EventFlipComplete {
		if evLen >= 32 {
			ev.Sequence = binary.LittleEndian.Uint32(b[24:28])
			ev.TVSec = binary.LittleEndian.Uint32(b[16:20])
			ev.TVUsec = binary.LittleEndian.Uint32(b[20:24])
			ev.CrtcID = binary.LittleEndian.Uint32(b[28:32])
		}
	}
	return ev
}

// ReadEvent 是无状态的兼容函数：读取并返回首个事件。
//
// 注意：它会丢弃一次 read 中首个事件之后的剩余字节。新代码应使用
// NewEventReader 以避免多屏场景下的事件丢失。
func ReadEvent(fd int) (*Event, error) {
	buf := make([]byte, eventBufSize)
	n, err := readFromFd(fd, buf)
	if err != nil {
		return nil, err
	}
	if n < 8 {
		return nil, io.ErrUnexpectedEOF
	}
	evType := binary.LittleEndian.Uint32(buf[0:4])
	evLen := binary.LittleEndian.Uint32(buf[4:8])
	if evLen < 8 || uint32(n) < evLen {
		return nil, io.ErrUnexpectedEOF
	}
	return parseEvent(buf[:evLen], evType, evLen), nil
}

func readFromFd(fd int, buf []byte) (int, error) {
	for {
		n, err := syscall.Read(fd, buf)
		if err == syscall.EINTR {
			continue
		}
		return n, err
	}
}
