package apikeys

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAKeyMatchesWhatItsFileHoldsNowAndNothingElse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "acme")
	file := NewFile(path)
	if file.Matches("anything") {
		t.Error("a key matched with no file")
	}
	if err := os.WriteFile(path, []byte("first-key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !file.Matches("first-key") || file.Matches("first") || file.Matches("") {
		t.Error("the key in the file does not match, or another does")
	}
	if err := os.WriteFile(path, []byte("second-key"), 0o600); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Second)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
	if file.Matches("first-key") || !file.Matches("second-key") {
		t.Error("a replaced key was not read again")
	}
	request := httptest.NewRequest("POST", "/", nil)
	request.Header.Set("Authorization", "Bearer second-key")
	if Of(request) != "second-key" {
		t.Errorf("Of = %q", Of(request))
	}
}
