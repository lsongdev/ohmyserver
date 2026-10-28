package system

import (
	"context"
	"errors"
	"testing"
)

// Recorded on Arch Linux (pacman 7, 2026).
const pacmanQi = `Name            : bash
Version         : 5.3.20-1
Description     : The GNU Bourne Again shell
Architecture    : x86_64
Optional Deps   : bash-completion: for tab completion
Installed Size  : 9.59 MiB
Build Date      : Wed Sep 16 03:47:19 2026

Name            : zfs-utils
Version         : 2.4.4-3
Description     : Kernel module support files for the Zettabyte File System.
Architecture    : x86_64
Optional Deps   : curl: HTTP(S) key locations and ZED web notifications [installed]
                  dracut: dracut initramfs integration
Installed Size  : 1.50 GiB

Name            : ca-certificates
Version         : 20240618-1
Description     : Common CA certificates (default providers)
Architecture    : any
Installed Size  : 0.00 B
`

func TestPacmanParsers(t *testing.T) {
	installed := parsePacmanInfo([]byte(pacmanQi))
	if len(installed) != 3 || installed[0].Name != "bash" || installed[0].Version != "5.3.20-1" ||
		installed[0].Size != pacmanSize("9.59 MiB") || installed[0].Size < 10_000_000 || installed[1].Size != int64(1.5*(1<<30)) || installed[2].Arch != "" {
		t.Fatalf("installed = %+v", installed)
	}
	found := parsePacmanSearch([]byte("extra/libblockdev-smart 3.5.0-2\n    A library for manipulating block devices - S.M.A.R.T. support\n" +
		"extra/smartmontools 7.5-2 [installed]\n    Control and monitor S.M.A.R.T. enabled ATA and SCSI Hard Drives\n"))
	if len(found) != 2 || found[1].Name != "smartmontools" || !found[1].Installed || found[1].Repo != "extra" || found[0].Summary == "" {
		t.Fatalf("search = %+v", found)
	}
	updates := parsePacmanUpdates([]byte("linux 6.17.1.arch1-1 -> 6.17.2.arch1-1\nzfs-linux 2.4.4_6.17.1.arch1.1-1 -> 2.4.4_6.17.2.arch1.1-1\n"))
	if len(updates) != 2 || updates[0].Current != "6.17.1.arch1-1" || updates[0].Version != "6.17.2.arch1-1" {
		t.Fatalf("updates = %+v", updates)
	}
}

func TestPacmanRefusesPartialUpgrades(t *testing.T) {
	p := &Pacman{}
	if err := p.Upgrade(context.Background(), []string{"bash"}, func(string) {}); !errors.Is(err, ErrPartialUpgrade) {
		t.Fatalf("err = %v", err)
	}
}
