package main

import "testing"

func TestSubscriptionType(t *testing.T) {
	for _, typ := range []string{"document", "documentPro", "spreadsheet"} {
		if !subscriptionType(typ) {
			t.Errorf("%s should be eligible", typ)
		}
	}
	for _, typ := range []string{"presentation", "folder", "cloud"} {
		if subscriptionType(typ) {
			t.Errorf("%s should not be eligible", typ)
		}
	}
}

func TestSelectSubscription(t *testing.T) {
	files := []map[string]any{{"guid": "one", "name": "第一"}, {"guid": "two", "name": "第二"}}
	got, err := selectSubscription(files, "2")
	if err != nil || str(got, "guid") != "two" {
		t.Fatalf("select by index = %#v, %v", got, err)
	}
	got, err = selectSubscription(files, "one")
	if err != nil || str(got, "name") != "第一" {
		t.Fatalf("select by guid = %#v, %v", got, err)
	}
	if _, err := selectSubscription(files, "3"); err == nil {
		t.Fatal("missing selection should fail")
	}
}
