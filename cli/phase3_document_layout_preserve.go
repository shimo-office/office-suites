package main

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// documentPagePreserveMargins converts the current section's header/footer
// distance from twips into the unit the page settings call uses (cm*10), so an
// explicit margin write cannot silently reset them to the editor default.
func documentPagePreserveMargins(s documentLayoutSpec, before documentLayoutSnapshot) map[string]float64 {
	keep := map[string]float64{}
	if s.Action != "document-page" || len(s.Margins) == 0 || len(before.SectionValues) == 0 {
		return keep
	}
	for _, k := range []string{"header", "footer"} {
		raw := before.SectionValues[0]["pgMar."+k]
		if raw == "" {
			continue
		}
		twips, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			continue
		}
		keep[k] = math.Round(twips/1440*25.4*100) / 100
	}
	return keep
}

// documentPagePreserveJS restores header/footer distances after a margin write.
// It is only called when the document proves those two values were rewritten
// by the editor even though the request never mentioned them.
func documentPagePreserveJS(keep map[string]float64) string {
	raw, _ := json.Marshal(keep)
	return `(async()=>{` + phase3PresenceGuardJS() + `const keep=` + string(raw) + `,e=window._sdk?.editor,a=e?.docsApi;if(typeof a?.updatePageSettings!=='function')return {ok:false,error:'页面设置方法未就绪'};const pg={};for(const k of Object.keys(keep))pg[k]=keep[k];a.updatePageSettings({scope:1,margins:pg});return {ok:true,restored:keep};})()`
}

func documentPageMarginsReset(before, after documentLayoutSnapshot) bool {
	if len(before.SectionValues) == 0 || len(after.SectionValues) == 0 {
		return false
	}
	for _, key := range []string{"pgMar.header", "pgMar.footer"} {
		if before.SectionValues[0][key] != after.SectionValues[0][key] {
			return true
		}
	}
	return false
}

// documentLayoutDiff reports what actually changed, so a rejected request is
// actionable instead of only saying "acceptance failed".
func documentLayoutDiff(before, after documentLayoutSnapshot) []string {
	out := []string{}
	if before.Background != after.Background {
		out = append(out, fmt.Sprintf("页面背景 %q → %q", before.Background, after.Background))
	}
	if len(before.Sections) != len(after.Sections) {
		out = append(out, fmt.Sprintf("节数 %d → %d", len(before.Sections), len(after.Sections)))
	} else {
		for i := range before.Sections {
			keys := map[string]bool{}
			for k := range before.Sections[i] {
				keys[k] = true
			}
			for k := range after.Sections[i] {
				keys[k] = true
			}
			names := make([]string, 0, len(keys))
			for k := range keys {
				names = append(names, k)
			}
			sort.Strings(names)
			for _, k := range names {
				if before.Sections[i][k] != after.Sections[i][k] {
					out = append(out, fmt.Sprintf("第%d节 %s 节点发生变化", i+1, k))
				}
			}
			keys = map[string]bool{}
			for k := range before.SectionValues[i] {
				keys[k] = true
			}
			for k := range after.SectionValues[i] {
				keys[k] = true
			}
			names = names[:0]
			for k := range keys {
				names = append(names, k)
			}
			sort.Strings(names)
			for _, k := range names {
				if before.SectionValues[i][k] != after.SectionValues[i][k] {
					out = append(out, fmt.Sprintf("%s %s → %s", k, valueOrEmpty(before.SectionValues[i][k]), valueOrEmpty(after.SectionValues[i][k])))
				}
			}
		}
	}
	if len(before.Paragraphs) != len(after.Paragraphs) {
		out = append(out, fmt.Sprintf("段落数 %d → %d", len(before.Paragraphs), len(after.Paragraphs)))
		return trimDiff(out)
	}
	changed := 0
	for i := range before.Paragraphs {
		b, a := before.Paragraphs[i], after.Paragraphs[i]
		if b.Text != a.Text {
			changed++
			out = append(out, fmt.Sprintf("第%d段正文改变", i+1))
			continue
		}
		if !reflect.DeepEqual(b.Props, a.Props) {
			changed++
			out = append(out, fmt.Sprintf("第%d段段落属性改变", i+1))
			continue
		}
		if !reflect.DeepEqual(b.Values, a.Values) {
			diff := []string{}
			for _, k := range sortedKeys(b.Values, a.Values) {
				if b.Values[k] != a.Values[k] {
					diff = append(diff, fmt.Sprintf("%s %s→%s", k, valueOrEmpty(b.Values[k]), valueOrEmpty(a.Values[k])))
				}
			}
			changed++
			out = append(out, fmt.Sprintf("第%d段段落设置：%s", i+1, strings.Join(diff, "，")))
			continue
		}
		var bc, ac []documentChar
		if i < len(before.Text.Paragraphs) {
			bc = documentExpanded(before.Text.Paragraphs[i])
		}
		if i < len(after.Text.Paragraphs) {
			ac = documentExpanded(after.Text.Paragraphs[i])
		}
		if len(bc) != len(ac) {
			changed++
			out = append(out, fmt.Sprintf("第%d段字符数 %d → %d", i+1, len(bc), len(ac)))
			continue
		}
		for j := range bc {
			if bc[j].Text != ac[j].Text {
				changed++
				out = append(out, fmt.Sprintf("第%d段第%d字符 %q → %q", i+1, j+1, bc[j].Text, ac[j].Text))
				break
			}
			if diff := attrDiff(bc[j].Attrs, ac[j].Attrs); diff != "" {
				changed++
				line := fmt.Sprintf("第%d段第%d字符 %s", i+1, j+1, diff)
				if changed > 0 {
					line += fmt.Sprintf("（该段共%d字）", utf8.RuneCountInString(b.Text))
				}
				out = append(out, line)
				break
			}
		}
	}
	return trimDiff(out)
}

func valueOrEmpty(v string) string {
	if v == "" {
		return "（空）"
	}
	return v
}

func sortedKeys(a, b map[string]string) []string {
	seen := map[string]bool{}
	for k := range a {
		seen[k] = true
	}
	for k := range b {
		seen[k] = true
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func attrDiff(before, after map[string]string) string {
	parts := []string{}
	for _, k := range sortedKeys(before, after) {
		if before[k] != after[k] {
			parts = append(parts, fmt.Sprintf("%s %q→%q", k, before[k], after[k]))
		}
	}
	return strings.Join(parts, "，")
}

func trimDiff(lines []string) []string {
	if len(lines) > 8 {
		rest := len(lines) - 8
		lines = append(lines[:8], fmt.Sprintf("另有%d处差异，见 failure-diff.json", rest))
	}
	return lines
}

// documentSectionDefaultEffects reports whether two section snapshots differ
// only by attributes that were previously implicit and now carry their default
// value explicitly. The page-settings write re-serialises the whole section, so
// this is a serialisation artefact, not a content change.
func documentSectionDefaultEffects(before, after map[string]string, requested map[string]float64) error {
	defaults := map[string]string{
		"cols.num": "1", "cols.equalWidth": "1", "cols.space": "425", "cols.sep": "0",
		"pgMar.gutter": "0", "pgSz.orient": "portrait",
	}
	for _, key := range sortedKeys(before, after) {
		if !strings.HasPrefix(key, "cols.") && !strings.HasPrefix(key, "pgMar.") && !strings.HasPrefix(key, "pgSz.") {
			continue
		}
		if strings.HasPrefix(key, "pgMar.") {
			// The four requested margins are allowed to change by definition.
			if _, ok := requested[strings.TrimPrefix(key, "pgMar.")]; ok {
				continue
			}
		}
		b, a := before[key], after[key]
		if b == a {
			continue
		}
		if def, ok := defaults[key]; ok {
			if b == "" {
				b = def
			}
			if a == "" {
				a = def
			}
		}
		if b != a {
			return fmt.Errorf("%s 由 %q 变为 %q", key, before[key], after[key])
		}
	}
	return nil
}
