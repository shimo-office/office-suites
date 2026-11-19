package main

import (
	"os/exec"
	"testing"
)

func TestPhase3FormReadyPublishedResults(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node unavailable")
	}
	setup := `global.window={modoc:{Delta:function(){}},logged:{edit:{query:{getForm:()=>({title:'测试表单',questions:[{subject:'题目一'}]}),getRouterPage:()=> 'result'}}}};global.document={body:{innerText:'测试表单\n结果统计\n数据汇总\n暂无回复'}};`
	script := setup + `if(!(` + phase3FormReady + `))throw new Error('rendered compact result page rejected');document.body.innerText='测试表单';if(` + phase3FormReady + `)throw new Error('loading shell accepted');`
	if out, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
}

func TestPhase3FormReadyReadsRenderedInputValues(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node unavailable")
	}
	setup := `global.window={modoc:{Delta:function(){}},logged:{edit:{query:{getForm:()=>({title:'测试表单',questions:[{subject:'题目一'}]}),getRouterPage:()=> 'questions'}}}};global.document={body:{innerText:'测试表单\n通用题型'},querySelectorAll:()=>[{value:'题目一',getClientRects:()=>[{width:100,height:20}]}]};`
	script := setup + `if(!(` + phase3FormReady + `))throw new Error('rendered question input rejected');document.querySelectorAll=()=>[];if(` + phase3FormReady + `)throw new Error('title shell accepted before question renderer');`
	if out, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
}

func TestPhase3FormFillUsesActualLoggerNamespace(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node unavailable")
	}
	setup := `global.window={logged:{"fill:query":{getForm:()=>({title:"published"}),getVisibleQuestions:()=>[{guid:"first"}]},"fill:command":{updateResponse:r=>r}}};`
	script := setup + `const q=` + phase3FormFillQueryJS + `,c=` + phase3FormFillCommandJS + `;if(q?.getForm()?.title!=="published"||q.getVisibleQuestions()[0].guid!=="first"||c?.updateResponse({guid:"first"}).guid!=="first")throw Error("native fill logger methods unavailable");delete window.logged;if((` + phase3FormFillQueryJS + `)!==undefined)throw Error("missing logger must stay unavailable");`
	if out, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
}
