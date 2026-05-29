package model

import "time"

type UserRole string

const (
	RoleAdmin UserRole = "admin"
	RoleUser  UserRole = "user"
)

type User struct {
	ID           string
	Username     string
	SaltHex      string
	PasswordHash string
	Role         UserRole
	Active       bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Group struct {
	ID        string
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Article struct {
	ID        string
	Title     string
	Slug      string
	Content   string
	OwnerID   string
	Archived  bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

type ArticleVersion struct {
	ID        string
	ArticleID string
	ActorID   string
	Title     string
	Slug      string
	Content   string
	CreatedAt time.Time
}

type File struct {
	ID           string
	SHA256       string
	StorageKey   string
	OriginalName string
	MIME         string
	Size         int64
	UploadedBy   string
	CreatedAt    time.Time
}

type Link struct {
	ArticleID  string
	TargetSlug string
	Alias      string
	CreatedAt  time.Time
}
