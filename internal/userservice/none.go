//go:build !linux && !darwin

package userservice

// ForSystem is none: only Linux and macOS have a manager of user services gitd uses.
func ForSystem() (Manager, error) { return nil, ErrUnavailable }
