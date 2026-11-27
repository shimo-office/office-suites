package main

import (
	"strings"
	"testing"
)

func tableFixtureForDuplicateFields() map[string]any {
	table := tableFixtureForEdit()
	table["fields"] = append(table["fields"].([]any),
		map[string]any{"guid": "date", "name": "日期", "type": "Date", "props": map[string]any{"dateFormat": "yyyy/M/d"}},
		map[string]any{"guid": "link", "name": "项目链接", "type": "Link", "props": nil},
		map[string]any{"guid": "attachment", "name": "文件材料", "type": "Attachment", "props": nil})
	rows := phase3TableMap(table, "records")
	rows["r1"]["valueMap"].(map[string]any)["date"] = float64(46353)
	rows["r1"]["valueMap"].(map[string]any)["link"] = map[string]any{"text": "t1", "url": "https://example.com/date", "logo": ""}
	rows["r1"]["valueMap"].(map[string]any)["attachment"] = []any{}
	rows["r2"]["valueMap"].(map[string]any)["attachment"] = nil
	view := phase3TableMap(table, "views")["v1"]
	view["fields"] = []any{"f1", "f2", "date", "link", "attachment"}
	view["calendar"] = map[string]any{"startFieldGuid": "date", "endFieldGuid": "date"}
	return table
}

func TestPhase3DuplicateAllowsLiteralFieldsAndEmptyAttachments(t *testing.T) {
	table := tableFixtureForDuplicateFields()
	if _, err := preparePhase3BaseEdit("table-duplicate", map[string]any{"tableGuid": "t1", "name": "copy"}, "b1", map[string]map[string]any{"t1": table}); err != nil {
		t.Fatal(err)
	}
	for _, typ := range []string{"Attachment", "ReferenceRecord", "Formula"} {
		t.Run(typ, func(t *testing.T) {
			table := tableFixtureForDuplicateFields()
			phase3TableMap(table, "fields")["attachment"]["type"] = typ
			phase3TableMap(table, "records")["r2"]["valueMap"].(map[string]any)["attachment"] = []any{map[string]any{"guid": "asset"}}
			_, err := preparePhase3BaseEdit("table-duplicate", map[string]any{"tableGuid": "t1", "name": "copy"}, "b1", map[string]map[string]any{"t1": table})
			if err == nil || !strings.Contains(err.Error(), "文件材料") || !strings.Contains(err.Error(), typ) {
				t.Fatalf("unsupported field must be identified: %v", err)
			}
		})
	}
}

func TestPhase3DuplicatePreservesDateLinkAndCalendarReferences(t *testing.T) {
	source := tableFixtureForDuplicateFields()
	target := phase3TableCopy(source)
	target["guid"], target["name"] = "copy", "copy"
	phase3TableMap(target, "fields")["date"]["guid"] = "new-date"
	for _, row := range phase3TableMap(target, "records") {
		values := row["valueMap"].(map[string]any)
		if value, exists := values["date"]; exists {
			values["new-date"] = value
			delete(values, "date")
		}
	}
	view := phase3TableMap(target, "views")["v1"]
	view["fields"] = []any{"f1", "f2", "new-date", "link", "attachment"}
	view["calendar"] = map[string]any{"startFieldGuid": "new-date", "endFieldGuid": "new-date"}
	if err := verifyPhase3TableDuplicate(source, target); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"date-value", "date-format", "link-url", "link-text", "link-logo", "calendar-binding", "attachment"} {
		t.Run(mode, func(t *testing.T) {
			bad := phase3TableCopy(target)
			values := phase3TableMap(bad, "records")["r1"]["valueMap"].(map[string]any)
			switch mode {
			case "date-value":
				values["new-date"] = float64(46354)
			case "date-format":
				phase3TableMap(bad, "fields")["new-date"]["props"].(map[string]any)["dateFormat"] = "yyyy"
			case "link-url":
				values["link"].(map[string]any)["url"] = "https://example.com/new-date"
			case "link-text":
				values["link"].(map[string]any)["text"] = "copy"
			case "link-logo":
				values["link"].(map[string]any)["logo"] = "https://example.com/icon.png"
			case "calendar-binding":
				phase3TableMap(bad, "views")["v1"]["calendar"].(map[string]any)["startFieldGuid"] = "f2"
			case "attachment":
				values["attachment"] = []any{map[string]any{"guid": "asset"}}
			}
			if err := verifyPhase3TableDuplicate(source, bad); err == nil {
				t.Fatal("corrupted copy accepted")
			}
		})
	}
}
