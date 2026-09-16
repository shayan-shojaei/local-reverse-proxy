package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/shayan/local-reverse-proxy/internal/installer"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "lrp:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return errors.New("a command is required")
	}
	app, err := installer.New()
	if err != nil {
		return err
	}
	ctx := context.Background()
	switch args[0] {
	case "install":
		flags := flag.NewFlagSet("install", flag.ContinueOnError)
		zone := flags.String("zone", "local.test", "reserved .test DNS zone")
		port := flags.Int("dashboard-port", 7400, "loopback dashboard port")
		version := flags.String("version", "latest", "release version/image tag")
		dryRun := flags.Bool("dry-run", false, "print host changes without applying them")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		return app.Install(ctx, installer.Options{Zone: *zone, DashboardPort: *port, Version: *version, DryRun: *dryRun, Writer: printStep})
	case "dashboard":
		return openDashboard(app)
	case "doctor":
		return doctor(ctx, app)
	case "upgrade":
		flags := flag.NewFlagSet("upgrade", flag.ContinueOnError)
		version := flags.String("version", "latest", "release version/image tag")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		return app.Upgrade(ctx, *version)
	case "uninstall":
		flags := flag.NewFlagSet("uninstall", flag.ContinueOnError)
		purge := flags.Bool("purge", false, "also permanently delete route and certificate data")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		return app.Uninstall(ctx, *purge, printStep)
	case "export":
		flags := flag.NewFlagSet("export", flag.ContinueOnError)
		output := flags.String("output", "lrp-config.json", "destination JSON file")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		return exportConfig(ctx, app, *output)
	case "import":
		flags := flag.NewFlagSet("import", flag.ContinueOnError)
		mode := flags.String("mode", "merge", "merge or replace")
		yes := flags.Bool("yes", false, "apply after printing the preview")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 1 {
			return errors.New("usage: lrp import [--mode merge|replace] [--yes] FILE")
		}
		return importConfig(ctx, app, flags.Arg(0), *mode, *yes)
	case "help", "--help", "-h":
		usage()
		return nil
	default:
		usage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func exportConfig(ctx context.Context, app *installer.Installer, destination string) error {
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := controllerRequest(ctx, app, http.MethodGet, "/api/v1/config/export", nil, &envelope); err != nil {
		return err
	}
	var formatted bytes.Buffer
	if err := json.Indent(&formatted, envelope.Data, "", "  "); err != nil {
		return err
	}
	formatted.WriteByte('\n')
	if err := os.WriteFile(destination, formatted.Bytes(), 0o600); err != nil {
		return err
	}
	fmt.Println("exported configuration to", destination)
	return nil
}

func importConfig(ctx context.Context, app *installer.Installer, path, mode string, apply bool) error {
	contents, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(contents) > 1<<20 {
		return errors.New("import file exceeds 1 MiB")
	}
	var config json.RawMessage
	if err := json.Unmarshal(contents, &config); err != nil {
		return fmt.Errorf("invalid import JSON: %w", err)
	}
	body, _ := json.Marshal(map[string]any{"mode": mode, "config": config})
	var envelope struct {
		Data struct {
			Digest     string `json:"digest"`
			TargetZone string `json:"targetZone"`
			Added      int    `json:"added"`
			Updated    int    `json:"updated"`
			Deleted    int    `json:"deleted"`
		} `json:"data"`
	}
	if err := controllerRequest(ctx, app, http.MethodPost, "/api/v1/config/import/preview", body, &envelope); err != nil {
		return err
	}
	fmt.Printf("target zone: %s\nadd: %d  update: %d  delete: %d\n", envelope.Data.TargetZone, envelope.Data.Added, envelope.Data.Updated, envelope.Data.Deleted)
	if !apply {
		fmt.Println("preview only; rerun with --yes to apply this import")
		return nil
	}
	applyBody, _ := json.Marshal(map[string]string{"digest": envelope.Data.Digest})
	return controllerRequest(ctx, app, http.MethodPost, "/api/v1/config/import/apply", applyBody, nil)
}

func controllerRequest(ctx context.Context, app *installer.Installer, method, path string, body []byte, output any) error {
	values, err := app.Env()
	if err != nil {
		return errors.New("not installed; run `lrp install` first")
	}
	request, err := http.NewRequestWithContext(ctx, method, fmt.Sprintf("http://127.0.0.1:%s%s", values["LRP_DASHBOARD_PORT"], path), bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+values["LRP_ADMIN_TOKEN"])
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := (&http.Client{Timeout: 15 * time.Second}).Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 16<<10))
		return fmt.Errorf("controller returned %s: %s", response.Status, string(message))
	}
	if output != nil {
		return json.NewDecoder(response.Body).Decode(output)
	}
	return nil
}

func openDashboard(app *installer.Installer) error {
	values, err := app.Env()
	if err != nil {
		return errors.New("not installed; run `lrp install` first")
	}
	url := fmt.Sprintf("http://127.0.0.1:%s/#token=%s", values["LRP_DASHBOARD_PORT"], values["LRP_ADMIN_TOKEN"])
	var command *exec.Cmd
	if runtime.GOOS == "darwin" {
		command = exec.Command("open", url)
	} else {
		command = exec.Command("xdg-open", url)
	}
	if err := command.Run(); err != nil {
		return fmt.Errorf("open browser: %w; navigate using `lrp dashboard` from a graphical session", err)
	}
	return nil
}

func doctor(ctx context.Context, app *installer.Installer) error {
	values, err := app.Env()
	if err != nil {
		return errors.New("installation files missing")
	}
	if err := app.Compose(ctx, "ps"); err != nil {
		return fmt.Errorf("Docker Compose stack unhealthy: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%s/api/v1/status", values["LRP_DASHBOARD_PORT"]), nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+values["LRP_ADMIN_TOKEN"])
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("dashboard unreachable: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("dashboard health check returned %s", response.Status)
	}
	fmt.Printf("ok  controller reachable\nok  zone %s\nok  dashboard loopback-only on port %s\n", values["LRP_ZONE"], values["LRP_DASHBOARD_PORT"])
	return nil
}

func printStep(message string) { fmt.Println("→", message) }

func usage() {
	fmt.Fprintln(os.Stderr, `Local Reverse Proxy

Usage:
  lrp install [--zone local.test] [--dashboard-port 7400] [--version latest] [--dry-run]
  lrp dashboard
  lrp doctor
  lrp export [--output lrp-config.json]
  lrp import [--mode merge|replace] [--yes] FILE
  lrp upgrade [--version VERSION]
  lrp uninstall [--purge]`)
}
