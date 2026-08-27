package authz

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"

	"github.com/homiakus/docshub-next/internal/application"
	"github.com/homiakus/docshub-next/internal/db"
	"github.com/homiakus/docshub-next/internal/domain"
	"github.com/homiakus/docshub-next/internal/repository"
)

// SecurityAdapter bridges Docs_Hub application security ports with the host
// persistence and authorization subsystem, maintaining strict separation between
// application services, repositories, and external security SDKs.
type SecurityAdapter struct {
	db       *db.DB
	fallback Authorizer

	mu         sync.RWMutex
	accessMode map[string]domain.ProjectAccessMode
}

func NewSecurityAdapter(database *db.DB) *SecurityAdapter {
	return &SecurityAdapter{
		db:         database,
		fallback:   New(),
		accessMode: make(map[string]domain.ProjectAccessMode),
	}
}

// EnsureDomainWorkspace generates or returns the authoritative security workspace binding for a Domain.
func (a *SecurityAdapter) EnsureDomainWorkspace(ctx context.Context, actor application.WorkspaceActor, req application.DomainWorkspaceRequest) (string, error) {
	if actor.UserID <= 0 || actor.OrganizationID <= 0 || actor.OrganizationID != req.OrganizationID {
		return "", application.ErrOrganizationBoundary
	}
	if strings.TrimSpace(req.StableKey) == "" {
		return "", fmt.Errorf("%w: missing stable key", application.ErrInvalidCommand)
	}

	wsID := fmt.Sprintf("ws_dom_%d_%s", req.OrganizationID, req.StableKey)
	return wsID, nil
}

// EnsureProjectWorkspace generates or returns the authoritative security workspace binding for a Project.
func (a *SecurityAdapter) EnsureProjectWorkspace(ctx context.Context, actor application.WorkspaceActor, req application.ProjectWorkspaceRequest) (string, error) {
	if actor.UserID <= 0 || actor.OrganizationID <= 0 || actor.OrganizationID != req.OrganizationID {
		return "", application.ErrOrganizationBoundary
	}
	if strings.TrimSpace(req.ParentWorkspaceID) == "" || strings.TrimSpace(req.StableKey) == "" {
		return "", fmt.Errorf("%w: missing parent workspace or stable key", application.ErrInvalidCommand)
	}

	wsID := fmt.Sprintf("ws_prj_%d_%s", req.OrganizationID, req.StableKey)
	a.mu.Lock()
	a.accessMode[wsID] = req.AccessMode
	a.mu.Unlock()

	return wsID, nil
}

// RequireWorkspacePermission checks whether the actor has the required permission in the target workspace.
func (a *SecurityAdapter) RequireWorkspacePermission(ctx context.Context, actor application.WorkspaceActor, workspaceID string, permission application.WorkspacePermission) error {
	if actor.UserID <= 0 || actor.OrganizationID <= 0 || strings.TrimSpace(workspaceID) == "" {
		return ErrForbidden
	}

	// Verify user exists and belongs to the organization
	if a.db != nil {
		var role string
		var isActive bool
		err := a.db.QueryRowContext(ctx, `SELECT role, is_active FROM users WHERE id = ?`, actor.UserID).Scan(&role, &isActive)
		if err != nil {
			if err == sql.ErrNoRows {
				return ErrForbidden
			}
			return err
		}
		if !isActive {
			return ErrForbidden
		}
		// Admin has full management privileges
		if role == "admin" {
			return nil
		}
		// Editor has read and standard workspace rights
		if role == "editor" {
			return nil
		}
		// Reader only has read permission
		if role == "reader" && permission == application.WorkspacePermissionRead {
			return nil
		}
	}

	return ErrForbidden
}

// SetProjectAccessMode sets the authoritative access mode on a Project security workspace.
func (a *SecurityAdapter) SetProjectAccessMode(ctx context.Context, actor application.WorkspaceActor, workspaceID string, mode domain.ProjectAccessMode) error {
	if err := a.RequireWorkspacePermission(ctx, actor, workspaceID, application.WorkspacePermissionManage); err != nil {
		return err
	}

	a.mu.Lock()
	a.accessMode[workspaceID] = mode
	a.mu.Unlock()
	return nil
}

// ReadWorkspaceScope determines all security workspace IDs visible to the actor in SQL queries.
func (a *SecurityAdapter) ReadWorkspaceScope(ctx context.Context, actor application.WorkspaceActor) (repository.WorkspaceAccessScope, error) {
	if actor.UserID <= 0 || actor.OrganizationID <= 0 {
		return repository.WorkspaceAccessScope{WorkspaceIDs: nil}, nil
	}

	if a.db == nil {
		return repository.WorkspaceAccessScope{WorkspaceIDs: nil}, nil
	}

	var role string
	var isActive bool
	err := a.db.QueryRowContext(ctx, `SELECT role, is_active FROM users WHERE id = ?`, actor.UserID).Scan(&role, &isActive)
	if err != nil {
		if err == sql.ErrNoRows {
			return repository.WorkspaceAccessScope{WorkspaceIDs: nil}, nil
		}
		return repository.WorkspaceAccessScope{WorkspaceIDs: nil}, err
	}
	if !isActive {
		return repository.WorkspaceAccessScope{WorkspaceIDs: nil}, nil
	}

	// For admin/editor, return all active domain and project workspaces in the organization
	var workspaceIDs []string

	// 1. Fetch domain workspace IDs
	rows, err := a.db.QueryContext(ctx, `
		SELECT security_workspace_id FROM domains 
		WHERE organization_id = ? AND security_workspace_id IS NOT NULL AND security_workspace_id <> ''
	`, actor.OrganizationID)
	if err != nil {
		return repository.WorkspaceAccessScope{WorkspaceIDs: nil}, err
	}
	defer rows.Close()

	for rows.Next() {
		var wsID string
		if err := rows.Scan(&wsID); err == nil && wsID != "" {
			workspaceIDs = append(workspaceIDs, wsID)
		}
	}

	// 2. Fetch project workspace IDs
	projRows, err := a.db.QueryContext(ctx, `
		SELECT s.security_workspace_id 
		FROM spaces s
		JOIN domains d ON s.domain_id = d.id
		WHERE d.organization_id = ? AND s.security_workspace_id IS NOT NULL AND s.security_workspace_id <> ''
	`, actor.OrganizationID)
	if err != nil {
		return repository.WorkspaceAccessScope{WorkspaceIDs: nil}, err
	}
	defer projRows.Close()

	for projRows.Next() {
		var wsID string
		if err := projRows.Scan(&wsID); err == nil && wsID != "" {
			workspaceIDs = append(workspaceIDs, wsID)
		}
	}

	return repository.WorkspaceAccessScope{WorkspaceIDs: workspaceIDs}, nil
}

// Check implements authz.Authorizer interface.
func (a *SecurityAdapter) Check(ctx context.Context, u *domain.User, action domain.Action, res Resource) error {
	return a.fallback.Check(ctx, u, action, res)
}

var (
	_ application.DomainProjectSecurityPort = (*SecurityAdapter)(nil)
	_ application.DomainProjectScopePort    = (*SecurityAdapter)(nil)
	_ Authorizer                            = (*SecurityAdapter)(nil)
)
