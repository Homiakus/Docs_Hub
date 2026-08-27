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
	"github.com/homiakus/docshub-next/internal/application"
	"github.com/homiakus/docshub-next/internal/auth"
	"github.com/homiakus/docshub-next/internal/authz"
	"github.com/homiakus/docshub-next/internal/config"
	"github.com/homiakus/docshub-next/internal/db"
	"github.com/homiakus/docshub-next/internal/domain"
	"github.com/homiakus/docshub-next/internal/markdownx"
	"github.com/homiakus/docshub-next/internal/repository/sqlite"
	"github.com/homiakus/docshub-next/internal/web"
)

type Server struct {
	cfg                       config.Config
	db                        *db.DB
	tpl                       *template.Template
	log                       *slog.Logger
	domainProjectService      *application.DomainProjectService
	domainProjectQueryService *application.DomainProjectQueryService
	authorizer                authz.Authorizer
}

type User struct {
	ID          int64
	Username    string
	DisplayName string
	Role        string
}

type Article struct {
	ID             int64
	OrganizationID int64
	SpaceID        int64
	SpaceName      string
	SpaceSlug      string
	Slug           string
	Title          string
	Status         string
	Classification string
	Language       string
	LockVersion    int
	Content        string
	HTML           template.HTML
	Visibility     string
	CategoryID     int64
	Category       string
	UpdatedAt      string
	HasMermaid     bool
	Headings       []markdownx.Heading
	Tags           []string
	OwnerID        int64
}

type WorkflowAction struct {
	Action string
	Label  string
	Style  string
}

type ArticleFilter struct {
	Query     string
	SpaceSlug string
	Status    string
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

type Space struct {
	ID          int64
	Name        string
	Slug        string
	Description string
	Count       int
}

type Page struct {
	SiteName        string
	Title           string
	User            *User
	Query           string
	Error           string
	Notice          string
	Spaces          []Space
	CurrentSpace    Space
	Drafts          []Article
	ReviewQueue     []Article
	RecentDocs      []Article
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
	CanRead         bool
	CanWrite        bool
	Stats           string
	UserCount       int
	ArticleCount    int
	CategoryCount   int
	FileCount       int
	Activities      []ActivityItem
	Templates       []application.DocumentTemplate
	TemplateID      string
	SearchSpace     string
	SearchStatus    string
	WorkflowActions []WorkflowAction
	Domains         []domain.Domain
	CurrentDomain   *domain.Domain
	Projects        []domain.Project
	CurrentProject  *domain.Project
	PDFKey          string
	PDFURL          string
	PDFName         string
	PDFPageCount    int
	CurrentPath     string
	IsEditor        bool
	CSRFToken       string
}

func New(cfg config.Config, d *db.DB, logger *slog.Logger) (*Server, error) {
	s := &Server{cfg: cfg, db: d, log: logger}
	if d != nil {
		domainRepo := sqlite.NewDomainRepository(d)
		projectRepo := sqlite.NewProjectRepository(d)
		secAdapter := authz.NewSecurityAdapter(d)
		s.domainProjectService = application.NewDomainProjectService(domainRepo, projectRepo, secAdapter)
		s.domainProjectQueryService = application.NewDomainProjectQueryService(domainRepo, domainRepo, projectRepo, secAdapter)
		s.authorizer = secAdapter
	}
	if err := s.bootstrap(context.Background()); err != nil {
		return nil, err
	}
	// Parse base template once at startup (page-specific templates are parsed per-request
	// because they all define the same "content" template)
	funcs := template.FuncMap{
		"eq": func(a, b any) bool { return fmt.Sprint(a) == fmt.Sprint(b) },
		"articlePath": func(slug string) template.URL {
			return template.URL("/a/" + url.PathEscape(slug))
		},
		"editPath": func(slug string) template.URL {
			return template.URL("/edit/" + url.PathEscape(slug))
		},
		"tagPath": func(tag string) template.URL {
			return template.URL("/search?q=" + url.QueryEscape("#"+tag))
		},
		"statusLabel": statusLabel,
		"initial":     firstRune,
	}
	tpl, err := template.New("base.html").Funcs(funcs).ParseFS(web.FS, "templates/base.html", "templates/components/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse base template: %w", err)
	}
	s.tpl = tpl
	return s, nil
}

func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer, s.securityHeaders, s.withUser)
	// CSRF protection for all state-changing requests (POST/PUT/DELETE)
	r.Use(s.csrfMiddleware)
	if s.cfg.RateLimit.Enabled {
		r.Use(s.rateLimiter())
	}
	r.With(staticCacheHeaders).Handle("/static/*", http.FileServerFS(web.FS))
	r.Get("/healthz", s.health)
	r.Get("/login", s.loginForm)
	r.With(s.loginRateLimiter()).Post("/login", s.login)
	r.Post("/logout", s.logout)
	r.Get("/", s.requireLogin(s.home))
	r.Get("/search", s.requireLogin(s.searchPage))
	r.Get("/api/v1/search/suggest", s.requireLogin(s.searchSuggestAPI))
	r.Get("/api/v1/domains", s.requireLogin(s.apiListDomains))
	r.Post("/api/v1/domains", s.requireEditor(s.apiCreateDomain))
	r.Get("/api/v1/domains/{id}/projects", s.requireLogin(s.apiListProjects))
	r.Post("/api/v1/projects", s.requireEditor(s.apiCreateProject))
	r.Get("/domains", s.requireLogin(s.domainsPage))
	r.Get("/domains/{slug}", s.requireLogin(s.showDomainPage))
	r.Get("/spaces", s.requireLogin(s.spacesPage))
	r.Get("/spaces/{slug}", s.requireLogin(s.showSpacePage))
	r.Get("/a/{slug}", s.requireLogin(s.article))
	r.Get("/new", s.requireEditor(s.editNew))
	r.Get("/edit/{slug}", s.requireEditor(s.editExisting))
	r.Post("/save", s.requireEditor(s.saveArticle))
	r.Put("/api/v1/documents/draft", s.requireEditor(s.saveDraftAPI))
	r.Post("/documents/{id}/workflow", s.requireEditor(s.transitionWorkflow))
	r.Post("/api/preview", s.requireLogin(s.preview))
	r.Post("/api/uploads", s.requireEditor(s.uploadFile))
	r.Get("/api/graph", s.requireLogin(s.graphAPI))
	r.Get("/uploads/{key}", s.requireLogin(s.serveUpload))
	r.Get("/graph", s.requireLogin(s.graphPage))
	r.Get("/pdf/viewer", s.requireLogin(s.pdfViewerPage))
	r.Get("/pdf/viewer/{key}", s.requireLogin(s.pdfViewerPage))
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
	p.CurrentPath = r.URL.Path
	p.CSRFToken = csrfFrom(r.Context())
	if p.Categories == nil {
		p.Categories, _ = s.listCategories(r.Context(), p.User)
	}
	if p.Spaces == nil {
		p.Spaces, _ = s.listSpaces(r.Context(), p.User)
	}
	if p.Activities == nil {
		p.Activities, _ = s.listRecentActivity(r.Context(), p.User)
	}
	tpl, err := s.tpl.Clone()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if _, err := tpl.ParseFS(web.FS, "templates/"+name+".html"); err != nil {
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
	u := userFrom(r.Context())
	arts, err := s.listArticles(r.Context(), u, q)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	var drafts, reviewQueue, recent []Article
	for _, a := range arts {
		if a.OwnerID == u.ID && (a.Status == "draft" || a.Status == "") {
			drafts = append(drafts, a)
		} else if a.Status == "in_review" {
			reviewQueue = append(reviewQueue, a)
		} else {
			recent = append(recent, a)
		}
	}
	drafts = limitArticles(drafts, 4)
	reviewQueue = limitArticles(reviewQueue, 5)
	recent = limitArticles(recent, 8)

	s.render(w, r, "home", Page{
		Title:       "Обзор",
		Query:       q,
		Articles:    arts,
		RecentDocs:  recent,
		Drafts:      drafts,
		ReviewQueue: reviewQueue,
	})
}

func (s *Server) searchPage(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	spaceSlug := strings.TrimSpace(r.URL.Query().Get("space"))
	status := validSearchStatus(r.URL.Query().Get("status"))
	arts, err := s.listArticlesFiltered(r.Context(), userFrom(r.Context()), ArticleFilter{
		Query: q, SpaceSlug: spaceSlug, Status: status,
	})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.render(w, r, "search", Page{
		Title: "Поиск", Query: q, Articles: arts,
		SearchSpace: spaceSlug, SearchStatus: status,
	})
}

func (s *Server) searchSuggestAPI(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	arts, err := s.listArticles(r.Context(), userFrom(r.Context()), q)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	type item struct {
		Title string `json:"title"`
		Slug  string `json:"slug"`
	}
	var results []item
	for _, a := range arts {
		results = append(results, item{Title: a.Title, Slug: a.Slug})
		if len(results) >= 8 {
			break
		}
	}
	w.Header().Set("content-type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"suggestions": results})
}

func (s *Server) saveDraftAPI(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID             int64  `json:"id"`
		Title          string `json:"title"`
		Slug           string `json:"slug"`
		Content        string `json:"content"`
		Visibility     string `json:"visibility"`
		Classification string `json:"classification"`
		Language       string `json:"language"`
		SpaceID        int64  `json:"space_id"`
		CategoryID     int64  `json:"category_id"`
		LockVersion    int    `json:"lock_version"`
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 3<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "Некорректные данные черновика")
		return
	}
	u := userFrom(r.Context())
	if u == nil {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized", "Требуется вход")
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		req.Title = "Без названия"
	}
	req.Visibility = validVisibility(req.Visibility)
	req.Classification = validClassification(req.Classification)
	req.Language = validLanguage(req.Language)
	if req.SpaceID == 0 {
		req.SpaceID = 1
	}
	if !s.spaceExists(r.Context(), req.SpaceID) {
		writeJSONError(w, http.StatusBadRequest, "invalid_space", "Пространство не найдено")
		return
	}
	categoryName, categorySlug, err := s.categoryMeta(r.Context(), req.CategoryID)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_category", err.Error())
		return
	}
	rendered, err := markdownx.Render(req.Content)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_markdown", err.Error())
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	var slug string
	var currentLock int
	if req.ID != 0 {
		var ownerID int64
		var currentVisibility string
		if err := s.db.QueryRowContext(r.Context(), `
			SELECT slug,coalesce(owner_id,0),visibility,coalesce(lock_version,1)
			FROM articles WHERE id=? AND deleted_at IS NULL`, req.ID).
			Scan(&slug, &ownerID, &currentVisibility, &currentLock); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSONError(w, http.StatusNotFound, "not_found", "Документ не найден")
			} else {
				writeJSONError(w, http.StatusInternalServerError, "database_error", "Не удалось прочитать документ")
			}
			return
		}
		if !s.canEditDocument(r.Context(), u, req.ID, ownerID, currentVisibility) {
			writeJSONError(w, http.StatusForbidden, "forbidden", "Недостаточно прав для изменения документа")
			return
		}
		if req.LockVersion == 0 {
			req.LockVersion = currentLock
		}
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "database_error", "Не удалось начать сохранение")
		return
	}
	defer tx.Rollback()

	var nextLock int
	if req.ID == 0 {
		slug, err = s.uniqueSlug(r.Context(), tx, 0, firstNonEmpty(req.Slug, req.Title, "draft"))
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "slug_error", "Не удалось создать адрес документа")
			return
		}
		result, execErr := tx.ExecContext(r.Context(), `
			INSERT INTO articles(
				organization_id,space_id,slug,title,status,classification,language,lock_version,
				content,rendered_html,owner_id,visibility,category_id,created_at,updated_at
			) VALUES(1,?,?,?,'draft',?,?,1,?,?,?,?,?,?,?)`,
			req.SpaceID, slug, req.Title, req.Classification, req.Language,
			req.Content, rendered.HTML, u.ID, req.Visibility, nullableID(req.CategoryID), now, now)
		if execErr != nil {
			writeJSONError(w, http.StatusInternalServerError, "save_failed", "Не удалось создать черновик")
			return
		}
		req.ID, err = result.LastInsertId()
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "save_failed", "Не удалось определить черновик")
			return
		}
		nextLock = 1
	} else {
		result, execErr := tx.ExecContext(r.Context(), `
			UPDATE articles SET title=?,content=?,rendered_html=?,visibility=?,classification=?,language=?,
				space_id=?,category_id=?,lock_version=lock_version+1,updated_at=?
			WHERE id=? AND deleted_at IS NULL AND lock_version=?`,
			req.Title, req.Content, rendered.HTML, req.Visibility, req.Classification, req.Language,
			req.SpaceID, nullableID(req.CategoryID), now, req.ID, req.LockVersion)
		if execErr != nil {
			writeJSONError(w, http.StatusInternalServerError, "save_failed", "Не удалось сохранить черновик")
			return
		}
		affected, _ := result.RowsAffected()
		if affected != 1 {
			writeJSON(w, http.StatusConflict, map[string]any{
				"error": "edit_conflict", "message": "Документ был изменён в другой вкладке",
				"server_lock_version": currentLock,
			})
			return
		}
		nextLock = req.LockVersion + 1
	}

	if err := s.syncArticleDerivedData(r.Context(), tx, req.ID, req.Title, slug, req.Content, categoryName, categorySlug, rendered); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "index_failed", "Не удалось обновить индекс документа")
		return
	}
	metadata, _ := json.Marshal(map[string]any{"slug": slug, "title": req.Title, "autosave": true})
	if _, err := tx.ExecContext(r.Context(), `
		INSERT INTO audit_events(actor_id,action,entity_type,entity_id,ip,metadata_json,created_at)
		VALUES(?,?,?,?,?,?,?)`, u.ID, "article.autosave", "article", fmt.Sprint(req.ID), clientIP(r), string(metadata), now); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "audit_failed", "Не удалось завершить сохранение")
		return
	}
	if err := tx.Commit(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "save_failed", "Не удалось завершить сохранение")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id": req.ID, "slug": slug, "status": "draft_saved", "lock_version": nextLock,
		"saved_at": now,
	})
}

func (s *Server) listSpaces(ctx context.Context, u *User) ([]Space, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT s.id,s.name,s.slug,s.description,coalesce(a.id,0),coalesce(a.status,''),
			coalesce(a.visibility,''),coalesce(a.owner_id,0)
		 FROM spaces s
		 LEFT JOIN articles a ON a.space_id = s.id AND a.deleted_at IS NULL
		 ORDER BY lower(s.name),a.updated_at DESC`)
	if err != nil {
		return nil, err
	}
	type spaceCandidate struct {
		space   Space
		article Article
	}
	var candidates []spaceCandidate
	for rows.Next() {
		var candidate spaceCandidate
		if err := rows.Scan(
			&candidate.space.ID, &candidate.space.Name, &candidate.space.Slug, &candidate.space.Description,
			&candidate.article.ID, &candidate.article.Status, &candidate.article.Visibility, &candidate.article.OwnerID,
		); err != nil {
			rows.Close()
			return nil, err
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	spaceOrder := make([]int64, 0)
	spacesByID := make(map[int64]Space)
	for _, candidate := range candidates {
		sp := candidate.space
		current, exists := spacesByID[sp.ID]
		if !exists {
			current = sp
			spaceOrder = append(spaceOrder, sp.ID)
		}
		if candidate.article.ID > 0 && s.canViewArticle(ctx, u, candidate.article) {
			current.Count++
		}
		spacesByID[sp.ID] = current
	}
	spaces := make([]Space, 0, len(spaceOrder))
	for _, id := range spaceOrder {
		spaces = append(spaces, spacesByID[id])
	}
	return spaces, nil
}

func (s *Server) spacesPage(w http.ResponseWriter, r *http.Request) {
	spaces, err := s.listSpaces(r.Context(), userFrom(r.Context()))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.render(w, r, "spaces/index", Page{Title: "Пространства", Spaces: spaces})
}

func (s *Server) showSpacePage(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	var sp Space
	err := s.db.QueryRowContext(r.Context(), `SELECT id, name, slug, description FROM spaces WHERE slug=?`, slug).Scan(&sp.ID, &sp.Name, &sp.Slug, &sp.Description)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	u := userFrom(r.Context())
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT id,slug,title,coalesce(status,'published'),content,visibility,updated_at,
			coalesce(owner_id,0),coalesce(lock_version,1),coalesce(classification,'internal'),coalesce(language,'ru')
		FROM articles WHERE space_id=? AND deleted_at IS NULL ORDER BY updated_at DESC`, sp.ID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	var candidates []Article
	for rows.Next() {
		var a Article
		if err := rows.Scan(&a.ID, &a.Slug, &a.Title, &a.Status, &a.Content, &a.Visibility, &a.UpdatedAt, &a.OwnerID, &a.LockVersion, &a.Classification, &a.Language); err != nil {
			rows.Close()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		a.SpaceID, a.SpaceName, a.SpaceSlug = sp.ID, sp.Name, sp.Slug
		candidates = append(candidates, a)
	}
	if err := rows.Close(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var articles []Article
	for _, a := range candidates {
		if s.canViewArticle(r.Context(), u, a) {
			articles = append(articles, a)
		}
	}
	sp.Count = len(articles)
	s.render(w, r, "spaces/show", Page{Title: sp.Name, CurrentSpace: sp, Articles: articles})
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
		if _, err := s.db.ExecContext(r.Context(), `DELETE FROM sessions WHERE id=?`, sid); err != nil {
			s.log.Error("logout delete session", "err", err)
		}
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
	u := userFrom(r.Context())
	if !s.canViewArticle(r.Context(), u, a) {
		http.Error(w, "forbidden", 403)
		return
	}
	back, _ := s.backlinks(r.Context(), u, a.Slug)
	wikiLinks, _ := s.articleWikiLinks(r.Context(), u, a.ID, a.Slug)
	versions, _ := s.articleVersions(r.Context(), a.ID)
	s.render(w, r, "article", Page{
		Title: a.Title, Article: a, WikiLinks: wikiLinks, Backlinks: back, Versions: versions,
		CanWrite:        s.canEditDocument(r.Context(), u, a.ID, a.OwnerID, a.Visibility),
		CurrentSpace:    Space{ID: a.SpaceID, Name: a.SpaceName, Slug: a.SpaceSlug},
		WorkflowActions: s.workflowActions(r.Context(), u, a),
		Notice:          r.URL.Query().Get("notice"),
	})
}

func (s *Server) editNew(w http.ResponseWriter, r *http.Request) {
	categories, _ := s.listAdminCategories(r.Context())
	templateService := application.NewTemplateService()
	templateID := strings.TrimSpace(r.URL.Query().Get("template"))
	article := Article{Visibility: "authenticated", Status: "draft", Classification: "internal", Language: "ru", SpaceID: 1, LockVersion: 1}
	if requestedSpace, _ := strconv.ParseInt(r.URL.Query().Get("space"), 10, 64); requestedSpace > 0 && s.spaceExists(r.Context(), requestedSpace) {
		article.SpaceID = requestedSpace
	}
	if selected := templateService.GetTemplateByID(templateID); selected != nil {
		article.Content = selected.Content
	}
	s.render(w, r, "edit", Page{
		Title: "Новый документ", Article: article, AdminCategories: categories,
		Templates: templateService.ListTemplates(), TemplateID: templateID, IsEditor: true,
	})
}

func (s *Server) editExisting(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	a, err := s.getArticle(r.Context(), slugParam(r))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !s.canEditDocument(r.Context(), u, a.ID, a.OwnerID, a.Visibility) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	categories, _ := s.listAdminCategories(r.Context())
	s.render(w, r, "edit", Page{Title: "Редактирование", Article: a, AdminCategories: categories, IsEditor: true})
}

func (s *Server) saveArticle(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	u := userFrom(r.Context())
	id, _ := strconv.ParseInt(r.Form.Get("id"), 10, 64)
	lockVersion, _ := strconv.Atoi(r.Form.Get("lock_version"))
	var currentLock int
	if id != 0 {
		var ownerID int64
		var currentVis string
		err := s.db.QueryRowContext(r.Context(), `SELECT coalesce(owner_id,0),visibility,coalesce(lock_version,1) FROM articles WHERE id=? AND deleted_at IS NULL`, id).Scan(&ownerID, &currentVis, &currentLock)
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if !s.canEditDocument(r.Context(), u, id, ownerID, currentVis) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if lockVersion == 0 {
			lockVersion = currentLock
		}
	} else {
		if !s.canEditDocument(r.Context(), u, 0, u.ID, "authenticated") {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
	}
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
	visibility := validVisibility(r.Form.Get("visibility"))
	classification := validClassification(r.Form.Get("classification"))
	language := validLanguage(r.Form.Get("language"))
	spaceID, _ := strconv.ParseInt(r.Form.Get("space_id"), 10, 64)
	if spaceID == 0 {
		spaceID = 1
	}
	if !s.spaceExists(r.Context(), spaceID) {
		http.Error(w, "пространство не найдено", http.StatusBadRequest)
		return
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
		row, err := tx.ExecContext(r.Context(), `
			INSERT INTO articles(
				organization_id,space_id,slug,title,status,classification,language,lock_version,
				content,rendered_html,owner_id,visibility,created_at,updated_at,category_id
			) VALUES(1,?,?,?,'draft',?,?,1,?,?,?,?,?,?,?)`,
			spaceID, slug, title, classification, language, content, res.HTML, u.ID, visibility, now, now, nullableID(categoryID))
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		id, err = row.LastInsertId()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	} else {
		result, updateErr := tx.ExecContext(r.Context(), `
			UPDATE articles SET slug=?,title=?,content=?,rendered_html=?,visibility=?,classification=?,language=?,
				space_id=?,updated_at=?,category_id=?,lock_version=lock_version+1
			WHERE id=? AND deleted_at IS NULL AND lock_version=?`,
			slug, title, content, res.HTML, visibility, classification, language,
			spaceID, now, nullableID(categoryID), id, lockVersion)
		err = updateErr
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if affected, _ := result.RowsAffected(); affected != 1 {
			http.Error(w, "Документ был изменён в другой вкладке. Обновите страницу перед повторным сохранением.", http.StatusConflict)
			return
		}
	}
	var versionNo int
	if err := tx.QueryRowContext(r.Context(), `SELECT coalesce(max(version_no),0)+1 FROM article_versions WHERE article_id=?`, id).Scan(&versionNo); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if _, err := tx.ExecContext(r.Context(), `INSERT INTO article_versions(article_id, version_no, title, content, rendered_html, author_id, created_at) VALUES(?,?,?,?,?,?,?)`, id, versionNo, title, content, res.HTML, u.ID, now); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if err := s.syncArticleDerivedData(r.Context(), tx, id, title, slug, content, categoryName, categorySlug, res); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	current := articleSnapshot{Slug: slug, Title: title, Content: content, Visibility: visibility, CategoryID: categoryID}
	summary := summarizeArticleChange(previous, current, hasPrevious)
	metadata, err := json.Marshal(map[string]any{
		"version": versionNo,
		"summary": summary,
		"slug":    slug,
		"title":   title,
	})
	if err != nil {
		s.log.Error("saveArticle marshal metadata", "err", err)
		metadata = []byte("{}")
	}
	if _, err := tx.ExecContext(r.Context(), `INSERT INTO audit_events(actor_id, action, entity_type, entity_id, ip, metadata_json, created_at) VALUES(?,?,?,?,?,?,?)`, u.ID, "article.save", "article", fmt.Sprint(id), clientIP(r), string(metadata), now); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	http.Redirect(w, r, "/a/"+url.PathEscape(slug), http.StatusSeeOther)
}

func (s *Server) transitionWorkflow(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	documentID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || documentID < 1 {
		http.NotFound(w, r)
		return
	}
	u := userFrom(r.Context())
	var article Article
	if err := s.db.QueryRowContext(r.Context(), `
		SELECT id,slug,title,coalesce(status,'published'),visibility,coalesce(owner_id,0),coalesce(lock_version,1)
		FROM articles WHERE id=? AND deleted_at IS NULL`, documentID).Scan(
		&article.ID, &article.Slug, &article.Title, &article.Status, &article.Visibility, &article.OwnerID, &article.LockVersion,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	action := strings.TrimSpace(r.Form.Get("action"))
	nextStatus, allowed := s.workflowTransitionAllowed(r.Context(), u, article, action)
	if !allowed {
		http.Error(w, "Недопустимый переход статуса или недостаточно прав", http.StatusUnprocessableEntity)
		return
	}
	expectedLock, _ := strconv.Atoi(r.Form.Get("lock_version"))
	if expectedLock == 0 {
		expectedLock = article.LockVersion
	}
	now := time.Now().UTC().Format(time.RFC3339)
	var archivedAt any
	if nextStatus == "archived" {
		archivedAt = now
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(r.Context(), `
		UPDATE articles SET status=?,lock_version=lock_version+1,updated_at=?,archived_at=?
		WHERE id=? AND deleted_at IS NULL AND lock_version=?`, nextStatus, now, archivedAt, article.ID, expectedLock)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		http.Error(w, "Документ уже изменён. Обновите страницу и повторите действие.", http.StatusConflict)
		return
	}
	metadata, _ := json.Marshal(map[string]any{"from": article.Status, "to": nextStatus, "action": action})
	if _, err := tx.ExecContext(r.Context(), `
		INSERT INTO audit_events(actor_id,action,entity_type,entity_id,ip,metadata_json,created_at)
		VALUES(?,?,?,?,?,?,?)`, u.ID, "article.workflow", "article", fmt.Sprint(article.ID), clientIP(r), string(metadata), now); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	notice := "Статус изменён: " + statusLabel(nextStatus)
	http.Redirect(w, r, "/a/"+url.PathEscape(article.Slug)+"?notice="+url.QueryEscape(notice), http.StatusSeeOther)
}

func (s *Server) workflowActions(ctx context.Context, u *User, article Article) []WorkflowAction {
	var actions []WorkflowAction
	add := func(action, label, style string) {
		if _, allowed := s.workflowTransitionAllowed(ctx, u, article, action); allowed {
			actions = append(actions, WorkflowAction{Action: action, Label: label, Style: style})
		}
	}
	switch article.Status {
	case "draft", "rejected", "":
		add("submit_review", "Отправить на проверку", "primary")
	case "in_review":
		add("approve", "Одобрить", "primary")
		add("return_draft", "Вернуть в черновики", "secondary")
	case "approved":
		add("publish", "Опубликовать", "primary")
		add("return_draft", "Вернуть в черновики", "secondary")
	case "published":
		add("archive", "В архив", "secondary")
	case "archived":
		add("reopen", "Вернуть в черновики", "primary")
	}
	return actions
}

func (s *Server) workflowTransitionAllowed(ctx context.Context, u *User, article Article, action string) (string, bool) {
	if u == nil {
		return "", false
	}
	canEdit := s.canEditDocument(ctx, u, article.ID, article.OwnerID, article.Visibility)
	switch action {
	case "submit_review":
		return "in_review", canEdit && (article.Status == "draft" || article.Status == "rejected" || article.Status == "")
	case "approve":
		return "approved", article.Status == "in_review" && (u.Role == "admin" || (u.Role == "editor" && s.canViewArticle(ctx, u, article)))
	case "return_draft":
		return "draft", (article.Status == "in_review" || article.Status == "approved") && (u.Role == "admin" || canEdit)
	case "publish":
		return "published", article.Status == "approved" && u.Role == "admin"
	case "archive":
		return "archived", article.Status == "published" && u.Role == "admin"
	case "reopen":
		return "draft", article.Status == "archived" && u.Role == "admin"
	default:
		return "", false
	}
}

func (s *Server) preview(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	res, err := markdownx.Render(string(body))
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	w.Header().Set("content-type", "text/html; charset=utf-8")
	if _, err := w.Write([]byte(res.HTML)); err != nil {
		s.log.Error("preview write", "err", err)
	}
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
		http.Error(w, "поддерживаются изображения, аудио, видео и PDF", http.StatusUnsupportedMediaType)
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
	pageCount := 0
	if kind == "pdf" {
		pageCount = pdfPageCount(data)
		if _, err := s.db.ExecContext(r.Context(), `UPDATE files SET page_count=? WHERE sha256=?`, pageCount, sha); err != nil {
			s.log.Error("uploadFile update PDF page count", "err", err)
		}
	}

	fileURL := "/uploads/" + url.PathEscape(storageKey)
	viewerURL := ""
	snippetURL := fileURL
	if kind == "pdf" {
		viewerURL = "/pdf/viewer/" + url.PathEscape(storageKey)
		snippetURL = viewerURL
	}
	w.Header().Set("content-type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"kind":       kind,
		"url":        fileURL,
		"viewer_url": viewerURL,
		"filename":   header.Filename,
		"mime":       mimeType,
		"page_count": pageCount,
		"markdown":   mediaSnippet(kind, snippetURL, header.Filename),
	}); err != nil {
		s.log.Error("uploadFile encode", "err", err)
	}
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
	if !s.canAccessFile(r.Context(), u, fileID, uploadedBy) {
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

func (s *Server) pdfViewerPage(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if key == "" {
		key = r.URL.Query().Get("key")
	}
	if decoded, err := url.PathUnescape(key); err == nil {
		key = decoded
	}
	if !validStorageKey(key) {
		http.NotFound(w, r)
		return
	}
	var fileID, uploadedBy int64
	var mimeType, originalName string
	var pageCount int
	if err := s.db.QueryRowContext(r.Context(), `
		SELECT id,mime,original_name,coalesce(uploaded_by,0),coalesce(page_count,0)
		FROM files WHERE storage_key=?`, key).Scan(&fileID, &mimeType, &originalName, &uploadedBy, &pageCount); err != nil {
		http.NotFound(w, r)
		return
	}
	if mimeType != "application/pdf" || !s.canAccessFile(r.Context(), userFrom(r.Context()), fileID, uploadedBy) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if pageCount < 1 {
		pageCount = 1
	}
	s.render(w, r, "pdf/viewer", Page{
		Title: "PDF · " + originalName, PDFKey: key,
		PDFURL: "/uploads/" + url.PathEscape(key), PDFName: originalName, PDFPageCount: pageCount,
	})
}

func (s *Server) graphAPI(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT a.id,a.slug,a.title,coalesce(a.status,'published'),a.visibility,coalesce(a.owner_id,0),
			coalesce(s.name,''),coalesce(s.slug,'')
		FROM articles a LEFT JOIN spaces s ON s.id=a.space_id
		WHERE a.deleted_at IS NULL ORDER BY lower(a.title)`)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	type candidate struct {
		id, ownerID                                int64
		slug, title, status, vis, space, spaceSlug string
	}
	var candidates []candidate
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.id, &item.slug, &item.title, &item.status, &item.vis, &item.ownerID, &item.space, &item.spaceSlug); err == nil {
			candidates = append(candidates, item)
		}
	}
	rows.Close()

	var nodes []map[string]string
	accessibleSlugs := make(map[string]bool)
	for _, item := range candidates {
		if s.canViewArticle(r.Context(), u, Article{ID: item.id, Status: item.status, Visibility: item.vis, OwnerID: item.ownerID}) {
			nodes = append(nodes, map[string]string{
				"id": item.slug, "label": item.title, "status": item.status,
				"space": item.space, "space_slug": item.spaceSlug,
			})
			accessibleSlugs[item.slug] = true
		}
	}
	lr, err := s.db.QueryContext(r.Context(), `SELECT a.slug,l.target_slug,coalesce(l.label,'') FROM links l JOIN articles a ON a.id=l.from_article_id WHERE a.deleted_at IS NULL`)
	if err != nil {
		s.log.Error("graphAPI query links", "err", err)
	} else {
		defer lr.Close()
		var links []map[string]string
		for lr.Next() {
			var a, b, label string
			if err := lr.Scan(&a, &b, &label); err != nil {
				s.log.Error("graphAPI scan link", "err", err)
				continue
			}
			if accessibleSlugs[a] && accessibleSlugs[b] {
				links = append(links, map[string]string{"source": a, "target": b, "label": label})
			}
		}
		w.Header().Set("content-type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"nodes": nodes, "links": links}); err != nil {
			s.log.Error("graphAPI encode", "err", err)
		}
		return
	}
	w.Header().Set("content-type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"nodes": nodes, "links": []map[string]string{}}); err != nil {
		s.log.Error("graphAPI encode", "err", err)
	}
}

func (s *Server) admin(w http.ResponseWriter, r *http.Request) {
	var users, articles, categories, files int
	if err := s.db.QueryRowContext(r.Context(), `SELECT count(*) FROM users`).Scan(&users); err != nil {
		s.log.Error("admin stats users", "err", err)
	}
	if err := s.db.QueryRowContext(r.Context(), `SELECT count(*) FROM articles WHERE deleted_at IS NULL`).Scan(&articles); err != nil {
		s.log.Error("admin stats articles", "err", err)
	}
	if err := s.db.QueryRowContext(r.Context(), `SELECT count(*) FROM categories`).Scan(&categories); err != nil {
		s.log.Error("admin stats categories", "err", err)
	}
	if err := s.db.QueryRowContext(r.Context(), `SELECT count(*) FROM files`).Scan(&files); err != nil {
		s.log.Error("admin stats files", "err", err)
	}
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
		UserCount:       users,
		ArticleCount:    articles,
		CategoryCount:   categories,
		FileCount:       files,
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
		if _, err := s.db.ExecContext(r.Context(), `DELETE FROM sessions WHERE user_id=?`, id); err != nil {
			s.log.Error("adminSaveUser delete sessions", "err", err, "userID", id)
		}
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
	if _, err := s.db.ExecContext(r.Context(), `DELETE FROM sessions WHERE user_id=?`, id); err != nil {
		s.log.Error("adminSetPassword delete sessions", "err", err, "userID", id)
	}
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
		if _, err := s.db.ExecContext(r.Context(), `DELETE FROM article_fts WHERE rowid=?`, id); err != nil {
			s.log.Error("adminSaveArticleSettings delete fts", "err", err, "articleID", id)
		}
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
	if _, err := s.db.ExecContext(r.Context(), `DELETE FROM acl_users WHERE article_id=? AND user_id=?`, articleID, userID); err != nil {
		s.log.Error("adminSaveAccess delete acl", "err", err, "articleID", articleID, "userID", userID)
	}
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
		if _, err := tx.ExecContext(r.Context(), `UPDATE articles SET category_id=NULL WHERE category_id=?`, id); err != nil {
			s.log.Error("adminSaveCategory unlink articles", "err", err, "categoryID", id)
		}
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

	// Upload all attachments, build filename→viewer map.
	type importedAttachment struct {
		URL  string
		Kind string
	}
	attachMap := make(map[string]importedAttachment)
	for _, a := range attachments {
		mimeType := detectMediaMIME(a.Name, "", a.Content)
		if mimeType == "" {
			continue
		}
		kind := mediaKind(mimeType)
		if kind == "" {
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
		if _, err := s.db.ExecContext(r.Context(), `INSERT OR IGNORE INTO files(sha256, storage_key, original_name, mime, size_bytes, uploaded_by, created_at) VALUES(?,?,?,?,?,?,?)`,
			sha, storageKey, filepath.Base(a.Name), mimeType, len(a.Content), u.ID, now); err != nil {
			s.log.Error("obsidian import insert file", "err", err, "name", a.Name)
		}
		attachmentURL := "/uploads/" + url.PathEscape(storageKey)
		if kind == "pdf" {
			attachmentURL = "/pdf/viewer/" + url.PathEscape(storageKey)
			if _, err := s.db.ExecContext(r.Context(), `UPDATE files SET page_count=? WHERE sha256=?`, pdfPageCount(a.Content), sha); err != nil {
				s.log.Error("obsidian import update PDF page count", "err", err, "name", a.Name)
			}
		}
		attachMap[filepath.Base(a.Name)] = importedAttachment{URL: attachmentURL, Kind: kind}
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
			if attachment, ok := attachMap[filename]; ok {
				if attachment.Kind == "image" && width != "" {
					return fmt.Sprintf(`<img src="%s" width="%s" alt="%s">`, attachment.URL, width, filename)
				}
				return mediaSnippet(attachment.Kind, attachment.URL, filename)
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

		result, err := tx.ExecContext(r.Context(), `
			INSERT INTO articles(
				organization_id,space_id,slug,title,status,classification,language,lock_version,
				content,rendered_html,owner_id,visibility,created_at,updated_at
			) VALUES(1,1,?,?,'draft','internal','ru',1,?,?,?,?,?,?)`,
			slug, title, content, res.HTML, u.ID, "authenticated", now, now)
		if err != nil {
			tx.Rollback()
			s.log.Warn("obsidian import insert", "file", mf.Name, "err", err)
			continue
		}

		articleID, _ := result.LastInsertId()

		// Save version 1
		if _, err := tx.ExecContext(r.Context(),
			`INSERT INTO article_versions(article_id, version_no, title, content, rendered_html, author_id, created_at) VALUES(?,1,?,?,?,?,?)`,
			articleID, title, content, res.HTML, u.ID, now); err != nil {
			s.log.Error("obsidian import insert version", "err", err, "articleID", articleID)
		}

		// Save tags
		for _, tag := range res.Tags {
			if _, err := tx.ExecContext(r.Context(), `INSERT OR IGNORE INTO tags(name) VALUES(?)`, tag); err != nil {
				s.log.Error("obsidian import insert tag", "err", err, "tag", tag)
			}
			if _, err := tx.ExecContext(r.Context(), `INSERT OR IGNORE INTO article_tags(article_id, tag_id) SELECT ?, id FROM tags WHERE name=?`, articleID, tag); err != nil {
				s.log.Error("obsidian import insert article_tag", "err", err, "tag", tag)
			}
		}

		// Save wiki links
		for _, l := range res.Links {
			if _, err := tx.ExecContext(r.Context(), `INSERT OR IGNORE INTO links(from_article_id, target_slug, label) VALUES(?,?,?)`, articleID, l.Slug, l.Label); err != nil {
				s.log.Error("obsidian import insert link", "err", err, "slug", l.Slug)
			}
		}

		// Associate attachments with the article
		attachKeys := extractUploadKeys(content)
		for _, key := range attachKeys {
			var fileID int64
			if err := tx.QueryRowContext(r.Context(), `SELECT id FROM files WHERE storage_key=?`, key).Scan(&fileID); err == nil {
				if _, err := tx.ExecContext(r.Context(), `INSERT OR IGNORE INTO article_files(article_id, file_id, role) VALUES(?,?,?)`, articleID, fileID, "inline"); err != nil {
					s.log.Error("obsidian import insert article_file", "err", err, "key", key)
				}
			}
		}

		// FTS index
		tags := articleSearchTags(res.Tags, "", "")
		if _, err := tx.ExecContext(r.Context(), `DELETE FROM article_fts WHERE rowid=?`, articleID); err != nil {
			s.log.Error("obsidian import delete fts", "err", err, "articleID", articleID)
		}
		if _, err := tx.ExecContext(r.Context(), `INSERT INTO article_fts(rowid,title,slug,content,tags) VALUES(?,?,?,?,?)`, articleID, title, slug, content, strings.Join(tags, " ")); err != nil {
			s.log.Error("obsidian import insert fts", "err", err, "articleID", articleID)
		}

		// Audit
		metadata, err := json.Marshal(map[string]any{
			"version": 1,
			"summary": "Импортировано из Obsidian vault: " + mf.Name,
			"slug":    slug,
			"title":   title,
		})
		if err != nil {
			s.log.Error("obsidian import marshal metadata", "err", err)
			metadata = []byte("{}")
		}
		if _, err := tx.ExecContext(r.Context(), `INSERT INTO audit_events(actor_id, action, entity_type, entity_id, ip, metadata_json, created_at) VALUES(?,?,?,?,?,?,?)`,
			u.ID, "obsidian.import", "article", fmt.Sprint(articleID), clientIP(r), string(metadata), now); err != nil {
			s.log.Error("obsidian import insert audit", "err", err, "articleID", articleID)
		}

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
	if err := json.NewEncoder(w).Encode(map[string]any{
		"status": status,
		"app":    "docshub-next",
		"time":   time.Now().UTC(),
		"db":     dbOK,
	}); err != nil {
		s.log.Error("health encode", "err", err)
	}
}

func (s *Server) listArticles(ctx context.Context, u *User, q string) ([]Article, error) {
	return s.listArticlesFiltered(ctx, u, ArticleFilter{Query: q})
}

func (s *Server) listArticlesFiltered(ctx context.Context, u *User, filter ArticleFilter) ([]Article, error) {
	filter.Query = strings.TrimSpace(filter.Query)
	filter.SpaceSlug = strings.TrimSpace(filter.SpaceSlug)
	filter.Status = validSearchStatus(filter.Status)

	selectSQL := `SELECT DISTINCT
		a.id,coalesce(a.organization_id,1),coalesce(a.space_id,1),coalesce(s.name,''),coalesce(s.slug,''),
		a.slug,a.title,coalesce(a.status,'published'),a.content,a.updated_at,a.visibility,
		coalesce(a.category_id,0),coalesce(c.name,''),coalesce(a.owner_id,0),
		coalesce(a.lock_version,1),coalesce(a.classification,'internal'),coalesce(a.language,'ru')`
	fromSQL := ` FROM articles a LEFT JOIN spaces s ON s.id=a.space_id LEFT JOIN categories c ON c.id=a.category_id`
	where := []string{"a.deleted_at IS NULL"}
	args := make([]any, 0, 8)
	orderBy := " ORDER BY a.updated_at DESC"

	if strings.HasPrefix(filter.Query, "#") {
		fromSQL += ` LEFT JOIN article_tags at ON at.article_id=a.id LEFT JOIN tags t ON t.id=at.tag_id`
		needle := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(filter.Query, "#")))
		where = append(where, `(lower(c.slug)=? OR lower(c.name)=? OR lower(t.name)=?)`)
		args = append(args, needle, needle, needle)
	} else if filter.Query != "" {
		fromSQL = ` FROM article_fts f JOIN articles a ON a.id=f.rowid LEFT JOIN spaces s ON s.id=a.space_id LEFT JOIN categories c ON c.id=a.category_id`
		where = append(where, `article_fts MATCH ?`)
		args = append(args, ftsPrefixQuery(filter.Query))
		orderBy = " ORDER BY rank,a.updated_at DESC"
	}
	if filter.SpaceSlug != "" {
		where = append(where, `s.slug=?`)
		args = append(args, filter.SpaceSlug)
	}
	if filter.Status != "" {
		where = append(where, `coalesce(a.status,'published')=?`)
		args = append(args, filter.Status)
	}

	query := selectSQL + fromSQL + " WHERE " + strings.Join(where, " AND ") + orderBy + " LIMIT 200"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	var candidates []Article
	for rows.Next() {
		var a Article
		if err := rows.Scan(
			&a.ID, &a.OrganizationID, &a.SpaceID, &a.SpaceName, &a.SpaceSlug,
			&a.Slug, &a.Title, &a.Status, &a.Content, &a.UpdatedAt, &a.Visibility,
			&a.CategoryID, &a.Category, &a.OwnerID, &a.LockVersion, &a.Classification, &a.Language,
		); err != nil {
			rows.Close()
			return nil, err
		}
		candidates = append(candidates, a)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var out []Article
	for _, a := range candidates {
		if s.canViewArticle(ctx, u, a) {
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
	err := s.db.QueryRowContext(ctx, `
		SELECT a.id,coalesce(a.organization_id,1),coalesce(a.space_id,1),coalesce(s.name,''),coalesce(s.slug,''),
			a.slug,a.title,coalesce(a.status,'published'),coalesce(a.classification,'internal'),
			coalesce(a.language,'ru'),coalesce(a.lock_version,1),a.content,a.rendered_html,a.visibility,
			a.updated_at,coalesce(a.category_id,0),coalesce(c.name,''),coalesce(a.owner_id,0)
		FROM articles a
		LEFT JOIN spaces s ON s.id=a.space_id
		LEFT JOIN categories c ON c.id=a.category_id
		WHERE a.slug=? AND a.deleted_at IS NULL`, slug).Scan(
		&a.ID, &a.OrganizationID, &a.SpaceID, &a.SpaceName, &a.SpaceSlug,
		&a.Slug, &a.Title, &a.Status, &a.Classification, &a.Language, &a.LockVersion,
		&a.Content, &html, &a.Visibility, &a.UpdatedAt, &a.CategoryID, &a.Category, &a.OwnerID,
	)
	if a.Content != "" {
		if res, renderErr := markdownx.Render(a.Content); renderErr == nil {
			if html == "" {
				html = res.HTML
			}
			a.HasMermaid = res.Mermaid
			a.Headings = res.Headings
			a.Tags = res.Tags
		}
	}
	a.HTML = template.HTML(html)
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
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id,c.name,c.slug,c.description,c.nav_order,c.is_visible,coalesce(a.id,0),
			coalesce(a.visibility,''),coalesce(a.status,''),coalesce(a.owner_id,0)
		FROM categories c
		LEFT JOIN articles a ON a.category_id=c.id AND a.deleted_at IS NULL
		WHERE c.is_visible=1 ORDER BY c.nav_order,lower(c.name)`)
	if err != nil {
		return nil, err
	}
	type candidate struct {
		category   Category
		articleID  int64
		visibility string
		status     string
		ownerID    int64
	}
	var candidates []candidate
	for rows.Next() {
		var item candidate
		var visible int
		if err := rows.Scan(&item.category.ID, &item.category.Name, &item.category.Slug, &item.category.Description, &item.category.NavOrder, &visible, &item.articleID, &item.visibility, &item.status, &item.ownerID); err == nil {
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
		if item.articleID > 0 && s.canViewArticle(ctx, u, Article{ID: item.articleID, Visibility: item.visibility, Status: item.status, OwnerID: item.ownerID}) {
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
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.id,a.slug,a.title,a.updated_at,a.visibility,coalesce(a.status,'published'),coalesce(a.owner_id,0)
		FROM links l JOIN articles a ON a.id=l.from_article_id
		WHERE l.target_slug=? AND a.deleted_at IS NULL ORDER BY a.updated_at DESC`, slug)
	if err != nil {
		return nil, err
	}
	var candidates []Article
	for rows.Next() {
		var a Article
		if err := rows.Scan(&a.ID, &a.Slug, &a.Title, &a.UpdatedAt, &a.Visibility, &a.Status, &a.OwnerID); err == nil {
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
		if s.canViewArticle(ctx, u, a) {
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
		if err := rows.Scan(&item.Slug, &item.Label); err != nil {
			s.log.Error("articleWikiLinks scan outbound", "err", err)
			continue
		}
		if item.Label == "" {
			item.Label = item.Slug
		}
		item.Direction = "out"
		out = append(out, item)
	}
	rows.Close()

	rows, err = s.db.QueryContext(ctx, `
		SELECT a.id,a.slug,a.title,a.visibility,coalesce(a.status,'published'),coalesce(a.owner_id,0)
		FROM links l JOIN articles a ON a.id=l.from_article_id
		WHERE l.target_slug=? AND a.deleted_at IS NULL ORDER BY a.updated_at DESC LIMIT 24`, slug)
	if err != nil {
		return out, err
	}
	type candidate struct {
		article Article
		item    WikiLinkItem
	}
	var candidates []candidate
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.article.ID, &item.item.Slug, &item.item.Label, &item.article.Visibility, &item.article.Status, &item.article.OwnerID); err == nil {
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
		if s.canViewArticle(ctx, u, candidate.article) {
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
	rows, err := s.db.QueryContext(ctx, `
		SELECT ae.entity_id,ae.metadata_json,ae.created_at,
			coalesce(nullif(actor.display_name,''),nullif(actor.username,''),'system'),
			coalesce(a.id,0),coalesce(a.slug,''),coalesce(a.title,''),coalesce(a.visibility,''),
			coalesce(a.status,''),coalesce(a.owner_id,0)
		FROM audit_events ae
		LEFT JOIN users actor ON actor.id=ae.actor_id
		LEFT JOIN articles a ON a.id=CAST(ae.entity_id AS INTEGER) AND ae.entity_type='article'
		WHERE ae.entity_type='article' ORDER BY ae.created_at DESC LIMIT 40`)
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
		status     string
		ownerID    int64
	}
	var candidates []candidate
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.entityID, &item.metadata, &item.createdAt, &item.actor, &item.articleID, &item.slug, &item.title, &item.visibility, &item.status, &item.ownerID); err == nil {
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
		if item.articleID > 0 && !s.canViewArticle(ctx, u, Article{ID: item.articleID, Visibility: item.visibility, Status: item.status, OwnerID: item.ownerID}) {
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
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.id,a.slug,a.title,coalesce(a.status,'published'),a.updated_at,a.visibility,
			coalesce(a.category_id,0),coalesce(c.name,''),coalesce(a.space_id,1),coalesce(s.name,''),
			coalesce(a.classification,'internal'),coalesce(a.language,'ru'),coalesce(a.lock_version,1)
		FROM articles a
		LEFT JOIN categories c ON c.id=a.category_id
		LEFT JOIN spaces s ON s.id=a.space_id
		WHERE a.deleted_at IS NULL ORDER BY a.updated_at DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Article
	for rows.Next() {
		var item Article
		if err := rows.Scan(&item.ID, &item.Slug, &item.Title, &item.Status, &item.UpdatedAt, &item.Visibility, &item.CategoryID, &item.Category, &item.SpaceID, &item.SpaceName, &item.Classification, &item.Language, &item.LockVersion); err == nil {
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
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM users WHERE id<>? AND role='admin' AND is_active=1`, userID).Scan(&otherAdmins); err != nil {
		s.log.Error("ensureAdminCanChangeUser count admins", "err", err)
		// If we can't verify, be safe and allow the change
		return nil
	}
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
	_ = s.db.QueryRowContext(ctx, `SELECT count(*) FROM article_files af JOIN articles a ON a.id=af.article_id WHERE af.file_id=? AND a.visibility='public' AND coalesce(a.status,'published')='published' AND a.deleted_at IS NULL`, fileID).Scan(&n)
	return n > 0
}

func (s *Server) userCanReadFile(ctx context.Context, u *User, fileID int64) bool {
	rows, err := s.db.QueryContext(ctx, `SELECT a.id,a.visibility,coalesce(a.status,'published'),coalesce(a.owner_id,0) FROM article_files af JOIN articles a ON a.id=af.article_id WHERE af.file_id=? AND a.deleted_at IS NULL`, fileID)
	if err != nil {
		return false
	}
	type candidate struct {
		articleID  int64
		visibility string
		status     string
		ownerID    int64
	}
	var candidates []candidate
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.articleID, &item.visibility, &item.status, &item.ownerID); err == nil {
			candidates = append(candidates, item)
		}
	}
	rows.Close()
	for _, item := range candidates {
		if s.canViewArticle(ctx, u, Article{ID: item.articleID, Visibility: item.visibility, Status: item.status, OwnerID: item.ownerID}) {
			return true
		}
	}
	return false
}

func (s *Server) canAccessFile(ctx context.Context, u *User, fileID, uploadedBy int64) bool {
	if u == nil {
		return s.fileHasPublicArticle(ctx, fileID)
	}
	return u.Role == "admin" || uploadedBy == u.ID || s.userCanReadFile(ctx, u, fileID)
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
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM acl_users WHERE article_id=? AND user_id=? AND permission IN ('read','write','admin')`, articleID, u.ID).Scan(&n); err != nil {
		s.log.Error("canRead acl_users", "err", err, "articleID", articleID, "userID", u.ID)
		return false
	}
	if n > 0 {
		return true
	}
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM acl_groups ag JOIN group_members gm ON gm.group_id=ag.group_id WHERE ag.article_id=? AND gm.user_id=? AND ag.permission IN ('read','write','admin')`, articleID, u.ID).Scan(&n); err != nil {
		s.log.Error("canRead acl_groups", "err", err, "articleID", articleID, "userID", u.ID)
		return false
	}
	return n > 0
}

func (s *Server) canViewArticle(ctx context.Context, u *User, article Article) bool {
	if !s.canRead(ctx, u, article.ID, article.Visibility) {
		return false
	}
	status := article.Status
	if status == "" {
		status = "published"
	}
	switch status {
	case "published", "archived":
		return true
	case "draft", "rejected":
		return u != nil && (u.Role == "admin" || u.ID == article.OwnerID || s.hasExplicitDocumentAccess(ctx, u, article.ID, "write", "admin"))
	case "in_review", "approved":
		return u != nil && (u.Role == "admin" || u.Role == "editor" || u.ID == article.OwnerID || s.hasExplicitDocumentAccess(ctx, u, article.ID, "write", "admin"))
	default:
		return false
	}
}

func (s *Server) hasExplicitDocumentAccess(ctx context.Context, u *User, articleID int64, permissions ...string) bool {
	if u == nil || articleID == 0 || len(permissions) == 0 {
		return false
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(permissions)), ",")
	args := []any{articleID, u.ID}
	for _, permission := range permissions {
		args = append(args, permission)
	}
	var n int
	query := `SELECT count(*) FROM acl_users WHERE article_id=? AND user_id=? AND permission IN (` + placeholders + `)`
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&n); err == nil && n > 0 {
		return true
	}
	query = `SELECT count(*) FROM acl_groups ag JOIN group_members gm ON gm.group_id=ag.group_id WHERE ag.article_id=? AND gm.user_id=? AND ag.permission IN (` + placeholders + `)`
	return s.db.QueryRowContext(ctx, query, args...).Scan(&n) == nil && n > 0
}
func (s *Server) canWrite(u *User) bool { return u != nil && (u.Role == "admin" || u.Role == "editor") }

func (s *Server) canEditDocument(ctx context.Context, u *User, articleID int64, ownerID int64, visibility string) bool {
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
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM acl_users WHERE article_id=? AND user_id=? AND permission IN ('write','admin')`, articleID, u.ID).Scan(&n); err == nil && n > 0 {
		return true
	}
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM acl_groups ag JOIN group_members gm ON gm.group_id=ag.group_id WHERE ag.article_id=? AND gm.user_id=? AND ag.permission IN ('write','admin')`, articleID, u.ID).Scan(&n); err == nil && n > 0 {
		return true
	}
	if visibility == "public" || visibility == "authenticated" {
		return true
	}
	return false
}

type articleSnapshot struct {
	Slug       string
	Title      string
	Content    string
	Visibility string
	CategoryID int64
}

var (
	storageKeyRe = regexp.MustCompile(`^[a-f0-9]{64}(\.[a-z0-9]+)?$`)
	uploadRefRe  = regexp.MustCompile(`/(?:uploads|pdf/viewer)/([A-Za-z0-9%._~-]+)`)
	extRe        = regexp.MustCompile(`^\.[a-z0-9]{1,12}$`)
	backupNameRe = regexp.MustCompile(`^docshub-[0-9]{8}T[0-9]{6}Z\.db$`)
	ftsTokenRe   = regexp.MustCompile(`[\p{L}\p{N}_]+`)
	pdfPageRe    = regexp.MustCompile(`/Type\s*/Page\b`)
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

func validClassification(classification string) string {
	switch classification {
	case "public", "internal", "confidential", "restricted":
		return classification
	default:
		return "internal"
	}
}

func validLanguage(language string) string {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "ru", "en", "de", "fr", "es", "zh":
		return strings.ToLower(strings.TrimSpace(language))
	default:
		return "ru"
	}
}

func validSearchStatus(status string) string {
	switch strings.TrimSpace(status) {
	case "draft", "in_review", "approved", "published", "rejected", "archived":
		return strings.TrimSpace(status)
	default:
		return ""
	}
}

func statusLabel(status string) string {
	switch status {
	case "draft", "":
		return "Черновик"
	case "in_review":
		return "На проверке"
	case "approved":
		return "Одобрен"
	case "published":
		return "Опубликован"
	case "rejected":
		return "Отклонён"
	case "archived":
		return "В архиве"
	default:
		return status
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func firstRune(value string) string {
	for _, r := range strings.TrimSpace(value) {
		return strings.ToUpper(string(r))
	}
	return "?"
}

func limitArticles(articles []Article, limit int) []Article {
	if limit < 0 || len(articles) <= limit {
		return articles
	}
	return articles[:limit]
}

func ftsPrefixQuery(query string) string {
	tokens := ftsTokenRe.FindAllString(strings.ToLower(query), -1)
	if len(tokens) == 0 {
		return `"__no_match__"*`
	}
	parts := make([]string, 0, len(tokens))
	for _, token := range tokens {
		parts = append(parts, `"`+strings.ReplaceAll(token, `"`, `""`)+`"*`)
	}
	return strings.Join(parts, " AND ")
}

func (s *Server) spaceExists(ctx context.Context, id int64) bool {
	var n int
	return s.db.QueryRowContext(ctx, `SELECT count(*) FROM spaces WHERE id=?`, id).Scan(&n) == nil && n == 1
}

func (s *Server) syncArticleDerivedData(ctx context.Context, tx *sql.Tx, articleID int64, title, slug, content, categoryName, categorySlug string, rendered markdownx.Result) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM article_tags WHERE article_id=?`, articleID); err != nil {
		return err
	}
	tags := articleSearchTags(rendered.Tags, categoryName, categorySlug)
	for _, tag := range tags {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO tags(name) VALUES(?)`, tag); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO article_tags(article_id,tag_id) SELECT ?,id FROM tags WHERE name=?`, articleID, tag); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM links WHERE from_article_id=?`, articleID); err != nil {
		return err
	}
	for _, link := range rendered.Links {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO links(from_article_id,target_slug,label) VALUES(?,?,?)`, articleID, link.Slug, link.Label); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM article_files WHERE article_id=?`, articleID); err != nil {
		return err
	}
	for _, key := range extractUploadKeys(content) {
		var fileID int64
		err := tx.QueryRowContext(ctx, `SELECT id FROM files WHERE storage_key=?`, key).Scan(&fileID)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO article_files(article_id,file_id,role) VALUES(?,?,?)`, articleID, fileID, "inline"); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM article_fts WHERE rowid=?`, articleID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO article_fts(rowid,title,slug,content,tags) VALUES(?,?,?,?,?)`, articleID, title, slug, content, strings.Join(tags, " "))
	return err
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("content-type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": code, "message": message})
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
	parse := func(value string) string {
		mediaType, _, err := mime.ParseMediaType(value)
		if err != nil {
			mediaType = value
		}
		return strings.ToLower(strings.TrimSpace(mediaType))
	}
	declared := parse(header)
	byExtension := parse(mime.TypeByExtension(strings.ToLower(filepath.Ext(filename))))
	sniffed := parse(http.DetectContentType(data))
	if mediaKind(sniffed) != "" {
		return sniffed
	}
	// Some audio/video formats are not recognized from a short prefix. In that
	// case require both the declared MIME and filename extension to agree on the
	// same supported kind; a caller-controlled header alone is not sufficient.
	if declaredKind, extensionKind := mediaKind(declared), mediaKind(byExtension); declaredKind != "" && declaredKind == extensionKind {
		return declared
	}
	return ""
}

func mediaKind(mimeType string) string {
	if mimeType == "image/svg+xml" {
		return ""
	}
	switch {
	case mimeType == "application/pdf":
		return "pdf"
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
	extMIME, _, _ := mime.ParseMediaType(mime.TypeByExtension(ext))
	if extRe.MatchString(ext) && mediaKind(strings.ToLower(extMIME)) == mediaKind(mimeType) {
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
	case "pdf":
		return fmt.Sprintf("[📄 %s](%s)", escapeMarkdownLabel(name), fileURL)
	default:
		return fmt.Sprintf("[%s](%s)", escapeMarkdownLabel(name), fileURL)
	}
}

func pdfPageCount(data []byte) int {
	count := len(pdfPageRe.FindAll(data, -1))
	if count < 1 {
		return 1
	}
	return count
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

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; worker-src 'self' blob: https://cdn.jsdelivr.net; connect-src 'self' https://cdn.jsdelivr.net; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; media-src 'self' blob:; frame-src 'self' blob:;")
		if s.cfg.TLS.Enabled || r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

func staticCacheHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=300, must-revalidate")
		next.ServeHTTP(w, r)
	})
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
		var stored, exp, csrf string
		err = s.db.QueryRowContext(r.Context(), `SELECT u.id,u.username,u.display_name,u.role,s.token_hash,s.expires_at,s.csrf_token FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.id=? AND u.is_active=1`, parts[0]).Scan(&u.ID, &u.Username, &u.DisplayName, &u.Role, &stored, &exp, &csrf)
		if err == nil && stored == hashToken(parts[1], s.cfg.SessionSecret) && exp > time.Now().UTC().Format(time.RFC3339) {
			ctx := context.WithValue(r.Context(), userKey{}, &u)
			ctx = context.WithValue(ctx, csrfKey{}, csrf)
			r = r.WithContext(ctx)
		}
		next.ServeHTTP(w, r)
	})
}

// csrfMiddleware validates CSRF tokens on state-changing requests.
// The CSRF token is loaded from the session during withUser authentication.
// Unauthenticated requests (no session) are passed through — endpoints like
// /login must handle their own security.
func (s *Server) csrfMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip safe methods: GET, HEAD, OPTIONS
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		// Skip unauthenticated requests (e.g., POST /login)
		if userFrom(r.Context()) == nil {
			next.ServeHTTP(w, r)
			return
		}
		sessionCSRF := csrfFrom(r.Context())
		if sessionCSRF == "" {
			http.Error(w, "forbidden: no session", http.StatusForbidden)
			return
		}
		// Accept CSRF token from form field or X-CSRF-Token header
		requestCSRF := r.FormValue("csrf_token")
		if requestCSRF == "" {
			requestCSRF = r.Header.Get("X-CSRF-Token")
		}
		if requestCSRF == "" || requestCSRF != sessionCSRF {
			http.Error(w, "forbidden: invalid CSRF token", http.StatusForbidden)
			return
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
type csrfKey struct{}

func userFrom(ctx context.Context) *User  { u, _ := ctx.Value(userKey{}).(*User); return u }
func csrfFrom(ctx context.Context) string { t, _ := ctx.Value(csrfKey{}).(string); return t }
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

// loginRateLimiter applies a strict rate limit (5 req/min, burst 3) on login attempts.
// This is a separate limiter from the global rate limiter for brute-force protection.
func (s *Server) loginRateLimiter() func(http.Handler) http.Handler {
	if !s.cfg.RateLimit.Enabled {
		return func(next http.Handler) http.Handler { return next }
	}
	type bucket struct {
		tokens   float64
		lastSeen time.Time
	}
	var (
		mu      sync.Mutex
		buckets = map[string]*bucket{}
	)
	rate := 5.0 / 60.0 // 5 tokens per minute
	burst := 3.0

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
				http.Error(w, `{"error":"too many login attempts"}`, http.StatusTooManyRequests)
				return
			}
			b.tokens--
			mu.Unlock()
			next.ServeHTTP(w, r)
		})
	}
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
