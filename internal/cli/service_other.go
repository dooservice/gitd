//go:build !windows

package cli

import (
	"context"

	"github.com/spf13/cobra"
)

// platformCommands are the commands only one system has: none here, where systemd runs gitd.
func platformCommands(*app) []*cobra.Command { return nil }

// asService serves as this process was started; only Windows starts gitd as a service.
func asService(ctx context.Context, _ string, serve func(context.Context) error) error {
	return serve(ctx)
}
