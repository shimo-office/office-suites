package main

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"path"
	"reflect"
	"strings"
)

// Typed direct-format/table/mention actions must not rewrite inherited styles.
// Capture conventional parts and relationship-bound parts (including custom paths),
// as well as the bindings: unchanged resource bytes with a changed binding are unsafe.
func documentStyleResources(entries map[string]*zip.File) (map[string]string, error) {
	out := map[string]string{}
	names := map[string]bool{}
	for name := range entries {
		if name == "word/styles.xml" || name == "word/stylesWithEffects.xml" || name == "word/numbering.xml" || strings.HasPrefix(name, "word/theme/") && strings.HasSuffix(name, ".xml") {
			names[name] = true
		}
	}
	for name := range entries {
		if !strings.HasSuffix(name, ".rels") {
			continue
		}
		data, err := zipEntryBytes(entries[name])
		if err != nil {
			return nil, err
		}
		root, err := parseArtifactXML(data)
		if err != nil {
			return nil, err
		}
		if root.Name.Space != artifactRelNS || root.Name.Local != "Relationships" {
			return nil, fmt.Errorf("无效资源关系 %s", name)
		}
		for _, rel := range root.children("Relationship") {
			kind := path.Base(rel.attr("Type"))
			if kind != "styles" && kind != "stylesWithEffects" && kind != "numbering" && kind != "theme" && kind != "themeOverride" {
				continue
			}
			if rel.attr("TargetMode") == "External" {
				return nil, fmt.Errorf("不支持外部样式资源")
			}
			target := rel.attr("Target")
			if target == "" || strings.ContainsAny(target, "\\?#") {
				return nil, fmt.Errorf("无效样式资源路径")
			}
			if strings.HasPrefix(target, "/") {
				target = strings.TrimPrefix(target, "/")
			} else {
				target = path.Join(path.Dir(path.Dir(name)), target)
			}
			target = path.Clean(target)
			if target == ".." || strings.HasPrefix(target, "../") || entries[target] == nil {
				return nil, fmt.Errorf("样式资源缺失或越界")
			}
			key := "binding:" + name + ":" + rel.attr("Id")
			if _, duplicate := out[key]; duplicate {
				return nil, fmt.Errorf("重复样式资源关系")
			}
			out[key] = documentResourceKey(rel)
			names[target] = true
		}
	}
	for name := range names {
		data, err := zipEntryBytes(entries[name])
		if err != nil {
			return nil, err
		}
		root, err := parseArtifactXML(data)
		if err != nil {
			return nil, err
		}
		out["part:"+name] = documentResourceKey(root)
	}
	// These settings alter the meaning of theme fonts/colors without changing a theme.
	if entries["word/settings.xml"] != nil {
		root, err := artifactPart(entries, "word/settings.xml", artifactWordNS, "settings")
		if err != nil {
			return nil, err
		}
		for _, name := range []string{"themeFontLang", "clrSchemeMapping"} {
			if n := root.child(name); n != nil {
				out["setting:"+name] = documentResourceKey(n)
			}
		}
	}
	return out, nil
}

func documentResourceKey(n *artifactNode) string {
	return documentTableExactKey(documentResourceNode(n))
}
func documentResourceNode(n *artifactNode) *artifactNode {
	clone := *n
	clone.Attr = nil
	clone.Children = nil
	for _, attr := range n.Attr {
		if attr.Name.Space != "xmlns" && attr.Name != (xml.Name{Local: "xmlns"}) {
			clone.Attr = append(clone.Attr, attr)
		}
	}
	for _, child := range n.Children {
		clone.Children = append(clone.Children, documentResourceNode(child))
	}
	return &clone
}
func verifyDocumentResources(before, after map[string]string) error {
	if len(before) == 0 && len(after) == 0 {
		return nil
	}
	if !reflect.DeepEqual(before, after) {
		return fmt.Errorf("请求外的继承样式、编号、主题资源或绑定改变")
	}
	return nil
}
