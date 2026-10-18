//go:build linux

package mounthelper

import (
	"bufio"
	"encoding/json"
	"errors"
	"log/slog"
	"net"

	"golang.org/x/sys/unix"
)

// Serve answers on listener until it closes: one request per connection, from a caller
// the kernel names by its credentials, never by anything it says.
func Serve(listener *net.UnixListener, config Config) error {
	for {
		connection, err := listener.AcceptUnix()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		go answer(connection, config)
	}
}

func answer(connection *net.UnixConn, config Config) {
	defer connection.Close()
	uid, err := peerUID(connection)
	if err != nil {
		return
	}
	var request Request
	reply := Answer{}
	if err := json.NewDecoder(bufio.NewReader(connection)).Decode(&request); err != nil {
		reply.Error = "unreadable request"
	} else if err := Do(config, uid, request); err != nil {
		reply.Error = err.Error()
		slog.Warn("refused", "op", request.Op, "target", request.Target, "uid", uid, "error", err)
	} else {
		slog.Info(request.Op, "target", request.Target, "uid", uid)
	}
	_ = json.NewEncoder(connection).Encode(reply)
}

func peerUID(connection *net.UnixConn) (int, error) {
	raw, err := connection.SyscallConn()
	if err != nil {
		return 0, err
	}
	var credentials *unix.Ucred
	var credentialsErr error
	if err := raw.Control(func(fd uintptr) {
		credentials, credentialsErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); err != nil {
		return 0, err
	}
	if credentialsErr != nil {
		return 0, credentialsErr
	}
	return int(credentials.Uid), nil
}
