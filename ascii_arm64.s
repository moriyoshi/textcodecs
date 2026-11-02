//go:build !purego

#include "textflag.h"

// Copy complete 16-byte blocks of printable ASCII. Stop before the first
// block containing any byte outside [0x20, 0x7e].
TEXT ·copyASCIIBlocks(SB), NOSPLIT, $0-32
	MOVD dst+0(FP), R0
	MOVD src+8(FP), R1
	MOVD n+16(FP), R2
	MOVD $0, R3
	MOVD $0x20, R4
	VDUP R4, V1.B16
	MOVD $0x7e, R4
	VDUP R4, V2.B16
loop:
	SUB R3, R2, R4
	CMP $16, R4
	BLT done
	ADD R3, R1, R5
	VLD1 (R5), [V0.B16]
	VCMHI V0.B16, V1.B16, V3.B16
	VCMHI V2.B16, V0.B16, V4.B16
	VORR V3.B16, V4.B16, V3.B16
	VMOV V3.D[0], R6
	VMOV V3.D[1], R7
	ORR R6, R7, R6
	CBNZ R6, done
	ADD R3, R0, R5
	VST1 [V0.B16], (R5)
	ADD $16, R3
	B loop
done:
	MOVD R3, ret+24(FP)
	RET
