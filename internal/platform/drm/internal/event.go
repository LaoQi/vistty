package internal

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

func ReadEvent(fd int) (*Event, error) {
	buf := make([]byte, 4096)
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

	ev := &Event{
		Type: uint8(evType),
	}

	if evType == EventVBlank || evType == EventFlipComplete {
		if evLen >= 32 {
			ev.Sequence = binary.LittleEndian.Uint32(buf[24:28])
			ev.TVSec = binary.LittleEndian.Uint32(buf[16:20])
			ev.TVUsec = binary.LittleEndian.Uint32(buf[20:24])
			ev.CrtcID = binary.LittleEndian.Uint32(buf[28:32])
		}
	}

	return ev, nil
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
