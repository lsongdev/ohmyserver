package modem

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var ErrNotFound = errors.New("RM520N AT port not found")

// Discover locates the AT interface by USB interface number independently
// of the order in which Linux assigned ttyUSB names.
func Discover() (string, error) {
	return discover("/sys/class/tty", "/dev")
}

func discover(sys, dev string) (string, error) {
	entries, err := filepath.Glob(filepath.Join(sys, "ttyUSB*"))
	if err != nil {
		return "", err
	}
	var found string
	for _, tty := range entries {
		serialDevice, err := filepath.EvalSymlinks(filepath.Join(tty, "device"))
		iface := filepath.Dir(serialDevice)
		if err != nil || attribute(iface, "bInterfaceNumber") != "02" {
			continue
		}
		device := filepath.Dir(iface)
		if attribute(device, "idVendor") != "2c7c" || attribute(device, "idProduct") != "0801" {
			continue
		}
		if found != "" {
			return "", errors.New("multiple Quectel AT ports found; configure an explicit port")
		}
		found = filepath.Join(dev, filepath.Base(tty))
	}
	if found == "" {
		return "", ErrNotFound
	}
	return found, nil
}

func attribute(dir, name string) string {
	data, _ := os.ReadFile(filepath.Join(dir, name))
	return strings.TrimSpace(string(data))
}
