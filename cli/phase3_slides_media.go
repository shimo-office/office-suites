package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const phase3SlideMediaMaxBytes = 8 << 20

type phase3SlideMediaSpec struct {
	Kind        string  `json:"kind"`
	File        string  `json:"file"`
	Name        string  `json:"name"`
	MIME        string  `json:"mime"`
	SHA256      string  `json:"sha256"`
	TriggerType string  `json:"triggerType,omitempty"`
	Size        int     `json:"size"`
	Duration    float64 `json:"decodedDuration,omitempty"`
}

func preparePhase3SlideMedia(p map[string]any) (*phase3SlideMediaSpec, error) {
	for k := range p {
		if k != "slideId" && k != "kind" && k != "file" && k != "triggerType" {
			return nil, fmt.Errorf("不支持媒体参数 %s", k)
		}
	}
	kind, _ := p["kind"].(string)
	filename, _ := p["file"].(string)
	if kind != "audio" && kind != "video" {
		return nil, fmt.Errorf("kind 必须为 audio 或 video")
	}
	if filename == "" {
		return nil, fmt.Errorf("file 必须为本地媒体路径")
	}
	file, e := filepath.Abs(filename)
	if e != nil {
		return nil, e
	}
	info, e := os.Stat(file)
	if e != nil {
		return nil, e
	}
	if !info.Mode().IsRegular() || info.Size() < 12 || info.Size() > phase3SlideMediaMaxBytes {
		return nil, fmt.Errorf("媒体必须为12字节至8MiB的普通文件")
	}
	name := filepath.Base(file)
	if len(name) > 200 || strings.ContainsAny(name, "\r\n\t\x00") {
		return nil, fmt.Errorf("媒体文件名无效")
	}
	types := map[string]string{".wav": "audio/wav", ".mp3": "audio/mpeg", ".m4a": "audio/mp4", ".mp4": "video/mp4", ".webm": "video/webm"}
	mime := types[strings.ToLower(filepath.Ext(file))]
	if !strings.HasPrefix(mime, kind+"/") {
		return nil, fmt.Errorf("媒体类型与扩展名不符或尚不支持")
	}
	trigger := ""
	if kind == "audio" {
		trigger = "click"
	}
	if value, ok := p["triggerType"]; ok {
		str, valid := value.(string)
		if !valid || kind != "audio" || (str != "click" && str != "auto") {
			return nil, fmt.Errorf("仅音频支持 triggerType=click 或 auto")
		}
		trigger = str
	}
	raw, e := os.ReadFile(file)
	if e != nil {
		return nil, e
	}
	return &phase3SlideMediaSpec{Kind: kind, File: file, Name: name, MIME: mime, SHA256: fmt.Sprintf("%x", sha256.Sum256(raw)), Size: len(raw), TriggerType: trigger}, nil
}
func phase3VerifySlideMediaNode(n *artifactNode, refs map[string]string, s phase3SlideMediaSpec) (string, error) {
	nv := phase3SlideChild(n, artifactSlideNS, "nvPicPr")
	id := phase3SlideChild(nv, artifactSlideNS, "cNvPr")
	if n.Name.Space != artifactSlideNS || n.Name.Local != "pic" || id == nil || id.attr("name") != s.Name || id.attr("id") == "" {
		return "", fmt.Errorf("新增媒体对象身份不符")
	}
	hidden := id.attr("hidden")
	if hidden != "" && hidden != "0" && hidden != "false" {
		return "", fmt.Errorf("媒体对象被隐藏")
	}
	transform := phase3SlideChild(phase3SlideChild(n, artifactSlideNS, "spPr"), artifactDrawingNS, "xfrm")
	offset := phase3SlideChild(transform, artifactDrawingNS, "off")
	size := phase3SlideChild(transform, artifactDrawingNS, "ext")
	for _, key := range []string{"x", "y", "cx", "cy"} {
		node := offset
		if key == "cx" || key == "cy" {
			node = size
		}
		value, e := phase3ShapeInteger(node, key)
		if e != nil || value < 0 || ((key == "cx" || key == "cy") && value == 0) {
			return "", fmt.Errorf("媒体对象缺少可见位置/尺寸")
		}
	}
	count := 0
	var failure error
	phase3WalkSlideNode(phase3SlideChild(nv, artifactSlideNS, "nvPr"), func(x *artifactNode) {
		if x.Name.Space == artifactDrawingNS && (x.Name.Local == "audioFile" || x.Name.Local == "videoFile") {
			count++
			if x.Name.Local != s.Kind+"File" {
				failure = fmt.Errorf("媒体类型不符")
			}
			link := ""
			for _, a := range x.Attr {
				if a.Name.Space == artifactOfficeRelNS && (a.Name.Local == "link" || a.Name.Local == "embed") {
					link = a.Value
				}
			}
			if refs[link] != artifactOfficeRelNS+"/"+s.Kind+":"+s.SHA256 {
				failure = fmt.Errorf("媒体实际资源与输入SHA256不符")
			}
		}
	})
	if count != 1 {
		return "", fmt.Errorf("缺少唯一真实媒体引用；海报图片不算媒体")
	}
	if failure != nil {
		return "", failure
	}
	return id.attr("id"), nil
}
