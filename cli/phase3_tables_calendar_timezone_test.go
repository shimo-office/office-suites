package main

import (
	"os/exec"
	"testing"
	"time"
)

func TestPhase3CalendarNativeTimezoneFallbackAndFixedOffset(t *testing.T) {
	for _, tc := range []struct{ document, browser, instant, day, source string }{
		{"UTC", "Asia/Shanghai", "2026-11-19T17:00:00Z", "2026-11-20", "browser-fallback"},
		{"America/New_York", "Asia/Shanghai", "2026-07-01T04:30:00Z", "2026-06-30", "native-fixed-offset"},
		{"Asia/Shanghai", "America/New_York", "2026-11-19T17:00:00Z", "2026-11-20", "native-fixed-offset"},
	} {
		zone, context, err := phase3CalendarDisplayZone(tc.document, tc.browser)
		if err != nil {
			t.Fatal(err)
		}
		instant, _ := time.Parse(time.RFC3339, tc.instant)
		if instant.In(zone).Format("2006-01-02") != tc.day || context["calendarTimezoneSource"] != tc.source {
			t.Fatal("native timezone semantics lost", tc, context)
		}
		if context["documentTimezone"] != tc.document || context["browserTimezone"] != tc.browser {
			t.Fatal("raw timezone binding lost")
		}
	}
	for _, pair := range [][2]string{{"UTC", ""}, {"UTC", "unrecognized"}, {"", "Asia/Shanghai"}} {
		if _, _, err := phase3CalendarDisplayZone(pair[0], pair[1]); err == nil {
			t.Fatal("unknown timezone accepted", pair)
		}
	}
}

func TestPhase3CalendarProofBindsRawAndBrowserTimezone(t *testing.T) {
	spec := map[string]any{"tableGuid": "t1", "viewGuid": "v1", "viewType": "Calendar", "documentTimezone": "UTC", "browserTimezone": "Asia/Shanghai", "expectedRecords": map[string]any{"r1": map[string]any{"title": "A"}}}
	actual := phase3TableCopy(spec)
	actual["timeOrigin"] = float64(2)
	actual["visibleNativeNodes"] = float64(1)
	actual["renderedRecords"] = actual["expectedRecords"]
	if err := verifyPhase3NativeViewProof(actual, spec, 1); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"documentTimezone", "browserTimezone"} {
		bad := phase3TableCopy(actual)
		bad[key] = "changed"
		if verifyPhase3NativeViewProof(bad, spec, 1) == nil {
			t.Fatal("wrong calendar environment accepted", key)
		}
	}
}

func TestPhase3CalendarReadsTimezoneFromBoundModel(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node unavailable")
	}
	script := `global.window=global;global.innerWidth=1200;global.innerHeight=1200;global.performance={timeOrigin:9};global.getComputedStyle=()=>({visibility:'visible',display:'block',overflowX:'visible',overflowY:'visible'});const rect=()=>({left:0,top:0,right:600,bottom:500,width:600,height:500});const view={getGuid:()=> 'calendar1',getType:()=> 'Calendar',getTable:()=>({getGuid:()=> 'table1',getBase:()=>({getDocumentTimezone:()=> 'UTC'})})};const task={guid:'r1',title:'P3-A',record:{getGuid:()=> 'r1',getValue:k=>k==='start'?46345.375:46345.70833333333},originStartDate:new Date(2026,10,19),originEndDate:new Date(2026,10,20)};const el={getBoundingClientRect:rect,parentElement:null,closest:()=>({getAttribute:()=> 'r1'}),innerText:'P3-A',__reactFiber$test:{memoizedProps:{task,viewModel:view},return:null}};global.document={querySelectorAll:()=>[el]};const read=` + phase3ReadNativeView + `;const result=read({tableGuid:'table1',viewGuid:'calendar1',viewType:'Calendar',startFieldGuid:'start',endFieldGuid:'end',documentTimezone:'fake-spec-zone',browserTimezone:'fake-browser'},'tasks');if(result.documentTimezone!=='UTC'||result.browserTimezone!==Intl.DateTimeFormat().resolvedOptions().timeZone||result.renderedRecords.r1.startDay!=='2026-11-19')throw Error(JSON.stringify(result));`
	if out, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}

func TestPhase3CalendarNative999MillisecondsCarryAcrossMidnight(t *testing.T) {
	table := tableFixtureForEdit()
	view := phase3TableCopy(phase3TableMap(table, "views")["v1"])
	view["type"] = "Calendar"
	view["calendar"] = map[string]any{"startFieldGuid": "start", "endFieldGuid": "end"}
	for _, row := range phase3TableMap(table, "records") {
		row["valueMap"].(map[string]any)["start"] = 46345.666666655095
		row["valueMap"].(map[string]any)["end"] = 46345.666666655095
	}
	spec, err := phase3ViewRecordSpec(table, view, "UTC", "Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range spec["expectedRecords"].(map[string]any) {
		row := raw.(map[string]any)
		if row["startDay"] != "2026-11-20" || row["endDay"] != "2026-11-20" {
			t.Fatal("native 999ms carry lost", row)
		}
	}
}
