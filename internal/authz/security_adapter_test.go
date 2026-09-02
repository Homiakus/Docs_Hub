package authz

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/homiakus/docshub-next/internal/application"
	"github.com/homiakus/docshub-next/internal/db"
	"github.com/homiakus/docshub-next/internal/domain"
)

func setupTestDB(t *testing.T) *db.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test_authz.db")
	database, err := db.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	_, err = database.ExecContext(context.Background(), `
		INSERT OR IGNORE INTO users(id, username, display_name, email, password_hash, role, is_active, created_at, updated_at)
		VALUES(1, 'admin', 'Administrator', 'admin@example.com', 'hash', 'admin', 1, datetime('now'), datetime('now'))
	`)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return database
}

func TestSecurityAdapterSingleOrganizationCompatibility(t *testing.T) {
	ctx := context.Background()
	database := setupTestDB(t)
	adapter := NewSecurityAdapter(database)
	actor := application.WorkspaceActor{UserID: 1, OrganizationID: 1}

	domWS, err := adapter.EnsureDomainWorkspace(ctx, actor, application.DomainWorkspaceRequest{
		OrganizationID: 1,
		StableKey:      "engineering",
		Name:           "Engineering",
	})
	if err != nil {
		t.Fatalf("ensure domain workspace: %v", err)
	}
	if domWS != "ws_dom_1_engineering" {
		t.Fatalf("unexpected domain workspace id: %s", domWS)
	}

	_, err = adapter.EnsureDomainWorkspace(ctx, application.WorkspaceActor{UserID: 1, OrganizationID: 2}, application.DomainWorkspaceRequest{
		OrganizationID: 1,
		StableKey:      "engineering",
		Name:           "Engineering",
	})
	if !errors.Is(err, application.ErrOrganizationBoundary) {
		t.Fatalf("expected ErrOrganizationBoundary, got %v", err)
	}
}

func TestGlobalAdminDoesNotBypassOrganizationMembership(t *testing.T) {
	ctx := context.Background()
	database := setupTestDB(t)
	adapter := NewSecurityAdapter(database)

	if _, err := database.ExecContext(ctx, `
		INSERT INTO organizations(id,name,slug,settings_json,created_at)
		VALUES(2,'Other Organization','other','{}',datetime('now'))
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO domains(stable_key,organization_id,security_workspace_id,slug,name,description,icon,status,sort_order,created_by,created_at,updated_at)
		VALUES('other-domain',2,'ws_dom_2_other','other','Other','','','active',0,'test',datetime('now'),datetime('now'))
	`); err != nil {
		t.Fatal(err)
	}

	actor := application.WorkspaceActor{UserID: 1, OrganizationID: 2}
	if err := adapter.RequireWorkspacePermission(ctx, actor, "ws_dom_2_other", application.WorkspacePermissionRead); !errors.Is(err, ErrForbidden) {
		t.Fatalf("global admin without org membership must be denied, got %v", err)
	}

	if _, err := database.ExecContext(ctx, `
		INSERT INTO role_bindings(organization_id,scope_type,scope_id,subject_type,subject_id,role,created_at)
		VALUES(2,'organization',2,'user',1,'admin',datetime('now'))
	`); err != nil {
		t.Fatal(err)
	}
	if err := adapter.RequireWorkspacePermission(ctx, actor, "ws_dom_2_other", application.WorkspacePermissionRead); err != nil {
		t.Fatalf("explicit organization member should be allowed: %v", err)
	}
}

func TestRestrictedProjectRequiresExplicitGrantAndSurvivesRestart(t *testing.T) {
	ctx := context.Background()
	database := setupTestDB(t)

	if _, err := database.ExecContext(ctx, `
		INSERT INTO users(id, username, display_name, email, password_hash, role, is_active, created_at, updated_at)
		VALUES(2, 'editor', 'Editor', 'editor@example.com', 'hash', 'editor', 1, datetime('now'), datetime('now'))
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `
		UPDATE domains SET security_workspace_id='ws_dom_1_general' WHERE organization_id=1;
		UPDATE spaces SET security_workspace_id='ws_prj_1_general', access_mode='restricted'
		WHERE id=1;
	`); err != nil {
		t.Fatal(err)
	}

	actor := application.WorkspaceActor{UserID: 2, OrganizationID: 1}
	adapter := NewSecurityAdapter(database)

	if err := adapter.RequireWorkspacePermission(ctx, actor, "ws_prj_1_general", application.WorkspacePermissionRead); !errors.Is(err, ErrForbidden) {
		t.Fatalf("restricted project without explicit grant must be denied, got %v", err)
	}
	scope, err := adapter.ReadWorkspaceScope(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	if workspaceScopeContains(scope.WorkspaceIDs, "ws_prj_1_general") {
		t.Fatalf("restricted project leaked into scope without explicit grant: %v", scope.WorkspaceIDs)
	}
	if !workspaceScopeContains(scope.WorkspaceIDs, "ws_dom_1_general") {
		t.Fatalf("domain should remain visible in inherited organization scope: %v", scope.WorkspaceIDs)
	}

	if _, err := database.ExecContext(ctx, `
		INSERT INTO space_members(space_id,subject_type,subject_id,role)
		VALUES(1,'user',2,'editor')
	`); err != nil {
		t.Fatal(err)
	}
	if err := adapter.RequireWorkspacePermission(ctx, actor, "ws_prj_1_general", application.WorkspacePermissionRead); err != nil {
		t.Fatalf("explicit project member should read restricted project: %v", err)
	}

	if err := adapter.SetProjectAccessMode(ctx, actor, "ws_prj_1_general", domain.ProjectAccessInherit); err != nil {
		t.Fatalf("set project mode: %v", err)
	}
	var mode string
	if err := database.QueryRowContext(ctx, `SELECT access_mode FROM spaces WHERE id=1`).Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != string(domain.ProjectAccessInherit) {
		t.Fatalf("persisted access mode=%q, want inherit", mode)
	}

	// A new adapter instance must observe the same durable boundary; no
	// process-local access-mode state is allowed to affect authorization.
	restarted := NewSecurityAdapter(database)
	scope, err = restarted.ReadWorkspaceScope(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	if !workspaceScopeContains(scope.WorkspaceIDs, "ws_prj_1_general") {
		t.Fatalf("persisted inherit mode not observed after adapter restart: %v", scope.WorkspaceIDs)
	}
}

func workspaceScopeContains(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
