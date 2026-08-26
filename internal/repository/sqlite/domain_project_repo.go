package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/homiakus/docshub-next/internal/db"
	"github.com/homiakus/docshub-next/internal/domain"
	"github.com/homiakus/docshub-next/internal/repository"
)

type DomainRepository struct{ db *db.DB }

type ProjectRepository struct{ db *db.DB }

func NewDomainRepository(d *db.DB) *DomainRepository { return &DomainRepository{db: d} }
func NewProjectRepository(d *db.DB) *ProjectRepository { return &ProjectRepository{db: d} }

func (r *DomainRepository) GetByID(ctx context.Context, id int64) (*domain.Domain, error) {
	return r.get(ctx, `WHERE id = ?`, id)
}

func (r *DomainRepository) GetByStableKey(ctx context.Context, stableKey string) (*domain.Domain, error) {
	return r.get(ctx, `WHERE stable_key = ?`, strings.TrimSpace(stableKey))
}

func (r *DomainRepository) GetBySlug(ctx context.Context, organizationID int64, slug string) (*domain.Domain, error) {
	return r.get(ctx, `WHERE organization_id = ? AND slug = ?`, organizationID, strings.TrimSpace(slug))
}

func (r *DomainRepository) ListByOrganization(ctx context.Context, organizationID int64, includeArchived bool) ([]domain.Domain, error) {
	query := domainSelect + ` WHERE organization_id = ?`
	if !includeArchived {
		query += ` AND status <> 'archived'`
	}
	query += ` ORDER BY sort_order, name, id`
	rows, err := r.db.QueryContext(ctx, query, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.Domain, 0)
	for rows.Next() {
		item, err := scanDomain(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (r *DomainRepository) Create(ctx context.Context, input repository.DomainCreateInput) (*domain.Domain, error) {
	input.StableKey = strings.TrimSpace(input.StableKey)
	input.SecurityWorkspaceID = strings.TrimSpace(input.SecurityWorkspaceID)
	input.Slug = strings.TrimSpace(input.Slug)
	input.Name = strings.TrimSpace(input.Name)
	input.CreatedBy = strings.TrimSpace(input.CreatedBy)
	if input.OrganizationID <= 0 || input.StableKey == "" || input.SecurityWorkspaceID == "" || input.Slug == "" || input.Name == "" {
		return nil, fmt.Errorf("domain repository: required create fields are missing")
	}
	if input.CreatedBy == "" {
		input.CreatedBy = "system"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO domains(
			stable_key, organization_id, security_workspace_id, slug, name,
			description, icon, status, sort_order, created_by, created_at, updated_at
		) VALUES(?,?,?,?,?,?,?,'active',?,?,?,?)`,
		input.StableKey, input.OrganizationID, input.SecurityWorkspaceID, input.Slug, input.Name,
		input.Description, input.Icon, input.SortOrder, input.CreatedBy, now, now)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return r.GetByID(ctx, id)
}

func (r *DomainRepository) UpdateMetadata(ctx context.Context, input repository.DomainUpdateInput) (*domain.Domain, error) {
	input.Slug = strings.TrimSpace(input.Slug)
	input.Name = strings.TrimSpace(input.Name)
	if input.ID <= 0 || input.Slug == "" || input.Name == "" || (input.Status != domain.DomainActive && input.Status != domain.DomainArchived) {
		return nil, fmt.Errorf("domain repository: invalid metadata update")
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE domains
		SET slug=?, name=?, description=?, icon=?, status=?, sort_order=?, updated_at=?
		WHERE id=?`,
		input.Slug, input.Name, input.Description, input.Icon, input.Status, input.SortOrder,
		time.Now().UTC().Format(time.RFC3339), input.ID)
	if err != nil {
		return nil, err
	}
	if err := requireAffected(res); err != nil {
		return nil, err
	}
	return r.GetByID(ctx, input.ID)
}

func (r *DomainRepository) BindSecurityWorkspace(ctx context.Context, id int64, workspaceID string) error {
	workspaceID = strings.TrimSpace(workspaceID)
	if id <= 0 || workspaceID == "" {
		return fmt.Errorf("domain repository: invalid security binding")
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE domains
		SET security_workspace_id=?, updated_at=?
		WHERE id=? AND (security_workspace_id IS NULL OR security_workspace_id='' OR security_workspace_id=?)`,
		workspaceID, time.Now().UTC().Format(time.RFC3339), id, workspaceID)
	if err != nil {
		return err
	}
	return requireAffected(res)
}

func (r *DomainRepository) get(ctx context.Context, where string, args ...any) (*domain.Domain, error) {
	return scanDomain(r.db.QueryRowContext(ctx, domainSelect+` `+where, args...))
}

func (r *ProjectRepository) GetByID(ctx context.Context, id int64) (*domain.Project, error) {
	return r.get(ctx, `WHERE s.id = ?`, id)
}

func (r *ProjectRepository) GetByStableKey(ctx context.Context, stableKey string) (*domain.Project, error) {
	return r.get(ctx, `WHERE s.stable_key = ?`, strings.TrimSpace(stableKey))
}

func (r *ProjectRepository) GetBySlug(ctx context.Context, domainID int64, slug string) (*domain.Project, error) {
	return r.get(ctx, `WHERE s.domain_id = ? AND s.slug = ?`, domainID, strings.TrimSpace(slug))
}

func (r *ProjectRepository) ListByDomain(ctx context.Context, domainID int64, includeArchived bool) ([]domain.Project, error) {
	query := projectSelect + ` WHERE s.domain_id = ?`
	if !includeArchived {
		query += ` AND s.status <> 'archived'`
	}
	query += ` ORDER BY s.sort_order, s.name, s.id`
	rows, err := r.db.QueryContext(ctx, query, domainID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.Project, 0)
	for rows.Next() {
		item, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (r *ProjectRepository) Create(ctx context.Context, input repository.ProjectCreateInput) (*domain.Project, error) {
	input.StableKey = strings.TrimSpace(input.StableKey)
	input.SecurityWorkspaceID = strings.TrimSpace(input.SecurityWorkspaceID)
	input.Slug = strings.TrimSpace(input.Slug)
	input.Name = strings.TrimSpace(input.Name)
	if input.OrganizationID <= 0 || input.DomainID <= 0 || input.StableKey == "" || input.SecurityWorkspaceID == "" || input.Slug == "" || input.Name == "" {
		return nil, fmt.Errorf("project repository: required create fields are missing")
	}
	if input.AccessMode == "" {
		input.AccessMode = domain.ProjectAccessInherit
	}
	if input.AccessMode != domain.ProjectAccessInherit && input.AccessMode != domain.ProjectAccessRestricted {
		return nil, fmt.Errorf("project repository: invalid access mode")
	}
	var domainOrganizationID int64
	if err := r.db.QueryRowContext(ctx, `SELECT organization_id FROM domains WHERE id=?`, input.DomainID).Scan(&domainOrganizationID); err != nil {
		return nil, err
	}
	if domainOrganizationID != input.OrganizationID {
		return nil, fmt.Errorf("project repository: domain organization mismatch")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO spaces(
			organization_id, parent_id, name, slug, description, default_visibility,
			created_at, updated_at, domain_id, stable_key, security_workspace_id,
			access_mode, status, sort_order
		) VALUES(?,NULL,?,?,?,'space_members',?,?,?,?,? ,?,'active',?)`,
		input.OrganizationID, input.Name, input.Slug, input.Description,
		now, now, input.DomainID, input.StableKey, input.SecurityWorkspaceID, input.AccessMode, input.SortOrder)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return r.GetByID(ctx, id)
}

func (r *ProjectRepository) UpdateMetadata(ctx context.Context, input repository.ProjectUpdateInput) (*domain.Project, error) {
	input.Slug = strings.TrimSpace(input.Slug)
	input.Name = strings.TrimSpace(input.Name)
	if input.ID <= 0 || input.Slug == "" || input.Name == "" || !validProjectStatus(input.Status) {
		return nil, fmt.Errorf("project repository: invalid metadata update")
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE spaces
		SET slug=?, name=?, description=?, status=?, sort_order=?, updated_at=?
		WHERE id=?`,
		input.Slug, input.Name, input.Description, input.Status, input.SortOrder,
		time.Now().UTC().Format(time.RFC3339), input.ID)
	if err != nil {
		return nil, err
	}
	if err := requireAffected(res); err != nil {
		return nil, err
	}
	return r.GetByID(ctx, input.ID)
}

func (r *ProjectRepository) SetAccessMode(ctx context.Context, id int64, mode domain.ProjectAccessMode) error {
	if id <= 0 || (mode != domain.ProjectAccessInherit && mode != domain.ProjectAccessRestricted) {
		return fmt.Errorf("project repository: invalid access mode update")
	}
	res, err := r.db.ExecContext(ctx, `UPDATE spaces SET access_mode=?, updated_at=? WHERE id=?`, mode, time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return err
	}
	return requireAffected(res)
}

func (r *ProjectRepository) BindSecurityWorkspace(ctx context.Context, id int64, workspaceID string) error {
	workspaceID = strings.TrimSpace(workspaceID)
	if id <= 0 || workspaceID == "" {
		return fmt.Errorf("project repository: invalid security binding")
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE spaces
		SET security_workspace_id=?, updated_at=?
		WHERE id=? AND (security_workspace_id IS NULL OR security_workspace_id='' OR security_workspace_id=?)`,
		workspaceID, time.Now().UTC().Format(time.RFC3339), id, workspaceID)
	if err != nil {
		return err
	}
	return requireAffected(res)
}

func (r *ProjectRepository) get(ctx context.Context, where string, args ...any) (*domain.Project, error) {
	return scanProject(r.db.QueryRowContext(ctx, projectSelect+` `+where, args...))
}

const domainSelect = `SELECT
	id, stable_key, organization_id, coalesce(security_workspace_id,''), slug,
	name, description, icon, status, sort_order, created_by, created_at, updated_at
FROM domains`

const projectSelect = `SELECT
	s.id, coalesce(s.stable_key,''), s.organization_id, coalesce(s.domain_id,0),
	coalesce(s.security_workspace_id,''), s.slug, s.name, s.description,
	s.status, s.access_mode, s.sort_order, s.created_at, s.updated_at
FROM spaces s`

type scanner interface{ Scan(dest ...any) error }

func scanDomain(row scanner) (*domain.Domain, error) {
	var item domain.Domain
	if err := row.Scan(
		&item.ID, &item.StableKey, &item.OrganizationID, &item.SecurityWorkspaceID,
		&item.Slug, &item.Name, &item.Description, &item.Icon, &item.Status,
		&item.SortOrder, &item.CreatedBy, &item.CreatedAt, &item.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &item, nil
}

func scanProject(row scanner) (*domain.Project, error) {
	var item domain.Project
	if err := row.Scan(
		&item.ID, &item.StableKey, &item.OrganizationID, &item.DomainID,
		&item.SecurityWorkspaceID, &item.Slug, &item.Name, &item.Description,
		&item.Status, &item.AccessMode, &item.SortOrder, &item.CreatedAt, &item.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &item, nil
}

func requireAffected(result sql.Result) error {
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func validProjectStatus(status domain.ProjectStatus) bool {
	return status == domain.ProjectActive || status == domain.ProjectPaused || status == domain.ProjectArchived
}

var (
	_ repository.DomainRepository  = (*DomainRepository)(nil)
	_ repository.ProjectRepository = (*ProjectRepository)(nil)
	_ = errors.Is
)
