package wallet

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizeNetwork(t *testing.T) {
	for in, want := range map[string]string{
		"base":         "eip155:8453",
		"eip155:8453":  "eip155:8453",
		"base-sepolia": "eip155:84532",
		"eip155:84532": "eip155:84532",
		"solana":       "solana", // passthrough
	} {
		if got := normalizeNetwork(in); got != want {
			t.Errorf("normalizeNetwork(%q)=%q want %q", in, got, want)
		}
	}
}

// TestCDPBearer certifies the JWT is well-formed (three segments, EdDSA header,
// correct claims) and that its signature verifies against the key — so a bad
// signing path can't silently ship.
func TestCDPBearer(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	cdpKeyID = "test-key-id"
	cdpKeySecret = base64.StdEncoding.EncodeToString(priv) // 64-byte seed+pub
	defer func() { cdpKeyID, cdpKeySecret = "", "" }()

	tok, err := cdpBearer("POST", "api.cdp.coinbase.com", "/platform/v2/x402/verify")
	if err != nil {
		t.Fatalf("cdpBearer: %v", err)
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 JWT segments, got %d", len(parts))
	}

	// Signature must verify over "header.payload".
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("decode sig: %v", err)
	}
	if !ed25519.Verify(pub, []byte(parts[0]+"."+parts[1]), sig) {
		t.Fatal("JWT signature does not verify")
	}

	var hdr map[string]any
	hb, _ := base64.RawURLEncoding.DecodeString(parts[0])
	_ = json.Unmarshal(hb, &hdr)
	if hdr["alg"] != "EdDSA" || hdr["kid"] != "test-key-id" || hdr["nonce"] == nil {
		t.Errorf("bad header: %v", hdr)
	}

	var claims map[string]any
	cb, _ := base64.RawURLEncoding.DecodeString(parts[1])
	_ = json.Unmarshal(cb, &claims)
	if claims["iss"] != "cdp" || claims["sub"] != "test-key-id" {
		t.Errorf("bad claims: %v", claims)
	}
	if claims["uri"] != "POST api.cdp.coinbase.com/platform/v2/x402/verify" {
		t.Errorf("bad uri claim: %v", claims["uri"])
	}
}
