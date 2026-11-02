package textcodecs

// icuDefaultIgnorable reports characters that ICU 74.2 consumes without
// output when an encoding has no explicit mapping for them.
func icuDefaultIgnorable(r rune) bool {
	switch {
	case r == 0x00ad, r == 0x034f, r == 0x061c,
		r >= 0x115f && r <= 0x1160,
		r >= 0x17b4 && r <= 0x17b5,
		r >= 0x180b && r <= 0x180f,
		r >= 0x200b && r <= 0x200f,
		r >= 0x202a && r <= 0x202e,
		r >= 0x2060 && r <= 0x206f,
		r == 0x3164, r >= 0xfe00 && r <= 0xfe0f, r == 0xfeff,
		r == 0xffa0, r >= 0xfff0 && r <= 0xfff8,
		r >= 0x1bca0 && r <= 0x1bca3,
		r >= 0x1d173 && r <= 0x1d17a,
		r >= 0xe0000 && r <= 0xe0fff:
		return true
	}
	return false
}
