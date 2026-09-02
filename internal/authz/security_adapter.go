package authz

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/homiakus/docshub-next/internal/application"
	"github.com/homiakus/docshub-next/internal/db"
	"github.com/homiakus/docshub-next/internal/domain"
	"github.com/homiakus/docshub-next/internal/repository"
)

// SecurityAdapter is the compatibility bridge between Docs_Hub application
// security ports and the legacy local ACL tables. SecureAccess remains the
// target authority; this adapter deliberately fails closed once more than one
// Organization exists unless the actor has an explicit organization-scoped
// role binding.
type SecurityAdapter struct {
	db       *db.DB
	fallback Authorizer
}

func NewSecurityAdapter(database *db.DB) *SecurityAdapter {
	return &SecurityAdapter{db: database, fallback: New()}
}

// EnsureDomainWorkspace returns the deterministic compatibility binding for a
// Domain only after proving the actor belongs to the requested Organization and
// may manage workspaces there.
func (a *SecurityAdapter) EnsureDomainWorkspace(ctx context.Context, actor application.WorkspaceActor, req application.DomainWorkspaceRequest) (string, error) {
	if actor.UserID <= 0 || actor.OrganizationID <= 0 || actor.OrganizationID != req.OrganizationID {
		return "", application.ErrOrganizationBoundary
	}
	if strings.TrimSpace(req.StableKey) == "" {
		return "", fmt.Errorf("%w: missing stable key", application.ErrInvalidCommand)
	}
	if err := a.requireOrganizationMembership(ctx, actor); err != nil {
		return "", err
	}
	role, err := a.activeUserRole(ctx, actor.UserID)
	if err != nil {
		return "", err
	}
	if err := roleAllowsWorkspacePermission(role, application.WorkspacePermissionManage); err != nil {
		return "", err
	}
	return fmt.Sprintf("ws_dom_%d_%s", req.OrganizationID, req.StableKey), nil
}

// EnsureProjectWorkspace proves access to the parent Domain first. The returned
// stable identifier is a compatibility binding until SecureAccess provisioning
// is wired into the composition root.
func (a *SecurityAdapter) EnsureProjectWorkspace(ctx context.Context, actor application.WorkspaceActor, req application.ProjectWorkspaceRequest) (string, error) {
	if actor.UserID <= 0 || actor.OrganizationID <= 0 || actor.OrganizationID != req.OrganizationID {
		return "", application.ErrOrganizationBoundary
	}
	if strings.TrimSpace(req.ParentWorkspaceID) == "" || strings.TrimSpace(req.StableKey) == "" {
		return "", fmt.Errorf("%w: missing parent workspace or stable key", application.ErrInvalidCommand)
	}
	if err := a.RequireWorkspacePermission(ctx, actor, req.ParentWorkspaceID, application.WorkspacePermissionManage); err != nil {
		return "", err
	}
	return fmt.Sprintf("ws_prj_%d_%s", req.OrganizationID, req.StableKey), nil
}

// RequireWorkspacePermission verifies three independent invariants before the
// legacy role can grant anything: active user, organization membership, and
// workspace ownership by that Organization. Restricted Projects additionally
// require an explicit project grant unless the actor is an administrator.
func (a *SecurityAdapter) RequireWorkspacePermission(ctx context.Context, actor application.WorkspaceActor, workspaceID string, permission application.WorkspacePermission) error {
	workspaceID = strings.TrimSpace(workspaceID)
	if actor.UserID <= 0 || actor.OrganizationID <= 0 || workspaceID == "" || a.db == nil {
		return ErrForbidden
	}
	if err := a.requireOrganizationMembership(ctx, actor); err != nil {
		return err
	}

	role, err := a.activeUserRole(ctx, actor.UserID)
	if err != nil {
		return err
	}
	kind, projectID, accessMode, err := a.lookupWorkspace(ctx, actor.OrganizationID, workspaceID)
	if err != nil {
		if errorsIsNoRows(err) {
			return ErrForbidden
		}
		return err
	}

	if kind == "project" && accessMode == domain.ProjectAccessRestricted && role != "admin" {
		allowed, err := a.hasExplicitProjectGrant(ctx, actor.UserID, actor.OrganizationID, projectID)
		if err != nil {
			return err
		}
		if !allowed {
			return ErrForbidden
		}
	}
	return roleAllowsWorkspacePermission(role, permission)
}

// SetProjectAccessMode persists the compatibility authority in SQLite. It is
// intentionally not process-local: a restart must never widen access. The
// application repository writes the same host projection after this call, so
// the operation remains idempotent while SecureAccess integration is pending.
func (a *SecurityAdapter) SetProjectAccessMode(ctx context.Context, actor application.WorkspaceActor, workspaceID string, mode domain.ProjectAccessMode) error {
	if mode != domain.ProjectAccessInherit && mode != domain.ProjectAccessRestricted {
		return fmt.Errorf("%w: invalid project access mode", application.ErrInvalidCommand)
	}
	if err := a.RequireWorkspacePermission(ctx, actor, workspaceID, application.WorkspacePermissionManage); err != nil {
		return err
	}
	result, err := a.db.ExecContext(ctx, `
		UPDATE spaces
		SET access_mode = ?, updated_at = datetime('now')
		WHERE security_workspace_id = ?
		  AND domain_id IN (SELECT id FROM domains WHERE organization_id = ?)
	`, string(mode), strings.TrimSpace(workspaceID), actor.OrganizationID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return ErrForbidden
	}
	return nil
}

// ReadWorkspaceScope returns only workspaces that are visible before the query
// layer performs ranking, aggregation, or LIMIT. In a restricted Project,
// non-admin users need an explicit space/project grant.
func (a *SecurityAdapter) ReadWorkspaceScope(ctx context.Context, actor application.WorkspaceActor) (repository.WorkspaceAccessScope, error) {
	empty := repository.WorkspaceAccessScope{WorkspaceIDs: nil}
	if actor.UserID <= 0 || actor.OrganizationID <= 0 || a.db == nil {
		return empty, nil
	}
	if err := a.requireOrganizationMembership(ctx, actor); err != nil {
		if err == ErrForbidden {
			return empty, nil
		}
		return empty, err
	}
	role, err := a.activeUserRole(ctx, actor.UserID)
	if err != nil {
		if err == ErrForbidden {
			return empty, nil
		}
		return empty, err
	}

	workspaceIDs := make([]string, 0, 16)
	domainRows, err := a.db.QueryContext(ctx, `
		SELECT security_workspace_id
		FROM domains
		WHERE organization_id = ?
		  AND security_workspace_id IS NOT NULL
		  AND security_workspace_id <> ''
	`, actor.OrganizationID)
	if err != nil {
		return empty, err
	}
	for domainRows.Next() {
		var wsID string
		if err := domainRows.Scan(&wsID); err != nil {
			domainRows.Close()
			return empty, err
		}
		workspaceIDs = append(workspaceIDs, wsID)
	}
	if err := domainRows.Err(); err != nil {
		domainRows.Close()
		return empty, err
	}
	domainRows.Close()

	projectRows, err := a.db.QueryContext(ctx, `
		SELECT s.id, s.security_workspace_id, s.access_mode
		FROM spaces s
		JOIN domains d ON d.id = s.domain_id
		WHERE d.organization_id = ?
		  AND s.security_workspace_id IS NOT NULL
		  AND s.security_workspace_id <> ''
	`, actor.OrganizationID)
	if err != nil {
		return empty, err
	}
	defer projectRows.Close()

	for projectRows.Next() {
		var projectID int64
		var wsID, rawMode string
		if err := projectRows.Scan(&projectID, &wsID, &rawMode); err != nil {
			return empty, err
		}
		mode := domain.ProjectAccessMode(rawMode)
		if mode == domain.ProjectAccessRestricted && role != "admin" {
			allowed, err := a.hasExplicitProjectGrant(ctx, actor.UserID, actor.OrganizationID, projectID)
			if err != nil {
				return empty, err
			}
			if !allowed {
				continue
			}
		}
		workspaceIDs = append(workspaceIDs, wsID)
	}
	if err := projectRows.Err(); err != nil {
		return empty, err
	}
	return repository.WorkspaceAccessScope{WorkspaceIDs: workspaceIDs}, nil
}

func (a *SecurityAdapter) requireOrganizationMembership(ctx context.Context, actor application.WorkspaceActor) error {
	if a.db == nil || actor.UserID <= 0 || actor.OrganizationID <= 0 {
		return ErrForbidden
	}
	if _, err := a.activeUserRole(ctx, actor.UserID); err != nil {
		return err
	}

	// Safe legacy compatibility: with exactly one Organization there is no
	// cross-tenant boundary to cross, so existing global users remain usable.
	var orgCount int
	if err := a.db.QueryRowContext(ctx, `SELECT count(*) FROM organizations`).Scan(&orgCount); err != nil {
		return err
	}
	if orgCount == 1 {
		var onlyOrgID int64
		if err := a.db.QueryRowContext(ctx, `SELECT id FROM organizations LIMIT 1`).Scan(&onlyOrgID); err != nil {
			return err
		}
		if onlyOrgID == actor.OrganizationID {
			return nil
		}
		return ErrForbidden
	}

	var one int
	err := a.db.QueryRowContext(ctx, `
		SELECT 1
		FROM role_bindings rb
		WHERE rb.organization_id = ?
		  AND (
			(rb.subject_type = 'user' AND rb.subject_id = ?)
			OR
			(rb.subject_type = 'group' AND rb.subject_id IN (
				SELECT gm.group_id FROM group_members gm WHERE gm.user_id = ?
			))
		  )
		LIMIT 1
	`, actor.OrganizationID, actor.UserID, actor.UserID).Scan(&one)
	if errorsIsNoRows(err) {
		return ErrForbidden
	}
	return err
}

func (a *SecurityAdapter) activeUserRole(ctx context.Context, userID int64) (string, error) {
	if a.db == nil || userID <= 0 {
		return "", ErrForbidden
	}
	var role string
	var active bool
	err := a.db.QueryRowContext(ctx, `SELECT role, is_active FROM users WHERE id = ?`, userID).Scan(&role, &active)
	if errorsIsNoRows(err) {
		return "", ErrForbidden
	}
	if err != nil {
		return "", err
	}
	if !active {
		return "", ErrForbidden
	}
	return role, nil
}

func roleAllowsWorkspacePermission(role string, permission application.WorkspacePermission) error {
	switch role {
	case "admin", "editor":
		return nil
	case "reader":
		if permission == application.WorkspacePermissionRead {
			return nil
		}
	}
	return ErrForbidden
}

func (a *SecurityAdapter) lookupWorkspace(ctx context.Context, organizationID int64, workspaceID string) (kind string, projectID int64, accessMode domain.ProjectAccessMode, err error) {
	var rawMode string
	err = a.db.QueryRowContext(ctx, `
		SELECT kind, project_id, access_mode
		FROM (
			SELECT 'domain' AS kind, 0 AS project_id, 'inherit' AS access_mode
			FROM domains
			WHERE organization_id = ? AND security_workspace_id = ?
			UNION ALL
			SELECT 'project' AS kind, s.id AS project_id, s.access_mode
			FROM spaces s
			JOIN domains d ON d.id = s.domain_id
			WHERE d.organization_id = ? AND s.security_workspace_id = ?
		)
		LIMIT 1
	`, organizationID, workspaceID, organizationID, workspaceID).Scan(&kind, &projectID, &rawMode)
	if err != nil {
		return "", 0, "", err
	}
	return kind, projectID, domain.ProjectAccessMode(rawMode), nil
}

func (a *SecurityAdapter) hasExplicitProjectGrant(ctx context.Context, userID, organizationID, projectID int64) (bool, error) {
	var one int
	err := a.db.QueryRowContext(ctx, `
		SELECT 1
		WHERE EXISTS (
			SELECT 1
			FROM space_members sm
			WHERE sm.space_id = ?
			  AND (
				(sm.subject_type = 'user' AND sm.subject_id = ?)
				OR
				(sm.subject_type = 'group' AND sm.subject_id IN (
					SELECT gm.group_id FROM group_members gm WHERE gm.user_id = ?
				))
			  )
		)
		OR EXISTS (
			SELECT 1
			FROM role_bindings rb
			WHERE rb.organization_id = ?
			  AND rb.scope_type = 'space'
			  AND rb.scope_id = ?
			  AND (
				(rb.subject_type = 'user' AND rb.subject_id = ?)
				OR
				(rb.subject_type = 'group' AND rb.subject_id IN (
					SELECT gm.group_id FROM group_members gm WHERE gm.user_id = ?
				))
			  )
		)
	`, projectID, userID, userID, organizationID, projectID, userID, userID).Scan(&one)
	if errorsIsNoRows(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func errorsIsNoRows(err error) bool {
	return err == sql.ErrNoRows
}

// Check keeps legacy document authorization behavior while the remaining HTTP
// paths are migrated to application services. New Domain/Project code must use
// the organization-aware methods above.
func (a *SecurityAdapter) Check(ctx context.Context, u *domain.User, action domain.Action, res Resource) error {
	return a.fallback.Check(ctx, u, action, res)
}

var (
	_ application.DomainProjectSecurityPort = (*SecurityAdapter)(nil)
	_ application.DomainProjectScopePort    = (*SecurityAdapter)(nil)
	_ Authorizer                            = (*SecurityAdapter)(nil)
)
