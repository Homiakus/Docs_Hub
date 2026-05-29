package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

type JSONDatabase struct {
	Version          int                    `json:"version"`
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

func ImportJSON(ctx context.Context, db *sql.DB, jsonPath string) error {
	blob, err := os.ReadFile(jsonPath)
	if err != nil {
		return fmt.Errorf("read json: %w", err)
	}

	var src JSONDatabase
	if err := json.Unmarshal(blob, &src); err != nil {
		return fmt.Errorf("decode json: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, u := range src.Users {
		active := 0
		if u.Active {
			active = 1
		}
		_, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO users
			(id, username, salt_hex, password_hash, role, active, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			u.ID, u.Username, u.SaltHex, u.PasswordHash, u.Role, active, ts(u.CreatedAt), ts(u.UpdatedAt))
		if err != nil {
			return fmt.Errorf("import user %s: %w", u.ID, err)
		}
	}

	for _, g := range src.Groups {
		_, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO groups (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`,
			g.ID, g.Name, ts(g.CreatedAt), ts(g.UpdatedAt))
		if err != nil {
			return fmt.Errorf("import group %s: %w", g.ID, err)
		}
		for _, userID := range g.MemberIDs {
			_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO group_members (group_id, user_id) VALUES (?, ?)`, g.ID, userID)
			if err != nil {
				return fmt.Errorf("import group member %s/%s: %w", g.ID, userID, err)
			}
		}
	}

	for _, a := range src.Articles {
		archived := boolInt(a.Archived)
		allUsers := boolInt(a.AllUsers)
		_, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO articles
			(id, title, slug, content, owner_id, archived, all_users, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			a.ID, a.Title, a.Slug, a.Content, nullEmpty(a.OwnerID), archived, allUsers, ts(a.CreatedAt), ts(a.UpdatedAt))
		if err != nil {
			return fmt.Errorf("import article %s: %w", a.ID, err)
		}

		for _, userID := range a.AllowedUserIDs {
			_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO article_acl_users (article_id, user_id, permission) VALUES (?, ?, 'read')`, a.ID, userID)
			if err != nil {
				return fmt.Errorf("import article acl user %s/%s: %w", a.ID, userID, err)
			}
		}

		for _, groupID := range a.AllowedGroupIDs {
			_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO article_acl_groups (article_id, group_id, permission) VALUES (?, ?, 'read')`, a.ID, groupID)
			if err != nil {
				return fmt.Errorf("import article acl group %s/%s: %w", a.ID, groupID, err)
			}
		}

		for _, tag := range a.Tags {
			tag = strings.ToLower(strings.TrimSpace(tag))
			if tag == "" {
				continue
			}
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO tags (name) VALUES (?)`, tag); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO article_tags (article_id, tag_id) SELECT ?, id FROM tags WHERE name = ?`, a.ID, tag); err != nil {
				return err
			}
		}

		for i, v := range a.Versions {
			versionID := fmt.Sprintf("%s-v%d", a.ID, i+1)
			_, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO article_versions
				(id, article_id, actor_id, title, slug, content, created_at)
				VALUES (?, ?, ?, ?, ?, ?, ?)`,
				versionID, a.ID, nullEmpty(v.ActorID), v.Title, v.Slug, v.Content, ts(v.At))
			if err != nil {
				return fmt.Errorf("import article version %s: %w", versionID, err)
			}
		}
	}

	for _, f := range src.Attachments {
		_, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO files
			(id, sha256, storage_key, original_name, mime, size, uploaded_by, created_at)
			VALUES (?, '', ?, ?, ?, ?, ?, ?)`,
			f.ID, f.StoredName, f.OriginalName, f.MIME, f.Size, nullEmpty(f.UploadedBy), ts(f.CreatedAt))
		if err != nil {
			return fmt.Errorf("import file %s: %w", f.ID, err)
		}
		_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO article_files (article_id, file_id, role, created_at) VALUES (?, ?, 'attachment', ?)`,
			f.ArticleID, f.ID, ts(f.CreatedAt))
		if err != nil {
			return fmt.Errorf("import article file %s/%s: %w", f.ArticleID, f.ID, err)
		}
	}

	_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations (version, name) VALUES (1, 'json import baseline')`)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func ts(t time.Time) string {
	if t.IsZero() {
		return time.Now().UTC().Format(time.RFC3339Nano)
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func nullEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}
