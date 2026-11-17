package gitops

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	"internal-developer-platform/src/shared/errors"
)

type GitManifestCommitter struct {
	signingSecret []byte
}

func NewGitManifestCommitter(signingSecret []byte) *GitManifestCommitter {
	if len(signingSecret) == 0 {
		signingSecret = []byte(DefaultGitopsSigningSecret)
	}
	return &GitManifestCommitter{
		signingSecret: signingSecret,
	}
}

func (c *GitManifestCommitter) Commit(ctx context.Context, req CommitRequest) (*CommitResult, error) {
	if req.Author == "" || req.Email == "" {
		return nil, errors.NewValidationError("author and email are required for git commit")
	}
	if req.Branch == "" {
		return nil, errors.NewValidationError("branch is required for git commit")
	}
	if req.Message == "" {
		return nil, errors.NewValidationError("commit message is required")
	}
	if len(req.Files) == 0 {
		return nil, errors.NewValidationError("at least one manifest file is required to commit")
	}

	sort.Slice(req.Files, func(i, j int) bool {
		return req.Files[i].Path < req.Files[j].Path
	})

	treeHasher := sha256.New()
	for _, file := range req.Files {
		if file.Path == "" || file.Content == "" {
			return nil, errors.NewValidationError("manifest file path and content cannot be empty")
		}
		treeHasher.Write([]byte(fmt.Sprintf("%s:%d:%s\n", file.Path, len(file.Content), file.Content)))
	}
	treeSHA := hex.EncodeToString(treeHasher.Sum(nil))

	committedAt := time.Now().UTC()
	commitData := fmt.Sprintf("tree %s\nauthor %s <%s> %d\ncommitter %s <%s> %d\n\n%s\n",
		treeSHA,
		req.Author, req.Email, committedAt.Unix(),
		req.Author, req.Email, committedAt.Unix(),
		req.Message,
	)

	commitHasher := sha256.New()
	commitHasher.Write([]byte(commitData))
	commitSHA := hex.EncodeToString(commitHasher.Sum(nil))

	sigHasher := hmac.New(sha256.New, c.signingSecret)
	sigHasher.Write([]byte(commitSHA + ":" + req.SignKeyID))
	signature := hex.EncodeToString(sigHasher.Sum(nil))

	return &CommitResult{
		CommitSHA:   commitSHA,
		TreeSHA:     treeSHA,
		Branch:      req.Branch,
		Signature:   signature,
		CommittedAt: committedAt,
	}, nil
}
