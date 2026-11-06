package userservice

import (
	"encoding/xml"
	"strings"
)

// launchAgent is the property list of gitd as a launchd agent, logging to log.
type launchAgent struct {
	service Service
	log     string
}

// String is the property list: started at login and restarted when it fails.
func (agent launchAgent) String() string {
	var text strings.Builder
	text.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + Label + `</string>
	<key>ProgramArguments</key>
	<array>
`)
	for _, word := range agent.service.command() {
		text.WriteString("\t\t<string>" + escaped(word) + "</string>\n")
	}
	text.WriteString(`	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<dict>
		<key>SuccessfulExit</key>
		<false/>
	</dict>
	<key>StandardOutPath</key>
	<string>` + escaped(agent.log) + `</string>
	<key>StandardErrorPath</key>
	<string>` + escaped(agent.log) + `</string>
</dict>
</plist>
`)
	return text.String()
}

// escaped is text as XML character data.
func escaped(text string) string {
	var out strings.Builder
	_ = xml.EscapeText(&out, []byte(text))
	return out.String()
}
