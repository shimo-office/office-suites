package main

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

// The Range stand-in deliberately uses UTF-16 offsets, as the browser does.
// Execute the production scripts rather than checking for source substrings.
func runLightDocUnicodeScript(t *testing.T, chunks []string, expression, assertion string) {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("existing Node for browser selection execution")
	}
	raw, err := json.Marshal(chunks)
	if err != nil {
		t.Fatal(err)
	}
	script := `const chunks=` + string(raw) + `;
const nodes=chunks.map(textContent=>({textContent}));
const text=chunks.join('');
const p={innerText:text,getAttribute:()=>null};
const ce={querySelectorAll:()=>[p],focus:()=>{}};
const absolute=(node,offset)=>{
  if(offset<0||offset>node.textContent.length)throw Error('invalid DOM offset');
  return nodes.slice(0,nodes.indexOf(node)).reduce((n,it)=>n+it.textContent.length,0)+offset;
};
const range={setStart:(node,offset)=>range.start=absolute(node,offset),
  setEnd:(node,offset)=>range.end=absolute(node,offset),collapse:()=>range.end=range.start};
const selection={removeAllRanges:()=>{},addRange:()=>{},toString:()=>text.slice(range.start,range.end)};
global.NodeFilter={SHOW_TEXT:4};
global.window={getSelection:()=>selection};
global.document={querySelector:()=>ce,createTreeWalker:()=>{let i=0;return {nextNode:()=>nodes[i++]||null}},
  createRange:()=>range,dispatchEvent:()=>{}};
const result=` + expression + `;
if(result.ok!==true)throw Error(JSON.stringify(result));
` + assertion
	if out, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("generated selection script failed: %v\n%s", err, out)
	}
}

func TestLightDocUnicodeSelection(t *testing.T) {
	for _, tc := range []struct {
		name   string
		chunks []string
		start  int
		end    int
		want   string
	}{
		{"bmp", []string{"中文AB"}, 1, 3, "文A"},
		{"whole_emoji_paragraph", []string{"A😀B"}, 0, 3, "A😀B"},
		{"after_emoji", []string{"A😀B"}, 2, 3, "B"},
		{"emoji_only", []string{"A😀B"}, 1, 2, "😀"},
		{"non_bmp_han", []string{"𠮷野家"}, 0, 2, "𠮷野"},
		{"across_nodes", []string{"A😀", "B𠮷", "C"}, 1, 4, "😀B𠮷"},
		{"node_boundary", []string{"😀", "中文"}, 1, 3, "中文"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want, _ := json.Marshal(tc.want)
			runLightDocUnicodeScript(t, tc.chunks,
				lightDocSelectJS(1, strings.Join(tc.chunks, ""), tc.start, tc.end),
				`if(selection.toString()!==`+string(want)+`)throw Error('wrong selection: '+JSON.stringify(result));`)
		})
	}
}

func TestLightDocUnicodeInsertionCursor(t *testing.T) {
	for _, chunks := range [][]string{{"中文"}, {"A😀B"}, {"😀"}, {"前缀", "𠮷😀末尾"}} {
		for _, position := range []string{"before", "after"} {
			t.Run(strings.Join(chunks, "_")+"/"+position, func(t *testing.T) {
				want := "0"
				if position == "after" {
					want = "text.length"
				}
				runLightDocUnicodeScript(t, chunks,
					lightDocCursorJS(1, strings.Join(chunks, ""), position),
					`if(range.start!==`+want+`||range.end!==range.start)throw Error('wrong cursor: '+JSON.stringify(range));`)
			})
		}
	}
}
