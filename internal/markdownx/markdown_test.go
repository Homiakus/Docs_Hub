package markdownx

import (
	"strings"
	"testing"
)

func TestRenderMermaidBlock(t *testing.T) {
	src := "# Title\n\n```mermaid\ngraph TD\n  A --> B\n```\n\nEnd."
	res, err := Render(src)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Mermaid {
		t.Error("expected HasMermaid=true")
	}
	if res.HTML == "" {
		t.Error("expected non-empty HTML")
	}
}

func TestRenderExtractsHeadings(t *testing.T) {
	src := "## Introduction\n\n### Details\n\nContent here.\n\n## Summary"
	res, err := Render(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Headings) < 2 {
		t.Errorf("expected at least 2 headings, got %d: %+v", len(res.Headings), res.Headings)
	}
	for _, h := range res.Headings {
		if h.Text == "" {
			t.Error("heading has empty text")
		}
	}
}

func TestRenderExtractsTags(t *testing.T) {
	src := "Let's talk about #golang and #testing in this article."
	res, err := Render(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Tags) < 2 {
		t.Errorf("expected at least 2 tags, got %d: %v", len(res.Tags), res.Tags)
	}
}

func TestRenderWikiLinks(t *testing.T) {
	src := "See [[Getting Started]] and [[API|the API docs]] for more."
	res, err := Render(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Links) < 2 {
		t.Errorf("expected at least 2 links, got %d: %+v", len(res.Links), res.Links)
	}
}

func TestRenderWikiLinksWithAnchor(t *testing.T) {
	src := "See [[Getting Started#Installation]] and [[API#auth|auth docs]]."
	res, err := Render(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Links) < 2 {
		t.Errorf("expected at least 2 links, got %d: %+v", len(res.Links), res.Links)
	}
	// First link should have anchor preserved
	if !strings.Contains(res.HTML, `/a/getting-started#Installation`) {
		t.Errorf("expected anchor #Installation in link, got HTML: %s", res.HTML)
	}
	// Second link should have alias and anchor
	if !strings.Contains(res.HTML, `>auth docs</a>`) && !strings.Contains(res.HTML, `auth docs`) {
		t.Errorf("expected label 'auth docs', got HTML: %s", res.HTML)
	}
}

func TestSlugify(t *testing.T) {
	tests := []struct{ in, out string }{
		{"Hello World", "hello-world"},
		{"Русский Текст", "русский-текст"},
		{"  spaces  ", "spaces"},
		{"special!@#chars", "specialchars"},
	}
	for _, tc := range tests {
		got := Slugify(tc.in)
		if got != tc.out {
			t.Errorf("Slugify(%q) = %q, want %q", tc.in, got, tc.out)
		}
	}
}

func TestRenderAutoTraceBlock(t *testing.T) {
	src := "# Architecture Diagram\n\n```autotrace\nversion: 1\nflow: left-to-right\nnodes:\n  - id: sensor\n    title: Sensor Unit\n    outputs: [data]\n  - id: processor\n    title: Processor\n    inputs: [data]\nedges:\n  - from: sensor.data\n    to: processor.data\n    label: SPI Bus\n```\n\nDiagram rendered successfully."
	res, err := Render(src)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if !res.HasAutoTrace {
		t.Errorf("expected HasAutoTrace=true")
	}
	if !strings.Contains(res.HTML, "autotrace-container") || !strings.Contains(res.HTML, "Sensor Unit") || !strings.Contains(res.HTML, "SPI Bus") {
		t.Errorf("rendered HTML missing AutoTrace diagram components: %s", res.HTML)
	}
}

