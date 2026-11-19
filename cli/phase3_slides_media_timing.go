package main

import (
	"fmt"
	"reflect"
	"strconv"
)

// This is the single click-audio timing schema emitted by the native exporter.
// IDs and volume are validated separately; all other structure stays exact.
const phase3ClickAudioTimingTemplate = `<ns0:timing xmlns:ns0="http://schemas.openxmlformats.org/presentationml/2006/main"><ns0:tnLst><ns0:par><ns0:cTn id="1" dur="indefinite" restart="never" nodeType="tmRoot"><ns0:childTnLst><ns0:seq concurrent="1" nextAc="seek"><ns0:cTn id="2" dur="indefinite" nodeType="mainSeq"><ns0:childTnLst><ns0:par><ns0:cTn id="4" fill="hold"><ns0:stCondLst><ns0:cond delay="indefinite" /></ns0:stCondLst><ns0:childTnLst><ns0:par><ns0:cTn id="5" fill="hold"><ns0:stCondLst><ns0:cond delay="0" /></ns0:stCondLst><ns0:childTnLst><ns0:par><ns0:cTn id="6" presetID="1" presetClass="mediacall" presetSubtype="0" fill="hold" nodeType="clickEffect"><ns0:stCondLst><ns0:cond delay="0" /></ns0:stCondLst><ns0:childTnLst><ns0:cmd type="call" cmd="playFrom(0.0)"><ns0:cBhvr><ns0:cTn id="7" dur="1" fill="hold" /><ns0:tgtEl><ns0:spTgt spid="7" /></ns0:tgtEl></ns0:cBhvr></ns0:cmd></ns0:childTnLst></ns0:cTn></ns0:par></ns0:childTnLst></ns0:cTn></ns0:par></ns0:childTnLst></ns0:cTn></ns0:par></ns0:childTnLst></ns0:cTn><ns0:prevCondLst><ns0:cond evt="onPrev" delay="0"><ns0:tgtEl><ns0:sldTgt /></ns0:tgtEl></ns0:cond></ns0:prevCondLst><ns0:nextCondLst><ns0:cond evt="onNext" delay="0"><ns0:tgtEl><ns0:sldTgt /></ns0:tgtEl></ns0:cond></ns0:nextCondLst></ns0:seq><ns0:audio><ns0:cMediaNode vol="80000" showWhenStopped="1"><ns0:cTn id="3" fill="hold"><ns0:stCondLst><ns0:cond delay="indefinite" /></ns0:stCondLst><ns0:endCondLst><ns0:cond evt="onStopAudio" delay="0"><ns0:tgtEl><ns0:sldTgt /></ns0:tgtEl></ns0:cond></ns0:endCondLst></ns0:cTn><ns0:tgtEl><ns0:spTgt spid="7" /></ns0:tgtEl></ns0:cMediaNode></ns0:audio></ns0:childTnLst></ns0:cTn></ns0:par></ns0:tnLst></ns0:timing>`

func phase3NormalizeClickAudioTiming(n *artifactNode, audioID string) (*artifactNode, error) {
	out := phase3ThemeCloneNode(n)
	ids := map[string]bool{}
	var failure error
	phase3WalkSlideNode(out, func(x *artifactNode) {
		if x.Name.Space != artifactSlideNS {
			return
		}
		for i, a := range x.Attr {
			switch {
			case x.Name.Local == "cTn" && a.Name.Local == "id":
				id, e := strconv.Atoi(a.Value)
				if e != nil || id < 1 || ids[a.Value] {
					failure = fmt.Errorf("音频时序ID无效或重复")
					return
				}
				ids[a.Value] = true
				x.Attr[i].Value = strconv.Itoa(len(ids))
			case x.Name.Local == "spTgt" && a.Name.Local == "spid":
				if a.Value != audioID {
					failure = fmt.Errorf("原音频时序引用其他对象")
				}
				x.Attr[i].Value = "audio"
			case x.Name.Local == "cMediaNode" && a.Name.Local == "vol":
				volume, e := strconv.Atoi(a.Value)
				if e != nil || volume < 0 || volume > 100000 {
					failure = fmt.Errorf("音频音量无效")
				}
				x.Attr[i].Value = "volume"
			}
		}
	})
	return out, failure
}

func phase3ValidateExistingClickAudio(page *artifactNode, excludedID string) error {
	tree, e := phase3ShapeTree(page)
	if e != nil {
		return e
	}
	audioID := ""
	mediaCount := 0
	for _, shape := range tree.Children {
		if excludedID != "" && phase3ThemeShapeID(shape) == excludedID {
			continue
		}
		phase3WalkSlideNode(shape, func(n *artifactNode) {
			if n.Name.Space == artifactDrawingNS && (n.Name.Local == "audioFile" || n.Name.Local == "videoFile") {
				mediaCount++
				if n.Name.Local == "audioFile" && shape.Name.Space == artifactSlideNS && shape.Name.Local == "pic" {
					audioID = phase3ThemeShapeID(shape)
				}
			}
		})
	}
	if mediaCount != 1 || audioID == "" {
		return fmt.Errorf("仅支持保留唯一现有点击音频")
	}
	timing := phase3SlideChild(page, artifactSlideNS, "timing")
	if timing == nil {
		return fmt.Errorf("原音频时序缺失")
	}
	actual, e := phase3NormalizeClickAudioTiming(timing, audioID)
	if e != nil {
		return e
	}
	expected, e := parseArtifactXML([]byte(phase3ClickAudioTimingTemplate))
	if e != nil {
		return e
	}
	expected, e = phase3NormalizeClickAudioTiming(expected, "7")
	if e != nil {
		return e
	}
	a, e := phase3SlideCanonical(actual, map[string]string{})
	if e != nil {
		return e
	}
	b, e := phase3SlideCanonical(expected, map[string]string{})
	if e != nil {
		return e
	}
	if !reflect.DeepEqual(a, b) {
		return fmt.Errorf("现有音频时序不是支持的单一点击结构")
	}
	return nil
}
