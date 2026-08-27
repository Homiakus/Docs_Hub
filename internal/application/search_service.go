package application

import (
	"context"
	"strings"

	"github.com/homiakus/docshub-next/internal/domain"
)

type SearchFilter struct {
	SpaceID int64
	Status  string
	Limit   int
}

type SearchSuggestion struct {
	Title string `json:"title"`
	Slug  string `json:"slug"`
}

type SearchService interface {
	Search(ctx context.Context, u *domain.User, query string, filter SearchFilter) ([]domain.Article, error)
	Suggest(ctx context.Context, u *domain.User, query string, limit int) ([]SearchSuggestion, error)
}

type ArticleSearchReader interface {
	ListArticles(ctx context.Context, u *domain.User, query string) ([]domain.Article, error)
}

type DefaultSearchService struct {
	reader ArticleSearchReader
}

func NewSearchService(reader ArticleSearchReader) *DefaultSearchService {
	return &DefaultSearchService{reader: reader}
}

func (s *DefaultSearchService) Search(ctx context.Context, u *domain.User, query string, filter SearchFilter) ([]domain.Article, error) {
	if s.reader == nil {
		return nil, ErrServiceMisconfigured
	}
	query = strings.TrimSpace(query)
	articles, err := s.reader.ListArticles(ctx, u, query)
	if err != nil {
		return nil, err
	}

	if filter.SpaceID <= 0 && filter.Status == "" && (filter.Limit <= 0 || len(articles) <= filter.Limit) {
		return articles, nil
	}

	var filtered []domain.Article
	for _, a := range articles {
		if filter.SpaceID > 0 && a.SpaceID != filter.SpaceID {
			continue
		}
		if filter.Status != "" && string(a.Status) != filter.Status {
			continue
		}
		filtered = append(filtered, a)
		if filter.Limit > 0 && len(filtered) >= filter.Limit {
			break
		}
	}
	return filtered, nil
}

func (s *DefaultSearchService) Suggest(ctx context.Context, u *domain.User, query string, limit int) ([]SearchSuggestion, error) {
	if s.reader == nil {
		return nil, ErrServiceMisconfigured
	}
	if limit <= 0 {
		limit = 8
	}
	query = strings.TrimSpace(query)
	articles, err := s.reader.ListArticles(ctx, u, query)
	if err != nil {
		return nil, err
	}

	out := make([]SearchSuggestion, 0, min(limit, len(articles)))
	for _, a := range articles {
		out = append(out, SearchSuggestion{
			Title: a.Title,
			Slug:  a.Slug,
		})
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

var _ SearchService = (*DefaultSearchService)(nil)
