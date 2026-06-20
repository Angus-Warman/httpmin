package middleware

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const algorithm = "EdDSA"

type header struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

type claims struct {
	Sub string `json:"sub"`
	Iat int64  `json:"iat"`
	Exp int64  `json:"exp"`
}

type jwtHandler struct {
	publicKey  ed25519.PublicKey
	privateKey ed25519.PrivateKey
}

// Rejects anything with duplicate keys
func jsonDecodeStrict[T any](data []byte) (T, error) {
	var zero T

	dec := json.NewDecoder(bytes.NewReader(data))
	t, err := dec.Token()
	if err != nil {
		return zero, err
	}
	if t != json.Delim('{') {
		return zero, errors.New("expected JSON object")
	}

	seen := map[string]bool{}
	for dec.More() {
		key, err := dec.Token()

		if err != nil {
			return zero, err
		}

		k := key.(string)

		if seen[k] {
			return zero, fmt.Errorf("duplicate key: %s", k)
		}

		seen[k] = true

		var raw json.RawMessage

		if err := dec.Decode(&raw); err != nil {
			return zero, err
		}
	}
	// Consume closing '}'
	if _, err := dec.Token(); err != nil {
		return zero, err
	}

	// Everything is fine, so use a normal Unmarshal

	var result T

	if err := json.Unmarshal(data, &result); err != nil {
		return zero, err
	}

	return result, nil
}

func newHandler(pub ed25519.PublicKey, priv ed25519.PrivateKey) (*jwtHandler, error) {
	if len(pub) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("jwt: public key must be %d bytes", ed25519.PublicKeySize)
	}

	if priv != nil && len(priv) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("jwt: private key must be %d bytes", ed25519.PrivateKeySize)
	}

	return &jwtHandler{publicKey: pub, privateKey: priv}, nil
}

func createJwtHandler(secret string) *jwtHandler {
	seed := sha256.Sum256([]byte(secret))
	priv := ed25519.NewKeyFromSeed(seed[:])
	pub := priv.Public().(ed25519.PublicKey)

	return &jwtHandler{
		publicKey:  pub,
		privateKey: priv,
	}
}

func (h *jwtHandler) createToken(sub string, validFor time.Duration) (string, error) {
	if h.privateKey == nil {
		return "", errors.New("jwt: handler has no private key, cannot sign")
	}

	if sub == "" {
		return "", errors.New("jwt: sub must not be empty")
	}

	now := time.Now()

	c := claims{
		Sub: sub,
		Iat: now.Unix(),
		Exp: now.Add(validFor).Unix(),
	}

	headerJSON, err := json.Marshal(header{Alg: algorithm, Typ: "JWT"})

	if err != nil {
		return "", fmt.Errorf("jwt: marshal header: %w", err)
	}

	claimsJSON, err := json.Marshal(c)

	if err != nil {
		return "", fmt.Errorf("jwt: marshal claims: %w", err)
	}

	signingInput := encodeSegment(headerJSON) + "." + encodeSegment(claimsJSON)
	sig := ed25519.Sign(h.privateKey, []byte(signingInput))

	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func (h *jwtHandler) getSubject(token string) (string, error) {
	c, err := h.validateToken(token)

	if err != nil {
		return "", err
	}

	return c.Sub, nil
}

func (h *jwtHandler) validateToken(token string) (claims, error) {
	var zero claims

	if len(token) > (8 * 1024) {
		return zero, errors.New("jwt: token too large")
	}

	parts := strings.Split(token, ".")

	if len(parts) != 3 {
		return zero, errors.New("jwt: malformed token")
	}

	headerB64, claimsB64, sigB64 := parts[0], parts[1], parts[2]

	if len(headerB64) == 0 || len(claimsB64) == 0 || len(sigB64) == 0 {
		return zero, errors.New("jwt: malformed token")
	}

	headerJSON, err := base64.RawURLEncoding.DecodeString(headerB64)

	if err != nil {
		return zero, fmt.Errorf("jwt: decode header: %w", err)
	}

	hdr, err := jsonDecodeStrict[header](headerJSON)

	if err != nil {
		return zero, fmt.Errorf("jwt: decode header: %w", err)
	}

	// Hard-coded checks
	if hdr.Alg != algorithm {
		return zero, errors.New("jwt: unexpected algorithm")
	}

	if hdr.Typ != "JWT" {
		return zero, errors.New("jwt: unexpected type")
	}

	sig, err := base64.RawURLEncoding.DecodeString(sigB64)

	if err != nil {
		return zero, fmt.Errorf("jwt: decode signature: %w", err)
	}

	signingInput := headerB64 + "." + claimsB64

	if !ed25519.Verify(h.publicKey, []byte(signingInput), sig) {
		return zero, errors.New("jwt: invalid signature")
	}

	claimsJSON, err := base64.RawURLEncoding.DecodeString(claimsB64)

	if err != nil {
		return zero, fmt.Errorf("jwt: decode claims: %w", err)
	}

	c, err := jsonDecodeStrict[claims](claimsJSON)

	if err != nil {
		return zero, fmt.Errorf("jwt: decode claims: %w", err)
	}

	now := time.Now().Unix()

	if c.Exp == 0 {
		return zero, errors.New("jwt: no expiry")
	}

	if c.Iat == 0 {
		return zero, errors.New("jwt: no iat")
	}

	if now >= c.Exp {
		return zero, errors.New("jwt: token expired")
	}

	if now < c.Iat {
		return zero, errors.New("jwt: token not yet valid")
	}

	if c.Sub == "" {
		return zero, errors.New("jwt: sub must not be empty")
	}

	return c, nil
}

func encodeSegment(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}
