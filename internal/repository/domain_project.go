package repository

import (
	"context"

	"github.com/homiakus/docshub-next/internal/domain"
)

// DomainCreateInput is accepted only after the caller has provisioned the
// corresponding SecureAccess workspace. SecurityWorkspaceID is therefore
// required for newly-created product Domains; legacy migration rows may still
// be unbound until reconciliation completes.
type DomainCreateInput struct {
	StableKey           string
	OrganizationID      int64
	SecurityWorkspaceID string
	Slug                string
	Name                string
	Description         string
	Icon                string
	SortOrder           int
	CreatedBy           string
}

type DomainUpdateInput struct {
	ID          int64
	Slug        string
	Name        string
	Description string
	Icon        string
	Status      domain.DomainStatus
	SortOrder   int
}

type DomainRepository interface {
	GetByID(ctx context.Context, id int64) (*domain.Domain, error)
	GetByStableKey(ctx context.Context, stableKey string) (*domain.Domain, error)
	GetBySlug(ctx context.Context, organizationID int64, slug string) (*domain.Domain, error)
	ListByOrganization(ctx context.Context, organizationID int64, includeArchived bool) ([]domain.Domain, error)
	Create(ctx context.Context, input DomainCreateInput) (*domain.Domain, error)
	UpdateMetadata(ctx context.Context, input DomainUpdateInput) (*domain.Domain, error)
	BindSecurityWorkspace(ctx context.Context, id int64, workspaceID string) error
}

type ProjectCreateInput struct {
	StableKey           string
	OrganizationID      int64
	DomainID            int64
	SecurityWorkspaceID string
	Slug                string
	Name                string
	Description         string
	AccessMode          domain.ProjectAccessMode
	SortOrder           int
}

type ProjectUpdateInput struct {
	ID          int64
	Slug        string
	Name        string
	Description string
	Status      domain.ProjectStatus
	SortOrder   int
}

type ProjectRepository interface {
	GetByID(ctx context.Context, id int64) (*domain.Project, error)
	GetByStableKey(ctx context.Context, stableKey string) (*domain.Project, error)
	GetBySlug(ctx context.Context, domainID int64, slug string) (*domain.Project, error)
	ListByDomain(ctx context.Context, domainID int64, includeArchived bool) ([]domain.Project, error)
	Create(ctx context.Context, input ProjectCreateInput) (*domain.Project, error)
	UpdateMetadata(ctx context.Context, input ProjectUpdateInput) (*domain.Project, error)
	SetAccessMode(ctx context.Context, id int64, mode domain.ProjectAccessMode) error
	BindSecurityWorkspace(ctx context.Context, id int64, workspaceID string) error
}
