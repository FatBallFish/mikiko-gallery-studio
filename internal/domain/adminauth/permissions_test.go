package adminauth

import "testing"

func TestRolePermissionResolver(t *testing.T) {
	tests := []struct {
		name       string
		role       string
		permission Permission
		want       bool
	}{
		{name: "super admin can manage admins", role: RoleSuperAdmin, permission: PermissionManageAdmins, want: true},
		{name: "super admin can manage dangerous config", role: RoleSuperAdmin, permission: PermissionManageDangerousConfig, want: true},
		{name: "admin can manage users", role: RoleAdmin, permission: PermissionManageUsers, want: true},
		{name: "admin can manage cashier", role: RoleAdmin, permission: PermissionManageCashier, want: true},
		{name: "admin cannot manage admins", role: RoleAdmin, permission: PermissionManageAdmins, want: false},
		{name: "admin cannot manage dangerous config", role: RoleAdmin, permission: PermissionManageDangerousConfig, want: false},
		{name: "unknown role cannot read", role: "viewer", permission: PermissionReadOnly, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			principal := AdminPrincipal{ID: 1001, Email: "ops@example.com", Role: tt.role, Status: "active"}
			if got := (RolePermissionResolver{}).HasPermission(principal, tt.permission); got != tt.want {
				t.Fatalf("HasPermission(%q, %q) = %v, want %v", tt.role, tt.permission, got, tt.want)
			}
		})
	}
}

func TestRolePermissionResolverPermissionsForRole(t *testing.T) {
	resolver := RolePermissionResolver{}
	adminPermissions := resolver.PermissionsForPrincipal(AdminPrincipal{ID: 1002, Email: "ops@example.com", Role: RoleAdmin, Status: "active"})

	if len(adminPermissions) == 0 {
		t.Fatalf("expected admin permissions")
	}
	if containsPermission(adminPermissions, PermissionManageDangerousConfig) {
		t.Fatalf("admin permissions must not include dangerous config: %#v", adminPermissions)
	}
	if !containsPermission(adminPermissions, PermissionManageCashier) {
		t.Fatalf("admin permissions should include cashier: %#v", adminPermissions)
	}
	if got := len(resolver.PermissionsForPrincipal(AdminPrincipal{ID: 1, Email: "root@example.com", Role: RoleSuperAdmin, Status: "active"})); got != len(KnownPermissions) {
		t.Fatalf("super_admin permission count = %d, want %d", got, len(KnownPermissions))
	}
}

func containsPermission(items []Permission, target Permission) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
