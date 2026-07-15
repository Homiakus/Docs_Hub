package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/homiakus/docshub-next/internal/db"
	"github.com/homiakus/docshub-next/internal/domain"
	"github.com/homiakus/docshub-next/internal/markdownx"
	"github.com/homiakus/docshub-next/internal/repository"
)

var (
	storageKeyRe = regexp.MustCompile(`^[a-f0-9]{64}(\.[a-z0-9]+)?$`)
	uploadRefRe  = regexp.MustCompile(`/uploads/([A-Za-z0-9%._~-]+)`)
)

type ArticleRepository struct {
	db  *db.DB
	log *slog.Logger
}

func NewArticleRepository(d *db.DB, logger *slog.Logger) *ArticleRepository {
	return &ArticleRepository{db: d, log: logger}
}

func (r *ArticleRepository) GetBySlug(ctx context.Context, slug string) (*domain.Article, error) {
	if decoded, err := url.PathUnescape(slug); err == nil {
		slug = decoded
	}
	var a domain.Article
	var html string
	err := r.db.QueryRowContext(ctx, `SELECT a.id,a.slug,a.title,a.content,a.rendered_html,a.visibility,a.updated_at,coalesce(a.category_id,0),coalesce(c.name,''),coalesce(a.owner_id,0) FROM articles a LEFT JOIN categories c ON c.id=a.category_id WHERE a.slug=? AND a.deleted_at IS NULL`, slug).Scan(&a.ID, &a.Slug, &a.Title, &a.Content, &html, &a.Visibility, &a.UpdatedAt, &a.CategoryID, &a.Category, &a.OwnerID)
	if err != nil {
		return nil, err
	}
	a.HTML = template.HTML(html)
	if a.Content != "" {
		if res, renderErr := markdownx.Render(a.Content); renderErr == nil {
			a.HasMermaid = res.Mermaid
			var headings []domain.Heading
			for _, h := range res.Headings {
				headings = append(headings, domain.Heading{Level: h.Level, Text: h.Text, ID: h.ID})
			}
			a.Headings = headings
			a.Tags = res.Tags
		}
	}
	return &a, nil
}

func (r *ArticleRepository) GetByID(ctx context.Context, id int64) (*domain.Article, error) {
	var a domain.Article
	var html string
	err := r.db.QueryRowContext(ctx, `SELECT a.id,a.slug,a.title,a.content,a.rendered_html,a.visibility,a.updated_at,coalesce(a.category_id,0),coalesce(c.name,''),coalesce(a.owner_id,0) FROM articles a LEFT JOIN categories c ON c.id=a.category_id WHERE a.id=? AND a.deleted_at IS NULL`, id).Scan(&a.ID, &a.Slug, &a.Title, &a.Content, &html, &a.Visibility, &a.UpdatedAt, &a.CategoryID, &a.Category, &a.OwnerID)
	if err != nil {
		return nil, err
	}
	a.HTML = template.HTML(html)
	return &a, nil
}

func (r *ArticleRepository) ListArticles(ctx context.Context, u *domain.User, query string) ([]domain.Article, error) {
	var rows *sql.Rows
	var err error
	if query != "" {
		ftsQuery := strings.TrimSpace(query)
		if !strings.ContainsAny(ftsQuery, " *\"") {
			ftsQuery += "*"
		}
		rows, err = r.db.QueryContext(ctx, `SELECT a.id, a.slug, a.title, a.content, a.rendered_html, a.visibility, a.updated_at, coalesce(a.category_id,0), coalesce(c.name,''), coalesce(a.owner_id,0) FROM article_fts f JOIN articles a ON a.id = f.rowid LEFT JOIN categories c ON c.id=a.category_id WHERE article_fts MATCH ? AND a.deleted_at IS NULL ORDER BY bm25(article_fts) LIMIT 100`, ftsQuery)
	} else {
		rows, err = r.db.QueryContext(ctx, `SELECT a.id, a.slug, a.title, a.content, a.rendered_html, a.visibility, a.updated_at, coalesce(a.category_id,0), coalesce(c.name,''), coalesce(a.owner_id,0) FROM articles a LEFT JOIN categories c ON c.id=a.category_id WHERE a.deleted_at IS NULL ORDER BY a.updated_at DESC LIMIT 100`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.Article
	for rows.Next() {
		var a domain.Article
		var html string
		if err := rows.Scan(&a.ID, &a.Slug, &a.Title, &a.Content, &html, &a.Visibility, &a.UpdatedAt, &a.CategoryID, &a.Category, &a.OwnerID); err != nil {
			continue
		}
		a.HTML = template.HTML(html)
		if r.CanRead(ctx, u, a.ID, a.Visibility) {
			out = append(out, a)
		}
	}
	return out, nil
}

func (r *ArticleRepository) ListAdminArticles(ctx context.Context) ([]domain.Article, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT a.id, a.slug, a.title, a.visibility, coalesce(a.category_id,0), coalesce(c.name,''), a.updated_at, coalesce(a.owner_id,0) FROM articles a LEFT JOIN categories c ON c.id=a.category_id WHERE a.deleted_at IS NULL ORDER BY a.updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Article
	for rows.Next() {
		var a domain.Article
		if err := rows.Scan(&a.ID, &a.Slug, &a.Title, &a.Visibility, &a.CategoryID, &a.Category, &a.UpdatedAt, &a.OwnerID); err == nil {
			out = append(out, a)
		}
	}
	return out, nil
}

func (r *ArticleRepository) ListRecentActivity(ctx context.Context, u *domain.User) ([]domain.ActivityItem, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT coalesce(u.display_name,u.username,'система'), e.entity_id, e.metadata_json, e.created_at FROM audit_events e LEFT JOIN users u ON u.id=e.actor_id WHERE e.entity_type='article' ORDER BY e.id DESC LIMIT 12`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ActivityItem
	for rows.Next() {
		var actor, entityID, metadataJSON, createdAt string
		if err := rows.Scan(&actor, &entityID, &metadataJSON, &createdAt); err != nil {
			continue
		}
		var meta struct {
			Title   string `json:"title"`
			Slug    string `json:"slug"`
			Summary string `json:"summary"`
		}
		_ = json.Unmarshal([]byte(metadataJSON), &meta)
		out = append(out, domain.ActivityItem{
			Actor:     actor,
			Title:     meta.Title,
			Slug:      meta.Slug,
			Summary:   meta.Summary,
			CreatedAt: createdAt,
		})
	}
	return out, nil
}

func (r *ArticleRepository) ListBacklinks(ctx context.Context, u *domain.User, slug string) ([]domain.Article, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT a.id, a.slug, a.title, a.visibility FROM links l JOIN articles a ON a.id=l.from_article_id WHERE l.target_slug=? AND a.deleted_at IS NULL ORDER BY a.title`, slug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Article
	for rows.Next() {
		var a domain.Article
		if err := rows.Scan(&a.ID, &a.Slug, &a.Title, &a.Visibility); err == nil {
			if r.CanRead(ctx, u, a.ID, a.Visibility) {
				out = append(out, a)
			}
		}
	}
	return out, nil
}

func (r *ArticleRepository) ListWikiLinks(ctx context.Context, u *domain.User, articleID int64, slug string) ([]domain.WikiLinkItem, error) {
	out := []domain.WikiLinkItem{}
	if articleID > 0 {
		rows, err := r.db.QueryContext(ctx, `SELECT l.target_slug, coalesce(a.title, l.target_slug) FROM links l LEFT JOIN articles a ON a.slug=l.target_slug AND a.deleted_at IS NULL WHERE l.from_article_id=? ORDER BY 2`, articleID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var targetSlug, label string
				if rows.Scan(&targetSlug, &label) == nil {
					out = append(out, domain.WikiLinkItem{Slug: targetSlug, Label: label, Direction: "out"})
				}
			}
		}
	}
	if slug != "" {
		rows, err := r.db.QueryContext(ctx, `SELECT a.slug, a.title, a.id, a.visibility FROM links l JOIN articles a ON a.id=l.from_article_id WHERE l.target_slug=? AND a.deleted_at IS NULL ORDER BY a.title`, slug)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var sourceSlug, title, vis string
				var sourceID int64
				if rows.Scan(&sourceSlug, &title, &sourceID, &vis) == nil && r.CanRead(ctx, u, sourceID, vis) {
					out = append(out, domain.WikiLinkItem{Slug: sourceSlug, Label: title, Direction: "back"})
				}
			}
		}
	}
	return out, nil
}

func (r *ArticleRepository) ListVersions(ctx context.Context, articleID int64) ([]domain.VersionEntry, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT v.version_no, v.title, coalesce(u.display_name,u.username,'система'), v.created_at FROM article_versions v LEFT JOIN users u ON u.id=v.author_id WHERE v.article_id=? ORDER BY v.version_no DESC`, articleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.VersionEntry
	for rows.Next() {
		var item domain.VersionEntry
		if err := rows.Scan(&item.VersionNo, &item.Title, &item.Author, &item.CreatedAt); err == nil {
			out = append(out, item)
		}
	}
	return out, nil
}

func (r *ArticleRepository) SaveArticle(ctx context.Context, input repository.ArticleSaveInput) (string, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	slug, err := r.uniqueSlugTx(ctx, tx, input.ID, input.Slug)
	if err != nil {
		return "", err
	}

	id := input.ID
	if id == 0 {
		var catID sql.NullInt64
		if input.CategoryID > 0 {
			catID = sql.NullInt64{Int64: input.CategoryID, Valid: true}
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO articles(slug,title,content,rendered_html,owner_id,visibility,created_at,updated_at,category_id) VALUES(?,?,?,?,?,?,?,?,?)`, slug, input.Title, input.Content, input.Rendered, input.AuthorID, input.Visibility, now, now, catID)
		if err != nil {
			return "", err
		}
		id, _ = res.LastInsertId()
	} else {
		var catID sql.NullInt64
		if input.CategoryID > 0 {
			catID = sql.NullInt64{Int64: input.CategoryID, Valid: true}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE articles SET slug=?, title=?, content=?, rendered_html=?, visibility=?, updated_at=?, category_id=? WHERE id=?`, slug, input.Title, input.Content, input.Rendered, input.Visibility, now, catID, id); err != nil {
			return "", err
		}
	}

	var versionNo int
	_ = tx.QueryRowContext(ctx, `SELECT coalesce(max(version_no),0)+1 FROM article_versions WHERE article_id=?`, id).Scan(&versionNo)
	_, _ = tx.ExecContext(ctx, `INSERT INTO article_versions(article_id, version_no, title, content, rendered_html, author_id, created_at) VALUES(?,?,?,?,?,?,?)`, id, versionNo, input.Title, input.Content, input.Rendered, input.AuthorID, now)

	_, _ = tx.ExecContext(ctx, `DELETE FROM article_tags WHERE article_id=?`, id)
	for _, tag := range input.Tags {
		_, _ = tx.ExecContext(ctx, `INSERT OR IGNORE INTO tags(name) VALUES(?)`, tag)
		_, _ = tx.ExecContext(ctx, `INSERT OR IGNORE INTO article_tags(article_id, tag_id) SELECT ?, id FROM tags WHERE name=?`, id, tag)
	}

	_, _ = tx.ExecContext(ctx, `DELETE FROM links WHERE from_article_id=?`, id)
	for _, l := range input.Links {
		_, _ = tx.ExecContext(ctx, `INSERT OR IGNORE INTO links(from_article_id, target_slug, label) VALUES(?,?,?)`, id, l.Slug, l.Label)
	}

	_, _ = tx.ExecContext(ctx, `DELETE FROM article_fts WHERE rowid=?`, id)
	_, _ = tx.ExecContext(ctx, `INSERT INTO article_fts(rowid,title,slug,content,tags) VALUES(?,?,?,?,?)`, id, input.Title, slug, input.Content, strings.Join(input.Tags, " "))

	metadata, _ := json.Marshal(map[string]any{
		"version": versionNo,
		"slug":    slug,
		"title":   input.Title,
	})
	_, _ = tx.ExecContext(ctx, `INSERT INTO audit_events(actor_id, action, entity_type, entity_id, ip, metadata_json, created_at) VALUES(?,?,?,?,?,?,?)`, input.AuthorID, "article.save", "article", fmt.Sprint(id), input.ClientIP, string(metadata), now)

	if err := tx.Commit(); err != nil {
		return "", err
	}
	return slug, nil
}

func (r *ArticleRepository) UpdateSettings(ctx context.Context, id int64, visibility string, categoryID int64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	var catID sql.NullInt64
	if categoryID > 0 {
		catID = sql.NullInt64{Int64: categoryID, Valid: true}
	}
	_, err := r.db.ExecContext(ctx, `UPDATE articles SET visibility=?, category_id=?, updated_at=? WHERE id=?`, visibility, catID, now, id)
	return err
}

func (r *ArticleRepository) SoftDelete(ctx context.Context, id int64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := r.db.ExecContext(ctx, `UPDATE articles SET deleted_at=?, updated_at=? WHERE id=?`, now, now, id); err != nil {
		return err
	}
	_, _ = r.db.ExecContext(ctx, `DELETE FROM article_fts WHERE rowid=?`, id)
	return nil
}

func (r *ArticleRepository) CountArticles(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM articles WHERE deleted_at IS NULL`).Scan(&count)
	return count, err
}

func (r *ArticleRepository) GraphData(ctx context.Context, u *domain.User) ([]map[string]string, []map[string]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,slug,title,visibility FROM articles WHERE deleted_at IS NULL ORDER BY title`)
	if err != nil {
		return nil, nil, err
	}
	type candidate struct {
		id         int64
		slug, title, vis string
	}
	var candidates []candidate
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.id, &item.slug, &item.title, &item.vis); err == nil {
			candidates = append(candidates, item)
		}
	}
	rows.Close()

	var nodes []map[string]string
	accessibleSlugs := make(map[string]bool)
	for _, item := range candidates {
		if r.CanRead(ctx, u, item.id, item.vis) {
			nodes = append(nodes, map[string]string{"id": item.slug, "label": item.title})
			accessibleSlugs[item.slug] = true
		}
	}
	lr, err := r.db.QueryContext(ctx, `SELECT a.slug, l.target_slug FROM links l JOIN articles a ON a.id=l.from_article_id WHERE a.deleted_at IS NULL`)
	if err != nil {
		return nodes, []map[string]string{}, nil
	}
	defer lr.Close()
	var links []map[string]string
	for lr.Next() {
		var a, b string
		if err := lr.Scan(&a, &b); err == nil {
			if accessibleSlugs[a] && accessibleSlugs[b] {
				links = append(links, map[string]string{"source": a, "target": b})
			}
		}
	}
	return nodes, links, nil
}

func (r *ArticleRepository) CanRead(ctx context.Context, u *domain.User, articleID int64, visibility string) bool {
	if visibility == "public" {
		return true
	}
	if u == nil {
		return false
	}
	if u.Role == "admin" || visibility == "authenticated" {
		return true
	}
	var n int
	if err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM acl_users WHERE article_id=? AND user_id=? AND permission IN ('read','write','admin')`, articleID, u.ID).Scan(&n); err == nil && n > 0 {
		return true
	}
	if err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM acl_groups ag JOIN group_members gm ON gm.group_id=ag.group_id WHERE ag.article_id=? AND gm.user_id=? AND ag.permission IN ('read','write','admin')`, articleID, u.ID).Scan(&n); err == nil && n > 0 {
		return true
	}
	return false
}

func (r *ArticleRepository) CanEdit(ctx context.Context, u *domain.User, articleID int64, ownerID int64, visibility string) bool {
	if u == nil {
		return false
	}
	if u.Role == "admin" {
		return true
	}
	if u.Role != "editor" {
		return false
	}
	if articleID == 0 {
		return true
	}
	if ownerID != 0 && u.ID == ownerID {
		return true
	}
	var n int
	if err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM acl_users WHERE article_id=? AND user_id=? AND permission IN ('write','admin')`, articleID, u.ID).Scan(&n); err == nil && n > 0 {
		return true
	}
	if err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM acl_groups ag JOIN group_members gm ON gm.group_id=ag.group_id WHERE ag.article_id=? AND gm.user_id=? AND ag.permission IN ('write','admin')`, articleID, u.ID).Scan(&n); err == nil && n > 0 {
		return true
	}
	return visibility == "public" || visibility == "authenticated"
}

func (r *ArticleRepository) ListAdminAccess(ctx context.Context) ([]domain.AdminAccessRow, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT a.id, a.title, a.slug, u.id, u.username, acl.permission FROM acl_users acl JOIN articles a ON a.id=acl.article_id JOIN users u ON u.id=acl.user_id WHERE a.deleted_at IS NULL ORDER BY a.title, u.username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.AdminAccessRow
	for rows.Next() {
		var item domain.AdminAccessRow
		if err := rows.Scan(&item.ArticleID, &item.ArticleTitle, &item.ArticleSlug, &item.UserID, &item.Username, &item.Permission); err == nil {
			out = append(out, item)
		}
	}
	return out, nil
}

func (r *ArticleRepository) SaveAccess(ctx context.Context, articleID, userID int64, permission string) error {
	if permission == "none" {
		_, err := r.db.ExecContext(ctx, `DELETE FROM acl_users WHERE article_id=? AND user_id=?`, articleID, userID)
		return err
	}
	_, err := r.db.ExecContext(ctx, `INSERT OR REPLACE INTO acl_users(article_id, user_id, permission) VALUES(?,?,?)`, articleID, userID, permission)
	return err
}

func (r *ArticleRepository) uniqueSlugTx(ctx context.Context, tx *sql.Tx, articleID int64, base string) (string, error) {
	base = markdownx.Slugify(base)
	if base == "" {
		base = "article"
	}
	for i := 0; i < 1000; i++ {
		candidate := base
		if i > 0 {
			candidate = fmt.Sprintf("%s-%d", base, i+1)
		}
		var existingID int64
		err := tx.QueryRowContext(ctx, `SELECT id FROM articles WHERE slug=? LIMIT 1`, candidate).Scan(&existingID)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && existingID == articleID) {
			return candidate, nil
		}
		if err != nil {
			return "", err
		}
	}
	return base, nil
}
