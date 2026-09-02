package httpapp

import (
	"bytes"
	"context"
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

func TestCommentMutationCannotCrossOrganizationBoundary(t *testing.T) {
	server, client, database := newTestApp(t)
	defer server.Close()

	csrf := loginTestUser(t, client, server.URL, database)
	ctx := context.Background()

	var adminID int64
	if err := database.QueryRowContext(ctx, `SELECT id FROM users WHERE username = 'admin'`).Scan(&adminID); err != nil {
		t.Fatalf("lookup admin: %v", err)
	}

	// A second tenant makes the compatibility principal resolver require an
	// explicit organization membership. Give the authenticated admin exactly
	// one membership (organization 1), then place the target comment in org 2.
	if _, err := database.ExecContext(ctx, `
		INSERT INTO organizations(id, name, slug, settings_json, created_at)
		VALUES(2, 'Other Organization', 'other-org', '{}', datetime('now'));
		INSERT INTO role_bindings(organization_id, scope_type, scope_id, subject_type, subject_id, role, created_at)
		VALUES(1, 'organization', 1, 'user', ?, 'admin', datetime('now'));
		INSERT INTO spaces(id, organization_id, parent_id, name, slug, description, default_visibility, created_at, updated_at)
		VALUES(2, 2, NULL, 'Other Space', 'other-space', '', 'space_members', datetime('now'), datetime('now'));
		INSERT INTO articles(id, organization_id, space_id, stable_key, slug, title, content, rendered_html, visibility, owner_id, status, created_at, updated_at)
		VALUES(200, 2, 2, 'foreign-article', 'foreign-article', 'Foreign Article', 'secret', '<p>secret</p>', 'authenticated', ?, 'published', datetime('now'), datetime('now'));
		INSERT INTO comments(id, document_id, author_id, body, status, created_at, updated_at)
		VALUES(300, 200, ?, 'foreign comment', 'open', datetime('now'), datetime('now'));
	`, adminID, adminID, adminID); err != nil {
		t.Fatalf("seed cross-organization fixture: %v", err)
	}

	resolveReq, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/comments/300/resolve", nil)
	if err != nil {
		t.Fatal(err)
	}
	resolveReq.Header.Set("X-CSRF-Token", csrf)
	resolveRes, err := client.Do(resolveReq)
	if err != nil {
		t.Fatalf("resolve foreign comment: %v", err)
	}
	resolveRes.Body.Close()
	if resolveRes.StatusCode != http.StatusNotFound {
		t.Fatalf("resolve foreign comment status=%d want %d", resolveRes.StatusCode, http.StatusNotFound)
	}

	var status string
	if err := database.QueryRowContext(ctx, `SELECT status FROM comments WHERE id = 300`).Scan(&status); err != nil {
		t.Fatalf("read foreign comment after resolve attempt: %v", err)
	}
	if status != "open" {
		t.Fatalf("foreign comment status=%q want open", status)
	}

	deleteReq, err := http.NewRequest(http.MethodDelete, server.URL+"/api/v1/comments/300", nil)
	if err != nil {
		t.Fatal(err)
	}
	deleteReq.Header.Set("X-CSRF-Token", csrf)
	deleteRes, err := client.Do(deleteReq)
	if err != nil {
		t.Fatalf("delete foreign comment: %v", err)
	}
	deleteRes.Body.Close()
	if deleteRes.StatusCode != http.StatusNotFound {
		t.Fatalf("delete foreign comment status=%d want %d", deleteRes.StatusCode, http.StatusNotFound)
	}

	var count int
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM comments WHERE id = 300 AND deleted_at IS NULL`).Scan(&count); err != nil {
		t.Fatalf("verify foreign comment retained: %v", err)
	}
	if count != 1 {
		t.Fatalf("foreign comment was mutated or deleted, count=%d", count)
	}
}
