/*
ALGORITHM & ARCHITECTURE BLUEPRINT: INFRA DB ISOLATION & SCHEMA LOCK TESTS
*/
package db

import (
	"os"
	"path/filepath"
	"testing"
)

func findRoot(t *testing.T) string {
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
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func TestBuildSessionPreludeStatements_RejectsInvalidIdentifiers(t *testing.T) {
	if _, err := BuildSessionPreludeStatements(FeatureTxScope{Schema: "bad-schema;", Role: "svc_rbac"}); err == nil {
		t.Fatal("expected error for invalid schema identifier")
	}
	stmts, err := BuildSessionPreludeStatements(FeatureTxScope{
		Schema:   "role_based_access_control",
		Role:     "svc_rbac",
		TenantID: "tenant-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stmts) != 4 {
		t.Fatalf("expected 4 prelude statements, got %d", len(stmts))
	}
}

func TestVerifySchemaLock_MatchesAllFeatureMigrations(t *testing.T) {
	root := findRoot(t)
	if err := VerifySchemaLock(root); err != nil {
		t.Fatalf("schema.lock verification failed: %v", err)
	}
}
