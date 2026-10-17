package publishing

import (
	"errors"
	"fmt"
	"os"
	"slices"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
	"github.com/dooservice/gitd/internal/files"
)

// ErrInUse is a preset settings still take, which cannot be removed.
var ErrInUse = errors.New("settings take this preset: change them first")

// presetsFile is where the presets set through the API are kept.
const presetsFile = "presets.json"

// Presets is every preset as it applies: gitd's own unless replaced, then the file's, then
// those set through the API, each name once.
func (store *Store) Presets() []*gitdv1.Preset {
	store.guard.RLock()
	defer store.guard.RUnlock()
	var applying []*gitdv1.Preset
	if _, replaced := store.namedLocked(RecommendedName); replaced.GetOrigin() == gitdv1.Preset_ORIGIN_BUILT_IN {
		applying = append(applying, builtIn())
	}
	for _, preset := range append(slices.Clone(store.filePresets), store.setPresets...) {
		applying = append(applying, clonedPreset(preset))
	}
	return applying
}

// namedLocked is the preset of a name as it applies — set through the API, else the
// file's, else gitd's own — and whether there is one. The guard is held.
func (store *Store) namedLocked(name string) (bool, *gitdv1.Preset) {
	for _, kept := range [][]*gitdv1.Preset{store.setPresets, store.filePresets} {
		if index := slices.IndexFunc(kept, func(preset *gitdv1.Preset) bool { return preset.GetName() == name }); index >= 0 {
			return true, kept[index]
		}
	}
	if name == RecommendedName {
		return true, builtIn()
	}
	return false, nil
}

// SetPreset keeps a preset, replacing one of its name set through the API, and returns it
// as kept. A name the file uses is the file's.
func (store *Store) SetPreset(preset *gitdv1.Preset) (*gitdv1.Preset, error) {
	kept, err := normalizedPreset(preset, gitdv1.Preset_ORIGIN_API)
	if err != nil {
		return nil, err
	}
	store.guard.Lock()
	defer store.guard.Unlock()
	if slices.ContainsFunc(store.filePresets, func(other *gitdv1.Preset) bool { return other.GetName() == kept.GetName() }) {
		return nil, ErrFromFile
	}
	changed := slices.DeleteFunc(slices.Clone(store.setPresets), func(other *gitdv1.Preset) bool {
		return other.GetName() == kept.GetName()
	})
	changed = append(changed, kept)
	if err := store.writePresets(changed); err != nil {
		return nil, err
	}
	store.setPresets = changed
	return clonedPreset(kept), nil
}

// RemovePreset forgets a preset set through the API. One settings still take is refused,
// unless gitd has its own of that name, which then applies again.
func (store *Store) RemovePreset(name string) error {
	store.guard.Lock()
	defer store.guard.Unlock()
	if slices.ContainsFunc(store.filePresets, func(other *gitdv1.Preset) bool { return other.GetName() == name }) {
		return ErrFromFile
	}
	changed := slices.DeleteFunc(slices.Clone(store.setPresets), func(other *gitdv1.Preset) bool {
		return other.GetName() == name
	})
	if len(changed) == len(store.setPresets) {
		return fmt.Errorf("%w: no preset %q was set through the API", ErrNotFound, name)
	}
	taken := slices.ContainsFunc(append(slices.Clone(store.fromFile), store.set...), func(settings *gitdv1.PublishSettings) bool {
		return settings.GetPreset() == name
	})
	if taken && name != RecommendedName {
		return ErrInUse
	}
	if err := store.writePresets(changed); err != nil {
		return err
	}
	store.setPresets = changed
	return nil
}

// readPresets reads the presets kept through the API, if any were.
func (store *Store) readPresets() error {
	data, err := os.ReadFile(store.presetsPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("publishing presets: %w", err)
	}
	var kept gitdv1.ListPresetsResponse
	if err := protojson.Unmarshal(data, &kept); err != nil {
		return fmt.Errorf("publishing presets: %s: %w", store.presetsPath, err)
	}
	store.setPresets = kept.GetPresets()
	return nil
}

func (store *Store) writePresets(presets []*gitdv1.Preset) error {
	data, err := protojson.MarshalOptions{Multiline: true}.Marshal(&gitdv1.ListPresetsResponse{Presets: presets})
	if err != nil {
		return err
	}
	if err := files.WritePrivate(store.presetsPath, data); err != nil {
		return fmt.Errorf("publishing presets: %w", err)
	}
	return nil
}

// clonedPreset is a copy of a preset that whoever holds the original cannot change.
func clonedPreset(preset *gitdv1.Preset) *gitdv1.Preset {
	return proto.Clone(preset).(*gitdv1.Preset)
}
