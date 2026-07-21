package setup

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"net/http"
	"time"
)

const (
	sessionVersionSize   = 8
	sessionExpirySize    = 8
	sessionNonceSize     = 32
	sessionPayloadSize   = sessionVersionSize + sessionExpirySize + sessionNonceSize
	sessionSignatureSize = sha256.Size
	sessionRawSize       = sessionPayloadSize + sessionSignatureSize
	setupSessionEncoded  = 107

	SetupSessionCookieName = "setup_session"
)

func signSession(version uint64, now time.Time, ttl time.Duration, nonce, signingKey []byte) (string, error) {
	if version == 0 || len(nonce) != sessionNonceSize || len(signingKey) != sha256.Size {
		return "", ErrInvalidConfiguration
	}
	expires := now.Add(ttl)
	if now.Unix() < 0 || !expires.After(now) || expires.Unix() <= now.Unix() || expires.Year() > 9999 {
		return "", ErrClock
	}
	raw := make([]byte, sessionRawSize)
	binary.BigEndian.PutUint64(raw[:sessionVersionSize], version)
	binary.BigEndian.PutUint64(raw[sessionVersionSize:sessionVersionSize+sessionExpirySize], uint64(expires.Unix()))
	copy(raw[sessionVersionSize+sessionExpirySize:sessionPayloadSize], nonce)
	signature := sessionSignature(raw[:sessionPayloadSize], signingKey)
	copy(raw[sessionPayloadSize:], signature)
	clear(signature)
	encoded := base64.RawURLEncoding.EncodeToString(raw)
	clear(raw)
	if len(encoded) != setupSessionEncoded {
		return "", ErrInvalidSession
	}
	return encoded, nil
}

func decodeSession(session string) ([]byte, bool) {
	if len(session) != setupSessionEncoded || len(session) > maxTokenInputBytes || session == "" {
		return nil, false
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(session)
	if err != nil || len(raw) != sessionRawSize || base64.RawURLEncoding.EncodeToString(raw) != session {
		clear(raw)
		return nil, false
	}
	return raw, true
}

func verifySignedSession(raw []byte, expectedVersion uint64, now time.Time, signingKey []byte) bool {
	if len(raw) != sessionRawSize || len(signingKey) != sha256.Size {
		return false
	}
	expectedSignature := sessionSignature(raw[:sessionPayloadSize], signingKey)
	signatureValid := subtle.ConstantTimeCompare(expectedSignature, raw[sessionPayloadSize:]) == 1
	clear(expectedSignature)
	version := binary.BigEndian.Uint64(raw[:sessionVersionSize])
	expiry := int64(binary.BigEndian.Uint64(raw[sessionVersionSize : sessionVersionSize+sessionExpirySize]))
	claimsValid := version == expectedVersion && expiry > 0 && now.Unix() < expiry
	return signatureValid && claimsValid
}

func sessionSignature(payload, signingKey []byte) []byte {
	mac := hmac.New(sha256.New, signingKey)
	_, _ = mac.Write(payload)
	return mac.Sum(nil)
}

func NewSetupSessionCookie(value string, now time.Time, ttl time.Duration, secure bool) (*http.Cookie, error) {
	if ttl < minSessionTTL || ttl > maxSessionTTL || now.IsZero() {
		return nil, ErrInvalidConfiguration
	}
	expires := now.Add(ttl).UTC()
	maxAge := int(ttl / time.Second)
	if ttl%time.Second != 0 {
		maxAge++
	}
	return &http.Cookie{
		Name: SetupSessionCookieName, Value: value, Path: "/",
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteStrictMode,
		Expires: expires, MaxAge: maxAge,
	}, nil
}

func ClearSetupSessionCookie(secure bool) *http.Cookie {
	return &http.Cookie{
		Name: SetupSessionCookieName, Path: "/", HttpOnly: true,
		Secure: secure, SameSite: http.SameSiteStrictMode,
		Expires: time.Unix(1, 0).UTC(), MaxAge: -1,
	}
}
