package api

import (
	"reflect"
	"testing"
)

func TestParseItemsTextSupportsPlainLinesAndJSONL(t *testing.T) {
	items, err := parseItemsText(`
docs/a.md

{"name":"review b","target":"docs/b.md"}
not-json
`)
	if err != nil {
		t.Fatalf("parseItemsText: %v", err)
	}
	want := []string{
		"docs/a.md",
		`{"name":"review b","target":"docs/b.md"}`,
		"not-json",
	}
	if !reflect.DeepEqual(items, want) {
		t.Fatalf("items = %#v, want %#v", items, want)
	}
}

func TestNormalizeItemsPrefersExplicitItems(t *testing.T) {
	items, err := normalizeItems([]string{" a ", "", "b"}, "ignored")
	if err != nil {
		t.Fatalf("normalizeItems: %v", err)
	}
	want := []string{"a", "b"}
	if !reflect.DeepEqual(items, want) {
		t.Fatalf("items = %#v, want %#v", items, want)
	}
}
