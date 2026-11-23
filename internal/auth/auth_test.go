package auth

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestHashAndCheckPassword(t *testing.T) {
	enc, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(enc, "correct horse battery staple") {
		t.Fatal("correct password rejected")
	}
	again, _ := HashPassword("correct horse battery staple")
	if again == enc {
		t.Fatal("two hashes of the same password are equal: salt is not random")
	}
	parts := strings.Split(enc, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		t.Fatalf("encoding %q", enc)
	}
	otherSalt := base64.RawStdEncoding.EncodeToString([]byte("0123456789abcdef"))
	for name, bad := range map[string]string{
		"wrong password":   "",
		"tampered salt":    strings.Join([]string{parts[0], parts[1], otherSalt, parts[3]}, "$"),
		"tampered hash":    strings.Join([]string{parts[0], parts[1], parts[2], otherSalt}, "$"),
		"fewer iterations": strings.Join([]string{parts[0], "1000", parts[2], parts[3]}, "$"),
		"bad iterations":   strings.Join([]string{parts[0], "x", parts[2], parts[3]}, "$"),
		"bad base64":       strings.Join([]string{parts[0], parts[1], "!!", parts[3]}, "$"),
		"unknown scheme":   strings.Join([]string{"bcrypt", parts[1], parts[2], parts[3]}, "$"),
		"too few parts":    strings.Join(parts[:3], "$"),
		"empty":            "-",
	} {
		encoded, pw := bad, "correct horse battery staple"
		if name == "wrong password" {
			encoded, pw = enc, "Correct horse battery staple"
		}
		if encoded == "-" {
			encoded = ""
		}
		if CheckPassword(encoded, pw) {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestSignVerifyToken(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	in := Session{UserID: "usr_1", Exp: 4102444800, IdP: "okta"}
	tok := Sign(key, in)
	var out Session
	if err := Verify(key, tok, &out); err != nil || out != in {
		t.Fatalf("round trip: %v %+v", err, out)
	}
	ps, sig, _ := strings.Cut(tok, ".")
	forged := base64.RawURLEncoding.EncodeToString([]byte(`{"uid":"usr_admin","exp":4102444800}`))
	flip := func(s string) string {
		b := []byte(s)
		if b[0] == 'A' {
			b[0] = 'B'
		} else {
			b[0] = 'A'
		}
		return string(b)
	}
	for name, bad := range map[string]string{
		"tampered payload":   forged + "." + sig,
		"tampered signature": ps + "." + flip(sig),
		"no separator":       ps + sig,
		"empty":              "",
		"bad signature b64":  ps + ".!!",
	} {
		var s Session
		if err := Verify(key, bad, &s); err == nil {
			t.Errorf("%s: verified as %+v", name, s)
		}
	}
	var s Session
	if err := Verify([]byte("another key, same length, 32 b.."), tok, &s); err == nil {
		t.Error("verified with the wrong key")
	}
}

func TestGenerateKey(t *testing.T) {
	for _, prefix := range []string{GatewayKeyPrefix, PATPrefix, SCIMPrefix} {
		plain, display, hash := GenerateKey(prefix)
		if !strings.HasPrefix(plain, prefix) || len(plain) != len(prefix)+40 {
			t.Errorf("%s: plain %q", prefix, plain)
		}
		if !strings.HasPrefix(display, plain[:len(prefix)+6]) || !strings.HasSuffix(display, "…"+plain[len(plain)-4:]) {
			t.Errorf("%s: display %q for %q", prefix, display, plain)
		}
		if strings.Contains(display, plain[len(prefix)+6:len(plain)-4]) {
			t.Errorf("%s: display %q reveals the secret", prefix, display)
		}
		if string(hash) != string(HashKey(plain)) || len(hash) != 32 {
			t.Errorf("%s: hash does not match HashKey", prefix)
		}
		if other, _, _ := GenerateKey(prefix); other == plain {
			t.Errorf("%s: two keys are equal", prefix)
		}
	}
}
