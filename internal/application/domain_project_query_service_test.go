package application

import (
	"context"
	"errors"
	"testing"

	"github.com/homiakus/docshub-next/internal/domain"
	"github.com/homiakus/docshub-next/internal/repository"
)

func TestDomainProjectQueryServicePassesScopeIntoDomainQuery(t *testing.T) {
	events := make([]string, 0)
	domains := newFakeDomainRepository(&events)
	reader := &fakeScopedCatalog{}
	scope := &fakeScopePort{scope: repository.WorkspaceAccessScope{WorkspaceIDs: []string{" ws-domain ", "ws-domain", ""}}}
	svc := NewDomainProjectQueryService(domains, reader, reader, scope)

	_, err := svc.ListDomains(context.Background(), WorkspaceActor{UserID: 1, OrganizationID: 7}, false)
	if err != nil {
		t.Fatal(err)
	}
	if reader.domainOrgID != 7 {
		t.Fatalf("organization id = %d, want 7", reader.domainOrgID)
	}
	if len(reader.domainScope.WorkspaceIDs) != 1 || reader.domainScope.WorkspaceIDs[0] != "ws-domain" {
		t.Fatalf("scope = %#v, want one normalized workspace", reader.domainScope.WorkspaceIDs)
	}
}

func TestDomainProjectQueryServiceRejectsInaccessibleDomainBeforeProjectQuery(t *testing.T) {
	events := make([]string, 0)
	domains := newFakeDomainRepository(&events)
	domains.put(&domain.Domain{
		ID: 9, StableKey: "domain:hidden", OrganizationID: 1,
		SecurityWorkspaceID: "ws-hidden", Slug: "hidden", Name: "Hidden", Status: domain.DomainActive,
	})
	reader := &fakeScopedCatalog{}
	svc := NewDomainProjectQueryService(domains, reader, reader, &fakeScopePort{
		scope: repository.WorkspaceAccessScope{WorkspaceIDs: []string{"ws-other"}},
	})

	_, err := svc.ListProjects(context.Background(), WorkspaceActor{UserID: 1, OrganizationID: 1}, 9, false)
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
	if reader.projectCalls != 0 {
		t.Fatalf("project repository queried despite inaccessible parent")
	}
}

func TestDomainProjectQueryServiceScopesProjectsInsideAuthorizedDomain(t *testing.T) {
	events := make([]string, 0)
	domains := newFakeDomainRepository(&events)
	domains.put(&domain.Domain{
		ID: 3, StableKey: "domain:engineering", OrganizationID: 1,
		SecurityWorkspaceID: "ws-domain", Slug: "engineering", Name: "Engineering", Status: domain.DomainActive,
	})
	reader := &fakeScopedCatalog{}
	svc := NewDomainProjectQueryService(domains, reader, reader, &fakeScopePort{
		scope: repository.WorkspaceAccessScope{WorkspaceIDs: []string{"ws-domain", "ws-project-a"}},
	})

	_, err := svc.ListProjects(context.Background(), WorkspaceActor{UserID: 2, OrganizationID: 1}, 3, false)
	if err != nil {
		t.Fatal(err)
	}
	if reader.projectCalls != 1 || reader.projectDomainID != 3 {
		t.Fatalf("project calls=%d domain=%d", reader.projectCalls, reader.projectDomainID)
	}
	if len(reader.projectScope.WorkspaceIDs) != 2 {
		t.Fatalf("project scope = %#v", reader.projectScope.WorkspaceIDs)
	}
}

type fakeScopePort struct {
	scope repository.WorkspaceAccessScope
	err   error
}

func (f *fakeScopePort) ReadWorkspaceScope(context.Context, WorkspaceActor) (repository.WorkspaceAccessScope, error) {
	return f.scope, f.err
}

type fakeScopedCatalog struct {
	domainOrgID      int64
	domainScope      repository.WorkspaceAccessScope
	projectDomainID  int64
	projectScope     repository.WorkspaceAccessScope
	projectCalls     int
}

func (f *fakeScopedCatalog) ListByOrganizationScoped(_ context.Context, organizationID int64, _ bool, scope repository.WorkspaceAccessScope) ([]domain.Domain, error) {
	f.domainOrgID = organizationID
	f.domainScope = scope
	return []domain.Domain{}, nil
}

func (f *fakeScopedCatalog) ListByDomainScoped(_ context.Context, domainID int64, _ bool, scope repository.WorkspaceAccessScope) ([]domain.Project, error) {
	f.projectCalls++
	f.projectDomainID = domainID
	f.projectScope = scope
	return []domain.Project{}, nil
}
