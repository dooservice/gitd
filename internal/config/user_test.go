//go:build !windows

package config

import "testing"

func TestAUsersPlacesFollowXDGAndFallBackUnderTheHome(t *testing.T) {
	set := map[string]string{
		"XDG_CONFIG_HOME": "/home/ana/cfg",
		"XDG_STATE_HOME":  "/home/ana/st",
		"XDG_RUNTIME_DIR": "/run/user/1000",
	}
	got := environment{home: "/home/ana", variable: func(name string) string { return set[name] }}.xdg()
	want := User{
		Config: "/home/ana/cfg/gitd/gitd.toml",
		State:  "/home/ana/st/gitd",
		Socket: "/run/user/1000/gitd/gitd.sock",
		Root:   "/home/ana/gitd/workspaces",
	}
	if got != want {
		t.Errorf("with the variables set, %+v; want %+v", got, want)
	}
	set = map[string]string{"XDG_CONFIG_HOME": "relative/cfg"}
	got = environment{home: "/home/ana", variable: func(name string) string { return set[name] }}.xdg()
	want = User{
		Config: "/home/ana/.config/gitd/gitd.toml",
		State:  "/home/ana/.local/state/gitd",
		Socket: "/home/ana/.local/state/gitd/gitd.sock",
		Root:   "/home/ana/gitd/workspaces",
	}
	if got != want {
		t.Errorf("with none set, or a relative one, %+v; want %+v", got, want)
	}
}

func TestAUsersPlacesOnMacOSAreInApplicationSupport(t *testing.T) {
	got := environment{home: "/Users/ana", variable: func(string) string { return "/ignored" }}.library()
	want := User{
		Config: "/Users/ana/Library/Application Support/gitd/gitd.toml",
		State:  "/Users/ana/Library/Application Support/gitd/state",
		Socket: "/Users/ana/Library/Application Support/gitd/gitd.sock",
		Root:   "/Users/ana/gitd/workspaces",
	}
	if got != want {
		t.Errorf("%+v; want %+v", got, want)
	}
}
