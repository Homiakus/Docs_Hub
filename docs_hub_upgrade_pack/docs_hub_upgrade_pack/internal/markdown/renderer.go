package markdown

import (
	"bytes"
	"html/template"
	"regexp"
	"sort"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

type Renderer struct {
	md     goldmark.Markdown
	policy *bluemonday.Policy
}

type WikiLink struct {
	Slug  string
	Alias string
	Raw   string
}

type Metadata struct {
	Tags      []string
	WikiLinks []WikiLink
}

func DefaultRenderer() *Renderer {
	policy := bluemonday.UGCPolicy()
	policy.AllowAttrs("class").OnElements("code", "pre", "span", "div")
	policy.AllowAttrs("id").OnElements("h1", "h2", "h3", "h4", "h5", "h6")

	return &Renderer{
		md: goldmark.New(
			goldmark.WithExtensions(
				extension.GFM,
				extension.Typographer,
			),
			goldmark.WithParserOptions(
				parser.WithAutoHeadingID(),
			),
			goldmark.WithRendererOptions(
				html.WithHardWraps(),
				html.WithXHTML(),
			),
		),
		policy: policy,
	}
}

func (r *Renderer) Render(source string) (template.HTML, error) {
	converted := ConvertWikiLinks(source)

	var buf bytes.Buffer
	if err := r.md.Convert([]byte(converted), &buf); err != nil {
		return "", err
	}

	safe := r.policy.Sanitize(buf.String())
	return template.HTML(safe), nil
}

var wikiLinkRE = regexp.MustCompile(`\[\[([^\]|]+)(?:\|([^\]]+))?\]\]`)
var tagRE = regexp.MustCompile(`(^|\s)#([\p{L}\p{N}_/-]{2,64})`)

func Extract(source string) Metadata {
	return Metadata{
		Tags:      ExtractTags(source),
		WikiLinks: ExtractWikiLinks(source),
	}
}

func ConvertWikiLinks(source string) string {
	return wikiLinkRE.ReplaceAllStringFunc(source, func(raw string) string {
		matches := wikiLinkRE.FindStringSubmatch(raw)
		if len(matches) < 2 {
			return raw
		}
		slug := strings.TrimSpace(matches[1])
		alias := slug
		if len(matches) >= 3 && strings.TrimSpace(matches[2]) != "" {
			alias = strings.TrimSpace(matches[2])
		}
		return "[" + alias + "](/article/" + Slugify(slug) + ")"
	})
}

func ExtractWikiLinks(source string) []WikiLink {
	matches := wikiLinkRE.FindAllStringSubmatch(source, -1)
	out := make([]WikiLink, 0, len(matches))
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		slug := Slugify(strings.TrimSpace(m[1]))
		alias := strings.TrimSpace(m[1])
		if len(m) >= 3 && strings.TrimSpace(m[2]) != "" {
			alias = strings.TrimSpace(m[2])
		}
		out = append(out, WikiLink{Slug: slug, Alias: alias, Raw: m[0]})
	}
	return out
}

func ExtractTags(source string) []string {
	matches := tagRE.FindAllStringSubmatch(source, -1)
	seen := map[string]struct{}{}
	for _, m := range matches {
		if len(m) < 3 {
			continue
		}
		tag := strings.ToLower(strings.Trim(m[2], "_-/"))
		if tag == "" {
			continue
		}
		seen[tag] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for tag := range seen {
		out = append(out, tag)
	}
	sort.Strings(out)
	return out
}

func Slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "-")
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || (r >= 'а' && r <= 'я') || r == 'ё'
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if r == '-' || r == '_' || r == '/' {
			if !lastDash {
				b.WriteRune('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
