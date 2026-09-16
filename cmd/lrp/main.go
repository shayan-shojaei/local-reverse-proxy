package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
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
	case "help", "--help", "-h":
		usage()
		return nil
	default:
		usage()
		return fmt.Errorf("unknown command %q", args[0])
	}
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
  lrp upgrade [--version VERSION]
  lrp uninstall [--purge]`)
}
