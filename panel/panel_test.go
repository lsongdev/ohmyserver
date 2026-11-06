package panel

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	jsImport    = regexp.MustCompile(`(?:from\s+|import\s*\(\s*|import\s+)'([^']+)'`)
	lineComment = regexp.MustCompile(`(?m)^\s*//.*$`) // usage examples in doc comments
)

// scripts is the copy of @lsongdev/scripts the shell serves to every module.
const scripts = "../internal/shell/ui/static/node_modules/@lsongdev/scripts/"

// Every import of the shell, the home module and the panel modules resolves:
// relative ones within their UI, "@lsongdev/panel/…" to this runtime and
// "@lsongdev/scripts/…" to the shell's copy, which the shell's import map
// points them at. No other bare specifier is mapped.
func TestImportsResolve(t *testing.T) {
	if _, err := os.Stat(scripts + "package.json"); err != nil {
		t.Fatalf("the shell's browser dependencies are missing, run make deps: %v", err)
	}
	uis, _ := filepath.Glob("../modules/*/web/ui")
	uis = append(uis, "../internal/shell/ui/static", "../home/ui", "ui")
	for _, ui := range uis {
		filepath.WalkDir(ui, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			if d.IsDir() || path.Ext(p) != ".js" {
				return nil
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			for _, m := range jsImport.FindAllSubmatch(lineComment.ReplaceAll(data, nil), -1) {
				spec := string(m[1])
				var target string
				switch {
				case strings.HasPrefix(spec, "."):
					target = filepath.Join(filepath.Dir(p), spec)
				case strings.HasPrefix(spec, "@lsongdev/panel/"):
					target = filepath.Join("ui", strings.TrimPrefix(spec, "@lsongdev/panel/"))
				case strings.HasPrefix(spec, "@lsongdev/scripts/"):
					target = scripts + strings.TrimPrefix(spec, "@lsongdev/scripts/")
				default:
					t.Errorf("%s imports %s, which the import map does not resolve", p, spec)
					continue
				}
				if _, err := os.Stat(target); err != nil {
					t.Errorf("%s imports %s: %v", p, spec, err)
				}
			}
			return nil
		})
	}
}
