package markdown

import (
	"strings"
	"testing"
)

func TestRenderSanitizesHTML(t *testing.T) {
	r := DefaultRenderer()
	html, err := r.Render(`# Hello

<script>alert(1)</script>

[[Start|Home]] #Onboarding`)
	if err != nil {
		t.Fatal(err)
	}
	out := string(html)
	if strings.Contains(out, "<script") {
		t.Fatalf("expected script to be removed, got: %s", out)
	}
	if !strings.Contains(out, `/article/start`) {
		t.Fatalf("expected wiki link conversion, got: %s", out)
	}
}

func TestExtractMetadata(t *testing.T) {
	meta := Extract(`See [[Start|Home]] and [[Runbook]]. #Ops #ops #onboarding`)
	if len(meta.WikiLinks) != 2 {
		t.Fatalf("expected 2 links, got %d", len(meta.WikiLinks))
	}
	if len(meta.Tags) != 2 {
		t.Fatalf("expected deduplicated tags, got %#v", meta.Tags)
	}
}

func TestSlugifyRussian(t *testing.T) {
	got := Slugify("База Знаний / Старт")
	want := "база-знаний-старт"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
