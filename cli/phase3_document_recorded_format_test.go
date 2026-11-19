package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocumentFormatAndMentionRecordedPackages(t *testing.T) {
	const base = "../tmp/phase3-live/docx-slides/acceptance-driver/runs"
	if _, err := os.Stat(base); os.IsNotExist(err) {
		t.Skip("local recorded acceptance packages unavailable")
	}
	checked := 0
	for _, name := range []string{"format-blue", "format-red", "format-super", "format-sub", "format-underline-repair", "self-mention-fixed"} {
		logs, err := filepath.Glob(filepath.Join(base, name, "*", "cli.json"))
		if err != nil {
			t.Fatal(err)
		}
		for _, log := range logs {
			b, err := os.ReadFile(log)
			if err != nil {
				t.Fatal(err)
			}
			var record struct {
				Accepted bool     `json:"accepted"`
				Argv     []string `json:"argv"`
			}
			if err = json.Unmarshal(b, &record); err != nil {
				t.Fatal(err)
			}
			if !record.Accepted {
				continue
			}
			t.Run(name, func(t *testing.T) {
				var payload map[string]any
				for i, arg := range record.Argv {
					if arg == "--payload" && i+1 < len(record.Argv) {
						if e := json.Unmarshal([]byte(record.Argv[i+1]), &payload); e != nil {
							t.Fatal(e)
						}
					}
				}
				beforeFiles, _ := filepath.Glob(filepath.Join(filepath.Dir(log), "phase3-*", "before", "*.docx"))
				reloadFiles, _ := filepath.Glob(filepath.Join(filepath.Dir(log), "phase3-*", "reload*", "*.docx"))
				if len(beforeFiles) != 1 || len(reloadFiles) != 1 {
					t.Fatal("recorded baseline/reload absent")
				}
				before, e := readDocumentSnapshot(beforeFiles[0])
				if e != nil {
					t.Fatal(e)
				}
				after, e := readDocumentSnapshot(reloadFiles[0])
				if e != nil {
					t.Fatal(e)
				}
				if name == "self-mention-fixed" {
					spec, e := prepareDocumentMention(payload)
					if e != nil {
						t.Fatal(e)
					}
					label := strings.TrimPrefix(after.Paragraphs[spec.Paragraph-1].Text, spec.Text+"@")
					if e = verifyDocumentMention(before, after, spec, label); e != nil {
						t.Fatal(e)
					}
				} else {
					spec, e := prepareDocumentFormat(payload)
					if e != nil {
						t.Fatal(e)
					}
					if e = verifyDocumentFormat(before, after, spec); e != nil {
						t.Fatal(e)
					}
				}
			})
			checked++
		}
	}
	if checked < 6 {
		t.Fatalf("expected six original successful package pairs, got %d", checked)
	}
}
