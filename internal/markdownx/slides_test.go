package markdownx

import (
	"testing"
)

func TestExtractSlidesWithExplicitSeparators(t *testing.T) {
	src := `
# Slide 1: Welcome

Introductory slide content.

:::notes
Remember to introduce the team!
:::

---

## Slide 2: Architecture

Key architecture principles.

---

## Slide 3: Conclusion

Summary and next steps.
`

	slides, err := ExtractSlides(src)
	if err != nil {
		t.Fatalf("ExtractSlides failed: %v", err)
	}

	if len(slides) != 3 {
		t.Fatalf("expected 3 slides, got %d", len(slides))
	}

	if slides[0].Notes != "Remember to introduce the team!" {
		t.Errorf("slide 1 notes mismatch: %q", slides[0].Notes)
	}

	if slides[1].Title != "Slide 2: Architecture" {
		t.Errorf("slide 2 title mismatch: %q", slides[1].Title)
	}
}

func TestExtractSlidesWithHeadingSplitting(t *testing.T) {
	src := `
# Intro
Beginning content.

## Section 1
Details 1.

## Section 2
Details 2.
`

	slides, err := ExtractSlides(src)
	if err != nil {
		t.Fatalf("ExtractSlides failed: %v", err)
	}

	if len(slides) != 3 {
		t.Fatalf("expected 3 slides by heading splitting, got %d", len(slides))
	}
}
