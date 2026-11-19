package main

import (
	"fmt"
	"strings"
)

// bwMode has no XSD default. The current SDK SpPr constructor initializes it to
// 0; absent JSON retains 0 and toJSON maps 0 to "auto". This exception is local
// to the edited shape/placeholder. Conservatively reject any non-auto mode anywhere
// in the package, including inherited master/layout modes, before writing and
// before comparing either artifact. Other objects' bwMode remains exact.
func phase3PlaceholderBWPreflight(filename string) error {
	z, entries, err := openArtifact(filename)
	if err != nil {
		return err
	}
	defer z.Close()
	for name, entry := range entries {
		if !strings.HasSuffix(name, ".xml") {
			continue
		}
		data, e := zipEntryBytes(entry)
		if e != nil {
			return e
		}
		root, e := parseArtifactXML(data)
		if e != nil {
			return e
		}
		invalid := false
		phase3WalkSlideNode(root, func(n *artifactNode) {
			for _, a := range n.Attr {
				if a.Name.Local == "bwMode" && a.Value != "auto" {
					invalid = true
				}
			}
		})
		if invalid {
			return fmt.Errorf("%s含非auto黑白呈现模式，拒绝目标形状中性序列化及写入", name)
		}
	}
	return nil
}
func phase3PlaceholderNativeBW(n *artifactNode) *artifactNode {
	out := *n
	out.Children = append([]*artifactNode(nil), n.Children...)
	for i, c := range n.Children {
		if c.Name.Space != artifactSlideNS || c.Name.Local != "spPr" {
			continue
		}
		copy := *c
		copy.Attr = nil
		for _, a := range c.Attr {
			if a.Name.Space == "" && a.Name.Local == "bwMode" && a.Value == "auto" {
				continue
			}
			copy.Attr = append(copy.Attr, a)
		}
		out.Children[i] = &copy
	}
	return &out
}
