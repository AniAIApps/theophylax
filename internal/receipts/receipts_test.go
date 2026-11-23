package receipts

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

func testSigner(t *testing.T) *Signer {
	t.Helper()
	s, err := NewSigner(nil, bytes.Repeat([]byte{0x2a}, 32))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func keysOf(s *Signer) map[string]ed25519.PublicKey {
	return map[string]ed25519.PublicKey{s.KID: s.Pub}
}

func TestSignVerifyRoundTrip(t *testing.T) {
	s := testSigner(t)
	in := &Receipt{Version: 1, ID: "req_1", Issuer: "https://gw.example", ReqSHA256: SHA256Hex([]byte("q")), RespSHA256: SHA256Hex([]byte("a")), Status: 200, CostUSD: 0.5}
	jws, leaf := s.Sign(in)
	out, err := Verify(jws, keysOf(s))
	if err != nil {
		t.Fatal(err)
	}
	if out.ID != in.ID || out.ReqSHA256 != in.ReqSHA256 || out.RespSHA256 != in.RespSHA256 || out.CostUSD != in.CostUSD {
		t.Fatalf("round trip: got %+v", out)
	}
	if hex.EncodeToString(leaf) != SHA256Hex([]byte(jws)) {
		t.Fatal("leaf is not SHA-256 of the JWS")
	}
}

// TestVerifyRejectsTampering flips every byte of the payload and signature
// segments in turn; none of the results may verify.
func TestVerifyRejectsTampering(t *testing.T) {
	s := testSigner(t)
	jws, _ := s.Sign(&Receipt{Version: 1, ID: "req_1", Status: 200})
	keys := keysOf(s)
	first := strings.IndexByte(jws, '.') + 1
	for i := first; i < len(jws); i++ {
		if jws[i] == '.' {
			continue
		}
		b := []byte(jws)
		if b[i] == 'A' {
			b[i] = 'B'
		} else {
			b[i] = 'A'
		}
		if _, err := Verify(string(b), keys); err == nil {
			t.Fatalf("byte %d changed (%q → %q) and the receipt still verified", i, jws[i], b[i])
		}
	}
}

func TestVerifyRejectsKeyAndAlg(t *testing.T) {
	s := testSigner(t)
	jws, _ := s.Sign(&Receipt{Version: 1, ID: "req_1"})
	other, _ := NewSigner(nil, bytes.Repeat([]byte{0x07}, 32))
	if _, err := Verify(jws, keysOf(other)); err == nil || !strings.Contains(err.Error(), "unknown signing key") {
		t.Fatalf("unknown kid: %v", err)
	}
	// Same kid, wrong key material.
	if _, err := Verify(jws, map[string]ed25519.PublicKey{s.KID: other.Pub}); err == nil {
		t.Fatal("verified with the wrong public key")
	}
	p := strings.Split(jws, ".")
	for _, alg := range []string{"none", "HS256", "ES256", "eddsa"} {
		hdr := b64.EncodeToString([]byte(`{"alg":"` + alg + `","kid":"` + s.KID + `","typ":"tpx-receipt+jwt"}`))
		if _, err := Verify(hdr+"."+p[1]+"."+p[2], keysOf(s)); err == nil || !strings.Contains(err.Error(), "unexpected alg") {
			t.Errorf("alg %q: %v", alg, err)
		}
	}
	for _, bad := range []string{"", "a.b", "a.b.c.d", p[0] + "." + p[1]} {
		if _, err := Verify(bad, keysOf(s)); err == nil {
			t.Errorf("%q verified", bad)
		}
	}
}

func TestParseJWKS(t *testing.T) {
	s := testSigner(t)
	good := s.JWK()
	short := b64.EncodeToString(s.Pub[:31])
	set := map[string]any{"keys": []any{
		good,
		map[string]string{"kty": "EC", "crv": "P-256", "x": good["x"], "kid": "ec"},
		map[string]string{"kty": "OKP", "crv": "X25519", "x": good["x"], "kid": "x25519"},
		map[string]string{"kty": "OKP", "crv": "Ed25519", "x": short, "kid": "short"},
		map[string]string{"kty": "OKP", "crv": "Ed25519", "x": "!!not base64!!", "kid": "garbage"},
	}}
	b, _ := json.Marshal(set)
	keys, err := ParseJWKS(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || !bytes.Equal(keys[s.KID], s.Pub) {
		t.Fatalf("want only %s, got %v", s.KID, keys)
	}
	if _, err := ParseJWKS([]byte("not json")); err == nil {
		t.Fatal("invalid JSON accepted")
	}
}

// TestMerkleRoot pins the root for 0–4 leaves so the format cannot drift
// silently. The expected values were computed independently from the
// definition: leaf = SHA-256(0x00 || leaf), node = SHA-256(0x01 || l || r),
// an odd last node is carried up unchanged, and no leaves is SHA-256("").
func TestMerkleRoot(t *testing.T) {
	want := []string{
		"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		"7f9c9e31ac8256ca2f258583df262dbc7d6f68f2a03043d5c99a4ae5a7396ce9",
		"28fb81e496897e0ce886f08602392e9239b65c659041e5202163e58ad898f444",
		"ba8d94b7fbcecae7b81c4c80574fe24734a6917bf9c1ecd66ff3e0c34ead4620",
		"fdea52008cdae79fa8bf806261959e23f5e11681646a2fa2bc9b5e56b32030a2",
	}
	var leaves [][]byte
	for n := 0; n <= 4; n++ {
		if got := hex.EncodeToString(MerkleRoot(leaves)); got != want[n] {
			t.Errorf("%d leaves: %s, want %s", n, got, want[n])
		}
		leaves = append(leaves, bytes.Repeat([]byte{byte(n)}, 32))
	}
	// Order matters.
	a := MerkleRoot([][]byte{leaves[0], leaves[1]})
	b := MerkleRoot([][]byte{leaves[1], leaves[0]})
	if bytes.Equal(a, b) {
		t.Fatal("swapping leaves did not change the root")
	}
}

func TestNewSigner(t *testing.T) {
	master := []byte("a master key used only in this test")
	a, err := NewSigner(master, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := NewSigner(master, nil)
	if a.KID != b.KID || !bytes.Equal(a.Pub, b.Pub) {
		t.Fatal("same master key produced different signers")
	}
	c, _ := NewSigner([]byte("another master key"), nil)
	if c.KID == a.KID || bytes.Equal(c.Pub, a.Pub) {
		t.Fatal("different master keys produced the same signer")
	}
	if !strings.HasPrefix(a.KID, "tpx-") || len(a.KID) != len("tpx-")+12 {
		t.Fatalf("kid %q", a.KID)
	}
	// Signatures from equal signers verify against each other's keys.
	jws, _ := a.Sign(&Receipt{Version: 1, ID: "req_1"})
	if _, err := Verify(jws, keysOf(b)); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSigner(nil, []byte("short")); err == nil {
		t.Fatal("short seed accepted")
	}
}
