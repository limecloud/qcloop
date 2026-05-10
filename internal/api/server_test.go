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

func TestNormalizeTemplatePayloadUsesJobRules(t *testing.T) {
	retries := 2
	req := templatePayload{
		Name:               "  文档 review 模板  ",
		Description:        "  睡前批处理  ",
		PromptTemplate:     "  review {{item}}  ",
		MaxQCRounds:        0,
		MaxExecutorRetries: &retries,
		ItemsText:          "  docs/a.md\n  ",
	}

	if err := normalizeTemplatePayload(&req, "codex"); err != nil {
		t.Fatalf("normalizeTemplatePayload: %v", err)
	}
	if req.Name != "文档 review 模板" || req.Description != "睡前批处理" {
		t.Fatalf("trimmed fields = name:%q desc:%q", req.Name, req.Description)
	}
	if req.MaxQCRounds != 3 || req.ExecutionMode != "standard" || req.ExecutorProvider != "codex" {
		t.Fatalf("defaults = rounds:%d mode:%q provider:%q", req.MaxQCRounds, req.ExecutionMode, req.ExecutorProvider)
	}
	if req.ItemsText != "docs/a.md" || req.MaxExecutorRetries == nil || *req.MaxExecutorRetries != 2 {
		t.Fatalf("items/retries = %q/%v", req.ItemsText, req.MaxExecutorRetries)
	}
}

func TestNormalizeTemplatePayloadRejectsInvalidExecutorRetries(t *testing.T) {
	retries := 6
	req := templatePayload{
		Name:               "bad",
		PromptTemplate:     "do {{item}}",
		MaxExecutorRetries: &retries,
	}
	if err := normalizeTemplatePayload(&req, "codex"); err == nil {
		t.Fatal("want max_executor_retries validation error")
	}
}
