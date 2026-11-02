//go:build arm64 && !purego

package textcodecs

//go:noescape
func copyASCIIBlocks(dst, src *byte, n int) int

func copyASCII(dst, src []byte) int {
	n := len(src)
	if len(dst) < n {
		n = len(dst)
	}
	i := 0
	if n >= 64 {
		i = copyASCIIBlocks(&dst[0], &src[0], n)
	}
	return i + copyASCIIScalar(dst[i:], src[i:])
}
