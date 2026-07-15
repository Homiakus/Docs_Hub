package authz

import (
	"context"
	"errors"

	"github.com/homiakus/docshub-next/internal/domain"
)

var (
	ErrUnauthorized = errors.New("unauthorized: missing authentication")
	ErrForbidden    = errors.New("forbidden: permission denied")
)

type Resource struct {
	Type       string // "organization", "space", "document"
	ID         int64
	OwnerID    int64
	Visibility string
}

type Authorizer interface {
	Check(ctx context.Context, u *domain.User, action domain.Action, res Resource) error
}

type DefaultAuthorizer struct{}

func New() Authorizer {
	return &DefaultAuthorizer{}
}

func (a *DefaultAuthorizer) Check(ctx context.Context, u *domain.User, action domain.Action, res Resource) error {
	if action == domain.ActionRead && res.Visibility == "public" {
		return nil
	}
	if u == nil {
		return ErrUnauthorized
	}
	if u.Role == "admin" {
		return nil
	}

	switch action {
	case domain.ActionRead:
		if res.Visibility == "authenticated" || u.ID == res.OwnerID {
			return nil
		}
		return ErrForbidden

	case domain.ActionCreate:
		if u.Role == "editor" || u.Role == "admin" {
			return nil
		}
		return ErrForbidden

	case domain.ActionEdit:
		if u.Role == "editor" && (res.OwnerID == 0 || u.ID == res.OwnerID || res.Visibility == "authenticated" || res.Visibility == "public") {
			return nil
		}
		return ErrForbidden

	case domain.ActionDelete, domain.ActionManageACL, domain.ActionPublish, domain.ActionArchive:
		if u.Role == "admin" || (res.OwnerID != 0 && u.ID == res.OwnerID) {
			return nil
		}
		return ErrForbidden

	default:
		return ErrForbidden
	}
}
