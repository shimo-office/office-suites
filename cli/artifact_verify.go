package main

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	artifactSheetNS     = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
	artifactWordNS      = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
	artifactSlideNS     = "http://schemas.openxmlformats.org/presentationml/2006/main"
	artifactDrawingNS   = "http://schemas.openxmlformats.org/drawingml/2006/main"
	artifactRelNS       = "http://schemas.openxmlformats.org/package/2006/relationships"
	artifactOfficeRelNS = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	artifactContentNS   = "http://schemas.openxmlformats.org/package/2006/content-types"
	artifactMaxPart     = 32 << 20
)

type ArtifactEvidence struct {
	Path    string            `json:"path"`
	Kind    string            `json:"kind"`
	Bytes   int64             `json:"bytes"`
	Entries []string          `json:"entries"`
	Checks  map[string]bool   `json:"checks"`
	Details map[string]string `json:"details,omitempty"`
}

type artifactExpectation struct {
	NumericText bool // Explicit exporter compatibility; the API must independently prove numeric types.
	Values      [][]any
	Range       string
	SheetName   string
	Attrs       map[string]any
	MergeRef    string
}

type artifactNode struct {
	Name     xml.Name
	Attr     []xml.Attr
	Text     string
	Children []*artifactNode
}

func (n *artifactNode) attr(name string) string {
	for _, a := range n.Attr {
		if a.Name.Local == name && a.Name.Space == "" {
			return a.Value
		}
	}
	return ""
}
func (n *artifactNode) child(name string) *artifactNode {
	for _, c := range n.Children {
		if c.Name.Local == name && c.Name.Space == n.Name.Space {
			return c
		}
	}
	return nil
}
func (n *artifactNode) children(name string) []*artifactNode {
	var out []*artifactNode
	for _, c := range n.Children {
		if c.Name.Local == name && c.Name.Space == n.Name.Space {
			out = append(out, c)
		}
	}
	return out
}
func (n *artifactNode) textOf(name string) string {
	if c := n.child(name); c != nil {
		return c.Text
	}
	return ""
}

func parseArtifactXML(data []byte) (*artifactNode, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var root *artifactNode
	var stack []*artifactNode
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			if root == nil || len(stack) != 0 {
				return nil, fmt.Errorf("XML 没有完整根节点")
			}
			return root, nil
		}
		if err != nil {
			return nil, err
		}
		switch v := tok.(type) {
		case xml.StartElement:
			if len(stack) > 256 {
				return nil, fmt.Errorf("XML 嵌套过深")
			}
			seenAttrs := map[xml.Name]bool{}
			for _, a := range v.Attr {
				if seenAttrs[a.Name] {
					return nil, fmt.Errorf("XML 属性重复")
				}
				seenAttrs[a.Name] = true
			}
			n := &artifactNode{Name: v.Name, Attr: v.Attr}
			if len(stack) == 0 {
				if root != nil {
					return nil, fmt.Errorf("XML 存在多个根节点")
				}
				root = n
			} else {
				p := stack[len(stack)-1]
				p.Children = append(p.Children, n)
			}
			stack = append(stack, n)
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) == 0 {
				if strings.TrimSpace(string(v)) != "" {
					return nil, fmt.Errorf("XML 根节点外存在文本")
				}
			} else {
				stack[len(stack)-1].Text += string(v)
			}
		case xml.Directive:
			return nil, fmt.Errorf("不支持 XML 指令/DTD")
		}
	}
}

func artifactPart(entries map[string]*zip.File, name, ns, root string) (*artifactNode, error) {
	b, err := zipEntryBytes(entries[name])
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	n, err := parseArtifactXML(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if n.Name.Local != root || n.Name.Space != ns {
		return nil, fmt.Errorf("%s: 根节点必须是 {%s}%s", name, ns, root)
	}
	return n, nil
}

func openArtifact(filename string) (*zip.ReadCloser, map[string]*zip.File, error) {
	z, err := zip.OpenReader(filename)
	if err != nil {
		return nil, nil, err
	}
	entries := map[string]*zip.File{}
	var total uint64
	for _, f := range z.File {
		if _, exists := entries[f.Name]; exists {
			z.Close()
			return nil, nil, fmt.Errorf("重复压缩包条目 %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			continue
		}
		if f.Name != path.Clean(f.Name) || strings.HasPrefix(f.Name, "/") || strings.HasPrefix(f.Name, "../") || strings.Contains(f.Name, "\\") {
			z.Close()
			return nil, nil, fmt.Errorf("无效压缩包路径 %s", f.Name)
		}
		if f.UncompressedSize64 > artifactMaxPart && (strings.HasSuffix(f.Name, ".xml") || strings.HasSuffix(f.Name, ".rels")) {
			z.Close()
			return nil, nil, fmt.Errorf("XML 条目过大 %s", f.Name)
		}
		if f.UncompressedSize64 > (256<<20)-total {
			z.Close()
			return nil, nil, fmt.Errorf("物证解压大小超过 256 MiB")
		}
		total += f.UncompressedSize64
		entries[f.Name] = f
	}
	return z, entries, nil
}

func verifyArtifact(filename, kind string, expected map[string]any) (ArtifactEvidence, error) {
	return verifyArtifactExpected(filename, kind, artifactExpectation{Attrs: expected})
}

func verifyArtifactExpected(filename, kind string, expected artifactExpectation) (ArtifactEvidence, error) {
	filename, err := filepath.Abs(filename)
	if err != nil {
		return ArtifactEvidence{}, err
	}
	st, err := os.Stat(filename)
	if err != nil {
		return ArtifactEvidence{}, err
	}
	if !st.Mode().IsRegular() || st.Size() == 0 {
		return ArtifactEvidence{}, fmt.Errorf("物证不是非空普通文件")
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "" {
		kind = strings.TrimPrefix(strings.ToLower(filepath.Ext(filename)), ".")
	}
	switch kind {
	case "doc":
		kind = "docx"
	case "sheet":
		kind = "xlsx"
	case "slide":
		kind = "pptx"
	}
	if kind != "docx" && kind != "xlsx" && kind != "pptx" {
		return ArtifactEvidence{}, fmt.Errorf("不支持的物证类型 %q", kind)
	}
	if kind != "xlsx" && (len(expected.Values) > 0 || len(expected.Attrs) > 0 || expected.MergeRef != "" || expected.Range != "" || expected.SheetName != "") {
		return ArtifactEvidence{}, fmt.Errorf("%s 暂不支持动作结果断言；只能检查包结构", kind)
	}
	z, entries, err := openArtifact(filename)
	if err != nil {
		return ArtifactEvidence{}, err
	}
	defer z.Close()
	mainPart := map[string]string{"docx": "word/document.xml", "xlsx": "xl/workbook.xml", "pptx": "ppt/presentation.xml"}[kind]
	if err = validateArtifactPackage(entries, mainPart); err != nil {
		return ArtifactEvidence{}, err
	}
	ev := ArtifactEvidence{Path: filename, Kind: kind, Bytes: st.Size(), Checks: map[string]bool{"zip": true, "package": true}, Details: map[string]string{"scope": "package-structure"}}
	for name := range entries {
		ev.Entries = append(ev.Entries, name)
	}
	sort.Strings(ev.Entries)
	switch kind {
	case "docx":
		doc, e := artifactPart(entries, mainPart, artifactWordNS, "document")
		if e != nil {
			return ArtifactEvidence{}, e
		}
		if doc.child("body") == nil {
			return ArtifactEvidence{}, fmt.Errorf("docx 缺少正文 body")
		}
		if _, e = artifactPart(entries, "word/styles.xml", artifactWordNS, "styles"); e != nil {
			return ArtifactEvidence{}, e
		}
		ev.Checks["document-xml"] = true
		ev.Checks["styles-xml"] = true
		ev.Details["has-text"] = strconv.FormatBool(strings.TrimSpace(artifactText(doc, "t")) != "")
		ev.Details["bold-nodes"] = strconv.Itoa(artifactCount(doc, "b"))
		ev.Details["size-nodes"] = strconv.Itoa(artifactCount(doc, "sz"))
		ev.Details["paragraph-align-nodes"] = strconv.Itoa(artifactCount(doc, "jc"))
	case "xlsx":
		wb, e := readArtifactWorkbook(entries)
		if e != nil {
			return ArtifactEvidence{}, e
		}
		ev.Checks["workbook-xml"] = true
		ev.Checks["styles-xml"] = true
		ev.Checks["sheet-xml"] = true
		ev.Details["sheet-count"] = strconv.Itoa(len(wb.Sheets))
		if e = verifyWorkbookExpectation(wb, expected, &ev); e != nil {
			return ArtifactEvidence{}, e
		}
	case "pptx":
		count, e := verifyArtifactPresentation(entries)
		if e != nil {
			return ArtifactEvidence{}, e
		}
		ev.Checks["slide-xml"] = true
		ev.Checks["theme"] = true
		ev.Details["slide-count"] = strconv.Itoa(count)
		ev.Details["media-count"] = strconv.Itoa(countPrefix(entries, "ppt/media/"))
	}
	return ev, nil
}

func validateArtifactPackage(entries map[string]*zip.File, mainPart string) error {
	ct, err := artifactPart(entries, "[Content_Types].xml", artifactContentNS, "Types")
	if err != nil {
		return err
	}
	found := false
	for _, c := range ct.children("Override") {
		if c.attr("PartName") == "/"+mainPart && c.attr("ContentType") == map[string]string{
			"xl/workbook.xml":      "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml",
			"word/document.xml":    "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml",
			"ppt/presentation.xml": "application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml",
		}[mainPart] {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("[Content_Types].xml 缺少主文档声明 %s", mainPart)
	}
	rels, err := artifactRelations(entries, "_rels/.rels", "")
	if err != nil {
		return err
	}
	for _, r := range rels {
		if r.Kind == artifactOfficeRelNS+"/officeDocument" && r.Target == mainPart {
			return nil
		}
	}
	return fmt.Errorf("包关系没有指向 %s", mainPart)
}

type artifactRelation struct{ Kind, Target string }

func artifactRelations(entries map[string]*zip.File, filename, base string) (map[string]artifactRelation, error) {
	root, err := artifactPart(entries, filename, artifactRelNS, "Relationships")
	if err != nil {
		return nil, err
	}
	out := map[string]artifactRelation{}
	for _, n := range root.children("Relationship") {
		id := n.attr("Id")
		if id == "" {
			return nil, fmt.Errorf("关系缺少 Id")
		}
		if _, exists := out[id]; exists {
			return nil, fmt.Errorf("重复关系 Id %s", id)
		}
		if n.attr("TargetMode") == "External" {
			continue
		}
		target := n.attr("Target")
		if target == "" || strings.ContainsAny(target, "\\?#") {
			return nil, fmt.Errorf("无效关系路径 %s", target)
		}
		if strings.HasPrefix(target, "/") {
			target = strings.TrimPrefix(target, "/")
		} else {
			target = path.Join(base, target)
		}
		target = path.Clean(target)
		if strings.HasPrefix(target, "../") || target == ".." {
			return nil, fmt.Errorf("关系指向包外")
		}
		out[id] = artifactRelation{Kind: n.attr("Type"), Target: target}
	}
	return out, nil
}
func relationID(n *artifactNode) string {
	for _, a := range n.Attr {
		if a.Name.Space == artifactOfficeRelNS && a.Name.Local == "id" {
			return a.Value
		}
	}
	return ""
}

func verifyArtifactPresentation(entries map[string]*zip.File) (int, error) {
	root, err := artifactPart(entries, "ppt/presentation.xml", artifactSlideNS, "presentation")
	if err != nil {
		return 0, err
	}
	rels, err := artifactRelations(entries, "ppt/_rels/presentation.xml.rels", "ppt")
	if err != nil {
		return 0, err
	}
	list := root.child("sldIdLst")
	if list == nil || len(list.children("sldId")) == 0 {
		return 0, fmt.Errorf("pptx 没有幻灯片")
	}
	seen := map[string]bool{}
	for _, n := range list.children("sldId") {
		rel, ok := rels[relationID(n)]
		if !ok || rel.Kind != artifactOfficeRelNS+"/slide" || seen[rel.Target] {
			return 0, fmt.Errorf("幻灯片关系无效或重复")
		}
		slide, e := artifactPart(entries, rel.Target, artifactSlideNS, "sld")
		if e != nil {
			return 0, e
		}
		if slide.child("cSld") == nil || slide.child("cSld").child("spTree") == nil {
			return 0, fmt.Errorf("幻灯片缺少 cSld")
		}
		seen[rel.Target] = true
	}
	themes := 0
	for name := range entries {
		if strings.HasPrefix(name, "ppt/theme/") && strings.HasSuffix(name, ".xml") {
			theme, e := artifactPart(entries, name, artifactDrawingNS, "theme")
			if e != nil {
				return 0, e
			}
			if theme.child("themeElements") == nil {
				return 0, fmt.Errorf("主题缺少 themeElements")
			}
			themes++
		}
	}
	if themes == 0 {
		return 0, fmt.Errorf("pptx 缺少主题")
	}
	return len(seen), nil
}

func zipEntryBytes(f *zip.File) ([]byte, error) {
	if f == nil {
		return nil, fmt.Errorf("压缩包条目不存在")
	}
	if f.UncompressedSize64 > artifactMaxPart {
		return nil, fmt.Errorf("条目超过 32 MiB")
	}
	r, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	b, err := io.ReadAll(io.LimitReader(r, artifactMaxPart+1))
	if err != nil {
		return nil, err
	}
	if len(b) > artifactMaxPart {
		return nil, fmt.Errorf("条目超过 32 MiB")
	}
	return b, nil
}
func artifactText(n *artifactNode, name string) string {
	s := ""
	if n.Name.Local == name {
		s = n.Text
	}
	for _, c := range n.Children {
		s += artifactText(c, name)
	}
	return s
}
func artifactCount(n *artifactNode, name string) int {
	count := 0
	if n.Name.Local == name {
		count++
	}
	for _, c := range n.Children {
		count += artifactCount(c, name)
	}
	return count
}
func countPrefix(entries map[string]*zip.File, prefix string) int {
	n := 0
	for name := range entries {
		if strings.HasPrefix(name, prefix) {
			n++
		}
	}
	return n
}
