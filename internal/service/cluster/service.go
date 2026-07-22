package cluster

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	domainaudit "github.com/fatballfish/pic-gallery/internal/domain/audit"
	domaincluster "github.com/fatballfish/pic-gallery/internal/domain/cluster"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

const (
	minimumTokenTTL = time.Minute
	maximumTokenTTL = 24 * time.Hour
	tokenSecretSize = 32
)

var (
	ErrInstallationNotFound = errors.New("cluster installation not found")
	ErrTokenNotFound        = errors.New("cluster token not found")
	ErrTokenUnavailable     = errors.New("cluster token unavailable")
	ErrNodeNotFound         = errors.New("cluster node not found")
	ErrNodeConflict         = errors.New("cluster node identity conflict")
)

type Store interface {
	GetInstallation(ctx context.Context, installationID string) (domaincluster.Installation, error)
	CreateToken(ctx context.Context, record domaincluster.TokenRecord) (domaincluster.TokenRecord, error)
	ListTokens(ctx context.Context, installationID string, request domaincluster.ListTokensRequest) (domaincluster.TokenPage, error)
	RevokeToken(ctx context.Context, installationID, tokenID, actorID string, revokedAt time.Time) (domaincluster.TokenRecord, error)
	AcceptEnrollment(ctx context.Context, installationID, tokenID, tokenHash string, node domaincluster.Node, acceptedAt time.Time) (domaincluster.TokenRecord, domaincluster.Node, error)
	HeartbeatNode(ctx context.Context, installationID string, request domaincluster.HeartbeatRequest, heartbeatAt time.Time) (domaincluster.Node, error)
}

type ServiceOptions struct {
	Store          Store
	InstallationID string
	DeploymentRole domaincluster.NodeRole
	Now            func() time.Time
	Entropy        io.Reader
}

type Service struct {
	store          Store
	installationID string
	deploymentRole domaincluster.NodeRole
	now            func() time.Time
	entropy        io.Reader
}

func NewService(options ServiceOptions) *Service {
	if options.Store == nil {
		options.Store = NewMemoryStore(domaincluster.Installation{InstallationID: options.InstallationID})
	}
	if options.Now == nil {
		options.Now = func() time.Time { return time.Now().UTC() }
	}
	if options.Entropy == nil {
		options.Entropy = rand.Reader
	}
	return &Service{
		store:          options.Store,
		installationID: strings.TrimSpace(options.InstallationID), deploymentRole: options.DeploymentRole,
		now: options.Now, entropy: options.Entropy,
	}
}

func (s *Service) CreateToken(ctx context.Context, request domaincluster.CreateTokenRequest) (domaincluster.IssuedToken, error) {
	if _, err := s.requireControlInstallation(ctx); err != nil {
		return domaincluster.IssuedToken{}, err
	}
	if !validJoinRole(request.Role) {
		return domaincluster.IssuedToken{}, errs.BadRequest("role must be api, worker, or web")
	}
	if request.TTL < minimumTokenTTL || request.TTL > maximumTokenTTL {
		return domaincluster.IssuedToken{}, errs.BadRequest("ttl must be between 1 minute and 24 hours")
	}
	actorID := strings.TrimSpace(request.ActorID)
	if actorID == "" {
		return domaincluster.IssuedToken{}, errs.BadRequest("actor_id is required")
	}
	secret := make([]byte, tokenSecretSize)
	if _, err := io.ReadFull(s.entropy, secret); err != nil {
		return domaincluster.IssuedToken{}, fmt.Errorf("generate cluster token secret: %w", err)
	}
	now := s.now().UTC()
	tokenID := uuid.NewString()
	credential := "pgjoin.v1." + tokenID + "." + base64.RawURLEncoding.EncodeToString(secret)
	digest := sha256.Sum256([]byte(credential))
	record, err := s.store.CreateToken(ctx, domaincluster.TokenRecord{
		Token: domaincluster.Token{
			TokenID: tokenID, InstallationID: s.installationID, Role: request.Role,
			ExpiresAt: now.Add(request.TTL), CreatedBy: actorID, CreatedAt: now, UpdatedAt: now,
		},
		TokenHash: hex.EncodeToString(digest[:]),
	})
	if err != nil {
		return domaincluster.IssuedToken{}, fmt.Errorf("create cluster token: %w", err)
	}
	return domaincluster.IssuedToken{Token: record.Token, Credential: credential}, nil
}

func (s *Service) ListTokens(ctx context.Context, request domaincluster.ListTokensRequest) (domaincluster.TokenPage, error) {
	if _, err := s.requireControlInstallation(ctx); err != nil {
		return domaincluster.TokenPage{}, err
	}
	if request.Role != "" && !validJoinRole(request.Role) {
		return domaincluster.TokenPage{}, errs.BadRequest("invalid role")
	}
	switch strings.TrimSpace(request.Status) {
	case "", "active", "expired", "consumed", "revoked":
	default:
		return domaincluster.TokenPage{}, errs.BadRequest("invalid status")
	}
	if request.Page <= 0 {
		request.Page = 1
	}
	if request.PageSize <= 0 {
		request.PageSize = 20
	}
	if request.PageSize > 100 {
		return domaincluster.TokenPage{}, errs.BadRequest("page_size must be at most 100")
	}
	request.At = s.now().UTC()
	return s.store.ListTokens(ctx, s.installationID, request)
}

func (s *Service) RevokeToken(ctx context.Context, tokenID, actorID string) (domaincluster.Token, error) {
	if _, err := s.requireControlInstallation(ctx); err != nil {
		return domaincluster.Token{}, err
	}
	tokenID, actorID = strings.TrimSpace(tokenID), strings.TrimSpace(actorID)
	if tokenID == "" || actorID == "" {
		return domaincluster.Token{}, errs.BadRequest("token_id and actor_id are required")
	}
	record, err := s.store.RevokeToken(ctx, s.installationID, tokenID, actorID, s.now().UTC())
	if err != nil {
		return domaincluster.Token{}, normalizeStoreError(err)
	}
	return record.Token, nil
}

func (s *Service) EnrollNode(ctx context.Context, credential string, request domaincluster.RegisterNodeRequest) (domaincluster.Enrollment, error) {
	installation, err := s.requireControlInstallation(ctx)
	if err != nil {
		return domaincluster.Enrollment{}, err
	}
	tokenID, tokenHash, err := parseTokenCredential(credential)
	if err != nil {
		return domaincluster.Enrollment{}, errs.Unauthorized("invalid cluster token")
	}
	request.NodeID = strings.TrimSpace(request.NodeID)
	request.ApplicationVersion = strings.TrimSpace(request.ApplicationVersion)
	if request.NodeID == "" || !validJoinedNodeRole(request.Role) || request.ApplicationVersion == "" || request.RuntimeSchemaVersion <= 0 || request.ConfigRevision < 0 {
		return domaincluster.Enrollment{}, errs.BadRequest("valid node identity, role, version, schema, and config revision are required")
	}
	now := s.now().UTC()
	record, node, err := s.store.AcceptEnrollment(ctx, s.installationID, tokenID, tokenHash, domaincluster.Node{
		NodeID: request.NodeID, InstallationID: installation.InstallationID, Role: request.Role,
		ApplicationVersion: request.ApplicationVersion, RuntimeSchemaVersion: request.RuntimeSchemaVersion,
		ConfigRevision: request.ConfigRevision, Health: domaincluster.NodeHealthJoining, CreatedAt: now, UpdatedAt: now,
	}, now)
	if err != nil {
		return domaincluster.Enrollment{}, normalizeTokenConsumeError(err)
	}
	return domaincluster.Enrollment{Token: record.Token, Node: node}, nil
}

func (s *Service) HeartbeatNode(ctx context.Context, request domaincluster.HeartbeatRequest) (domaincluster.Node, error) {
	if _, err := s.requireControlInstallation(ctx); err != nil {
		return domaincluster.Node{}, err
	}
	request.NodeID = strings.TrimSpace(request.NodeID)
	request.ApplicationVersion = strings.TrimSpace(request.ApplicationVersion)
	request.LastError = strings.TrimSpace(request.LastError)
	if request.NodeID == "" || !validNodeHealth(request.Health) || request.ApplicationVersion == "" || request.RuntimeSchemaVersion <= 0 || request.ConfigRevision < 0 || len(request.LastError) > 1024 {
		return domaincluster.Node{}, errs.BadRequest("invalid node heartbeat")
	}
	node, err := s.store.HeartbeatNode(ctx, s.installationID, request, s.now().UTC())
	if err != nil {
		return domaincluster.Node{}, normalizeStoreError(err)
	}
	return node, nil
}

func (s *Service) requireControlInstallation(ctx context.Context) (domaincluster.Installation, error) {
	if s.installationID == "" || (s.deploymentRole != domaincluster.NodeRoleSingle && s.deploymentRole != domaincluster.NodeRoleControl) {
		return domaincluster.Installation{}, errs.New(http.StatusForbidden, errs.CodeForbidden, "cluster control authority is required")
	}
	installation, err := s.store.GetInstallation(ctx, s.installationID)
	if err != nil || installation.InstallationID != s.installationID || !installation.Initialized {
		return domaincluster.Installation{}, errs.New(http.StatusForbidden, errs.CodeForbidden, "initialized control installation is required")
	}
	return installation, nil
}

func parseTokenCredential(credential string) (string, string, error) {
	parts := strings.Split(strings.TrimSpace(credential), ".")
	if len(parts) != 4 || parts[0] != "pgjoin" || parts[1] != "v1" {
		return "", "", errors.New("invalid token format")
	}
	if _, err := uuid.Parse(parts[2]); err != nil {
		return "", "", errors.New("invalid token id")
	}
	secret, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil || len(secret) != tokenSecretSize {
		return "", "", errors.New("invalid token secret")
	}
	digest := sha256.Sum256([]byte(strings.TrimSpace(credential)))
	return parts[2], hex.EncodeToString(digest[:]), nil
}

func validJoinRole(role domaincluster.JoinRole) bool {
	return role == domaincluster.JoinRoleAPI || role == domaincluster.JoinRoleWorker || role == domaincluster.JoinRoleWeb
}

func validJoinedNodeRole(role domaincluster.NodeRole) bool {
	return role == domaincluster.NodeRoleAPI || role == domaincluster.NodeRoleWorker || role == domaincluster.NodeRoleWeb
}

func validNodeHealth(health domaincluster.NodeHealth) bool {
	switch health {
	case domaincluster.NodeHealthJoining, domaincluster.NodeHealthHealthy, domaincluster.NodeHealthDegraded, domaincluster.NodeHealthUnready, domaincluster.NodeHealthOffline:
		return true
	default:
		return false
	}
}

func EnrollmentAuditRecords(token domaincluster.Token, node domaincluster.Node) []domainaudit.RecordRequest {
	return []domainaudit.RecordRequest{
		{
			ActorType: "cluster_node", ActorID: node.NodeID, Action: "cluster.token.consume",
			TargetType: "cluster_token", TargetID: token.TokenID, Result: "success",
			Metadata: map[string]any{"role": token.Role},
		},
		{
			ActorType: "cluster_node", ActorID: node.NodeID, Action: "cluster.node.register",
			TargetType: "cluster_node", TargetID: node.NodeID, Result: "success",
			Metadata: map[string]any{
				"role": node.Role, "application_version": node.ApplicationVersion,
				"runtime_schema_version": node.RuntimeSchemaVersion, "config_revision": node.ConfigRevision,
			},
		},
	}
}

func TokenCreateAuditRecord(token domaincluster.Token) domainaudit.RecordRequest {
	return domainaudit.RecordRequest{
		ActorType: "admin", ActorID: token.CreatedBy, Action: "cluster.token.create",
		TargetType: "cluster_token", TargetID: token.TokenID, Result: "success",
		Metadata: map[string]any{"role": token.Role, "expires_at": token.ExpiresAt},
	}
}

func TokenRevokeAuditRecord(token domaincluster.Token, actorID string) domainaudit.RecordRequest {
	return domainaudit.RecordRequest{
		ActorType: "admin", ActorID: actorID, Action: "cluster.token.revoke",
		TargetType: "cluster_token", TargetID: token.TokenID, Result: "success",
		Metadata: map[string]any{"role": token.Role},
	}
}

func normalizeStoreError(err error) error {
	switch {
	case errors.Is(err, ErrTokenNotFound), errors.Is(err, ErrNodeNotFound), errors.Is(err, ErrInstallationNotFound):
		return errs.New(http.StatusNotFound, errs.CodeNotFound, "cluster resource not found")
	case errors.Is(err, ErrTokenUnavailable), errors.Is(err, ErrNodeConflict):
		return errs.New(http.StatusConflict, errs.CodeConflict, "cluster resource conflict")
	default:
		return err
	}
}

func normalizeTokenConsumeError(err error) error {
	if errors.Is(err, ErrTokenNotFound) {
		return errs.Unauthorized("invalid or expired cluster token")
	}
	if errors.Is(err, ErrTokenUnavailable) {
		return errs.New(http.StatusConflict, errs.CodeConflict, "cluster token is already consumed or revoked")
	}
	return normalizeStoreError(err)
}
