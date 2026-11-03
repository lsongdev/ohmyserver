package system

import (
	"errors"
	"fmt"
	"strings"
)

// ErrInvalid marks a request that names or asks for something malformed.
var ErrInvalid = errors.New("invalid")

// ErrConflict marks a change based on a state that no longer holds.
var ErrConflict = errors.New("conflict")

// ErrNotFound marks a dataset or snapshot that does not exist.
var ErrNotFound = errors.New("not found")

// validComponent checks one part of a ZFS name. It is stricter than zfs on
// purpose: it refuses "%" and "," outright, because `zfs destroy a@b%c` and
// `a@b,c` destroy ranges and lists of snapshots.
func validComponent(s string) bool {
	if s == "" || len(s) > 255 || s[0] == '-' || s == "." || s == ".." {
		return false
	}
	for _, c := range s {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '-', c == '_', c == '.', c == ':', c == ' ':
		default:
			return false
		}
	}
	return s[0] != ' ' && s[len(s)-1] != ' '
}

// ValidDataset checks a dataset name such as data/media.
func ValidDataset(name string) error {
	parts := strings.Split(name, "/")
	for _, part := range parts {
		if !validComponent(part) {
			return fmt.Errorf("%w dataset name %q", ErrInvalid, name)
		}
	}
	return nil
}

// validPool checks a pool's name: one part of a dataset name, starting with a
// letter as zpool requires.
func validPool(name string) bool {
	return validComponent(name) && (name[0] >= 'a' && name[0] <= 'z' || name[0] >= 'A' && name[0] <= 'Z') && !strings.Contains(name, " ")
}

// ValidSnapshotName checks the part of a snapshot's name after the @.
func ValidSnapshotName(name string) error {
	if !validComponent(name) {
		return fmt.Errorf("%w snapshot name %q: use letters, digits and - _ . : (not starting with -)", ErrInvalid, name)
	}
	return nil
}

// SplitSnapshot splits dataset@name. Every command that destroys or rolls
// back goes through it: a name without its @ would be a whole dataset.
func SplitSnapshot(full string) (dataset, name string, err error) {
	dataset, name, ok := strings.Cut(full, "@")
	if !ok || strings.Contains(name, "@") {
		return "", "", fmt.Errorf("%w snapshot %q: expected dataset@name", ErrInvalid, full)
	}
	if err := ValidDataset(dataset); err != nil {
		return "", "", err
	}
	if err := ValidSnapshotName(name); err != nil {
		return "", "", err
	}
	return dataset, name, nil
}
