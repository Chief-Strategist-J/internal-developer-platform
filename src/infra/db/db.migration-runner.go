/*
ALGORITHM & ARCHITECTURE BLUEPRINT: FEATURE MIGRATION DISCOVERY & LOCK VERIFICATION (D-02, II.3)

 1. Role:
    - Discovers per-feature SQL migrations across src/features/<feature>/database/migrations/,
    sorts chronologically by timestamp prefix, and validates schema.lock checksums.
 2. Invariants:
    - Zero inline comments inside function bodies (C-01).
*/
package db

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type FeatureMigration struct {
	Feature  string
	Filename string
	Path     string
	SHA256   string
}

type SchemaLockFile struct {
	Version       string            `json:"version"`
	LastMigration string            `json:"last_migration"`
	Checksums     map[string]string `json:"checksums"`
}

func DiscoverUpMigrations(repoRoot string) ([]FeatureMigration, error) {
	featuresRoot := filepath.Join(repoRoot, "src", "features")
	entries, err := os.ReadDir(featuresRoot)
	if err != nil {
		return nil, err
	}
	var out []FeatureMigration
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		migDir := filepath.Join(featuresRoot, entry.Name(), "database", "migrations")
		files, readErr := os.ReadDir(migDir)
		if readErr != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".up.sql") {
				continue
			}
			fullPath := filepath.Join(migDir, f.Name())
			raw, err := os.ReadFile(fullPath)
			if err != nil {
				return nil, err
			}
			sum := sha256.Sum256(raw)
			out = append(out, FeatureMigration{
				Feature:  entry.Name(),
				Filename: f.Name(),
				Path:     fullPath,
				SHA256:   hex.EncodeToString(sum[:]),
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Filename == out[j].Filename {
			return out[i].Feature < out[j].Feature
		}
		return out[i].Filename < out[j].Filename
	})
	return out, nil
}

func VerifySchemaLock(repoRoot string) error {
	lockPath := filepath.Join(repoRoot, "database", "schema.lock")
	rawLock, err := os.ReadFile(lockPath)
	if err != nil {
		return err
	}
	var lock SchemaLockFile
	if err := json.Unmarshal(rawLock, &lock); err != nil {
		return err
	}
	migs, err := DiscoverUpMigrations(repoRoot)
	if err != nil {
		return err
	}
	for _, m := range migs {
		key := fmt.Sprintf("%s/%s", m.Feature, m.Filename)
		recorded, ok := lock.Checksums[key]
		if !ok {
			return fmt.Errorf("migration %s missing from database/schema.lock", key)
		}
		if recorded != m.SHA256 {
			return fmt.Errorf("checksum mismatch for %s", key)
		}
	}
	return nil
}
