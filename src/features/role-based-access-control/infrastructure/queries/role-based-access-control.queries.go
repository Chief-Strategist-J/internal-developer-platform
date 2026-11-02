package queries

import "fmt"

const (
	KeyPrefixRevocationSub = "revocation:sub:"
	KeyPrefixRevocationJTI = "revocation:jti:"
	DefaultTTLSeconds      = 86400
	RevokedSentinelValue   = "revoked"
	ExistsPositiveResult   = ":1"
	OkResponsePrefix       = "+OK"
)

func BuildRevocationSubKey(subjectID string) string {
	return fmt.Sprintf("%s%s", KeyPrefixRevocationSub, subjectID)
}

func BuildRevocationJTIKey(jti string) string {
	return fmt.Sprintf("%s%s", KeyPrefixRevocationJTI, jti)
}

func BuildExistsCommand(key string) string {
	return fmt.Sprintf("EXISTS %s", key)
}

func BuildSetexCommand(key string, ttl int, value string) string {
	return fmt.Sprintf("SETEX %s %d %s", key, ttl, value)
}
