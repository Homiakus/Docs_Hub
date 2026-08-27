package application

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/homiakus/docshub-next/internal/domain"
	"github.com/homiakus/docshub-next/internal/repository"
)

var (
	ErrInvalidCommand      = errors.New("application: invalid command")
	ErrOrganizationBoundary = errors.New("application: organization boundary violation")
	ErrSecurityBinding     = errors.New("application: security workspace binding is missing")
	ErrResourceConflict    = errors.New("application: resource identity conflict")
	ErrServiceMisconfigured = errors.New("application: domain/project service is misconfigured")
)

// WorkspaceActor is the minimum request identity the Domain/Project use-case
// layer needs. The SecureAccess bridge owns translation from the authenticated
// request/session into the security library's Principal representation.
type WorkspaceActor struct {
	UserID         int64
	OrganizationID int64
}

type WorkspacePermission string

const (
	WorkspacePermissionRead   WorkspacePermission = "read"
	WorkspacePermissionManage WorkspacePermission = "manage_workspace"
)

type DomainWorkspaceRequest struct {
	OrganizationID int64
	StableKey      string
	Name           string
}

type ProjectWorkspaceRequest struct {
	OrganizationID    int64
	ParentWorkspaceID string
	StableKey         string
	Name              string
	AccessMode        domain.ProjectAccessMode
}

// DomainProjectSecurityPort deliberately contains product-shaped operations
// rather than SecureAccess SDK types. A concrete adapter is introduced in the
// next slice. Repositories and HTTP handlers never need to import SecureAccess.
type DomainProjectSecurityPort interface {
	// EnsureDomainWorkspace must authorize creation in the Organization security
	// root and idempotently return the workspace bound to StableKey.
	EnsureDomainWorkspace(ctx context.Context, actor WorkspaceActor, req DomainWorkspaceRequest) (string, error)

	// EnsureProjectWorkspace must authorize management of ParentWorkspaceID and
	// idempotently return the child workspace bound to StableKey.
	EnsureProjectWorkspace(ctx context.Context, actor WorkspaceActor, req ProjectWorkspaceRequest) (string, error)

	RequireWorkspacePermission(ctx context.Context, actor WorkspaceActor, workspaceID string, permission WorkspacePermission) error

	// SetProjectAccessMode changes the authoritative security boundary first.
	// The Docs_Hub Project access_mode column is only a host-side projection.
	SetProjectAccessMode(ctx context.Context, actor WorkspaceActor, workspaceID string, mode domain.ProjectAccessMode) error
}

type DomainProjectService struct {
	domains  repository.DomainRepository
	projects repository.ProjectRepository
	security DomainProjectSecurityPort
}

func NewDomainProjectService(domains repository.DomainRepository, projects repository.ProjectRepository, security DomainProjectSecurityPort) *DomainProjectService {
	return &DomainProjectService{domains: domains, projects: projects, security: security}
}

type CreateDomainCommand struct {
	StableKey      string
	OrganizationID int64
	Slug           string
	Name           string
	Description    string
	Icon           string
	SortOrder      int
}

type UpdateDomainCommand struct {
	ID          int64
	Slug        string
	Name        string
	Description string
	Icon        string
	Status      domain.DomainStatus
	SortOrder   int
}

type CreateProjectCommand struct {
	StableKey      string
	OrganizationID int64
	DomainID       int64
	Slug           string
	Name           string
	Description    string
	AccessMode     domain.ProjectAccessMode
	SortOrder      int
}

type UpdateProjectCommand struct {
	ID          int64
	Slug        string
	Name        string
	Description string
	Status      domain.ProjectStatus
	SortOrder   int
}

func (s *DomainProjectService) GetDomain(ctx context.Context, actor WorkspaceActor, id int64) (*domain.Domain, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if err := validateActor(actor); err != nil || id <= 0 {
		return nil, fmt.Errorf("%w: actor/domain id", ErrInvalidCommand)
	}
	item, err := s.domains.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if item.OrganizationID != actor.OrganizationID {
		return nil, repository.ErrNotFound
	}
	if item.SecurityWorkspaceID == "" {
		return nil, ErrSecurityBinding
	}
	if err := s.security.RequireWorkspacePermission(ctx, actor, item.SecurityWorkspaceID, WorkspacePermissionRead); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *DomainProjectService) GetProject(ctx context.Context, actor WorkspaceActor, id int64) (*domain.Project, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if err := validateActor(actor); err != nil || id <= 0 {
		return nil, fmt.Errorf("%w: actor/project id", ErrInvalidCommand)
	}
	item, err := s.projects.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if item.OrganizationID != actor.OrganizationID {
		return nil, repository.ErrNotFound
	}
	if item.SecurityWorkspaceID == "" {
		return nil, ErrSecurityBinding
	}
	if err := s.security.RequireWorkspacePermission(ctx, actor, item.SecurityWorkspaceID, WorkspacePermissionRead); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *DomainProjectService) CreateDomain(ctx context.Context, actor WorkspaceActor, cmd CreateDomainCommand) (*domain.Domain, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	cmd.StableKey = strings.TrimSpace(cmd.StableKey)
	cmd.Slug = strings.TrimSpace(cmd.Slug)
	cmd.Name = strings.TrimSpace(cmd.Name)
	if err := validateActor(actor); err != nil || cmd.OrganizationID <= 0 || actor.OrganizationID != cmd.OrganizationID || !validStableKey(cmd.StableKey) || cmd.Slug == "" || cmd.Name == "" {
		return nil, fmt.Errorf("%w: create domain", ErrInvalidCommand)
	}

	// Security provisioning happens first. If persistence fails afterwards, the
	// external stable key makes retry/reconciliation deterministic and fail-closed.
	workspaceID, err := s.security.EnsureDomainWorkspace(ctx, actor, DomainWorkspaceRequest{
		OrganizationID: cmd.OrganizationID,
		StableKey:      cmd.StableKey,
		Name:           cmd.Name,
	})
	if err != nil {
		return nil, err
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return nil, ErrSecurityBinding
	}

	if existing, err := s.domains.GetByStableKey(ctx, cmd.StableKey); err == nil {
		return s.reconcileExistingDomain(ctx, existing, cmd.OrganizationID, workspaceID)
	} else if !repository.IsNotFound(err) {
		return nil, err
	}

	created, err := s.domains.Create(ctx, repository.DomainCreateInput{
		StableKey:           cmd.StableKey,
		OrganizationID:      cmd.OrganizationID,
		SecurityWorkspaceID: workspaceID,
		Slug:                cmd.Slug,
		Name:                cmd.Name,
		Description:         cmd.Description,
		Icon:                cmd.Icon,
		SortOrder:           cmd.SortOrder,
		CreatedBy:           strconv.FormatInt(actor.UserID, 10),
	})
	if err == nil {
		return created, nil
	}

	// A concurrent retry may have won the unique stable-key insert. Re-read and
	// accept only the exact security identity; never convert an arbitrary DB
	// failure into success.
	if existing, lookupErr := s.domains.GetByStableKey(ctx, cmd.StableKey); lookupErr == nil {
		if reconciled, reconcileErr := s.reconcileExistingDomain(ctx, existing, cmd.OrganizationID, workspaceID); reconcileErr == nil {
			return reconciled, nil
		}
	}
	return nil, err
}

func (s *DomainProjectService) UpdateDomain(ctx context.Context, actor WorkspaceActor, cmd UpdateDomainCommand) (*domain.Domain, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	cmd.Slug = strings.TrimSpace(cmd.Slug)
	cmd.Name = strings.TrimSpace(cmd.Name)
	if err := validateActor(actor); err != nil || cmd.ID <= 0 || cmd.Slug == "" || cmd.Name == "" || !validDomainStatus(cmd.Status) {
		return nil, fmt.Errorf("%w: update domain", ErrInvalidCommand)
	}
	current, err := s.domains.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}
	if current.OrganizationID != actor.OrganizationID {
		return nil, repository.ErrNotFound
	}
	if current.SecurityWorkspaceID == "" {
		return nil, ErrSecurityBinding
	}
	if err := s.security.RequireWorkspacePermission(ctx, actor, current.SecurityWorkspaceID, WorkspacePermissionManage); err != nil {
		return nil, err
	}
	return s.domains.UpdateMetadata(ctx, repository.DomainUpdateInput{
		ID: cmd.ID, Slug: cmd.Slug, Name: cmd.Name, Description: cmd.Description,
		Icon: cmd.Icon, Status: cmd.Status, SortOrder: cmd.SortOrder,
	})
}

func (s *DomainProjectService) CreateProject(ctx context.Context, actor WorkspaceActor, cmd CreateProjectCommand) (*domain.Project, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	cmd.StableKey = strings.TrimSpace(cmd.StableKey)
	cmd.Slug = strings.TrimSpace(cmd.Slug)
	cmd.Name = strings.TrimSpace(cmd.Name)
	if cmd.AccessMode == "" {
		cmd.AccessMode = domain.ProjectAccessInherit
	}
	if err := validateActor(actor); err != nil || cmd.OrganizationID <= 0 || actor.OrganizationID != cmd.OrganizationID || cmd.DomainID <= 0 || !validStableKey(cmd.StableKey) || cmd.Slug == "" || cmd.Name == "" || !validProjectAccessMode(cmd.AccessMode) {
		return nil, fmt.Errorf("%w: create project", ErrInvalidCommand)
	}

	parent, err := s.domains.GetByID(ctx, cmd.DomainID)
	if err != nil {
		return nil, err
	}
	if parent.OrganizationID != actor.OrganizationID || parent.OrganizationID != cmd.OrganizationID {
		return nil, repository.ErrNotFound
	}
	if parent.SecurityWorkspaceID == "" {
		return nil, ErrSecurityBinding
	}

	workspaceID, err := s.security.EnsureProjectWorkspace(ctx, actor, ProjectWorkspaceRequest{
		OrganizationID:    cmd.OrganizationID,
		ParentWorkspaceID: parent.SecurityWorkspaceID,
		StableKey:         cmd.StableKey,
		Name:              cmd.Name,
		AccessMode:        cmd.AccessMode,
	})
	if err != nil {
		return nil, err
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return nil, ErrSecurityBinding
	}

	if existing, err := s.projects.GetByStableKey(ctx, cmd.StableKey); err == nil {
		return s.reconcileExistingProject(ctx, existing, cmd.OrganizationID, cmd.DomainID, workspaceID, cmd.AccessMode)
	} else if !repository.IsNotFound(err) {
		return nil, err
	}

	created, err := s.projects.Create(ctx, repository.ProjectCreateInput{
		StableKey:           cmd.StableKey,
		OrganizationID:      cmd.OrganizationID,
		DomainID:            cmd.DomainID,
		SecurityWorkspaceID: workspaceID,
		Slug:                cmd.Slug,
		Name:                cmd.Name,
		Description:         cmd.Description,
		AccessMode:          cmd.AccessMode,
		SortOrder:           cmd.SortOrder,
	})
	if err == nil {
		return created, nil
	}
	if existing, lookupErr := s.projects.GetByStableKey(ctx, cmd.StableKey); lookupErr == nil {
		if reconciled, reconcileErr := s.reconcileExistingProject(ctx, existing, cmd.OrganizationID, cmd.DomainID, workspaceID, cmd.AccessMode); reconcileErr == nil {
			return reconciled, nil
		}
	}
	return nil, err
}

func (s *DomainProjectService) UpdateProject(ctx context.Context, actor WorkspaceActor, cmd UpdateProjectCommand) (*domain.Project, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	cmd.Slug = strings.TrimSpace(cmd.Slug)
	cmd.Name = strings.TrimSpace(cmd.Name)
	if err := validateActor(actor); err != nil || cmd.ID <= 0 || cmd.Slug == "" || cmd.Name == "" || !validProjectStatus(cmd.Status) {
		return nil, fmt.Errorf("%w: update project", ErrInvalidCommand)
	}
	current, err := s.projects.GetByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}
	if current.OrganizationID != actor.OrganizationID {
		return nil, repository.ErrNotFound
	}
	if current.SecurityWorkspaceID == "" {
		return nil, ErrSecurityBinding
	}
	if err := s.security.RequireWorkspacePermission(ctx, actor, current.SecurityWorkspaceID, WorkspacePermissionManage); err != nil {
		return nil, err
	}
	return s.projects.UpdateMetadata(ctx, repository.ProjectUpdateInput{
		ID: cmd.ID, Slug: cmd.Slug, Name: cmd.Name, Description: cmd.Description,
		Status: cmd.Status, SortOrder: cmd.SortOrder,
	})
}

func (s *DomainProjectService) SetProjectAccessMode(ctx context.Context, actor WorkspaceActor, projectID int64, mode domain.ProjectAccessMode) (*domain.Project, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if err := validateActor(actor); err != nil || projectID <= 0 || !validProjectAccessMode(mode) {
		return nil, fmt.Errorf("%w: set project access mode", ErrInvalidCommand)
	}
	current, err := s.projects.GetByID(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if current.OrganizationID != actor.OrganizationID {
		return nil, repository.ErrNotFound
	}
	if current.SecurityWorkspaceID == "" {
		return nil, ErrSecurityBinding
	}

	// SecureAccess is authoritative. Persist the host mirror only after the
	// security boundary changed successfully. A failed mirror write is safe to
	// retry because the security operation must be idempotent.
	if err := s.security.SetProjectAccessMode(ctx, actor, current.SecurityWorkspaceID, mode); err != nil {
		return nil, err
	}
	if err := s.projects.SetAccessMode(ctx, projectID, mode); err != nil {
		return nil, err
	}
	return s.projects.GetByID(ctx, projectID)
}

func (s *DomainProjectService) reconcileExistingDomain(ctx context.Context, existing *domain.Domain, organizationID int64, workspaceID string) (*domain.Domain, error) {
	if existing == nil || existing.OrganizationID != organizationID {
		return nil, ErrResourceConflict
	}
	if existing.SecurityWorkspaceID == "" {
		if err := s.domains.BindSecurityWorkspace(ctx, existing.ID, workspaceID); err != nil {
			return nil, err
		}
		return s.domains.GetByID(ctx, existing.ID)
	}
	if existing.SecurityWorkspaceID != workspaceID {
		return nil, ErrResourceConflict
	}
	return existing, nil
}

func (s *DomainProjectService) reconcileExistingProject(ctx context.Context, existing *domain.Project, organizationID, domainID int64, workspaceID string, mode domain.ProjectAccessMode) (*domain.Project, error) {
	if existing == nil || existing.OrganizationID != organizationID || existing.DomainID != domainID || existing.AccessMode != mode {
		return nil, ErrResourceConflict
	}
	if existing.SecurityWorkspaceID == "" {
		if err := s.projects.BindSecurityWorkspace(ctx, existing.ID, workspaceID); err != nil {
			return nil, err
		}
		return s.projects.GetByID(ctx, existing.ID)
	}
	if existing.SecurityWorkspaceID != workspaceID {
		return nil, ErrResourceConflict
	}
	return existing, nil
}

func (s *DomainProjectService) ready() error {
	if s == nil || s.domains == nil || s.projects == nil || s.security == nil {
		return ErrServiceMisconfigured
	}
	return nil
}

func validateActor(actor WorkspaceActor) error {
	if actor.UserID <= 0 || actor.OrganizationID <= 0 {
		return ErrInvalidCommand
	}
	return nil
}

func validStableKey(value string) bool {
	if len(value) < 8 || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' || r == ':' {
			continue
		}
		return false
	}
	return true
}

func validDomainStatus(status domain.DomainStatus) bool {
	return status == domain.DomainActive || status == domain.DomainArchived
}

func validProjectStatus(status domain.ProjectStatus) bool {
	return status == domain.ProjectActive || status == domain.ProjectPaused || status == domain.ProjectArchived
}

func validProjectAccessMode(mode domain.ProjectAccessMode) bool {
	return mode == domain.ProjectAccessInherit || mode == domain.ProjectAccessRestricted
}
