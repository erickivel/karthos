package deployer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"
)

const (
	DefaultReleaseName = "oksg-otel"
	DefaultChartPath   = "./deploy/helm/oksg-otel"
	DefaultTimeout     = 5 * time.Minute
)

type Runner func(ctx context.Context, name string, args []string) error

type Options struct {
	ReleaseName  string
	ChartPath    string
	Namespace    string
	Timeout      time.Duration
	DryRun       bool
	RenderedPath string
}

type Deployer struct {
	runner Runner
}

func New(runner Runner) *Deployer {
	if runner == nil {
		runner = defaultRunner
	}

	return &Deployer{runner: runner}
}

func (d *Deployer) Apply(ctx context.Context, values []byte, options Options) (err error) {
	if len(values) == 0 {
		return fmt.Errorf("helm values are required")
	}

	options = withDefaults(options)

	valuesFile, err := writeTempFile("oksg-values-*.yaml", values)
	if err != nil {
		return err
	}
	defer func() {
		if removeErr := os.Remove(valuesFile); removeErr != nil {
			err = errors.Join(err, fmt.Errorf("remove temp values file: %w", removeErr))
		}
	}()

	if err := d.lint(ctx, valuesFile, options); err != nil {
		return err
	}

	if options.RenderedPath != "" {
		if err := d.template(ctx, valuesFile, options); err != nil {
			return err
		}
	}

	args := []string{
		"upgrade", "--install", options.ReleaseName, options.ChartPath,
		"--namespace", options.Namespace,
		"--create-namespace",
		"-f", valuesFile,
		"--timeout", options.Timeout.String(),
	}
	if options.DryRun {
		args = append(args, "--dry-run")
	}

	return d.runner(ctx, "helm", args)
}

func (d *Deployer) Template(ctx context.Context, values []byte, options Options) (err error) {
	if len(values) == 0 {
		return fmt.Errorf("helm values are required")
	}
	if options.RenderedPath == "" {
		return fmt.Errorf("rendered output path is required")
	}

	options = withDefaults(options)

	valuesFile, err := writeTempFile("oksg-values-*.yaml", values)
	if err != nil {
		return err
	}
	defer func() {
		if removeErr := os.Remove(valuesFile); removeErr != nil {
			err = errors.Join(err, fmt.Errorf("remove temp values file: %w", removeErr))
		}
	}()

	if err := d.lint(ctx, valuesFile, options); err != nil {
		return err
	}

	return d.template(ctx, valuesFile, options)
}

func withDefaults(options Options) Options {
	if options.ReleaseName == "" {
		options.ReleaseName = DefaultReleaseName
	}
	if options.ChartPath == "" {
		options.ChartPath = DefaultChartPath
	}
	if options.Timeout == 0 {
		options.Timeout = DefaultTimeout
	}
	return options
}

func writeTempFile(pattern string, payload []byte) (string, error) {
	file, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}

	if _, err := file.Write(payload); err != nil {
		if closeErr := file.Close(); closeErr != nil {
			return "", errors.Join(fmt.Errorf("write temp file: %w", err), fmt.Errorf("close temp file: %w", closeErr))
		}
		return "", fmt.Errorf("write temp file: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close temp file: %w", err)
	}
	return file.Name(), nil
}

func (d *Deployer) lint(ctx context.Context, valuesFile string, options Options) error {
	return d.runner(ctx, "helm", []string{"lint", options.ChartPath, "-f", valuesFile})
}

func (d *Deployer) template(ctx context.Context, valuesFile string, options Options) error {
	return d.runner(ctx, "helm", []string{
		"template", options.ReleaseName, options.ChartPath,
		"--namespace", options.Namespace,
		"-f", valuesFile,
		"--output-dir", options.RenderedPath,
	})
}

func defaultRunner(ctx context.Context, name string, args []string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v failed: %w: %s", name, args, err, string(output))
	}
	return nil
}
