package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/homiakus/docshub-next/internal/domain"
	"github.com/homiakus/docshub-next/internal/repository"
)

// DomainProjectScopePort is the query-side security contract. The future
// SecureAccess adapter translates Service.Scope(..., PermissionRead) into this
// product-shaped projection without leaking SDK types into repositories.
type DomainProjectScopePort interface {
	ReadWorkspaceScope(ctx context.Context, actor WorkspaceActor) (repository.WorkspaceAccessScope, error)
}

type DomainProjectQueryService struct {
	domains        repository.DomainRepository
	scopedDomains repository.ScopedDomainReader
	scopedProjects repository.ScopedProjectReader
	scope          DomainProjectScopePort
}

func NewDomainProjectQueryService(
	domains repository.DomainRepository,
	scopedDomains repository.ScopedDomainReader,
	scopedProjects repository.ScopedProjectReader,
	scope DomainProjectScopePort,
) *DomainProjectQueryService {
	return &DomainProjectQueryService{
		domains: domains,
		scopedDomains: scopedDomains,
		scopedProjects: scopedProjects,
		scope: scope,
	}
}

func (s *DomainProjectQueryService) ListDomains(ctx context.Context, actor WorkspaceActor, includeArchived bool) ([]domain.Domain, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if err := validateActor(actor); err != nil {
		return nil, fmt.Errorf("%w: list domains", ErrInvalidCommand)
	}
	scope, err := s.scope.ReadWorkspaceScope(ctx, actor)
	if err != nil {
		return nil, err
	}
	return s.scopedDomains.ListByOrganizationScoped(ctx, actor.OrganizationID, includeArchived, sanitizeWorkspaceScope(scope))
}

func (s *DomainProjectQueryService) ListProjects(ctx context.Context, actor WorkspaceActor, domainID int64, includeArchived bool) ([]domain.Project, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if err := validateActor(actor); err != nil || domainID <= 0 {
		return nil, fmt.Errorf("%w: list projects", ErrInvalidCommand)
	}

	parent, err := s.domains.GetByID(ctx, domainID)
	if err != nil {
		return nil, err
	}
	if parent.OrganizationID != actor.OrganizationID {
		return nil, repository.ErrNotFound
	}
	if strings.TrimSpace(parent.SecurityWorkspaceID) == "" {
		return nil, ErrSecurityBinding
	}

	scope, err := s.scope.ReadWorkspaceScope(ctx, actor)
	if err != nil {
		return nil, err
	}
	scope = sanitizeWorkspaceScope(scope)
	if !scopeContains(scope, parent.SecurityWorkspaceID) {
		// Deliberately use not-found semantics so a caller cannot distinguish an
		// inaccessible Domain from a missing one through the catalog endpoint.
		return nil, repository.ErrNotFound
	}
	return s.scopedProjects.ListByDomainScoped(ctx, domainID, includeArchived, scope)
}

func (s *DomainProjectQueryService) ready() error {
	if s == nil || s.domains == nil || s.scopedDomains == nil || s.scopedProjects == nil || s.scope == nil {
		return ErrServiceMisconfigured
	}
	return nil
}

func sanitizeWorkspaceScope(scope repository.WorkspaceAccessScope) repository.WorkspaceAccessScope {
	seen := make(map[string]struct{}, len(scope.WorkspaceIDs))
	out := make([]string, 0, len(scope.WorkspaceIDs))
	for _, id := range scope.WorkspaceIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return repository.WorkspaceAccessScope{WorkspaceIDs: out}
}

func scopeContains(scope repository.WorkspaceAccessScope, workspaceID string) bool {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return false
	}
	for _, allowed := range scope.WorkspaceIDs {
		if allowed == workspaceID {
			return true
		}
	}
	return false
}
