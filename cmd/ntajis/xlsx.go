package main

// The workbook reader implements the subset of OOXML needed by the National
// Tax Agency's spreadsheet. It reads the named sheet, not a hard-coded index.
import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
)

const sheetName = "JIS縮退マップ"

type xmlText struct {
	Text string `xml:"t"`
	Runs []struct {
		Text string `xml:"t"`
	} `xml:"r"`
}

func (x xmlText) value() string {
	var b strings.Builder
	b.WriteString(x.Text)
	for _, r := range x.Runs {
		b.WriteString(r.Text)
	}
	return b.String()
}

type workbookXML struct {
	Sheets []struct {
		Name string `xml:"name,attr"`
		ID   string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
	} `xml:"sheets>sheet"`
}
type relationshipsXML struct {
	Items []struct {
		ID     string `xml:"Id,attr"`
		Target string `xml:"Target,attr"`
		Mode   string `xml:"TargetMode,attr"`
		Type   string `xml:"Type,attr"`
	} `xml:"Relationship"`
}
type sharedXML struct {
	Items []xmlText `xml:"si"`
}
type sheetXML struct {
	Rows []struct {
		Number int `xml:"r,attr"`
		Cells  []struct {
			Ref    string  `xml:"r,attr"`
			Type   string  `xml:"t,attr"`
			Value  string  `xml:"v"`
			Inline xmlText `xml:"is"`
		} `xml:"c"`
	} `xml:"sheetData>row"`
}
type sheetRow struct {
	number int
	cells  [17]string
}

func readZIPXML(files map[string]*zip.File, name string, result any) error {
	f, ok := files[name]
	if !ok {
		return fmt.Errorf("workbook is missing %s", name)
	}
	if f.UncompressedSize64 > 64<<20 {
		return fmt.Errorf("workbook part %s is too large", name)
	}
	r, err := f.Open()
	if err != nil {
		return err
	}
	defer r.Close()
	if err := xml.NewDecoder(io.LimitReader(r, 64<<20+1)).Decode(result); err != nil {
		return fmt.Errorf("parse %s: %w", name, err)
	}
	return nil
}
func cellColumn(ref string) (int, error) {
	n, i := 0, 0
	for i < len(ref) && ref[i] >= 'A' && ref[i] <= 'Z' {
		n = n*26 + int(ref[i]-'A'+1)
		i++
	}
	if n == 0 || i == len(ref) {
		return 0, fmt.Errorf("invalid cell reference %q", ref)
	}
	for ; i < len(ref); i++ {
		if ref[i] < '0' || ref[i] > '9' {
			return 0, fmt.Errorf("invalid cell reference %q", ref)
		}
	}
	return n - 1, nil
}
func readSheet(data []byte) ([]sheetRow, error) {
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	files := make(map[string]*zip.File, len(z.File))
	for _, f := range z.File {
		if _, ok := files[f.Name]; ok {
			return nil, fmt.Errorf("duplicate workbook part %s", f.Name)
		}
		files[f.Name] = f
	}
	var book workbookXML
	if err := readZIPXML(files, "xl/workbook.xml", &book); err != nil {
		return nil, err
	}
	id := ""
	for _, s := range book.Sheets {
		if s.Name == sheetName {
			if id != "" {
				return nil, fmt.Errorf("duplicate sheet %s", sheetName)
			}
			id = s.ID
		}
	}
	if id == "" {
		return nil, fmt.Errorf("sheet %q not found", sheetName)
	}
	var rels relationshipsXML
	if err := readZIPXML(files, "xl/_rels/workbook.xml.rels", &rels); err != nil {
		return nil, err
	}
	target := ""
	for _, rel := range rels.Items {
		if rel.ID == id {
			if rel.Mode == "External" || !strings.HasSuffix(rel.Type, "/worksheet") {
				return nil, fmt.Errorf("invalid worksheet relationship %s", id)
			}
			target = path.Clean(path.Join("xl", rel.Target))
			break
		}
	}
	if !strings.HasPrefix(target, "xl/worksheets/") {
		return nil, fmt.Errorf("invalid worksheet target %q", target)
	}
	var shared sharedXML
	if _, ok := files["xl/sharedStrings.xml"]; ok {
		if err := readZIPXML(files, "xl/sharedStrings.xml", &shared); err != nil {
			return nil, err
		}
	}
	var sheet sheetXML
	if err := readZIPXML(files, target, &sheet); err != nil {
		return nil, err
	}
	rows := make([]sheetRow, 0, len(sheet.Rows))
	for _, row := range sheet.Rows {
		result := sheetRow{number: row.Number}
		var seen [17]bool
		for _, cell := range row.Cells {
			col, err := cellColumn(cell.Ref)
			if err != nil {
				return nil, err
			}
			if col >= len(result.cells) {
				continue
			}
			if seen[col] {
				return nil, fmt.Errorf("duplicate cell %s", cell.Ref)
			}
			seen[col] = true
			switch cell.Type {
			case "s":
				if cell.Value == "" {
					continue
				}
				index, err := strconv.Atoi(cell.Value)
				if err != nil || index < 0 || index >= len(shared.Items) {
					return nil, fmt.Errorf("invalid shared string at %s", cell.Ref)
				}
				result.cells[col] = shared.Items[index].value()
			case "inlineStr":
				result.cells[col] = cell.Inline.value()
			case "", "n", "str":
				result.cells[col] = cell.Value
			default:
				return nil, fmt.Errorf("unsupported cell type %q at %s", cell.Type, cell.Ref)
			}
		}
		rows = append(rows, result)
	}
	return rows, nil
}
