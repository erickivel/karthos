package deployer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDeployUsesHelmLintTemplateAndUpgrade(t *testing.T) {
	var commands [][]string
	renderedPath := filepath.Join(t.TempDir(), "rendered")

	deployer := New(func(_ context.Context, name string, args []string) error {
		commands = append(commands, append([]string{name}, args...))
		return nil
	})

	err := deployer.Apply(context.Background(), []byte("compiled-values"), Options{
		ReleaseName:  "karthos",
		ChartPath:    "./deploy/helm/karthos",
		Namespace:    "karthos-system",
		Timeout:      30 * time.Second,
		DryRun:       true,
		RenderedPath: renderedPath,
	})
	require.NoError(t, err)
	require.Len(t, commands, 3)
	require.Equal(t, "helm", commands[0][0])
	require.Equal(t, []string{"helm", "lint", "./deploy/helm/karthos"}, commands[0][:3])
	require.Equal(t, []string{"helm", "template", "karthos", "./deploy/helm/karthos"}, commands[1][:4])
	require.Equal(t, []string{"helm", "upgrade", "--install", "karthos", "./deploy/helm/karthos"}, commands[2][:5])
	require.True(t, contains(commands[2], "--force-conflicts"))
	require.True(t, contains(commands[2], "--dry-run"))
}

func TestTemplateRequiresOutputDirectory(t *testing.T) {
	deployer := New(func(_ context.Context, name string, args []string) error {
		return nil
	})

	err := deployer.Template(context.Background(), []byte("compiled-values"), Options{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "rendered output path is required")
}

func TestTemplateClearsStaleRenderedArtifactsBeforeHelmWrites(t *testing.T) {
	renderedPath := filepath.Join(t.TempDir(), "rendered")
	require.NoError(t, os.MkdirAll(renderedPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(renderedPath, "stale.yaml"), []byte("stale"), 0o600))

	deployer := New(func(_ context.Context, _ string, args []string) error {
		if !contains(args, "template") {
			return nil
		}
		entries, err := os.ReadDir(renderedPath)
		require.NoError(t, err)
		require.Empty(t, entries, "Helm must receive an empty rendered-artifact directory")
		return nil
	})

	err := deployer.Template(context.Background(), []byte("global:\n  clusterName: kind-test\n"), Options{RenderedPath: renderedPath})
	require.NoError(t, err)
}

func TestTemplateRestrictsRenderedOutputDirectoryPermissions(t *testing.T) {
	renderedPath := filepath.Join(t.TempDir(), "rendered")
	deployer := New(func(_ context.Context, _ string, _ []string) error {
		return nil
	})

	require.NoError(t, deployer.Template(context.Background(), []byte("compiled-values"), Options{RenderedPath: renderedPath}))
	info, err := os.Stat(renderedPath)
	require.NoError(t, err)
	require.Zero(t, info.Mode().Perm()&0o027)
}

func TestWriteTempFileRemovesPartialFileWhenWriteOrCloseFails(t *testing.T) {
	testCases := []struct {
		name     string
		writeErr error
		closeErr error
	}{
		{name: "write failure", writeErr: errors.New("write failed")},
		{name: "close failure", closeErr: errors.New("close failed")},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "partial-values.yaml")
			require.NoError(t, os.WriteFile(path, []byte("partial"), 0o600))

			_, err := writeTempFileWith(func(_, _ string) (tempFile, error) {
				return failingTempFile{path: path, writeErr: testCase.writeErr, closeErr: testCase.closeErr}, nil
			}, "ignored", []byte("values"))
			require.Error(t, err)
			_, statErr := os.Stat(path)
			require.ErrorIs(t, statErr, os.ErrNotExist)
		})
	}
}

type failingTempFile struct {
	path     string
	writeErr error
	closeErr error
}

func (f failingTempFile) Name() string { return f.path }

func (f failingTempFile) Write(payload []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return len(payload), nil
}

func (f failingTempFile) Close() error { return f.closeErr }

func contains(values []string, needle string) bool {
	return strings.Contains(strings.Join(values, " "), needle)
}
