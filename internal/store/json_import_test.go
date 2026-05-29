package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/homiakus/docshub-next/internal/db"
)

func TestImportJSONImportsArticlesMetadataAndFiles(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	database, err := db.Open(ctx, filepath.Join(dir, "docshub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC)
	src := JSONDatabase{
		Users: map[string]*JSONUser{
			"u1": {ID: "u1", Username: "legacy-admin", PasswordHash: "legacy-hash", Role: "admin", Active: true, CreatedAt: now, UpdatedAt: now},
		},
		Groups: map[string]*JSONGroup{
			"g1": {ID: "g1", Name: "Docs", MemberIDs: []string{"u1"}, CreatedAt: now, UpdatedAt: now},
		},
		Articles: map[string]*JSONArticle{
			"a1": {
				ID:              "a1",
				Title:           "Русская статья",
				Content:         "# Привет\n\n[[Runbook|гайд]] #Ops\n<script>alert(1)</script>",
				Tags:            []string{"imported"},
				AllUsers:        true,
				AllowedUserIDs:  []string{"u1"},
				AllowedGroupIDs: []string{"g1"},
				OwnerID:         "u1",
				CreatedAt:       now,
				UpdatedAt:       now,
			},
		},
		Attachments: map[string]*JSONFile{
			"f1": {ID: "f1", ArticleID: "a1", StoredName: "legacy/f1.pdf", OriginalName: "Runbook.pdf", MIME: "application/pdf", Size: 42, UploadedBy: "u1", CreatedAt: now},
		},
	}
	blob, err := json.Marshal(src)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "storage.json")
	if err := os.WriteFile(path, blob, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := ImportJSON(ctx, database.DB, path); err != nil {
		t.Fatal(err)
	}

	var articleID int64
	var slug, html string
	if err := database.QueryRowContext(ctx, `SELECT id, slug, rendered_html FROM articles WHERE title=?`, "Русская статья").Scan(&articleID, &slug, &html); err != nil {
		t.Fatal(err)
	}
	if slug != "русская-статья" {
		t.Fatalf("slug = %q, want русская-статья", slug)
	}
	if strings.Contains(html, "<script") {
		t.Fatalf("rendered HTML was not sanitized: %s", html)
	}
	if !strings.Contains(html, "/a/runbook") {
		t.Fatalf("rendered HTML does not contain converted wiki link: %s", html)
	}

	assertCount(t, database.DB, `SELECT count(*) FROM article_tags WHERE article_id=?`, articleID, 2)
	assertCount(t, database.DB, `SELECT count(*) FROM links WHERE from_article_id=? AND target_slug='runbook'`, articleID, 1)
	assertCount(t, database.DB, `SELECT count(*) FROM article_versions WHERE article_id=?`, articleID, 1)
	assertCount(t, database.DB, `SELECT count(*) FROM article_files WHERE article_id=?`, articleID, 1)
}

func assertCount(t *testing.T, q queryer, query string, arg any, want int) {
	t.Helper()
	var got int
	if err := q.QueryRowContext(context.Background(), query, arg).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("%s count = %d, want %d", query, got, want)
	}
}

type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}
