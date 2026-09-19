package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/shayan-shojaei/local-reverse-proxy/internal/installer"
)

// version is set at build time via -ldflags "-X main.version=X.Y.Z".
var version = "dev"

const repository = "shayan-shojaei/local-reverse-proxy"

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
	switch args[0] {
	case "--version", "-v", "version":
		fmt.Println("lrp", version)
		return nil
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
	case "start":
		if _, err := app.Env(); err != nil {
			return errors.New("not installed; run `lrp install` first")
		}
		return app.Compose(ctx, "up", "-d")
	case "stop":
		if _, err := app.Env(); err != nil {
			return errors.New("not installed; run `lrp install` first")
		}
		return app.Compose(ctx, "stop")
	case "dashboard":
		return openDashboard(app)
	case "doctor":
		return doctor(ctx, app)
	case "upgrade":
		flags := flag.NewFlagSet("upgrade", flag.ContinueOnError)
		requested := flags.String("version", "latest", "release version/image tag")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		tag, err := resolveTag(ctx, *requested)
		if err != nil {
			return err
		}
		if err := upgradeCLI(ctx, tag); err != nil {
			return err
		}
		return app.Upgrade(ctx, strings.TrimPrefix(tag, "v"))
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
  lrp start
  lrp stop
  lrp dashboard
  lrp doctor
  lrp export [--output lrp-config.json]
  lrp import [--mode merge|replace] [--yes] FILE
  lrp upgrade [--version VERSION]
  lrp uninstall [--purge]
  lrp --version`)
}

// resolveTag turns a requested "latest" or bare version into a GitHub release
// tag (e.g. "v0.2.0"), matching install.sh's convention.
func resolveTag(ctx context.Context, requested string) (string, error) {
	if requested != "" && requested != "latest" {
		if !strings.HasPrefix(requested, "v") {
			requested = "v" + requested
		}
		return requested, nil
	}
	body, err := httpGet(ctx, fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repository))
	if err != nil {
		return "", fmt.Errorf("check latest release: %w", err)
	}
	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("check latest release: %w", err)
	}
	if payload.TagName == "" {
		return "", errors.New("check latest release: no tag returned")
	}
	return payload.TagName, nil
}

// upgradeCLI replaces the running lrp binary with the one published for tag,
// unless it is already at that version.
func upgradeCLI(ctx context.Context, tag string) error {
	if version != "dev" && "v"+version == tag {
		fmt.Println("lrp CLI already at", tag)
		return nil
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return fmt.Errorf("CLI self-upgrade is not supported on %s", runtime.GOOS)
	}
	archive := fmt.Sprintf("lrp-%s-%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	base := fmt.Sprintf("https://github.com/%s/releases/download/%s", repository, tag)
	data, err := httpGet(ctx, base+"/"+archive)
	if err != nil {
		return fmt.Errorf("download %s: %w", archive, err)
	}
	checksum, err := httpGet(ctx, base+"/"+archive+".sha256")
	if err != nil {
		return fmt.Errorf("download %s.sha256: %w", archive, err)
	}
	sum := sha256.Sum256(data)
	if fields := strings.Fields(string(checksum)); len(fields) == 0 || fields[0] != hex.EncodeToString(sum[:]) {
		return fmt.Errorf("checksum mismatch for %s", archive)
	}
	binary, err := extractBinary(data)
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	staged := executable + ".upgrade"
	if err := os.WriteFile(staged, binary, 0o755); err != nil {
		return fmt.Errorf("write new lrp binary (try running with sudo): %w", err)
	}
	if err := os.Rename(staged, executable); err != nil {
		os.Remove(staged)
		return fmt.Errorf("replace lrp binary (try running with sudo): %w", err)
	}
	fmt.Println("upgraded lrp CLI to", tag)
	return nil
}

func extractBinary(archiveData []byte) ([]byte, error) {
	gzipReader, err := gzip.NewReader(bytes.NewReader(archiveData))
	if err != nil {
		return nil, err
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if header.Name == "lrp" {
			return io.ReadAll(tarReader)
		}
	}
	return nil, errors.New("lrp binary not found in release archive")
}

func httpGet(ctx context.Context, url string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	response, err := (&http.Client{Timeout: 30 * time.Second}).Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, errors.New(response.Status)
	}
	return io.ReadAll(response.Body)
}
