package main

import (
	"net/url"
	"testing"
)

func TestPhase3TableTargetURLReplacesStaleViewAndTable(t *testing.T) {
	target, err := phase3TableTargetURL("https://shimo.net/tables/file?table=old&view=old-view&foo=bar", "new-table")
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}
	if u.Query().Get("table") != "new-table" || u.Query().Get("view") != "" || u.Query().Get("foo") != "bar" || u.Path != "/tables/file" {
		t.Fatal("capture URL not bound to requested table", target)
	}
}
