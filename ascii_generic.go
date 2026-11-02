//go:build (!arm64 && !amd64) || purego

package textcodecs

func copyASCII(dst, src []byte) int { return copyASCIIScalar(dst, src) }
