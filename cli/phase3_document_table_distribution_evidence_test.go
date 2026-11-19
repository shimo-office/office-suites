package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPhase3DocumentDistributionExistingArtifacts(t *testing.T) {
	source := os.Getenv("SHIMO_PHASE3_DOC_DISTRIBUTION_EVIDENCE")
	if source == "" {
		t.Skip("explicit local evidence required")
	}
	beforeFiles, err := filepath.Glob(filepath.Join(source, "before", "*.docx"))
	if err != nil || len(beforeFiles) != 1 {
		t.Fatal("one before artifact required", err)
	}
	afterFiles, err := filepath.Glob(filepath.Join(source, "after", "*.docx"))
	if err != nil || len(afterFiles) != 1 {
		t.Fatal("one after artifact required", err)
	}
	before, err := readDocumentTables(beforeFiles[0])
	if err != nil {
		t.Fatal(err)
	}
	after, err := readDocumentTables(afterFiles[0])
	if err != nil {
		t.Fatal(err)
	}
	spec := documentTableSpec{Operation: "distribute-columns", Table: 1, Row: 1, Column: 1, ExpectedCell: "P3_A1"}
	if err = verifyDocumentTable(before, after, spec); err != nil {
		t.Fatal(err)
	}
	t.Log("original before/after artifacts pass bounded verifier; no network or mutation; fresh reload still required")
}
