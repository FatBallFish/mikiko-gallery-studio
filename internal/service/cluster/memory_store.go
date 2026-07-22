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
	challenges   map[string]domaincluster.ChallengeRecord
	auditRecords []domainaudit.RecordRequest
}

func NewMemoryStore(installation domaincluster.Installation) *MemoryStore {
	return &MemoryStore{installation: installation, tokens: map[string]domaincluster.TokenRecord{}, nodes: map[string]domaincluster.Node{}, challenges: map[string]domaincluster.ChallengeRecord{}}
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

func (s *MemoryStore) GetTokenForEnrollment(_ context.Context, installationID, tokenID string, at time.Time) (domaincluster.TokenRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, exists := s.tokens[tokenID]
	if !exists || record.InstallationID != installationID || !record.ExpiresAt.After(at) {
		return domaincluster.TokenRecord{}, ErrTokenNotFound
	}
	if record.ConsumedAt != nil || record.RevokedAt != nil {
		return domaincluster.TokenRecord{}, ErrTokenUnavailable
	}
	return record, nil
}

func (s *MemoryStore) CreateChallenge(_ context.Context, record domaincluster.ChallengeRecord) (domaincluster.ChallengeRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.challenges[record.ChallengeID]; exists {
		return domaincluster.ChallengeRecord{}, ErrChallengeUnavailable
	}
	s.challenges[record.ChallengeID] = record
	return record, nil
}

func (s *MemoryStore) GetChallenge(_ context.Context, installationID, challengeID string) (domaincluster.ChallengeRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, exists := s.challenges[challengeID]
	if !exists || record.InstallationID != installationID {
		return domaincluster.ChallengeRecord{}, ErrChallengeNotFound
	}
	return record, nil
}

func (s *MemoryStore) ListNodes(_ context.Context, installationID string, request domaincluster.ListNodesRequest) ([]domaincluster.Node, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]domaincluster.Node, 0, len(s.nodes))
	for _, node := range s.nodes {
		if node.InstallationID == installationID && (request.Role == "" || node.Role == request.Role) {
			items = append(items, node)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].UpdatedAt.After(items[j].UpdatedAt) })
	total := len(items)
	start := (request.Page - 1) * request.PageSize
	if start > total {
		start = total
	}
	end := start + request.PageSize
	if end > total {
		end = total
	}
	return items[start:end], total, nil
}

func (s *MemoryStore) AcceptEnrollmentWithChallenge(_ context.Context, installationID, challengeID, tokenID, tokenHash string, node domaincluster.Node, acceptedAt time.Time) (domaincluster.TokenRecord, domaincluster.Node, domaincluster.ChallengeRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	challenge, exists := s.challenges[challengeID]
	if !exists || challenge.InstallationID != installationID || challenge.TokenID != tokenID || challenge.NodeID != node.NodeID || challenge.Role != domaincluster.JoinRole(node.Role) {
		return domaincluster.TokenRecord{}, domaincluster.Node{}, domaincluster.ChallengeRecord{}, ErrChallengeNotFound
	}
	if challenge.ConsumedAt != nil || !challenge.ExpiresAt.After(acceptedAt) {
		return domaincluster.TokenRecord{}, domaincluster.Node{}, domaincluster.ChallengeRecord{}, ErrChallengeUnavailable
	}
	if node.InstallationID != installationID {
		return domaincluster.TokenRecord{}, domaincluster.Node{}, domaincluster.ChallengeRecord{}, ErrNodeConflict
	}
	token, exists := s.tokens[tokenID]
	if !exists || token.InstallationID != installationID || token.Role != domaincluster.JoinRole(node.Role) || subtle.ConstantTimeCompare([]byte(token.TokenHash), []byte(tokenHash)) != 1 || !token.ExpiresAt.After(acceptedAt) {
		return domaincluster.TokenRecord{}, domaincluster.Node{}, domaincluster.ChallengeRecord{}, ErrTokenNotFound
	}
	if token.ConsumedAt != nil || token.RevokedAt != nil {
		return domaincluster.TokenRecord{}, domaincluster.Node{}, domaincluster.ChallengeRecord{}, ErrTokenUnavailable
	}
	if _, exists := s.nodes[node.NodeID]; exists {
		return domaincluster.TokenRecord{}, domaincluster.Node{}, domaincluster.ChallengeRecord{}, ErrNodeConflict
	}
	challenge.ConsumedAt = timePointer(acceptedAt)
	challenge.UpdatedAt = acceptedAt
	token.ConsumedAt = timePointer(acceptedAt)
	token.ConsumedByNodeID = node.NodeID
	token.UpdatedAt = acceptedAt
	s.challenges[challengeID] = challenge
	s.tokens[tokenID] = token
	s.nodes[node.NodeID] = node
	s.auditRecords = append(s.auditRecords, EnrollmentAuditRecords(token.Token, node)...)
	return token, node, challenge, nil
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
	if !exists || node.InstallationID != installationID || node.Role != request.Role {
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
