package textcodecs

// Algorithmic ranges from ICU's ucnvmbcs.cpp at the pinned mapping-tree
// revision. Each range is consecutive in Unicode and in GB18030's four-byte
// mixed-radix index. Explicit UCM mappings take precedence over these ranges.
type gb18030Range struct {
	first, last rune
	encoded     uint32
}

var gb18030Ranges = [...]gb18030Range{
	{0x10000, 0x10ffff, 0x90308130}, {0x9fa6, 0xd7ff, 0x82358f33},
	{0x0452, 0x1e3e, 0x8130d330}, {0x1e40, 0x200f, 0x8135f438},
	{0xe865, 0xf92b, 0x8336d030}, {0x2643, 0x2e80, 0x8137a839},
	{0xfa2a, 0xfe2f, 0x84309c38}, {0x3ce1, 0x4055, 0x8231d438},
	{0x361b, 0x3917, 0x8230a633}, {0x49b8, 0x4c76, 0x8234a131},
	{0x4160, 0x4336, 0x8232c937}, {0x478e, 0x4946, 0x8233e838},
	{0x44d7, 0x464b, 0x8233a339}, {0xffe6, 0xffff, 0x8431a234},
}

func gbLinear(key uint32) int {
	a, b, c, d := int(byte(key>>24)), int(byte(key>>16)), int(byte(key>>8)), int(byte(key))
	return (((a-0x81)*10+(b-0x30))*126+(c-0x81))*10 + (d - 0x30)
}
func gbKey(index int) uint32 {
	d := 0x30 + index%10
	index /= 10
	c := 0x81 + index%126
	index /= 126
	b := 0x30 + index%10
	index /= 10
	a := 0x81 + index
	return uint32(a)<<24 | uint32(b)<<16 | uint32(c)<<8 | uint32(d)
}
func gbDecode(key uint32) (rune, bool) {
	index := gbLinear(key)
	for _, r := range gb18030Ranges {
		delta := index - gbLinear(r.encoded)
		if delta >= 0 && delta <= int(r.last-r.first) {
			return r.first + rune(delta), true
		}
	}
	return 0, false
}
func gbEncode(cp rune) (uint32, bool) {
	for _, r := range gb18030Ranges {
		if cp >= r.first && cp <= r.last {
			return gbKey(gbLinear(r.encoded) + int(cp-r.first)), true
		}
	}
	return 0, false
}
