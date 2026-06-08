package adminauth

type Permission string

const (
	RoleSuperAdmin = "super_admin"
	RoleAdmin      = "admin"

	PermissionReadOnly              Permission = "read:all"
	PermissionManageAdmins          Permission = "manage:admins"
	PermissionManageUsers           Permission = "manage:users"
	PermissionManageBilling         Permission = "manage:billing"
	PermissionManageCashier         Permission = "manage:cashier"
	PermissionManageModels          Permission = "manage:models"
	PermissionManageReviews         Permission = "manage:reviews"
	PermissionManageConfig          Permission = "manage:config"
	PermissionManageDangerousConfig Permission = "manage:dangerous_config"
	PermissionViewAudit             Permission = "view:audit"
)

var KnownPermissions = []Permission{
	PermissionReadOnly,
	PermissionManageAdmins,
	PermissionManageUsers,
	PermissionManageBilling,
	PermissionManageCashier,
	PermissionManageModels,
	PermissionManageReviews,
	PermissionManageConfig,
	PermissionManageDangerousConfig,
	PermissionViewAudit,
}

type RolePermissionResolver struct{}

type AdminPrincipal struct {
	ID     int64
	Email  string
	Role   string
	Status string
}

type PermissionResolver interface {
	HasPermission(principal AdminPrincipal, permission Permission) bool
	PermissionsForPrincipal(principal AdminPrincipal) []Permission
}

func (r RolePermissionResolver) HasPermission(principal AdminPrincipal, permission Permission) bool {
	return r.HasRolePermission(principal.Role, permission)
}

func (r RolePermissionResolver) PermissionsForPrincipal(principal AdminPrincipal) []Permission {
	return r.PermissionsForRole(principal.Role)
}

func (RolePermissionResolver) HasRolePermission(role string, permission Permission) bool {
	switch role {
	case RoleSuperAdmin:
		return true
	case RoleAdmin:
		return permission != PermissionManageAdmins && permission != PermissionManageDangerousConfig
	default:
		return false
	}
}

func (r RolePermissionResolver) PermissionsForRole(role string) []Permission {
	permissions := make([]Permission, 0, len(KnownPermissions))
	for _, permission := range KnownPermissions {
		if r.HasRolePermission(role, permission) {
			permissions = append(permissions, permission)
		}
	}
	return permissions
}
