package hostsetup

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/shayan-shojaei/local-reverse-proxy/internal/domain"
)

type Action struct {
	Description string
	Command     []string
}

type Plan struct {
	Actions []Action
}

func (p Plan) Descriptions() []string {
	result := make([]string, 0, len(p.Actions))
	for _, action := range p.Actions {
		result = append(result, action.Description)
	}
	return result
}

func PlanInstall(osName, zone, installDir string) (Plan, error) {
	if err := domain.ValidateZone(zone); err != nil {
		return Plan{}, err
	}
	switch osName {
	case "darwin":
		return Plan{Actions: []Action{
			{Description: "create macOS resolver directory", Command: []string{"sudo", "mkdir", "-p", "/etc/resolver"}},
			{Description: fmt.Sprintf("route %s DNS queries to 127.0.0.1", zone), Command: []string{"sudo", "install", "-m", "0644", filepath.Join(installDir, "resolver"), filepath.Join("/etc/resolver", zone)}},
		}}, nil
	case "linux":
		return Plan{Actions: []Action{
			{Description: "route loopback DNS through 127.0.0.1", Command: []string{"sudo", "resolvectl", "dns", "lo", "127.0.0.1"}},
			{Description: fmt.Sprintf("route %s DNS queries to 127.0.0.1", zone), Command: []string{"sudo", "resolvectl", "domain", "lo", "~" + zone}},
		}}, nil
	default:
		return Plan{}, errors.New("v1 supports only macOS and Linux")
	}
}

func PlanTrust(osName, certificatePath string) (Plan, error) {
	switch osName {
	case "darwin":
		return Plan{Actions: []Action{{
			Description: "trust the LRP certificate authority in the macOS system keychain",
			Command:     []string{"sudo", "security", "add-trusted-cert", "-d", "-r", "trustRoot", "-k", "/Library/Keychains/System.keychain", certificatePath},
		}}}, nil
	case "linux":
		return Plan{Actions: []Action{
			{Description: "install the LRP certificate authority", Command: []string{"sudo", "install", "-m", "0644", certificatePath, "/usr/local/share/ca-certificates/lrp-local-ca.crt"}},
			{Description: "refresh Linux system certificate trust", Command: []string{"sudo", "update-ca-certificates"}},
		}}, nil
	default:
		return Plan{}, errors.New("v1 supports only macOS and Linux")
	}
}

func PlanUninstall(osName, zone string) (Plan, error) {
	switch osName {
	case "darwin":
		return Plan{Actions: []Action{{Description: "remove the app-owned macOS split-DNS rule", Command: []string{"sudo", "rm", "-f", filepath.Join("/etc/resolver", zone)}}}}, nil
	case "linux":
		return Plan{Actions: []Action{
			{Description: "remove the app-owned Linux split-DNS rule", Command: []string{"sudo", "resolvectl", "revert", "lo"}},
			{Description: "remove the LRP certificate authority", Command: []string{"sudo", "rm", "-f", "/usr/local/share/ca-certificates/lrp-local-ca.crt"}},
			{Description: "refresh Linux system certificate trust", Command: []string{"sudo", "update-ca-certificates"}},
		}}, nil
	default:
		return Plan{}, errors.New("v1 supports only macOS and Linux")
	}
}
