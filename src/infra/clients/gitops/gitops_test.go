package gitops

import (
	"context"
	"testing"
)

func TestGitManifestCommitterSuccess(t *testing.T) {
	committer := NewGitManifestCommitter([]byte("test-signing-key-32-bytes-long!"))
	ctx := context.Background()

	files := []ManifestFile{
		{
			Path:    "clusters/prod/workloads/tenant-alpha/deployment.yaml",
			Content: "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: app",
		},
		{
			Path:    "clusters/prod/workloads/tenant-alpha/service.yaml",
			Content: "apiVersion: v1\nkind: Service\nmetadata:\n  name: app",
		},
	}

	req := CommitRequest{
		Author:    "idp-agent",
		Email:     "agent@company.internal",
		Message:   "feat(workload): deploy app to tenant-alpha",
		Branch:    "main",
		Files:     files,
		SignKeyID: "key-sec-001",
	}

	res, err := committer.Commit(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error committing manifests: %v", err)
	}

	if len(res.CommitSHA) != 64 {
		t.Errorf("expected 64-char hex SHA256 commit hash, got %s", res.CommitSHA)
	}
	if len(res.TreeSHA) != 64 {
		t.Errorf("expected 64-char hex SHA256 tree hash, got %s", res.TreeSHA)
	}
	if len(res.Signature) != 64 {
		t.Errorf("expected 64-char HMAC signature, got %s", res.Signature)
	}
	if res.Branch != "main" {
		t.Errorf("expected branch main, got %s", res.Branch)
	}
}

func TestGitManifestCommitterValidation(t *testing.T) {
	committer := NewGitManifestCommitter(nil)
	ctx := context.Background()

	_, err := committer.Commit(ctx, CommitRequest{
		Author: "",
		Email:  "test@example.com",
		Branch: "main",
	})
	if err == nil {
		t.Fatalf("expected error for empty author")
	}

	_, err = committer.Commit(ctx, CommitRequest{
		Author: "author",
		Email:  "test@example.com",
		Branch: "main",
		Files:  nil,
	})
	if err == nil {
		t.Fatalf("expected error for empty files list")
	}
}

