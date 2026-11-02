// Command codec-browser serves a local interactive view of the codec registry.
package main

import (
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"textcodecs"
)

//go:embed static/*
var assets embed.FS

type mapping struct {
	Bytes      string   `json:"bytes"`
	Text       string   `json:"text,omitempty"`
	Codepoints []string `json:"codepoints,omitempty"`
	Names      []string `json:"names,omitempty"`
	Status     string   `json:"status"`
	ShiftTo    string   `json:"shiftTo,omitempty"`
	Expandable bool     `json:"expandable,omitempty"`
}

func mappings(name, prefix string, atomicWidth int) ([]mapping, error) {
	codec, ok := textcodecs.Open(name)
	if !ok {
		return nil, fmt.Errorf("unknown codec")
	}
	prefix = strings.ReplaceAll(strings.ReplaceAll(prefix, " ", ""), ":", "")
	if len(prefix) > 14 {
		return nil, fmt.Errorf("prefix must be at most 7 bytes")
	}
	p, err := hex.DecodeString(prefix)
	if err != nil {
		return nil, fmt.Errorf("prefix must contain complete hexadecimal bytes")
	}
	_, stateful := textcodecs.OpenStateful(name)
	if !stateful {
		_, stateful = textcodecs.OpenWideShift(name)
	}
	// Reject a table sequence when any proper prefix already produces a
	// character; that would concatenate shorter mappings.
	multiple := false
	if atomicWidth > 1 {
		if atomicWidth > 4 || len(p) < atomicWidth-1 {
			return nil, fmt.Errorf("invalid sequence width")
		}
		dst := make([]byte, 128)
		contextLen := len(p) - (atomicWidth - 1)
		codec.Reset()
		baseline, _, _ := codec.Decode(dst, p[:contextLen], false)
		for length := 1; length < atomicWidth; length++ {
			codec.Reset()
			output, _, _ := codec.Decode(dst, p[:contextLen+length], false)
			if output > baseline {
				multiple = true
				break
			}
		}
	}
	result := make([]mapping, 256)
	for i := range result {
		input := append(append([]byte(nil), p...), byte(i))
		codec.Reset()
		dst := make([]byte, 128)
		n, consumed, err := codec.Decode(dst, input, true)
		m := mapping{Bytes: strings.ToUpper(hex.EncodeToString(input)), Status: "unmapped"}
		codec.Reset()
		streamDst := make([]byte, 128)
		_, _, streamErr := codec.Decode(streamDst, input, false)
		m.Expandable = errors.Is(streamErr, textcodecs.ErrShortSrc)
		if err == nil && consumed == len(input) && n > 0 && utf8.Valid(dst[:n]) {
			m.Text = string(dst[:n])
			m.Status = "mapped"
			for _, r := range m.Text {
				m.Codepoints = append(m.Codepoints, fmt.Sprintf("U+%04X", r))
				m.Names = append(m.Names, unicodeName(r))
			}
		} else if err == nil && consumed == len(input) {
			m.Status = "no output"
		}
		if multiple && m.Status == "mapped" {
			m.Status = "multiple characters"
			m.Text = ""
			m.Codepoints = nil
			m.Names = nil
		}
		if stateful && len(p) == 0 && i == 0x0e {
			m.Status, m.ShiftTo = "shift out", "0E"
		} else if stateful && len(p) == 1 && p[0] == 0x0e && i == 0x0f {
			m.Status, m.ShiftTo = "shift in", "initial"
		}
		result[i] = m
	}
	return result, nil
}

func handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/codecs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(textcodecs.EncodingNames())
	})
	mux.HandleFunc("/api/codec-info", func(w http.ResponseWriter, r *http.Request) {
		info := make(map[string]textcodecs.CodecInfo)
		for _, name := range textcodecs.EncodingNames() {
			if codec, ok := textcodecs.LookupCodecInfo(name); ok {
				info[strings.ToLower(name)] = codec
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(info)
	})
	mux.HandleFunc("/api/single-byte-codecs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(textcodecs.SingleByteNames())
	})
	mux.HandleFunc("/api/minimap", func(w http.ResponseWriter, r *http.Request) {
		width, err := strconv.Atoi(r.URL.Query().Get("width"))
		if err != nil {
			http.Error(w, "invalid sequence width", http.StatusBadRequest)
			return
		}
		cells, err := minimap(r.Context(), r.URL.Query().Get("codec"), r.URL.Query().Get("prefix"), width)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cells)
	})
	mux.HandleFunc("/api/mappings", func(w http.ResponseWriter, r *http.Request) {
		atomic := 0
		if r.URL.Query().Get("atomic") != "" {
			var err error
			atomic, err = strconv.Atoi(r.URL.Query().Get("atomic"))
			if err != nil || atomic < 2 || atomic > 4 {
				http.Error(w, "invalid sequence width", http.StatusBadRequest)
				return
			}
		}
		entries, err := mappings(r.URL.Query().Get("codec"), r.URL.Query().Get("prefix"), atomic)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(entries)
	})
	static, _ := fs.Sub(assets, "static")
	mux.Handle("/", http.FileServer(http.FS(static)))
	return mux
}

func main() {
	addr := flag.String("listen", "127.0.0.1:8080", "HTTP listen address")
	flag.Parse()
	log.Printf("Codec browser listening at http://%s", *addr)
	log.Fatal(http.ListenAndServe(*addr, handler()))
}
