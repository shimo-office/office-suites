package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
)

func phase3NativeShowProperties(v map[string]any) (map[string]any, error) {
	if v["persistenceSource"] != "collaboration.PresentationPr" {
		return nil, fmt.Errorf("缺少已加载的原生协同设置")
	}
	properties, ok := v["nativePresentationPr"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("原生演示属性结构无效")
	}
	raw, err := json.Marshal(properties)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err = json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	show, exists := out["showPr"]
	pr, ok := show.(map[string]any)
	if exists && !ok {
		return nil, fmt.Errorf("原生showPr结构无效")
	}
	if !exists {
		pr = map[string]any{}
		out["showPr"] = pr
	}
	loop, timing := false, true
	if value, exists := pr["loop"]; exists {
		var ok bool
		loop, ok = value.(bool)
		if !ok {
			return nil, fmt.Errorf("loop不是布尔值")
		}
	}
	if value, exists := pr["useTimings"]; exists {
		var ok bool
		timing, ok = value.(bool)
		if !ok {
			return nil, fmt.Errorf("useTimings不是布尔值")
		}
	}
	if v["loopShow"] != loop || v["useTiming"] != timing {
		return nil, fmt.Errorf("原生内存设置与协同保存设置不一致")
	}
	// The native ShowPr constructor explicitly initializes present=false.
	if pr["present"] == false {
		delete(pr, "present")
	}
	pr["loop"], pr["useTimings"] = loop, timing
	return out, nil
}

func phase3VerifyNativeShowState(before, after map[string]any, s phase3SlideShowSpec) error {
	if before["target"] == "" || before["target"] == nil || before["target"] != after["target"] {
		return fmt.Errorf("重开原生设置目标文件不一致")
	}
	want, err := phase3NativeShowProperties(before)
	if err != nil {
		return err
	}
	actual, err := phase3NativeShowProperties(after)
	if err != nil {
		return err
	}
	show := want["showPr"].(map[string]any)
	if s.LoopShow != nil {
		show["loop"] = *s.LoopShow
	}
	if s.UseTiming != nil {
		show["useTimings"] = *s.UseTiming
	}
	if !reflect.DeepEqual(want, actual) {
		return fmt.Errorf("重开原生演示设置未持久化或改变了其他设置")
	}
	return nil
}

func phase3PrepareNativeShow(b *phase3EditorBrowser, target string, s *phase3SlideShowSpec) (map[string]any, error) {
	v, err := b.Eval(phase3SlideShowReadJS())
	if err != nil {
		return nil, err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("演示设置快照无效")
	}
	if err = phase3BindNativeShow(m, target, s); err != nil {
		return nil, err
	}
	return m, nil
}

func phase3BindNativeShow(m map[string]any, target string, s *phase3SlideShowSpec) error {
	actual, _ := m["target"].(string)
	want, err := url.Parse(target)
	if err != nil || want.Scheme != "https" || want.Host == "" || !phase3SameOrigin(actual, want.Scheme+"://"+want.Host) || guidFromURL(actual) == "" || guidFromURL(actual) != guidFromURL(target) {
		return fmt.Errorf("原生设置快照未绑定请求文件")
	}
	if _, err = phase3NativeShowProperties(m); err != nil {
		return err
	}
	if (s.LoopShow == nil || m["loopShow"] == *s.LoopShow) && (s.UseTiming == nil || m["useTiming"] == *s.UseTiming) {
		return fmt.Errorf("原生演示设置已经满足目标，未写入")
	}
	s.NativeBefore = m
	return nil
}

func phase3ShowExportMatches(filename string, s phase3SlideShowSpec) (bool, error) {
	state, err := phase3ReadShowProperties(filename)
	if err != nil {
		return false, err
	}
	return (s.LoopShow == nil || state.LoopShow == *s.LoopShow) && (s.UseTiming == nil || state.UseTiming == *s.UseTiming), nil
}
