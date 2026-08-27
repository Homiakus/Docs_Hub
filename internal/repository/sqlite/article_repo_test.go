package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/homiakus/docshub-next/internal/db"
	"github.com/homiakus/docshub-next/internal/domain"
)

func setupArticleTestDB(t *testing.T) *db.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test_articles.db")
	database, err := db.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	ctx := context.Background()
	// Insert users: admin (1), reader (2), other_reader (3)
	_, err = database.ExecContext(ctx, `
		INSERT OR IGNORE INTO users(id, username, display_name, email, password_hash, role, is_active, created_at, updated_at)
		VALUES
		(1, 'admin', 'Admin User', 'admin@example.com', 'hash', 'admin', 1, datetime('now'), datetime('now')),
		(2, 'alice', 'Alice Reader', 'alice@example.com', 'hash', 'reader', 1, datetime('now'), datetime('now')),
		(3, 'bob', 'Bob Reader', 'bob@example.com', 'hash', 'reader', 1, datetime('now'), datetime('now'));
	`)
	if err != nil {
		t.Fatalf("seed users: %v", err)
	}

	// Insert articles with different visibilities and owners
	_, err = database.ExecContext(ctx, `
		INSERT INTO articles(id, organization_id, space_id, stable_key, slug, title, content, rendered_html, visibility, owner_id, status, created_at, updated_at)
		VALUES
		(1, 1, 1, 'art-1', 'public-article', 'Public Guide', 'Public content', '<p>Public content</p>', 'public', 1, 'published', datetime('now'), datetime('now')),
		(2, 1, 1, 'art-2', 'internal-article', 'Internal Docs', 'Internal only', '<p>Internal only</p>', 'authenticated', 1, 'published', datetime('now'), datetime('now')),
		(3, 1, 1, 'art-3', 'alice-private', 'Alice Private Note', 'Alice secret content', '<p>Alice secret</p>', 'private', 2, 'draft', datetime('now'), datetime('now')),
		(4, 1, 1, 'art-4', 'bob-private', 'Bob Private Note', 'Bob secret content', '<p>Bob secret</p>', 'private', 3, 'draft', datetime('now'), datetime('now'));
	`)
	if err != nil {
		t.Fatalf("seed articles: %v", err)
	}

	// Index in FTS
	_, err = database.ExecContext(ctx, `
		INSERT INTO article_fts(rowid, title, content, tags)
		VALUES
		(1, 'Public Guide', 'Public content', ''),
		(2, 'Internal Docs', 'Internal only', ''),
		(3, 'Alice Private Note', 'Alice secret content', ''),
		(4, 'Bob Private Note', 'Bob secret content', '');
	`)
	if err != nil {
		t.Fatalf("seed fts: %v", err)
	}

	return database
}

func TestArticleRepositorySQLAuthorization(t *testing.T) {
	ctx := context.Background()
	database := setupArticleTestDB(t)
	repo := NewArticleRepository(database, nil)

	// 1. Anonymous user (nil domain.User)
	anonArticles, err := repo.ListArticles(ctx, nil, "")
	if err != nil {
		t.Fatalf("anon list articles: %v", err)
	}
	if len(anonArticles) != 1 || anonArticles[0].Slug != "public-article" {
		t.Fatalf("anonymous user should only see public-article, got %d items", len(anonArticles))
	}

	// Anonymous search query
	anonSearch, err := repo.ListArticles(ctx, nil, "secret")
	if err != nil {
		t.Fatalf("anon search: %v", err)
	}
	if len(anonSearch) != 0 {
		t.Fatalf("anonymous search leaked private articles! got %d items", len(anonSearch))
	}

	// 2. Authenticated reader (Alice - user ID 2)
	alice := &domain.User{ID: 2, Username: "alice", Role: "reader", Active: true}
	aliceArticles, err := repo.ListArticles(ctx, alice, "")
	if err != nil {
		t.Fatalf("alice list articles: %v", err)
	}
	// Alice should see: public-article, internal-article, alice-private (3 total, Bob's private is excluded)
	if len(aliceArticles) != 3 {
		t.Fatalf("alice should see 3 articles, got %d", len(aliceArticles))
	}
	for _, a := range aliceArticles {
		if a.Slug == "bob-private" {
			t.Fatalf("alice saw bob-private article! ACL leakage detected")
		}
	}

	// Alice search for 'secret' -> should only return alice-private
	aliceSearch, err := repo.ListArticles(ctx, alice, "secret")
	if err != nil {
		t.Fatalf("alice search: %v", err)
	}
	if len(aliceSearch) != 1 || aliceSearch[0].Slug != "alice-private" {
		t.Fatalf("alice search for secret should only match alice-private, got %v", aliceSearch)
	}

	// 3. Admin user (user ID 1)
	admin := &domain.User{ID: 1, Username: "admin", Role: "admin", Active: true}
	adminArticles, err := repo.ListArticles(ctx, admin, "")
	if err != nil {
		t.Fatalf("admin list articles: %v", err)
	}
	if len(adminArticles) != 4 {
		t.Fatalf("admin should see all 4 articles, got %d", len(adminArticles))
	}
}
