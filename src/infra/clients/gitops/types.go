package gitops

import (
	"time"
)

const (
	DefaultGitopsSigningSecret = "idp-platform-default-gitops-signing-secret"
)

type ManifestFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type CommitRequest struct {
	Author    string         `json:"author"`
	Email     string         `json:"email"`
	Message   string         `json:"message"`
	Branch    string         `json:"branch"`
	Files     []ManifestFile `json:"files"`
	SignKeyID string         `json:"signKeyId,omitempty"`
}

type CommitResult struct {
	CommitSHA   string    `json:"commitSha"`
	TreeSHA     string    `json:"treeSha"`
	Branch      string    `json:"branch"`
	Signature   string    `json:"signature"`
	CommittedAt time.Time `json:"committedAt"`
}
