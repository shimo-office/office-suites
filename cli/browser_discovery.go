package main

import (
	"os"
	"os/exec"
	"regexp"
	"strings"
)

func browserLabelForBinary(binary string) string {
	path := strings.ToLower(strings.ReplaceAll(binary, `\`, "/"))
	for _, candidate := range []struct{ marker, kind string }{
		{"chromium", "chromium"}, {"msedge", "edge"}, {"microsoft edge", "edge"},
		{"microsoft-edge", "edge"}, {"brave", "brave"}, {"vivaldi", "vivaldi"}, {"chrome", "chrome"},
	} {
		if strings.Contains(path, candidate.marker) {
			return browserLabel(candidate.kind)
		}
	}
	return "Chromium-compatible"
}

// Query the registered HTTPS handler, not whichever installed browser happens
// to be found first. Never run the registry's command string or its arguments.
func windowsDefaultChromiumBinary() (string, string) {
	out, err := exec.Command("reg.exe", "query", `HKCU\Software\Microsoft\Windows\Shell\Associations\UrlAssociations\https\UserChoice`, "/v", "ProgId").Output()
	if err != nil {
		return "", ""
	}
	progID := registryStringValue(string(out), "ProgId")
	if !regexp.MustCompile(`^[A-Za-z0-9_.-]+$`).MatchString(progID) {
		return "", ""
	}
	out, err = exec.Command("reg.exe", "query", `HKCR\`+progID+`\shell\open\command`, "/ve").Output()
	if err != nil {
		return "", ""
	}
	binary, label := windowsBrowserRegistration(progID, registryStringValue(string(out), ""))
	if st, err := os.Stat(binary); binary != "" && err == nil && !st.IsDir() {
		return binary, label
	}
	return "", ""
}

func registryStringValue(output, name string) string {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 || (name != "" && !strings.EqualFold(fields[0], name)) {
			continue
		}
		if fields[1] != "REG_SZ" && fields[1] != "REG_EXPAND_SZ" {
			continue
		}
		_, value, ok := strings.Cut(line, fields[1])
		if ok {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func windowsBrowserRegistration(progID, command string) (string, string) {
	var kind, executable string
	switch id := strings.ToLower(progID); {
	case strings.HasPrefix(id, "chromehtml"):
		kind, executable = "chrome", "chrome.exe"
	case strings.HasPrefix(id, "chromiumhtm"):
		kind, executable = "chromium", "chrome.exe"
	case strings.HasPrefix(id, "msedgehtm"):
		kind, executable = "edge", "msedge.exe"
	case strings.HasPrefix(id, "bravehtml"):
		kind, executable = "brave", "brave.exe"
	case strings.HasPrefix(id, "vivaldihtm"):
		kind, executable = "vivaldi", "vivaldi.exe"
	default:
		return "", ""
	}
	command = strings.TrimSpace(command)
	var binary string
	if strings.HasPrefix(command, `"`) {
		end := strings.Index(command[1:], `"`)
		if end < 0 {
			return "", ""
		}
		binary = command[1 : end+1]
	} else if fields := strings.Fields(command); len(fields) > 0 {
		binary = fields[0]
	}
	// Require an absolute drive path and the expected browser executable. Unknown
	// handlers, wrappers and ambiguous unquoted paths use the system open path.
	if !regexp.MustCompile(`^[A-Za-z]:[\\/]`).MatchString(binary) {
		return "", ""
	}
	parts := strings.Split(strings.ReplaceAll(binary, `\`, "/"), "/")
	if !strings.EqualFold(parts[len(parts)-1], executable) {
		return "", ""
	}
	return binary, browserLabel(kind)
}
