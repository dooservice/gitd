package githubapp

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"strings"
	"testing"
	"time"
)

// testKey is a private key in PEM, as GitHub gives one (PKCS #1), and its public half.
func testKey(t *testing.T) ([]byte, *rsa.PublicKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	data := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return data, &key.PublicKey
}

func TestAJWTIsSignedWithTheAppsKeyAndSaysWhichAppAsks(t *testing.T) {
	data, public := testKey(t)
	signer, err := NewSigner(123456, func() ([]byte, error) { return data, nil })
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0)
	token, err := signer.JWT(now)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token = %q", token)
	}
	signature, _ := base64.RawURLEncoding.DecodeString(parts[2])
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(public, crypto.SHA256, digest[:], signature); err != nil {
		t.Fatalf("the signature does not verify: %v", err)
	}
	payload, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var claims struct {
		IssuedAt  int64  `json:"iat"`
		ExpiresAt int64  `json:"exp"`
		Issuer    string `json:"iss"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatal(err)
	}
	if claims.Issuer != "123456" || claims.IssuedAt != now.Unix()-60 || claims.ExpiresAt-now.Unix() > 600 {
		t.Errorf("claims = %+v", claims)
	}
}

func TestAKeyInPKCS8IsReadAndAnotherKeyIsRefused(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pkcs8, _ := x509.MarshalPKCS8PrivateKey(key)
	data := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})
	if _, err := NewSigner(1, func() ([]byte, error) { return data, nil }); err != nil {
		t.Errorf("PKCS #8: %v", err)
	}
	if _, err := NewSigner(1, func() ([]byte, error) { return []byte("not a key"), nil }); err == nil {
		t.Error("a key that is not one was accepted")
	}
}

func TestAKeyReplacedWhereItIsKeptIsUsedFromThenOn(t *testing.T) {
	first, _ := testKey(t)
	second, public := testKey(t)
	current := first
	signer, err := NewSigner(1, func() ([]byte, error) { return current, nil })
	if err != nil {
		t.Fatal(err)
	}
	current = second
	token, err := signer.JWT(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(token, ".")
	signature, _ := base64.RawURLEncoding.DecodeString(parts[2])
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(public, crypto.SHA256, digest[:], signature) != nil {
		t.Error("the JWT was not signed with the key that replaced the first")
	}
}
