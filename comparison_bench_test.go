package textcodecs

import (
	"bytes"
	"testing"
)

// BenchmarkJapaneseCodecs compares the same text across codec families.
func BenchmarkJapaneseCodecs(b *testing.B) {
	const text = "日本語の文章を変換します。半角ｶﾅもあります。"
	src := bytes.Repeat([]byte(text), 256)
	chars := len([]rune(text)) * 256
	for _, name := range []string{"ibm-943_P15A-2003", "euc-jp-2007"} {
		codec, ok := Open(name)
		if !ok {
			b.Fatal(name)
		}
		dst := make([]byte, len(src)*3)
		n, _, err := codec.Encode(dst, src, true)
		if err != nil {
			b.Fatal(err)
		}
		encoded := append([]byte(nil), dst[:n]...)
		b.Run(name+"/encode", func(b *testing.B) {
			b.SetBytes(int64(len(src)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, _, err := codec.Encode(dst, src, true); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(b.N)*float64(chars)/b.Elapsed().Seconds()/1e6, "Mchar/s")
		})
		b.Run(name+"/decode", func(b *testing.B) {
			b.SetBytes(int64(len(encoded)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, _, err := codec.Decode(dst, encoded, true); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(b.N)*float64(chars)/b.Elapsed().Seconds()/1e6, "Mchar/s")
		})
	}
}
