// Package tests verifies v3.0 architectural boundary invariants for role-based-access-control (Part XII.3).
//
// Blueprint:
//  1. Enforces G-02: RBAC never imports any other feature package under src/features/.
//  2. Enforces G-01: src/infra/middleware never imports any feature package under src/features/.
//  3. Enforces G-07 & G-11: service/ and domain/ never invoke os.Getenv, go statements, or time.NewTicker.
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
	rbacRoot := filepath.Join(root, "src", "features", "role-based-access-control")
	fset := token.NewFileSet()

	err := filepath.Walk(rbacRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return walkErr
		}
		file, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return parseErr
		}
		for _, imp := range file.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			if strings.Contains(importPath, "src/features/") && !strings.Contains(importPath, "src/features/role-based-access-control") {
				t.Errorf("G-02 violation in %s: forbidden cross-feature import %s", path, importPath)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk rbac: %v", err)
	}
}

func TestInfraMiddlewareArchitecture_ZeroFeatureImports(t *testing.T) {
	root := findRepoRoot(t)
	mwRoot := filepath.Join(root, "src", "infra", "middleware")
	fset := token.NewFileSet()

	err := filepath.Walk(mwRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return walkErr
		}
		file, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return parseErr
		}
		for _, imp := range file.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			if strings.Contains(importPath, "src/features/") {
				t.Errorf("G-01 violation in %s: Layer 1 middleware MUST NOT import feature package %s", path, importPath)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk middleware: %v", err)
	}
}

func TestRBACServiceAndDomain_NoEnvOrTimersOrGoroutines(t *testing.T) {
	root := findRepoRoot(t)
	targets := []string{
		filepath.Join(root, "src", "features", "role-based-access-control", "service"),
		filepath.Join(root, "src", "features", "role-based-access-control", "domain"),
	}
	fset := token.NewFileSet()

	for _, dir := range targets {
		err := filepath.Walk(dir, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
				return walkErr
			}
			file, parseErr := parser.ParseFile(fset, path, nil, 0)
			if parseErr != nil {
				return parseErr
			}
			ast.Inspect(file, func(n ast.Node) bool {
				switch node := n.(type) {
				case *ast.GoStmt:
					t.Errorf("G-11 violation in %s: go statement forbidden in service/domain", path)
				case *ast.SelectorExpr:
					if ident, ok := node.X.(*ast.Ident); ok {
						if ident.Name == "os" && node.Sel.Name == "Getenv" {
							t.Errorf("G-07 violation in %s: os.Getenv forbidden outside infra/config", path)
						}
						if ident.Name == "time" && node.Sel.Name == "NewTicker" {
							t.Errorf("G-11 violation in %s: time.NewTicker forbidden in service/domain", path)
						}
					}
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}
}
