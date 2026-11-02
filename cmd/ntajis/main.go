// ntajis converts the National Tax Agency's JIS Reduction Map XLSX into UCM.
package main

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".ntajis-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Chmod(0644); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
func main() {
	input := flag.String("input", "", "National Tax Agency XLSX file")
	output := flag.String("output", "data/nta/nta-jis-x-0213-euc.ucm", "generated UCM file")
	sjisOutput := flag.String("sjis-output", "data/nta/nta-jis-x-0213-sjis.ucm", "generated Shift_JIS UCM file")
	report := flag.String("report", "data/nta/comparison.md", "comparison summary")
	details := flag.String("details", "data/nta/nta-jis-comparison.tsv", "comparison details")
	expected := flag.String("sha256", "", "require this spreadsheet SHA-256")
	flag.Parse()
	if *input == "" || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: ntajis -input workbook.xlsx [options]")
		flag.PrintDefaults()
		os.Exit(2)
	}
	raw, err := os.ReadFile(*input)
	if err == nil {
		hash := fmt.Sprintf("%x", sha256.Sum256(raw))
		if *expected != "" && hash != *expected {
			fmt.Fprintf(os.Stderr, "spreadsheet SHA-256 is %s, expected %s\n", hash, *expected)
			os.Exit(1)
		}
		var rows []sheetRow
		rows, err = readSheet(raw)
		if err == nil {
			var mappings []mapping
			mappings, err = parseMappings(rows)
			if err == nil {
				var outputData []byte
				outputData, err = renderUCM(mappings, hash, false)
				var sjisData []byte
				if err == nil {
					sjisData, err = renderUCM(mappings, hash, true)
				}
				var reportData, detailData []byte
				if err == nil {
					reportData, detailData, err = comparisonFiles(mappings, hash)
				}
				if err == nil {
					err = writeFile(*output, outputData)
				}
				if err == nil {
					err = writeFile(*sjisOutput, sjisData)
				}
				if err == nil {
					err = writeFile(*report, reportData)
				}
				if err == nil {
					err = writeFile(*details, detailData)
				}
				if err == nil {
					fmt.Fprintf(os.Stderr, "wrote %d JIS mappings to %s and %s\n", len(mappings), *output, *sjisOutput)
				}
			}
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
