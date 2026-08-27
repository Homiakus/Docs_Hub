package domain

import "html/template"

type Action string

const (
	ActionRead      Action = "read"
	ActionCreate    Action = "create"
	ActionEdit      Action = "edit"
	ActionComment   Action = "comment"
	ActionReview    Action = "review"
	ActionPublish   Action = "publish"
	ActionManageACL Action = "manage_acl"
	ActionArchive   Action = "archive"
	ActionDelete    Action = "delete"
	ActionExport    Action = "export"
)

type WorkflowStatus string

const (
	StatusDraft     WorkflowStatus = "draft"
	StatusInReview  WorkflowStatus = "in_review"
	StatusApproved  WorkflowStatus = "approved"
	StatusPublished WorkflowStatus = "published"
	StatusArchived  WorkflowStatus = "archived"
	StatusRejected  WorkflowStatus = "rejected"
)

type Organization struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	Settings  string `json:"settings_json"`
	CreatedAt string `json:"created_at"`
}

type DomainStatus string

const (
	DomainActive   DomainStatus = "active"
	DomainArchived DomainStatus = "archived"
)

type ProjectStatus string

const (
	ProjectActive   ProjectStatus = "active"
	ProjectPaused   ProjectStatus = "paused"
	ProjectArchived ProjectStatus = "archived"
)

type ProjectAccessMode string

const (
	ProjectAccessInherit    ProjectAccessMode = "inherit"
	ProjectAccessRestricted ProjectAccessMode = "restricted"
)

// Domain is the primary product navigation and authorization boundary. StableKey
// is immutable host identity; SecurityWorkspaceID is populated only after the
// corresponding SecureAccess workspace has been provisioned and reconciled.
type Domain struct {
	ID                  int64        `json:"id"`
	StableKey           string       `json:"stable_key"`
	OrganizationID      int64        `json:"organization_id"`
	SecurityWorkspaceID string       `json:"security_workspace_id,omitempty"`
	Slug                string       `json:"slug"`
	Name                string       `json:"name"`
	Description         string       `json:"description"`
	Icon                string       `json:"icon"`
	Status              DomainStatus `json:"status"`
	SortOrder           int          `json:"sort_order"`
	CreatedBy           string       `json:"created_by"`
	CreatedAt           string       `json:"created_at"`
	UpdatedAt           string       `json:"updated_at"`
}

// Project is the product model that maps to the legacy physical `spaces` table
// during migration M1/M2. New application code should use Project rather than
// adding new dependencies on Space.
type Project struct {
	ID                  int64             `json:"id"`
	StableKey           string            `json:"stable_key"`
	OrganizationID      int64             `json:"organization_id"`
	DomainID            int64             `json:"domain_id"`
	SecurityWorkspaceID string            `json:"security_workspace_id,omitempty"`
	Slug                string            `json:"slug"`
	Name                string            `json:"name"`
	Description         string            `json:"description"`
	Status              ProjectStatus     `json:"status"`
	AccessMode          ProjectAccessMode `json:"access_mode"`
	SortOrder           int               `json:"sort_order"`
	CreatedAt           string            `json:"created_at"`
	UpdatedAt           string            `json:"updated_at"`
}

// Space is retained only as a compatibility model while production handlers
// still read the physical `spaces` table. It is removed after M2 convergence.
type Space struct {
	ID                int64  `json:"id"`
	OrganizationID    int64  `json:"organization_id"`
	ParentID          *int64 `json:"parent_id,omitempty"`
	Name              string `json:"name"`
	Slug              string `json:"slug"`
	Description       string `json:"description"`
	OwnerGroupID      *int64 `json:"owner_group_id,omitempty"`
	DefaultVisibility string `json:"default_visibility"`
	CreatedAt         string `json:"created_at"`
	UpdatedAt         string `json:"updated_at"`
}

type SpaceMember struct {
	SpaceID     int64  `json:"space_id"`
	SubjectType string `json:"subject_type"`
	SubjectID   int64  `json:"subject_id"`
	Role        string `json:"role"`
}

type DocumentPermission struct {
	DocumentID  int64  `json:"document_id"`
	SubjectType string `json:"subject_type"`
	SubjectID   int64  `json:"subject_id"`
	Permission  string `json:"permission"`
	Effect      string `json:"effect"`
}

type DocumentRevision struct {
	ID            int64  `json:"id"`
	DocumentID    int64  `json:"document_id"`
	RevisionNo    int    `json:"revision_no"`
	SourceFormat  string `json:"source_format"`
	Content       string `json:"content"`
	RenderedHTML  string `json:"rendered_html"`
	MetadataJSON  string `json:"metadata_json"`
	ChangeSummary string `json:"change_summary"`
	CreatedBy     int64  `json:"created_by"`
	CreatedAt     string `json:"created_at"`
}

type DocumentReview struct {
	ID         int64  `json:"id"`
	DocumentID int64  `json:"document_id"`
	RevisionID int64  `json:"revision_id"`
	ReviewerID int64  `json:"reviewer_id"`
	Status     string `json:"status"`
	Comment    string `json:"comment"`
	DecidedAt  string `json:"decided_at,omitempty"`
}

type User struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email,omitempty"`
	Role        string `json:"role"`
	Active      bool   `json:"active"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

type Heading struct {
	Level int    `json:"level"`
	Text  string `json:"text"`
	ID    string `json:"id"`
}

type Article struct {
	ID             int64          `json:"id"`
	OrganizationID int64          `json:"organization_id"`
	SpaceID        int64          `json:"space_id"`
	StableKey      string         `json:"stable_key"`
	Slug           string         `json:"slug"`
	Title          string         `json:"title"`
	Status         WorkflowStatus `json:"status"`
	Classification string         `json:"classification"`
	Language       string         `json:"language"`
	LockVersion    int            `json:"lock_version"`
	Content        string         `json:"content"`
	HTML           template.HTML  `json:"html"`
	Visibility     string         `json:"visibility"`
	CategoryID     int64          `json:"category_id"`
	Category       string         `json:"category"`
	UpdatedAt      string         `json:"updated_at"`
	HasMermaid     bool           `json:"has_mermaid"`
	Headings       []Heading      `json:"headings"`
	Tags           []string       `json:"tags"`
	OwnerID        int64          `json:"owner_id"`
}

type Category struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	NavOrder    int    `json:"nav_order"`
	Visible     bool   `json:"visible"`
	Count       int    `json:"count"`
}

type WikiLinkItem struct {
	Slug      string `json:"slug"`
	Label     string `json:"label"`
	Direction string `json:"direction"`
}

type VersionEntry struct {
	VersionNo int    `json:"version_no"`
	Title     string `json:"title"`
	Author    string `json:"author"`
	CreatedAt string `json:"created_at"`
	Summary   string `json:"summary"`
}

type ActivityItem struct {
	Actor     string `json:"actor"`
	Title     string `json:"title"`
	Slug      string `json:"slug"`
	Summary   string `json:"summary"`
	CreatedAt string `json:"created_at"`
}

type FileObject struct {
	ID           int64  `json:"id"`
	SHA256       string `json:"sha256"`
	StorageKey   string `json:"storage_key"`
	OriginalName string `json:"original_name"`
	MIME         string `json:"mime"`
	SizeBytes    int64  `json:"size_bytes"`
	UploadedBy   int64  `json:"uploaded_by"`
	CreatedAt    string `json:"created_at"`
}

type AdminUserRow struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Role        string `json:"role"`
	Active      bool   `json:"active"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type AdminAccessRow struct {
	ArticleID    int64  `json:"article_id"`
	ArticleTitle string `json:"article_title"`
	ArticleSlug  string `json:"article_slug"`
	UserID       int64  `json:"user_id"`
	Username     string `json:"username"`
	Permission   string `json:"permission"`
}

type BackupRow struct {
	Name      string `json:"name"`
	SizeBytes int64  `json:"size_bytes"`
	CreatedAt string `json:"created_at"`
}

type CommentStatus string

const (
	CommentStatusOpen     CommentStatus = "open"
	CommentStatusResolved CommentStatus = "resolved"
	CommentStatusOrphaned CommentStatus = "orphaned"
)

type Comment struct {
	ID             int64         `json:"id"`
	DocumentID     int64         `json:"document_id"`
	AuthorID       int64         `json:"author_id"`
	AuthorName     string        `json:"author_name,omitempty"`
	ParentID       *int64        `json:"parent_id,omitempty"`
	BaseRevisionID int64         `json:"base_revision_id"`
	StartOffset    int           `json:"start_offset"`
	EndOffset      int           `json:"end_offset"`
	QuoteExact     string        `json:"quote_exact"`
	QuotePrefix    string        `json:"quote_prefix"`
	QuoteSuffix    string        `json:"quote_suffix"`
	ASTNodeKind    string        `json:"ast_node_kind"`
	ASTPath        string        `json:"ast_path"`
	HeadingID      string        `json:"heading_id"`
	Status         CommentStatus `json:"status"`
	Body           string        `json:"body"`
	CreatedAt      string        `json:"created_at"`
	UpdatedAt      string        `json:"updated_at"`
	Replies        []Comment     `json:"replies,omitempty"`
}
