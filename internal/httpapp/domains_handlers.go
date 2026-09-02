package httpapp

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/homiakus/docshub-next/internal/application"
	"github.com/homiakus/docshub-next/internal/domain"
	"github.com/homiakus/docshub-next/internal/repository"
)

func (s *Server) actorFrom(r *http.Request) application.WorkspaceActor {
	u := userFrom(r.Context())
	if u == nil {
		return application.WorkspaceActor{}
	}
	return s.resolveWorkspaceActor(r.Context(), u.ID)
}

type domainDTO struct {
	ID                  int64  `json:"id"`
	StableKey           string `json:"stable_key"`
	OrganizationID      int64  `json:"organization_id"`
	SecurityWorkspaceID string `json:"security_workspace_id"`
	Slug                string `json:"slug"`
	Name                string `json:"name"`
	Description         string `json:"description"`
	Icon                string `json:"icon"`
	Status              string `json:"status"`
	SortOrder           int    `json:"sort_order"`
	CreatedAt           string `json:"created_at"`
	UpdatedAt           string `json:"updated_at"`
}

type projectDTO struct {
	ID                  int64  `json:"id"`
	DomainID            int64  `json:"domain_id"`
	StableKey           string `json:"stable_key"`
	SecurityWorkspaceID string `json:"security_workspace_id"`
	Slug                string `json:"slug"`
	Name                string `json:"name"`
	Description         string `json:"description"`
	AccessMode          string `json:"access_mode"`
	Status              string `json:"status"`
	SortOrder           int    `json:"sort_order"`
	CreatedAt           string `json:"created_at"`
	UpdatedAt           string `json:"updated_at"`
}

func toDomainDTO(d domain.Domain) domainDTO {
	return domainDTO{
		ID:                  d.ID,
		StableKey:           d.StableKey,
		OrganizationID:      d.OrganizationID,
		SecurityWorkspaceID: d.SecurityWorkspaceID,
		Slug:                d.Slug,
		Name:                d.Name,
		Description:         d.Description,
		Icon:                d.Icon,
		Status:              string(d.Status),
		SortOrder:           d.SortOrder,
		CreatedAt:           d.CreatedAt,
		UpdatedAt:           d.UpdatedAt,
	}
}

func toProjectDTO(p domain.Project) projectDTO {
	return projectDTO{
		ID:                  p.ID,
		DomainID:            p.DomainID,
		StableKey:           p.StableKey,
		SecurityWorkspaceID: p.SecurityWorkspaceID,
		Slug:                p.Slug,
		Name:                p.Name,
		Description:         p.Description,
		AccessMode:          string(p.AccessMode),
		Status:              string(p.Status),
		SortOrder:           p.SortOrder,
		CreatedAt:           p.CreatedAt,
		UpdatedAt:           p.UpdatedAt,
	}
}

func (s *Server) apiListDomains(w http.ResponseWriter, r *http.Request) {
	actor := s.actorFrom(r)
	if s.domainProjectQueryService == nil {
		http.Error(w, `{"error":"service unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	includeArchived := r.URL.Query().Get("archived") == "true"
	domains, err := s.domainProjectQueryService.ListDomains(r.Context(), actor, includeArchived)
	if err != nil {
		http.Error(w, `{"error":"failed to list domains"}`, http.StatusInternalServerError)
		return
	}

	dtos := make([]domainDTO, 0, len(domains))
	for _, d := range domains {
		dtos = append(dtos, toDomainDTO(d))
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"domains": dtos})
}

func (s *Server) apiCreateDomain(w http.ResponseWriter, r *http.Request) {
	actor := s.actorFrom(r)
	if s.domainProjectService == nil {
		http.Error(w, `{"error":"service unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	var req struct {
		StableKey   string `json:"stable_key"`
		Slug        string `json:"slug"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Icon        string `json:"icon"`
		SortOrder   int    `json:"sort_order"`
	}

	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(req.StableKey) == "" {
		req.StableKey = "domain-" + req.Slug
	}

	created, err := s.domainProjectService.CreateDomain(r.Context(), actor, application.CreateDomainCommand{
		StableKey:      req.StableKey,
		OrganizationID: actor.OrganizationID,
		Slug:           req.Slug,
		Name:           req.Name,
		Description:    req.Description,
		Icon:           req.Icon,
		SortOrder:      req.SortOrder,
	})
	if err != nil {
		if errors.Is(err, application.ErrInvalidCommand) {
			http.Error(w, `{"error":"invalid domain parameters"}`, http.StatusBadRequest)
			return
		}
		if errors.Is(err, application.ErrResourceConflict) {
			http.Error(w, `{"error":"domain slug already exists"}`, http.StatusConflict)
			return
		}
		http.Error(w, `{"error":"failed to create domain"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"domain": toDomainDTO(*created)})
}

func (s *Server) apiListProjects(w http.ResponseWriter, r *http.Request) {
	actor := s.actorFrom(r)
	if s.domainProjectQueryService == nil {
		http.Error(w, `{"error":"service unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	domainIDStr := chi.URLParam(r, "id")
	domainID, err := strconv.ParseInt(domainIDStr, 10, 64)
	if err != nil || domainID <= 0 {
		http.Error(w, `{"error":"invalid domain id"}`, http.StatusBadRequest)
		return
	}

	includeArchived := r.URL.Query().Get("archived") == "true"
	projects, err := s.domainProjectQueryService.ListProjects(r.Context(), actor, domainID, includeArchived)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			http.Error(w, `{"error":"domain not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error":"failed to list projects"}`, http.StatusInternalServerError)
		return
	}

	dtos := make([]projectDTO, 0, len(projects))
	for _, p := range projects {
		dtos = append(dtos, toProjectDTO(p))
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"projects": dtos})
}

func (s *Server) apiCreateProject(w http.ResponseWriter, r *http.Request) {
	actor := s.actorFrom(r)
	if s.domainProjectService == nil {
		http.Error(w, `{"error":"service unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	var req struct {
		DomainID    int64  `json:"domain_id"`
		StableKey   string `json:"stable_key"`
		Slug        string `json:"slug"`
		Name        string `json:"name"`
		Description string `json:"description"`
		AccessMode  string `json:"access_mode"`
		SortOrder   int    `json:"sort_order"`
	}

	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(req.StableKey) == "" {
		req.StableKey = "project-" + req.Slug
	}
	accessMode := domain.ProjectAccessInherit
	if req.AccessMode == string(domain.ProjectAccessRestricted) {
		accessMode = domain.ProjectAccessRestricted
	}

	created, err := s.domainProjectService.CreateProject(r.Context(), actor, application.CreateProjectCommand{
		DomainID:       req.DomainID,
		OrganizationID: actor.OrganizationID,
		StableKey:      req.StableKey,
		Slug:           req.Slug,
		Name:           req.Name,
		Description:    req.Description,
		AccessMode:     accessMode,
		SortOrder:      req.SortOrder,
	})
	if err != nil {
		if errors.Is(err, application.ErrInvalidCommand) {
			http.Error(w, `{"error":"invalid project parameters"}`, http.StatusBadRequest)
			return
		}
		if errors.Is(err, application.ErrResourceConflict) {
			http.Error(w, `{"error":"project slug already exists"}`, http.StatusConflict)
			return
		}
		if errors.Is(err, repository.ErrNotFound) {
			http.Error(w, `{"error":"domain not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error":"failed to create project"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"project": toProjectDTO(*created)})
}

func (s *Server) domainsPage(w http.ResponseWriter, r *http.Request) {
	actor := s.actorFrom(r)
	var domains []domain.Domain
	if s.domainProjectQueryService != nil {
		domains, _ = s.domainProjectQueryService.ListDomains(r.Context(), actor, false)
	}
	s.render(w, r, "domains/index", Page{
		Title:   "Домены знаний",
		Domains: domains,
	})
}

func (s *Server) showDomainPage(w http.ResponseWriter, r *http.Request) {
	actor := s.actorFrom(r)
	slug := slugParam(r)
	if slug == "" || s.domainProjectQueryService == nil {
		http.NotFound(w, r)
		return
	}

	domains, err := s.domainProjectQueryService.ListDomains(r.Context(), actor, false)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	var current *domain.Domain
	for _, d := range domains {
		if d.Slug == slug {
			domCopy := d
			current = &domCopy
			break
		}
	}
	if current == nil {
		http.NotFound(w, r)
		return
	}

	projects, _ := s.domainProjectQueryService.ListProjects(r.Context(), actor, current.ID, false)
	s.render(w, r, "domains/show", Page{
		Title:         current.Name,
		CurrentDomain: current,
		Projects:      projects,
	})
}
