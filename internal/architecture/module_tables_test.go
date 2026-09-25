package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var sqlTableReference = regexp.MustCompile(`(?i)\b(?:FROM|JOIN|INTO|REFERENCES|TABLE(?:\s+IF\s+NOT\s+EXISTS)?)\s+([a-z_][a-z0-9_]*)`)
var sqlUpdateReference = regexp.MustCompile(`(?i)\bUPDATE\s+([a-z_][a-z0-9_]*)\s+SET\b`)
var sqlPragmaTableReference = regexp.MustCompile(`(?i)\bPRAGMA\s+(?:table_info|foreign_key_list)\s*\(\s*['"]?([a-z_][a-z0-9_]*)`)
var sqlCreateTable = regexp.MustCompile(`(?i)\bCREATE\s+TABLE(?:\s+IF\s+NOT\s+EXISTS)?\s+([a-z_][a-z0-9_]*)`)
var sqlStatementStart = regexp.MustCompile(`(?i)^\s*(?:SELECT|INSERT|REPLACE|UPDATE|DELETE|CREATE|ALTER|DROP|PRAGMA|WITH)\b`)
var dynamicSQLTableIdentifier = regexp.MustCompile(`(?i)\b(?:FROM|JOIN|INTO|REFERENCES|UPDATE|TABLE(?:\s+IF\s+NOT\s+EXISTS)?)\s+(?:__DYNAMIC_SQL_VALUE__|%[a-z]|\?)`)

func sqlExpressionText(expr ast.Expr) string {
	switch value := expr.(type) {
	case *ast.BasicLit:
		if value.Kind != token.STRING {
			return "__DYNAMIC_SQL_VALUE__"
		}
		text, err := strconv.Unquote(value.Value)
		if err != nil {
			return "__DYNAMIC_SQL_VALUE__"
		}
		return text
	case *ast.BinaryExpr:
		if value.Op != token.ADD {
			return "__DYNAMIC_SQL_VALUE__"
		}
		return sqlExpressionText(value.X) + sqlExpressionText(value.Y)
	case *ast.CallExpr:
		if selector, ok := value.Fun.(*ast.SelectorExpr); ok && selector.Sel.Name == "Sprintf" && len(value.Args) > 0 {
			return sqlExpressionText(value.Args[0]) + " __DYNAMIC_SQL_VALUE__"
		}
		return "__DYNAMIC_SQL_VALUE__"
	default:
		return "__DYNAMIC_SQL_VALUE__"
	}
}

func referencedTables(sql string) []string {
	var tables []string
	for _, pattern := range []*regexp.Regexp{sqlTableReference, sqlUpdateReference, sqlPragmaTableReference} {
		for _, match := range pattern.FindAllStringSubmatch(sql, -1) {
			tables = append(tables, strings.ToLower(match[1]))
		}
	}
	return tables
}

type sourceSQL struct {
	module string
	file   string
	text   string
}

func TestModulesOnlyReferenceOwnedTables(t *testing.T) {
	root := filepath.Join("..", "..", "modules")
	var sources []sourceSQL
	err := filepath.WalkDir(root, func(file string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(file, ".go") || strings.HasSuffix(file, "_test.go") {
			return nil
		}
		relative, err := filepath.Rel(root, file)
		if err != nil {
			return err
		}
		module := strings.Split(filepath.ToSlash(relative), "/")[0]
		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if ok && literal.Kind == token.STRING {
				sqlText, err := strconv.Unquote(literal.Value)
				if err == nil {
					sources = append(sources, sourceSQL{module: module, file: file, text: sqlText})
				}
			}
			if expression, ok := node.(ast.Expr); ok {
				text := sqlExpressionText(expression)
				if sqlStatementStart.MatchString(text) && dynamicSQLTableIdentifier.MatchString(text) {
					t.Errorf("%s dynamically constructs a SQL table identifier in %q; use a static, module-owned table name", file, text)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Discover table ownership from module schema declarations, avoiding a
	// second manually maintained list that could drift from repository schemas.
	owners := make(map[string]string)
	for _, source := range sources {
		for _, match := range sqlCreateTable.FindAllStringSubmatch(source.text, -1) {
			table := strings.ToLower(match[1])
			if owner, exists := owners[table]; exists && owner != source.module {
				t.Errorf("table %q is created by both modules %q and %q", table, owner, source.module)
			}
			owners[table] = source.module
		}
	}
	for _, source := range sources {
		for _, table := range referencedTables(source.text) {
			if table == "set" || table == "sqlite_master" || table == "pragma_table_info" {
				continue
			}
			owner, known := owners[table]
			if !known {
				t.Errorf("%s references table %q with no module owner", source.file, table)
				continue
			}
			if owner != source.module {
				t.Errorf("%s references table %q owned by module %q", source.file, table, owner)
			}
		}
	}
}

func TestHostOnlyTouchesFeatureTablesDuringExplicitLegacyMigrations(t *testing.T) {
	moduleRoot := filepath.Join("..", "..", "modules")
	owners := make(map[string]string)
	err := filepath.WalkDir(moduleRoot, func(file string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(file, ".go") || strings.HasSuffix(file, "_test.go") {
			return nil
		}
		relative, err := filepath.Rel(moduleRoot, file)
		if err != nil {
			return err
		}
		module := strings.Split(filepath.ToSlash(relative), "/")[0]
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			text, err := strconv.Unquote(literal.Value)
			if err == nil {
				for _, match := range sqlCreateTable.FindAllStringSubmatch(text, -1) {
					owners[strings.ToLower(match[1])] = module
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// These are one-time migrations from schemas owned by the pre-module
	// application. Keep exceptions narrow to the exact bridge and source file.
	legacyMigrationTables := map[string]map[string]map[string]bool{
		"dhcp.go": {
			"initDHCPSchema": {"dhcp_reservations": true},
		},
		"database.go": {
			"importLegacyDNSProviders": {"dns_providers": true},
		},
		"webserver.go": {
			"migrateLegacyWebCertificateReferences": {"web_listeners": true, "web_hosts": true},
			"migrateLegacyWebWorkerScripts":         {"workers": true},
		},
	}
	hostRoot := filepath.Join("..", "host")
	err = filepath.WalkDir(hostRoot, func(file string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(file, ".go") || strings.HasSuffix(file, "_test.go") {
			return nil
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			return err
		}
		base := filepath.Base(file)
		ast.Inspect(parsed, func(node ast.Node) bool {
			function, ok := node.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				return true
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				literal, ok := node.(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					return true
				}
				sqlText, err := strconv.Unquote(literal.Value)
				if err != nil || !sqlStatementStart.MatchString(sqlText) {
					return true
				}
				for _, table := range referencedTables(sqlText) {
					if strings.HasSuffix(table, "_new") {
						table = strings.TrimSuffix(table, "_new")
					}
					if _, owned := owners[table]; !owned || legacyMigrationTables[base][function.Name.Name][table] {
						continue
					}
					t.Errorf("%s.%s directly references feature table %q; use its module repository (or a narrowly documented legacy migration)", file, function.Name.Name, table)
				}
				return true
			})
			return false
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDynamicSQLTableIdentifiersAreDetected(t *testing.T) {
	for _, test := range []struct {
		name string
		expr ast.Expr
		want bool
	}{
		{
			name: "concatenated table name",
			expr: &ast.BinaryExpr{X: stringLiteral(`SELECT * FROM `), Op: token.ADD, Y: ast.NewIdent("tableName")},
			want: true,
		},
		{
			name: "formatted update table",
			expr: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("fmt"), Sel: ast.NewIdent("Sprintf")}, Args: []ast.Expr{stringLiteral(`UPDATE %s SET value=?`), ast.NewIdent("tableName")}},
			want: true,
		},
		{
			name: "dynamic columns with fixed table",
			expr: &ast.BinaryExpr{X: stringLiteral(`SELECT `), Op: token.ADD, Y: ast.NewIdent("columns")},
			want: false,
		},
		{
			name: "fixed alter table with dynamic column",
			expr: &ast.BinaryExpr{X: stringLiteral(`ALTER TABLE owned_table ADD COLUMN `), Op: token.ADD, Y: ast.NewIdent("columnName")},
			want: false,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := dynamicSQLTableIdentifier.MatchString(sqlExpressionText(test.expr))
			if got != test.want {
				t.Fatalf("dynamic SQL check = %v, want %v for %q", got, test.want, sqlExpressionText(test.expr))
			}
		})
	}
}

func TestModulesDoNotImportOtherModulesOrHostPackages(t *testing.T) {
	const importRoot = "github.com/lsongdev/ohmyserver/"
	const moduleRoot = importRoot + "modules/"
	hostPackages := map[string]bool{
		"config": true, "frontend": true, "internal": true, "server": true, "web": true,
	}
	root := filepath.Join("..", "..", "modules")
	err := filepath.WalkDir(root, func(file string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(file, ".go") {
			return nil
		}
		relative, err := filepath.Rel(root, file)
		if err != nil {
			return err
		}
		module := strings.Split(filepath.ToSlash(relative), "/")[0]
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range parsed.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			if strings.HasPrefix(importPath, moduleRoot) {
				target := strings.Split(strings.TrimPrefix(importPath, moduleRoot), "/")[0]
				if target != module {
					t.Errorf("%s imports module %q; modules must integrate through host-injected capabilities", file, target)
				}
				continue
			}
			if strings.HasPrefix(importPath, importRoot) {
				target := strings.Split(strings.TrimPrefix(importPath, importRoot), "/")[0]
				if hostPackages[target] {
					t.Errorf("%s imports host package %q; modules must not depend on the application composition root", file, target)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func stringLiteral(value string) ast.Expr {
	return &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(value)}
}

func TestSQLTableReferenceDetectionCoversMigrationAndPragmaForms(t *testing.T) {
	sql := `SELECT * FROM alpha JOIN beta ON beta.id=alpha.id;
UPDATE gamma SET value=1;
ALTER TABLE delta ADD COLUMN value TEXT;
DROP TABLE epsilon;
PRAGMA table_info(zeta);
PRAGMA foreign_key_list('eta');`
	want := map[string]bool{"alpha": false, "beta": false, "gamma": false, "delta": false, "epsilon": false, "zeta": false, "eta": false}
	for _, table := range referencedTables(sql) {
		if _, exists := want[table]; !exists {
			t.Errorf("unexpected table reference %q", table)
		}
		want[table] = true
	}
	for table, found := range want {
		if !found {
			t.Errorf("table reference %q was not detected", table)
		}
	}
}

func TestFeatureModulesDoNotImportCompositionRootOrOtherFeatureModules(t *testing.T) {
	const moduleImportPrefix = "github.com/lsongdev/ohmyserver/modules/"
	const applicationPackage = "github.com/lsongdev/ohmyserver/internal/host"
	const compositionPackage = "github.com/lsongdev/ohmyserver/internal/app"
	root := filepath.Join("..", "..", "modules")
	err := filepath.WalkDir(root, func(file string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(file, ".go") || strings.HasSuffix(file, "_test.go") {
			return nil
		}
		relative, err := filepath.Rel(root, file)
		if err != nil {
			return err
		}
		owner := strings.Split(filepath.ToSlash(relative), "/")[0]
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imported := range parsed.Imports {
			importPath, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				return err
			}
			if importPath == applicationPackage || importPath == compositionPackage {
				t.Errorf("%s imports the application composition package %q; feature modules must stay reusable outside the panel", file, importPath)
				continue
			}
			if !strings.HasPrefix(importPath, moduleImportPrefix) {
				continue
			}
			dependency := strings.Split(strings.TrimPrefix(importPath, moduleImportPrefix), "/")[0]
			if dependency != owner {
				t.Errorf("%s imports feature module %q; cross-module dependencies belong in the application composition layer", file, dependency)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestFeatureModulesDoNotCreateUnscopedContexts(t *testing.T) {
	root := filepath.Join("..", "..", "modules")
	err := filepath.WalkDir(root, func(file string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(file, ".go") || strings.HasSuffix(file, "_test.go") {
			return nil
		}
		// CLI entrypoints own the process lifetime and must create the root
		// signal context. Feature services themselves must propagate requests.
		if strings.Contains(filepath.ToSlash(file), "/cmd/") {
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
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "Background" && selector.Sel.Name != "TODO" {
				return true
			}
			packageName, ok := selector.X.(*ast.Ident)
			if ok && packageName.Name == "context" {
				t.Errorf("%s creates an unscoped context; propagate the caller context or let the composition layer own background work", file)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestFeatureServiceInterfacesRequireRequestContext(t *testing.T) {
	root := filepath.Join("..", "..", "modules")
	err := filepath.WalkDir(root, func(file string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(file, ".go") || strings.HasSuffix(file, "_test.go") {
			return nil
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			spec, ok := node.(*ast.TypeSpec)
			if !ok || spec.Name.Name != "Service" {
				return true
			}
			iface, ok := spec.Type.(*ast.InterfaceType)
			if !ok {
				return true
			}
			for _, method := range iface.Methods.List {
				fn, ok := method.Type.(*ast.FuncType)
				if !ok {
					t.Errorf("%s Service embeds a non-method field; keep capabilities explicit", file)
					continue
				}
				if fn.Params == nil || len(fn.Params.List) == 0 {
					t.Errorf("%s Service method %s must accept context.Context", file, methodName(method))
					continue
				}
				selector, ok := fn.Params.List[0].Type.(*ast.SelectorExpr)
				if !ok {
					t.Errorf("%s Service method %s must take context.Context as its first argument", file, methodName(method))
					continue
				}
				contextPackage, isIdent := selector.X.(*ast.Ident)
				if !isIdent || contextPackage.Name != "context" || selector.Sel.Name != "Context" {
					t.Errorf("%s Service method %s must take context.Context as its first argument", file, methodName(method))
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func methodName(field *ast.Field) string {
	if len(field.Names) == 0 {
		return "<embedded>"
	}
	return field.Names[0].Name
}
