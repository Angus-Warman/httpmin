package middleware

import (
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

type keyHandler struct {
	publicKey  ed25519.PublicKey
	privateKey ed25519.PrivateKey
}

func jsonTo[T any](data []byte) (T, error) {
	var into T

	err := json.Unmarshal(data, &into)

	if err != nil {
		var zero T
		return zero, err
	}

	return into, nil
}

func newHandler(pub ed25519.PublicKey, priv ed25519.PrivateKey) (*keyHandler, error) {
	if len(pub) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("jwt: public key must be %d bytes", ed25519.PublicKeySize)
	}

	if priv != nil && len(priv) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("jwt: private key must be %d bytes", ed25519.PrivateKeySize)
	}

	return &keyHandler{publicKey: pub, privateKey: priv}, nil
}

func createHandler(secret string) *keyHandler {
	seed := sha256.Sum256([]byte(secret))
	priv := ed25519.NewKeyFromSeed(seed[:])
	pub := priv.Public().(ed25519.PublicKey)

	return &keyHandler{
		publicKey:  pub,
		privateKey: priv,
	}
}

func (h *keyHandler) createToken(sub string, validFor time.Duration) (string, error) {
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

func (h *keyHandler) getSubject(token string) (string, error) {
	c, err := h.validateToken(token)

	if err != nil {
		return "", err
	}

	return c.Sub, err
}

func (h *keyHandler) validateToken(token string) (claims, error) {
	var zero claims

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return zero, errors.New("jwt: malformed token")
	}
	headerB64, claimsB64, sigB64 := parts[0], parts[1], parts[2]

	headerJSON, err := base64.RawURLEncoding.DecodeString(headerB64)

	if err != nil {
		return zero, fmt.Errorf("jwt: decode header: %w", err)
	}

	hdr, err := jsonTo[header](headerJSON)

	if err != nil {
		return zero, fmt.Errorf("jwt: decode header: %w", err)
	}

	// Hard-coded check
	if hdr.Alg != algorithm {
		return zero, errors.New("jwt: unexpected algorithm")
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

	c, err := jsonTo[claims](claimsJSON)

	if err != nil {
		return zero, fmt.Errorf("jwt: decode claims: %w", err)
	}

	now := time.Now().Unix()

	if c.Exp != 0 && now >= c.Exp {
		return zero, errors.New("jwt: token expired")
	}

	if c.Iat != 0 && now < c.Iat {
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
