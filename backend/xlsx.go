package main

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func loadWorkbookTasks(path string) ([]workbookTask, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("xlsx file not found at %s: %w", path, err)
	}

	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer zr.Close()

	files := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		files[f.Name] = f
	}

	shared, _ := readSharedStrings(files["xl/sharedStrings.xml"])
	sheetPath, err := workbookSheetPath(files["xl/workbook.xml"], files["xl/_rels/workbook.xml.rels"])
	if err != nil {
		return nil, err
	}
	sheetRows, err := readSheetRows(files[sheetPath], shared)
	if err != nil {
		return nil, err
	}

	tasks := make([]workbookTask, 0, len(sheetRows))
	for _, row := range sheetRows {
		if row["A"] == "" || row["A"] == "NO" || row["A"] == "MONITORING PEKERJAAN" {
			continue
		}
		no, err := strconv.Atoi(strings.TrimSpace(row["A"]))
		if err != nil {
			continue
		}
		status := normalizeStatus(row["E"])
		tasks = append(tasks, workbookTask{
			No:          no,
			PageSection: strings.TrimSpace(row["B"]),
			Component:   strings.TrimSpace(row["C"]),
			Issue:       strings.TrimSpace(row["D"]),
			Status:      status,
		})
	}
	return tasks, nil
}

func workbookSheetPath(workbookFile, relsFile *zip.File) (string, error) {
	if workbookFile == nil || relsFile == nil {
		return "", fmt.Errorf("workbook metadata missing")
	}

	type workbook struct {
		Sheets struct {
			Sheets []struct {
				Name string `xml:"name,attr"`
				RID  string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
			} `xml:"sheet"`
		} `xml:"sheets"`
	}
	type rels struct {
		Relationships []struct {
			ID     string `xml:"Id,attr"`
			Target string `xml:"Target,attr"`
		} `xml:"Relationship"`
	}

	var wb workbook
	if err := decodeXMLZipFile(workbookFile, &wb); err != nil {
		return "", err
	}
	var rs rels
	if err := decodeXMLZipFile(relsFile, &rs); err != nil {
		return "", err
	}

	var firstRID string
	for _, sheet := range wb.Sheets.Sheets {
		if sheet.Name != "" {
			firstRID = sheet.RID
			break
		}
	}
	if firstRID == "" {
		return "", fmt.Errorf("no sheets found")
	}

	for _, rel := range rs.Relationships {
		if rel.ID == firstRID {
			target := filepath.Clean(filepath.Join("xl", rel.Target))
			if !strings.HasPrefix(target, "xl/") {
				target = "xl/" + strings.TrimPrefix(target, "./")
			}
			return target, nil
		}
	}
	return "", fmt.Errorf("sheet relationship not found")
}

func readSharedStrings(file *zip.File) ([]string, error) {
	if file == nil {
		return nil, nil
	}
	rc, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	dec := xml.NewDecoder(rc)
	out := make([]string, 0, 128)
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		start, ok := tok.(xml.StartElement)
		if !ok || start.Name.Local != "si" {
			continue
		}

		var text strings.Builder
		for {
			tok, err := dec.Token()
			if err != nil {
				return nil, err
			}
			switch el := tok.(type) {
			case xml.StartElement:
				if el.Name.Local == "t" {
					var value string
					if err := dec.DecodeElement(&value, &el); err != nil {
						return nil, err
					}
					text.WriteString(value)
				}
			case xml.EndElement:
				if el.Name.Local == "si" {
					out = append(out, strings.TrimSpace(text.String()))
					goto nextSharedItem
				}
			}
		}
	nextSharedItem:
	}
	return out, nil
}

func readSheetRows(file *zip.File, shared []string) ([]map[string]string, error) {
	if file == nil {
		return nil, fmt.Errorf("sheet file missing")
	}
	type cell struct {
		Ref string `xml:"r,attr"`
		T   string `xml:"t,attr"`
		V   string `xml:"v"`
		Is  struct {
			Text string `xml:",chardata"`
		} `xml:"is"`
	}
	type row struct {
		Cells []cell `xml:"c"`
	}
	type sheetData struct {
		Rows []row `xml:"sheetData>row"`
	}

	var data sheetData
	if err := decodeXMLZipFile(file, &data); err != nil {
		return nil, err
	}

	rows := make([]map[string]string, 0, len(data.Rows))
	for _, r := range data.Rows {
		record := map[string]string{}
		for _, c := range r.Cells {
			col := cellCol(c.Ref)
			switch c.T {
			case "s":
				idx, err := strconv.Atoi(c.V)
				if err != nil || idx < 0 || idx >= len(shared) {
					continue
				}
				record[col] = shared[idx]
			case "inlineStr":
				record[col] = strings.TrimSpace(c.Is.Text)
			default:
				record[col] = strings.TrimSpace(c.V)
			}
		}
		rows = append(rows, record)
	}
	return rows, nil
}

func decodeXMLZipFile(file *zip.File, out any) error {
	rc, err := file.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return err
	}
	return xml.Unmarshal(data, out)
}

func cellCol(ref string) string {
	i := 0
	for i < len(ref) && ref[i] >= 'A' && ref[i] <= 'Z' {
		i++
	}
	return ref[:i]
}

func normalizeStatus(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	switch s {
	case "", "todo", "to do", "belum", "belum dikerjakan":
		return "todo"
	case "in_progress", "in progress", "berjalan", "progress":
		return "in_progress"
	case "blocked", "block", "blocked ":
		return "blocked"
	case "done", "selesai", "complete", "completed":
		return "done"
	default:
		return "todo"
	}
}
