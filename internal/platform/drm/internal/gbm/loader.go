package gbm

import (
	"fmt"
	"sync"
)

type LibHandle struct {
	name   string
	handle uintptr
}

var (
	libMu      sync.Mutex
	libCache   = make(map[string]*LibHandle)
)

func OpenLib(name string) (*LibHandle, error) {
	libMu.Lock()
	defer libMu.Unlock()

	if h, ok := libCache[name]; ok {
		return h, nil
	}

	handle, err := dlopen(name, rtldLazy|rtldGlobal)
	if err != nil {
		return nil, err
	}

	h := &LibHandle{name: name, handle: handle}
	libCache[name] = h
	return h, nil
}

func (h *LibHandle) Sym(sym string) (uintptr, error) {
	addr, err := dlsym(h.handle, sym)
	if err != nil {
		return 0, fmt.Errorf("dlsym %s from %s: %w", sym, h.name, err)
	}
	return addr, nil
}

func (h *LibHandle) MustSym(sym string) uintptr {
	addr, err := h.Sym(sym)
	if err != nil {
		panic(err)
	}
	return addr
}

func (h *LibHandle) Close() {
	libMu.Lock()
	defer libMu.Unlock()
	if _, ok := libCache[h.name]; ok {
		delete(libCache, h.name)
		dlclose(h.handle)
	}
}

type multiError struct {
	errors []error
}

func (m *multiError) Error() string {
	if len(m.errors) == 0 {
		return "no errors"
	}
	msg := m.errors[0].Error()
	for _, e := range m.errors[1:] {
		msg += "; " + e.Error()
	}
	return msg
}

func (m *multiError) add(err error) {
	if err != nil {
		m.errors = append(m.errors, err)
	}
}

func (m *multiError) hasErrors() bool {
	return len(m.errors) > 0
}

func (m *multiError) asError() error {
	if !m.hasErrors() {
		return nil
	}
	return m
}
