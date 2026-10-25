package apikey

// LegacyScopes retains only the capabilities available before scoped keys.
// A nil persisted scope list represents a key created before scope support;
// an explicit empty list grants no permissions.
func LegacyScopes() []string {
	return []string{"account:read", "models:read", "images:read", "images:write"}
}

func SupportedScopes() []string {
	return []string{"account:read", "models:read", "images:read", "images:write", "videos:read", "videos:write", "videos:delete", "assets:read", "assets:write", "assets:delete", "uploads:read", "uploads:write", "uploads:delete", "projects:read", "projects:write", "projects:delete", "groups:read", "groups:write", "groups:delete"}
}

func EffectiveScopes(scopes []string) []string {
	if scopes == nil {
		return LegacyScopes()
	}
	return append([]string{}, scopes...)
}

func HasScope(scopes []string, required string) bool {
	for _, scope := range EffectiveScopes(scopes) {
		if scope == required {
			return true
		}
	}
	return false
}
