package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
)

// Fingerprint every relationship reachable from a resource, including media,
// chart workbooks and nested style parts. Cycles (layout/master) terminate only
// after the referenced node's bytes have been included. Never fetch externals.
func phase3SlideResourceFingerprint(entries map[string]*zip.File, root string) (string, error) {
	calls := 0
	var walk func(string, map[string]bool) (string, error)
	walk = func(name string, stack map[string]bool) (string, error) {
		calls++
		if calls > 10000 || len(stack) > 64 {
			return "", fmt.Errorf("资源引用闭包超过安全上限")
		}
		raw, err := zipEntryBytes(entries[name])
		if err != nil {
			return "", err
		}
		self := fmt.Sprintf("%x", sha256.Sum256(raw))
		relationPath := path.Join(path.Dir(name), "_rels", path.Base(name)+".rels")
		var relations *artifactNode
		var internal map[string]artifactRelation
		if entries[relationPath] != nil {
			relations, err = artifactPart(entries, relationPath, artifactRelNS, "Relationships")
			if err != nil {
				return "", err
			}
			internal, err = artifactRelations(entries, relationPath, path.Dir(name))
			if err != nil {
				return "", err
			}
		}
		// Notes' link back to their owner slide is identity, not note content.
		// It changes when page numbering changes and must not create a false cycle.
		notes := strings.HasPrefix(name, "ppt/notesSlides/") && strings.HasSuffix(name, ".xml")
		references := map[string]bool{}
		if notes {
			n, e := parseArtifactXML(raw)
			if e != nil {
				return "", e
			}
			ids := map[string]string{}
			if relations != nil {
				for _, r := range relations.Children {
					ids[r.attr("Id")] = r.attr("Id")
				}
			}
			var collect func(*artifactNode)
			collect = func(node *artifactNode) {
				for _, a := range node.Attr {
					if a.Name.Space == artifactOfficeRelNS {
						references[a.Value] = true
					}
				}
				for _, c := range node.Children {
					collect(c)
				}
			}
			collect(n)
			v, e := phase3SlideCanonical(n, ids)
			if e != nil {
				return "", e
			}
			b, _ := json.Marshal(v)
			self = fmt.Sprintf("%x", sha256.Sum256(b))
		}
		if stack[name] {
			return "cycle:" + self, nil
		}
		nextStack := make(map[string]bool, len(stack)+1)
		for k, v := range stack {
			nextStack[k] = v
		}
		nextStack[name] = true
		edges := []string{}
		seen := map[string]bool{}
		if relations != nil {
			for _, r := range relations.children("Relationship") {
				id, kind, mode := r.attr("Id"), r.attr("Type"), r.attr("TargetMode")
				if id == "" || seen[id] || kind == "" {
					return "", fmt.Errorf("资源关系ID/类型无效")
				}
				seen[id] = true
				if notes && kind == artifactOfficeRelNS+"/slide" && !references[id] {
					continue
				}
				value := ""
				if mode == "External" {
					value = "external:" + r.attr("Target")
					if r.attr("Target") == "" {
						return "", fmt.Errorf("外部关系目标为空")
					}
				} else {
					if mode != "" && mode != "Internal" {
						return "", fmt.Errorf("未知资源关系模式")
					}
					rel, ok := internal[id]
					if !ok {
						return "", fmt.Errorf("资源关系无法解析")
					}
					value, err = walk(rel.Target, nextStack)
					if err != nil {
						return "", err
					}
				}
				edge, _ := json.Marshal([]string{id, kind, value})
				edges = append(edges, string(edge))
			}
		}
		sort.Strings(edges)
		body, _ := json.Marshal([]any{self, edges})
		return fmt.Sprintf("%x", sha256.Sum256(body)), nil
	}
	return walk(root, map[string]bool{})
}
