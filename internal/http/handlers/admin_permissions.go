package handlers

import (
	"net/http"

	domainadminauth "github.com/fatballfish/pic-gallery/internal/domain/adminauth"
	adminauthservice "github.com/fatballfish/pic-gallery/internal/service/adminauth"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

func (a *API) requireAdminPermission(r *http.Request, permission domainadminauth.Permission) (*adminauthservice.Claims, *errs.Error) {
	admin, appErr := a.requireAdmin(r)
	if appErr != nil {
		return nil, appErr
	}
	if !a.adminHasPermission(admin, permission) {
		return admin, errs.New(http.StatusForbidden, errs.CodeForbidden, "admin permission denied")
	}
	return admin, nil
}

func (a *API) requireAdminPermissionWithQueryToken(r *http.Request, permission domainadminauth.Permission) (*adminauthservice.Claims, *errs.Error) {
	admin, appErr := a.requireAdminWithQueryToken(r)
	if appErr != nil {
		return nil, appErr
	}
	if !a.adminHasPermission(admin, permission) {
		return nil, errs.New(http.StatusForbidden, errs.CodeForbidden, "admin permission denied")
	}
	return admin, nil
}

func (a *API) adminHasPermission(admin *adminauthservice.Claims, permission domainadminauth.Permission) bool {
	if admin == nil {
		return false
	}
	return a.adminPermissionResolver().HasPermission(adminPrincipalFromClaims(admin), permission)
}

func (a *API) adminPermissionsForPrincipal(principal domainadminauth.AdminPrincipal) []string {
	permissions := a.adminPermissionResolver().PermissionsForPrincipal(principal)
	result := make([]string, 0, len(permissions))
	for _, permission := range permissions {
		result = append(result, string(permission))
	}
	return result
}

func (a *API) adminPermissionResolver() domainadminauth.PermissionResolver {
	if a == nil || a.adminPerms == nil {
		return domainadminauth.RolePermissionResolver{}
	}
	return a.adminPerms
}

func configUpdatePermission(tabKey string) domainadminauth.Permission {
	switch tabKey {
	case "auth_security", "payments":
		return domainadminauth.PermissionManageDangerousConfig
	default:
		return domainadminauth.PermissionManageConfig
	}
}

func adminPrincipalFromClaims(admin *adminauthservice.Claims) domainadminauth.AdminPrincipal {
	if admin == nil {
		return domainadminauth.AdminPrincipal{}
	}
	return domainadminauth.AdminPrincipal{
		ID:     admin.AdminID,
		Email:  admin.Email,
		Role:   admin.Role,
		Status: admin.Status,
	}
}
