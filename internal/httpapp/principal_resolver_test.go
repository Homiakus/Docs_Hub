package httpapp

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/homiakus/docshub-next/internal/db"
)

func TestResolveWorkspaceActorSingleOrganizationCompatibility(t *testing.T) {
	ctx := context.Background()
	database := openPrincipalTestDB(t)
	server := &Server{db: database}

	actor := server.resolveWorkspaceActor(ctx, 41)
	if actor.UserID != 41 || actor.OrganizationID != 1 {
		t.Fatalf("single-org actor=%+v, want user=41 org=1", actor)
	}
}

func TestResolveWorkspaceActorRequiresExplicitMembershipInMultiOrg(t *testing.T) {
	ctx := context.Background()
	database := openPrincipalTestDB(t)
	server := &Server{db: database}

	if _, err := database.ExecContext(ctx, `
		INSERT INTO organizations(id,name,slug,settings_json,created_at)
		VALUES(2,'Other','other','{}',datetime('now'))
	`); err != nil {
		t.Fatal(err)
	}
	if actor := server.resolveWorkspaceActor(ctx, 41); actor.OrganizationID != 0 {
		t.Fatalf("actor without explicit multi-org membership must fail closed: %+v", actor)
	}

	if _, err := database.ExecContext(ctx, `
		INSERT INTO role_bindings(organization_id,scope_type,scope_id,subject_type,subject_id,role,created_at)
		VALUES(2,'organization',2,'user',41,'reader',datetime('now'))
	`); err != nil {
		t.Fatal(err)
	}
	actor := server.resolveWorkspaceActor(ctx, 41)
	if actor.UserID != 41 || actor.OrganizationID != 2 {
		t.Fatalf("explicit membership actor=%+v, want user=41 org=2", actor)
	}

	if _, err := database.ExecContext(ctx, `
		INSERT INTO role_bindings(organization_id,scope_type,scope_id,subject_type,subject_id,role,created_at)
		VALUES(1,'organization',1,'user',41,'reader',datetime('now'))
	`); err != nil {
		t.Fatal(err)
	}
	if actor := server.resolveWorkspaceActor(ctx, 41); actor.OrganizationID != 0 {
		t.Fatalf("multiple memberships require explicit session selection, got %+v", actor)
	}
}

func openPrincipalTestDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.Open(context.Background(), filepath.Join(t.TempDir(), "principal.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}
