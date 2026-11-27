package main

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func lightDocLineMultiple(label string) (float64, error) {
	label = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(label), "倍"))
	n, err := strconv.ParseFloat(label, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n <= 0 || n > 10 {
		return 0, errors.New("lineHeight 必须是正数行距倍率，例如 1.5")
	}
	return n, nil
}

// DOM line-height is an editor rendering value, not the requested line multiple.
// Verify persisted OOXML's 240ths-of-a-line, never infer success from any change.
func verifyLightDocLineHeight(filename, text, requested string) (map[string]any, error) {
	want, err := lightDocLineMultiple(requested)
	if err != nil {
		return nil, err
	}
	z, parts, err := openArtifact(filename)
	if err != nil {
		return nil, err
	}
	defer z.Close()
	root, err := artifactPart(parts, "word/document.xml", artifactWordNS, "document")
	if err != nil {
		return nil, err
	}
	var matches []*artifactNode
	var walk func(*artifactNode)
	walk = func(n *artifactNode) {
		if n.Name.Space == artifactWordNS && n.Name.Local == "p" {
			var s strings.Builder
			var collect func(*artifactNode)
			collect = func(x *artifactNode) {
				if x.Name.Space == artifactWordNS && x.Name.Local == "t" {
					s.WriteString(x.Text)
				}
				for _, c := range x.Children {
					collect(c)
				}
			}
			collect(n)
			if s.String() == text {
				matches = append(matches, n)
			}
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(root)
	if len(matches) != 1 {
		return nil, fmt.Errorf("DOCX行距核验需要唯一的目标段落正文，匹配%d处", len(matches))
	}
	props := matches[0].child("pPr")
	if props == nil {
		return nil, errors.New("DOCX目标段落缺少显式行距")
	}
	spacing := props.child("spacing")
	if spacing == nil {
		return nil, errors.New("DOCX目标段落缺少显式行距")
	}
	line, rule := documentAttr(spacing, "line"), documentAttr(spacing, "lineRule")
	n, err := strconv.ParseFloat(line, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || (rule != "" && rule != "auto") || math.Abs(n-want*240) > 0.01 {
		return nil, fmt.Errorf("DOCX行距与请求不符：请求%s倍，w:line=%s，w:lineRule=%s", requested, line, rule)
	}
	return map[string]any{"requested": want, "observed": line, "normalized": n / 240, "source": "exported-docx/word/document.xml/w:pPr/w:spacing", "unit": "multiple", "artifact": filename, "verified": true}, nil
}

func (a *App) lightDocVerifySavedLineHeight(guid, dir, text, requested string) (map[string]any, error) {
	art, err := a.exportFile(guid, "docx")
	if err != nil {
		return nil, err
	}
	body, err := httpGet(art["url"])
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "line-height.docx")
	if err = os.WriteFile(path, body, 0600); err != nil {
		return nil, err
	}
	return verifyLightDocLineHeight(path, text, requested)
}
