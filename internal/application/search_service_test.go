package application

import (
	"context"
	"testing"

	"github.com/homiakus/docshub-next/internal/domain"
)

type mockArticleSearchReader struct {
	articles []domain.Article
}

func (m *mockArticleSearchReader) ListArticles(ctx context.Context, u *domain.User, query string) ([]domain.Article, error) {
	return m.articles, nil
}

func TestSearchServiceSearchAndSuggest(t *testing.T) {
	ctx := context.Background()
	reader := &mockArticleSearchReader{
		articles: []domain.Article{
			{ID: 1, Slug: "intro-guide", Title: "Intro Guide", SpaceID: 1, Status: domain.StatusPublished},
			{ID: 2, Slug: "backend-arch", Title: "Backend Architecture", SpaceID: 2, Status: domain.StatusPublished},
			{ID: 3, Slug: "draft-notes", Title: "Draft Notes", SpaceID: 1, Status: domain.StatusDraft},
		},
	}
	svc := NewSearchService(reader)
	u := &domain.User{ID: 1, Username: "alice", Role: "editor"}

	// 1. Unfiltered search
	results, err := svc.Search(ctx, u, "guide", SearchFilter{})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 articles, got %d", len(results))
	}

	// 2. Filtered search by space ID
	engResults, err := svc.Search(ctx, u, "", SearchFilter{SpaceID: 2})
	if err != nil {
		t.Fatalf("search engineering: %v", err)
	}
	if len(engResults) != 1 || engResults[0].Slug != "backend-arch" {
		t.Fatalf("expected backend-arch, got %v", engResults)
	}

	// 3. Filtered search by status and limit
	pubResults, err := svc.Search(ctx, u, "", SearchFilter{Status: "published", Limit: 1})
	if err != nil {
		t.Fatalf("search published: %v", err)
	}
	if len(pubResults) != 1 {
		t.Fatalf("expected 1 result due to limit, got %d", len(pubResults))
	}

	// 4. Suggest with limit
	suggestions, err := svc.Suggest(ctx, u, "guide", 2)
	if err != nil {
		t.Fatalf("suggest: %v", err)
	}
	if len(suggestions) != 2 {
		t.Fatalf("expected 2 suggestions, got %d", len(suggestions))
	}
	if suggestions[0].Title != "Intro Guide" || suggestions[0].Slug != "intro-guide" {
		t.Fatalf("unexpected suggestion 0: %v", suggestions[0])
	}
}
