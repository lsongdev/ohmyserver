package system

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeRunner answers commands from recorded output and records what ran.
type fakeRunner struct {
	outputs map[string]string // command line → output
	codes   map[string]int    // command line → exit code
	ran     []string
}

func (f *fakeRunner) Output(_ context.Context, env []string, name string, args ...string) ([]byte, error) {
	line := strings.Join(append([]string{name}, args...), " ")
	f.ran = append(f.ran, line)
	out := f.outputs[line]
	if code := f.codes[line]; code != 0 {
		return []byte(out), &ExitError{Command: name, Code: code}
	}
	return []byte(out), nil
}

func (f *fakeRunner) Stream(ctx context.Context, env []string, out func(string), name string, args ...string) error {
	data, err := f.Output(ctx, env, name, args...)
	for _, line := range lines(data) {
		out(line)
	}
	return err
}

// Recorded on Alibaba Cloud Linux 3.
const checkUpdate = `
bpftool.x86_64                   5.10.134-19.9.al8               alinux3-plus
ca-certificates.noarch           2024.2.69_v8.0.303-80.0.1.al8   alinux3-updates
kernel-headers.x86_64            5.10.134-19.9.al8               alinux3-plus
Obsoleting Packages
kernel-headers.x86_64            5.10.134-19.9.al8               alinux3-plus
    kernel-headers.x86_64        5.10.134-19.8.al8               @alinux3-plus
Security: kernel-core-5.10.134-19.8.al8.x86_64 is an installed security update
`

func TestDNFUpdates(t *testing.T) {
	run := &fakeRunner{outputs: map[string]string{"dnf -q check-update": checkUpdate}, codes: map[string]int{"dnf -q check-update": 100}}
	updates, err := (&DNF{run: run}).Updates(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := fmt.Sprint(updates)
	want := "[{bpftool x86_64 5.10.134-19.9.al8  alinux3-plus} {ca-certificates noarch 2024.2.69_v8.0.303-80.0.1.al8  alinux3-updates} {kernel-headers x86_64 5.10.134-19.9.al8  alinux3-plus}]"
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestDNFUpdatesFailure(t *testing.T) {
	run := &fakeRunner{codes: map[string]int{"dnf -q check-update": 1}}
	if _, err := (&DNF{run: run}).Updates(context.Background()); err == nil {
		t.Fatal("exit 1 is a failure")
	}
}

func TestParseRPM(t *testing.T) {
	out := "python3-pip-wheel\t(none)\t9.0.3-24.0.1.al8\tnoarch\t913900\tThe pip wheel\n" +
		"docker-ce\t3\t26.1.3-1.el8\tx86_64\t95000000\tThe open-source application container engine\n" +
		"gpg-pubkey\t(none)\t621e9f35-58adea78\t(none)\t0\tgpg(Docker Release (CE rpm))\n"
	got := parseRPM([]byte(out))
	if len(got) != 2 || got[0].Version != "9.0.3-24.0.1.al8" || got[1].Version != "3:26.1.3-1.el8" || got[1].Size != 95000000 || !got[1].Installed {
		t.Fatalf("got %+v", got)
	}
}

func TestDNFSearch(t *testing.T) {
	query := "dnf -q repoquery --latest-limit 1 --qf " + repoqueryFormat + " *tree*"
	run := &fakeRunner{outputs: map[string]string{query: "libtree-ldd\t3.0.2-1.el8\tx86_64\tepel\tLike ldd but as a tree\n\n" +
		"tree\t1.7.0-15.el8\tx86_64\talinux3-os\tFile system tree viewer\n\n" +
		"tree\t1.7.0-15.el8\tsrc\talinux3-os\tFile system tree viewer\n\n" +
		"git-subtree\t2.43.7-1.0.1.al8\tx86_64\talinux3-updates\tGit tools\n"}}
	got, err := (&DNF{run: run}).Search(context.Background(), "tree")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range got {
		names = append(names, p.Name)
	}
	if strings.Join(names, " ") != "tree git-subtree libtree-ldd" {
		t.Fatalf("got %v", names)
	}
}

func TestRejectsOptionsAsNames(t *testing.T) {
	run := &fakeRunner{}
	for _, name := range []string{"--setopt=x", "-y", "a b", "a;b", "", "$(id)"} {
		if err := (&DNF{run: run}).Install(context.Background(), []string{name}, func(string) {}); !errors.Is(err, ErrInvalid) {
			t.Errorf("%q: err = %v", name, err)
		}
	}
	if _, err := (&APT{run: run}).Search(context.Background(), "-o"); !errors.Is(err, ErrInvalid) {
		t.Errorf("search -o: err = %v", err)
	}
	if len(run.ran) != 0 {
		t.Fatalf("ran %v", run.ran)
	}
	if err := ValidateNames([]string{"bash.x86_64", "libc6:amd64", "nginx=1.24.0-1", "g++"}); err != nil {
		t.Fatal(err)
	}
}

func TestParseDpkg(t *testing.T) {
	out := "ii \tbash\t5.2.21-2ubuntu4\tamd64\t1648\tGNU Bourne Again SHell\n" +
		"rc \told\t1.0\tamd64\t10\tremoved, config left\n"
	got := parseDpkg([]byte(out))
	if len(got) != 1 || got[0].Name != "bash" || got[0].Size != 1648*1024 {
		t.Fatalf("got %+v", got)
	}
}

func TestParseAptList(t *testing.T) {
	out := "Listing...\n" +
		"curl/noble-updates,noble-security 8.5.0-2ubuntu10.6 amd64 [upgradable from: 8.5.0-2ubuntu10.5]\n" +
		"tree/noble 2.1.1-2ubuntu3 amd64\n" +
		"bash/noble,now 5.2.21-2ubuntu4 amd64 [installed]\n"
	got := parseAptList([]byte(out))
	if len(got) != 3 || got[0].Repo != "noble-updates" || got[0].Version != "8.5.0-2ubuntu10.6" || got[1].Installed || !got[2].Installed {
		t.Fatalf("got %+v", got)
	}
}

func TestELMajor(t *testing.T) {
	for data, want := range map[string]int{
		"ID=\"alinux\"\nVERSION_ID=\"3\"\nPLATFORM_ID=\"platform:al8\"\n":                                  8,
		"ID=\"rocky\"\nID_LIKE=\"rhel centos fedora\"\nVERSION_ID=\"9.4\"\nPLATFORM_ID=\"platform:el9\"\n": 9,
		"ID=centos\nID_LIKE=\"rhel fedora\"\nVERSION_ID=\"7\"\n":                                           7,
		"ID=fedora\nVERSION_ID=40\nPLATFORM_ID=\"platform:f40\"\n":                                         40,
		"ID=ubuntu\nVERSION_ID=\"24.04\"\n":                                                                0,
	} {
		if got := parseOS([]byte(data)).ELMajor(); got != want {
			t.Errorf("%q: got %d, want %d", data, got, want)
		}
	}
}

func TestDockerOnAlibabaCloudLinux(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "run", "systemd", "system"), 0o755)
	run := &fakeRunner{}
	d := &Docker{Manager: &DNF{run: run}, Run: run, Root: root,
		OS: parseOS([]byte("ID=\"alinux\"\nID_LIKE=\"rhel fedora centos anolis\"\nVERSION_ID=\"3\"\nPLATFORM_ID=\"platform:al8\"\n"))}
	if err := d.Install(context.Background(), "aliyun", func(string) {}); err != nil {
		t.Fatal(err)
	}
	repo, _ := os.ReadFile(filepath.Join(root, "etc/yum.repos.d/docker-ce.repo"))
	if !strings.Contains(string(repo), "baseurl=https://mirrors.aliyun.com/docker-ce/linux/centos/8/$basearch/stable") {
		t.Fatalf("repo:\n%s", repo)
	}
	want := []string{
		"dnf -y install docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin",
		"systemctl enable --now docker",
	}
	if strings.Join(run.ran, "\n") != strings.Join(want, "\n") {
		t.Fatalf("ran %q", run.ran)
	}
}

func TestDockerOnUbuntu(t *testing.T) {
	key := "-----BEGIN PGP PUBLIC KEY BLOCK-----\n...\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/linux/ubuntu/gpg" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, key)
	}))
	defer srv.Close()
	saved := DockerSources
	DockerSources = []DockerSource{{ID: "test", URL: srv.URL}}
	defer func() { DockerSources = saved }()

	root := t.TempDir() // no systemd
	run := &fakeRunner{}
	d := &Docker{Manager: &APT{run: run}, Run: run, Root: root, HTTP: srv.Client(),
		OS: parseOS([]byte("ID=ubuntu\nID_LIKE=debian\nVERSION_ID=\"24.04\"\nVERSION_CODENAME=noble\nUBUNTU_CODENAME=noble\n"))}
	if err := d.Install(context.Background(), "test", func(string) {}); err != nil {
		t.Fatal(err)
	}
	list, _ := os.ReadFile(filepath.Join(root, "etc/apt/sources.list.d/docker.list"))
	if string(list) != "deb [signed-by=/etc/apt/keyrings/docker.asc] "+srv.URL+"/linux/ubuntu noble stable\n" {
		t.Fatalf("list: %q", list)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "etc/apt/keyrings/docker.asc")); string(got) != key {
		t.Fatalf("key: %q", got)
	}
	if len(run.ran) != 4 || run.ran[0] != "apt-get update" || !strings.HasSuffix(run.ran[1], "install ca-certificates") ||
		run.ran[2] != "apt-get update" || !strings.HasSuffix(run.ran[3], "install docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin") {
		t.Fatalf("ran %q", run.ran)
	}
}

func TestDockerUnsupportedDistribution(t *testing.T) {
	d := &Docker{Manager: &APT{run: &fakeRunner{}}, Run: &fakeRunner{}, OS: parseOS([]byte("ID=linuxmint\n"))}
	if err := d.Install(context.Background(), "docker", func(string) {}); err == nil || !strings.Contains(err.Error(), "no packages for linuxmint") {
		t.Fatalf("err = %v", err)
	}
}
