package cluster

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	domaincluster "github.com/fatballfish/pic-gallery/internal/domain/cluster"
)

func TestCryptoPossessionProofBindsCompleteEnrollmentTranscript(t *testing.T) {
	client := mustEphemeralKey(t, 0x11)
	server := mustEphemeralKey(t, 0x22)
	credential := fixedJoinCredential(0x33)
	challenge := cryptoTestChallenge(client.PublicKey(), server.PublicKey())
	proofKey, err := TokenProofKeyFromCredential(credential)
	if err != nil {
		t.Fatal(err)
	}
	proof, err := ComputeClientPossessionProof(proofKey, challenge, "worker-node-a")
	if err != nil {
		t.Fatal(err)
	}
	_, transcriptHash, _ := enrollmentTranscript(challenge, "worker-node-a")
	authShared, _ := x25519Shared(proofKey.privateKey, server.PublicKey())
	nodeShared, _ := x25519Shared(client.privateKey, server.PublicKey())
	wantVector := map[string]string{
		"token_public":    "66MNtMxpysq7ZDOVz5idC7uCKXejofQbPzZ-5aSs6nE",
		"client_public":   "e06Qm75__kTEZaIgA31gjuNYl9Me-XLwf3SJLLD3PxM",
		"server_public":   "D6poTtKIZ7l_Smot7l34zpdOdrcBjj8iocTPJnhXDyA",
		"auth_shared":     "929be24fde7cebe07986045ed164e6dff87c0874cb57d3dc0906cf78453b4758",
		"node_shared":     "9e004098efc091d4ec2663b4e9f5cfd4d7064571690b4bea97ab146ab9f35056",
		"transcript_hash": "f2830a8fcc470330fd8e622e0bebc7882e765e797c42e6e67d2955beb670eddd",
		"proof":           "IVyAMyYZSwlB4-8eQ96aWX_Lm7YsWA9BTCDsQpX6HAQ",
	}
	gotVector := map[string]string{
		"token_public": proofKey.PublicKey(), "client_public": client.PublicKey(), "server_public": server.PublicKey(),
		"auth_shared": fmt.Sprintf("%x", authShared), "node_shared": fmt.Sprintf("%x", nodeShared),
		"transcript_hash": fmt.Sprintf("%x", transcriptHash), "proof": proof,
	}
	if !reflect.DeepEqual(gotVector, wantVector) {
		t.Fatalf("protocol vector = %#v, want %#v", gotVector, wantVector)
	}
	if !VerifyServerPossessionProof(server, proofKey.PublicKey(), challenge, "worker-node-a", proof) {
		t.Fatal("valid possession proof was rejected")
	}
	mutations := []struct {
		name      string
		challenge domaincluster.EnrollmentChallenge
		nodeID    string
	}{
		{name: "installation", challenge: mutateChallenge(challenge, func(value *domaincluster.EnrollmentChallenge) { value.InstallationID = "other-installation" }), nodeID: "worker-node-a"},
		{name: "role", challenge: mutateChallenge(challenge, func(value *domaincluster.EnrollmentChallenge) { value.Role = domaincluster.JoinRoleAPI }), nodeID: "worker-node-a"},
		{name: "application version", challenge: mutateChallenge(challenge, func(value *domaincluster.EnrollmentChallenge) { value.ApplicationVersion = "v2" }), nodeID: "worker-node-a"},
		{name: "runtime schema", challenge: mutateChallenge(challenge, func(value *domaincluster.EnrollmentChallenge) { value.RuntimeSchemaVersion++ }), nodeID: "worker-node-a"},
		{name: "client public key", challenge: mutateChallenge(challenge, func(value *domaincluster.EnrollmentChallenge) { value.ClientPublicKey = server.PublicKey() }), nodeID: "worker-node-a"},
		{name: "server public key", challenge: mutateChallenge(challenge, func(value *domaincluster.EnrollmentChallenge) { value.ServerPublicKey = client.PublicKey() }), nodeID: "worker-node-a"},
		{name: "server nonce", challenge: mutateChallenge(challenge, func(value *domaincluster.EnrollmentChallenge) {
			value.ServerNonce = base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x44}, 32))
		}), nodeID: "worker-node-a"},
		{name: "node id", challenge: challenge, nodeID: "worker-node-b"},
	}
	for _, testCase := range mutations {
		t.Run(testCase.name, func(t *testing.T) {
			if VerifyServerPossessionProof(server, proofKey.PublicKey(), testCase.challenge, testCase.nodeID, proof) {
				t.Fatal("mutated enrollment transcript accepted the original proof")
			}
		})
	}
	wrongKey, err := TokenProofKeyFromCredential(fixedJoinCredential(0x34))
	if err != nil {
		t.Fatal(err)
	}
	if VerifyServerPossessionProof(server, wrongKey.PublicKey(), challenge, "worker-node-a", proof) {
		t.Fatal("wrong token accepted the possession proof")
	}
}

func TestCryptoEnvelopeAuthenticatesCiphertextAndContainsNoPlaintextSecrets(t *testing.T) {
	client := mustEphemeralKey(t, 0x51)
	server := mustEphemeralKey(t, 0x52)
	credential := fixedJoinCredential(0x53)
	proofKey, err := TokenProofKeyFromCredential(credential)
	if err != nil {
		t.Fatal(err)
	}
	challenge := cryptoTestChallenge(client.PublicKey(), server.PublicKey())
	payload := domaincluster.RuntimeEnvelopePayload{
		Protocol: EnrollmentProtocolV1, InstallationID: challenge.InstallationID, NodeID: "worker-node-a",
		Role: domaincluster.NodeRoleWorker, ApplicationVersion: challenge.ApplicationVersion,
		RuntimeSchemaVersion: 1,
		ConfigRevision:       7,
		Values: map[string]string{
			"DATABASE_URL":    "postgres://app:plaintext-password@db:5432/app",
			"REDIS_URL":       "redis://:plaintext-redis@redis:6379/0",
			"DEPLOYMENT_ROLE": "worker",
		},
	}
	sealed, err := SealRuntimeEnvelope(server, proofKey.PublicKey(), challenge, "worker-node-a", payload, bytes.NewReader(bytes.Repeat([]byte{0x54}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	if sealed.Algorithm != EnvelopeAlgorithmV1 || sealed.Nonce != "VFRUVFRUVFRUVFRUVFRUVFRUVFRUVFRU" || sealed.Ciphertext != "qCxIRpKKdu0vuMBUpSNRWC4cL4illNNrgNlGITDwcrThouHWORXaHrpzTu6PE3tbxoaZ9_6xXlnRNZMW0ASXWuUOVMUTxm6K3baaZcPm-kEbrgXnzkji_PiIgu0DEYktfGZmynTCBK2Oej6nDBGa5Q5aH9bj3kJKZ4AvAKlUDV2C_2BoiIg7GmwRVakgeTmpYlCZhSr1OBNJzYbw0tvgfArWNiDVfP2OYtuq8LfWNvhWyErZBnU7u5xS8E4jiO5w7idQj0NdnT41R8kCEparxxX4rsJsAdX9Am1eSya5qCmBBJVDa2mmG7jcr8ARwqOFZ9zyr5l-vuvTWoNQyxhyiDfeDz_A8nmJZ1T6lSLYGm-y-U5Ub7YFtx1KZdeF5zguJrCOFkvDx9I8ONLS49oIAuUM_kysEBW23N7v0hwVgukXEVX2JKLgDG3OGxBHI4wUzfKnC7w7bXvjW4Ju4DYi4rjb9CIJtuhz8bziViRHQcDnawezy7mwg23b0cK_" {
		t.Fatalf("envelope vector changed: %#v", sealed)
	}
	encoded, _ := json.Marshal(sealed)
	for _, plaintext := range payload.Values {
		if strings.Contains(string(encoded), plaintext) {
			t.Fatalf("encrypted envelope exposed plaintext %q", plaintext)
		}
	}
	opened, err := OpenRuntimeEnvelope(client, proofKey, challenge, "worker-node-a", sealed)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(opened, payload) {
		t.Fatalf("opened payload = %#v, want %#v", opened, payload)
	}

	tampered := sealed
	tampered.Ciphertext = tamperBase64(t, sealed.Ciphertext)
	if _, err := OpenRuntimeEnvelope(client, proofKey, challenge, "worker-node-a", tampered); err == nil {
		t.Fatal("tampered ciphertext was accepted")
	}
	wrongKey, _ := TokenProofKeyFromCredential(fixedJoinCredential(0x55))
	if _, err := OpenRuntimeEnvelope(client, wrongKey, challenge, "worker-node-a", sealed); err == nil {
		t.Fatal("wrong token opened the envelope")
	}
	if _, err := OpenRuntimeEnvelope(client, proofKey, challenge, "worker-node-b", sealed); err == nil {
		t.Fatal("wrong node identity opened the envelope")
	}
}

func TestRuntimeKeysForRoleAreExplicitAndSecretMinimized(t *testing.T) {
	apiKeys := RuntimeKeysForRole(domaincluster.NodeRoleAPI)
	workerKeys := RuntimeKeysForRole(domaincluster.NodeRoleWorker)
	webKeys := RuntimeKeysForRole(domaincluster.NodeRoleWeb)
	for _, key := range []string{"DATABASE_URL", "REDIS_URL", "AUTH_ACCESS_TOKEN_SECRET", "API_KEY_SIGNING_SECRET_ENCRYPTION_KEY", "CASHIER_PROVIDER_CONFIG_ENCRYPTION_KEY", "PROMPT_OPTIMIZATION_QUOTE_SIGNING_KEY"} {
		if !slices.Contains(apiKeys, key) {
			t.Fatalf("API role is missing %s", key)
		}
	}
	for _, key := range []string{"DATABASE_URL", "REDIS_URL", "PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY", "STORAGE_S3_SECRET_ACCESS_KEY"} {
		if !slices.Contains(workerKeys, key) {
			t.Fatalf("Worker role is missing %s", key)
		}
	}
	for _, forbidden := range []string{"SETUP_TOKEN", "AUTH_ACCESS_TOKEN_SECRET", "API_KEY_SIGNING_SECRET_ENCRYPTION_KEY", "CASHIER_PROVIDER_CONFIG_ENCRYPTION_KEY", "PROMPT_OPTIMIZATION_QUOTE_SIGNING_KEY"} {
		if slices.Contains(workerKeys, forbidden) {
			t.Fatalf("Worker role received forbidden key %s", forbidden)
		}
	}
	for _, forbidden := range []string{"SETUP_TOKEN", "DATABASE_URL", "REDIS_URL", "AUTH_ACCESS_TOKEN_SECRET", "PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY", "STORAGE_S3_SECRET_ACCESS_KEY"} {
		if slices.Contains(webKeys, forbidden) {
			t.Fatalf("Web role received forbidden key %s", forbidden)
		}
	}
	if !slices.Contains(webKeys, "PUBLIC_API_URL") {
		t.Fatal("Web role is missing PUBLIC_API_URL")
	}
}

func TestEphemeralPrivateKeyRepresentationsAreRedacted(t *testing.T) {
	key := mustEphemeralKey(t, 0x61)
	proofKey, err := TokenProofKeyFromCredential(fixedJoinCredential(0x62))
	if err != nil {
		t.Fatal(err)
	}
	privateMarker := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x61}, 32))
	for _, representation := range []string{key.String(), key.GoString(), string(mustMarshalCrypto(t, key)), proofKey.String(), proofKey.GoString(), string(mustMarshalCrypto(t, proofKey))} {
		if strings.Contains(representation, privateMarker) || !strings.Contains(strings.ToLower(representation), "redact") {
			t.Fatalf("unsafe key representation %q", representation)
		}
	}
}

func mustEphemeralKey(t *testing.T, value byte) EphemeralKeyPair {
	t.Helper()
	key, err := GenerateEphemeralKey(bytes.NewReader(bytes.Repeat([]byte{value}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func fixedJoinCredential(value byte) string {
	return "pgjoin.v1.019d0000-0000-7000-8000-000000000901." + base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{value}, 32))
}

func cryptoTestChallenge(clientPublicKey, serverPublicKey string) domaincluster.EnrollmentChallenge {
	return domaincluster.EnrollmentChallenge{
		Protocol:    EnrollmentProtocolV1,
		ChallengeID: "019d0000-0000-7000-8000-000000000902", InstallationID: "019d0000-0000-7000-8000-000000000903",
		TokenID: "019d0000-0000-7000-8000-000000000901", Role: domaincluster.JoinRoleWorker,
		NodeID:          "worker-node-a",
		ClientPublicKey: clientPublicKey, ServerPublicKey: serverPublicKey,
		ServerNonce:        base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x23}, 32)),
		ApplicationVersion: "v1", RuntimeSchemaVersion: 1, ConfigRevision: 7,
		ExpiresAt: time.Date(2026, 7, 23, 8, 2, 0, 0, time.UTC),
	}
}

func mutateChallenge(challenge domaincluster.EnrollmentChallenge, mutate func(*domaincluster.EnrollmentChallenge)) domaincluster.EnrollmentChallenge {
	mutate(&challenge)
	return challenge
}

func tamperBase64(t *testing.T, value string) string {
	t.Helper()
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	decoded[len(decoded)-1] ^= 0x01
	return base64.RawURLEncoding.EncodeToString(decoded)
}

func mustMarshalCrypto(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
