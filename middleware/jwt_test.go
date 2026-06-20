package middleware

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func b64(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

func buildRawToken(t *testing.T, priv ed25519.PrivateKey, headerJSON, claimsJSON []byte) string {
	t.Helper()
	signingInput := b64(headerJSON) + "." + b64(claimsJSON)
	var sig []byte
	if priv != nil {
		sig = ed25519.Sign(priv, []byte(signingInput))
	}
	return signingInput + "." + b64(sig)
}

func TestCreateAndValidate_RoundTrip(t *testing.T) {
	h := createHandler("12345")

	token, err := h.createToken("user-123", time.Hour)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	sub, ok := h.validateToken(token)
	if !ok {
		t.Fatalf("Validate: expected ok=true, got false")
	}
	if sub != "user-123" {
		t.Errorf("Validate: got sub %q, want %q", sub, "user-123")
	}
}

func TestValidate_WithSeparatePublicKeyOnlyHandler(t *testing.T) {
	signer := createHandler("12345")
	token, err := signer.createToken("user-123", time.Hour)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// A verify-only handler (no private key), as would be used on a
	// downstream service that only has the public key.
	verifier, err := newHandler(signer.publicKey, nil)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	sub, ok := verifier.validateToken(token)
	if !ok || sub != "user-123" {
		t.Errorf("Validate on verify-only handler: got (%q, %v), want (%q, true)", sub, ok, "user-123")
	}
}

func TestCreate_VerifyOnlyHandlerCannotSign(t *testing.T) {
	h := createHandler("12345")
	verifyOnly, err := newHandler(h.publicKey, nil)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	if _, err := verifyOnly.createToken("user-123", time.Hour); err == nil {
		t.Error("Create on verify-only handler: expected error, got nil")
	}
}

func TestCreate_RejectsEmptySubject(t *testing.T) {
	h := createHandler("12345")
	if _, err := h.createToken("", time.Hour); err == nil {
		t.Error("Create with empty sub: expected error, got nil")
	}
}

// --- tampering / forgery -----------------------------------------------

func TestValidate_RejectsTamperedPayload(t *testing.T) {
	h := createHandler("12345")
	token, _ := h.createToken("user-123", time.Hour)

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("unexpected token shape: %d parts", len(parts))
	}

	// Forge a new payload claiming a different subject, but keep the
	// original signature -- this must fail.
	forgedClaims, _ := json.Marshal(claims{
		Sub: "admin",
		Iat: time.Now().Unix(),
		Exp: time.Now().Add(time.Hour).Unix(),
	})
	tampered := parts[0] + "." + b64(forgedClaims) + "." + parts[2]

	if sub, ok := h.validateToken(tampered); ok {
		t.Errorf("Validate accepted tampered payload, returned sub=%q", sub)
	}
}

func TestValidate_RejectsTamperedHeader(t *testing.T) {
	h := createHandler("12345")
	token, _ := h.createToken("user-123", time.Hour)
	parts := strings.Split(token, ".")

	forgedHeader, _ := json.Marshal(header{Alg: algorithm, Typ: "weird"})
	tampered := b64(forgedHeader) + "." + parts[1] + "." + parts[2]

	if sub, ok := h.validateToken(tampered); ok {
		t.Errorf("Validate accepted tampered header, returned sub=%q", sub)
	}
}

func TestValidate_RejectsBitFlippedSignature(t *testing.T) {
	h := createHandler("12345")
	token, _ := h.createToken("user-123", time.Hour)
	parts := strings.Split(token, ".")

	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("decode sig: %v", err)
	}
	sig[0] ^= 0xFF // flip bits in the first byte
	tampered := parts[0] + "." + parts[1] + "." + b64(sig)

	if sub, ok := h.validateToken(tampered); ok {
		t.Errorf("Validate accepted flipped-bit signature, returned sub=%q", sub)
	}
}

func TestValidate_RejectsTruncatedSignature(t *testing.T) {
	h := createHandler("12345")
	token, _ := h.createToken("user-123", time.Hour)
	parts := strings.Split(token, ".")

	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	truncated := b64(sig[:len(sig)-1])
	tampered := parts[0] + "." + parts[1] + "." + truncated

	if sub, ok := h.validateToken(tampered); ok {
		t.Errorf("Validate accepted truncated signature, returned sub=%q", sub)
	}
}

func TestValidate_RejectsEmptySignature(t *testing.T) {
	h := createHandler("12345")
	token, _ := h.createToken("user-123", time.Hour)
	parts := strings.Split(token, ".")

	tampered := parts[0] + "." + parts[1] + "."

	if sub, ok := h.validateToken(tampered); ok {
		t.Errorf("Validate accepted empty signature, returned sub=%q", sub)
	}
}

// --- algorithm confusion -------------------------------------------------

func TestValidate_RejectsAlgNone(t *testing.T) {
	h := createHandler("12345")

	// Classic "alg: none" forgery attempt: claim no signature is
	// needed at all.
	forgedHeader, _ := json.Marshal(map[string]string{"alg": "none", "typ": "JWT"})
	forgedClaims, _ := json.Marshal(claims{
		Sub: "admin",
		Iat: time.Now().Unix(),
		Exp: time.Now().Add(time.Hour).Unix(),
	})
	token := b64(forgedHeader) + "." + b64(forgedClaims) + "."

	if sub, ok := h.validateToken(token); ok {
		t.Errorf("Validate accepted alg=none token, returned sub=%q", sub)
	}
}

func TestValidate_RejectsDifferentAlgWithValidLookingStructure(t *testing.T) {
	h := createHandler("12345")

	// Header claims HS256 even though we generated this with the
	// Ed25519 helper -- the alg field is attacker controlled and
	// must never be trusted to select verification behavior.
	forgedHeader, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	claimsJSON, _ := json.Marshal(claims{
		Sub: "user-123",
		Iat: time.Now().Unix(),
		Exp: time.Now().Add(time.Hour).Unix(),
	})
	token := buildRawToken(t, h.privateKey, forgedHeader, claimsJSON)

	if sub, ok := h.validateToken(token); ok {
		t.Errorf("Validate accepted non-EdDSA alg header, returned sub=%q", sub)
	}
}

func TestValidate_RejectsMissingAlgField(t *testing.T) {
	h := createHandler("12345")

	forgedHeader, _ := json.Marshal(map[string]string{"typ": "JWT"}) // no "alg" at all
	claimsJSON, _ := json.Marshal(claims{
		Sub: "user-123",
		Iat: time.Now().Unix(),
		Exp: time.Now().Add(time.Hour).Unix(),
	})
	token := buildRawToken(t, h.privateKey, forgedHeader, claimsJSON)

	if sub, ok := h.validateToken(token); ok {
		t.Errorf("Validate accepted token with missing alg, returned sub=%q", sub)
	}
}

// --- key confusion ------------------------------------------------------

func TestValidate_RejectsTokenSignedByDifferentKeyPair(t *testing.T) {
	legit := createHandler("12345")
	attacker := createHandler("54321") // different key pair entirely

	// Attacker signs a token with their own key, but we only trust
	// legit's public key for verification.
	forgedToken, err := attacker.createToken("admin", time.Hour)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if sub, ok := legit.validateToken(forgedToken); ok {
		t.Errorf("Validate accepted token signed by a different key pair, returned sub=%q", sub)
	}
}

func TestValidate_PublicKeyMustMatchSigningKey(t *testing.T) {
	correctH := createHandler("12345")
	wrongH := createHandler("54321")

	token, _ := correctH.createToken("user-123", time.Hour)

	if sub, ok := wrongH.validateToken(token); ok {
		t.Errorf("Validate accepted token verified against mismatched public key, returned sub=%q", sub)
	}
}

func TestValidate_RejectsExpiredToken(t *testing.T) {
	h := createHandler("12345")
	token, _ := h.createToken("user-123", -time.Minute) // already expired

	if sub, ok := h.validateToken(token); ok {
		t.Errorf("Validate accepted expired token, returned sub=%q", sub)
	}
}

func TestValidate_RejectsTokenExactlyAtExpiry(t *testing.T) {
	h := createHandler("12345")

	// Build a token whose exp equals "now" -- treat the boundary as
	// expired (now >= exp), not valid.
	now := time.Now().Unix()
	claimsJSON, _ := json.Marshal(claims{Sub: "user-123", Iat: now - 1, Exp: now})
	headerJSON, _ := json.Marshal(header{Alg: algorithm, Typ: "JWT"})
	token := buildRawToken(t, h.privateKey, headerJSON, claimsJSON)

	if sub, ok := h.validateToken(token); ok {
		t.Errorf("Validate accepted token at exact expiry boundary, returned sub=%q", sub)
	}
}

func TestValidate_AcceptsTokenJustBeforeExpiry(t *testing.T) {
	h := createHandler("12345")
	token, _ := h.createToken("user-123", 2*time.Second)

	sub, ok := h.validateToken(token)
	if !ok || sub != "user-123" {
		t.Errorf("Validate rejected still-valid token: sub=%q ok=%v", sub, ok)
	}
}

func TestValidate_RejectsFutureIat(t *testing.T) {
	h := createHandler("12345")

	future := time.Now().Add(time.Hour).Unix()
	claimsJSON, _ := json.Marshal(claims{Sub: "user-123", Iat: future, Exp: future + 3600})
	headerJSON, _ := json.Marshal(header{Alg: algorithm, Typ: "JWT"})
	token := buildRawToken(t, h.privateKey, headerJSON, claimsJSON)

	if sub, ok := h.validateToken(token); ok {
		t.Errorf("Validate accepted token with iat in the future, returned sub=%q", sub)
	}
}

func TestValidate_RejectsEmptySubjectInClaims(t *testing.T) {
	h := createHandler("12345")

	now := time.Now().Unix()
	claimsJSON, _ := json.Marshal(claims{Sub: "", Iat: now, Exp: now + 3600})
	headerJSON, _ := json.Marshal(header{Alg: algorithm, Typ: "JWT"})
	// Sign this forged-but-correctly-signed claims block with the
	// legitimate key, to isolate the empty-sub check from signature
	// verification.
	token := buildRawToken(t, h.privateKey, headerJSON, claimsJSON)

	if sub, ok := h.validateToken(token); ok {
		t.Errorf("Validate accepted token with empty sub claim, returned sub=%q", sub)
	}
}

// --- malformed input / parser robustness ---------------------------------

func TestValidate_RejectsMalformedTokens(t *testing.T) {
	h := createHandler("12345")

	validHeaderJSON, _ := json.Marshal(header{Alg: algorithm, Typ: "JWT"})
	validHeaderB64 := b64(validHeaderJSON)

	cases := map[string]string{
		"empty string":          "",
		"single segment":        "abc",
		"two segments":          "abc.def",
		"four segments":         "abc.def.ghi.jkl",
		"only dots":             "..",
		"invalid base64 header": "not-valid-b64!!.def.ghi",
		"invalid base64 claims": "abc.not-valid-b64!!.ghi",
		"invalid base64 sig":    "abc.def.not-valid-b64!!",
		"header not JSON":       b64([]byte("not json")) + ".def.ghi",
		"claims not JSON":       validHeaderB64 + "." + b64([]byte("not json")) + ".ghi",
	}

	for name, token := range cases {
		t.Run(name, func(t *testing.T) {
			if sub, ok := h.validateToken(token); ok {
				t.Errorf("Validate(%q) = (%q, true), want ok=false", token, sub)
			}
		})
	}
}

func TestValidate_DoesNotPanicOnAdversarialInput(t *testing.T) {
	h := createHandler("12345")

	// A grab-bag of inputs designed to probe for panics (index out of
	// range, nil deref, etc.) rather than just wrong results. A panic
	// here would be a DoS vector if this ever sits behind a public
	// endpoint that calls Validate directly on user input.
	inputs := []string{
		"",
		".",
		"..",
		"...",
		strings.Repeat("a", 100000),
		strings.Repeat("a.", 50000) + "a",
		"\x00\x00\x00",
		"🙂.🙂.🙂",
		"a.b.c.d.e.f.g",
	}

	for _, in := range inputs {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Validate(%.20q...) panicked: %v", in, r)
				}
			}()
			h.validateToken(in)
		}()
	}
}

// --- cross-token isolation -----------------------------------------------

func TestValidate_RejectsMixAndMatchSegmentsFromTwoValidTokens(t *testing.T) {
	h := createHandler("12345")

	tokenA, _ := h.createToken("alice", time.Hour)
	tokenB, _ := h.createToken("bob", time.Hour)

	partsA := strings.Split(tokenA, ".")
	partsB := strings.Split(tokenB, ".")

	// Take alice's header+claims but bob's signature. Both tokens are
	// independently valid, but this Frankenstein combination must not
	// validate.
	mixed := partsA[0] + "." + partsA[1] + "." + partsB[2]

	if sub, ok := h.validateToken(mixed); ok {
		t.Errorf("Validate accepted mixed-and-matched token segments, returned sub=%q", sub)
	}
}

func TestCreate_TokensForSameSubjectAreNotIdentical(t *testing.T) {
	// Not a strict security requirement, but a sanity check that
	// iat/exp are actually being set per-call rather than cached.
	h := createHandler("12345")

	t1, _ := h.createToken("user-123", time.Hour)
	time.Sleep(1100 * time.Millisecond) // ensure iat (second resolution) differs
	t2, _ := h.createToken("user-123", time.Hour)

	if t1 == t2 {
		t.Error("two tokens created at different times for the same subject were identical")
	}
}
