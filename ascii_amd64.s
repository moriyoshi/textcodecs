//go:build !purego

#include "textflag.h"

// SSE2 is part of the amd64 baseline. Reject a complete block if it contains
// any byte outside the printable ASCII interval.
TEXT ·copyASCIIBlocks(SB), NOSPLIT, $0-32
	MOVQ dst+0(FP), DI
	MOVQ src+8(FP), SI
	MOVQ n+16(FP), CX
	XORQ AX, AX
	MOVQ $0x2020202020202020, DX
	MOVQ DX, X1
	PUNPCKLQDQ X1, X1
	MOVQ $0x7e7e7e7e7e7e7e7e, DX
	MOVQ DX, X2
	PUNPCKLQDQ X2, X2
loop:
	MOVQ CX, DX
	SUBQ AX, DX
	CMPQ DX, $16
	JL done
	MOVOU (SI)(AX*1), X0
	MOVOU X1, X3
	PCMPGTB X0, X3
	MOVOU X0, X4
	PCMPGTB X2, X4
	POR X4, X3
	PMOVMSKB X3, DX
	TESTL DX, DX
	JNE done
	MOVOU X0, (DI)(AX*1)
	ADDQ $16, AX
	JMP loop
done:
	MOVQ AX, ret+24(FP)
	RET
