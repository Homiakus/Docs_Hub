package application

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/homiakus/docshub-next/internal/domain"
	"github.com/homiakus/docshub-next/internal/repository"
)

func TestDomainProjectServiceCreateDomainSecurityFirstAndRetryIdempotent(t *testing.T) {
	ctx := context.Background()
	events := make([]string, 0)
	domains := newFakeDomainRepository(&events)
	projects := newFakeProjectRepository(&events)
	security := &fakeDomainProjectSecurity{events: &events}
	svc := NewDomainProjectService(domains, projects, security)
	actor := WorkspaceActor{UserID: 7, OrganizationID: 1}
	cmd := CreateDomainCommand{
		StableKey: "domain:engineering",
		OrganizationID: 1,
		Slug: "engineering",
		Name: "Engineering",
	}

	first, err := svc.CreateDomain(ctx, actor, cmd)
	if err != nil {
		t.Fatalf("CreateDomain(first): %v", err)
	}
	if first.SecurityWorkspaceID != "ws-domain:engineering" {
		t.Fatalf("unexpected workspace binding: %q", first.SecurityWorkspaceID)
	}
	if domains.creates != 1 {
		t.Fatalf("create count = %d, want 1", domains.creates)
	}
	if want := []string{"security:ensure-domain", "repo:create-domain"}; !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %#v, want %#v", events, want)
	}

	events = events[:0]
	security.events = &events
	domains.events = &events
	second, err := svc.CreateDomain(ctx, actor, cmd)
	if err != nil {
		t.Fatalf("CreateDomain(retry): %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("retry returned id %d, want %d", second.ID, first.ID)
	}
	if domains.creates != 1 {
		t.Fatalf("retry created duplicate; create count = %d", domains.creates)
	}
	if want := []string{"security:ensure-domain"}; !reflect.DeepEqual(events, want) {
		t.Fatalf("retry events = %#v, want %#v", events, want)
	}
}

func TestDomainProjectServiceCreateDomainDoesNotSwallowLookupFailure(t *testing.T) {
	events := make([]string, 0)
	domains := newFakeDomainRepository(&events)
	domains.getStableErr = errors.New("database offline")
	security := &fakeDomainProjectSecurity{events: &events}
	svc := NewDomainProjectService(domains, newFakeProjectRepository(&events), security)

	_, err := svc.CreateDomain(context.Background(), WorkspaceActor{UserID: 1, OrganizationID: 1}, CreateDomainCommand{
		StableKey: "domain:failure", OrganizationID: 1, Slug: "failure", Name: "Failure",
	})
	if err == nil || err.Error() != "database offline" {
		t.Fatalf("error = %v, want database offline", err)
	}
	if domains.creates != 0 {
		t.Fatalf("repository create must not run after lookup failure")
	}
}

func TestDomainProjectServiceCreateDomainReconcilesLegacyBinding(t *testing.T) {
	events := make([]string, 0)
	domains := newFakeDomainRepository(&events)
	domains.put(&domain.Domain{ID: 9, StableKey: "domain:legacy", OrganizationID: 1, Slug: "legacy", Name: "Legacy", Status: domain.DomainActive})
	security := &fakeDomainProjectSecurity{events: &events}
	svc := NewDomainProjectService(domains, newFakeProjectRepository(&events), security)

	got, err := svc.CreateDomain(context.Background(), WorkspaceActor{UserID: 2, OrganizationID: 1}, CreateDomainCommand{
		StableKey: "domain:legacy", OrganizationID: 1, Slug: "legacy", Name: "Legacy",
	})
	if err != nil {
		t.Fatalf("CreateDomain: %v", err)
	}
	if got.SecurityWorkspaceID != "ws-domain:legacy" {
		t.Fatalf("binding = %q", got.SecurityWorkspaceID)
	}
	if domains.binds != 1 || domains.creates != 0 {
		t.Fatalf("binds=%d creates=%d, want 1/0", domains.binds, domains.creates)
	}
}

func TestDomainProjectServiceCreateProjectRequiresBoundParent(t *testing.T) {
	events := make([]string, 0)
	domains := newFakeDomainRepository(&events)
	domains.put(&domain.Domain{ID: 4, StableKey: "domain:unbound", OrganizationID: 1, Slug: "unbound", Name: "Unbound", Status: domain.DomainActive})
	security := &fakeDomainProjectSecurity{events: &events}
	svc := NewDomainProjectService(domains, newFakeProjectRepository(&events), security)

	_, err := svc.CreateProject(context.Background(), WorkspaceActor{UserID: 3, OrganizationID: 1}, CreateProjectCommand{
		StableKey: "project:blocked", OrganizationID: 1, DomainID: 4, Slug: "blocked", Name: "Blocked",
	})
	if !errors.Is(err, ErrSecurityBinding) {
		t.Fatalf("error = %v, want ErrSecurityBinding", err)
	}
	if len(events) != 0 {
		t.Fatalf("security must not be called for an unbound parent: %#v", events)
	}
}

func TestDomainProjectServiceUpdateProjectAuthorizesBeforeWrite(t *testing.T) {
	events := make([]string, 0)
	domains := newFakeDomainRepository(&events)
	projects := newFakeProjectRepository(&events)
	projects.put(&domain.Project{
		ID: 12, StableKey: "project:update", OrganizationID: 1, DomainID: 2,
		SecurityWorkspaceID: "ws-project", Slug: "old", Name: "Old",
		Status: domain.ProjectActive, AccessMode: domain.ProjectAccessInherit,
	})
	security := &fakeDomainProjectSecurity{events: &events}
	svc := NewDomainProjectService(domains, projects, security)

	got, err := svc.UpdateProject(context.Background(), WorkspaceActor{UserID: 4, OrganizationID: 1}, UpdateProjectCommand{
		ID: 12, Slug: "new", Name: "New", Status: domain.ProjectActive,
	})
	if err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}
	if got.Name != "New" {
		t.Fatalf("name = %q", got.Name)
	}
	want := []string{"security:require:manage_workspace", "repo:update-project"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %#v, want %#v", events, want)
	}
}

func TestDomainProjectServiceSetProjectAccessModeSecurityFirst(t *testing.T) {
	events := make([]string, 0)
	projects := newFakeProjectRepository(&events)
	projects.put(&domain.Project{
		ID: 21, StableKey: "project:mode", OrganizationID: 1, DomainID: 2,
		SecurityWorkspaceID: "ws-mode", Slug: "mode", Name: "Mode",
		Status: domain.ProjectActive, AccessMode: domain.ProjectAccessInherit,
	})
	security := &fakeDomainProjectSecurity{events: &events}
	svc := NewDomainProjectService(newFakeDomainRepository(&events), projects, security)

	got, err := svc.SetProjectAccessMode(context.Background(), WorkspaceActor{UserID: 5, OrganizationID: 1}, 21, domain.ProjectAccessRestricted)
	if err != nil {
		t.Fatalf("SetProjectAccessMode: %v", err)
	}
	if got.AccessMode != domain.ProjectAccessRestricted {
		t.Fatalf("mode = %q", got.AccessMode)
	}
	want := []string{"security:set-mode:restricted", "repo:set-mode:restricted"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %#v, want %#v", events, want)
	}
}

func TestDomainProjectServiceGetProjectHidesCrossOrganizationExistence(t *testing.T) {
	events := make([]string, 0)
	projects := newFakeProjectRepository(&events)
	projects.put(&domain.Project{
		ID: 30, StableKey: "project:other", OrganizationID: 2, DomainID: 3,
		SecurityWorkspaceID: "ws-other", Slug: "other", Name: "Other",
		Status: domain.ProjectActive, AccessMode: domain.ProjectAccessInherit,
	})
	security := &fakeDomainProjectSecurity{events: &events}
	svc := NewDomainProjectService(newFakeDomainRepository(&events), projects, security)

	_, err := svc.GetProject(context.Background(), WorkspaceActor{UserID: 5, OrganizationID: 1}, 30)
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("error = %v, want repository.ErrNotFound", err)
	}
	if len(events) != 0 {
		t.Fatalf("security call would reveal cross-org existence: %#v", events)
	}
}

type fakeDomainProjectSecurity struct {
	events *[]string
	err    error
}

func (f *fakeDomainProjectSecurity) add(event string) {
	if f.events != nil {
		*f.events = append(*f.events, event)
	}
}

func (f *fakeDomainProjectSecurity) EnsureDomainWorkspace(_ context.Context, _ WorkspaceActor, req DomainWorkspaceRequest) (string, error) {
	f.add("security:ensure-domain")
	if f.err != nil {
		return "", f.err
	}
	return "ws-" + req.StableKey, nil
}

func (f *fakeDomainProjectSecurity) EnsureProjectWorkspace(_ context.Context, _ WorkspaceActor, req ProjectWorkspaceRequest) (string, error) {
	f.add("security:ensure-project")
	if f.err != nil {
		return "", f.err
	}
	return "ws-" + req.StableKey, nil
}

func (f *fakeDomainProjectSecurity) RequireWorkspacePermission(_ context.Context, _ WorkspaceActor, _ string, permission WorkspacePermission) error {
	f.add("security:require:" + string(permission))
	return f.err
}

func (f *fakeDomainProjectSecurity) SetProjectAccessMode(_ context.Context, _ WorkspaceActor, _ string, mode domain.ProjectAccessMode) error {
	f.add("security:set-mode:" + string(mode))
	return f.err
}

type fakeDomainRepository struct {
	byID         map[int64]*domain.Domain
	byStable     map[string]*domain.Domain
	nextID       int64
	creates      int
	binds        int
	getStableErr error
	events       *[]string
}

func newFakeDomainRepository(events *[]string) *fakeDomainRepository {
	return &fakeDomainRepository{byID: map[int64]*domain.Domain{}, byStable: map[string]*domain.Domain{}, nextID: 100, events: events}
}

func (f *fakeDomainRepository) add(event string) {
	if f.events != nil {
		*f.events = append(*f.events, event)
	}
}

func (f *fakeDomainRepository) put(item *domain.Domain) {
	copy := *item
	f.byID[copy.ID] = &copy
	f.byStable[copy.StableKey] = &copy
	if copy.ID >= f.nextID {
		f.nextID = copy.ID + 1
	}
}

func (f *fakeDomainRepository) GetByID(_ context.Context, id int64) (*domain.Domain, error) {
	item := f.byID[id]
	if item == nil {
		return nil, repository.ErrNotFound
	}
	copy := *item
	return &copy, nil
}

func (f *fakeDomainRepository) GetByStableKey(_ context.Context, key string) (*domain.Domain, error) {
	if f.getStableErr != nil {
		return nil, f.getStableErr
	}
	item := f.byStable[key]
	if item == nil {
		return nil, repository.ErrNotFound
	}
	copy := *item
	return &copy, nil
}

func (f *fakeDomainRepository) GetBySlug(_ context.Context, organizationID int64, slug string) (*domain.Domain, error) {
	for _, item := range f.byID {
		if item.OrganizationID == organizationID && item.Slug == slug {
			copy := *item
			return &copy, nil
		}
	}
	return nil, repository.ErrNotFound
}

func (f *fakeDomainRepository) ListByOrganization(_ context.Context, organizationID int64, includeArchived bool) ([]domain.Domain, error) {
	out := make([]domain.Domain, 0)
	for _, item := range f.byID {
		if item.OrganizationID == organizationID && (includeArchived || item.Status != domain.DomainArchived) {
			out = append(out, *item)
		}
	}
	return out, nil
}

func (f *fakeDomainRepository) Create(_ context.Context, input repository.DomainCreateInput) (*domain.Domain, error) {
	f.add("repo:create-domain")
	f.creates++
	if _, exists := f.byStable[input.StableKey]; exists {
		return nil, ErrResourceConflict
	}
	id := f.nextID
	f.nextID++
	item := &domain.Domain{
		ID: id, StableKey: input.StableKey, OrganizationID: input.OrganizationID,
		SecurityWorkspaceID: input.SecurityWorkspaceID, Slug: input.Slug, Name: input.Name,
		Description: input.Description, Icon: input.Icon, Status: domain.DomainActive,
		SortOrder: input.SortOrder, CreatedBy: input.CreatedBy,
	}
	f.put(item)
	return f.GetByID(context.Background(), id)
}

func (f *fakeDomainRepository) UpdateMetadata(_ context.Context, input repository.DomainUpdateInput) (*domain.Domain, error) {
	f.add("repo:update-domain")
	item := f.byID[input.ID]
	if item == nil {
		return nil, repository.ErrNotFound
	}
	item.Slug, item.Name, item.Description, item.Icon = input.Slug, input.Name, input.Description, input.Icon
	item.Status, item.SortOrder = input.Status, input.SortOrder
	return f.GetByID(context.Background(), input.ID)
}

func (f *fakeDomainRepository) BindSecurityWorkspace(_ context.Context, id int64, workspaceID string) error {
	item := f.byID[id]
	if item == nil {
		return repository.ErrNotFound
	}
	if item.SecurityWorkspaceID != "" && item.SecurityWorkspaceID != workspaceID {
		return ErrResourceConflict
	}
	f.binds++
	item.SecurityWorkspaceID = workspaceID
	return nil
}

type fakeProjectRepository struct {
	byID     map[int64]*domain.Project
	byStable map[string]*domain.Project
	nextID   int64
	creates  int
	binds    int
	events   *[]string
}

func newFakeProjectRepository(events *[]string) *fakeProjectRepository {
	return &fakeProjectRepository{byID: map[int64]*domain.Project{}, byStable: map[string]*domain.Project{}, nextID: 200, events: events}
}

func (f *fakeProjectRepository) add(event string) {
	if f.events != nil {
		*f.events = append(*f.events, event)
	}
}

func (f *fakeProjectRepository) put(item *domain.Project) {
	copy := *item
	f.byID[copy.ID] = &copy
	f.byStable[copy.StableKey] = &copy
	if copy.ID >= f.nextID {
		f.nextID = copy.ID + 1
	}
}

func (f *fakeProjectRepository) GetByID(_ context.Context, id int64) (*domain.Project, error) {
	item := f.byID[id]
	if item == nil {
		return nil, repository.ErrNotFound
	}
	copy := *item
	return &copy, nil
}

func (f *fakeProjectRepository) GetByStableKey(_ context.Context, key string) (*domain.Project, error) {
	item := f.byStable[key]
	if item == nil {
		return nil, repository.ErrNotFound
	}
	copy := *item
	return &copy, nil
}

func (f *fakeProjectRepository) GetBySlug(_ context.Context, domainID int64, slug string) (*domain.Project, error) {
	for _, item := range f.byID {
		if item.DomainID == domainID && item.Slug == slug {
			copy := *item
			return &copy, nil
		}
	}
	return nil, repository.ErrNotFound
}

func (f *fakeProjectRepository) ListByDomain(_ context.Context, domainID int64, includeArchived bool) ([]domain.Project, error) {
	out := make([]domain.Project, 0)
	for _, item := range f.byID {
		if item.DomainID == domainID && (includeArchived || item.Status != domain.ProjectArchived) {
			out = append(out, *item)
		}
	}
	return out, nil
}

func (f *fakeProjectRepository) Create(_ context.Context, input repository.ProjectCreateInput) (*domain.Project, error) {
	f.add("repo:create-project")
	f.creates++
	if _, exists := f.byStable[input.StableKey]; exists {
		return nil, ErrResourceConflict
	}
	id := f.nextID
	f.nextID++
	item := &domain.Project{
		ID: id, StableKey: input.StableKey, OrganizationID: input.OrganizationID, DomainID: input.DomainID,
		SecurityWorkspaceID: input.SecurityWorkspaceID, Slug: input.Slug, Name: input.Name,
		Description: input.Description, Status: domain.ProjectActive, AccessMode: input.AccessMode,
		SortOrder: input.SortOrder,
	}
	f.put(item)
	return f.GetByID(context.Background(), id)
}

func (f *fakeProjectRepository) UpdateMetadata(_ context.Context, input repository.ProjectUpdateInput) (*domain.Project, error) {
	f.add("repo:update-project")
	item := f.byID[input.ID]
	if item == nil {
		return nil, repository.ErrNotFound
	}
	item.Slug, item.Name, item.Description = input.Slug, input.Name, input.Description
	item.Status, item.SortOrder = input.Status, input.SortOrder
	return f.GetByID(context.Background(), input.ID)
}

func (f *fakeProjectRepository) SetAccessMode(_ context.Context, id int64, mode domain.ProjectAccessMode) error {
	f.add("repo:set-mode:" + string(mode))
	item := f.byID[id]
	if item == nil {
		return repository.ErrNotFound
	}
	item.AccessMode = mode
	return nil
}

func (f *fakeProjectRepository) BindSecurityWorkspace(_ context.Context, id int64, workspaceID string) error {
	item := f.byID[id]
	if item == nil {
		return repository.ErrNotFound
	}
	if item.SecurityWorkspaceID != "" && item.SecurityWorkspaceID != workspaceID {
		return ErrResourceConflict
	}
	f.binds++
	item.SecurityWorkspaceID = workspaceID
	return nil
}
