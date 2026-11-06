//go:build !windows

package cli

import (
	"strings"
	"testing"

	gitdv1 "github.com/dooservice/gitd/gen/go/gitd/v1"
)

func copied(reason string) copyOnWrite {
	return copyOnWrite{answer: &gitdv1.InfoResponse{Strategy: gitdv1.Strategy_STRATEGY_COPY, StrategyReason: reason}}
}

func TestOnlyACopyForWantOfAMountIsAdvised(t *testing.T) {
	unprivileged := "overlay cannot run here: strategy.Kernel: operation not permitted; mounthelper.Client: dial unix: no such file"
	if copied(unprivileged).hint() == "" {
		t.Errorf("no hint for %q", unprivileged)
	}
	for _, advice := range []copyOnWrite{
		copied("configured"),
		copied("overlay cannot run here: /var/lib/gitd is itself on an overlay, as a container's own filesystem is: put it on a volume"),
		copied("overlay cannot run here: strategy.Kernel: not supported here: macOS has no overlay; templates cannot be cloned here: x"),
		{answer: &gitdv1.InfoResponse{Strategy: gitdv1.Strategy_STRATEGY_OVERLAY, StrategyReason: "an overlay mounts here"}},
	} {
		if hint := advice.hint(); hint != "" {
			t.Errorf("for %q, %q", advice.answer.GetStrategyReason(), hint)
		}
		if steps := advice.steps(localMachine{system: "linux"}); steps != "" {
			t.Errorf("for %q, %q", advice.answer.GetStrategyReason(), steps)
		}
	}
}

func TestReflinkIsAdvisedWhereTheStateClonesFiles(t *testing.T) {
	steps := copied("overlay cannot run here: strategy.Kernel: operation not permitted").steps(localMachine{
		system: "linux", state: "/home/ana/.local/state/gitd", config: "/home/ana/.config/gitd/gitd.toml", reflink: true,
		restart: "run gitd down && gitd up",
	})
	for _, want := range []string{"add this to /home/ana/.config/gitd/gitd.toml", "[workspaces]\n  strategy = \"reflink\"", "gitd down && gitd up"} {
		if !strings.Contains(steps, want) {
			t.Errorf("the steps\n%s\nsay nothing of %q", steps, want)
		}
	}
	if strings.Contains(steps, "sudo") {
		t.Errorf("reflink needs no root, yet\n%s", steps)
	}
}

func TestGitdMountIsInstalledForTheUserFromTheRelease(t *testing.T) {
	machine := localMachine{
		system: "linux", arch: "arm64", user: "ana", version: "0.15.0", config: "/home/ana/.config/gitd/gitd.toml",
		state: "/home/ana/.local/state/gitd", root: "/home/ana/my work/gitd", restart: "run gitd down && gitd up",
	}
	steps := copied("overlay cannot run here: strategy.Kernel: operation not permitted").steps(machine)
	for _, want := range []string{
		"sudo curl -fsSLo /usr/local/bin/gitd-mount https://github.com/dooservice/gitd/releases/download/0.15.0/gitd-mount-linux-arm64\n",
		"https://raw.githubusercontent.com/dooservice/gitd/0.15.0/packaging/gitd-mount.service\n",
		"s/^SocketUser=gitd$/SocketUser=ana/",
		"printf '%s\\n' 'user ana' 'templates /home/ana/.local/state/gitd/templates' 'layers /home/ana/.local/state/gitd/layers' 'root /home/ana/my work/gitd' | sudo tee /etc/gitd/gitd-mount.conf",
		"sudo systemctl enable --now gitd-mount.socket\nThen run gitd down && gitd up.\n",
	} {
		if !strings.Contains(steps, want) {
			t.Errorf("the steps\n%s\nsay nothing of %q", steps, want)
		}
	}
	machine.version = "0.15.0-3-gabc1234"
	if steps := copied("overlay cannot run here: x").steps(machine); !strings.Contains(steps, "/releases/latest/download/") ||
		!strings.Contains(steps, "/gitd/main/packaging/") {
		t.Errorf("a build of no release should take the latest one:\n%s", steps)
	}
	machine.system = "darwin"
	if steps := copied("overlay cannot run here: x").steps(machine); steps != "" {
		t.Errorf("gitd-mount is Linux's, yet on macOS:\n%s", steps)
	}
}
