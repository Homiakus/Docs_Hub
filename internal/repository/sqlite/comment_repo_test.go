package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/homiakus/docshub-next/internal/db"
	"github.com/homiakus/docshub-next/internal/domain"
)

func setupCommentTestDB(t *testing.T) *db.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test_comments.db")
	database, err := db.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	ctx := context.Background()
	_, err = database.ExecContext(ctx, `
		INSERT OR IGNORE INTO users(id, username, display_name, email, password_hash, role, is_active, created_at, updated_at)
		VALUES
		(1, 'alice', 'Alice Author', 'alice@example.com', 'hash', 'editor', 1, datetime('now'), datetime('now')),
		(2, 'bob', 'Bob Reviewer', 'bob@example.com', 'hash', 'reader', 1, datetime('now'), datetime('now'));

		INSERT OR IGNORE INTO articles(id, organization_id, space_id, stable_key, slug, title, content, rendered_html, visibility, owner_id, status, created_at, updated_at)
		VALUES
		(100, 1, 1, 'art-100', 'design-spec', 'Design Spec', 'Content', '<p>Content</p>', 'authenticated', 1, 'published', datetime('now'), datetime('now'));
	`)
	if err != nil {
		t.Fatalf("seed db: %v", err)
	}

	return database
}

func TestCommentRepositoryLifecycle(t *testing.T) {
	ctx := context.Background()
	database := setupCommentTestDB(t)
	repo := NewCommentRepository(database)

	// 1. Create root comment
	root := &domain.Comment{
		DocumentID:     100,
		AuthorID:       2,
		BaseRevisionID: 1,
		StartOffset:    10,
		EndOffset:      25,
		QuoteExact:     "architecture",
		QuotePrefix:    "The key ",
		QuoteSuffix:    " principles",
		ASTNodeKind:    "paragraph",
		Body:           "Should we mention microservices here?",
	}
	if err := repo.CreateComment(ctx, root); err != nil {
		t.Fatalf("create root comment: %v", err)
	}
	if root.ID == 0 {
		t.Fatalf("expected non-zero comment ID")
	}

	// 2. Create reply comment
	reply := &domain.Comment{
		DocumentID:     100,
		AuthorID:       1,
		ParentID:       &root.ID,
		BaseRevisionID: 1,
		Body:           "No, we stick to modular monolith.",
	}
	if err := repo.CreateComment(ctx, reply); err != nil {
		t.Fatalf("create reply comment: %v", err)
	}

	// 3. Query comments by document
	comments, err := repo.GetCommentsByDocument(ctx, 100)
	if err != nil {
		t.Fatalf("get comments: %v", err)
	}
	if len(comments) != 1 {
		t.Fatalf("expected 1 root comment, got %d", len(comments))
	}
	if len(comments[0].Replies) != 1 {
		t.Fatalf("expected 1 reply under root comment, got %d", len(comments[0].Replies))
	}
	if comments[0].QuoteExact != "architecture" {
		t.Fatalf("quote mismatch: %s", comments[0].QuoteExact)
	}

	// 4. Resolve thread
	if err := repo.ResolveComment(ctx, root.ID); err != nil {
		t.Fatalf("resolve comment: %v", err)
	}
	resolved, err := repo.GetCommentsByDocument(ctx, 100)
	if err != nil {
		t.Fatalf("get comments after resolve: %v", err)
	}
	if resolved[0].Status != domain.CommentStatusResolved {
		t.Fatalf("expected resolved status, got %s", resolved[0].Status)
	}
}
