package hostsetup

import (
	"runtime"
	"strings"
	"testing"
)

func TestPlanUsesPlatformSpecificSplitDNS(t *testing.T) {
	for _, osName := range []string{"darwin", "linux"} {
		t.Run(osName, func(t *testing.T) {
			plan, err := PlanInstall(osName, "dev.test", "/tmp/lrp")
			if err != nil {
				t.Fatalf("PlanInstall: %v", err)
			}
			joined := strings.Join(plan.Descriptions(), "\n")
			if !strings.Contains(joined, "dev.test") || !strings.Contains(joined, "127.0.0.1") {
				t.Fatalf("plan does not configure zone: %s", joined)
			}
		})
	}
}

func TestPlanRejectsUnsupportedOperatingSystem(t *testing.T) {
	if _, err := PlanInstall("windows", "local.test", "/tmp/lrp"); err == nil {
		t.Fatal("PlanInstall succeeded for Windows")
	}
}

func TestCurrentPlatformIsSupported(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("v1 supports macOS and Linux")
	}
	if _, err := PlanInstall(runtime.GOOS, "local.test", "/tmp/lrp"); err != nil {
		t.Fatalf("PlanInstall current platform: %v", err)
	}
}
