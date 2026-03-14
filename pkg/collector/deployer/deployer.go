package deployer

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

type Runner func(ctx context.Context, name string, args []string, stdin string) error

type Deployer struct {
	runner Runner
}

func New(runner Runner) *Deployer {
	if runner == nil {
		runner = defaultRunner
	}

	return &Deployer{runner: runner}
}

func (d *Deployer) Apply(ctx context.Context, manifests []byte) error {
	if len(manifests) == 0 {
		return fmt.Errorf("manifests are required")
	}

	return d.runner(ctx, "kubectl", []string{"apply", "-f", "-"}, string(manifests))
}

func defaultRunner(ctx context.Context, name string, args []string, stdin string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = bytes.NewBufferString(stdin)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v failed: %w: %s", name, args, err, string(output))
	}
	return nil
}
