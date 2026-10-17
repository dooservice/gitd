package secrets

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeKey(t *testing.T, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func randomKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, keySize)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	return key
}

func TestASecretSealedWithAKeyOpensOnlyWithIt(t *testing.T) {
	box, err := Open(writeKey(t, randomKey(t)))
	if err != nil {
		t.Fatal(err)
	}
	kept, err := box.Seal([]byte("ghs_token"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(kept, "ghs_token") || !strings.HasPrefix(kept, sealedMark) {
		t.Fatalf("kept %q shows the secret or is not sealed", kept)
	}
	opened, err := box.Unseal(kept)
	if err != nil || string(opened) != "ghs_token" {
		t.Fatalf("Unseal = %q, %v", opened, err)
	}
	other, err := Open(writeKey(t, randomKey(t)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Unseal(kept); err == nil {
		t.Error("another key opened the secret")
	}
	if _, err := (&Box{}).Unseal(kept); err == nil {
		t.Error("no key opened the secret")
	}
}

func TestWithoutAKeyASecretIsKeptAsItIs(t *testing.T) {
	box, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	kept, err := box.Seal([]byte("token"))
	if err != nil || kept != "token" {
		t.Fatalf("Seal = %q, %v", kept, err)
	}
	keyed, err := Open(writeKey(t, randomKey(t)))
	if err != nil {
		t.Fatal(err)
	}
	if opened, err := keyed.Unseal(kept); err != nil || string(opened) != "token" {
		t.Errorf("a secret kept before the key was given: %q, %v", opened, err)
	}
}

func TestAKeyIsReadRawInHexOrInBase64(t *testing.T) {
	key := randomKey(t)
	for name, content := range map[string][]byte{
		"raw":    key,
		"hex":    []byte(hex.EncodeToString(key) + "\n"),
		"base64": []byte(base64.StdEncoding.EncodeToString(key) + "\n"),
	} {
		box, err := Open(writeKey(t, content))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		kept, err := box.Seal([]byte("secret"))
		if err != nil {
			t.Fatal(err)
		}
		reference, _ := Open(writeKey(t, key))
		if opened, err := reference.Unseal(kept); err != nil || !bytes.Equal(opened, []byte("secret")) {
			t.Errorf("%s: the key read is not the key written", name)
		}
	}
	if _, err := Open(writeKey(t, []byte("short"))); err == nil {
		t.Error("a key of the wrong size was accepted")
	}
}
