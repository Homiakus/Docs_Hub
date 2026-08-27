package authz

import (
	"context"
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

func TestSecurityAdapterWorkspaceLifecycle(t *testing.T) {
	ctx := context.Background()
	database := setupTestDB(t)
	adapter := NewSecurityAdapter(database)

	actor := application.WorkspaceActor{UserID: 1, OrganizationID: 1}

	// 1. Ensure Domain Workspace
	domReq := application.DomainWorkspaceRequest{
		OrganizationID: 1,
		StableKey:      "engineering",
		Name:           "Engineering",
	}
	domWS, err := adapter.EnsureDomainWorkspace(ctx, actor, domReq)
	if err != nil {
		t.Fatalf("ensure domain workspace: %v", err)
	}
	if domWS != "ws_dom_1_engineering" {
		t.Fatalf("unexpected domain workspace id: %s", domWS)
	}

	// Cross-organization rejection
	_, err = adapter.EnsureDomainWorkspace(ctx, application.WorkspaceActor{UserID: 1, OrganizationID: 2}, domReq)
	if err != application.ErrOrganizationBoundary {
		t.Fatalf("expected ErrOrganizationBoundary, got %v", err)
	}

	// 2. Ensure Project Workspace
	prjReq := application.ProjectWorkspaceRequest{
		OrganizationID:    1,
		ParentWorkspaceID: domWS,
		StableKey:         "backend-core",
		Name:              "Backend Core",
		AccessMode:        domain.ProjectAccessInherit,
	}
	prjWS, err := adapter.EnsureProjectWorkspace(ctx, actor, prjReq)
	if err != nil {
		t.Fatalf("ensure project workspace: %v", err)
	}
	if prjWS != "ws_prj_1_backend-core" {
		t.Fatalf("unexpected project workspace id: %s", prjWS)
	}

	// 3. Permissions & Access Mode
	if err := adapter.SetProjectAccessMode(ctx, actor, prjWS, domain.ProjectAccessRestricted); err != nil {
		t.Fatalf("set project access mode: %v", err)
	}

	// 4. Read Scope
	scope, err := adapter.ReadWorkspaceScope(ctx, actor)
	if err != nil {
		t.Fatalf("read workspace scope: %v", err)
	}
	_ = scope
}
