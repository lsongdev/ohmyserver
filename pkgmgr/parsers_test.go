package pkgmgr

import (
	"reflect"
	"testing"
)

func TestPackageParsers(t *testing.T) {
	t.Run("apt upgradable", func(t *testing.T) {
		got := parseAPTUpgradable("Listing...\ncurl/stable 8.1.0 amd64 [upgradable from: 8.0.0]\n")
		want := []Package{{
			Name: "curl", Version: "8.0.0", LatestVersion: "8.1.0",
			Architecture: "amd64", Repository: "stable", Installed: true,
		}}
		assertPackages(t, got, want)
	})

	t.Run("dnf tabs", func(t *testing.T) {
		got, err := parseTabPackages("curl\t8.1-1\tx86_64\tcommand line tool\n", true, false)
		if err != nil {
			t.Fatal(err)
		}
		want := []Package{{
			Name: "curl", Version: "8.1-1", Architecture: "x86_64",
			Description: "command line tool", Installed: true,
		}}
		assertPackages(t, got, want)
	})

	t.Run("pacman search", func(t *testing.T) {
		got := parsePacmanSearch("core/curl 8.1-1 [installed]\n    command line tool\n")
		want := []Package{{
			Name: "curl", Version: "8.1-1", Repository: "core",
			Description: "command line tool", Installed: true,
		}}
		assertPackages(t, got, want)
	})

	t.Run("apk list", func(t *testing.T) {
		got := parseAPKList("py3-requests-2.32.3-r0 x86_64 {py3-requests} [installed]\n", true, false)
		want := []Package{{
			Name: "py3-requests", Version: "2.32.3-r0",
			Architecture: "x86_64", Installed: true,
		}}
		assertPackages(t, got, want)
	})

	t.Run("opkg upgradable", func(t *testing.T) {
		got := parseOPKGList("curl - 8.0 - 8.1\n", true, true)
		want := []Package{{
			Name: "curl", Version: "8.0", LatestVersion: "8.1", Installed: true,
		}}
		assertPackages(t, got, want)
	})

	t.Run("homebrew json", func(t *testing.T) {
		got, err := parseBrewJSON(`{
			"formulae": [{
				"name": "curl",
				"full_name": "curl",
				"desc": "transfer tool",
				"installed_versions": ["8.0"],
				"current_version": "8.1"
			}],
			"casks": [{
				"name": "firefox",
				"desc": "Web browser",
				"installed_versions": ["127.0"],
				"current_version": "128.0"
			}]
		}`, true, true)
		if err != nil {
			t.Fatal(err)
		}
		want := []Package{
			{
				Name: "curl", Version: "8.0", LatestVersion: "8.1",
				Description: "transfer tool", Installed: true,
			},
			{
				Name: "firefox", Version: "127.0", LatestVersion: "128.0",
				Description: "Web browser", Installed: true,
			},
		}
		assertPackages(t, got, want)
	})

	t.Run("homebrew installed", func(t *testing.T) {
		got := parseBrewInstalled("curl 8.0\nopenssl@3 3.6.0 3.6.1\n")
		want := []Package{
			{Name: "curl", Version: "8.0", Installed: true},
			{Name: "openssl@3", Version: "3.6.1", Installed: true},
		}
		assertPackages(t, got, want)
	})
}

func TestControlParagraphContinuation(t *testing.T) {
	got := parseControlParagraphs(
		"Package: curl\nVersion: 8.1\nArchitecture: amd64\nDescription: first line\n second line\n\n",
		controlMapping{
			name: "Package", version: "Version", description: "Description",
			architecture: "Architecture",
		},
	)
	want := []Package{{
		Name: "curl", Version: "8.1", Architecture: "amd64",
		Description: "first line second line",
	}}
	assertPackages(t, got, want)
}

func assertPackages(t *testing.T, got, want []Package) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("packages = %#v, want %#v", got, want)
	}
}
