package httpapp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/homiakus/docshub-next/internal/auth"
	"github.com/homiakus/docshub-next/internal/db"
)

// Helper to create a user in DB for testing authorization
func createTestUser(t *testing.T, database *db.DB, username, role string) int64 {
	t.Helper()
	hash, err := auth.HashPassword("user12345")
	if err != nil {
		t.Fatal(err)
	}
	res, err := database.ExecContext(context.Background(),
		`INSERT INTO users(username, display_name, password_hash, role, is_active, created_at, updated_at) VALUES(?,?,?,?,1,datetime('now'),datetime('now'))`,
		username, username, hash, role)
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// loginAs credentials helper
func loginAs(t *testing.T, client *http.Client, baseURL, username, password string, database *db.DB) string {
	t.Helper()
	res, err := client.PostForm(baseURL+"/login", url.Values{
		"username": {username},
		"password": {password},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("login as %s status = %d, want %d", username, res.StatusCode, http.StatusSeeOther)
	}
	urlParsed, _ := url.Parse(baseURL)
	for _, c := range client.Jar.Cookies(urlParsed) {
		if c.Name == "dh_session" {
			sid := strings.SplitN(c.Value, ".", 2)[0]
			var csrf string
			err := database.QueryRowContext(context.Background(), `SELECT csrf_token FROM sessions WHERE id=?`, sid).Scan(&csrf)
			if err != nil {
				t.Fatalf("query csrf_token for %s: %v", username, err)
			}
			return csrf
		}
	}
	t.Fatalf("dh_session cookie not found for %s", username)
	return ""
}

func TestBaselinePrivateGraphLeak(t *testing.T) {
	ts, client, database := newTestApp(t)
	defer ts.Close()

	// Admin logs in and creates a public and a private article
	adminCSRF := loginTestUser(t, client, ts.URL, database)
	saveArticle(t, client, ts.URL, url.Values{
		"slug":       {"public-doc"},
		"title":      {"Public Doc"},
		"visibility": {"authenticated"},
		"content":    {"Public content with [[private-doc]] link"},
	}, adminCSRF)

	saveArticle(t, client, ts.URL, url.Values{
		"slug":       {"private-doc"},
		"title":      {"Confidential Plan"},
		"visibility": {"private"},
		"content":    {"Private financial plan"},
	}, adminCSRF)

	// Create regular reader user
	createTestUser(t, database, "reader1", "reader")

	// Create secondary client for reader1
	readerClient, err := newTestClient()
	if err != nil {
		t.Fatal(err)
	}
	loginAs(t, readerClient, ts.URL, "reader1", "user12345", database)

	// Query /api/graph as reader1
	res, err := readerClient.Get(ts.URL + "/api/graph")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("graph status = %d, want 200", res.StatusCode)
	}

	body, _ := io.ReadAll(res.Body)
	var graph struct {
		Nodes []struct {
			ID    string `json:"id"`
			Label string `json:"label"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(body, &graph); err != nil {
		t.Fatalf("unmarshal graph json: %v", err)
	}

	// Verify that private-doc is NOT returned to regular user who lacks access
	for _, node := range graph.Nodes {
		if node.ID == "private-doc" || strings.Contains(node.Label, "Confidential Plan") {
			t.Fatalf("SECURITY VIOLATION: /api/graph leaked private article %q to user without access!", node.ID)
		}
	}
}

func TestBaselineEditAuthorizationBypass(t *testing.T) {
	ts, client, database := newTestApp(t)
	defer ts.Close()

	// Admin creates a private article
	adminCSRF := loginTestUser(t, client, ts.URL, database)
	saveArticle(t, client, ts.URL, url.Values{
		"slug":       {"admin-private-secret"},
		"title":      {"Admin Top Secret"},
		"visibility": {"private"},
		"content":    {"Secret data"},
	}, adminCSRF)

	// Fetch private article ID from DB
	var articleID int64
	err := database.QueryRowContext(context.Background(), `SELECT id FROM articles WHERE slug='admin-private-secret'`).Scan(&articleID)
	if err != nil {
		t.Fatal(err)
	}

	// Create another user with editor role
	createTestUser(t, database, "other_editor", "editor")

	editorClient, err := newTestClient()
	if err != nil {
		t.Fatal(err)
	}
	editorCSRF := loginAs(t, editorClient, ts.URL, "other_editor", "user12345", database)

	// other_editor tries to open /edit/admin-private-secret
	res, err := editorClient.Get(ts.URL + "/edit/admin-private-secret")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden && res.StatusCode != http.StatusNotFound {
		t.Fatalf("SECURITY VIOLATION: /edit/ returned status %d for unaccessible private document (expected 403 or 404)", res.StatusCode)
	}

	// other_editor tries to overwrite admin-private-secret via POST /save with id
	res, err = editorClient.PostForm(ts.URL+"/save", url.Values{
		"csrf_token": {editorCSRF},
		"id":         {strconv.FormatInt(articleID, 10)},
		"slug":       {"admin-private-secret"},
		"title":      {"Hacked Title"},
		"visibility": {"public"},
		"content":    {"Defaced content"},
	})
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	if res.StatusCode != http.StatusForbidden && res.StatusCode != http.StatusNotFound {
		t.Fatalf("SECURITY VIOLATION: POST /save returned status %d when attempting to edit unaccessible private document!", res.StatusCode)
	}
}

func newTestClient() (*http.Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &http.Client{
		Jar: jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}, nil
}
