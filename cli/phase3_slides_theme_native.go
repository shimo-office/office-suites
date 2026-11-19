package main

// These two schema-specific transformations follow the native constructors:
// GradFill defaults rotateWithShape to true; font references resolve through
// the selected theme font scheme. Explicit false and all other values remain.
func phase3ThemeNativeDefaults(v any) any {
	out := phase3ThemeCopyJSON(v)
	root, _ := out.(map[string]any)
	elements, _ := root["themeElements"].(map[string]any)
	scheme, _ := elements["fmtScheme"].(map[string]any)
	for _, key := range []string{"fillStyleLst", "bgFillStyleLst"} {
		list, _ := scheme[key].([]any)
		for _, value := range list {
			wrapper, _ := value.(map[string]any)
			fill, _ := wrapper["fill"].(map[string]any)
			if fill["type"] != float64(4) {
				continue
			}
			data, _ := fill["data"].(map[string]any)
			if data == nil {
				continue
			}
			if _, exists := data["rotWithShape"]; !exists {
				data["rotWithShape"] = true
			}
		}
	}
	return out
}
func phase3ThemeResolvedFonts(v any, theme map[string]any) any {
	elements, _ := theme["themeElements"].(map[string]any)
	scheme, _ := elements["fontScheme"].(map[string]any)
	refs := map[string]string{}
	for prefix, kind := range map[string]string{"+mj-": "majorFont", "+mn-": "minorFont"} {
		fonts, _ := scheme[kind].(map[string]any)
		for suffix, script := range map[string]string{"lt": "latin", "ea": "ea", "cs": "cs"} {
			font, _ := fonts[script].(map[string]any)
			if name, ok := font["typeface"].(string); ok {
				refs[prefix+suffix] = name
			}
		}
	}
	var walk func(any) any
	walk = func(value any) any {
		switch x := value.(type) {
		case []any:
			out := make([]any, len(x))
			for i, n := range x {
				out[i] = walk(n)
			}
			return out
		case map[string]any:
			out := map[string]any{}
			for key, n := range x {
				out[key] = walk(n)
			}
			if run, ok := out["defaultRunPr"].(map[string]any); ok {
				for _, script := range []string{"latin", "ea", "cs"} {
					font, _ := run[script].(map[string]any)
					ref, _ := font["typeface"].(string)
					if name, found := refs[ref]; found {
						font["typeface"] = name
					}
				}
			}
			return out
		default:
			return value
		}
	}
	return walk(v)
}
func phase3ThemeCopyJSON(v any) any {
	switch x := v.(type) {
	case []any:
		out := make([]any, len(x))
		for i, n := range x {
			out[i] = phase3ThemeCopyJSON(n)
		}
		return out
	case map[string]any:
		out := map[string]any{}
		for k, n := range x {
			out[k] = phase3ThemeCopyJSON(n)
		}
		return out
	default:
		return v
	}
}
