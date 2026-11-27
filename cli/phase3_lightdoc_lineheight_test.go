package main

import "testing"

func TestLightDocLineHeightUsesDOCXBusinessValue(t *testing.T) {
	for _, tc := range []struct {
		name, props string
		wantErr     bool
	}{
		{"one-and-half", `<w:spacing w:line="360"/>`, false},
		{"wrong-multiple", `<w:spacing w:line="600"/>`, true},
		{"exact-points-not-multiple", `<w:spacing w:line="360" w:lineRule="exact"/>`, true},
		{"missing", ``, true},
		{"nonfinite", `<w:spacing w:line="NaN"/>`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := artifactTestZip(t, map[string]string{"word/document.xml": `<w:document xmlns:w="` + artifactWordNS + `"><w:body><w:p><w:pPr>` + tc.props + `</w:pPr><w:r><w:t>目标正文</w:t></w:r></w:p></w:body></w:document>`})
			proof, err := verifyLightDocLineHeight(p, "目标正文", "1.5")
			if (err != nil) != tc.wantErr {
				t.Fatalf("proof=%v err=%v", proof, err)
			}
			if err == nil && proof["normalized"] != 1.5 {
				t.Fatal(proof)
			}
		})
	}
}
