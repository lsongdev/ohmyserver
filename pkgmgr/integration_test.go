package pkgmgr

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestDetectedManagerReadOnly(t *testing.T) {
	if os.Getenv("PKGMGR_INTEGRATION") == "" {
		t.Skip("set PKGMGR_INTEGRATION=1 to query the host package manager")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	manager, err := Detect(Options{})
	if err != nil {
		t.Fatal(err)
	}
	packages, err := manager.ListInstalled(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(packages) == 0 {
		t.Fatalf("%s returned no installed packages", manager.Name())
	}
	assertPackageNames(t, "installed", packages)

	upgradable, err := manager.ListUpgradable(ctx)
	if err != nil {
		t.Fatal(err)
	}
	assertPackageNames(t, "upgradable", upgradable)

	probePackage := "curl"
	if manager.Backend() == APK || manager.Backend() == OPKG {
		probePackage = "busybox"
	}
	found, err := manager.Search(ctx, probePackage)
	if err != nil {
		t.Fatal(err)
	}
	assertPackageNames(t, "search", found)

	info, err := manager.Info(ctx, []string{probePackage})
	if err != nil {
		t.Fatal(err)
	}
	if len(info) == 0 {
		t.Fatalf("%s returned no package info for %s", manager.Name(), probePackage)
	}
	assertPackageNames(t, "info", info)
}

func assertPackageNames(t *testing.T, operation string, packages []Package) {
	t.Helper()
	for _, item := range packages {
		if item.Name == "" {
			t.Fatalf("%s returned a package with an empty name", operation)
		}
	}
}
