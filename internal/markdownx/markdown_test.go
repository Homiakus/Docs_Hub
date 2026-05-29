package markdownx

import "testing"

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
