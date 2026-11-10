package main

import "testing"

func TestExtractExamples(t *testing.T) {
	cases := []struct {
		file string
		fn   func(string) (string, error)
	}{
		{"examples/接口验证.docx", extractDocx},
		{"examples/接口验证.xlsx", extractXlsx},
		{"examples/接口验证.pptx", extractPptx},
	}
	for _, c := range cases {
		text, err := c.fn(c.file)
		if err != nil {
			t.Fatalf("%s: %v", c.file, err)
		}
		if text == "" {
			t.Errorf("%s: empty text", c.file)
		}
	}
}
