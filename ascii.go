package textcodecs

import "encoding/binary"

// copyASCII copies a leading run of printable ASCII. ICU's CP932 mapping
// permutes three control characters, so arbitrary ASCII is not an identity.
func copyASCIIScalar(dst, src []byte) int {
	n := len(src)
	if len(dst) < n {
		n = len(dst)
	}
	i := 0
	for ; i+8 <= n; i += 8 {
		v := binary.LittleEndian.Uint64(src[i:])
		if (v|(v-0x2020202020202020)|(v+0x0101010101010101))&0x8080808080808080 != 0 {
			break
		}
		binary.LittleEndian.PutUint64(dst[i:], v)
	}
	for i < n && src[i] >= 0x20 && src[i] < 0x7f {
		dst[i] = src[i]
		i++
	}
	return i
}
