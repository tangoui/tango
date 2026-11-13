package autostart

import (
	"strings"
)

// Entry formats, kept free of build tags so every platform tests them.

// launchAgentPlist is a launch agent that runs cmd once at login.
func launchAgentPlist(label string, cmd []string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>`)
	b.WriteString(xmlEscape(label))
	b.WriteString("</string>\n\t<key>ProgramArguments</key>\n\t<array>\n")
	for _, a := range cmd {
		b.WriteString("\t\t<string>")
		b.WriteString(xmlEscape(a))
		b.WriteString("</string>\n")
	}
	// Interactive: no background throttling. LimitLoadToSessionType keeps
	// it out of SSH sessions. No KeepAlive: quitting the app is final.
	b.WriteString(`	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>ProcessType</key>
	<string>Interactive</string>
	<key>LimitLoadToSessionType</key>
	<string>Aqua</string>
</dict>
</plist>
`)
	return b.String()
}

func xmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;")
	return r.Replace(s)
}

// desktopEntry is an XDG autostart entry running cmd.
func desktopEntry(name string, cmd []string) string {
	args := make([]string, len(cmd))
	for i, a := range cmd {
		args[i] = desktopExecArg(a)
	}
	return "[Desktop Entry]\nType=Application\nName=" + desktopString(name) +
		"\nExec=" + strings.Join(args, " ") +
		"\nTerminal=false\nNoDisplay=true\nX-GNOME-Autostart-enabled=true\n"
}

// desktopString escapes a string value (Desktop Entry spec, "Possible
// value types").
func desktopString(s string) string {
	return strings.NewReplacer(`\`, `\\`, "\n", `\n`, "\t", `\t`, "\r", `\r`).Replace(s)
}

// desktopExecArg quotes one Exec argument: reserved characters force
// double quotes, inside which ", `, $ and \ are backslash-escaped; % is
// doubled (field codes). The result is then escaped as a string value,
// which doubles the backslashes again, as the spec requires.
func desktopExecArg(a string) string {
	a = strings.ReplaceAll(a, "%", "%%")
	if a == "" || strings.ContainsAny(a, " \t\n\"'\\><~|&;$*?#()`=") {
		a = `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`", `$`, `\$`).Replace(a) + `"`
	}
	return desktopString(a)
}

// desktopEntryEnabled reads an autostart entry the way desktops do.
func desktopEntryEnabled(data string) bool {
	group := ""
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			group = line
			continue
		}
		if group != "[Desktop Entry]" {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(k) {
		case "Hidden":
			if strings.TrimSpace(v) == "true" {
				return false
			}
		case "X-GNOME-Autostart-enabled":
			if strings.TrimSpace(v) == "false" {
				return false
			}
		}
	}
	return true
}

// windowsCommandLine joins cmd for CreateProcess (the Run key value),
// quoting like syscall.EscapeArg; the program is always quoted.
func windowsCommandLine(cmd []string) string {
	var b strings.Builder
	for i, a := range cmd {
		if i > 0 {
			b.WriteByte(' ')
		}
		if i == 0 {
			b.WriteString(`"` + a + `"`)
			continue
		}
		b.WriteString(windowsEscapeArg(a))
	}
	return b.String()
}

func windowsEscapeArg(s string) string {
	if s == "" {
		return `""`
	}
	if !strings.ContainsAny(s, " \t\"") {
		return s
	}
	var b strings.Builder
	b.WriteByte('"')
	slashes := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\\':
			slashes++
		case '"':
			b.WriteString(strings.Repeat(`\`, slashes+1))
			slashes = 0
		default:
			slashes = 0
		}
		b.WriteByte(c)
	}
	b.WriteString(strings.Repeat(`\`, slashes))
	b.WriteByte('"')
	return b.String()
}

// startupApprovedEnabled reads a StartupApproved\Run value (written by
// Task Manager): the first byte is even when enabled (0x02, 0x06) and odd
// when disabled (0x03, 0x07). No value means enabled.
func startupApprovedEnabled(v []byte) bool {
	return len(v) == 0 || v[0]&1 == 0
}
