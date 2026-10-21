package security

import (
	"testing"
	"time"
)

func TestRequestSignatureRoundTrip(t *testing.T) {
	secret := NewToken(32)
	body := []byte(`{"platform":"linux"}`)
	sig := SignRequestHMAC(secret, "POST", "/api/v1/check", "1770000000", "nonce-1", body)

	if !VerifyRequestHMAC(secret, "POST", "/api/v1/check", "1770000000", "nonce-1", body, sig) {
		t.Fatal("valid signature was rejected")
	}
	if VerifyRequestHMAC(secret, "POST", "/api/v1/check", "1770000000", "nonce-2", body, sig) {
		t.Fatal("signature must cover the nonce")
	}
	if VerifyRequestHMAC(secret, "POST", "/api/v1/check", "1770000001", "nonce-1", body, sig) {
		t.Fatal("signature must cover the timestamp")
	}
	if VerifyRequestHMAC(secret, "GET", "/api/v1/check", "1770000000", "nonce-1", body, sig) {
		t.Fatal("signature must cover the method")
	}
	if VerifyRequestHMAC(secret, "POST", "/api/v1/other", "1770000000", "nonce-1", body, sig) {
		t.Fatal("signature must cover the path")
	}
	if VerifyRequestHMAC(secret, "POST", "/api/v1/check", "1770000000", "nonce-1", []byte(`{"platform":"windows"}`), sig) {
		t.Fatal("signature must cover the body")
	}
	if VerifyRequestHMAC("other-secret", "POST", "/api/v1/check", "1770000000", "nonce-1", body, sig) {
		t.Fatal("signature must depend on the secret")
	}
}

func TestRequestSignatureUsesRawSecretBytes(t *testing.T) {
	// Clients in any language must be able to hash the secret verbatim, without
	// guessing whether the value happens to look like base64.
	body := []byte("{}")
	sig := SignRequestHMAC("abcd", "POST", "/p", "1", "n", body)
	if !VerifyRequestHMAC("abcd", "POST", "/p", "1", "n", body, sig) {
		t.Fatal("signature verification failed for a base64-looking secret")
	}
}

func TestManifestSignatureRoundTrip(t *testing.T) {
	pub, priv, err := GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("generate keys: %v", err)
	}
	payload := ManifestPayload{
		AppID:            "demo",
		Version:          "1.2.3",
		VersionCode:      1002003,
		Platform:         "linux",
		Arch:             "amd64",
		Channel:          "stable",
		PackageURL:       "https://example.com/app.zip",
		PackageSize:      1024,
		PackageSHA256:    "abc",
		Force:            false,
		MinSupportedCode: 1000000,
	}

	sig, err := SignManifest(priv, payload)
	if err != nil {
		t.Fatalf("sign manifest: %v", err)
	}
	ok, err := VerifyManifest(pub, payload, sig)
	if err != nil {
		t.Fatalf("verify manifest: %v", err)
	}
	if !ok {
		t.Fatal("manifest signature did not verify")
	}

	tampered := payload
	tampered.Version = "9.9.9"
	ok, err = VerifyManifest(pub, tampered, sig)
	if err != nil {
		t.Fatalf("verify tampered manifest: %v", err)
	}
	if ok {
		t.Fatal("tampered manifest must not verify")
	}
}

func TestDownloadTokenRoundTrip(t *testing.T) {
	secret := NewToken(32)
	token := IssueDownloadToken(secret, 42, "client", time.Minute)

	if err := VerifyDownloadToken(secret, 42, "client", token); err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}
	if err := VerifyDownloadToken(secret, 43, "client", token); err == nil {
		t.Fatal("token must be bound to the package id")
	}
	if err := VerifyDownloadToken("other", 42, "client", token); err == nil {
		t.Fatal("token must be bound to the secret")
	}
	if err := VerifyDownloadToken(secret, 42, "client", token+"x"); err == nil {
		t.Fatal("tampered token accepted")
	}
}

func TestDownloadTokenExpires(t *testing.T) {
	secret := NewToken(32)
	token := IssueDownloadToken(secret, 1, "", -time.Second)
	if err := VerifyDownloadToken(secret, 1, "", token); err == nil {
		t.Fatal("expired token accepted")
	}
}

func TestHashTokenIsStable(t *testing.T) {
	a := HashToken("abc")
	b := HashToken("abc")
	if a != b {
		t.Fatal("HashToken must be deterministic")
	}
	if a == HashToken("abd") {
		t.Fatal("HashToken must differ for different input")
	}
	if len(a) != 64 {
		t.Fatalf("expected a hex sha256, got %q", a)
	}
}

func TestStablePercentIsDeterministic(t *testing.T) {
	first := StablePercent("install-1")
	for i := 0; i < 10; i++ {
		if StablePercent("install-1") != first {
			t.Fatal("rollout bucket must be stable for one client")
		}
	}
	if first < 0 || first > 99 {
		t.Fatalf("bucket out of range: %d", first)
	}
}
