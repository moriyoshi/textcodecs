package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"

	"textcodecs"
)

const minimapRows = 256

type minimapCell struct {
	Index int    `json:"index"`
	Value uint64 `json:"value"`
}

// Each minimap row aggregates a range of table rows; its 16 columns match the
// table's low hexadecimal digit. Value is an actual mapping in that cell.
func minimap(ctx context.Context, name, prefix string, width int) ([]minimapCell, error) {
	if _, ok := textcodecs.LookupCodecInfo(name); !ok {
		return nil, fmt.Errorf("unknown codec")
	}
	if width < 2 || width > 4 {
		return nil, fmt.Errorf("invalid sequence width")
	}
	prefix = strings.ReplaceAll(strings.ReplaceAll(prefix, " ", ""), ":", "")
	if len(prefix) > 14 {
		return nil, fmt.Errorf("prefix must be at most 7 bytes")
	}
	p, err := hex.DecodeString(prefix)
	if err != nil {
		return nil, fmt.Errorf("prefix must contain complete hexadecimal bytes")
	}
	out := make([]minimapCell, 0)
	size := (uint64(1) << uint(width*8)) / minimapRows
	for row := 0; row < minimapRows; row++ {
		options := textcodecs.MappingOptions{Length: width, Context: p, Start: uint64(row) * size, End: uint64(row+1) * size}
		var seen [16]bool
		found := 0
		err := textcodecs.WalkMappings(ctx, name, options, func(m textcodecs.DecodedMapping) bool {
			col := int(m.Bytes[len(m.Bytes)-1] & 15)
			if !seen[col] {
				seen[col] = true
				found++
				var value uint64
				for _, b := range m.Bytes {
					value = value<<8 | uint64(b)
				}
				out = append(out, minimapCell{Index: row*16 + col, Value: value})
			}
			return found < 16
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}
