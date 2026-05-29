package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/homiakus/docshub-next/internal/markdownx"
)

type JSONDatabase struct {
	Version          int                     `json:"version"`
	Users            map[string]*JSONUser    `json:"users"`
	Groups           map[string]*JSONGroup   `json:"groups"`
	Articles         map[string]*JSONArticle `json:"articles"`
	Attachments      map[string]*JSONFile    `json:"attachments"`
	RibbonArticleIDs []string                `json:"ribbon_article_ids"`
}

type JSONUser struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	SaltHex      string    `json:"salt_hex"`
	PasswordHash string    `json:"password_hash"`
	Role         string    `json:"role"`
	Active       bool      `json:"active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type JSONGroup struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	MemberIDs []string  `json:"member_ids"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type JSONVersion struct {
	At      time.Time `json:"at"`
	ActorID string    `json:"actor_id"`
	Title   string    `json:"title"`
	Slug    string    `json:"slug"`
	Content string    `json:"content"`
}

type JSONArticle struct {
	ID              string        `json:"id"`
	Title           string        `json:"title"`
	Slug            string        `json:"slug"`
	Content         string        `json:"content"`
	Tags            []string      `json:"tags"`
	AllUsers        bool          `json:"all_users"`
	AllowedUserIDs  []string      `json:"allowed_user_ids"`
	AllowedGroupIDs []string      `json:"allowed_group_ids"`
	OwnerID         string        `json:"owner_id"`
	Archived        bool          `json:"archived"`
	Versions        []JSONVersion `json:"versions"`
	CreatedAt       time.Time     `json:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at"`
}

type JSONFile struct {
	ID           string    `json:"id"`
	ArticleID    string    `json:"article_id"`
	StoredName   string    `json:"stored_name"`
	OriginalName string    `json:"original_name"`
	MIME         string    `json:"mime"`
	Size         int64     `json:"size"`
	UploadedBy   string    `json:"uploaded_by"`
	CreatedAt    time.Time `json:"created_at"`
}

func ImportJSON(ctx context.Context, database *sql.DB, jsonPath string) error {
	blob, err := os.ReadFile(jsonPath)
	if err != nil {
		return fmt.Errorf("read json: %w", err)
	}

	var src JSONDatabase
	if err := json.Unmarshal(blob, &src); err != nil {
		return fmt.Errorf("decode json: %w", err)
	}

	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	userIDs, err := importUsers(ctx, tx, src.Users)
	if err != nil {
		return err
	}
	groupIDs, err := importGroups(ctx, tx, src.Groups, userIDs)
	if err != nil {
		return err
	}
	articleIDs, err := importArticles(ctx, tx, src.Articles, userIDs, groupIDs)
	if err != nil {
		return err
	}
	if err := importFiles(ctx, tx, src.Attachments, articleIDs, userIDs); err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = tx.ExecContext(ctx, `INSERT INTO audit_events(action, entity_type, entity_id, created_at) VALUES(?,?,?,?)`, "json.import", "database", jsonPath, now)

	return tx.Commit()
}

func importUsers(ctx context.Context, tx *sql.Tx, users map[string]*JSONUser) (map[string]int64, error) {
	out := map[string]int64{}
	for oldID, u := range users {
		if u == nil || strings.TrimSpace(u.Username) == "" {
			continue
		}
		created, updated := ts(u.CreatedAt), ts(u.UpdatedAt)
		active := 0
		if u.Active {
			active = 1
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO users(username, display_name, password_hash, role, is_active, created_at, updated_at)
			VALUES(?,?,?,?,?,?,?)
			ON CONFLICT(username) DO UPDATE SET display_name=excluded.display_name, password_hash=excluded.password_hash, role=excluded.role, is_active=excluded.is_active, updated_at=excluded.updated_at`,
			u.Username, u.Username, u.PasswordHash, normalizeRole(u.Role), active, created, updated)
		if err != nil {
			return nil, fmt.Errorf("import user %s: %w", oldID, err)
		}
		id, err := lookupInt64(ctx, tx, `SELECT id FROM users WHERE username=?`, u.Username)
		if err != nil {
			return nil, err
		}
		out[oldID] = id
	}
	return out, nil
}

func importGroups(ctx context.Context, tx *sql.Tx, groups map[string]*JSONGroup, userIDs map[string]int64) (map[string]int64, error) {
	out := map[string]int64{}
	for oldID, g := range groups {
		if g == nil || strings.TrimSpace(g.Name) == "" {
			continue
		}
		_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO groups(name, created_at) VALUES(?,?)`, g.Name, ts(g.CreatedAt))
		if err != nil {
			return nil, fmt.Errorf("import group %s: %w", oldID, err)
		}
		groupID, err := lookupInt64(ctx, tx, `SELECT id FROM groups WHERE name=?`, g.Name)
		if err != nil {
			return nil, err
		}
		out[oldID] = groupID
		for _, oldUserID := range g.MemberIDs {
			userID, ok := userIDs[oldUserID]
			if !ok {
				continue
			}
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO group_members(group_id, user_id) VALUES(?,?)`, groupID, userID); err != nil {
				return nil, fmt.Errorf("import group member %s/%s: %w", oldID, oldUserID, err)
			}
		}
	}
	return out, nil
}

func importArticles(ctx context.Context, tx *sql.Tx, articles map[string]*JSONArticle, userIDs map[string]int64, groupIDs map[string]int64) (map[string]int64, error) {
	out := map[string]int64{}
	usedSlugs := map[string]struct{}{}
	for oldID, a := range articles {
		if a == nil {
			continue
		}
		title := strings.TrimSpace(a.Title)
		if title == "" {
			title = "Без названия"
		}
		slug, err := uniqueSlug(ctx, tx, usedSlugs, firstNonEmpty(a.Slug, title, oldID))
		if err != nil {
			return nil, err
		}
		rendered, err := markdownx.Render(a.Content)
		if err != nil {
			return nil, fmt.Errorf("render article %s: %w", oldID, err)
		}
		visibility := "private"
		if a.AllUsers {
			visibility = "authenticated"
		}
		var deletedAt any
		if a.Archived {
			deletedAt = ts(a.UpdatedAt)
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO articles(slug, title, content, rendered_html, owner_id, visibility, created_at, updated_at, deleted_at)
			VALUES(?,?,?,?,?,?,?,?,?)`,
			slug, title, a.Content, rendered.HTML, nullMappedInt(userIDs, a.OwnerID), visibility, ts(a.CreatedAt), ts(a.UpdatedAt), deletedAt)
		if err != nil {
			return nil, fmt.Errorf("import article %s: %w", oldID, err)
		}
		articleID, err := lookupInt64(ctx, tx, `SELECT id FROM articles WHERE slug=?`, slug)
		if err != nil {
			return nil, err
		}
		out[oldID] = articleID

		tags := mergeStrings(a.Tags, rendered.Tags)
		if err := replaceArticleRelations(ctx, tx, articleID, tags, rendered.Links); err != nil {
			return nil, err
		}
		for _, oldUserID := range a.AllowedUserIDs {
			if userID, ok := userIDs[oldUserID]; ok {
				_, _ = tx.ExecContext(ctx, `INSERT OR IGNORE INTO acl_users(article_id, user_id, permission) VALUES(?,?, 'read')`, articleID, userID)
			}
		}
		for _, oldGroupID := range a.AllowedGroupIDs {
			if groupID, ok := groupIDs[oldGroupID]; ok {
				_, _ = tx.ExecContext(ctx, `INSERT OR IGNORE INTO acl_groups(article_id, group_id, permission) VALUES(?,?, 'read')`, articleID, groupID)
			}
		}
		if err := importVersions(ctx, tx, articleID, a, userIDs, rendered.HTML); err != nil {
			return nil, err
		}
		_, _ = tx.ExecContext(ctx, `DELETE FROM article_fts WHERE rowid=?`, articleID)
		_, _ = tx.ExecContext(ctx, `INSERT INTO article_fts(rowid,title,slug,content,tags) VALUES(?,?,?,?,?)`, articleID, title, slug, a.Content, strings.Join(tags, " "))
	}
	return out, nil
}

func importVersions(ctx context.Context, tx *sql.Tx, articleID int64, a *JSONArticle, userIDs map[string]int64, currentHTML string) error {
	versions := a.Versions
	if len(versions) == 0 {
		versions = []JSONVersion{{At: a.UpdatedAt, ActorID: a.OwnerID, Title: a.Title, Slug: a.Slug, Content: a.Content}}
	}
	for i, v := range versions {
		title := strings.TrimSpace(v.Title)
		if title == "" {
			title = strings.TrimSpace(a.Title)
		}
		if title == "" {
			title = "Без названия"
		}
		html := currentHTML
		if v.Content != a.Content {
			rendered, err := markdownx.Render(v.Content)
			if err != nil {
				return fmt.Errorf("render article version %d: %w", i+1, err)
			}
			html = rendered.HTML
		}
		_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO article_versions(article_id, version_no, title, content, rendered_html, author_id, created_at)
			VALUES(?,?,?,?,?,?,?)`, articleID, i+1, title, v.Content, html, nullMappedInt(userIDs, v.ActorID), ts(v.At))
		if err != nil {
			return fmt.Errorf("import article version %d: %w", i+1, err)
		}
	}
	return nil
}

func replaceArticleRelations(ctx context.Context, tx *sql.Tx, articleID int64, tags []string, links []markdownx.WikiLink) error {
	for _, tag := range tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO tags(name) VALUES(?)`, tag); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO article_tags(article_id, tag_id) SELECT ?, id FROM tags WHERE name=?`, articleID, tag); err != nil {
			return err
		}
	}
	for _, link := range links {
		if link.Slug == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO links(from_article_id, target_slug, label) VALUES(?,?,?)`, articleID, link.Slug, link.Label); err != nil {
			return err
		}
	}
	return nil
}

func importFiles(ctx context.Context, tx *sql.Tx, files map[string]*JSONFile, articleIDs map[string]int64, userIDs map[string]int64) error {
	for oldID, f := range files {
		if f == nil {
			continue
		}
		articleID, ok := articleIDs[f.ArticleID]
		if !ok {
			continue
		}
		storageKey := firstNonEmpty(f.StoredName, oldID)
		if storageKey == "" {
			continue
		}
		_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO files(sha256, storage_key, original_name, mime, size_bytes, uploaded_by, created_at)
			VALUES(?,?,?,?,?,?,?)`,
			legacyFileHash(f), storageKey, firstNonEmpty(f.OriginalName, storageKey), firstNonEmpty(f.MIME, "application/octet-stream"), f.Size, nullMappedInt(userIDs, f.UploadedBy), ts(f.CreatedAt))
		if err != nil {
			return fmt.Errorf("import file %s: %w", oldID, err)
		}
		fileID, err := lookupInt64(ctx, tx, `SELECT id FROM files WHERE storage_key=?`, storageKey)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO article_files(article_id, file_id, role) VALUES(?,?, 'attachment')`, articleID, fileID); err != nil {
			return fmt.Errorf("import article file %s/%s: %w", f.ArticleID, oldID, err)
		}
	}
	return nil
}

func uniqueSlug(ctx context.Context, tx *sql.Tx, used map[string]struct{}, base string) (string, error) {
	base = markdownx.Slugify(base)
	if base == "" {
		base = "article"
	}
	for i := 0; i < 1000; i++ {
		candidate := base
		if i > 0 {
			candidate = fmt.Sprintf("%s-%d", base, i+1)
		}
		if _, ok := used[candidate]; ok {
			continue
		}
		var id int64
		err := tx.QueryRowContext(ctx, `SELECT id FROM articles WHERE slug=? LIMIT 1`, candidate).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			used[candidate] = struct{}{}
			return candidate, nil
		}
		if err != nil {
			return "", err
		}
	}
	candidate := fmt.Sprintf("%s-%d", base, time.Now().UTC().UnixNano())
	used[candidate] = struct{}{}
	return candidate, nil
}

func lookupInt64(ctx context.Context, tx *sql.Tx, query string, args ...any) (int64, error) {
	var id int64
	if err := tx.QueryRowContext(ctx, query, args...).Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

func normalizeRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "admin":
		return "admin"
	case "editor":
		return "editor"
	default:
		return "reader"
	}
}

func ts(t time.Time) string {
	if t.IsZero() {
		return time.Now().UTC().Format(time.RFC3339)
	}
	return t.UTC().Format(time.RFC3339)
}

func nullMappedInt(ids map[string]int64, oldID string) any {
	id, ok := ids[oldID]
	if !ok {
		return nil
	}
	return id
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func mergeStrings(groups ...[]string) []string {
	seen := map[string]struct{}{}
	for _, group := range groups {
		for _, value := range group {
			value = strings.ToLower(strings.TrimSpace(value))
			if value != "" {
				seen[value] = struct{}{}
			}
		}
	}
	out := make([]string, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	return out
}

func legacyFileHash(f *JSONFile) string {
	sum := sha256.Sum256([]byte(f.ID + "|" + f.StoredName + "|" + f.OriginalName + "|" + fmt.Sprint(f.Size)))
	return hex.EncodeToString(sum[:])
}
