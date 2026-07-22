package cluster

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	domaincluster "github.com/fatballfish/pic-gallery/internal/domain/cluster"
)

func TestEnrollmentProtocolEncryptsRoleMinimizedRuntimeAndCommitsAtomically(t *testing.T) {
	fixture := newProtocolFixture(t)
	issued := fixture.issueToken(t, domaincluster.JoinRoleWorker)
	client := mustEphemeralKey(t, 0x71)
	nodeID := "worker-enrollment-a"
	challenge, err := fixture.service.CreateChallenge(t.Context(), domaincluster.CreateChallengeRequest{
		Protocol: EnrollmentProtocolV1, TokenID: issued.Token.TokenID, NodeID: nodeID,
		NodePublicKey: client.PublicKey(), ApplicationVersion: "v1", RuntimeSchemaVersion: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	proofKey, err := TokenProofKeyFromCredential(issued.Credential)
	if err != nil {
		t.Fatal(err)
	}
	defer proofKey.Clear()
	proof, err := ComputeClientPossessionProof(proofKey, challenge, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	joined, err := fixture.service.Join(t.Context(), domaincluster.JoinRequest{
		Protocol: EnrollmentProtocolV1, ChallengeID: challenge.ChallengeID, Proof: proof,
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := OpenRuntimeEnvelope(client, proofKey, challenge, nodeID, joined.EncryptedEnvelope)
	if err != nil {
		t.Fatal(err)
	}
	wantValues := map[string]string{
		"DATABASE_URL": "postgres://app:db-secret@db:5432/app", "REDIS_URL": "redis://:redis-secret@redis:6379/0",
		"REDIS_KEY_PREFIX": "cluster", "STORAGE_DRIVER": "s3", "STORAGE_S3_ENDPOINT": "http://minio:9000",
		"STORAGE_S3_REGION": "us-east-1", "STORAGE_S3_BUCKET": "assets", "STORAGE_S3_ACCESS_KEY_ID": "access",
		"STORAGE_S3_SECRET_ACCESS_KEY": "storage-secret", "STORAGE_S3_FORCE_PATH_STYLE": "true",
		"PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY": "secure-config-secret",
	}
	if !reflect.DeepEqual(payload.Values, wantValues) {
		t.Fatalf("worker runtime values = %#v, want %#v", payload.Values, wantValues)
	}
	encoded, _ := json.Marshal(joined)
	for _, secret := range []string{
		issued.Credential, fixture.runtimeValues["SETUP_TOKEN"], fixture.sealKey,
		wantValues["DATABASE_URL"], wantValues["REDIS_URL"], wantValues["STORAGE_S3_SECRET_ACCESS_KEY"], wantValues["PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY"],
	} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("join response exposed secret %q", secret)
		}
	}
	storedToken := fixture.store.tokens[issued.Token.TokenID]
	storedChallenge := fixture.store.challenges[challenge.ChallengeID]
	if storedToken.ConsumedAt == nil || storedToken.ConsumedByNodeID != nodeID || storedChallenge.ConsumedAt == nil || fixture.store.nodes[nodeID].NodeID != nodeID {
		t.Fatalf("atomic enrollment state token=%#v challenge=%#v node=%#v", storedToken, storedChallenge, fixture.store.nodes[nodeID])
	}
	if len(fixture.store.auditRecords) != 3 {
		t.Fatalf("audit records = %#v", fixture.store.auditRecords)
	}
	if _, err := fixture.service.Join(t.Context(), domaincluster.JoinRequest{Protocol: EnrollmentProtocolV1, ChallengeID: challenge.ChallengeID, Proof: proof}); appErrorStatus(err) != 409 {
		t.Fatalf("replayed challenge error = %v", err)
	}
}

func TestEnrollmentProtocolRejectsInvalidProofWithoutConsumingState(t *testing.T) {
	fixture := newProtocolFixture(t)
	issued := fixture.issueToken(t, domaincluster.JoinRoleAPI)
	client := mustEphemeralKey(t, 0x72)
	challenge, err := fixture.service.CreateChallenge(t.Context(), domaincluster.CreateChallengeRequest{
		Protocol: EnrollmentProtocolV1, TokenID: issued.Token.TokenID, NodeID: "api-node-a",
		NodePublicKey: client.PublicKey(), ApplicationVersion: "v1", RuntimeSchemaVersion: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.Join(t.Context(), domaincluster.JoinRequest{
		Protocol: EnrollmentProtocolV1, ChallengeID: challenge.ChallengeID,
		Proof: base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x99}, 32)),
	}); appErrorStatus(err) != 401 {
		t.Fatalf("invalid proof error = %v", err)
	}
	if fixture.store.tokens[issued.Token.TokenID].ConsumedAt != nil || fixture.store.challenges[challenge.ChallengeID].ConsumedAt != nil {
		t.Fatal("invalid proof consumed enrollment state")
	}
	proofKey, _ := TokenProofKeyFromCredential(issued.Credential)
	defer proofKey.Clear()
	proof, _ := ComputeClientPossessionProof(proofKey, challenge, "api-node-a")
	if _, err := fixture.service.Join(t.Context(), domaincluster.JoinRequest{Protocol: EnrollmentProtocolV1, ChallengeID: challenge.ChallengeID, Proof: proof}); err != nil {
		t.Fatalf("valid retry after invalid proof: %v", err)
	}
}

func TestEnrollmentProtocolRejectsVersionMismatchAndExpiredChallenge(t *testing.T) {
	fixture := newProtocolFixture(t)
	issued := fixture.issueToken(t, domaincluster.JoinRoleWeb)
	client := mustEphemeralKey(t, 0x73)
	for _, testCase := range []struct {
		name    string
		version string
		schema  int
	}{
		{name: "application", version: "v2", schema: 1},
		{name: "schema", version: "v1", schema: 2},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := fixture.service.CreateChallenge(t.Context(), domaincluster.CreateChallengeRequest{
				Protocol: EnrollmentProtocolV1, TokenID: issued.Token.TokenID, NodeID: "web-node-a",
				NodePublicKey: client.PublicKey(), ApplicationVersion: testCase.version, RuntimeSchemaVersion: testCase.schema,
			})
			if appErrorStatus(err) != 409 {
				t.Fatalf("version mismatch error = %v", err)
			}
		})
	}
	challenge, err := fixture.service.CreateChallenge(t.Context(), domaincluster.CreateChallengeRequest{
		Protocol: EnrollmentProtocolV1, TokenID: issued.Token.TokenID, NodeID: "web-node-a",
		NodePublicKey: client.PublicKey(), ApplicationVersion: "v1", RuntimeSchemaVersion: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	proofKey, _ := TokenProofKeyFromCredential(issued.Credential)
	defer proofKey.Clear()
	proof, _ := ComputeClientPossessionProof(proofKey, challenge, "web-node-a")
	fixture.now = challenge.ExpiresAt
	if _, err := fixture.service.Join(t.Context(), domaincluster.JoinRequest{Protocol: EnrollmentProtocolV1, ChallengeID: challenge.ChallengeID, Proof: proof}); appErrorStatus(err) != 401 {
		t.Fatalf("expired challenge error = %v", err)
	}
	if fixture.store.tokens[issued.Token.TokenID].ConsumedAt != nil || fixture.store.challenges[challenge.ChallengeID].ConsumedAt != nil {
		t.Fatal("expired challenge consumed enrollment state")
	}
}

type protocolFixture struct {
	service       *Service
	store         *MemoryStore
	runtimeValues map[string]string
	sealKey       string
	now           time.Time
}

func newProtocolFixture(t *testing.T) *protocolFixture {
	t.Helper()
	fixture := &protocolFixture{
		now:     time.Date(2026, 7, 23, 9, 0, 0, 0, time.UTC),
		sealKey: base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x74}, 32)),
		runtimeValues: map[string]string{
			"DATABASE_URL": "postgres://app:db-secret@db:5432/app", "REDIS_URL": "redis://:redis-secret@redis:6379/0",
			"REDIS_KEY_PREFIX": "cluster", "STORAGE_DRIVER": "s3", "STORAGE_S3_ENDPOINT": "http://minio:9000",
			"STORAGE_S3_REGION": "us-east-1", "STORAGE_S3_BUCKET": "assets", "STORAGE_S3_ACCESS_KEY_ID": "access",
			"STORAGE_S3_SECRET_ACCESS_KEY": "storage-secret", "STORAGE_S3_FORCE_PATH_STYLE": "true",
			"PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY": "secure-config-secret", "AUTH_ACCESS_TOKEN_SECRET": "api-auth-secret",
			"API_KEY_SIGNING_SECRET_ENCRYPTION_KEY": "api-key-secret", "CASHIER_PROVIDER_CONFIG_ENCRYPTION_KEY": "cashier-secret",
			"PROMPT_OPTIMIZATION_QUOTE_SIGNING_KEY": "prompt-secret", "PUBLIC_API_URL": "http://api.example.test:8080",
			"SETUP_TOKEN": "must-never-leave-control",
		},
	}
	fixture.store = NewMemoryStore(domaincluster.Installation{
		InstallationID: clusterTestInstallationID, Initialized: true, ApplicationVersion: "v1", RuntimeSchemaVersion: 1, ConfigRevision: 7,
	})
	fixture.service = NewService(ServiceOptions{
		Store: fixture.store, InstallationID: clusterTestInstallationID, DeploymentRole: domaincluster.NodeRoleControl,
		RuntimeValues: fixture.runtimeValues, EnrollmentSealKey: fixture.sealKey,
		Now: func() time.Time { return fixture.now }, Entropy: bytes.NewReader(bytes.Repeat([]byte{0x75}, 4096)),
	})
	return fixture
}

func (fixture *protocolFixture) issueToken(t *testing.T, role domaincluster.JoinRole) domaincluster.IssuedToken {
	t.Helper()
	issued, err := fixture.service.CreateToken(t.Context(), domaincluster.CreateTokenRequest{Role: role, TTL: time.Hour, ActorID: "1"})
	if err != nil {
		t.Fatal(err)
	}
	return issued
}
