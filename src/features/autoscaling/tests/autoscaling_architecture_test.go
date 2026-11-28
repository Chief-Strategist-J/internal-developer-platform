/*
ALGORITHM & ARCHITECTURE BLUEPRINT: AUTOSCALING ARCHITECTURE TEST (V.3 role 14, Part XII.3)

1. Purpose:
   - Mandatory architecture tier: G-02 no cross-feature imports, G-07 no env access, G-11 no concurrency
     or timers in service/ and domain/, F-04 closed file set, C-01 zero inline comments, function
     size limit (30 lines), context.yml declarations match the code.
2. Invariants:
   - Walks the feature tree with go/parser; zero inline comments below this header.
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

const featureImportPrefix = "internal-developer-platform/src/features/"
const ownFeatureImport = featureImportPrefix + "autoscaling"
const maxFunctionLines = 30

func listFeatureGoFiles(t *testing.T) []string {
	t.Helper()
	var files []string
	err := filepath.Walk("..", func(path string, info os.FileInfo, walkErr error) error {
		if walkErr == nil && !info.IsDir() && strings.HasSuffix(path, ".go") {
			files = append(files, path)
		}
		return walkErr
	})
	if err != nil {
		t.Fatalf("walk feature: %v", err)
	}
	return files
}

func parseFile(t *testing.T, path string, mode parser.Mode) (*token.FileSet, *ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, mode)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return fset, file
}

func TestAutoscalingArchitecture_NoCrossFeatureImports(t *testing.T) {
	for _, path := range listFeatureGoFiles(t) {
		_, file := parseFile(t, path, parser.ImportsOnly)
		for _, spec := range file.Imports {
			imported := strings.Trim(spec.Path.Value, `"`)
			if strings.HasPrefix(imported, featureImportPrefix) && !strings.HasPrefix(imported, ownFeatureImport) {
				t.Errorf("G-02: %s imports another feature: %s", path, imported)
			}
		}
	}
}

func TestAutoscalingArchitecture_NoEnvAccessOrConcurrencyInCore(t *testing.T) {
	for _, path := range listFeatureGoFiles(t) {
		slashed := filepath.ToSlash(path)
		core := strings.Contains(slashed, "/service/") || strings.Contains(slashed, "/domain/") || strings.Contains(slashed, "/contracts/")
		_, file := parseFile(t, path, 0)
		ast.Inspect(file, func(n ast.Node) bool {
			if _, isGo := n.(*ast.GoStmt); isGo && core {
				t.Errorf("G-11: go statement in %s", path)
			}
			if sel, ok := n.(*ast.SelectorExpr); ok {
				checkForbiddenSelector(t, path, sel, core)
			}
			return true
		})
	}
}

func checkForbiddenSelector(t *testing.T, path string, sel *ast.SelectorExpr, core bool) {
	t.Helper()
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return
	}
	if pkg.Name == "os" && (sel.Sel.Name == "Getenv" || sel.Sel.Name == "LookupEnv") {
		t.Errorf("G-07: env access in %s", path)
	}
	if core && pkg.Name == "time" && (sel.Sel.Name == "NewTicker" || sel.Sel.Name == "AfterFunc" || sel.Sel.Name == "Now") {
		t.Errorf("G-11/V.2: time.%s in %s", sel.Sel.Name, path)
	}
}

func TestAutoscalingArchitecture_NoInlineComments(t *testing.T) {
	for _, path := range listFeatureGoFiles(t) {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		fset, file := parseFile(t, path, parser.ParseComments)
		for _, group := range file.Comments {
			if fset.Position(group.Pos()).Line > fset.Position(file.Package).Line {
				t.Errorf("C-01: inline comment in %s at line %d", path, fset.Position(group.Pos()).Line)
			}
		}
	}
}

func TestAutoscalingArchitecture_FunctionSizeLimit(t *testing.T) {
	for _, path := range listFeatureGoFiles(t) {
		fset, file := parseFile(t, path, 0)
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			lines := fset.Position(fn.End()).Line - fset.Position(fn.Pos()).Line + 1
			if lines > maxFunctionLines {
				t.Errorf("function %s in %s has %d lines (max %d)", fn.Name.Name, path, lines, maxFunctionLines)
			}
		}
	}
}

func TestAutoscalingArchitecture_ClosedFileSet(t *testing.T) {
	allowed := map[string]bool{
		"index.go": true, "autoscaling.module.go": true, "context.yml": true,
		"autoscaling.rest.handler.go": true, "autoscaling.router.go": true,
		"autoscaling.types.go": true, "autoscaling.schema.go": true,
		"autoscaling.rules.go": true, "autoscaling.serverless.rules.go": true, "autoscaling.service.go": true,
	}
	err := filepath.Walk("..", func(path string, info os.FileInfo, walkErr error) error {
		isTest := strings.HasSuffix(path, "_test.go")
		if walkErr == nil && !info.IsDir() && !isTest && !allowed[info.Name()] {
			t.Errorf("F-04: file outside the closed set: %s", path)
		}
		return walkErr
	})
	if err != nil {
		t.Fatalf("walk feature: %v", err)
	}
}

func TestAutoscalingArchitecture_ContextDeclaresIngressAndPorts(t *testing.T) {
	raw, err := os.ReadFile("../context.yml")
	if err != nil {
		t.Fatalf("read context.yml: %v", err)
	}
	for _, want := range []string{"feature: autoscaling", "type: backend", "capability: authz", "rest:", "AutoscalingServicePort"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("S-01: context.yml missing %q", want)
		}
	}
}
