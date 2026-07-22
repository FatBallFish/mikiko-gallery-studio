package cluster

import (
	"context"
	"crypto/subtle"
	"sort"
	"strings"
	"sync"
	"time"

	domainaudit "github.com/fatballfish/pic-gallery/internal/domain/audit"
	domaincluster "github.com/fatballfish/pic-gallery/internal/domain/cluster"
)

type MemoryStore struct {
	mu           sync.Mutex
	installation domaincluster.Installation
	tokens       map[string]domaincluster.TokenRecord
	nodes        map[string]domaincluster.Node
	auditRecords []domainaudit.RecordRequest
}

func NewMemoryStore(installation domaincluster.Installation) *MemoryStore {
	return &MemoryStore{installation: installation, tokens: map[string]domaincluster.TokenRecord{}, nodes: map[string]domaincluster.Node{}}
}

func (s *MemoryStore) GetInstallation(_ context.Context, installationID string) (domaincluster.Installation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.installation.InstallationID != installationID {
		return domaincluster.Installation{}, ErrInstallationNotFound
	}
	return s.installation, nil
}

func (s *MemoryStore) CreateToken(_ context.Context, record domaincluster.TokenRecord) (domaincluster.TokenRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.tokens[record.TokenID]; exists {
		return domaincluster.TokenRecord{}, ErrTokenUnavailable
	}
	s.tokens[record.TokenID] = record
	s.auditRecords = append(s.auditRecords, TokenCreateAuditRecord(record.Token))
	return record, nil
}

func (s *MemoryStore) ListTokens(_ context.Context, installationID string, request domaincluster.ListTokensRequest) (domaincluster.TokenPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := request.At
	if now.IsZero() {
		now = time.Now().UTC()
	}
	items := make([]domaincluster.Token, 0, len(s.tokens))
	for _, record := range s.tokens {
		if record.InstallationID != installationID || request.Role != "" && record.Role != request.Role || !memoryTokenStatusMatches(record.Token, request.Status, now) {
			continue
		}
		items = append(items, record.Token)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	total := len(items)
	start := (request.Page - 1) * request.PageSize
	if start > total {
		start = total
	}
	end := start + request.PageSize
	if end > total {
		end = total
	}
	return domaincluster.TokenPage{Items: items[start:end], Page: request.Page, PageSize: request.PageSize, Total: total}, nil
}

func memoryTokenStatusMatches(token domaincluster.Token, status string, now time.Time) bool {
	switch strings.TrimSpace(status) {
	case "":
		return true
	case "active":
		return token.ConsumedAt == nil && token.RevokedAt == nil && token.ExpiresAt.After(now)
	case "expired":
		return token.ConsumedAt == nil && token.RevokedAt == nil && !token.ExpiresAt.After(now)
	case "consumed":
		return token.ConsumedAt != nil
	case "revoked":
		return token.RevokedAt != nil
	default:
		return false
	}
}

func (s *MemoryStore) RevokeToken(_ context.Context, installationID, tokenID, actorID string, revokedAt time.Time) (domaincluster.TokenRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, exists := s.tokens[tokenID]
	if !exists || record.InstallationID != installationID {
		return domaincluster.TokenRecord{}, ErrTokenNotFound
	}
	if record.ConsumedAt != nil || record.RevokedAt != nil || !record.ExpiresAt.After(revokedAt) {
		return domaincluster.TokenRecord{}, ErrTokenUnavailable
	}
	record.RevokedAt = timePointer(revokedAt)
	record.UpdatedAt = revokedAt
	s.tokens[tokenID] = record
	s.auditRecords = append(s.auditRecords, TokenRevokeAuditRecord(record.Token, actorID))
	return record, nil
}

func (s *MemoryStore) AcceptEnrollment(_ context.Context, installationID, tokenID, tokenHash string, node domaincluster.Node, acceptedAt time.Time) (domaincluster.TokenRecord, domaincluster.Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if node.InstallationID != installationID {
		return domaincluster.TokenRecord{}, domaincluster.Node{}, ErrNodeConflict
	}
	record, exists := s.tokens[tokenID]
	if !exists || record.InstallationID != installationID || record.Role != domaincluster.JoinRole(node.Role) || subtle.ConstantTimeCompare([]byte(record.TokenHash), []byte(tokenHash)) != 1 || !record.ExpiresAt.After(acceptedAt) {
		return domaincluster.TokenRecord{}, domaincluster.Node{}, ErrTokenNotFound
	}
	if record.ConsumedAt != nil || record.RevokedAt != nil {
		return domaincluster.TokenRecord{}, domaincluster.Node{}, ErrTokenUnavailable
	}
	if _, exists := s.nodes[node.NodeID]; exists {
		return domaincluster.TokenRecord{}, domaincluster.Node{}, ErrNodeConflict
	}
	record.ConsumedAt = timePointer(acceptedAt)
	record.ConsumedByNodeID = node.NodeID
	record.UpdatedAt = acceptedAt
	s.tokens[tokenID] = record
	s.nodes[node.NodeID] = node
	s.auditRecords = append(s.auditRecords, EnrollmentAuditRecords(record.Token, node)...)
	return record, node, nil
}

func (s *MemoryStore) CreateNode(_ context.Context, node domaincluster.Node) (domaincluster.Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, exists := s.nodes[node.NodeID]; exists {
		if existing.InstallationID == node.InstallationID && existing.Role == node.Role && existing.ApplicationVersion == node.ApplicationVersion && existing.RuntimeSchemaVersion == node.RuntimeSchemaVersion && existing.ConfigRevision == node.ConfigRevision {
			return existing, nil
		}
		return domaincluster.Node{}, ErrNodeConflict
	}
	s.nodes[node.NodeID] = node
	return node, nil
}

func (s *MemoryStore) HeartbeatNode(_ context.Context, installationID string, request domaincluster.HeartbeatRequest, heartbeatAt time.Time) (domaincluster.Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	node, exists := s.nodes[request.NodeID]
	if !exists || node.InstallationID != installationID {
		return domaincluster.Node{}, ErrNodeNotFound
	}
	node.Health = request.Health
	node.LastError = request.LastError
	node.ApplicationVersion = request.ApplicationVersion
	node.RuntimeSchemaVersion = request.RuntimeSchemaVersion
	node.ConfigRevision = request.ConfigRevision
	node.LastHeartbeatAt = timePointer(heartbeatAt)
	node.UpdatedAt = heartbeatAt
	s.nodes[node.NodeID] = node
	return node, nil
}

func timePointer(value time.Time) *time.Time { return &value }
