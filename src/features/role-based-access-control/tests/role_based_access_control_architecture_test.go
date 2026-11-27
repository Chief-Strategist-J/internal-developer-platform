/*
ALGORITHM & ARCHITECTURE BLUEPRINT: PACKAGE ARCHITECTURE ENFORCEMENT TESTS (Part XII)

 1. Scope:
    - G-01/L-01: Layer 0 shared/ and Layer 1 infra/ never import upward; main/app import feature root packages only.
    - G-02: No feature under src/features/ imports any other feature.
    - G-05: src/shared/ never imports IO/transport packages (database/sql, net/http, redis).
    - G-07: Non-test Go code outside src/infra/config never calls os.Getenv.
    - G-11: Feature service/ and domain/ packages never contain go statements or time.NewTicker.
    - X-05/GO-INT-001: Active feature internals sit under internal/ and never export Service().
    - D-01/D-02/II.3: Per-feature SQL migrations with timeouts, schema isolation, zero flat database/migrations/, and src/infra/db runtime.
*/
package tests

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not locate go.mod")
		}
		dir = parent
	}
}

func TestRBACArchitecture_NoCrossFeatureImports(t *testing.T) {
	root := findRepoRoot(t)
	featuresRoot := filepath.Join(root, "src", "features")
	fset := token.NewFileSet()

	err := filepath.Walk(featuresRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return walkErr
		}
		rel, err := filepath.Rel(featuresRoot, path)
		if err != nil {
			return err
		}
		ownerFeature := strings.Split(filepath.ToSlash(rel), "/")[0]
		file, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return parseErr
		}
		for _, imp := range file.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			const marker = "src/features/"
			idx := strings.Index(importPath, marker)
			if idx == -1 {
				continue
			}
			targetFeature := strings.Split(importPath[idx+len(marker):], "/")[0]
			if targetFeature != ownerFeature {
				t.Errorf("G-02 violation in %s: feature %s MUST NOT import feature %s (%s)", path, ownerFeature, targetFeature, importPath)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk features: %v", err)
	}
}

func TestInfraMiddlewareArchitecture_ZeroFeatureImports(t *testing.T) {
	root := findRepoRoot(t)
	layerChecks := []struct {
		dir       string
		forbidden []string
	}{
		{dir: filepath.Join(root, "src", "shared"), forbidden: []string{"src/infra/", "src/app/", "src/features/", "database/sql", "net/http", "github.com/redis"}},
		{dir: filepath.Join(root, "src", "infra"), forbidden: []string{"src/app/", "src/features/"}},
	}
	fset := token.NewFileSet()

	for _, chk := range layerChecks {
		err := filepath.Walk(chk.dir, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
				return walkErr
			}
			file, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
			if parseErr != nil {
				return parseErr
			}
			for _, imp := range file.Imports {
				importPath := strings.Trim(imp.Path.Value, `"`)
				for _, forb := range chk.forbidden {
					if strings.Contains(importPath, forb) {
						t.Errorf("G-01/G-05 violation in %s: forbidden import %s", path, importPath)
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", chk.dir, err)
		}
	}
}

func TestAppAndMainArchitecture_RootIndexImportsOnly(t *testing.T) {
	root := findRepoRoot(t)
	fset := token.NewFileSet()
	targets := []string{
		filepath.Join(root, "src", "main.go"),
		filepath.Join(root, "src", "app"),
	}

	for _, target := range targets {
		err := filepath.Walk(target, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
				return walkErr
			}
			file, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
			if parseErr != nil {
				return parseErr
			}
			for _, imp := range file.Imports {
				importPath := strings.Trim(imp.Path.Value, `"`)
				const marker = "src/features/"
				idx := strings.Index(importPath, marker)
				if idx == -1 {
					continue
				}
				rest := importPath[idx+len(marker):]
				if strings.Contains(rest, "/") {
					t.Errorf("L-01/X-05/GO-INT-001 violation in %s: must import feature root only, got %s", path, importPath)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", target, err)
		}
	}
}

func TestDatabasePhase5Architecture_PerFeatureMigrationsAndIsolation(t *testing.T) {
	root := findRepoRoot(t)
	if _, err := os.Stat(filepath.Join(root, "database", "migrations")); err == nil {
		t.Fatal("D-01-001 violation: flat database/migrations directory must not exist in Phase 5")
	}
	if _, err := os.Stat(filepath.Join(root, "database", "engine")); err == nil {
		t.Fatal("II.3 violation: database/engine must not exist; moved to src/infra/db")
	}
	if _, err := os.Stat(filepath.Join(root, "database", "roles.sql")); err != nil {
		t.Fatalf("D-01 violation: database/roles.sql must exist: %v", err)
	}
	persistentFeatures := []string{
		"disaster-recovery",
		"gitops-reconciliation",
		"kubernetes-workload-provisioning",
		"platform-regime",
		"policy-as-code",
		"role-based-access-control",
		"service-catalog",
	}
	for _, feat := range persistentFeatures {
		migDir := filepath.Join(root, "src", "features", feat, "database", "migrations")
		entries, err := os.ReadDir(migDir)
		if err != nil || len(entries) == 0 {
			t.Fatalf("D-01-001 violation: persistent feature %s missing database/migrations/", feat)
		}
		upCount := 0
		downCount := 0
		for _, e := range entries {
			name := e.Name()
			if strings.HasSuffix(name, ".up.sql") {
				upCount++
				raw, readErr := os.ReadFile(filepath.Join(migDir, name))
				if readErr != nil {
					t.Fatalf("read migration %s: %v", name, readErr)
				}
				sqlText := string(raw)
				if !strings.Contains(sqlText, "lock_timeout") || !strings.Contains(sqlText, "statement_timeout") {
					t.Errorf("D-02 violation in %s/%s: migration must set lock_timeout and statement_timeout", feat, name)
				}
			}
			if strings.HasSuffix(name, ".down.sql") {
				downCount++
			}
		}
		if upCount == 0 || upCount != downCount {
			t.Errorf("D-02 violation in %s: mismatched up (%d) and down (%d) migration files", feat, upCount, downCount)
		}
	}
}

func TestRBACServiceAndDomain_NoEnvOrTimersOrGoroutines(t *testing.T) {
	root := findRepoRoot(t)
	srcRoot := filepath.Join(root, "src")
	fset := token.NewFileSet()

	err := filepath.Walk(srcRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return walkErr
		}
		slashPath := filepath.ToSlash(path)
		if (filepath.Base(path) == "index.go" || strings.HasSuffix(path, ".module.go")) && strings.Contains(slashPath, "/src/features/") {
			raw, readErr := os.ReadFile(path)
			if readErr == nil {
				content := string(raw)
				if strings.Contains(content, "Service =") || strings.Contains(content, "Port =") || strings.Contains(content, ") Service() *service.") {
					t.Errorf("X-05 violation in %s: must export Module/bridge contracts only, not Service or Port", path)
				}
			}
		}
		if strings.Contains(slashPath, "/src/features/") {
			for _, seg := range []string{"/api/", "/contracts/", "/domain/", "/infrastructure/", "/service/"} {
				if idx := strings.Index(slashPath, seg); idx != -1 && !strings.Contains(slashPath[:idx], "/internal") {
					t.Errorf("GO-INT-001 violation in %s: feature internal directory %s must sit under internal/", path, seg)
				}
			}
		}
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		inConfig := strings.Contains(slashPath, "/src/infra/config/")
		inServiceOrDomain := strings.Contains(slashPath, "/service/") || strings.Contains(slashPath, "/domain/")
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.GoStmt:
				if inServiceOrDomain {
					t.Errorf("G-11 violation in %s: go statement forbidden in service/domain", path)
				}
			case *ast.SelectorExpr:
				if ident, ok := node.X.(*ast.Ident); ok {
					if !inConfig && ident.Name == "os" && node.Sel.Name == "Getenv" {
						t.Errorf("G-07 violation in %s: os.Getenv forbidden outside infra/config", path)
					}
					if inServiceOrDomain && ident.Name == "time" && node.Sel.Name == "NewTicker" {
						t.Errorf("G-11 violation in %s: time.NewTicker forbidden in service/domain", path)
					}
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walk src: %v", err)
	}
}
