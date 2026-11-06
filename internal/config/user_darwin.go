//go:build darwin

package config

// userPlaces is the places in ~/Library.
func userPlaces(env environment) (User, bool) { return env.library(), true }
