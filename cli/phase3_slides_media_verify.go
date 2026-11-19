package main

import (
	"crypto/sha256"
	"fmt"
	"reflect"
)

func phase3SlideMediaReferences(filename string, ids []string, target string) (map[string]string, error) {
	z, entries, e := openArtifact(filename)
	if e != nil {
		return nil, e
	}
	defer z.Close()
	root, e := artifactPart(entries, "ppt/presentation.xml", artifactSlideNS, "presentation")
	if e != nil {
		return nil, e
	}
	rels, e := phase3ThemeRelations(entries, "ppt/presentation.xml")
	if e != nil {
		return nil, e
	}
	list := phase3SlideChild(root, artifactSlideNS, "sldIdLst")
	if list == nil || len(list.Children) != len(ids) {
		return nil, fmt.Errorf("媒体导出页序无效")
	}
	for i, id := range ids {
		if id != target {
			continue
		}
		part := rels[relationID(list.Children[i])].Target
		refs, e := phase3ThemeRelations(entries, part)
		if e != nil {
			return nil, e
		}
		result := map[string]string{}
		for key, r := range refs {
			if r.Kind == artifactOfficeRelNS+"/audio" || r.Kind == artifactOfficeRelNS+"/video" {
				raw, e := zipEntryBytes(entries[r.Target])
				if e != nil {
					return nil, e
				}
				result[key] = r.Kind + ":" + fmt.Sprintf("%x", sha256.Sum256(raw))
			}
		}
		return result, nil
	}
	return nil, fmt.Errorf("媒体目标页不存在")
}
func phase3MediaTargetFilter(ids []string, s phase3SlidesSpec, refs map[string]string, after bool) func(int, *artifactNode) (*artifactNode, error) {
	return func(i int, n *artifactNode) (*artifactNode, error) {
		if ids[i] != s.SlideID {
			return n, nil
		}
		tree, e := phase3ShapeTree(n)
		if e != nil {
			return nil, e
		}
		at := -1
		for j, x := range tree.Children {
			nv := phase3SlideChild(x, artifactSlideNS, "nvPicPr")
			id := phase3SlideChild(nv, artifactSlideNS, "cNvPr")
			if id != nil && id.attr("name") == s.Media.Name {
				if at >= 0 {
					return nil, fmt.Errorf("重复媒体对象名")
				}
				at = j
			}
		}
		timing := phase3SlideChild(n, artifactSlideNS, "timing")
		if !after {
			if at >= 0 {
				return nil, fmt.Errorf("已存在同名媒体，禁止重复插入")
			}
			if timing != nil && len(timing.Children) > 0 {
				if s.Media.Kind != "video" {
					return nil, fmt.Errorf("目标页已有动画/媒体时序，当前动作拒绝覆盖")
				}
				if e := phase3ValidateExistingClickAudio(n, ""); e != nil {
					return nil, e
				}
			}
			return n, nil
		}
		if at < 0 {
			return nil, fmt.Errorf("新增媒体对象未导出")
		}
		shapeID, e := phase3VerifySlideMediaNode(tree.Children[at], refs, *s.Media)
		if e != nil {
			return nil, e
		}
		preserveAudioTiming := s.Media.Kind == "video" && timing != nil && phase3ValidateExistingClickAudio(n, shapeID) == nil
		if timing != nil && !preserveAudioTiming {
			targets := 0
			var failure error
			phase3WalkSlideNode(timing, func(x *artifactNode) {
				if x.Name.Space != artifactSlideNS {
					return
				}
				switch x.Name.Local {
				case "spTgt":
					targets++
					if x.attr("spid") != shapeID {
						failure = fmt.Errorf("媒体时序绑定了其他对象")
					}
				case "anim", "animClr", "animEffect", "animMotion", "animRot", "animScale", "set":
					failure = fmt.Errorf("媒体插入新增了对象动画")
				}
				if (x.Name.Local == "audio" || x.Name.Local == "video") && x.Name.Local != s.Media.Kind {
					failure = fmt.Errorf("媒体时序类型不符")
				}
			})
			if failure != nil {
				return nil, failure
			}
			if targets == 0 && len(timing.Children) > 0 {
				return nil, fmt.Errorf("媒体时序没有目标对象")
			}
		}
		rootCopy := *n
		rootCopy.Children = nil
		for _, c := range n.Children {
			if c.Name.Space == artifactSlideNS && c.Name.Local == "timing" && !preserveAudioTiming {
				continue
			}
			if c.Name.Space == artifactSlideNS && c.Name.Local == "cSld" {
				content := *c
				content.Children = nil
				for _, x := range c.Children {
					if x == tree {
						copy := *tree
						copy.Children = append(append([]*artifactNode{}, tree.Children[:at]...), tree.Children[at+1:]...)
						content.Children = append(content.Children, &copy)
					} else {
						content.Children = append(content.Children, x)
					}
				}
				rootCopy.Children = append(rootCopy.Children, &content)
			} else {
				rootCopy.Children = append(rootCopy.Children, c)
			}
		}
		return &rootCopy, nil
	}
}
func verifyPhase3SlideMediaArtifacts(before, after string, ids, next []string, s phase3SlidesSpec) error {
	if s.Media == nil || !reflect.DeepEqual(ids, next) {
		return fmt.Errorf("插入媒体不得改变页面列表")
	}
	refs, e := phase3SlideMediaReferences(after, next, s.SlideID)
	if e != nil {
		return e
	}
	b, e := readPhase3SlideArtifactFiltered(before, ids, phase3MediaTargetFilter(ids, s, nil, false))
	if e != nil {
		return e
	}
	a, e := readPhase3SlideArtifactFiltered(after, next, phase3MediaTargetFilter(next, s, refs, true))
	if e != nil {
		return e
	}
	if !reflect.DeepEqual(b, a) {
		return fmt.Errorf("媒体插入改变了非目标内容、版式或资源")
	}
	return nil
}
