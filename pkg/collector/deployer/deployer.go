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

type tempFile interface {
	Name() string
	Write([]byte) (int, error)
	Close() error
}

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
		"--force-conflicts",
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
	return writeTempFileWith(func(dir, pattern string) (tempFile, error) {
		return os.CreateTemp(dir, pattern)
	}, pattern, payload)
}

func writeTempFileWith(create func(dir, pattern string) (tempFile, error), pattern string, payload []byte) (string, error) {
	file, err := create("", pattern)
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}

	if _, err := file.Write(payload); err != nil {
		path := file.Name()
		if closeErr := file.Close(); closeErr != nil {
			return "", errors.Join(fmt.Errorf("write temp file: %w", err), fmt.Errorf("close temp file: %w", closeErr), removeTempFile(path))
		}
		return "", errors.Join(fmt.Errorf("write temp file: %w", err), removeTempFile(path))
	}
	if err := file.Close(); err != nil {
		return "", errors.Join(fmt.Errorf("close temp file: %w", err), removeTempFile(file.Name()))
	}
	return file.Name(), nil
}

func (d *Deployer) lint(ctx context.Context, valuesFile string, options Options) error {
	return d.runner(ctx, "helm", []string{"lint", options.ChartPath, "-f", valuesFile})
}

func (d *Deployer) template(ctx context.Context, valuesFile string, options Options) error {
	if err := clearRenderedPath(options.RenderedPath); err != nil {
		return err
	}
	return d.runner(ctx, "helm", []string{
		"template", options.ReleaseName, options.ChartPath,
		"--namespace", options.Namespace,
		"-f", valuesFile,
		"--output-dir", options.RenderedPath,
	})
}

func clearRenderedPath(path string) error {
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("clear rendered output directory: %w", err)
	}
	if err := os.MkdirAll(path, 0o750); err != nil {
		return fmt.Errorf("create rendered output directory: %w", err)
	}
	return nil
}

func removeTempFile(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove temp file: %w", err)
	}
	return nil
}

func defaultRunner(ctx context.Context, name string, args []string) error {
	cmd := exec.CommandContext(ctx, name, args...) //#nosec G204 -- production callers fix name to "helm"; tests inject the runner
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v failed: %w: %s", name, args, err, string(output))
	}
	return nil
}
