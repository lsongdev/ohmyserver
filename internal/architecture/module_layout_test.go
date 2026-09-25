package architecture

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type moduleManifest struct {
	ContractVersion int      `json:"contractVersion"`
	ID              string   `json:"id"`
	Entry           string   `json:"entry"`
	Styles          []string `json:"styles"`
}

func TestHostLifecycleIsNotRegisteredThroughPackageInit(t *testing.T) {
	root := filepath.Join("..", "host")
	err := filepath.WalkDir(root, func(file string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(file) != ".go" || strings.HasSuffix(file, "_test.go") {
			return nil
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.Ident)
			if !ok {
				return true
			}
			if strings.HasPrefix(selector.Name, "Register") && strings.HasSuffix(selector.Name, "Hook") {
				t.Errorf("%s registers lifecycle behavior through package initialization; list it explicitly in host/lifecycle.go", file)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestFeatureModulesHavePublishableLayout(t *testing.T) {
	root := filepath.Join("..", "..", "modules")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		moduleRoot := filepath.Join(root, name)
		t.Run(name, func(t *testing.T) {
			moduleFile, err := os.ReadFile(filepath.Join(moduleRoot, "go.mod"))
			if err != nil {
				t.Fatalf("module needs its own go.mod: %v", err)
			}
			wantPath := "module github.com/lsongdev/ohmyserver/modules/" + name
			if !strings.Contains(string(moduleFile), wantPath) {
				t.Errorf("go.mod must declare %q", wantPath)
			}
			readme, err := os.ReadFile(filepath.Join(moduleRoot, "README.md"))
			if err != nil || len(strings.TrimSpace(string(readme))) == 0 {
				t.Errorf("module needs a non-empty README.md: %v", err)
			}
			if _, err := os.Stat(filepath.Join(moduleRoot, "cmd", name, "main.go")); err != nil {
				t.Errorf("module needs cmd/%s/main.go: %v", name, err)
			}

			assetsRoot := filepath.Join(moduleRoot, "web", "assets")
			for _, file := range []string{"index.html", "index.js", "module.js", "manifest.json"} {
				if _, err := os.Stat(filepath.Join(assetsRoot, file)); err != nil {
					t.Errorf("module needs web/assets/%s: %v", file, err)
				}
			}
			manifestBytes, err := os.ReadFile(filepath.Join(assetsRoot, "manifest.json"))
			if err != nil {
				t.Fatalf("read module manifest: %v", err)
			}
			var manifest moduleManifest
			if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
				t.Fatalf("parse module manifest: %v", err)
			}
			if manifest.ContractVersion != 1 || manifest.ID != name || manifest.Entry != "assets/module.js" {
				t.Errorf("manifest must declare contractVersion=1, id=%q, entry=assets/module.js; got %+v", name, manifest)
			}
			for _, asset := range append([]string{manifest.Entry}, manifest.Styles...) {
				clean := filepath.Clean(filepath.FromSlash(asset))
				if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
					t.Errorf("manifest asset %q must stay inside the module assets", asset)
					continue
				}
				if _, err := os.Stat(filepath.Join(moduleRoot, "web", clean)); err != nil {
					t.Errorf("manifest asset %q does not exist: %v", asset, err)
				}
			}
			page, err := os.ReadFile(filepath.Join(assetsRoot, "index.html"))
			if err == nil && !strings.Contains(string(page), "lsong.org/stylesheets/index.css") {
				t.Error("standalone page must use the shared component stylesheet")
			}
			entry, err := os.ReadFile(filepath.Join(assetsRoot, "index.js"))
			if err == nil && !strings.Contains(string(entry), "lsong.org/scripts/react/index.js") {
				adapter, adapterErr := os.ReadFile(filepath.Join(assetsRoot, "standalone-ui.js"))
				if adapterErr != nil || !strings.Contains(string(adapter), "lsong.org/scripts/react/index.js") || !strings.Contains(string(entry), "./module.js") {
					t.Error("standalone entry must either use the shared Preact/HTM runtime directly or load module.js through a shared runtime adapter")
				}
			}
		})
	}
}
