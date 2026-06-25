#include "textflag.h"

// C function pointer call trampolines for amd64 Linux.
// Sets up System V AMD64 ABI calling convention (RDI/RSI/RDX/RCX/R8/R9)
// and handles 16-byte stack alignment required by the C ABI.
//
// Go maintains 16-byte stack alignment. At function entry (after Go's CALL
// pushes the return address), SP%16==8. We SUBQ $8 to make SP%16==0 before
// the C CALL, so that inside the C function SP%16==8 (correct per SysV ABI).

// func ccall0(fn uintptr) uintptr
TEXT ·ccall0(SB), NOSPLIT, $0-16
	MOVQ fn+0(FP), AX
	SUBQ $8, SP
	CALL AX
	ADDQ $8, SP
	MOVQ AX, ret+8(FP)
	RET

// func ccall1(fn, a1 uintptr) uintptr
TEXT ·ccall1(SB), NOSPLIT, $0-24
	MOVQ fn+0(FP), AX
	MOVQ a1+8(FP), DI
	SUBQ $8, SP
	CALL AX
	ADDQ $8, SP
	MOVQ AX, ret+16(FP)
	RET

// func ccall2(fn, a1, a2 uintptr) uintptr
TEXT ·ccall2(SB), NOSPLIT, $0-32
	MOVQ fn+0(FP), AX
	MOVQ a1+8(FP), DI
	MOVQ a2+16(FP), SI
	SUBQ $8, SP
	CALL AX
	ADDQ $8, SP
	MOVQ AX, ret+24(FP)
	RET

// func ccall3(fn, a1, a2, a3 uintptr) uintptr
TEXT ·ccall3(SB), NOSPLIT, $0-40
	MOVQ fn+0(FP), AX
	MOVQ a1+8(FP), DI
	MOVQ a2+16(FP), SI
	MOVQ a3+24(FP), DX
	SUBQ $8, SP
	CALL AX
	ADDQ $8, SP
	MOVQ AX, ret+32(FP)
	RET

// func ccall4(fn, a1, a2, a3, a4 uintptr) uintptr
TEXT ·ccall4(SB), NOSPLIT, $0-48
	MOVQ fn+0(FP), AX
	MOVQ a1+8(FP), DI
	MOVQ a2+16(FP), SI
	MOVQ a3+24(FP), DX
	MOVQ a4+32(FP), CX
	SUBQ $8, SP
	CALL AX
	ADDQ $8, SP
	MOVQ AX, ret+40(FP)
	RET

// func ccall5(fn, a1, a2, a3, a4, a5 uintptr) uintptr
TEXT ·ccall5(SB), NOSPLIT, $0-56
	MOVQ fn+0(FP), AX
	MOVQ a1+8(FP), DI
	MOVQ a2+16(FP), SI
	MOVQ a3+24(FP), DX
	MOVQ a4+32(FP), CX
	MOVQ a5+40(FP), R8
	SUBQ $8, SP
	CALL AX
	ADDQ $8, SP
	MOVQ AX, ret+48(FP)
	RET

// func ccall6(fn, a1, a2, a3, a4, a5, a6 uintptr) uintptr
TEXT ·ccall6(SB), NOSPLIT, $0-64
	MOVQ fn+0(FP), AX
	MOVQ a1+8(FP), DI
	MOVQ a2+16(FP), SI
	MOVQ a3+24(FP), DX
	MOVQ a4+32(FP), CX
	MOVQ a5+40(FP), R8
	MOVQ a6+48(FP), R9
	SUBQ $8, SP
	CALL AX
	ADDQ $8, SP
	MOVQ AX, ret+56(FP)
	RET
