package deployer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeployUsesKubectlApply(t *testing.T) {
	var gotCommand []string
	var gotInput string

	deployer := New(func(_ context.Context, name string, args []string, stdin string) error {
		gotCommand = append([]string{name}, args...)
		gotInput = stdin
		return nil
	})

	err := deployer.Apply(context.Background(), []byte("kind: ConfigMap\nmetadata:\n  name: test"))
	require.NoError(t, err)
	require.Equal(t, []string{"kubectl", "apply", "-f", "-"}, gotCommand)
	require.Contains(t, gotInput, "kind: ConfigMap")
}
