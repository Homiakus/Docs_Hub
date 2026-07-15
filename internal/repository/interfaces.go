package repository

import (
	"context"

	"github.com/homiakus/docshub-next/internal/domain"
)

type UserRepository interface {
	GetByUsername(ctx context.Context, username string) (*domain.User, string, error)
	GetByID(ctx context.Context, id int64) (*domain.User, error)
	ListAdminUsers(ctx context.Context) ([]domain.AdminUserRow, error)
	CreateUser(ctx context.Context, username, displayName, email, passwordHash, role string, active bool) error
	UpdateUser(ctx context.Context, id int64, username, displayName, email, role string, active bool) error
	SetPassword(ctx context.Context, id int64, passwordHash string) error
	CountUsers(ctx context.Context) (int, error)
	DeleteUserSessions(ctx context.Context, userID int64) error
}

type ArticleSaveInput struct {
	ID         int64
	Slug       string
	Title      string
	Content    string
	Rendered   string
	Visibility string
	CategoryID int64
	AuthorID   int64
	ClientIP   string
	Tags       []string
	Links      []struct{ Slug, Label string }
}

type ArticleRepository interface {
	GetBySlug(ctx context.Context, slug string) (*domain.Article, error)
	GetByID(ctx context.Context, id int64) (*domain.Article, error)
	ListArticles(ctx context.Context, u *domain.User, query string) ([]domain.Article, error)
	ListAdminArticles(ctx context.Context) ([]domain.Article, error)
	ListRecentActivity(ctx context.Context, u *domain.User) ([]domain.ActivityItem, error)
	ListBacklinks(ctx context.Context, u *domain.User, slug string) ([]domain.Article, error)
	ListWikiLinks(ctx context.Context, u *domain.User, articleID int64, slug string) ([]domain.WikiLinkItem, error)
	ListVersions(ctx context.Context, articleID int64) ([]domain.VersionEntry, error)
	SaveArticle(ctx context.Context, input ArticleSaveInput) (string, error)
	UpdateSettings(ctx context.Context, id int64, visibility string, categoryID int64) error
	SoftDelete(ctx context.Context, id int64) error
	CountArticles(ctx context.Context) (int, error)
	GraphData(ctx context.Context, u *domain.User) ([]map[string]string, []map[string]string, error)
	CanRead(ctx context.Context, u *domain.User, articleID int64, visibility string) bool
	CanEdit(ctx context.Context, u *domain.User, articleID int64, ownerID int64, visibility string) bool
	ListAdminAccess(ctx context.Context) ([]domain.AdminAccessRow, error)
	SaveAccess(ctx context.Context, articleID, userID int64, permission string) error
}

type CategoryRepository interface {
	ListCategories(ctx context.Context, u *domain.User) ([]domain.Category, error)
	ListAdminCategories(ctx context.Context) ([]domain.Category, error)
	GetMeta(ctx context.Context, categoryID int64) (string, string, error)
	SaveCategory(ctx context.Context, id int64, name, slug, description string, navOrder int, visible bool) error
	CountCategories(ctx context.Context) (int, error)
}

type FileRepository interface {
	SaveFile(ctx context.Context, sha, storageKey, filename, mimeType string, size int64, u *domain.User) (*domain.FileObject, error)
	GetByKey(ctx context.Context, key string) (*domain.FileObject, error)
	UserCanReadFile(ctx context.Context, u *domain.User, fileID int64) bool
	CountFiles(ctx context.Context) (int, error)
}
