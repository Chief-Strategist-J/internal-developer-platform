/*
ALGORITHM & ARCHITECTURE BLUEPRINT: GITOPS WORKLOAD DELIVERY ADAPTER

1. GitOps Delivery Flow:
   - Implements GitOpsDeliveryPort backed by infra gitops.GitManifestCommitter.
   - Enforces Single Writer Invariant: manifests are committed to Git rather than mutating Kubernetes directly.
   - Flattens ManifestBundle into structured YAML files under tenants/<tenant>/workloads/<workload>/.
   - Calculates tree SHA, author credentials, and HMAC commit signature.

2. Invariants:
   - Zero inline comments inside method bodies.
   - All methods strictly maintain length <= 25 lines.
*/
package adapters

import (
	"context"
	"fmt"

	"internal-developer-platform/src/features/kubernetes-workload-provisioning/contracts"
	"internal-developer-platform/src/infra/clients/gitops"
)

type GitOpsDeliveryAdapter struct {
	committer *gitops.GitManifestCommitter
	branch    string
	author    string
	email     string
}

func NewGitOpsDeliveryAdapter(committer *gitops.GitManifestCommitter, branch, author, email string) *GitOpsDeliveryAdapter {
	if branch == "" {
		branch = "main"
	}
	if author == "" {
		author = "idp-platform-bot"
	}
	if email == "" {
		email = "platform@internal.company.com"
	}
	return &GitOpsDeliveryAdapter{
		committer: committer,
		branch:    branch,
		author:    author,
		email:     email,
	}
}

func (a *GitOpsDeliveryAdapter) CommitManifestBundle(ctx context.Context, tenantID, workloadName string, bundle contracts.ManifestBundle) error {
	files := buildManifestFiles(tenantID, workloadName, bundle)
	req := gitops.CommitRequest{
		Author:    a.author,
		Email:     a.email,
		Branch:    a.branch,
		Message:   fmt.Sprintf("chore(workload): deploy %s for tenant %s", workloadName, tenantID),
		Files:     files,
		SignKeyID: "idp-platform-key-1",
	}
	_, err := a.committer.Commit(ctx, req)
	return err
}

func buildManifestFiles(tenantID, workloadName string, bundle contracts.ManifestBundle) []gitops.ManifestFile {
	basePath := fmt.Sprintf("tenants/%s/workloads/%s", tenantID, workloadName)
	files := make([]gitops.ManifestFile, 0, 8)
	files = appendIfPresent(files, fmt.Sprintf("%s/namespace.yaml", basePath), bundle.NamespaceYAML)
	files = appendIfPresent(files, fmt.Sprintf("%s/quota.yaml", basePath), bundle.ResourceQuotaYAML)
	files = appendIfPresent(files, fmt.Sprintf("%s/limitrange.yaml", basePath), bundle.LimitRangeYAML)
	files = appendIfPresent(files, fmt.Sprintf("%s/networkpolicy.yaml", basePath), bundle.NetworkPolicyYAML)
	files = appendIfPresent(files, fmt.Sprintf("%s/deployment.yaml", basePath), bundle.DeploymentYAML)
	files = appendIfPresent(files, fmt.Sprintf("%s/pdb.yaml", basePath), bundle.PDBYAML)
	files = appendIfPresent(files, fmt.Sprintf("%s/argocd-app.yaml", basePath), bundle.ArgoCDApplicationYAML)
	files = appendIfPresent(files, fmt.Sprintf("%s/rollout.yaml", basePath), bundle.RolloutYAML)
	return files
}

func appendIfPresent(files []gitops.ManifestFile, path, content string) []gitops.ManifestFile {
	if content != "" {
		files = append(files, gitops.ManifestFile{Path: path, Content: content})
	}
	return files
}

