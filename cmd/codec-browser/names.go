package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"strconv"
	"strings"
	"sync"
)

// Unicode 15.0 character names, generated from Python's unicodedata module.
//
//go:embed unicode-names.txt.gz
var compressedNames []byte

var namesOnce sync.Once
var unicodeNames map[rune]string

func unicodeName(r rune) string {
	namesOnce.Do(func() {
		reader, err := gzip.NewReader(bytes.NewReader(compressedNames))
		if err != nil {
			panic(err)
		}
		defer reader.Close()
		unicodeNames = make(map[rune]string)
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			code, name, ok := strings.Cut(scanner.Text(), ";")
			if !ok {
				panic("invalid Unicode names data")
			}
			value, err := strconv.ParseInt(code, 16, 32)
			if err != nil {
				panic(err)
			}
			unicodeNames[rune(value)] = name
		}
		if err := scanner.Err(); err != nil {
			panic(err)
		}
	})
	if name := unicodeNames[r]; name != "" {
		return name
	}
	return "Name unavailable"
}
