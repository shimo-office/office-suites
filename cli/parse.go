package main

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

func extractDocx(path string) (string, error) {
	z, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	defer z.Close()
	var f io.ReadCloser
	for _, file := range z.File {
		if file.Name == "word/document.xml" {
			f, err = file.Open()
			break
		}
	}
	if f == nil {
		return "", fmt.Errorf("docx 缺少 word/document.xml")
	}
	defer f.Close()
	dec := xml.NewDecoder(f)
	var paras []string
	var cur strings.Builder
	inP := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "p" {
				inP = true
				cur.Reset()
			}
		case xml.CharData:
			if inP {
				cur.Write([]byte(t))
			}
		case xml.EndElement:
			if t.Name.Local == "p" && inP {
				if s := strings.TrimSpace(cur.String()); s != "" {
					paras = append(paras, s)
				}
				inP = false
			}
		}
	}
	return strings.Join(paras, "\n"), nil
}

func extractPptx(path string) (string, error) {
	z, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	defer z.Close()
	re := regexp.MustCompile(`^ppt/slides/slide(\d+)\.xml$`)
	type slide struct {
		n int
		f *zip.File
	}
	var slides []slide
	for _, f := range z.File {
		if m := re.FindStringSubmatch(f.Name); len(m) == 2 {
			n, _ := strconv.Atoi(m[1])
			slides = append(slides, slide{n, f})
		}
	}
	sort.Slice(slides, func(i, j int) bool { return slides[i].n < slides[j].n })
	parts := []string{}
	for i, s := range slides {
		rc, err := s.f.Open()
		if err != nil {
			return "", err
		}
		dec := xml.NewDecoder(rc)
		var texts []string
		for {
			tok, err := dec.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				rc.Close()
				return "", err
			}
			if t, ok := tok.(xml.CharData); ok {
				if x := strings.TrimSpace(string(t)); x != "" {
					texts = append(texts, x)
				}
			}
		}
		rc.Close()
		parts = append(parts, fmt.Sprintf("## 幻灯片 %d\n%s", i+1, strings.Join(texts, " ")))
	}
	return strings.Join(parts, "\n\n"), nil
}

func extractXlsx(path string) (string, error) {
	z, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	defer z.Close()
	shared := []string{}
	for _, f := range z.File {
		if f.Name != "xl/sharedStrings.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		dec := xml.NewDecoder(rc)
		var cur strings.Builder
		inSI := false
		for {
			tok, err := dec.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				rc.Close()
				return "", err
			}
			switch t := tok.(type) {
			case xml.StartElement:
				if t.Name.Local == "si" {
					inSI = true
					cur.Reset()
				}
			case xml.CharData:
				if inSI {
					cur.Write([]byte(t))
				}
			case xml.EndElement:
				if t.Name.Local == "si" && inSI {
					shared = append(shared, cur.String())
					inSI = false
				}
			}
		}
		rc.Close()
	}
	re := regexp.MustCompile(`^xl/worksheets/sheet(\d+)\.xml$`)
	type sheet struct {
		n int
		f *zip.File
	}
	var sheets []sheet
	for _, f := range z.File {
		if m := re.FindStringSubmatch(f.Name); len(m) == 2 {
			n, _ := strconv.Atoi(m[1])
			sheets = append(sheets, sheet{n, f})
		}
	}
	sort.Slice(sheets, func(i, j int) bool { return sheets[i].n < sheets[j].n })
	parts := []string{}
	for i, s := range sheets {
		rc, err := s.f.Open()
		if err != nil {
			return "", err
		}
		type cell struct {
			ref string
			typ string
			val string
		}
		var rows [][]string
		var row []string
		var c cell
		inCell := false
		dec := xml.NewDecoder(rc)
		for {
			tok, err := dec.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				rc.Close()
				return "", err
			}
			switch t := tok.(type) {
			case xml.StartElement:
				if t.Name.Local == "row" {
					row = nil
				} else if t.Name.Local == "c" {
					c = cell{}
					inCell = true
					for _, a := range t.Attr {
						if a.Name.Local == "t" {
							c.typ = a.Value
						}
					}
				} else if t.Name.Local == "v" && inCell {
					var x string
					if err := dec.DecodeElement(&x, &t); err == nil {
						c.val = x
					}
				}
			case xml.EndElement:
				if t.Name.Local == "c" && inCell {
					val := c.val
					if c.typ == "s" {
						if idx, err := strconv.Atoi(val); err == nil && idx >= 0 && idx < len(shared) {
							val = shared[idx]
						}
					}
					row = append(row, val)
					inCell = false
				} else if t.Name.Local == "row" {
					if len(row) > 0 {
						rows = append(rows, row)
					}
				}
			}
		}
		rc.Close()
		lines := []string{}
		for _, r := range rows {
			nonEmpty := false
			for _, v := range r {
				if strings.TrimSpace(v) != "" {
					nonEmpty = true
				}
			}
			if nonEmpty {
				lines = append(lines, strings.Join(r, "\t"))
			}
		}
		parts = append(parts, fmt.Sprintf("## Sheet %d\n%s", i+1, strings.Join(lines, "\n")))
	}
	return strings.Join(parts, "\n\n"), nil
}
