package textcodecs

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestCodecInfo(t *testing.T) {
	for _, tc := range []struct {
		name     string
		lengths  []int
		stateful bool
	}{
		{"cns-11643-1992", []int{3}, false},
		{"gb18030-2022", []int{1, 2, 4}, false},
		{"euc-jis-2004", []int{1, 2, 3}, false},
		{"ibm-930_P120-1999", []int{1, 2}, true},
	} {
		info, ok := LookupCodecInfo(tc.name)
		if !ok || !reflect.DeepEqual(info.ByteLengths, tc.lengths) || info.Stateful != tc.stateful {
			t.Errorf("%s: %+v, %v", tc.name, info, ok)
		}
	}
	if _, ok := LookupCodecInfo("missing"); ok {
		t.Fatal("unknown codec accepted")
	}
}

func TestWalkMappings(t *testing.T) {
	for _, tc := range []struct {
		name  string
		opts  MappingOptions
		want  string
		bytes []byte
	}{
		{"ibm-943_P15A-2003", MappingOptions{Length: 2, Start: 0x82a0, End: 0x82a1}, "あ", []byte{0x82, 0xa0}},
		{"ibm-943_P15A-2003", MappingOptions{Length: 1, Context: []byte{0x82}, Prefix: []byte{0xa0}}, "あ", []byte{0xa0}},
		{"ibm-930_P120-1999", MappingOptions{Length: 2, Context: []byte{0x0e}, Start: 0x4481, End: 0x4482}, "あ", []byte{0x44, 0x81}},
		{"euc-jis-2004", MappingOptions{Length: 2, Start: 0xa4f7, End: 0xa4f8}, "か\u309a", []byte{0xa4, 0xf7}},
		{"euc-tw-2014", MappingOptions{Length: 4, Start: 0x8ea4a1a6, End: 0x8ea4a1a7}, "丂", []byte{0x8e, 0xa4, 0xa1, 0xa6}},
	} {
		var got []DecodedMapping
		err := WalkMappings(context.Background(), tc.name, tc.opts, func(m DecodedMapping) bool { got = append(got, m); return true })
		if err != nil || len(got) != 1 || got[0].Text != tc.want || !reflect.DeepEqual(got[0].Bytes, tc.bytes) {
			t.Errorf("%s: %v %v", tc.name, got, err)
		}
	}
	count := 0
	err := WalkMappings(context.Background(), "ibm-943_P15A-2003", MappingOptions{Length: 2, Start: 0x4142, End: 0x4143}, func(m DecodedMapping) bool { count++; return true })
	if err != nil || count != 0 {
		t.Fatalf("adjacent characters: %d %v", count, err)
	}
	count = 0
	err = WalkMappings(context.Background(), "ibm-943_P15A-2003", MappingOptions{Length: 1}, func(m DecodedMapping) bool { count++; return false })
	if err != nil || count != 1 {
		t.Fatalf("early stop: %d %v", count, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err = WalkMappings(ctx, "ibm-943_P15A-2003", MappingOptions{Length: 1}, func(DecodedMapping) bool { return true }); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	if err = WalkMappings(context.Background(), "ibm-943_P15A-2003", MappingOptions{Length: 2, Start: 3, End: 2}, func(DecodedMapping) bool { return true }); err == nil {
		t.Fatal("invalid range accepted")
	}
}
