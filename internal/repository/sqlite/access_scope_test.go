package sqlite

import (
	"context"
	"testing"

	"github.com/homiakus/docshub-next/internal/domain"
	"github.com/homiakus/docshub-next/internal/repository"
)

func TestScopedDomainListFailsClosedAndReturnsOnlyAuthorizedBindings(t *testing.T) {
	ctx := context.Background()
	database := openDomainProjectTestDB(t)
	domains := NewDomainRepository(database)

	visible, err := domains.Create(ctx, repository.DomainCreateInput{
		StableKey: "domain-visible", OrganizationID: 1, SecurityWorkspaceID: "ws-visible",
		Slug: "visible", Name: "Visible", CreatedBy: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = domains.Create(ctx, repository.DomainCreateInput{
		StableKey: "domain-hidden", OrganizationID: 1, SecurityWorkspaceID: "ws-hidden",
		Slug: "hidden", Name: "Hidden", CreatedBy: "test",
	})
	if err != nil {
		t.Fatal(err)
	}

	empty, err := domains.ListByOrganizationScoped(ctx, 1, false, repository.WorkspaceAccessScope{})
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Fatalf("empty scope leaked %d domains", len(empty))
	}

	got, err := domains.ListByOrganizationScoped(ctx, 1, false, repository.WorkspaceAccessScope{
		WorkspaceIDs: []string{" ws-visible ", "ws-visible", ""},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != visible.ID {
		t.Fatalf("scoped domains = %#v, want only %#v", got, visible)
	}
}

func TestScopedProjectListAppliesWorkspacePredicateBeforeCatalogResult(t *testing.T) {
	ctx := context.Background()
	database := openDomainProjectTestDB(t)
	domains := NewDomainRepository(database)
	projects := NewProjectRepository(database)

	parent, err := domains.Create(ctx, repository.DomainCreateInput{
		StableKey: "domain-scope-parent", OrganizationID: 1, SecurityWorkspaceID: "ws-parent",
		Slug: "scope-parent", Name: "Scope Parent", CreatedBy: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	allowed, err := projects.Create(ctx, repository.ProjectCreateInput{
		StableKey: "project-allowed", OrganizationID: 1, DomainID: parent.ID,
		SecurityWorkspaceID: "ws-project-allowed", Slug: "allowed", Name: "Allowed",
		AccessMode: domain.ProjectAccessInherit,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = projects.Create(ctx, repository.ProjectCreateInput{
		StableKey: "project-denied", OrganizationID: 1, DomainID: parent.ID,
		SecurityWorkspaceID: "ws-project-denied", Slug: "denied", Name: "Denied",
		AccessMode: domain.ProjectAccessRestricted,
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := projects.ListByDomainScoped(ctx, parent.ID, false, repository.WorkspaceAccessScope{
		WorkspaceIDs: []string{"ws-parent", "ws-project-allowed"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != allowed.ID {
		t.Fatalf("scoped projects = %#v, want only %#v", got, allowed)
	}
}
