package config

import "strings"

func RuntimeHeartbeatRoles(runtime RuntimeConfig) []DeploymentRole {
	found := map[DeploymentRole]bool{}
	for _, module := range runtime.DeploymentModules {
		switch strings.ToLower(strings.TrimSpace(module)) {
		case string(DeploymentRoleAPI):
			found[DeploymentRoleAPI] = true
		case string(DeploymentRoleWorker):
			found[DeploymentRoleWorker] = true
		}
	}
	roles := make([]DeploymentRole, 0, 2)
	for _, role := range []DeploymentRole{DeploymentRoleAPI, DeploymentRoleWorker} {
		if found[role] {
			roles = append(roles, role)
		}
	}
	if len(roles) > 0 {
		return roles
	}
	switch runtime.DeploymentRole {
	case DeploymentRoleSingle:
		return []DeploymentRole{DeploymentRoleAPI, DeploymentRoleWorker}
	case DeploymentRoleAPI, DeploymentRoleWorker:
		return []DeploymentRole{runtime.DeploymentRole}
	default:
		return nil
	}
}
