package markdownx

import (
	"bytes"
	"regexp"
	"sort"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

type Result struct {
	HTML  string
	Tags  []string
	Links []WikiLink
}

type WikiLink struct {
	Slug  string
	Label string
}

var (
	tagRe  = regexp.MustCompile(`(^|\s)#([\p{L}\p{N}_\-/]+)`)
	linkRe = regexp.MustCompile(`\[\[([^\]|#]+)(?:#[^\]|]+)?(?:\|([^\]]+))?\]\]`)
)

func Render(source string) (Result, error) {
	prepared := ReplaceWikiLinks(source)
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM, extension.Typographer, highlighting.NewHighlighting()),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		goldmark.WithRendererOptions(html.WithUnsafe()),
	)
	var buf bytes.Buffer
	if err := md.Convert([]byte(prepared), &buf); err != nil {
		return Result{}, err
	}
	policy := bluemonday.UGCPolicy()
	policy.AllowRelativeURLs(true)
	policy.AllowAttrs("class").OnElements("code", "pre", "span", "div")
	policy.AllowAttrs("data-slug").OnElements("a")
	policy.AllowAttrs("target", "rel").OnElements("a")
	policy.AllowElements("img", "audio", "video", "source")
	policy.AllowAttrs("src", "alt", "title", "loading").OnElements("img")
	policy.AllowAttrs("src", "title", "controls", "preload").OnElements("audio", "video")
	policy.AllowAttrs("poster").OnElements("video")
	policy.AllowAttrs("src", "type").OnElements("source")
	return Result{HTML: policy.Sanitize(buf.String()), Tags: ExtractTags(source), Links: ExtractWikiLinks(source)}, nil
}

func ReplaceWikiLinks(s string) string {
	return linkRe.ReplaceAllStringFunc(s, func(raw string) string {
		m := linkRe.FindStringSubmatch(raw)
		if len(m) == 0 {
			return raw
		}
		slug := Slugify(m[1])
		label := strings.TrimSpace(m[2])
		if label == "" {
			label = strings.TrimSpace(m[1])
		}
		return "[" + label + "](/a/" + slug + ")"
	})
}

func ExtractTags(s string) []string {
	seen := map[string]struct{}{}
	for _, m := range tagRe.FindAllStringSubmatch(s, -1) {
		name := strings.ToLower(strings.Trim(m[2], "-_/"))
		if name != "" {
			seen[name] = struct{}{}
		}
	}
	return keys(seen)
}

func ExtractWikiLinks(s string) []WikiLink {
	seen := map[string]WikiLink{}
	for _, m := range linkRe.FindAllStringSubmatch(s, -1) {
		slug := Slugify(m[1])
		label := strings.TrimSpace(m[2])
		if slug != "" {
			seen[slug+"\x00"+label] = WikiLink{Slug: slug, Label: label}
		}
	}
	out := make([]WikiLink, 0, len(seen))
	for _, v := range seen {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out
}

func Slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "-")
	s = regexp.MustCompile(`[^\p{L}\p{N}_\-]+`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`-+`).ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

func keys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
