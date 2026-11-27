package main

import (
	"math"
	"reflect"
)

// Compare against the official source once. The native outer-shadow parser
// divides blurRad by 36000, then its serializer multiplies and truncates to a
// signed 32-bit integer. A value such as 365269 therefore serializes as 365268.
// Only that exact conversion is accepted, never a symmetric ±1 tolerance.
func phase3ThemeDeltaEqual(expected, actual any) bool {
	if reflect.DeepEqual(expected, actual) {
		return true
	}
	switch source := expected.(type) {
	case []any:
		value, ok := actual.([]any)
		if !ok || len(source) != len(value) || (source == nil) != (value == nil) {
			return false
		}
		for i := range source {
			if !phase3ThemeDeltaEqual(source[i], value[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		value, ok := actual.(map[string]any)
		if !ok || len(source) != len(value) || (source == nil) != (value == nil) {
			return false
		}
		attrs, _ := source["attributes"].(map[string]any)
		if source["action"] == "insert" && attrs["opType"] == "spPr" {
			return reflect.DeepEqual(source, phase3ThemeShadowReadback(source, value))
		}
		for key, v := range source {
			candidate, exists := value[key]
			if !exists || !phase3ThemeDeltaEqual(v, candidate) {
				return false
			}
		}
		return true
	}
	return false
}

func phase3ThemeShapeEffects(v any) []any {
	for _, key := range []string{"data", "object", "spPr", "extra", "effects"} {
		object, _ := v.(map[string]any)
		v = object[key]
	}
	effects, _ := v.(map[string]any)
	if effects["type"] != "effectLst" {
		return nil
	}
	list, _ := effects["data"].([]any)
	return list
}

func phase3ThemeShadowReadback(expected, actual map[string]any) any {
	out := phase3ThemeCopyJSON(actual)
	source, target := phase3ThemeShapeEffects(expected), phase3ThemeShapeEffects(out)
	if len(source) != len(target) {
		return out
	}
	for i, effect := range source {
		original, _ := effect.(map[string]any)
		readback, _ := target[i].(map[string]any)
		if original["name"] != "outerShdw" || readback["name"] != "outerShdw" {
			continue
		}
		radius, ok := original["blurRad"].(float64)
		if !ok || radius < 0 || radius > 2147483647 || math.Trunc(radius) != radius {
			continue
		}
		millimeters := float64(radius / 36000)
		serialized := math.Trunc(millimeters * 36000)
		if serialized >= 0 && serialized <= 2147483647 && math.Abs(serialized-radius) <= 1 && readback["blurRad"] == serialized {
			readback["blurRad"] = radius
		}
	}
	return out
}

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
