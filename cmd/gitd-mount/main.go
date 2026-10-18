//go:build linux

// Command gitd-mount mounts and unmounts gitd's overlay workspaces, so that gitd itself
// needs no privilege. It runs as root, answers only the user its configuration names, on a
// socket only that user can reach, and only within the directories that configuration
// fixes (internal/mounthelper).
package main

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/dooservice/gitd/internal/mounthelper"
)

var version = "dev"

func main() {
	configPath := flag.String("config", "/etc/gitd/gitd-mount.conf", "configuration file, root's")
	socket := flag.String("socket", "/run/gitd-mount/gitd-mount.sock", "socket to answer on, unless systemd hands one")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))
	if err := run(*configPath, *socket); err != nil {
		slog.Error("gitd-mount stopped", "error", err)
		os.Exit(1)
	}
}

func run(configPath, socket string) error {
	config, err := mounthelper.Load(configPath)
	if err != nil {
		return err
	}
	listener, err := listen(socket, config.UID)
	if err != nil {
		return err
	}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	go func() { <-stop; _ = listener.Close() }()
	slog.Info("gitd-mount answering", "uid", config.UID, "templates", config.Templates, "layers", config.Layers, "roots", config.Roots)
	return mounthelper.Serve(listener, config)
}

// listen takes the socket systemd hands over (socket activation), or makes one at path
// that only uid can reach.
func listen(path string, uid int) (*net.UnixListener, error) {
	if os.Getenv("LISTEN_PID") == strconv.Itoa(os.Getpid()) && os.Getenv("LISTEN_FDS") == "1" {
		listener, err := net.FileListener(os.NewFile(3, "systemd"))
		if err != nil {
			return nil, err
		}
		unixListener, ok := listener.(*net.UnixListener)
		if !ok {
			return nil, errors.New("systemd handed a socket that is not a Unix one")
		}
		return unixListener, nil
	}
	if err := os.MkdirAll(dirOf(path), 0o755); err != nil {
		return nil, err
	}
	_ = os.Remove(path)
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, err
	}
	// Mode first, while the socket is still root's: once it is uid's, a root without
	// CAP_FOWNER may no longer change it.
	if err := os.Chmod(path, 0o600); err != nil {
		return nil, err
	}
	return listener, os.Chown(path, uid, -1)
}

func dirOf(path string) string {
	for index := len(path) - 1; index > 0; index-- {
		if path[index] == '/' {
			return path[:index]
		}
	}
	return "/"
}
