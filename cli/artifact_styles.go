package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

func artifactIndexedNode(root *artifactNode, collection, element string, index int) (*artifactNode, error) {
	list := root.child(collection)
	if list == nil {
		return nil, fmt.Errorf("样式缺少 %s", collection)
	}
	nodes := list.children(element)
	if index < 0 || index >= len(nodes) {
		return nil, fmt.Errorf("%s 索引 %d 越界", collection, index)
	}
	return nodes[index], nil
}
func artifactStyleIndex(xf *artifactNode, key string) (int, error) {
	raw := xf.attr(key)
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("样式索引 %s 无效", key)
	}
	return n, nil
}
func artifactStyleComponent(styles, xf *artifactNode, key, apply, collection, element string) (*artifactNode, error) {
	source := xf
	if xf.attr(apply) == "0" || xf.attr(apply) == "false" {
		base, err := artifactStyleIndex(xf, "xfId")
		if err != nil {
			return nil, err
		}
		source, err = artifactIndexedNode(styles, "cellStyleXfs", "xf", base)
		if err != nil {
			return nil, err
		}
	}
	index, err := artifactStyleIndex(source, key)
	if err != nil {
		return nil, err
	}
	return artifactIndexedNode(styles, collection, element, index)
}

func artifactCellAttr(styles *artifactNode, index int, attr string, want any) (bool, error) {
	xf, err := artifactIndexedNode(styles, "cellXfs", "xf", index)
	if err != nil {
		return false, err
	}
	if attr == "background" {
		fill, e := artifactStyleComponent(styles, xf, "fillId", "applyFill", "fills", "fill")
		if e != nil {
			return false, e
		}
		pattern := fill.child("patternFill")
		if pattern == nil || pattern.attr("patternType") != "solid" {
			return false, nil
		}
		return artifactColorMatches(pattern.child("fgColor"), want), nil
	}
	font, err := artifactStyleComponent(styles, xf, "fontId", "applyFont", "fonts", "font")
	if err != nil {
		return false, err
	}
	if attr == "color" {
		return artifactColorMatches(font.child("color"), want), nil
	}
	name := map[string]string{"bold": "b", "italic": "i", "underline": "u"}[attr]
	if name == "" {
		return false, fmt.Errorf("不支持校验格式属性 %s", attr)
	}
	expected, ok := want.(bool)
	if !ok {
		return false, fmt.Errorf("格式属性 %s 必须是 bool", attr)
	}
	n := font.child(name)
	got := false
	if n != nil {
		raw := n.attr("val")
		got = raw != "0" && raw != "false" && raw != "off" && raw != "none"
	}
	return got == expected, nil
}

func artifactColorMatches(n *artifactNode, want any) bool {
	if n == nil || n.attr("theme") != "" || n.attr("indexed") != "" || n.attr("tint") != "" {
		return false
	}
	color, ok := want.(string)
	if !ok {
		return false
	}
	color = strings.ToUpper(strings.TrimPrefix(color, "#"))
	if len(color) != 6 {
		return false
	}
	if _, err := hex.DecodeString(color); err != nil {
		return false
	}
	got := strings.ToUpper(n.attr("rgb"))
	if len(got) == 8 && (got[:2] == "FF" || got[:2] == "00") {
		got = got[2:]
	}
	return got == color
}

// Resolve effective number formatting before fingerprinting. Custom format
// IDs are references, not meanings: exporters may renumber them, and the same
// ID can point to a changed formatCode after a mutation.
func artifactNumberFormatKey(styles, xf *artifactNode) (string, error) {
	source := xf
	if xf.attr("applyNumberFormat") == "0" || xf.attr("applyNumberFormat") == "false" {
		base, err := artifactStyleIndex(xf, "xfId")
		if err != nil {
			return "", err
		}
		source, err = artifactIndexedNode(styles, "cellStyleXfs", "xf", base)
		if err != nil {
			return "", err
		}
	}
	id, err := artifactStyleIndex(source, "numFmtId")
	if err != nil {
		return "", err
	}
	custom := map[int]string{}
	if formats := styles.child("numFmts"); formats != nil {
		for _, n := range formats.children("numFmt") {
			fid, e := strconv.Atoi(n.attr("numFmtId"))
			if e != nil || fid < 0 || n.attr("formatCode") == "" {
				return "", fmt.Errorf("数字格式定义无效")
			}
			if _, exists := custom[fid]; exists {
				return "", fmt.Errorf("数字格式 ID %d 重复", fid)
			}
			custom[fid] = n.attr("formatCode")
		}
	}
	if code, ok := custom[id]; ok {
		return "code:" + code, nil
	}
	// The remaining built-ins include locale-dependent currency/date formats;
	// keep their standard IDs instead of guessing locale-specific formatCode.
	builtin := map[int]string{
		0: "General", 1: "0", 2: "0.00", 3: "#,##0", 4: "#,##0.00",
		9: "0%", 10: "0.00%", 11: "0.00E+00", 12: "# ?/?", 13: "# ??/??",
		45: "mm:ss", 46: "[h]:mm:ss", 47: "mmss.0",
		48: "##0.0E+0", 49: "@",
	}
	if code, ok := builtin[id]; ok {
		return "code:" + code, nil
	}
	if id < 164 {
		return fmt.Sprintf("builtin:%d", id), nil
	}
	return "", fmt.Errorf("自定义数字格式 %d 没有 formatCode 定义", id)
}

func artifactStyleKey(styles *artifactNode, index int) (string, error) {
	xf, err := artifactIndexedNode(styles, "cellXfs", "xf", index)
	if err != nil {
		return "", err
	}
	font, err := artifactStyleComponent(styles, xf, "fontId", "applyFont", "fonts", "font")
	if err != nil {
		return "", err
	}
	fill, err := artifactStyleComponent(styles, xf, "fillId", "applyFill", "fills", "fill")
	if err != nil {
		return "", err
	}
	border, err := artifactStyleComponent(styles, xf, "borderId", "applyBorder", "borders", "border")
	if err != nil {
		return "", err
	}
	numberFormat, err := artifactNumberFormatKey(styles, xf)
	if err != nil {
		return "", err
	}
	// Style IDs can be renumbered by export. Fingerprint the actual components
	// and non-reference xf attributes so structural comparisons survive that.
	attrs := []string{}
	for _, a := range xf.Attr {
		if a.Name.Local == "fontId" || a.Name.Local == "fillId" || a.Name.Local == "borderId" || a.Name.Local == "xfId" || a.Name.Local == "numFmtId" || a.Name.Local == "applyNumberFormat" {
			continue
		}
		attrs = append(attrs, a.Name.Local+"="+a.Value)
	}
	sort.Strings(attrs)
	parts := []any{artifactNodeKey(font), artifactNodeKey(fill), artifactNodeKey(border), numberFormat, attrs}
	for _, child := range xf.Children {
		parts = append(parts, artifactNodeKey(child))
	}
	b, err := json.Marshal(parts)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}
func artifactNodeKey(n *artifactNode) any {
	attrs := []string{}
	for _, a := range n.Attr {
		if a.Name.Space == "xmlns" || a.Name.Local == "xmlns" {
			continue
		}
		attrs = append(attrs, a.Name.Space+":"+a.Name.Local+"="+a.Value)
	}
	sort.Strings(attrs)
	children := []any{}
	for _, c := range n.Children {
		children = append(children, artifactNodeKey(c))
	}
	return []any{n.Name.Space, n.Name.Local, attrs, n.Text, children}
}
