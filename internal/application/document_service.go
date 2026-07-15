package application

import (
	"context"

	"github.com/homiakus/docshub-next/internal/domain"
	"github.com/homiakus/docshub-next/internal/markdownx"
	"github.com/homiakus/docshub-next/internal/repository"
)

type DocumentService struct {
	articles   repository.ArticleRepository
	categories repository.CategoryRepository
}

func NewDocumentService(articles repository.ArticleRepository, categories repository.CategoryRepository) *DocumentService {
	return &DocumentService{articles: articles, categories: categories}
}

func (s *DocumentService) GetArticle(ctx context.Context, slug string) (*domain.Article, error) {
	return s.articles.GetBySlug(ctx, slug)
}

func (s *DocumentService) ListArticles(ctx context.Context, u *domain.User, query string) ([]domain.Article, error) {
	return s.articles.ListArticles(ctx, u, query)
}

func (s *DocumentService) CanRead(ctx context.Context, u *domain.User, articleID int64, visibility string) bool {
	return s.articles.CanRead(ctx, u, articleID, visibility)
}

func (s *DocumentService) CanEdit(ctx context.Context, u *domain.User, articleID int64, ownerID int64, visibility string) bool {
	return s.articles.CanEdit(ctx, u, articleID, ownerID, visibility)
}

func (s *DocumentService) SaveArticle(ctx context.Context, u *domain.User, id int64, title, rawSlug, content, visibility string, categoryID int64, clientIP string) (string, error) {
	slug := markdownx.Slugify(rawSlug)
	if slug == "" {
		slug = markdownx.Slugify(title)
	}
	if slug == "" {
		slug = "article"
	}
	if title == "" {
		title = "Без названия"
	}
	if visibility == "" {
		visibility = "authenticated"
	}

	categoryName, categorySlug, err := s.categories.GetMeta(ctx, categoryID)
	if err != nil {
		return "", err
	}

	res, err := markdownx.Render(content)
	if err != nil {
		return "", err
	}

	var tags []string
	seen := map[string]struct{}{}
	addTag := func(t string) {
		t = stringsTrimLower(t)
		if t == "" {
			return
		}
		if _, ok := seen[t]; !ok {
			seen[t] = struct{}{}
			tags = append(tags, t)
		}
	}
	for _, tag := range res.Tags {
		addTag(tag)
	}
	if categoryName != "" {
		addTag(categoryName)
	}
	if categorySlug != "" {
		addTag(categorySlug)
	}

	var links []struct{ Slug, Label string }
	for _, l := range res.Links {
		links = append(links, struct{ Slug, Label string }{Slug: l.Slug, Label: l.Label})
	}

	input := repository.ArticleSaveInput{
		ID:         id,
		Slug:       slug,
		Title:      title,
		Content:    content,
		Rendered:   res.HTML,
		Visibility: visibility,
		CategoryID: categoryID,
		AuthorID:   u.ID,
		ClientIP:   clientIP,
		Tags:       tags,
		Links:      links,
	}

	return s.articles.SaveArticle(ctx, input)
}

func (s *DocumentService) GraphData(ctx context.Context, u *domain.User) ([]map[string]string, []map[string]string, error) {
	return s.articles.GraphData(ctx, u)
}

func stringsTrimLower(s string) string {
	return markdownx.Slugify(s)
}
