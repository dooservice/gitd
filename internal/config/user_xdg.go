//go:build !windows && !darwin

package config

// userPlaces is the XDG places.
func userPlaces(env environment) (User, bool) { return env.xdg(), true }
