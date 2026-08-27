package markdownx

import (
	"html/template"
	"regexp"
	"strings"
)

type Slide struct {
	Index   int
	Title   string
	Content template.HTML
	Notes   string
}

var (
	notesRe = regexp.MustCompile(`(?s):::notes\s*([\s\S]*?)\s*:::`)
	slideRe = regexp.MustCompile(`(?m)^---\s*$`)
)

func ExtractSlides(source string) ([]Slide, error) {
	chunks := slideRe.Split(source, -1)
	var rawSlides []string

	if len(chunks) > 1 {
		for _, c := range chunks {
			trimmed := strings.TrimSpace(c)
			if trimmed != "" {
				rawSlides = append(rawSlides, trimmed)
			}
		}
	} else {
		// Single chunk or fallback: split by top-level / h2 headings
		h2Re := regexp.MustCompile(`(?m)^(?:##\s+.*)`)
		locs := h2Re.FindAllStringIndex(source, -1)
		if len(locs) > 0 {
			start := 0
			for i := 0; i < len(locs); i++ {
				if locs[i][0] > start {
					chunk := strings.TrimSpace(source[start:locs[i][0]])
					if chunk != "" {
						rawSlides = append(rawSlides, chunk)
					}
				}
				start = locs[i][0]
			}
			if start < len(source) {
				chunk := strings.TrimSpace(source[start:])
				if chunk != "" {
					rawSlides = append(rawSlides, chunk)
				}
			}
		} else {
			rawSlides = append(rawSlides, strings.TrimSpace(source))
		}
	}

	var slides []Slide
	for i, raw := range rawSlides {
		notes := ""
		if m := notesRe.FindStringSubmatch(raw); len(m) > 1 {
			notes = strings.TrimSpace(m[1])
			raw = notesRe.ReplaceAllString(raw, "")
		}

		res, err := Render(raw)
		if err != nil {
			return nil, err
		}

		title := "Slide"
		if len(res.Headings) > 0 {
			title = res.Headings[0].Text
		}

		slides = append(slides, Slide{
			Index:   i + 1,
			Title:   title,
			Content: template.HTML(res.HTML),
			Notes:   notes,
		})
	}

	return slides, nil
}
