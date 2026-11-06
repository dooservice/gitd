//go:build windows

package config

// userPlaces is none: on Windows gitd runs as the service, for every member of its group.
func userPlaces(environment) (User, bool) { return User{}, false }
