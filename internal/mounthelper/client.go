package mounthelper

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

// Client is gitd's side: a strategy.Mounter that asks gitd-mount on its socket.
type Client struct {
	Socket string
}

// MountOverlay asks for target to be mounted as the overlay of upper on lowers, the topmost
// first, which travel as one field joined by colons, as overlayfs names them.
func (client Client) MountOverlay(lowers []string, upper, work, target string) error {
	return client.ask(Request{Op: "mount", Lower: strings.Join(lowers, ":"), Upper: upper, Work: work, Target: target})
}

// Unmount asks for the overlay at target to be unmounted.
func (client Client) Unmount(target string) error {
	return client.ask(Request{Op: "unmount", Target: target})
}

func (client Client) ask(request Request) error {
	connection, err := net.DialTimeout("unix", client.Socket, 5*time.Second)
	if err != nil {
		return fmt.Errorf("gitd-mount: %w", err)
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(30 * time.Second))
	if err := json.NewEncoder(connection).Encode(request); err != nil {
		return fmt.Errorf("gitd-mount: %w", err)
	}
	var answer Answer
	if err := json.NewDecoder(connection).Decode(&answer); err != nil {
		return fmt.Errorf("gitd-mount: %w", err)
	}
	if answer.Error != "" {
		return errors.New("gitd-mount: " + answer.Error)
	}
	return nil
}
