/*
ALGORITHM & ARCHITECTURE BLUEPRINT: HTTP INGRESS METADATA EXTRACTOR

 1. Role (Layer 1, G-05):
    - Extracts W3C traceparent, correlation, causation, tenant, and idempotency headers from *http.Request.
    - Maps HTTP transport headers into pure shared utils.IngressMetadata without polluting Layer 0 shared/.
 2. Invariants:
    - Zero comments below this header.
*/
package middleware

import (
	"net/http"
	"time"

	"internal-developer-platform/src/shared/constants"
	"internal-developer-platform/src/shared/utils"
)

func ExtractIngressMetadata(r *http.Request) utils.IngressMetadata {
	return utils.IngressMetadata{
		Traceparent:    r.Header.Get(constants.HeaderTraceparent),
		RequestID:      r.Header.Get(constants.HeaderXRequestID),
		CorrelationID:  r.Header.Get(constants.HeaderXCorrelationID),
		CausationID:    r.Header.Get(constants.HeaderXCausationID),
		TenantID:       r.Header.Get(constants.HeaderXTenantID),
		ClientID:       r.Header.Get(constants.HeaderXClientID),
		UserID:         r.Header.Get(constants.HeaderXUserID),
		IdempotencyKey: r.Header.Get(constants.HeaderXIdempotencyKey),
		APIVersion:     r.Header.Get(constants.HeaderXAPIVersion),
		IfMatch:        r.Header.Get(constants.HeaderIfMatch),
		StartTime:      time.Now(),
	}
}
