package system

import "testing"

func TestBrewParsers(t *testing.T) {
	installed, err := parseBrewInfo([]byte(`{"formulae":[
		{"name":"abseil","tap":"homebrew/core","desc":"C++ Common Libraries","installed":[{"version":"20260817.0"}]},
		{"name":"gone","installed":[]}],
	 "casks":[{"token":"orbstack","desc":"Docker and Linux on macOS","installed":"2.0.1"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(installed) != 2 || installed[0].Version != "20260817.0" || installed[0].Repo != "homebrew/core" ||
		installed[1].Name != "orbstack" || installed[1].Repo != "cask" || !installed[1].Installed {
		t.Fatalf("installed = %+v", installed)
	}
	updates, err := parseBrewOutdated([]byte(`{"formulae":[{"name":"aria2","installed_versions":["1.37.0_2"],"current_version":"1.37.0_3"}],
		"casks":[{"name":"firefox","installed_versions":["150.0"],"current_version":"151.0"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) != 2 || updates[0].Current != "1.37.0_2" || updates[0].Version != "1.37.0_3" || updates[1].Repo != "cask" {
		t.Fatalf("updates = %+v", updates)
	}
	names := parseBrewSearch([]byte("==> Formulae\nsmartmontools\ngsmartcontrol\n\nIf you meant \"smart\" precisely:\n"))
	if len(names) != 2 || names[0] != "smartmontools" {
		t.Fatalf("search = %v", names)
	}
}

func TestReadMacOSVersion(t *testing.T) {
	o := parseMacOS([]byte(`<dict><key>ProductName</key>
	<string>macOS</string><key>ProductVersion</key>
	<string>27.1</string></dict>`))
	if o.ID != "macos" || o.PrettyName != "macOS 27.1" {
		t.Fatalf("os = %+v", o)
	}
}
