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
		inText := false
		// Slide XML mixes可见正文 with animation metadata. Timing nodes carry
		// values such as style.visibility as character data, so only text runs
		// outside the timing tree are document content. Tracking the element
		// path keeps this independent of namespace prefixes: fixtures and strict
		// OOXML use different URIs for the same vocabulary.
		depth, timingDepth := 0, 0
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
				depth++
				if t.Name.Local == "timing" && timingDepth == 0 {
					timingDepth = depth
				}
				inText = timingDepth == 0 && t.Name.Local == "t"
			case xml.CharData:
				if inText {
					if x := strings.TrimSpace(string(t)); x != "" {
						texts = append(texts, x)
					}
				}
			case xml.EndElement:
				if t.Name.Local == "timing" && timingDepth == depth {
					timingDepth = 0
				}
				if t.Name.Local == "t" {
					inText = false
				}
				depth--
			}
		}
		rc.Close()
		parts = append(parts, fmt.Sprintf("## 幻灯片 %d\n%s", i+1, strings.Join(texts, " ")))
	}
	return strings.Join(parts, "\n\n"), nil
}

const xlsxTextStrictNS = "http://purl.oclc.org/ooxml/spreadsheetml/main"
const xlsxTextStrictRelNS = "http://purl.oclc.org/ooxml/officeDocument/relationships"

// Text extraction accepts the two published XLSX namespace families. Action
// verification keeps its existing, separate artifact validation contract.
func xlsxTextPart(entries map[string]*zip.File, name, root string) (*artifactNode, error) {
	data, err := zipEntryBytes(entries[name])
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	n, err := parseArtifactXML(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if n.Name.Local != root || (n.Name.Space != artifactSheetNS && n.Name.Space != xlsxTextStrictNS) {
		return nil, fmt.Errorf("%s: 不是受支持的 XLSX %s 根节点", name, root)
	}
	return n, nil
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
	entries := make(map[string]*zip.File, len(z.File))
	for _, f := range z.File {
		if _, exists := entries[f.Name]; exists {
			return "", fmt.Errorf("工作簿包含重复文件 %s", f.Name)
		}
		entries[f.Name] = f
	}
	root, err := xlsxTextPart(entries, "xl/workbook.xml", "workbook")
	if err != nil {
		return "", err
	}
	rels, err := artifactRelations(entries, "xl/_rels/workbook.xml.rels", "xl")
	if err != nil {
		return "", err
	}
	list := root.child("sheets")
	if list == nil || len(list.children("sheet")) == 0 {
		return "", fmt.Errorf("工作簿没有工作表")
	}
	relNS := artifactOfficeRelNS
	if root.Name.Space == xlsxTextStrictNS {
		relNS = xlsxTextStrictRelNS
	}
	type xlsxTextSheet struct {
		file *zip.File
		name string
	}
	var sheets []xlsxTextSheet
	for _, n := range list.children("sheet") {
		id := ""
		for _, attr := range n.Attr {
			if attr.Name.Space == relNS && attr.Name.Local == "id" {
				id = attr.Value
			}
		}
		rel, ok := rels[id]
		if !ok || (rel.Kind != relNS+"/worksheet" && rel.Kind != relNS+"/chartsheet") {
			return "", fmt.Errorf("工作表 %s 的关系不存在或类型错误", n.attr("name"))
		}
		partType := "worksheet"
		if rel.Kind == relNS+"/chartsheet" {
			partType = "chartsheet"
		}
		part, err := xlsxTextPart(entries, rel.Target, partType)
		if err != nil {
			return "", err
		}
		if part.Name.Space != root.Name.Space {
			return "", fmt.Errorf("工作表 %s 的命名空间与工作簿不一致", n.attr("name"))
		}
		if partType == "worksheet" {
			sheets = append(sheets, xlsxTextSheet{file: entries[rel.Target], name: n.attr("name")})
		}
	}
	parts := []string{}
	for i, s := range sheets {
		rc, err := s.file.Open()
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
				} else if inCell && (t.Name.Local == "v" || (c.typ == "inlineStr" && t.Name.Local == "t")) {
					var x string
					if err := dec.DecodeElement(&x, &t); err != nil {
						rc.Close()
						return "", err
					}
					if c.typ == "inlineStr" {
						c.val += x
					} else {
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
		title := strings.TrimSpace(s.name)
		if title == "" {
			title = fmt.Sprintf("Sheet %d", i+1)
		}
		parts = append(parts, fmt.Sprintf("## %s\n%s", title, strings.Join(lines, "\n")))
	}
	return strings.Join(parts, "\n\n"), nil
}
