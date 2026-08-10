package cluster

import (
	"bytes"
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	domaincluster "github.com/fatballfish/pic-gallery/internal/domain/cluster"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
)

const (
	EnrollmentProtocolV1 = "pg-cluster-enrollment-v1"
	EnvelopeAlgorithmV1  = "X25519-HKDF-SHA256-XCHACHA20-POLY1305"

	tokenProofSaltDomain = "pic-gallery/cluster/token-proof-salt/v1"
	tokenProofInfoDomain = "pic-gallery/cluster/token-proof-x25519/v1"
	transcriptDomain     = "pic-gallery/cluster/transcript/v1"
	proofKeyDomain       = "pic-gallery/cluster/proof-key/v1"
	envelopeKeyDomain    = "pic-gallery/cluster/envelope-key/v1"
	envelopeAADDomain    = "pic-gallery/cluster/envelope-aad/v1"
	challengeSealDomain  = "pic-gallery/cluster/challenge-seal/v1"
)

type EphemeralKeyPair struct {
	privateKey []byte
	publicKey  string
}

type TokenProofKeyPair struct {
	privateKey []byte
	publicKey  string
}

func GenerateEphemeralKey(entropy io.Reader) (EphemeralKeyPair, error) {
	privateKey, publicKey, err := generateX25519Key(entropy)
	if err != nil {
		return EphemeralKeyPair{}, err
	}
	return EphemeralKeyPair{privateKey: privateKey, publicKey: publicKey}, nil
}

func TokenProofKeyFromCredential(credential string) (TokenProofKeyPair, error) {
	tokenID, secret, err := credentialSecret(credential)
	if err != nil {
		return TokenProofKeyPair{}, err
	}
	defer clear(secret)
	salt := sha256.Sum256(append([]byte(tokenProofSaltDomain+":"), tokenID...))
	privateKey := make([]byte, 32)
	if _, err := io.ReadFull(hkdf.New(sha256.New, secret, salt[:], []byte(tokenProofInfoDomain)), privateKey); err != nil {
		return TokenProofKeyPair{}, fmt.Errorf("derive token proof key: %w", err)
	}
	parsed, err := ecdh.X25519().NewPrivateKey(privateKey)
	if err != nil {
		clear(privateKey)
		return TokenProofKeyPair{}, errors.New("derived token proof key is invalid")
	}
	return TokenProofKeyPair{
		privateKey: privateKey,
		publicKey:  base64.RawURLEncoding.EncodeToString(parsed.PublicKey().Bytes()),
	}, nil
}

func (pair EphemeralKeyPair) PublicKey() string  { return pair.publicKey }
func (pair TokenProofKeyPair) PublicKey() string { return pair.publicKey }

func (pair *EphemeralKeyPair) Clear() {
	if pair == nil {
		return
	}
	clear(pair.privateKey)
	pair.privateKey = nil
}

func (pair *TokenProofKeyPair) Clear() {
	if pair == nil {
		return
	}
	clear(pair.privateKey)
	pair.privateKey = nil
}

func (pair EphemeralKeyPair) String() string {
	return fmt.Sprintf("EphemeralKeyPair{PublicKey:%q, PrivateKey:<redacted>}", pair.publicKey)
}
func (pair EphemeralKeyPair) GoString() string { return pair.String() }
func (pair EphemeralKeyPair) MarshalJSON() ([]byte, error) {
	return redactedKeyJSON(pair.publicKey)
}

func (pair TokenProofKeyPair) String() string {
	return fmt.Sprintf("TokenProofKeyPair{PublicKey:%q, PrivateKey:<redacted>}", pair.publicKey)
}
func (pair TokenProofKeyPair) GoString() string { return pair.String() }
func (pair TokenProofKeyPair) MarshalJSON() ([]byte, error) {
	return redactedKeyJSON(pair.publicKey)
}

func ComputeClientPossessionProof(tokenProof TokenProofKeyPair, challenge domaincluster.EnrollmentChallenge, nodeID string) (string, error) {
	transcript, transcriptHash, err := enrollmentTranscript(challenge, nodeID)
	if err != nil {
		return "", err
	}
	authShared, err := x25519Shared(tokenProof.privateKey, challenge.ServerPublicKey)
	if err != nil {
		return "", err
	}
	defer clear(authShared)
	proofKey, err := deriveProofKey(authShared, challenge.ServerNonce, transcriptHash)
	if err != nil {
		return "", err
	}
	defer clear(proofKey)
	digest := hmac.New(sha256.New, proofKey)
	_, _ = digest.Write(transcript)
	return base64.RawURLEncoding.EncodeToString(digest.Sum(nil)), nil
}

func VerifyServerPossessionProof(server EphemeralKeyPair, tokenProofPublicKey string, challenge domaincluster.EnrollmentChallenge, nodeID, proof string) bool {
	transcript, transcriptHash, err := enrollmentTranscript(challenge, nodeID)
	if err != nil {
		return false
	}
	authShared, err := x25519Shared(server.privateKey, tokenProofPublicKey)
	if err != nil {
		return false
	}
	defer clear(authShared)
	proofKey, err := deriveProofKey(authShared, challenge.ServerNonce, transcriptHash)
	if err != nil {
		return false
	}
	defer clear(proofKey)
	provided, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(proof))
	if err != nil || len(provided) != sha256.Size {
		return false
	}
	digest := hmac.New(sha256.New, proofKey)
	_, _ = digest.Write(transcript)
	return hmac.Equal(provided, digest.Sum(nil))
}

func SealRuntimeEnvelope(server EphemeralKeyPair, tokenProofPublicKey string, challenge domaincluster.EnrollmentChallenge, nodeID string, payload domaincluster.RuntimeEnvelopePayload, entropy io.Reader) (domaincluster.EncryptedRuntimeEnvelope, error) {
	if err := validateRuntimePayload(payload, challenge, nodeID); err != nil {
		return domaincluster.EncryptedRuntimeEnvelope{}, err
	}
	aead, aad, err := serverEnvelopeAEAD(server, tokenProofPublicKey, challenge, nodeID)
	if err != nil {
		return domaincluster.EncryptedRuntimeEnvelope{}, err
	}
	nonce := make([]byte, aead.NonceSize())
	if entropy == nil {
		return domaincluster.EncryptedRuntimeEnvelope{}, errors.New("envelope entropy is required")
	}
	if _, err := io.ReadFull(entropy, nonce); err != nil {
		return domaincluster.EncryptedRuntimeEnvelope{}, fmt.Errorf("read envelope nonce: %w", err)
	}
	plaintext, err := json.Marshal(payload)
	if err != nil {
		return domaincluster.EncryptedRuntimeEnvelope{}, fmt.Errorf("marshal runtime envelope: %w", err)
	}
	defer clear(plaintext)
	return domaincluster.EncryptedRuntimeEnvelope{
		Algorithm:  EnvelopeAlgorithmV1,
		Nonce:      base64.RawURLEncoding.EncodeToString(nonce),
		Ciphertext: base64.RawURLEncoding.EncodeToString(aead.Seal(nil, nonce, plaintext, aad)),
	}, nil
}

func OpenRuntimeEnvelope(client EphemeralKeyPair, tokenProof TokenProofKeyPair, challenge domaincluster.EnrollmentChallenge, nodeID string, envelope domaincluster.EncryptedRuntimeEnvelope) (domaincluster.RuntimeEnvelopePayload, error) {
	if envelope.Algorithm != EnvelopeAlgorithmV1 {
		return domaincluster.RuntimeEnvelopePayload{}, errors.New("runtime envelope algorithm is unsupported")
	}
	aead, aad, err := clientEnvelopeAEAD(client, tokenProof, challenge, nodeID)
	if err != nil {
		return domaincluster.RuntimeEnvelopePayload{}, err
	}
	nonce, err := base64.RawURLEncoding.DecodeString(envelope.Nonce)
	if err != nil || len(nonce) != aead.NonceSize() {
		return domaincluster.RuntimeEnvelopePayload{}, errors.New("runtime envelope nonce is invalid")
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(envelope.Ciphertext)
	if err != nil || len(ciphertext) < aead.Overhead() {
		return domaincluster.RuntimeEnvelopePayload{}, errors.New("runtime envelope ciphertext is invalid")
	}
	plaintext, err := aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return domaincluster.RuntimeEnvelopePayload{}, errors.New("runtime envelope authentication failed")
	}
	defer clear(plaintext)
	var payload domaincluster.RuntimeEnvelopePayload
	decoder := json.NewDecoder(bytes.NewReader(plaintext))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return domaincluster.RuntimeEnvelopePayload{}, errors.New("runtime envelope payload is invalid")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return domaincluster.RuntimeEnvelopePayload{}, errors.New("runtime envelope payload must contain one JSON object")
	}
	if err := validateRuntimePayload(payload, challenge, nodeID); err != nil {
		return domaincluster.RuntimeEnvelopePayload{}, err
	}
	return payload, nil
}

func SealChallengePrivateKey(pair EphemeralKeyPair, sealKey string, challenge domaincluster.EnrollmentChallenge, entropy io.Reader) (string, error) {
	aead, aad, err := challengeSealAEAD(sealKey, challenge)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if entropy == nil {
		return "", errors.New("challenge seal entropy is required")
	}
	if _, err := io.ReadFull(entropy, nonce); err != nil {
		return "", fmt.Errorf("read challenge seal nonce: %w", err)
	}
	ciphertext := aead.Seal(nil, nonce, pair.privateKey, aad)
	return "pgchallenge.v1." + base64.RawURLEncoding.EncodeToString(nonce) + "." + base64.RawURLEncoding.EncodeToString(ciphertext), nil
}

func OpenChallengePrivateKey(sealed, sealKey string, challenge domaincluster.EnrollmentChallenge) (EphemeralKeyPair, error) {
	parts := strings.Split(strings.TrimSpace(sealed), ".")
	if len(parts) != 4 || parts[0] != "pgchallenge" || parts[1] != "v1" {
		return EphemeralKeyPair{}, errors.New("sealed challenge key is invalid")
	}
	aead, aad, err := challengeSealAEAD(sealKey, challenge)
	if err != nil {
		return EphemeralKeyPair{}, err
	}
	nonce, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(nonce) != aead.NonceSize() {
		return EphemeralKeyPair{}, errors.New("sealed challenge nonce is invalid")
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil || len(ciphertext) < aead.Overhead() {
		return EphemeralKeyPair{}, errors.New("sealed challenge ciphertext is invalid")
	}
	privateKey, err := aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return EphemeralKeyPair{}, errors.New("sealed challenge authentication failed")
	}
	parsed, err := ecdh.X25519().NewPrivateKey(privateKey)
	if err != nil {
		clear(privateKey)
		return EphemeralKeyPair{}, errors.New("sealed challenge private key is invalid")
	}
	publicKey := base64.RawURLEncoding.EncodeToString(parsed.PublicKey().Bytes())
	if publicKey != challenge.ServerPublicKey {
		clear(privateKey)
		return EphemeralKeyPair{}, errors.New("sealed challenge key does not match transcript")
	}
	return EphemeralKeyPair{privateKey: privateKey, publicKey: publicKey}, nil
}

func challengeSealAEAD(sealKey string, challenge domaincluster.EnrollmentChallenge) (cipherAEAD, []byte, error) {
	key, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(sealKey))
	if err != nil || len(key) != chacha20poly1305.KeySize {
		return nil, nil, errors.New("cluster enrollment seal key is invalid")
	}
	defer clear(key)
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, nil, fmt.Errorf("create challenge seal AEAD: %w", err)
	}
	transcript, _, err := enrollmentTranscript(challenge, challenge.NodeID)
	if err != nil {
		return nil, nil, err
	}
	aad := appendLengthPrefixed(nil, challengeSealDomain)
	aad = append(aad, transcript...)
	return aead, aad, nil
}

func serverEnvelopeAEAD(server EphemeralKeyPair, tokenProofPublicKey string, challenge domaincluster.EnrollmentChallenge, nodeID string) (cipherAEAD, []byte, error) {
	authShared, err := x25519Shared(server.privateKey, tokenProofPublicKey)
	if err != nil {
		return nil, nil, err
	}
	defer clear(authShared)
	nodeShared, err := x25519Shared(server.privateKey, challenge.ClientPublicKey)
	if err != nil {
		return nil, nil, err
	}
	defer clear(nodeShared)
	return enrollmentEnvelopeAEAD(authShared, nodeShared, challenge, nodeID)
}

func clientEnvelopeAEAD(client EphemeralKeyPair, tokenProof TokenProofKeyPair, challenge domaincluster.EnrollmentChallenge, nodeID string) (cipherAEAD, []byte, error) {
	authShared, err := x25519Shared(tokenProof.privateKey, challenge.ServerPublicKey)
	if err != nil {
		return nil, nil, err
	}
	defer clear(authShared)
	nodeShared, err := x25519Shared(client.privateKey, challenge.ServerPublicKey)
	if err != nil {
		return nil, nil, err
	}
	defer clear(nodeShared)
	return enrollmentEnvelopeAEAD(authShared, nodeShared, challenge, nodeID)
}

type cipherAEAD interface {
	NonceSize() int
	Overhead() int
	Seal(dst, nonce, plaintext, additionalData []byte) []byte
	Open(dst, nonce, ciphertext, additionalData []byte) ([]byte, error)
}

func enrollmentEnvelopeAEAD(authShared, nodeShared []byte, challenge domaincluster.EnrollmentChallenge, nodeID string) (cipherAEAD, []byte, error) {
	transcript, transcriptHash, err := enrollmentTranscript(challenge, nodeID)
	if err != nil {
		return nil, nil, err
	}
	input := append(append(make([]byte, 0, len(authShared)+len(nodeShared)), authShared...), nodeShared...)
	defer clear(input)
	key := make([]byte, chacha20poly1305.KeySize)
	info := append([]byte(envelopeKeyDomain), transcriptHash[:]...)
	if _, err := io.ReadFull(hkdf.New(sha256.New, input, transcriptHash[:], info), key); err != nil {
		return nil, nil, fmt.Errorf("derive runtime envelope key: %w", err)
	}
	defer clear(key)
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, nil, fmt.Errorf("create runtime envelope AEAD: %w", err)
	}
	aad := appendLengthPrefixed(nil, envelopeAADDomain)
	aad = append(aad, transcript...)
	return aead, aad, nil
}

func deriveProofKey(authShared []byte, serverNonce string, transcriptHash [sha256.Size]byte) ([]byte, error) {
	nonce, err := base64.RawURLEncoding.DecodeString(serverNonce)
	if err != nil || len(nonce) != 32 {
		return nil, errors.New("server nonce is invalid")
	}
	key := make([]byte, sha256.Size)
	info := append([]byte(proofKeyDomain), transcriptHash[:]...)
	if _, err := io.ReadFull(hkdf.New(sha256.New, authShared, nonce, info), key); err != nil {
		return nil, fmt.Errorf("derive possession proof key: %w", err)
	}
	return key, nil
}

func enrollmentTranscript(challenge domaincluster.EnrollmentChallenge, nodeID string) ([]byte, [sha256.Size]byte, error) {
	if challenge.Protocol != EnrollmentProtocolV1 || challenge.ChallengeID == "" || challenge.InstallationID == "" || challenge.TokenID == "" || challenge.Role == "" || strings.TrimSpace(nodeID) == "" || challenge.NodeID != strings.TrimSpace(nodeID) || challenge.ClientPublicKey == "" || challenge.ServerPublicKey == "" || challenge.ServerNonce == "" || challenge.ApplicationVersion == "" || challenge.RuntimeSchemaVersion <= 0 || challenge.ConfigRevision <= 0 || challenge.ExpiresAt.IsZero() {
		return nil, [sha256.Size]byte{}, errors.New("enrollment challenge binding is invalid")
	}
	transcript := appendLengthPrefixed(nil, transcriptDomain)
	for _, value := range []string{
		challenge.Protocol, challenge.ChallengeID, challenge.InstallationID, challenge.TokenID, string(challenge.Role),
		challenge.NodeID, challenge.ClientPublicKey, challenge.ServerPublicKey, challenge.ServerNonce, challenge.ApplicationVersion,
	} {
		transcript = appendLengthPrefixed(transcript, value)
	}
	var number [8]byte
	binary.BigEndian.PutUint32(number[:4], uint32(challenge.RuntimeSchemaVersion))
	transcript = append(transcript, number[:4]...)
	binary.BigEndian.PutUint64(number[:], uint64(challenge.ConfigRevision))
	transcript = append(transcript, number[:]...)
	binary.BigEndian.PutUint64(number[:], uint64(challenge.ExpiresAt.UTC().Unix()))
	transcript = append(transcript, number[:]...)
	digest := sha256.Sum256(transcript)
	return transcript, digest, nil
}

func validateRuntimePayload(payload domaincluster.RuntimeEnvelopePayload, challenge domaincluster.EnrollmentChallenge, nodeID string) error {
	if payload.Protocol != EnrollmentProtocolV1 || payload.InstallationID != challenge.InstallationID || payload.NodeID != strings.TrimSpace(nodeID) || payload.Role != domaincluster.NodeRole(challenge.Role) || payload.ApplicationVersion != challenge.ApplicationVersion || payload.RuntimeSchemaVersion != challenge.RuntimeSchemaVersion || payload.ConfigRevision != challenge.ConfigRevision || payload.Values == nil {
		return errors.New("runtime envelope binding is invalid")
	}
	return nil
}

func generateX25519Key(entropy io.Reader) ([]byte, string, error) {
	if entropy == nil {
		return nil, "", errors.New("ephemeral key entropy is required")
	}
	privateBytes := make([]byte, 32)
	if _, err := io.ReadFull(entropy, privateBytes); err != nil {
		return nil, "", fmt.Errorf("read ephemeral key entropy: %w", err)
	}
	privateKey, err := ecdh.X25519().NewPrivateKey(privateBytes)
	if err != nil {
		clear(privateBytes)
		return nil, "", fmt.Errorf("create ephemeral key: %w", err)
	}
	return privateBytes, base64.RawURLEncoding.EncodeToString(privateKey.PublicKey().Bytes()), nil
}

func x25519Shared(privateKey []byte, publicKey string) ([]byte, error) {
	private, err := ecdh.X25519().NewPrivateKey(privateKey)
	if err != nil {
		return nil, errors.New("X25519 private key is invalid")
	}
	publicBytes, err := base64.RawURLEncoding.DecodeString(publicKey)
	if err != nil || len(publicBytes) != 32 {
		return nil, errors.New("X25519 public key is invalid")
	}
	public, err := ecdh.X25519().NewPublicKey(publicBytes)
	if err != nil {
		return nil, errors.New("X25519 public key is invalid")
	}
	shared, err := private.ECDH(public)
	if err != nil {
		return nil, errors.New("X25519 key agreement failed")
	}
	return shared, nil
}

func ValidateX25519PublicKey(publicKey string) error {
	probePrivate := make([]byte, 32)
	probePrivate[0] = 1
	defer clear(probePrivate)
	shared, err := x25519Shared(probePrivate, publicKey)
	if err != nil {
		return err
	}
	clear(shared)
	return nil
}

func credentialSecret(credential string) (string, []byte, error) {
	parts := strings.Split(strings.TrimSpace(credential), ".")
	if len(parts) != 4 || parts[0] != "pgjoin" || parts[1] != "v1" {
		return "", nil, errors.New("invalid cluster token format")
	}
	if _, _, err := parseTokenCredential(credential); err != nil {
		return "", nil, err
	}
	secret, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil || len(secret) != 32 {
		return "", nil, errors.New("invalid cluster token secret")
	}
	return parts[2], secret, nil
}

func appendLengthPrefixed(target []byte, value string) []byte {
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(value)))
	target = append(target, length[:]...)
	return append(target, value...)
}

func redactedKeyJSON(publicKey string) ([]byte, error) {
	return json.Marshal(struct {
		PublicKey  string `json:"public_key"`
		PrivateKey string `json:"private_key"`
	}{PublicKey: publicKey, PrivateKey: "REDACTED"})
}

func RuntimeKeysForRole(role domaincluster.NodeRole) []string {
	database := []string{"DATABASE_URL", "DATABASE_MAX_OPEN_CONNS", "DATABASE_MAX_IDLE_CONNS", "DATABASE_CONN_MAX_LIFETIME"}
	redis := []string{"REDIS_URL", "REDIS_KEY_PREFIX"}
	storage := []string{
		"STORAGE_DRIVER", "STORAGE_LOCAL_ROOT", "STORAGE_PUBLIC_BASE_URL", "STORAGE_SHARED_VOLUME",
		"STORAGE_S3_ENDPOINT", "STORAGE_S3_REGION", "STORAGE_S3_BUCKET", "STORAGE_S3_ACCESS_KEY_ID",
		"STORAGE_S3_SECRET_ACCESS_KEY", "STORAGE_S3_FORCE_PATH_STYLE", "STORAGE_S3_PREFIX",
	}
	var keys []string
	switch role {
	case domaincluster.NodeRoleAPI:
		keys = append(keys, database...)
		keys = append(keys, redis...)
		keys = append(keys, storage...)
		keys = append(keys,
			"AUTH_ACCESS_TOKEN_SECRET", "API_KEY_SIGNING_SECRET_ENCRYPTION_KEY",
			"CASHIER_PROVIDER_CONFIG_ENCRYPTION_KEY", "PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY",
			"PROMPT_OPTIMIZATION_QUOTE_SIGNING_KEY", "PUBLIC_API_URL", "PIC_GALLERY_DOCS_URL", "PIC_GALLERY_DOCS_PROBE_URL", "CORS_ALLOWED_ORIGINS",
		)
	case domaincluster.NodeRoleWorker:
		keys = append(keys, database...)
		keys = append(keys, redis...)
		keys = append(keys, storage...)
		keys = append(keys, "PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY")
	case domaincluster.NodeRoleWeb:
		keys = append(keys, "PUBLIC_API_URL", "PIC_GALLERY_DOCS_URL")
	default:
		return nil
	}
	return keys
}
