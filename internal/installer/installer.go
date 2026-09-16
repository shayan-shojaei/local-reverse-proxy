package installer

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/shayan/local-reverse-proxy/internal/domain"
	"github.com/shayan/local-reverse-proxy/internal/hostsetup"
)

//go:embed assets/*
var assets embed.FS

type Options struct {
	Zone          string
	DashboardPort int
	Version       string
	DryRun        bool
	Writer        func(string)
}

type Installer struct {
	Dir    string
	Runner func(context.Context, string, ...string) error
}

func New() (*Installer, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	return &Installer{Dir: filepath.Join(configDir, "lrp"), Runner: runCommand}, nil
}

func (i *Installer) Install(ctx context.Context, options Options) error {
	if err := domain.ValidateZone(options.Zone); err != nil {
		return err
	}
	if options.DashboardPort < 1 || options.DashboardPort > 65535 {
		return errors.New("dashboard port must be between 1 and 65535")
	}
	plan, err := hostsetup.PlanInstall(runtime.GOOS, options.Zone, i.Dir)
	if err != nil {
		return err
	}
	if options.DryRun {
		options.print("would create installation files in " + i.Dir)
		options.print("would start controller, Caddy, and CoreDNS with Docker Compose")
		for _, description := range plan.Descriptions() {
			options.print("would " + description)
		}
		options.print("would install Caddy's local CA in the host trust store")
		return nil
	}
	if err := i.materialize(options); err != nil {
		return err
	}
	if err := i.compose(ctx, "pull"); err != nil {
		return err
	}
	if err := i.compose(ctx, "up", "-d"); err != nil {
		return err
	}
	if err := i.executePlan(ctx, plan, options.Writer); err != nil {
		return err
	}
	certificate := filepath.Join(i.Dir, "lrp-local-ca.crt")
	if err := i.copyCA(ctx, certificate); err != nil {
		return err
	}
	trustPlan, err := hostsetup.PlanTrust(runtime.GOOS, certificate)
	if err != nil {
		return err
	}
	if err := i.executePlan(ctx, trustPlan, options.Writer); err != nil {
		return err
	}
	options.print("installed Local Reverse Proxy; run `lrp dashboard`")
	return nil
}

func (i *Installer) Upgrade(ctx context.Context, version string) error {
	if version == "" {
		version = "latest"
	}
	if err := replaceEnv(filepath.Join(i.Dir, ".env"), "LRP_VERSION", version); err != nil {
		return err
	}
	if err := i.compose(ctx, "pull"); err != nil {
		return err
	}
	return i.compose(ctx, "up", "-d", "--remove-orphans")
}

func (i *Installer) Uninstall(ctx context.Context, purge bool, writer func(string)) error {
	values, err := readEnv(filepath.Join(i.Dir, ".env"))
	if err != nil {
		return fmt.Errorf("read installation: %w", err)
	}
	plan, err := hostsetup.PlanUninstall(runtime.GOOS, values["LRP_ZONE"])
	if err != nil {
		return err
	}
	if runtime.GOOS == "darwin" {
		plan.Actions = append(plan.Actions, hostsetup.Action{
			Description: "remove the LRP certificate authority from the macOS system keychain",
			Command:     []string{"sudo", "security", "delete-certificate", "-c", "LRP Local Authority", "/Library/Keychains/System.keychain"},
		})
	}
	if err := i.executePlan(ctx, plan, writer); err != nil {
		return err
	}
	args := []string{"down", "--remove-orphans"}
	if purge {
		args = append(args, "--volumes")
	}
	if err := i.compose(ctx, args...); err != nil {
		return err
	}
	if purge {
		return os.RemoveAll(i.Dir)
	}
	return nil
}

func (i *Installer) Env() (map[string]string, error) {
	return readEnv(filepath.Join(i.Dir, ".env"))
}

func (i *Installer) Compose(ctx context.Context, args ...string) error {
	return i.compose(ctx, args...)
}

func (i *Installer) materialize(options Options) error {
	if err := os.MkdirAll(i.Dir, 0o700); err != nil {
		return err
	}
	for _, name := range []string{"compose.yaml", "Corefile", "caddy-bootstrap.json"} {
		contents, err := fs.ReadFile(assets, "assets/"+name)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(i.Dir, name), contents, 0o600); err != nil {
			return err
		}
	}
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return err
	}
	envFile := fmt.Sprintf("LRP_ZONE=%s\nLRP_DASHBOARD_PORT=%d\nLRP_VERSION=%s\nLRP_ADMIN_TOKEN=%s\n",
		options.Zone, options.DashboardPort, options.Version, base64.RawURLEncoding.EncodeToString(token))
	if err := os.WriteFile(filepath.Join(i.Dir, ".env"), []byte(envFile), 0o600); err != nil {
		return err
	}
	resolver := fmt.Sprintf("nameserver 127.0.0.1\nport 53\n# managed by Local Reverse Proxy for %s\n", options.Zone)
	return os.WriteFile(filepath.Join(i.Dir, "resolver"), []byte(resolver), 0o600)
}

func (i *Installer) copyCA(ctx context.Context, destination string) error {
	var lastErr error
	for attempt := 0; attempt < 20; attempt++ {
		lastErr = i.compose(ctx, "cp", "caddy:/data/caddy/pki/authorities/local/root.crt", destination)
		if lastErr == nil {
			return os.Chmod(destination, 0o644)
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("copy Caddy root CA: %w", lastErr)
}

func (i *Installer) compose(ctx context.Context, args ...string) error {
	base := []string{"compose", "--env-file", filepath.Join(i.Dir, ".env"), "-f", filepath.Join(i.Dir, "compose.yaml")}
	return i.Runner(ctx, "docker", append(base, args...)...)
}

func (i *Installer) executePlan(ctx context.Context, plan hostsetup.Plan, writer func(string)) error {
	for _, action := range plan.Actions {
		if writer != nil {
			writer(action.Description)
		}
		if err := i.Runner(ctx, action.Command[0], action.Command[1:]...); err != nil {
			return fmt.Errorf("%s: %w", action.Description, err)
		}
	}
	return nil
}

func runCommand(ctx context.Context, name string, args ...string) error {
	command := exec.CommandContext(ctx, name, args...)
	command.Stdout, command.Stderr, command.Stdin = os.Stdout, os.Stderr, os.Stdin
	return command.Run()
}

func readEnv(path string) (map[string]string, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	values := make(map[string]string)
	for _, line := range strings.Split(string(contents), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok {
			values[key] = value
		}
	}
	return values, nil
}

func replaceEnv(path, key, value string) error {
	values, err := readEnv(path)
	if err != nil {
		return err
	}
	values[key] = value
	order := []string{"LRP_ZONE", "LRP_DASHBOARD_PORT", "LRP_VERSION", "LRP_ADMIN_TOKEN"}
	var output strings.Builder
	for _, name := range order {
		fmt.Fprintf(&output, "%s=%s\n", name, values[name])
	}
	return os.WriteFile(path, []byte(output.String()), 0o600)
}

func (o Options) print(message string) {
	if o.Writer != nil {
		o.Writer(message)
	}
}
