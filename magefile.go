//go:build mage

package main

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

const golangCILintVersion = "v2.12.2"

var Default = Build

func Build() error {
	if err := os.MkdirAll("dist", 0o755); err != nil {
		return err
	}
	if err := sh.RunV("cargo", "build", "--locked", "--release", "-p", "bwkp-native"); err != nil {
		return err
	}
	if err := buildKeePassXC(); err != nil {
		return err
	}
	version := envOr("VERSION", "dev")
	commit := envOr("COMMIT", "unknown")
	date := envOr("BUILD_DATE", "unknown")
	output := "dist/bwkp"
	if runtime.GOOS == "windows" {
		output += ".exe"
	}
	ldflags := fmt.Sprintf("-s -w -X github.com/Neur0toxine/bitwarden-keepass-exporter/internal/buildinfo.Version=%s -X github.com/Neur0toxine/bitwarden-keepass-exporter/internal/buildinfo.Commit=%s -X github.com/Neur0toxine/bitwarden-keepass-exporter/internal/buildinfo.Date=%s", version, commit, date)
	// CGo does not include external archive mtimes in Go's build cache key.
	return sh.RunWithV(map[string]string{"CGO_ENABLED": "1"}, "go", "build", "-a", "-trimpath", "-tags", "native", "-ldflags", ldflags, "-o", output, "./cmd/bwkp")
}

func buildKeePassXC() error {
	if err := sh.RunV("cmake", "-S", "native/kpdb", "-B", "target/keepassxc", "-DCMAKE_BUILD_TYPE=Release"); err != nil {
		return err
	}
	return sh.RunV("cmake", "--build", "target/keepassxc", "--config", "Release", "--target", "bwkp_kpdb", "--parallel")
}

type Test mg.Namespace

func (Test) Unit() error {
	if err := sh.RunV("go", "test", "-race", "./..."); err != nil {
		return err
	}
	return sh.RunV("cargo", "test", "--locked", "--workspace", "--exclude", "bwkp-e2e-register")
}

func (Test) Native() error {
	mg.Deps(Build)
	return sh.RunV("go", "test", "-tags", "native", "./...")
}

func (Test) E2E() error {
	mg.Deps(Build)
	return sh.RunV("bash", "test/e2e/run.sh")
}

func (Test) All() { mg.Deps(Test.Unit, Test.Native, Test.E2E) }

func Coverage() error {
	packages := []string{"./pkg/convert", "./pkg/bwapi", "./pkg/kpdb", "./internal/app", "./internal/atomicfile", "./internal/security", "./internal/prompt"}
	arguments := append([]string{"test", "-coverprofile=dist/coverage.out"}, packages...)
	if err := os.MkdirAll("dist", 0o755); err != nil {
		return err
	}
	if err := sh.RunV("go", arguments...); err != nil {
		return err
	}
	output, err := sh.Output("go", "tool", "cover", "-func=dist/coverage.out")
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	fields := strings.Fields(lines[len(lines)-1])
	coverage, err := strconv.ParseFloat(strings.TrimSuffix(fields[len(fields)-1], "%"), 64)
	if err != nil {
		return err
	}
	if coverage < 70 {
		return fmt.Errorf("Go coverage %.1f%% is below 70%%", coverage)
	}
	fmt.Printf("Go core coverage: %.1f%%\n", coverage)
	return nil
}

func Lint() error {
	if err := sh.RunV("go", "run", "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@"+golangCILintVersion, "run"); err != nil {
		return err
	}
	if err := sh.RunV("cargo", "fmt", "--all", "--", "--check"); err != nil {
		return err
	}
	return sh.RunV("cargo", "clippy", "--locked", "--workspace", "--all-targets", "--", "-D", "warnings")
}

func Verify() { mg.Deps(Lint, Coverage, Test.Unit, Test.Native) }

type E2E mg.Namespace

func (E2E) Up() error {
	return sh.RunV("docker", "compose", "-f", "test/e2e/compose.yml", "up", "--detach", "--wait")
}
func (E2E) Down() error {
	return sh.RunV("docker", "compose", "-f", "test/e2e/compose.yml", "down", "--volumes", "--remove-orphans")
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
