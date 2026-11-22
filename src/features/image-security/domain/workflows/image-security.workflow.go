package workflows

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"internal-developer-platform/src/features/image-security/contracts"
	"internal-developer-platform/src/features/image-security/domain/rules"
	"internal-developer-platform/src/infra/observability"
)

const (
	SpanImageSecurityScanDAG = "image_security.dag.scan"
	AttrImageDigest          = "image.digest"
	AttrAdmissionVerdict     = "image.admitted"
)

func ExecuteImageSecurityWorkflow(ctx context.Context, cmd contracts.ImageScanCommand, now time.Time) (contracts.ImageAdmissionVerdict, error) {
	return observability.RunTraced(ctx, SpanImageSecurityScanDAG, func(ctx context.Context) (contracts.ImageAdmissionVerdict, error) {
		verdict := rules.EvaluateImageSecurityPolicy(cmd, now)
		return verdict, nil
	}, attribute.String(AttrImageDigest, cmd.ImageDigest))
}
