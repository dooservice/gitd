//go:build !windows

package server

import "os"

// restrictSocket lets only gitd's user and group connect.
func restrictSocket(socket string) error {
	return os.Chmod(socket, 0o660)
}
