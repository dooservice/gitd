package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

// copyOnWrite says how gitd could make workspaces without writing every file, when it
// writes them because it may not mount an overlay: on Linux, unprivileged, without
// gitd-mount, and neither inside a container nor where cloning was tried instead.
type copyOnWrite struct {
	answer *gitdv1.InfoResponse
}

// unprivileged says whether workspaces are copied for want of a mount.
func (advice copyOnWrite) unprivileged() bool {
	reason := advice.answer.GetStrategyReason()
	return advice.answer.GetStrategy() == gitdv1.Strategy_STRATEGY_COPY &&
		strings.HasPrefix(reason, "overlay cannot run here: ") &&
		!strings.Contains(reason, "is itself on an overlay") &&
		!strings.Contains(reason, "templates cannot be cloned")
}

// hint is the line `gitd info` adds under the reason, saying where to read how; empty when
// there is nothing to add.
func (advice copyOnWrite) hint() string {
	if !advice.unprivileged() {
		return ""
	}
	return "  for copy-on-write, install gitd-mount (https://github.com/dooservice/gitd#install), " +
		"or on btrfs or XFS set strategy = \"reflink\" in [workspaces]\n"
}

// steps is what `gitd up` tells the user whose gitd it started on this machine: with the
// state on a filesystem that clones files, the one line of configuration that clones
// templates with no privilege at all; else the commands that install gitd-mount for them.
func (advice copyOnWrite) steps(machine localMachine) string {
	if machine.system != "linux" || !advice.unprivileged() {
		return ""
	}
	if machine.reflink {
		return fmt.Sprintf("Workspaces are copied, and %s's filesystem clones files: to clone templates instead,\n"+
			"with no root, add this to %s, then %s:\n\n"+
			"  [workspaces]\n  strategy = \"reflink\"\n\n"+
			"gitd-mount, installed as root, makes them faster still: https://github.com/dooservice/gitd#install\n",
			machine.state, machine.config, machine.restart)
	}
	release := machine.release()
	binary := "https://github.com/dooservice/gitd/releases/latest/download/gitd-mount-linux-" + machine.arch
	packaging := "https://raw.githubusercontent.com/dooservice/gitd/main/packaging/"
	if release != "" {
		binary = "https://github.com/dooservice/gitd/releases/download/" + release + "/gitd-mount-linux-" + machine.arch
		packaging = "https://raw.githubusercontent.com/dooservice/gitd/" + release + "/packaging/"
	}
	conf := []string{
		"user " + machine.user,
		"templates " + filepath.Join(machine.state, "templates"),
		"layers " + filepath.Join(machine.state, "layers"),
		"root " + machine.root,
	}
	quoted := make([]string, len(conf))
	for index, line := range conf {
		quoted[index] = shellWord(line)
	}
	return "Workspaces are copied: gitd may not mount an overlay without root. gitd-mount, a small root\n" +
		"service, mounts them for it (https://github.com/dooservice/gitd#install). To install it for you:\n\n" +
		"  sudo curl -fsSLo /usr/local/bin/gitd-mount " + binary + "\n" +
		"  sudo chmod 755 /usr/local/bin/gitd-mount\n" +
		"  sudo curl -fsSLo /etc/systemd/system/gitd-mount.service " + packaging + "gitd-mount.service\n" +
		"  curl -fsSL " + packaging + "gitd-mount.socket | sed 's/^SocketUser=gitd$/SocketUser=" + machine.user + "/' |" +
		" sudo tee /etc/systemd/system/gitd-mount.socket >/dev/null\n" +
		"  sudo mkdir -p /etc/gitd\n" +
		"  printf '%s\\n' " + strings.Join(quoted, " ") + " | sudo tee /etc/gitd/gitd-mount.conf >/dev/null\n" +
		"  sudo systemctl daemon-reload && sudo systemctl enable --now gitd-mount.socket\n" +
		"Then " + machine.restart + ".\n"
}

// localMachine is what `gitd up` knows of the machine its gitd runs on: the system and
// architecture, as Go names them, the user, gitd's version and configuration file, its
// state and first root, whether the state's filesystem clones files, and how gitd is
// restarted to take a change.
type localMachine struct {
	system, arch string
	user         string
	version      string
	config       string
	state, root  string
	reflink      bool
	restart      string
}

// release is gitd's version when it is a release's, which names its files on GitHub: three
// numbers, as 0.15.0; empty for a build of anything else.
func (machine localMachine) release() string {
	parts := strings.Split(machine.version, ".")
	if len(parts) != 3 {
		return ""
	}
	for _, part := range parts {
		if part == "" || strings.Trim(part, "0123456789") != "" {
			return ""
		}
	}
	return machine.version
}
