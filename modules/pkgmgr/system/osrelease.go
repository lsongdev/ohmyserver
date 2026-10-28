package system

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// OS describes the distribution, from /etc/os-release.
type OS struct {
	ID         string   `json:"id"`             // e.g. ubuntu, debian, alinux, rocky
	Like       []string `json:"like,omitempty"` // ID_LIKE
	Version    string   `json:"version"`        // VERSION_ID
	Codename   string   `json:"codename"`       // Debian and Ubuntu release codename
	Platform   string   `json:"platform"`       // PLATFORM_ID, e.g. platform:el9
	PrettyName string   `json:"name"`
}

// ReadOS reads root/etc/os-release (or root/usr/lib/os-release), or on
// macOS its SystemVersion.plist.
func ReadOS(root string) OS {
	data, err := os.ReadFile(filepath.Join(root, "etc", "os-release"))
	if err != nil {
		data, err = os.ReadFile(filepath.Join(root, "usr", "lib", "os-release"))
	}
	if err != nil {
		if mac, err := os.ReadFile(filepath.Join(root, "System", "Library", "CoreServices", "SystemVersion.plist")); err == nil {
			return parseMacOS(mac)
		}
	}
	return parseOS(data)
}

var plistString = regexp.MustCompile(`<key>(ProductName|ProductVersion)</key>\s*<string>([^<]*)</string>`)

// parseMacOS reads ProductName and ProductVersion from SystemVersion.plist.
func parseMacOS(data []byte) OS {
	values := map[string]string{}
	for _, m := range plistString.FindAllSubmatch(data, -1) {
		values[string(m[1])] = string(m[2])
	}
	name := cmpOr(values["ProductName"], "macOS")
	return OS{ID: "macos", Version: values["ProductVersion"], PrettyName: strings.TrimSpace(name + " " + values["ProductVersion"])}
}

func parseOS(data []byte) OS {
	values := map[string]string{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		key, value, ok := strings.Cut(strings.TrimSpace(scanner.Text()), "=")
		if !ok || strings.HasPrefix(key, "#") {
			continue
		}
		if unquoted, err := strconv.Unquote(value); err == nil {
			value = unquoted
		} else {
			value = strings.Trim(value, `'"`)
		}
		values[key] = value
	}
	return OS{
		ID: values["ID"], Like: strings.Fields(values["ID_LIKE"]), Version: values["VERSION_ID"],
		Codename:   cmpOr(values["UBUNTU_CODENAME"], values["VERSION_CODENAME"]),
		Platform:   values["PLATFORM_ID"],
		PrettyName: cmpOr(values["PRETTY_NAME"], values["NAME"]),
	}
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// Is reports whether the distribution is id or derives from it.
func (o OS) Is(id string) bool { return o.ID == id || slices.Contains(o.Like, id) }

var platformMajor = regexp.MustCompile(`^platform:[a-z]+(\d+)$`)

// ELMajor returns the Enterprise Linux release the distribution is
// compatible with (8 for Alibaba Cloud Linux 3 and Rocky 8), or 0.
func (o OS) ELMajor() int {
	if m := platformMajor.FindStringSubmatch(o.Platform); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	if o.Is("rhel") || o.Is("centos") {
		major, _, _ := strings.Cut(o.Version, ".")
		n, _ := strconv.Atoi(major)
		return n
	}
	return 0
}
