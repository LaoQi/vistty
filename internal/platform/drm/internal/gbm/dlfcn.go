package gbm

import (
	"fmt"
	"os"
	"strings"
	"unsafe"
)

const (
	elfClass64    = 2
	elfDataLE     = 1
	elfTypeDyn    = 3
	elfMachineAmd = 62

	ptLoad    = 1
	ptDynamic = 2

	dtNull     = 0
	dtHash     = 4
	dtStrtab   = 5
	dtSymtab   = 6
	dtStrsz    = 10
	dtSyment   = 11
	dtGNUHash  = 0x6ffffef5

	elfSymSize = 24
)

type elf64Ehdr struct {
	Ident     [16]byte
	Type      uint16
	Machine   uint16
	Version   uint32
	Entry     uint64
	Phoff     uint64
	Shoff     uint64
	Flags     uint32
	Ehsize    uint16
	Phentsize uint16
	Phnum     uint16
	Shentsize uint16
	Shnum     uint16
	Shstrndx  uint16
}

type elf64Phdr struct {
	Type   uint32
	Flags  uint32
	Offset uint64
	Vaddr  uint64
	Paddr  uint64
	Filesz uint64
	Memsz  uint64
	Align  uint64
}

type elf64Dyn struct {
	Tag int64
	Val uint64
}

type elf64Sym struct {
	Name  uint32
	Info  uint8
	Other uint8
	Shndx uint16
	Value uint64
	Size  uint64
}

func readU8(ptr uintptr) uint8   { return *(*uint8)(unsafe.Pointer(ptr)) }
func readU32(ptr uintptr) uint32 { return *(*uint32)(unsafe.Pointer(ptr)) }
func readU64(ptr uintptr) uint64 { return *(*uint64)(unsafe.Pointer(ptr)) }

func readEhdr(base uintptr) *elf64Ehdr {
	return (*elf64Ehdr)(unsafe.Pointer(base))
}

func readPhdr(base uintptr, index int, phoff uint64, phentsize uint16) *elf64Phdr {
	addr := base + uintptr(phoff) + uintptr(index)*uintptr(phentsize)
	return (*elf64Phdr)(unsafe.Pointer(addr))
}

func gnuHash(name string) uint32 {
	h := uint32(5381)
	for i := 0; i < len(name); i++ {
		h = (h << 5) + h + uint32(name[i])
	}
	return h
}

func elfHash(name string) uint32 {
	h := uint32(0)
	for i := 0; i < len(name); i++ {
		h = (h << 4) + uint32(name[i])
		g := h & 0xf0000000
		if g != 0 {
			h ^= g >> 24
		}
		h &^= g
	}
	return h
}

func findLibBase(libPattern string) (uintptr, error) {
	data, err := os.ReadFile("/proc/self/maps")
	if err != nil {
		return 0, fmt.Errorf("read /proc/self/maps: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if !strings.Contains(line, libPattern) {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 6 {
			continue
		}
		addrRange := parts[0]
		idx := strings.Index(addrRange, "-")
		if idx <= 0 {
			continue
		}
		var base uint64
		fmt.Sscanf(addrRange[:idx], "%x", &base)
		if base == 0 {
			continue
		}
		return uintptr(base), nil
	}
	return 0, fmt.Errorf("library %q not found in /proc/self/maps", libPattern)
}

func resolveSymbol(libBase uintptr, name string) (uintptr, error) {
	ehdr := readEhdr(libBase)
	if ehdr.Ident[0] != 0x7f || ehdr.Ident[1] != 'E' || ehdr.Ident[2] != 'L' || ehdr.Ident[3] != 'F' {
		return 0, fmt.Errorf("invalid ELF magic at %x", libBase)
	}

	var loadBias uintptr = libBase
	for i := 0; i < int(ehdr.Phnum); i++ {
		ph := readPhdr(libBase, i, ehdr.Phoff, ehdr.Phentsize)
		if ph.Type == ptLoad {
			loadBias = libBase - uintptr(ph.Vaddr)
			break
		}
	}

	var dynAddr uintptr
	for i := 0; i < int(ehdr.Phnum); i++ {
		ph := readPhdr(libBase, i, ehdr.Phoff, ehdr.Phentsize)
		if ph.Type == ptDynamic {
			dynAddr = loadBias + uintptr(ph.Vaddr)
			break
		}
	}
	if dynAddr == 0 {
		return 0, fmt.Errorf("PT_DYNAMIC not found")
	}

	var symtab, strtab, gnuHashTab, hashTab uintptr
	var syment uint64 = elfSymSize

	for {
		d := (*elf64Dyn)(unsafe.Pointer(dynAddr))
		dynAddr += 16
		switch d.Tag {
		case dtNull:
			goto done
		case dtSymtab:
			symtab = loadBias + uintptr(d.Val)
		case dtStrtab:
			strtab = loadBias + uintptr(d.Val)
		case dtGNUHash:
			gnuHashTab = loadBias + uintptr(d.Val)
		case dtHash:
			hashTab = loadBias + uintptr(d.Val)
		case dtSyment:
			syment = d.Val
		}
	}
done:

	if symtab == 0 || strtab == 0 {
		return 0, fmt.Errorf("symbol table or string table not found")
	}

	if gnuHashTab != 0 {
		addr, err := lookupGNUHash(gnuHashTab, symtab, strtab, syment, name)
		if err == nil && addr != 0 {
			return loadBias + addr, nil
		}
	}

	if hashTab != 0 {
		addr, err := lookupSysVHash(hashTab, symtab, strtab, syment, name)
		if err == nil && addr != 0 {
			return loadBias + addr, nil
		}
	}

	return 0, fmt.Errorf("symbol %q not found", name)
}

func lookupGNUHash(hashTab, symtab, strtab uintptr, syment uint64, name string) (uintptr, error) {
	nbuckets := readU32(hashTab)
	symoffset := readU32(hashTab + 4)
	bloomSize := readU32(hashTab + 8)
	bloomShift := readU32(hashTab + 12)

	bloomOff := hashTab + 16
	bucketsOff := bloomOff + uintptr(bloomSize)*8
	chainOff := bucketsOff + uintptr(nbuckets)*4

	h := gnuHash(name)
	bloomIdx := (uintptr(h) / 64) % uintptr(bloomSize)
	bloomWord := readU64(bloomOff + bloomIdx*8)
	mask1 := uint64(1) << (uintptr(h) % 64)
	mask2 := uint64(1) << ((uintptr(h) >> uint32(bloomShift)) % 64)
	if (bloomWord & (mask1 | mask2)) != (mask1 | mask2) {
		return 0, fmt.Errorf("bloom filter rejected %q", name)
	}

	bucketIdx := uintptr(h) % uintptr(nbuckets)
	symIdx := readU32(bucketsOff + bucketIdx*4)
	if symIdx == 0 {
		return 0, fmt.Errorf("bucket empty for %q", name)
	}

	chainIdx := uintptr(symIdx) - uintptr(symoffset)
	for {
		chainVal := readU32(chainOff + chainIdx*4)
		if (chainVal & 0x7fffffff) == (h & 0x7fffffff) {
			symAddr := symtab + uintptr(symIdx)*uintptr(syment)
			sym := (*elf64Sym)(unsafe.Pointer(symAddr))
			symName := readCString(strtab + uintptr(sym.Name))
			if symName == name {
				return uintptr(sym.Value), nil
			}
		}
		symIdx++
		chainIdx++
		if (chainVal & 0x80000000) != 0 {
			break
		}
	}

	return 0, fmt.Errorf("symbol %q not found in GNU hash", name)
}

func lookupSysVHash(hashTab, symtab, strtab uintptr, syment uint64, name string) (uintptr, error) {
	nbuckets := readU32(hashTab)
	nchain := readU32(hashTab + 4)

	bucketsOff := hashTab + 8
	chainOff := bucketsOff + uintptr(nbuckets)*4

	h := elfHash(name)
	bucketIdx := uintptr(h) % uintptr(nbuckets)
	symIdx := readU32(bucketsOff + bucketIdx*4)

	for symIdx != 0 {
		symAddr := symtab + uintptr(symIdx)*uintptr(syment)
		sym := (*elf64Sym)(unsafe.Pointer(symAddr))
		if symIdx < nchain {
			symName := readCString(strtab + uintptr(sym.Name))
			if symName == name {
				return uintptr(sym.Value), nil
			}
		}
		symIdx = readU32(chainOff + uintptr(symIdx)*4)
	}

	return 0, fmt.Errorf("symbol %q not found in SysV hash", name)
}

func readCString(ptr uintptr) string {
	var sb strings.Builder
	for {
		c := readU8(ptr)
		if c == 0 {
			break
		}
		sb.WriteByte(c)
		ptr++
	}
	return sb.String()
}

var (
	dlSymCache struct {
		dlopen  uintptr
		dlsym   uintptr
		dlclose uintptr
		dlerror uintptr
	}
	dlResolved bool
)

func resolveDlFns() error {
	if dlResolved {
		return nil
	}

	candidates := []string{"libc.so", "libc-", "/libc."}
	var libBase uintptr
	var err error
	for _, pat := range candidates {
		libBase, err = findLibBase(pat)
		if err == nil {
			break
		}
	}
	if libBase == 0 {
		return fmt.Errorf("libc not found in /proc/self/maps (static binary?): %w", err)
	}

	syms := map[string]*uintptr{
		"dlopen":  &dlSymCache.dlopen,
		"dlsym":   &dlSymCache.dlsym,
		"dlclose": &dlSymCache.dlclose,
		"dlerror": &dlSymCache.dlerror,
	}
	for name, ptr := range syms {
		addr, err := resolveSymbol(libBase, name)
		if err != nil {
			return fmt.Errorf("resolve %s: %w", name, err)
		}
		*ptr = addr
	}

	dlResolved = true
	return nil
}

const (
	rtldLazy   = 1
	rtldNow    = 2
	rtldGlobal = 0x100
)

func dlopen(path string, mode int) (uintptr, error) {
	if err := resolveDlFns(); err != nil {
		return 0, err
	}
	cpath, err := syscallBytePtr(path)
	if err != nil {
		return 0, err
	}
	ret := ccall2(dlSymCache.dlopen, uintptr(unsafe.Pointer(cpath)), uintptr(mode))
	if ret == 0 {
		return 0, fmt.Errorf("dlopen(%q) failed", path)
	}
	return ret, nil
}

func dlsym(handle uintptr, sym string) (uintptr, error) {
	if err := resolveDlFns(); err != nil {
		return 0, err
	}
	csym, err := syscallBytePtr(sym)
	if err != nil {
		return 0, err
	}
	ret := ccall2(dlSymCache.dlsym, handle, uintptr(unsafe.Pointer(csym)))
	if ret == 0 {
		return 0, fmt.Errorf("dlsym(%q) failed", sym)
	}
	return ret, nil
}

func dlclose(handle uintptr) {
	if err := resolveDlFns(); err != nil {
		return
	}
	ccall1(dlSymCache.dlclose, handle)
}

func syscallBytePtr(s string) (*byte, error) {
	b := make([]byte, len(s)+1)
	copy(b, s)
	return &b[0], nil
}
