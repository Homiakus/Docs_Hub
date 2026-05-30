package httpapp

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/homiakus/docshub-next/internal/auth"
	"github.com/homiakus/docshub-next/internal/config"
	"github.com/homiakus/docshub-next/internal/db"
	"github.com/homiakus/docshub-next/internal/markdownx"
	"github.com/homiakus/docshub-next/internal/web"
)

type Server struct {
	cfg config.Config
	db  *db.DB
	tpl *template.Template
	log *slog.Logger
}

type User struct {
	ID          int64
	Username    string
	DisplayName string
	Role        string
}

type Article struct {
	ID         int64
	Slug       string
	Title      string
	Content    string
	HTML       template.HTML
	Visibility string
	CategoryID int64
	Category   string
	UpdatedAt  string
	HasMermaid bool
	Headings   []markdownx.Heading
	Tags       []string
}

type Category struct {
	ID          int64
	Name        string
	Slug        string
	Description string
	NavOrder    int
	Visible     bool
	Count       int
}

type WikiLinkItem struct {
	Slug      string
	Label     string
	Direction string
}

type VersionEntry struct {
	VersionNo int
	Title     string
	Author    string
	CreatedAt string
	Summary   string
}

type ActivityItem struct {
	Actor     string
	Title     string
	Slug      string
	Summary   string
	CreatedAt string
}

type AdminUserRow struct {
	ID          int64
	Username    string
	DisplayName string
	Email       string
	Role        string
	Active      bool
	CreatedAt   string
	UpdatedAt   string
}

type AdminAccessRow struct {
	ArticleID    int64
	ArticleTitle string
	ArticleSlug  string
	UserID       int64
	Username     string
	Permission   string
}

type BackupRow struct {
	Name      string
	SizeBytes int64
	CreatedAt string
}

type Page struct {
	SiteName        string
	Title           string
	User            *User
	Query           string
	Error           string
	Notice          string
	Articles        []Article
	Article         Article
	Categories      []Category
	AdminCategories []Category
	AdminUsers      []AdminUserRow
	AdminAccess     []AdminAccessRow
	Backups         []BackupRow
	WikiLinks       []WikiLinkItem
	Backlinks       []Article
	Versions        []VersionEntry
	Activities      []ActivityItem
	CanWrite        bool
	Stats           string
}

func New(cfg config.Config, d *db.DB, logger *slog.Logger) (*Server, error) {
	s := &Server{cfg: cfg, db: d, log: logger}
	if err := s.bootstrap(context.Background()); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer, s.withUser)
	if s.cfg.RateLimit.Enabled {
		r.Use(s.rateLimiter())
	}
	r.Handle("/static/*", http.FileServerFS(web.FS))
	r.Get("/healthz", s.health)
	r.Get("/login", s.loginForm)
	r.Post("/login", s.login)
	r.Post("/logout", s.logout)
	r.Get("/", s.requireLogin(s.home))
	r.Get("/a/{slug}", s.requireLogin(s.article))
	r.Get("/new", s.requireEditor(s.editNew))
	r.Get("/edit/{slug}", s.requireEditor(s.editExisting))
	r.Post("/save", s.requireEditor(s.saveArticle))
	r.Post("/api/preview", s.requireLogin(s.preview))
	r.Post("/api/uploads", s.requireEditor(s.uploadFile))
	r.Get("/api/graph", s.requireLogin(s.graphAPI))
	r.Get("/uploads/{key}", s.requireLogin(s.serveUpload))
	r.Get("/graph", s.requireLogin(s.graphPage))
	r.Get("/admin", s.requireAdmin(s.admin))
	r.Post("/admin/users", s.requireAdmin(s.adminSaveUser))
	r.Post("/admin/users/password", s.requireAdmin(s.adminSetPassword))
	r.Post("/admin/articles", s.requireAdmin(s.adminSaveArticleSettings))
	r.Post("/admin/access", s.requireAdmin(s.adminSaveAccess))
	r.Post("/admin/categories", s.requireAdmin(s.adminSaveCategory))
	r.Post("/admin/backups", s.requireAdmin(s.adminBackupAction))
	r.Get("/admin/backups/{name}", s.requireAdmin(s.adminDownloadBackup))
	r.Post("/admin/import-obsidian", s.requireAdmin(s.importObsidian))
	return r
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, name string, p Page) {
	p.SiteName = s.cfg.SiteName
	p.User = userFrom(r.Context())
	if p.Categories == nil {
		p.Categories, _ = s.listCategories(r.Context(), p.User)
	}
	if p.Activities == nil {
		p.Activities, _ = s.listRecentActivity(r.Context(), p.User)
	}
	funcs := template.FuncMap{
		"eq": func(a, b any) bool { return fmt.Sprint(a) == fmt.Sprint(b) },
		"articlePath": func(slug string) template.URL {
			return template.URL("/a/" + url.PathEscape(slug))
		},
		"editPath": func(slug string) template.URL {
			return template.URL("/edit/" + url.PathEscape(slug))
		},
		"tagPath": func(tag string) template.URL {
			return template.URL("/?q=" + url.QueryEscape("#"+tag))
		},
	}
	tpl, err := template.New("base.html").Funcs(funcs).ParseFS(web.FS, "templates/base.html", "templates/"+name+".html")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("content-type", "text/html; charset=utf-8")
	if err := tpl.ExecuteTemplate(w, "base", p); err != nil {
		s.log.Error("template", "err", err)
	}
}

func (s *Server) bootstrap(ctx context.Context) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM users`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	hash, err := auth.HashPassword(s.cfg.AdminPassword)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.ExecContext(ctx, `INSERT INTO users(username, display_name, password_hash, role, created_at, updated_at) VALUES(?,?,?,?,?,?)`, s.cfg.AdminUser, "Administrator", hash, "admin", now, now)
	return err
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	arts, err := s.listArticles(r.Context(), userFrom(r.Context()), q)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	categories, _ := s.listCategories(r.Context(), userFrom(r.Context()))
	s.render(w, r, "home", Page{Title: "Главная", Query: q, Articles: arts, Categories: categories})
}

func (s *Server) loginForm(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "login", Page{Title: "Вход"})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	username, password := r.Form.Get("username"), r.Form.Get("password")
	var u User
	var hash string
	err := s.db.QueryRowContext(r.Context(), `SELECT id, username, display_name, role, password_hash FROM users WHERE username=? AND is_active=1`, username).Scan(&u.ID, &u.Username, &u.DisplayName, &u.Role, &hash)
	if err != nil || !auth.VerifyPassword(hash, password) {
		s.render(w, r, "login", Page{Title: "Вход", Error: "Неверный логин или пароль"})
		return
	}
	sid, token := randomID(24), randomID(32)
	csrf := randomID(32)
	exp := time.Now().UTC().Add(7 * 24 * time.Hour)
	_, err = s.db.ExecContext(r.Context(), `INSERT INTO sessions(id, token_hash, user_id, csrf_token, expires_at, created_at) VALUES(?,?,?,?,?,?)`, sid, hashToken(token, s.cfg.SessionSecret), u.ID, csrf, exp.Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "dh_session", Value: sid + "." + token, Path: "/", HttpOnly: true, Secure: s.cfg.CookieSecure, SameSite: http.SameSiteLaxMode, Expires: exp})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("dh_session"); err == nil {
		sid := strings.SplitN(c.Value, ".", 2)[0]
		_, _ = s.db.ExecContext(r.Context(), `DELETE FROM sessions WHERE id=?`, sid)
	}
	http.SetCookie(w, &http.Cookie{Name: "dh_session", Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) article(w http.ResponseWriter, r *http.Request) {
	slug := slugParam(r)
	a, err := s.getArticle(r.Context(), slug)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !s.canRead(r.Context(), userFrom(r.Context()), a.ID, a.Visibility) {
		http.Error(w, "forbidden", 403)
		return
	}
	back, _ := s.backlinks(r.Context(), userFrom(r.Context()), a.Slug)
	wikiLinks, _ := s.articleWikiLinks(r.Context(), userFrom(r.Context()), a.ID, a.Slug)
	versions, _ := s.articleVersions(r.Context(), a.ID)
	s.render(w, r, "article", Page{Title: a.Title, Article: a, WikiLinks: wikiLinks, Backlinks: back, Versions: versions, CanWrite: s.canWrite(userFrom(r.Context()))})
}

func (s *Server) editNew(w http.ResponseWriter, r *http.Request) {
	categories, _ := s.listAdminCategories(r.Context())
	s.render(w, r, "edit", Page{Title: "Новая статья", Article: Article{Visibility: "authenticated"}, AdminCategories: categories})
}

func (s *Server) editExisting(w http.ResponseWriter, r *http.Request) {
	a, err := s.getArticle(r.Context(), slugParam(r))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	categories, _ := s.listAdminCategories(r.Context())
	s.render(w, r, "edit", Page{Title: "Редактирование", Article: a, AdminCategories: categories})
}

func (s *Server) saveArticle(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	u := userFrom(r.Context())
	id, _ := strconv.ParseInt(r.Form.Get("id"), 10, 64)
	title := strings.TrimSpace(r.Form.Get("title"))
	slug := markdownx.Slugify(r.Form.Get("slug"))
	if slug == "" {
		slug = markdownx.Slugify(title)
	}
	if slug == "" {
		slug = "article"
	}
	if title == "" {
		title = "Без названия"
	}
	content := r.Form.Get("content")
	visibility := r.Form.Get("visibility")
	if visibility == "" {
		visibility = "authenticated"
	}
	categoryID, _ := strconv.ParseInt(r.Form.Get("category_id"), 10, 64)
	categoryName, categorySlug, err := s.categoryMeta(r.Context(), categoryID)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	res, err := markdownx.Render(content)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer tx.Rollback()
	slug, err = s.uniqueSlug(r.Context(), tx, id, slug)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	var previous articleSnapshot
	hasPrevious := false
	if id != 0 {
		err = tx.QueryRowContext(r.Context(), `SELECT slug,title,content,visibility,coalesce(category_id,0) FROM articles WHERE id=? AND deleted_at IS NULL`, id).Scan(&previous.Slug, &previous.Title, &previous.Content, &previous.Visibility, &previous.CategoryID)
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		hasPrevious = true
	}
	if id == 0 {
		row, err := tx.ExecContext(r.Context(), `INSERT INTO articles(slug,title,content,rendered_html,owner_id,visibility,created_at,updated_at,category_id) VALUES(?,?,?,?,?,?,?,?,?)`, slug, title, content, res.HTML, u.ID, visibility, now, now, nullableID(categoryID))
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		id, _ = row.LastInsertId()
	} else {
		_, err = tx.ExecContext(r.Context(), `UPDATE articles SET slug=?, title=?, content=?, rendered_html=?, visibility=?, updated_at=?, category_id=? WHERE id=?`, slug, title, content, res.HTML, visibility, now, nullableID(categoryID), id)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	}
	var versionNo int
	_ = tx.QueryRowContext(r.Context(), `SELECT coalesce(max(version_no),0)+1 FROM article_versions WHERE article_id=?`, id).Scan(&versionNo)
	_, _ = tx.ExecContext(r.Context(), `INSERT INTO article_versions(article_id, version_no, title, content, rendered_html, author_id, created_at) VALUES(?,?,?,?,?,?,?)`, id, versionNo, title, content, res.HTML, u.ID, now)
	_, _ = tx.ExecContext(r.Context(), `DELETE FROM article_tags WHERE article_id=?`, id)
	tags := articleSearchTags(res.Tags, categoryName, categorySlug)
	for _, tag := range tags {
		_, _ = tx.ExecContext(r.Context(), `INSERT OR IGNORE INTO tags(name) VALUES(?)`, tag)
		_, _ = tx.ExecContext(r.Context(), `INSERT OR IGNORE INTO article_tags(article_id, tag_id) SELECT ?, id FROM tags WHERE name=?`, id, tag)
	}
	_, _ = tx.ExecContext(r.Context(), `DELETE FROM links WHERE from_article_id=?`, id)
	for _, l := range res.Links {
		_, _ = tx.ExecContext(r.Context(), `INSERT OR IGNORE INTO links(from_article_id, target_slug, label) VALUES(?,?,?)`, id, l.Slug, l.Label)
	}
	_, _ = tx.ExecContext(r.Context(), `DELETE FROM article_files WHERE article_id=?`, id)
	for _, key := range extractUploadKeys(content) {
		var fileID int64
		if err := tx.QueryRowContext(r.Context(), `SELECT id FROM files WHERE storage_key=?`, key).Scan(&fileID); err == nil {
			_, _ = tx.ExecContext(r.Context(), `INSERT OR IGNORE INTO article_files(article_id, file_id, role) VALUES(?,?,?)`, id, fileID, "inline")
		}
	}
	_, _ = tx.ExecContext(r.Context(), `DELETE FROM article_fts WHERE rowid=?`, id)
	_, _ = tx.ExecContext(r.Context(), `INSERT INTO article_fts(rowid,title,slug,content,tags) VALUES(?,?,?,?,?)`, id, title, slug, content, strings.Join(tags, " "))
	current := articleSnapshot{Slug: slug, Title: title, Content: content, Visibility: visibility, CategoryID: categoryID}
	summary := summarizeArticleChange(previous, current, hasPrevious)
	metadata, _ := json.Marshal(map[string]any{
		"version": versionNo,
		"summary": summary,
		"slug":    slug,
		"title":   title,
	})
	_, _ = tx.ExecContext(r.Context(), `INSERT INTO audit_events(actor_id, action, entity_type, entity_id, ip, metadata_json, created_at) VALUES(?,?,?,?,?,?,?)`, u.ID, "article.save", "article", fmt.Sprint(id), clientIP(r), string(metadata), now)
	if err := tx.Commit(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	http.Redirect(w, r, "/a/"+url.PathEscape(slug), http.StatusSeeOther)
}

func (s *Server) preview(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	res, err := markdownx.Render(string(body))
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	w.Header().Set("content-type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(res.HTML))
}

func (s *Server) uploadFile(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<20)
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		http.Error(w, "file is too large or malformed", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file field is required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, 64<<20))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(data) == 0 {
		http.Error(w, "empty file", http.StatusBadRequest)
		return
	}
	mimeType := detectMediaMIME(header.Filename, header.Header.Get("Content-Type"), data)
	kind := mediaKind(mimeType)
	if kind == "" {
		http.Error(w, "only image, audio, and video files are supported", http.StatusUnsupportedMediaType)
		return
	}
	sum := sha256.Sum256(data)
	sha := hex.EncodeToString(sum[:])
	ext := safeMediaExt(header.Filename, mimeType)
	storageKey := sha + ext
	if err := os.MkdirAll(s.cfg.UploadDir, 0o750); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	path := filepath.Join(s.cfg.UploadDir, storageKey)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(path, data, 0o640); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	u := userFrom(r.Context())
	_, err = s.db.ExecContext(r.Context(), `INSERT OR IGNORE INTO files(sha256, storage_key, original_name, mime, size_bytes, uploaded_by, created_at) VALUES(?,?,?,?,?,?,?)`, sha, storageKey, header.Filename, mimeType, len(data), u.ID, now)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.db.QueryRowContext(r.Context(), `SELECT storage_key, mime, original_name FROM files WHERE sha256=?`, sha).Scan(&storageKey, &mimeType, &header.Filename); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fileURL := "/uploads/" + url.PathEscape(storageKey)
	w.Header().Set("content-type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"kind":     kind,
		"url":      fileURL,
		"filename": header.Filename,
		"mime":     mimeType,
		"markdown": mediaSnippet(kind, fileURL, header.Filename),
	})
}

func (s *Server) serveUpload(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if decoded, err := url.PathUnescape(key); err == nil {
		key = decoded
	}
	if !validStorageKey(key) {
		http.NotFound(w, r)
		return
	}
	var fileID, uploadedBy int64
	var mimeType, originalName string
	err := s.db.QueryRowContext(r.Context(), `SELECT id,mime,original_name,coalesce(uploaded_by,0) FROM files WHERE storage_key=?`, key).Scan(&fileID, &mimeType, &originalName, &uploadedBy)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	u := userFrom(r.Context())
	if u == nil && !s.fileHasPublicArticle(r.Context(), fileID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if u != nil && u.Role != "admin" && uploadedBy != u.ID && !s.userCanReadFile(r.Context(), u, fileID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	w.Header().Set("content-type", mimeType)
	w.Header().Set("content-disposition", fmt.Sprintf("inline; filename=%q", originalName))
	http.ServeFile(w, r, filepath.Join(s.cfg.UploadDir, key))
}

func (s *Server) graphPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "graph", Page{Title: "Граф"})
}

func (s *Server) graphAPI(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT slug,title FROM articles WHERE deleted_at IS NULL ORDER BY title`)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	type node struct {
		ID, Label string `json:",omitempty"`
	}
	type link struct {
		Source, Target string `json:",omitempty"`
	}
	var nodes []map[string]string
	for rows.Next() {
		var slug, title string
		_ = rows.Scan(&slug, &title)
		nodes = append(nodes, map[string]string{"id": slug, "label": title})
	}
	lr, _ := s.db.QueryContext(r.Context(), `SELECT a.slug, l.target_slug FROM links l JOIN articles a ON a.id=l.from_article_id`)
	defer lr.Close()
	var links []map[string]string
	for lr.Next() {
		var a, b string
		_ = lr.Scan(&a, &b)
		links = append(links, map[string]string{"source": a, "target": b})
	}
	w.Header().Set("content-type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"nodes": nodes, "links": links})
}

func (s *Server) admin(w http.ResponseWriter, r *http.Request) {
	var users, articles, categories, files int
	_ = s.db.QueryRowContext(r.Context(), `SELECT count(*) FROM users`).Scan(&users)
	_ = s.db.QueryRowContext(r.Context(), `SELECT count(*) FROM articles WHERE deleted_at IS NULL`).Scan(&articles)
	_ = s.db.QueryRowContext(r.Context(), `SELECT count(*) FROM categories`).Scan(&categories)
	_ = s.db.QueryRowContext(r.Context(), `SELECT count(*) FROM files`).Scan(&files)
	adminUsers, _ := s.listAdminUsers(r.Context())
	adminArticles, _ := s.listAdminArticles(r.Context())
	adminCategories, _ := s.listAdminCategories(r.Context())
	adminAccess, _ := s.listAdminAccess(r.Context())
	backups, _ := s.listBackups()
	s.render(w, r, "admin", Page{
		Title:           "Админ",
		Notice:          r.URL.Query().Get("notice"),
		Error:           r.URL.Query().Get("error"),
		Stats:           fmt.Sprintf("Пользователи: %d\nСтатьи: %d\nКатегории: %d\nФайлы: %d", users, articles, categories, files),
		Articles:        adminArticles,
		AdminUsers:      adminUsers,
		AdminCategories: adminCategories,
		AdminAccess:     adminAccess,
		Backups:         backups,
	})
}

func (s *Server) adminSaveUser(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id, _ := strconv.ParseInt(r.Form.Get("id"), 10, 64)
	username := strings.TrimSpace(r.Form.Get("username"))
	displayName := strings.TrimSpace(r.Form.Get("display_name"))
	email := strings.TrimSpace(r.Form.Get("email"))
	role := validRole(r.Form.Get("role"))
	active := r.Form.Get("is_active") == "1"
	now := time.Now().UTC().Format(time.RFC3339)

	if username == "" {
		s.redirectAdmin(w, r, "", "Логин обязателен")
		return
	}
	if id == 0 {
		password := r.Form.Get("password")
		if password == "" {
			s.redirectAdmin(w, r, "", "Пароль для нового пользователя обязателен")
			return
		}
		hash, err := auth.HashPassword(password)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, err = s.db.ExecContext(r.Context(), `INSERT INTO users(username, display_name, email, password_hash, role, is_active, created_at, updated_at) VALUES(?,?,?,?,?,?,?,?)`, username, displayName, email, hash, role, boolInt(active), now, now)
		if err != nil {
			s.redirectAdmin(w, r, "", err.Error())
			return
		}
		s.redirectAdmin(w, r, "Пользователь создан", "")
		return
	}
	if err := s.ensureAdminCanChangeUser(r.Context(), id, role, active); err != nil {
		s.redirectAdmin(w, r, "", err.Error())
		return
	}
	_, err := s.db.ExecContext(r.Context(), `UPDATE users SET username=?, display_name=?, email=?, role=?, is_active=?, updated_at=? WHERE id=?`, username, displayName, email, role, boolInt(active), now, id)
	if err != nil {
		s.redirectAdmin(w, r, "", err.Error())
		return
	}
	if !active {
		_, _ = s.db.ExecContext(r.Context(), `DELETE FROM sessions WHERE user_id=?`, id)
	}
	s.redirectAdmin(w, r, "Пользователь обновлен", "")
}

func (s *Server) adminSetPassword(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id, _ := strconv.ParseInt(r.Form.Get("id"), 10, 64)
	password := r.Form.Get("password")
	if id == 0 || password == "" {
		s.redirectAdmin(w, r, "", "Выберите пользователя и задайте пароль")
		return
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.ExecContext(r.Context(), `UPDATE users SET password_hash=?, updated_at=? WHERE id=?`, hash, now, id)
	if err != nil {
		s.redirectAdmin(w, r, "", err.Error())
		return
	}
	_, _ = s.db.ExecContext(r.Context(), `DELETE FROM sessions WHERE user_id=?`, id)
	s.redirectAdmin(w, r, "Пароль обновлен, активные сессии сброшены", "")
}

func (s *Server) adminSaveArticleSettings(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id, _ := strconv.ParseInt(r.Form.Get("id"), 10, 64)
	if id == 0 {
		s.redirectAdmin(w, r, "", "Статья не выбрана")
		return
	}
	if r.Form.Get("action") == "delete" {
		now := time.Now().UTC().Format(time.RFC3339)
		_, err := s.db.ExecContext(r.Context(), `UPDATE articles SET deleted_at=?, updated_at=? WHERE id=?`, now, now, id)
		if err != nil {
			s.redirectAdmin(w, r, "", err.Error())
			return
		}
		_, _ = s.db.ExecContext(r.Context(), `DELETE FROM article_fts WHERE rowid=?`, id)
		s.redirectAdmin(w, r, "Статья скрыта", "")
		return
	}
	visibility := validVisibility(r.Form.Get("visibility"))
	categoryID, _ := strconv.ParseInt(r.Form.Get("category_id"), 10, 64)
	if _, _, err := s.categoryMeta(r.Context(), categoryID); err != nil {
		s.redirectAdmin(w, r, "", err.Error())
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(r.Context(), `UPDATE articles SET visibility=?, category_id=?, updated_at=? WHERE id=?`, visibility, nullableID(categoryID), now, id)
	if err != nil {
		s.redirectAdmin(w, r, "", err.Error())
		return
	}
	s.redirectAdmin(w, r, "Параметры статьи обновлены", "")
}

func (s *Server) adminSaveAccess(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	articleID, _ := strconv.ParseInt(r.Form.Get("article_id"), 10, 64)
	userID, _ := strconv.ParseInt(r.Form.Get("user_id"), 10, 64)
	permission := r.Form.Get("permission")
	if articleID == 0 || userID == 0 {
		s.redirectAdmin(w, r, "", "Выберите статью и пользователя")
		return
	}
	_, _ = s.db.ExecContext(r.Context(), `DELETE FROM acl_users WHERE article_id=? AND user_id=?`, articleID, userID)
	if permission != "" && permission != "remove" {
		permission = validPermission(permission)
		_, err := s.db.ExecContext(r.Context(), `INSERT INTO acl_users(article_id, user_id, permission) VALUES(?,?,?)`, articleID, userID, permission)
		if err != nil {
			s.redirectAdmin(w, r, "", err.Error())
			return
		}
		s.redirectAdmin(w, r, "Доступ обновлен", "")
		return
	}
	s.redirectAdmin(w, r, "Доступ удален", "")
}

func (s *Server) adminSaveCategory(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id, _ := strconv.ParseInt(r.Form.Get("id"), 10, 64)
	if r.Form.Get("action") == "delete" {
		if id == 0 {
			s.redirectAdmin(w, r, "", "Категория не выбрана")
			return
		}
		tx, err := s.db.BeginTx(r.Context(), nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()
		_, _ = tx.ExecContext(r.Context(), `UPDATE articles SET category_id=NULL WHERE category_id=?`, id)
		if _, err := tx.ExecContext(r.Context(), `DELETE FROM categories WHERE id=?`, id); err != nil {
			s.redirectAdmin(w, r, "", err.Error())
			return
		}
		if err := tx.Commit(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.redirectAdmin(w, r, "Категория удалена", "")
		return
	}
	name := strings.TrimSpace(r.Form.Get("name"))
	slug := markdownx.Slugify(r.Form.Get("slug"))
	if slug == "" {
		slug = markdownx.Slugify(name)
	}
	if name == "" || slug == "" {
		s.redirectAdmin(w, r, "", "Название категории обязательно")
		return
	}
	description := strings.TrimSpace(r.Form.Get("description"))
	navOrder, _ := strconv.Atoi(r.Form.Get("nav_order"))
	visible := r.Form.Get("is_visible") == "1"
	now := time.Now().UTC().Format(time.RFC3339)
	if id == 0 {
		_, err := s.db.ExecContext(r.Context(), `INSERT INTO categories(name, slug, description, nav_order, is_visible, created_at, updated_at) VALUES(?,?,?,?,?,?,?)`, name, slug, description, navOrder, boolInt(visible), now, now)
		if err != nil {
			s.redirectAdmin(w, r, "", err.Error())
			return
		}
		s.redirectAdmin(w, r, "Категория создана", "")
		return
	}
	_, err := s.db.ExecContext(r.Context(), `UPDATE categories SET name=?, slug=?, description=?, nav_order=?, is_visible=?, updated_at=? WHERE id=?`, name, slug, description, navOrder, boolInt(visible), now, id)
	if err != nil {
		s.redirectAdmin(w, r, "", err.Error())
		return
	}
	s.redirectAdmin(w, r, "Категория обновлена", "")
}

func (s *Server) adminBackupAction(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	switch r.Form.Get("action") {
	case "delete":
		name := r.Form.Get("name")
		if !validBackupName(name) {
			s.redirectAdmin(w, r, "", "Некорректное имя бэкапа")
			return
		}
		if err := os.Remove(filepath.Join(s.backupDir(), name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			s.redirectAdmin(w, r, "", err.Error())
			return
		}
		s.redirectAdmin(w, r, "Бэкап удален", "")
	default:
		if err := os.MkdirAll(s.backupDir(), 0o750); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		name := "docshub-" + time.Now().UTC().Format("20060102T150405Z") + ".db"
		path := filepath.Join(s.backupDir(), name)
		quoted := strings.ReplaceAll(path, `'`, `''`)
		if _, err := s.db.ExecContext(r.Context(), `VACUUM INTO '`+quoted+`'`); err != nil {
			s.redirectAdmin(w, r, "", err.Error())
			return
		}
		s.redirectAdmin(w, r, "Бэкап создан: "+name, "")
	}
}

func (s *Server) adminDownloadBackup(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if decoded, err := url.PathUnescape(name); err == nil {
		name = decoded
	}
	if !validBackupName(name) {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, filepath.Join(s.backupDir(), name))
}

func (s *Server) importObsidian(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 128<<20) // 128MB limit
	if err := r.ParseMultipartForm(128 << 20); err != nil {
		s.redirectAdmin(w, r, "", "архив слишком большой или повреждён")
		return
	}
	file, header, err := r.FormFile("vault")
	if err != nil {
		s.redirectAdmin(w, r, "", "файл vault обязателен")
		return
	}
	defer file.Close()

	if !strings.HasSuffix(strings.ToLower(header.Filename), ".zip") {
		s.redirectAdmin(w, r, "", "ожидается .zip архив Obsidian хранилища")
		return
	}

	// Read entire ZIP into memory (125MB cap via MaxBytesReader)
	data, err := io.ReadAll(file)
	if err != nil {
		s.redirectAdmin(w, r, "", "не удалось прочитать архив: "+err.Error())
		return
	}

	zipReader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		s.redirectAdmin(w, r, "", "некорректный zip-архив: "+err.Error())
		return
	}

	u := userFrom(r.Context())
	now := time.Now().UTC().Format(time.RFC3339)
	var importedFiles, importedArticles int

	// First pass: collect all files, split into attachments and markdown
	type zipEntry struct {
		Name    string
		Content []byte
	}
	var attachments []zipEntry
	var markdownFiles []zipEntry

	for _, f := range zipReader.File {
		if f.FileInfo().IsDir() {
			continue
		}
		// Skip hidden files and MacOS resource forks
		base := filepath.Base(f.Name)
		if strings.HasPrefix(base, ".") || strings.HasPrefix(base, "__MACOSX") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		content, err := io.ReadAll(io.LimitReader(rc, 10<<20)) // 10MB per file
		rc.Close()
		if err != nil || len(content) == 0 {
			continue
		}

		ext := strings.ToLower(filepath.Ext(f.Name))
		if ext == ".md" {
			markdownFiles = append(markdownFiles, zipEntry{Name: f.Name, Content: content})
		} else if isMediaFile(ext) {
			attachments = append(attachments, zipEntry{Name: f.Name, Content: content})
		}
	}

	// Upload all attachments, build filename→URL map
	attachMap := make(map[string]string) // original filename → /uploads/key
	for _, a := range attachments {
		mimeType := detectMediaMIME(a.Name, "", a.Content)
		if mimeType == "" {
			continue
		}
		if mediaKind(mimeType) == "" {
			continue
		}
		sum := sha256.Sum256(a.Content)
		sha := hex.EncodeToString(sum[:])
		ext := safeMediaExt(a.Name, mimeType)
		storageKey := sha + ext

		if err := os.MkdirAll(s.cfg.UploadDir, 0o750); err != nil {
			continue
		}
		diskPath := filepath.Join(s.cfg.UploadDir, storageKey)
		if _, err := os.Stat(diskPath); errors.Is(err, os.ErrNotExist) {
			if err := os.WriteFile(diskPath, a.Content, 0o640); err != nil {
				continue
			}
		}
		_, _ = s.db.ExecContext(r.Context(), `INSERT OR IGNORE INTO files(sha256, storage_key, original_name, mime, size_bytes, uploaded_by, created_at) VALUES(?,?,?,?,?,?,?)`,
			sha, storageKey, filepath.Base(a.Name), mimeType, len(a.Content), u.ID, now)
		attachMap[filepath.Base(a.Name)] = "/uploads/" + url.PathEscape(storageKey)
		importedFiles++
	}

	// Obsidian embed regex: ![[filename.png]] or ![[filename.png|300]]
	embedRe := regexp.MustCompile(`!\[\[([^\]|]+)(?:\|(\d+))?\]\]`)

	// Second pass: import markdown files as articles
	for _, mf := range markdownFiles {
		content := string(mf.Content)
		title := strings.TrimSuffix(filepath.Base(mf.Name), ".md")

		// Replace ![[file]] embeds with proper markdown or HTML
		content = embedRe.ReplaceAllStringFunc(content, func(raw string) string {
			parts := embedRe.FindStringSubmatch(raw)
			if len(parts) == 0 {
				return raw
			}
			filename := strings.TrimSpace(parts[1])
			width := parts[2]
			if url, ok := attachMap[filename]; ok {
				if width != "" {
					return fmt.Sprintf(`<img src="%s" width="%s" alt="%s">`, url, width, filename)
				}
				return fmt.Sprintf("![%s](%s)", filename, url)
			}
			// Attachment not found in vault — leave as plain text link
			return fmt.Sprintf("[📎 %s](%s)", filename, filename)
		})

		res, err := markdownx.Render(content)
		if err != nil {
			s.log.Warn("obsidian import render", "file", mf.Name, "err", err)
			continue
		}

		slug := markdownx.Slugify(title)
		if slug == "" {
			slug = "obsidian-" + markdownx.Slugify(filepath.Base(mf.Name))
		}

		tx, err := s.db.BeginTx(r.Context(), nil)
		if err != nil {
			continue
		}

		slug, _ = s.uniqueSlug(r.Context(), tx, 0, slug)

		result, err := tx.ExecContext(r.Context(),
			`INSERT INTO articles(slug,title,content,rendered_html,owner_id,visibility,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`,
			slug, title, content, res.HTML, u.ID, "authenticated", now, now)
		if err != nil {
			tx.Rollback()
			s.log.Warn("obsidian import insert", "file", mf.Name, "err", err)
			continue
		}

		articleID, _ := result.LastInsertId()

		// Save version 1
		_, _ = tx.ExecContext(r.Context(),
			`INSERT INTO article_versions(article_id, version_no, title, content, rendered_html, author_id, created_at) VALUES(?,1,?,?,?,?,?)`,
			articleID, title, content, res.HTML, u.ID, now)

		// Save tags
		for _, tag := range res.Tags {
			_, _ = tx.ExecContext(r.Context(), `INSERT OR IGNORE INTO tags(name) VALUES(?)`, tag)
			_, _ = tx.ExecContext(r.Context(), `INSERT OR IGNORE INTO article_tags(article_id, tag_id) SELECT ?, id FROM tags WHERE name=?`, articleID, tag)
		}

		// Save wiki links
		for _, l := range res.Links {
			_, _ = tx.ExecContext(r.Context(), `INSERT OR IGNORE INTO links(from_article_id, target_slug, label) VALUES(?,?,?)`, articleID, l.Slug, l.Label)
		}

		// Associate attachments with the article
		attachKeys := extractUploadKeys(content)
		for _, key := range attachKeys {
			var fileID int64
			if err := tx.QueryRowContext(r.Context(), `SELECT id FROM files WHERE storage_key=?`, key).Scan(&fileID); err == nil {
				_, _ = tx.ExecContext(r.Context(), `INSERT OR IGNORE INTO article_files(article_id, file_id, role) VALUES(?,?,?)`, articleID, fileID, "inline")
			}
		}

		// FTS index
		tags := articleSearchTags(res.Tags, "", "")
		_, _ = tx.ExecContext(r.Context(), `DELETE FROM article_fts WHERE rowid=?`, articleID)
		_, _ = tx.ExecContext(r.Context(), `INSERT INTO article_fts(rowid,title,slug,content,tags) VALUES(?,?,?,?,?)`, articleID, title, slug, content, strings.Join(tags, " "))

		// Audit
		metadata, _ := json.Marshal(map[string]any{
			"version": 1,
			"summary": "Импортировано из Obsidian vault: " + mf.Name,
			"slug":    slug,
			"title":   title,
		})
		_, _ = tx.ExecContext(r.Context(), `INSERT INTO audit_events(actor_id, action, entity_type, entity_id, ip, metadata_json, created_at) VALUES(?,?,?,?,?,?,?)`,
			u.ID, "obsidian.import", "article", fmt.Sprint(articleID), clientIP(r), string(metadata), now)

		if err := tx.Commit(); err != nil {
			tx.Rollback()
			continue
		}
		importedArticles++
	}

	s.redirectAdmin(w, r,
		fmt.Sprintf("Obsidian vault импортирован: %d статей, %d файлов", importedArticles, importedFiles),
		"")
}

func isMediaFile(ext string) bool {
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".bmp",
		".mp3", ".wav", ".ogg", ".flac", ".m4a",
		".mp4", ".webm", ".mov", ".avi",
		".pdf":
		return true
	}
	return false
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("content-type", "application/json")
	dbOK := s.db.PingContext(r.Context()) == nil
	status := "ok"
	httpStatus := 200
	if !dbOK {
		status = "degraded"
		httpStatus = 503
	}
	w.WriteHeader(httpStatus)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": status,
		"app":    "docshub-next",
		"time":   time.Now().UTC(),
		"db":     dbOK,
	})
}

func (s *Server) listArticles(ctx context.Context, u *User, q string) ([]Article, error) {
	var rows *sql.Rows
	var err error
	if q != "" {
		needle := strings.TrimPrefix(q, "#")
		if strings.HasPrefix(q, "#") {
			rows, err = s.db.QueryContext(ctx, `SELECT DISTINCT a.id,a.slug,a.title,a.updated_at,a.visibility,coalesce(a.category_id,0),coalesce(c.name,'') FROM articles a LEFT JOIN categories c ON c.id=a.category_id LEFT JOIN article_tags at ON at.article_id=a.id LEFT JOIN tags t ON t.id=at.tag_id WHERE a.deleted_at IS NULL AND (c.slug=? OR c.name=? OR t.name=?) ORDER BY a.updated_at DESC`, needle, needle, needle)
		} else {
			rows, err = s.db.QueryContext(ctx, `SELECT a.id,a.slug,a.title,a.updated_at,a.visibility,coalesce(a.category_id,0),coalesce(c.name,'') FROM article_fts f JOIN articles a ON a.id=f.rowid LEFT JOIN categories c ON c.id=a.category_id WHERE article_fts MATCH ? AND a.deleted_at IS NULL ORDER BY rank`, needle+"*")
		}
	} else {
		rows, err = s.db.QueryContext(ctx, `SELECT a.id,a.slug,a.title,a.updated_at,a.visibility,coalesce(a.category_id,0),coalesce(c.name,'') FROM articles a LEFT JOIN categories c ON c.id=a.category_id WHERE a.deleted_at IS NULL ORDER BY a.updated_at DESC LIMIT 100`)
	}
	if err != nil {
		return nil, err
	}
	var candidates []Article
	for rows.Next() {
		var a Article
		if err := rows.Scan(&a.ID, &a.Slug, &a.Title, &a.UpdatedAt, &a.Visibility, &a.CategoryID, &a.Category); err == nil {
			candidates = append(candidates, a)
		}
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var out []Article
	for _, a := range candidates {
		if s.canRead(ctx, u, a.ID, a.Visibility) {
			out = append(out, a)
		}
	}
	return out, nil
}

func (s *Server) getArticle(ctx context.Context, slug string) (Article, error) {
	if decoded, err := url.PathUnescape(slug); err == nil {
		slug = decoded
	}
	var a Article
	var html string
	err := s.db.QueryRowContext(ctx, `SELECT a.id,a.slug,a.title,a.content,a.rendered_html,a.visibility,a.updated_at,coalesce(a.category_id,0),coalesce(c.name,'') FROM articles a LEFT JOIN categories c ON c.id=a.category_id WHERE a.slug=? AND a.deleted_at IS NULL`, slug).Scan(&a.ID, &a.Slug, &a.Title, &a.Content, &html, &a.Visibility, &a.UpdatedAt, &a.CategoryID, &a.Category)
	a.HTML = template.HTML(html)
	if a.Content != "" {
		if res, renderErr := markdownx.Render(a.Content); renderErr == nil {
			a.HasMermaid = res.Mermaid
			a.Headings = res.Headings
			a.Tags = res.Tags
		}
	}
	return a, err
}

func (s *Server) uniqueSlug(ctx context.Context, tx *sql.Tx, articleID int64, base string) (string, error) {
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
	return fmt.Sprintf("%s-%d", base, time.Now().UTC().UnixNano()), nil
}

func (s *Server) listCategories(ctx context.Context, u *User) ([]Category, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT c.id,c.name,c.slug,c.description,c.nav_order,c.is_visible,coalesce(a.id,0),coalesce(a.visibility,'') FROM categories c LEFT JOIN articles a ON a.category_id=c.id AND a.deleted_at IS NULL WHERE c.is_visible=1 ORDER BY c.nav_order, lower(c.name)`)
	if err != nil {
		return nil, err
	}
	type candidate struct {
		category   Category
		articleID  int64
		visibility string
	}
	var candidates []candidate
	for rows.Next() {
		var item candidate
		var visible int
		if err := rows.Scan(&item.category.ID, &item.category.Name, &item.category.Slug, &item.category.Description, &item.category.NavOrder, &visible, &item.articleID, &item.visibility); err == nil {
			item.category.Visible = visible == 1
			candidates = append(candidates, item)
		}
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	categories := map[int64]Category{}
	for _, item := range candidates {
		cat := categories[item.category.ID]
		if cat.ID == 0 {
			cat = item.category
		}
		if item.articleID > 0 && s.canRead(ctx, u, item.articleID, item.visibility) {
			cat.Count++
		}
		categories[item.category.ID] = cat
	}
	out := make([]Category, 0, len(categories))
	for _, category := range categories {
		out = append(out, category)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].NavOrder == out[j].NavOrder {
			return out[i].Name < out[j].Name
		}
		return out[i].NavOrder < out[j].NavOrder
	})
	if len(out) > 80 {
		out = out[:80]
	}
	return out, nil
}

func (s *Server) backlinks(ctx context.Context, u *User, slug string) ([]Article, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT a.id,a.slug,a.title,a.updated_at,a.visibility FROM links l JOIN articles a ON a.id=l.from_article_id WHERE l.target_slug=? AND a.deleted_at IS NULL ORDER BY a.updated_at DESC`, slug)
	if err != nil {
		return nil, err
	}
	var candidates []Article
	for rows.Next() {
		var a Article
		if err := rows.Scan(&a.ID, &a.Slug, &a.Title, &a.UpdatedAt, &a.Visibility); err == nil {
			candidates = append(candidates, a)
		}
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var out []Article
	for _, a := range candidates {
		if s.canRead(ctx, u, a.ID, a.Visibility) {
			out = append(out, a)
		}
	}
	return out, nil
}

func (s *Server) articleWikiLinks(ctx context.Context, u *User, articleID int64, slug string) ([]WikiLinkItem, error) {
	var out []WikiLinkItem
	rows, err := s.db.QueryContext(ctx, `SELECT target_slug,label FROM links WHERE from_article_id=? ORDER BY target_slug LIMIT 24`, articleID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var item WikiLinkItem
		_ = rows.Scan(&item.Slug, &item.Label)
		if item.Label == "" {
			item.Label = item.Slug
		}
		item.Direction = "out"
		out = append(out, item)
	}
	rows.Close()

	rows, err = s.db.QueryContext(ctx, `SELECT a.id,a.slug,a.title,a.visibility FROM links l JOIN articles a ON a.id=l.from_article_id WHERE l.target_slug=? AND a.deleted_at IS NULL ORDER BY a.updated_at DESC LIMIT 24`, slug)
	if err != nil {
		return out, err
	}
	type candidate struct {
		id         int64
		item       WikiLinkItem
		visibility string
	}
	var candidates []candidate
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.id, &item.item.Slug, &item.item.Label, &item.visibility); err == nil {
			candidates = append(candidates, item)
		}
	}
	if err := rows.Close(); err != nil {
		return out, err
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	for _, candidate := range candidates {
		if s.canRead(ctx, u, candidate.id, candidate.visibility) {
			candidate.item.Direction = "back"
			out = append(out, candidate.item)
		}
	}
	return out, nil
}

func (s *Server) articleVersions(ctx context.Context, articleID int64) ([]VersionEntry, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT av.version_no,av.title,av.content,coalesce(nullif(u.display_name,''), nullif(u.username,''), 'system'),av.created_at FROM article_versions av LEFT JOIN users u ON u.id=av.author_id WHERE av.article_id=? ORDER BY av.version_no DESC LIMIT 18`, articleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type snapshot struct {
		VersionEntry
		Content string
	}
	var snaps []snapshot
	for rows.Next() {
		var s snapshot
		if err := rows.Scan(&s.VersionNo, &s.Title, &s.Content, &s.Author, &s.CreatedAt); err == nil {
			snaps = append(snaps, s)
		}
	}
	for i := range snaps {
		if i+1 >= len(snaps) {
			snaps[i].Summary = "Создана статья"
			continue
		}
		prev := articleSnapshot{Title: snaps[i+1].Title, Content: snaps[i+1].Content}
		cur := articleSnapshot{Title: snaps[i].Title, Content: snaps[i].Content}
		snaps[i].Summary = summarizeArticleChange(prev, cur, true)
	}
	out := make([]VersionEntry, 0, len(snaps))
	for _, item := range snaps {
		out = append(out, item.VersionEntry)
	}
	return out, rows.Err()
}

func (s *Server) listRecentActivity(ctx context.Context, u *User) ([]ActivityItem, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT ae.entity_id,ae.metadata_json,ae.created_at,coalesce(nullif(actor.display_name,''), nullif(actor.username,''), 'system'),coalesce(a.id,0),coalesce(a.slug,''),coalesce(a.title,''),coalesce(a.visibility,'') FROM audit_events ae LEFT JOIN users actor ON actor.id=ae.actor_id LEFT JOIN articles a ON a.id=CAST(ae.entity_id AS INTEGER) AND ae.entity_type='article' WHERE ae.entity_type='article' ORDER BY ae.created_at DESC LIMIT 40`)
	if err != nil {
		return nil, err
	}
	type candidate struct {
		entityID   string
		metadata   string
		createdAt  string
		actor      string
		articleID  int64
		slug       string
		title      string
		visibility string
	}
	var candidates []candidate
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.entityID, &item.metadata, &item.createdAt, &item.actor, &item.articleID, &item.slug, &item.title, &item.visibility); err == nil {
			candidates = append(candidates, item)
		}
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var out []ActivityItem
	for _, item := range candidates {
		if item.articleID > 0 && !s.canRead(ctx, u, item.articleID, item.visibility) {
			continue
		}
		var meta map[string]any
		_ = json.Unmarshal([]byte(item.metadata), &meta)
		if item.title == "" {
			if v, ok := meta["title"].(string); ok {
				item.title = v
			}
		}
		if item.title == "" {
			item.title = "article " + item.entityID
		}
		summary := "Сохранена статья"
		if v, ok := meta["summary"].(string); ok && v != "" {
			summary = v
		}
		out = append(out, ActivityItem{Actor: item.actor, Title: item.title, Slug: item.slug, Summary: summary, CreatedAt: item.createdAt})
		if len(out) == 6 {
			break
		}
	}
	return out, nil
}

func (s *Server) listAdminUsers(ctx context.Context) ([]AdminUserRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,username,display_name,email,role,is_active,created_at,updated_at FROM users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AdminUserRow
	for rows.Next() {
		var item AdminUserRow
		var active int
		if err := rows.Scan(&item.ID, &item.Username, &item.DisplayName, &item.Email, &item.Role, &active, &item.CreatedAt, &item.UpdatedAt); err == nil {
			item.Active = active == 1
			out = append(out, item)
		}
	}
	return out, rows.Err()
}

func (s *Server) listAdminArticles(ctx context.Context) ([]Article, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT a.id,a.slug,a.title,a.updated_at,a.visibility,coalesce(a.category_id,0),coalesce(c.name,'') FROM articles a LEFT JOIN categories c ON c.id=a.category_id WHERE a.deleted_at IS NULL ORDER BY a.updated_at DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Article
	for rows.Next() {
		var item Article
		if err := rows.Scan(&item.ID, &item.Slug, &item.Title, &item.UpdatedAt, &item.Visibility, &item.CategoryID, &item.Category); err == nil {
			out = append(out, item)
		}
	}
	return out, rows.Err()
}

func (s *Server) listAdminCategories(ctx context.Context) ([]Category, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT c.id,c.name,c.slug,c.description,c.nav_order,c.is_visible,count(a.id) FROM categories c LEFT JOIN articles a ON a.category_id=c.id AND a.deleted_at IS NULL GROUP BY c.id,c.name,c.slug,c.description,c.nav_order,c.is_visible ORDER BY c.nav_order, lower(c.name)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Category
	for rows.Next() {
		var item Category
		var visible int
		if err := rows.Scan(&item.ID, &item.Name, &item.Slug, &item.Description, &item.NavOrder, &visible, &item.Count); err == nil {
			item.Visible = visible == 1
			out = append(out, item)
		}
	}
	return out, rows.Err()
}

func (s *Server) listAdminAccess(ctx context.Context) ([]AdminAccessRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT a.id,a.title,a.slug,u.id,u.username,au.permission FROM acl_users au JOIN articles a ON a.id=au.article_id JOIN users u ON u.id=au.user_id WHERE a.deleted_at IS NULL ORDER BY a.title,u.username,au.permission`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AdminAccessRow
	for rows.Next() {
		var item AdminAccessRow
		if err := rows.Scan(&item.ArticleID, &item.ArticleTitle, &item.ArticleSlug, &item.UserID, &item.Username, &item.Permission); err == nil {
			out = append(out, item)
		}
	}
	return out, rows.Err()
}

func (s *Server) listBackups() ([]BackupRow, error) {
	entries, err := os.ReadDir(s.backupDir())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []BackupRow
	for _, entry := range entries {
		if entry.IsDir() || !validBackupName(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		out = append(out, BackupRow{Name: entry.Name(), SizeBytes: info.Size(), CreatedAt: info.ModTime().Format(time.RFC3339)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out, nil
}

func (s *Server) categoryMeta(ctx context.Context, id int64) (string, string, error) {
	if id == 0 {
		return "", "", nil
	}
	var name, slug string
	if err := s.db.QueryRowContext(ctx, `SELECT name,slug FROM categories WHERE id=?`, id).Scan(&name, &slug); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", "", fmt.Errorf("категория не найдена")
		}
		return "", "", err
	}
	return name, slug, nil
}

func (s *Server) ensureAdminCanChangeUser(ctx context.Context, userID int64, nextRole string, active bool) error {
	var currentRole string
	var currentActive int
	if err := s.db.QueryRowContext(ctx, `SELECT role,is_active FROM users WHERE id=?`, userID).Scan(&currentRole, &currentActive); err != nil {
		return err
	}
	if currentRole != "admin" || (nextRole == "admin" && active) {
		return nil
	}
	var otherAdmins int
	_ = s.db.QueryRowContext(ctx, `SELECT count(*) FROM users WHERE id<>? AND role='admin' AND is_active=1`, userID).Scan(&otherAdmins)
	if otherAdmins == 0 {
		return fmt.Errorf("нельзя отключить или понизить последнего активного администратора")
	}
	return nil
}

func (s *Server) redirectAdmin(w http.ResponseWriter, r *http.Request, notice, errText string) {
	values := url.Values{}
	if notice != "" {
		values.Set("notice", notice)
	}
	if errText != "" {
		values.Set("error", errText)
	}
	target := "/admin"
	if encoded := values.Encode(); encoded != "" {
		target += "?" + encoded
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func (s *Server) backupDir() string {
	base := s.cfg.DataDir
	if base == "" {
		base = filepath.Dir(s.cfg.DBPath)
	}
	return filepath.Join(base, "backups")
}

func (s *Server) fileHasPublicArticle(ctx context.Context, fileID int64) bool {
	var n int
	_ = s.db.QueryRowContext(ctx, `SELECT count(*) FROM article_files af JOIN articles a ON a.id=af.article_id WHERE af.file_id=? AND a.visibility='public' AND a.deleted_at IS NULL`, fileID).Scan(&n)
	return n > 0
}

func (s *Server) userCanReadFile(ctx context.Context, u *User, fileID int64) bool {
	rows, err := s.db.QueryContext(ctx, `SELECT a.id,a.visibility FROM article_files af JOIN articles a ON a.id=af.article_id WHERE af.file_id=? AND a.deleted_at IS NULL`, fileID)
	if err != nil {
		return false
	}
	type candidate struct {
		articleID  int64
		visibility string
	}
	var candidates []candidate
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.articleID, &item.visibility); err == nil {
			candidates = append(candidates, item)
		}
	}
	rows.Close()
	for _, item := range candidates {
		if s.canRead(ctx, u, item.articleID, item.visibility) {
			return true
		}
	}
	return false
}

func (s *Server) canRead(ctx context.Context, u *User, articleID int64, visibility string) bool {
	if visibility == "public" {
		return true
	}
	if u == nil {
		return false
	}
	if u.Role == "admin" {
		return true
	}
	if visibility == "authenticated" {
		return true
	}
	var n int
	_ = s.db.QueryRowContext(ctx, `SELECT count(*) FROM acl_users WHERE article_id=? AND user_id=? AND permission IN ('read','write','admin')`, articleID, u.ID).Scan(&n)
	if n > 0 {
		return true
	}
	_ = s.db.QueryRowContext(ctx, `SELECT count(*) FROM acl_groups ag JOIN group_members gm ON gm.group_id=ag.group_id WHERE ag.article_id=? AND gm.user_id=? AND ag.permission IN ('read','write','admin')`, articleID, u.ID).Scan(&n)
	return n > 0
}
func (s *Server) canWrite(u *User) bool { return u != nil && (u.Role == "admin" || u.Role == "editor") }

type articleSnapshot struct {
	Slug       string
	Title      string
	Content    string
	Visibility string
	CategoryID int64
}

var (
	storageKeyRe = regexp.MustCompile(`^[a-f0-9]{64}(\.[a-z0-9]+)?$`)
	uploadRefRe  = regexp.MustCompile(`/uploads/([A-Za-z0-9%._~-]+)`)
	extRe        = regexp.MustCompile(`^\.[a-z0-9]{1,12}$`)
	backupNameRe = regexp.MustCompile(`^docshub-[0-9]{8}T[0-9]{6}Z\.db$`)
)

func summarizeArticleChange(previous, current articleSnapshot, hasPrevious bool) string {
	if !hasPrevious {
		return "Создана статья"
	}
	var changes []string
	if previous.Title != current.Title {
		changes = append(changes, "изменен заголовок")
	}
	if previous.Slug != "" && previous.Slug != current.Slug {
		changes = append(changes, "изменен slug")
	}
	if previous.Visibility != "" && previous.Visibility != current.Visibility {
		changes = append(changes, "изменена видимость")
	}
	if previous.CategoryID != current.CategoryID {
		changes = append(changes, "изменена категория")
	}
	if previous.Content != current.Content {
		added, removed := lineDelta(previous.Content, current.Content)
		if added == 0 && removed == 0 {
			changes = append(changes, "изменен текст")
		} else {
			changes = append(changes, fmt.Sprintf("текст: +%d / -%d строк", added, removed))
		}
		mediaAdded, mediaRemoved := mediaDelta(previous.Content, current.Content)
		if mediaAdded > 0 || mediaRemoved > 0 {
			changes = append(changes, fmt.Sprintf("медиа: +%d / -%d", mediaAdded, mediaRemoved))
		}
	}
	if len(changes) == 0 {
		return "Сохранение без изменений"
	}
	return strings.Join(changes, ", ")
}

func validRole(role string) string {
	switch role {
	case "admin", "editor", "reader":
		return role
	default:
		return "reader"
	}
}

func validVisibility(visibility string) string {
	switch visibility {
	case "private", "authenticated", "public":
		return visibility
	default:
		return "authenticated"
	}
}

func validPermission(permission string) string {
	switch permission {
	case "read", "write", "admin":
		return permission
	default:
		return "read"
	}
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func nullableID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

func articleSearchTags(markdownTags []string, categoryName, categorySlug string) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	add(categoryName)
	add(categorySlug)
	for _, tag := range markdownTags {
		add(tag)
	}
	return out
}

func validBackupName(name string) bool {
	return backupNameRe.MatchString(name)
}

func lineDelta(previous, current string) (int, int) {
	prev := lineCounts(previous)
	cur := lineCounts(current)
	var added, removed int
	for line, n := range cur {
		if diff := n - prev[line]; diff > 0 {
			added += diff
		}
	}
	for line, n := range prev {
		if diff := n - cur[line]; diff > 0 {
			removed += diff
		}
	}
	return added, removed
}

func lineCounts(s string) map[string]int {
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	out := make(map[string]int, len(lines))
	for _, line := range lines {
		out[line]++
	}
	return out
}

func mediaDelta(previous, current string) (int, int) {
	prev := stringSet(extractUploadKeys(previous))
	cur := stringSet(extractUploadKeys(current))
	var added, removed int
	for key := range cur {
		if _, ok := prev[key]; !ok {
			added++
		}
	}
	for key := range prev {
		if _, ok := cur[key]; !ok {
			removed++
		}
	}
	return added, removed
}

func stringSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func extractUploadKeys(s string) []string {
	seen := map[string]struct{}{}
	for _, match := range uploadRefRe.FindAllStringSubmatch(s, -1) {
		key := match[1]
		if decoded, err := url.PathUnescape(key); err == nil {
			key = decoded
		}
		if validStorageKey(key) {
			seen[key] = struct{}{}
		}
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func validStorageKey(key string) bool {
	return storageKeyRe.MatchString(key)
}

func detectMediaMIME(filename, header string, data []byte) string {
	candidates := []string{header, mime.TypeByExtension(strings.ToLower(filepath.Ext(filename))), http.DetectContentType(data)}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		mediaType, _, err := mime.ParseMediaType(candidate)
		if err != nil {
			mediaType = candidate
		}
		mediaType = strings.ToLower(strings.TrimSpace(mediaType))
		if mediaKind(mediaType) != "" {
			return mediaType
		}
	}
	return ""
}

func mediaKind(mimeType string) string {
	if mimeType == "image/svg+xml" {
		return ""
	}
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		return "image"
	case strings.HasPrefix(mimeType, "audio/"):
		return "audio"
	case strings.HasPrefix(mimeType, "video/"):
		return "video"
	default:
		return ""
	}
}

func safeMediaExt(filename, mimeType string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	if extRe.MatchString(ext) {
		return ext
	}
	exts, _ := mime.ExtensionsByType(mimeType)
	if len(exts) > 0 && extRe.MatchString(exts[0]) {
		return exts[0]
	}
	return ""
}

func mediaSnippet(kind, fileURL, filename string) string {
	name := cleanMediaName(filename)
	switch kind {
	case "image":
		return fmt.Sprintf("![%s](%s)", escapeMarkdownLabel(name), fileURL)
	case "audio":
		return fmt.Sprintf(`<audio controls="controls" preload="metadata" src="%s" title="%s"></audio>`, template.HTMLEscapeString(fileURL), template.HTMLEscapeString(name))
	case "video":
		return fmt.Sprintf(`<video controls="controls" preload="metadata" src="%s" title="%s"></video>`, template.HTMLEscapeString(fileURL), template.HTMLEscapeString(name))
	default:
		return fmt.Sprintf("[%s](%s)", escapeMarkdownLabel(name), fileURL)
	}
}

func cleanMediaName(filename string) string {
	name := strings.TrimSpace(filepath.Base(filename))
	if name == "." || name == string(filepath.Separator) || name == "" {
		return "media"
	}
	return name
}

func escapeMarkdownLabel(s string) string {
	return strings.NewReplacer(`\`, `\\`, `[`, `\[`, `]`, `\]`).Replace(s)
}

func (s *Server) withUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("dh_session")
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		parts := strings.SplitN(c.Value, ".", 2)
		if len(parts) != 2 {
			next.ServeHTTP(w, r)
			return
		}
		var u User
		var stored, exp string
		err = s.db.QueryRowContext(r.Context(), `SELECT u.id,u.username,u.display_name,u.role,s.token_hash,s.expires_at FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.id=? AND u.is_active=1`, parts[0]).Scan(&u.ID, &u.Username, &u.DisplayName, &u.Role, &stored, &exp)
		if err == nil && stored == hashToken(parts[1], s.cfg.SessionSecret) && exp > time.Now().UTC().Format(time.RFC3339) {
			r = r.WithContext(context.WithValue(r.Context(), userKey{}, &u))
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requireLogin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if userFrom(r.Context()) == nil {
			http.Redirect(w, r, "/login", 303)
			return
		}
		next(w, r)
	}
}
func (s *Server) requireEditor(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := userFrom(r.Context())
		if u == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if !s.canWrite(u) {
			http.Error(w, "forbidden", 403)
			return
		}
		next(w, r)
	}
}
func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := userFrom(r.Context())
		if u == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if u.Role != "admin" {
			http.Error(w, "forbidden", 403)
			return
		}
		next(w, r)
	}
}

type userKey struct{}

func userFrom(ctx context.Context) *User { u, _ := ctx.Value(userKey{}).(*User); return u }
func slugParam(r *http.Request) string {
	raw := chi.URLParam(r, "slug")
	if decoded, err := url.PathUnescape(raw); err == nil {
		return decoded
	}
	return raw
}
func randomID(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
func hashToken(token, secret string) string {
	h := sha256.Sum256([]byte(secret + ":" + token))
	return hex.EncodeToString(h[:])
}
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// rateLimiter returns a simple token-bucket rate limiting middleware.
// Uses in-memory per-IP tracking; for multi-instance deployments, replace
// with Redis-backed rate limiter.
func (s *Server) rateLimiter() func(http.Handler) http.Handler {
	type bucket struct {
		tokens   float64
		lastSeen time.Time
	}
	var (
		mu      sync.Mutex
		buckets = map[string]*bucket{}
	)
	rate := float64(s.cfg.RateLimit.RequestsPerMin) / 60.0 // tokens per second
	burst := float64(s.cfg.RateLimit.Burst)

	// Cleanup goroutine
	go func() {
		for {
			time.Sleep(5 * time.Minute)
			mu.Lock()
			now := time.Now()
			for ip, b := range buckets {
				if now.Sub(b.lastSeen) > 10*time.Minute {
					delete(buckets, ip)
				}
			}
			mu.Unlock()
		}
	}()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r)
			mu.Lock()
			b, ok := buckets[ip]
			if !ok {
				b = &bucket{tokens: burst, lastSeen: time.Now()}
				buckets[ip] = b
			}
			now := time.Now()
			elapsed := now.Sub(b.lastSeen).Seconds()
			b.tokens = min(burst, b.tokens+elapsed*rate)
			b.lastSeen = now
			if b.tokens < 1 {
				mu.Unlock()
				w.Header().Set("Retry-After", "60")
				http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
				return
			}
			b.tokens--
			mu.Unlock()
			next.ServeHTTP(w, r)
		})
	}
}
