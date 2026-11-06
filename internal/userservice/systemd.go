package userservice

import "strings"

// systemdUnit is the unit file of gitd as a systemd user service.
type systemdUnit struct {
	service Service
}

// String is the unit's text, started with the user's session and restarted on failure.
func (unit systemdUnit) String() string {
	words := make([]string, 0, 7)
	for _, word := range unit.service.command() {
		words = append(words, systemdWord(word))
	}
	return "[Unit]\n" +
		"Description=gitd, run for you by gitd up\n" +
		"Documentation=https://github.com/dooservice/gitd\n" +
		"\n" +
		"[Service]\n" +
		"ExecStart=" + strings.Join(words, " ") + "\n" +
		"Restart=on-failure\n" +
		"RestartSec=5\n" +
		"\n" +
		"[Install]\n" +
		"WantedBy=default.target\n"
}

// systemdWord is one word of ExecStart as systemd reads it back: its specifiers (%) and
// variables ($) doubled, and quoted when it holds a space, a quote or a backslash.
func systemdWord(word string) string {
	word = strings.NewReplacer("%", "%%", "$", "$$").Replace(word)
	if word != "" && !strings.ContainsAny(word, " \t\"'\\;") {
		return word
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(word) + `"`
}
