package main

import "testing"

func TestDefaultBrowserFlagParsing(t *testing.T) {
	for _, args := range [][]string{
		{"open", "https://shimo.net/docs/demo", "--default"},
		{"open", "--default", "https://shimo.net/docs/demo"},
	} {
		a := newApp(args)
		if !a.has("--default") {
			t.Fatalf("--default not parsed for %#v", args)
		}
		if len(a.Pos) != 1 || a.Pos[0] != "https://shimo.net/docs/demo" {
			t.Fatalf("positionals = %#v for %#v", a.Pos, args)
		}
	}
}

func TestCreateTypesIncludeDocumentPro(t *testing.T) {
	if createTypes["doc"] != "document" {
		t.Fatalf("doc = %q", createTypes["doc"])
	}
	for _, alias := range []string{"docx", "docpro", "documentpro", "word", "文稿"} {
		if createTypes[alias] != "documentPro" {
			t.Fatalf("%s = %q, want documentPro", alias, createTypes[alias])
		}
	}
}
