package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBootstrapFilesAreBoundedAndDoNotExposeContents(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "bootstrap.json")
	secret := []byte(`{"unknown":"secret-that-must-not-be-logged"}`)
	if e := os.WriteFile(p, secret, 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := readBounded(p, 4); e == nil {
		t.Fatal("oversized bootstrap accepted")
	}
	if _, e := readBounded(dir, 16384); e == nil {
		t.Fatal("directory accepted")
	}
	var cfg bootstrap
	if e := decodeFile(p, &cfg); e == nil || e.Error() != "invalid bootstrap configuration" {
		t.Fatal("unexpected configuration failure", e)
	}
	if e := os.WriteFile(p, []byte(`{} {}`), 0600); e != nil {
		t.Fatal(e)
	}
	if e := decodeFile(p, &cfg); e == nil {
		t.Fatal("trailing configuration accepted")
	}
}
