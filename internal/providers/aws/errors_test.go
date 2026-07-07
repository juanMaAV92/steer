package aws

import (
	"errors"
	"testing"

	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/stretchr/testify/require"
)

func TestFriendlyErrorMappings(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string // substring del mensaje amable
	}{
		{"sso expirado", errors.New("failed to refresh cached credentials, the SSO session has expired or is invalid"),
			"AWS session expired — try: aws sso login --profile <your-profile>"},
		{"sin perfil", errors.New(`failed to get shared config profile, devx`),
			"try: aws configure --profile"},
		{"sin credenciales", errors.New("failed to retrieve credentials, no EC2 IMDS role found"),
			"no AWS credentials found"},
		{"access denied", errors.New("api error AccessDeniedException: User is not authorized to perform: ecs:ListServices"),
			"access denied"},
		{"cluster no existe", &ecstypes.ClusterNotFoundException{},
			"cluster not found in this account/region"},
		{"timeout", errors.New("dial tcp: lookup ecs.us-east-1.amazonaws.com: i/o timeout"),
			"could not reach AWS"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg, ok := FriendlyError(tc.err)
			require.True(t, ok, "debe mapear: %v", tc.err)
			require.Contains(t, msg, tc.want)
			require.Contains(t, msg, "— try:", "formato qué-pasó — try: remedio")
		})
	}
}

func TestFriendlyErrorFallback(t *testing.T) {
	_, ok := FriendlyError(errors.New("algo totalmente distinto"))
	require.False(t, ok)
}

func TestFriendlyErrorTruncatesOriginal(t *testing.T) {
	long := errors.New("failed to refresh cached credentials, the SSO session has expired " +
		string(make([]byte, 300)))
	msg, ok := FriendlyError(long)
	require.True(t, ok)
	require.LessOrEqual(t, len(msg), 400, "el original va truncado a ~120 chars")
}
