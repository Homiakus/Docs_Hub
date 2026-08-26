package httpapp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/homiakus/docshub-next/internal/searchquality"
)

type searchSuggestionResponse struct {
	Suggestions []struct {
		Title string `json:"title"`
		Slug  string `json:"slug"`
	} `json:"suggestions"`
}

func fetchSuggestionSlugs(t *testing.T, client *http.Client, baseURL, query string) []string {
	t.Helper()
	res, err := client.Get(baseURL + "/api/v1/search/suggest?q=" + url.QueryEscape(query))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("suggest status=%d body=%s", res.StatusCode, body)
	}
	var payload searchSuggestionResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode suggestions: %v", err)
	}
	out := make([]string, 0, len(payload.Suggestions))
	for _, suggestion := range payload.Suggestions {
		out = append(out, suggestion.Slug)
	}
	return out
}

func TestSearchSuggestBaselineRelevanceCorpus(t *testing.T) {
	ts, client, database := newTestApp(t)
	defer ts.Close()
	csrf := loginTestUser(t, client, ts.URL, database)

	docs := []struct {
		slug, title, content string
	}{
		{"hydraulic-pump-calibration", "Hydraulic Pump Calibration", "Hydraulic pressure calibration procedure and pump verification."},
		{"cytology-cell-segmentation", "Cytology Cell Segmentation", "Cytology image segmentation for nuclei and cytoplasm."},
		{"backup-power-design", "Backup Power Design", "Backup battery sizing and power supply architecture."},
	}
	for _, doc := range docs {
		saveArticle(t, client, ts.URL, url.Values{
			"slug": {doc.slug}, "title": {doc.title}, "content": {doc.content}, "visibility": {"authenticated"},
		}, csrf)
		if _, err := database.ExecContext(context.Background(), `UPDATE articles SET status='published' WHERE slug=?`, doc.slug); err != nil {
			t.Fatal(err)
		}
	}

	queries := []struct {
		q, want string
	}{
		{"hydraulic", "hydraulic-pump-calibration"},
		{"cytology", "cytology-cell-segmentation"},
		{"backup", "backup-power-design"},
	}
	results := make([][]string, 0, len(queries))
	relevant := make([]map[string]struct{}, 0, len(queries))
	grades := make([]map[string]int, 0, len(queries))
	for _, query := range queries {
		results = append(results, fetchSuggestionSlugs(t, client, ts.URL, query.q))
		relevant = append(relevant, map[string]struct{}{query.want: {}})
		grades = append(grades, map[string]int{query.want: 3})
	}

	mrr := searchquality.MRRAtK(results, relevant, 8)
	recall := searchquality.RecallAtK(results, relevant, 8)
	ndcg := searchquality.NDCGAtK(results, grades, 8)
	if mrr < 0.80 {
		t.Fatalf("baseline MRR@8=%0.3f below 0.80; results=%v", mrr, results)
	}
	if recall != 1.0 {
		t.Fatalf("baseline Recall@8=%0.3f want 1.0; results=%v", recall, results)
	}
	if ndcg < 0.80 {
		t.Fatalf("baseline nDCG@8=%0.3f below 0.80; results=%v", ndcg, results)
	}
}

func TestSearchSuggestDoesNotLeakPublishedPrivateDocumentMetadata(t *testing.T) {
	ts, adminClient, database := newTestApp(t)
	defer ts.Close()
	csrf := loginTestUser(t, adminClient, ts.URL, database)

	saveArticle(t, adminClient, ts.URL, url.Values{
		"slug": {"visible-thermal-guide"}, "title": {"Visible Thermal Guide"},
		"visibility": {"authenticated"}, "content": {"thermal public baseline"},
	}, csrf)
	saveArticle(t, adminClient, ts.URL, url.Values{
		"slug": {"private-thermal-acquisition"}, "title": {"Private Thermal Acquisition"},
		"visibility": {"private"}, "content": {"thermal acquisition confidential"},
	}, csrf)
	if _, err := database.ExecContext(context.Background(), `UPDATE articles SET status='published' WHERE slug IN ('visible-thermal-guide','private-thermal-acquisition')`); err != nil {
		t.Fatal(err)
	}

	createTestUser(t, database, "suggest_reader", "reader")
	readerClient, err := newTestClient()
	if err != nil { t.Fatal(err) }
	loginAs(t, readerClient, ts.URL, "suggest_reader", "user12345", database)

	slugs := fetchSuggestionSlugs(t, readerClient, ts.URL, "thermal")
	joined := strings.Join(slugs, "\n")
	if !strings.Contains(joined, "visible-thermal-guide") {
		t.Fatalf("authorized published document missing from suggestions: %v", slugs)
	}
	if strings.Contains(joined, "private-thermal-acquisition") {
		t.Fatalf("SECURITY VIOLATION: published private document leaked through suggestions: %v", slugs)
	}
}
