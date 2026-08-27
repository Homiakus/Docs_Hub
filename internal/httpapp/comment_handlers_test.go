package httpapp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"
)

func TestCommentAPIIntegration(t *testing.T) {
	server, client, database := newTestApp(t)
	defer server.Close()

	csrf := loginTestUser(t, client, server.URL, database)

	// 1. Create an article
	articleURL := saveArticle(t, client, server.URL, url.Values{
		"slug":       {"annotation-guide"},
		"title":      {"Annotation Guide"},
		"visibility": {"authenticated"},
		"content":    {"This is the main text of the guide."},
	}, csrf)
	if articleURL == "" {
		t.Fatalf("failed to create article")
	}

	// 2. Query initial comments (empty)
	res, err := client.Get(server.URL + "/api/v1/documents/1/comments")
	if err != nil {
		t.Fatalf("get comments: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("get comments status=%d want %d", res.StatusCode, http.StatusOK)
	}

	// 3. Create a comment
	commentBody, _ := json.Marshal(map[string]any{
		"base_revision_id": 1,
		"start_offset":     0,
		"end_offset":       4,
		"quote_exact":      "This",
		"quote_prefix":     "",
		"quote_suffix":     " is the",
		"ast_node_kind":    "paragraph",
		"body":             "Is this word necessary?",
	})
	createReq, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/documents/1/comments", bytes.NewReader(commentBody))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("X-CSRF-Token", csrf)
	createRes, err := client.Do(createReq)
	if err != nil {
		t.Fatalf("create comment: %v", err)
	}
	defer createRes.Body.Close()
	if createRes.StatusCode != http.StatusCreated {
		t.Fatalf("create comment status=%d want %d", createRes.StatusCode, http.StatusCreated)
	}

	var created struct {
		Comment struct {
			ID   int64  `json:"id"`
			Body string `json:"body"`
		} `json:"comment"`
	}
	if err := json.NewDecoder(createRes.Body).Decode(&created); err != nil {
		t.Fatalf("decode create comment: %v", err)
	}
	if created.Comment.ID == 0 || created.Comment.Body != "Is this word necessary?" {
		t.Fatalf("unexpected comment payload: %+v", created)
	}

	// 4. Resolve comment
	resolveReq, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/v1/comments/%d/resolve", server.URL, created.Comment.ID), nil)
	resolveReq.Header.Set("X-CSRF-Token", csrf)
	resolveRes, err := client.Do(resolveReq)
	if err != nil {
		t.Fatalf("resolve comment: %v", err)
	}
	defer resolveRes.Body.Close()
	if resolveRes.StatusCode != http.StatusOK {
		t.Fatalf("resolve comment status=%d want %d", resolveRes.StatusCode, http.StatusOK)
	}

	// 5. Delete comment
	delReq, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/api/v1/comments/%d", server.URL, created.Comment.ID), nil)
	delReq.Header.Set("X-CSRF-Token", csrf)
	delRes, err := client.Do(delReq)
	if err != nil {
		t.Fatalf("delete comment: %v", err)
	}
	defer delRes.Body.Close()
	if delRes.StatusCode != http.StatusOK {
		t.Fatalf("delete comment status=%d want %d", delRes.StatusCode, http.StatusOK)
	}
}
