package connections

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/dooservice/gitd/internal/files"
	"github.com/dooservice/gitd/internal/secrets"
)

// ErrNotFound is a connection no ID names.
var ErrNotFound = errors.New("no connection has that ID")

// recordSuffix ends every file that holds a connection.
const recordSuffix = ".json"

// Store is the connections kept in a directory, and their secrets opened, in memory.
type Store struct {
	directory string
	box       *secrets.Box
	now       func() time.Time
	guard     sync.RWMutex
	records   map[string]Record
	opened    map[string][]byte
}

// Open reads the connections kept in directory, making it if it is not there. A secret
// that cannot be opened — sealed with another key, or with none given — stops the start,
// rather than leaving its repositories to fail one by one.
func Open(directory string, box *secrets.Box) (*Store, error) {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("connections: %w", err)
	}
	store := &Store{directory: directory, box: box, now: time.Now,
		records: map[string]Record{}, opened: map[string][]byte{}}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("connections: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), recordSuffix) {
			continue
		}
		if err := store.load(filepath.Join(directory, entry.Name())); err != nil {
			return nil, err
		}
	}
	return store, nil
}

func (store *Store) load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("connections: %w", err)
	}
	var record Record
	if err := json.Unmarshal(data, &record); err != nil {
		return fmt.Errorf("connections: %s: %w", path, err)
	}
	secret, err := store.box.Unseal(record.Secret)
	if err != nil {
		return fmt.Errorf("connections: %s: %w", record.ID, err)
	}
	store.records[record.ID] = record
	store.opened[record.ID] = secret
	return nil
}

// Add keeps a new connection with its secret, giving it an ID and the time now, and
// returns it as kept.
func (store *Store) Add(record Record, secret []byte) (Record, error) {
	id, err := newID(record.Kind)
	if err != nil {
		return Record{}, err
	}
	record.ID, record.Created = id, store.now().UTC()
	return record, store.keep(record, secret)
}

// Replace keeps a connection again with a new secret or new details, under its ID: a
// sign-in's renewal token is replaced each time it is used.
func (store *Store) Replace(record Record, secret []byte) error {
	store.guard.RLock()
	_, known := store.records[record.ID]
	store.guard.RUnlock()
	if !known {
		return ErrNotFound
	}
	return store.keep(record, secret)
}

func (store *Store) keep(record Record, secret []byte) error {
	sealed, err := store.box.Seal(secret)
	if err != nil {
		return err
	}
	record.Secret = sealed
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	if err := files.WritePrivate(store.path(record.ID), data); err != nil {
		return fmt.Errorf("connections: %w", err)
	}
	store.guard.Lock()
	defer store.guard.Unlock()
	store.records[record.ID] = record
	store.opened[record.ID] = slices.Clone(secret)
	return nil
}

// Remove forgets a connection and deletes its file.
func (store *Store) Remove(id string) error {
	store.guard.Lock()
	defer store.guard.Unlock()
	if _, known := store.records[id]; !known {
		return ErrNotFound
	}
	if err := os.Remove(store.path(id)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("connections: %w", err)
	}
	delete(store.records, id)
	delete(store.opened, id)
	return nil
}

// List is every connection kept, oldest first, their secrets as kept, never opened.
func (store *Store) List() []Record {
	store.guard.RLock()
	defer store.guard.RUnlock()
	listed := make([]Record, 0, len(store.records))
	for _, record := range store.records {
		listed = append(listed, record)
	}
	slices.SortFunc(listed, func(first, second Record) int {
		return first.Created.Compare(second.Created)
	})
	return listed
}

// Get is one connection and its secret, opened.
func (store *Store) Get(id string) (Record, []byte, error) {
	store.guard.RLock()
	defer store.guard.RUnlock()
	record, known := store.records[id]
	if !known {
		return Record{}, nil, ErrNotFound
	}
	return record, slices.Clone(store.opened[id]), nil
}

// path is the file of the connection with an ID. IDs are made by gitd, never given, so
// none reaches outside the directory.
func (store *Store) path(id string) string {
	return filepath.Join(store.directory, id+recordSuffix)
}

// newID is an ID for a connection of a kind: its kind's prefix and twelve random hex digits.
func newID(kind Kind) (string, error) {
	prefix, known := idPrefix[kind]
	if !known {
		return "", fmt.Errorf("connections: no kind %q", kind)
	}
	random := make([]byte, 6)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(random), nil
}
