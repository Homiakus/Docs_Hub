package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/homiakus/docshub-next/internal/db"
	"github.com/homiakus/docshub-next/internal/domain"
	"github.com/homiakus/docshub-next/internal/repository"
)

func openDomainProjectTestDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.Open(context.Background(), filepath.Join(t.TempDir(), "docshub.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func TestDomainProjectRepositoriesReadCompatibilityRows(t *testing.T) {
	ctx := context.Background()
	database := openDomainProjectTestDB(t)
	domains := NewDomainRepository(database)
	projects := NewProjectRepository(database)

	d, err := domains.GetByStableKey(ctx, "legacy-domain-1")
	if err != nil {
		t.Fatal(err)
	}
	if d.Slug != "general" || d.Status != domain.DomainActive {
		t.Fatalf("unexpected legacy domain: %#v", d)
	}

	p, err := projects.GetByStableKey(ctx, "legacy-project-1")
	if err != nil {
		t.Fatal(err)
	}
	if p.DomainID != d.ID || p.AccessMode != domain.ProjectAccessInherit || p.Status != domain.ProjectActive {
		t.Fatalf("unexpected legacy project: %#v", p)
	}
}

func TestDomainProjectRepositoriesCreateAndQuery(t *testing.T) {
	ctx := context.Background()
	database := openDomainProjectTestDB(t)
	domains := NewDomainRepository(database)
	projects := NewProjectRepository(database)

	d, err := domains.Create(ctx, repository.DomainCreateInput{
		StableKey:           "domain-engineering-immutable",
		OrganizationID:      1,
		SecurityWorkspaceID: "sa-domain-engineering",
		Slug:                "engineering",
		Name:                "Engineering",
		Description:         "Mechanical and electrical engineering",
		Icon:                "wrench",
		SortOrder:           10,
		CreatedBy:           "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.ID == 0 || d.StableKey != "domain-engineering-immutable" || d.SecurityWorkspaceID != "sa-domain-engineering" {
		t.Fatalf("unexpected domain: %#v", d)
	}

	p, err := projects.Create(ctx, repository.ProjectCreateInput{
		StableKey:           "project-hp4-immutable",
		OrganizationID:      1,
		DomainID:            d.ID,
		SecurityWorkspaceID: "sa-project-hp4",
		Slug:                "hp4",
		Name:                "HP4",
		Description:         "HP4 development",
		AccessMode:          domain.ProjectAccessInherit,
		SortOrder:           20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.DomainID != d.ID || p.SecurityWorkspaceID != "sa-project-hp4" {
		t.Fatalf("unexpected project: %#v", p)
	}

	bySlug, err := projects.GetBySlug(ctx, d.ID, "hp4")
	if err != nil || bySlug.ID != p.ID {
		t.Fatalf("project by slug: %#v err=%v", bySlug, err)
	}
	listed, err := projects.ListByDomain(ctx, d.ID, false)
	if err != nil || len(listed) != 1 || listed[0].ID != p.ID {
		t.Fatalf("project list: %#v err=%v", listed, err)
	}
}

func TestDomainProjectRepositoriesMetadataDoesNotChangeSecurityIdentity(t *testing.T) {
	ctx := context.Background()
	database := openDomainProjectTestDB(t)
	domains := NewDomainRepository(database)
	projects := NewProjectRepository(database)

	d, err := domains.Create(ctx, repository.DomainCreateInput{
		StableKey: "domain-regulatory", OrganizationID: 1, SecurityWorkspaceID: "sa-domain-regulatory",
		Slug: "regulatory", Name: "Regulatory", CreatedBy: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	updatedDomain, err := domains.UpdateMetadata(ctx, repository.DomainUpdateInput{
		ID: d.ID, Slug: "regulatory-affairs", Name: "Regulatory Affairs",
		Description: "Updated", Icon: "shield", Status: domain.DomainActive, SortOrder: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updatedDomain.StableKey != d.StableKey || updatedDomain.SecurityWorkspaceID != d.SecurityWorkspaceID {
		t.Fatalf("domain security identity changed: before=%#v after=%#v", d, updatedDomain)
	}

	p, err := projects.Create(ctx, repository.ProjectCreateInput{
		StableKey: "project-ivd", OrganizationID: 1, DomainID: d.ID, SecurityWorkspaceID: "sa-project-ivd",
		Slug: "ivd", Name: "IVD", AccessMode: domain.ProjectAccessInherit,
	})
	if err != nil {
		t.Fatal(err)
	}
	updatedProject, err := projects.UpdateMetadata(ctx, repository.ProjectUpdateInput{
		ID: p.ID, Slug: "ivd-platform", Name: "IVD Platform", Description: "Updated",
		Status: domain.ProjectPaused, SortOrder: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updatedProject.StableKey != p.StableKey || updatedProject.SecurityWorkspaceID != p.SecurityWorkspaceID || updatedProject.DomainID != p.DomainID {
		t.Fatalf("project security identity changed: before=%#v after=%#v", p, updatedProject)
	}
}

func TestDomainProjectRepositoriesBindingIsOneWay(t *testing.T) {
	ctx := context.Background()
	database := openDomainProjectTestDB(t)
	domains := NewDomainRepository(database)
	projects := NewProjectRepository(database)

	legacyDomain, err := domains.GetByStableKey(ctx, "legacy-domain-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := domains.BindSecurityWorkspace(ctx, legacyDomain.ID, "sa-general"); err != nil {
		t.Fatal(err)
	}
	if err := domains.BindSecurityWorkspace(ctx, legacyDomain.ID, "sa-general"); err != nil {
		t.Fatalf("idempotent domain bind failed: %v", err)
	}
	if err := domains.BindSecurityWorkspace(ctx, legacyDomain.ID, "sa-other"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("domain rebind error=%v want sql.ErrNoRows", err)
	}

	legacyProject, err := projects.GetByStableKey(ctx, "legacy-project-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := projects.BindSecurityWorkspace(ctx, legacyProject.ID, "sa-general-project"); err != nil {
		t.Fatal(err)
	}
	if err := projects.BindSecurityWorkspace(ctx, legacyProject.ID, "sa-other-project"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("project rebind error=%v want sql.ErrNoRows", err)
	}
}

func TestProjectRepositoryRejectsCrossOrganizationDomain(t *testing.T) {
	ctx := context.Background()
	database := openDomainProjectTestDB(t)
	projects := NewProjectRepository(database)

	if _, err := database.ExecContext(ctx, `INSERT INTO organizations(id,name,slug,settings_json,created_at) VALUES(2,'Other','other','{}',datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	if _, err := projects.Create(ctx, repository.ProjectCreateInput{
		StableKey: "project-invalid", OrganizationID: 2, DomainID: 1,
		SecurityWorkspaceID: "sa-project-invalid", Slug: "invalid", Name: "Invalid",
		AccessMode: domain.ProjectAccessInherit,
	}); err == nil {
		t.Fatal("cross-organization project creation unexpectedly succeeded")
	}
}

func TestProjectRepositoryAccessModeTransitionPersistence(t *testing.T) {
	ctx := context.Background()
	database := openDomainProjectTestDB(t)
	projects := NewProjectRepository(database)

	p, err := projects.GetByStableKey(ctx, "legacy-project-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := projects.SetAccessMode(ctx, p.ID, domain.ProjectAccessRestricted); err != nil {
		t.Fatal(err)
	}
	p, err = projects.GetByID(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if p.AccessMode != domain.ProjectAccessRestricted {
		t.Fatalf("access mode=%q want %q", p.AccessMode, domain.ProjectAccessRestricted)
	}
}
