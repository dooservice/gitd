// Package apikeys checks the keys callers bring: a tenant's, or the one gitd's network
// address asks for without tenants. A key is read from its file — best a systemd credential
// — and kept hashed; replacing the file replaces the key, with no restart.
package apikeys

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// Header is where a caller brings its key: `Authorization: Bearer <key>`.
const Header = "Authorization"

// File is one key, read from a file and read again when the file changes.
type File struct {
	path     string
	mu       sync.Mutex
	hashed   [sha256.Size]byte
	loaded   bool
	modified time.Time
	size     int64
}

// NewFile returns the key in the file at path; it is read on first use.
func NewFile(path string) *File {
	return &File{path: path}
}

// Matches says whether key is this one, in constant time. A file that cannot be read, or
// holds nothing, matches no key.
func (file *File) Matches(key string) bool {
	if key == "" {
		return false
	}
	hashed, ok := file.current()
	if !ok {
		return false
	}
	asked := sha256.Sum256([]byte(key))
	return subtle.ConstantTimeCompare(hashed[:], asked[:]) == 1
}

func (file *File) current() ([sha256.Size]byte, bool) {
	file.mu.Lock()
	defer file.mu.Unlock()
	info, err := os.Stat(file.path)
	if err != nil {
		return [sha256.Size]byte{}, false
	}
	if file.loaded && info.ModTime().Equal(file.modified) && info.Size() == file.size {
		return file.hashed, true
	}
	data, err := os.ReadFile(file.path)
	key := strings.TrimSpace(string(data))
	if err != nil || key == "" {
		return [sha256.Size]byte{}, false
	}
	file.hashed, file.loaded, file.modified, file.size = sha256.Sum256([]byte(key)), true, info.ModTime(), info.Size()
	return file.hashed, true
}

// Of is the key a request brings, if any.
func Of(request *http.Request) string {
	key, ok := strings.CutPrefix(request.Header.Get(Header), "Bearer ")
	if !ok {
		return ""
	}
	return strings.TrimSpace(key)
}
