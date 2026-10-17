package publishing

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"google.golang.org/protobuf/encoding/protojson"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/files"
)

// Errors a change to the settings is refused with.
var (
	ErrFromFile = errors.New("these settings are the configuration file's: change them there")
	ErrNotFound = errors.New("no settings have that prefix")
)

// settingsFile is where the settings set through the API are kept.
const settingsFile = "settings.json"

// Store is every setting: the configuration file's, then those set through the API.
type Store struct {
	path     string
	fromFile []*gitdv1.PublishSettings
	guard    sync.RWMutex
	set      []*gitdv1.PublishSettings
}

// Open reads the settings kept in directory, making it if it is not there, beside those of
// the configuration file (FromFile).
func Open(directory string, fromFile []*gitdv1.PublishSettings) (*Store, error) {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("publishing settings: %w", err)
	}
	store := &Store{path: filepath.Join(directory, settingsFile), fromFile: fromFile}
	data, err := os.ReadFile(store.path)
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return nil, fmt.Errorf("publishing settings: %w", err)
	}
	var kept gitdv1.GetSettingsResponse
	if err := protojson.Unmarshal(data, &kept); err != nil {
		return nil, fmt.Errorf("publishing settings: %s: %w", store.path, err)
	}
	store.set = kept.GetSettings()
	return store, nil
}

// All is every setting, the file's first, as copies.
func (store *Store) All() []*gitdv1.PublishSettings {
	store.guard.RLock()
	defer store.guard.RUnlock()
	all := make([]*gitdv1.PublishSettings, 0, len(store.fromFile)+len(store.set))
	for _, settings := range append(slices.Clone(store.fromFile), store.set...) {
		all = append(all, cloned(settings))
	}
	return all
}

// Set keeps the settings of a prefix, replacing those it had, once checked alone and
// against every other, and returns them as kept.
func (store *Store) Set(settings *gitdv1.PublishSettings) (*gitdv1.PublishSettings, error) {
	kept, err := normalized(settings)
	if err != nil {
		return nil, err
	}
	kept.FromFile = false
	store.guard.Lock()
	defer store.guard.Unlock()
	if store.fileHas(kept.GetUrlPrefix()) {
		return nil, ErrFromFile
	}
	changed := slices.DeleteFunc(slices.Clone(store.set), func(earlier *gitdv1.PublishSettings) bool {
		return earlier.GetUrlPrefix() == kept.GetUrlPrefix()
	})
	changed = append(changed, kept)
	if err := consistent(append(slices.Clone(store.fromFile), changed...)); err != nil {
		return nil, err
	}
	if err := store.write(changed); err != nil {
		return nil, err
	}
	store.set = changed
	return cloned(kept), nil
}

// Remove forgets the settings of a prefix set through the API.
func (store *Store) Remove(prefix string) error {
	normal := prefix
	if prefix != "" {
		checked, err := normalized(&gitdv1.PublishSettings{UrlPrefix: prefix})
		if err != nil {
			return err
		}
		normal = checked.GetUrlPrefix()
	}
	store.guard.Lock()
	defer store.guard.Unlock()
	if store.fileHas(normal) {
		return ErrFromFile
	}
	changed := slices.DeleteFunc(slices.Clone(store.set), func(earlier *gitdv1.PublishSettings) bool {
		return earlier.GetUrlPrefix() == normal
	})
	if len(changed) == len(store.set) {
		return ErrNotFound
	}
	if err := store.write(changed); err != nil {
		return err
	}
	store.set = changed
	return nil
}

func (store *Store) fileHas(prefix string) bool {
	return slices.ContainsFunc(store.fromFile, func(settings *gitdv1.PublishSettings) bool {
		return settings.GetUrlPrefix() == prefix
	})
}

func (store *Store) write(settings []*gitdv1.PublishSettings) error {
	data, err := protojson.MarshalOptions{Multiline: true}.Marshal(&gitdv1.GetSettingsResponse{Settings: settings})
	if err != nil {
		return err
	}
	if err := files.WritePrivate(store.path, data); err != nil {
		return fmt.Errorf("publishing settings: %w", err)
	}
	return nil
}
