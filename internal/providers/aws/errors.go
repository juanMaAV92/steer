package aws

import (
	"errors"
	"strings"

	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

// friendlyRule mapea una firma de error AWS a un mensaje que enseña el remedio.
type friendlyRule struct {
	match func(err error, lower string) bool
	msg   string
}

// Detección mixta: tipos del SDK cuando existen (errors.As) y substrings
// documentados cuando el SDK solo da texto (mismo criterio que IsProvisioningFailure).
var friendlyRules = []friendlyRule{
	{func(_ error, l string) bool {
		return strings.Contains(l, "sso session has expired") || strings.Contains(l, "sso session is invalid") ||
			strings.Contains(l, "token has expired")
	}, "AWS session expired — try: aws sso login --profile <your-profile>"},
	{func(_ error, l string) bool { return strings.Contains(l, "failed to get shared config profile") },
		"AWS profile not found — try: aws configure --profile <name> (or check ~/.aws/config)"},
	{func(_ error, l string) bool { return strings.Contains(l, "failed to retrieve credentials") },
		"no AWS credentials found — try: aws configure, or aws sso login if your team uses SSO"},
	{func(_ error, l string) bool {
		return strings.Contains(l, "accessdenied") || strings.Contains(l, "not authorized to perform")
	}, "access denied — try: ask whoever manages AWS to grant your role ECS/ECR read permissions"},
	{func(err error, _ string) bool {
		var cnf *ecstypes.ClusterNotFoundException
		return errors.As(err, &cnf)
	}, "cluster not found in this account/region — try: check the cluster name in steer.toml and the profile's region"},
	{func(_ error, l string) bool {
		return strings.Contains(l, "i/o timeout") || strings.Contains(l, "no such host") ||
			strings.Contains(l, "connection refused")
	}, "could not reach AWS — try: check your network/VPN and retry"},
}

// FriendlyError traduce errores comunes de AWS a mensajes accionables.
// ok=false si no hay mapeo (el llamador muestra el error tal cual).
func FriendlyError(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	lower := strings.ToLower(err.Error())
	for _, r := range friendlyRules {
		if r.match(err, lower) {
			orig := err.Error()
			if len(orig) > 120 {
				orig = orig[:117] + "..."
			}
			return r.msg + "\n  (" + orig + ")", true
		}
	}
	return "", false
}
