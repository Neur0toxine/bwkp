//go:build mage

package main

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

const golangCILintVersion = "v2.12.2"

const termuxPackagesCommit = "c0294462552ec4a03633a11afd72fc903a550182"

var Default = Build

func Build() error {
	if err := os.MkdirAll("dist", 0o755); err != nil {
		return err
	}
	if err := sh.RunV("cargo", "build", "--locked", "--release", "-p", "bwkp-native"); err != nil {
		return err
	}
	if err := stageRustLibrary(); err != nil {
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
	ldflags := fmt.Sprintf("-s -w -X github.com/Neur0toxine/bwkp/internal/buildinfo.Version=%s -X github.com/Neur0toxine/bwkp/internal/buildinfo.Commit=%s -X github.com/Neur0toxine/bwkp/internal/buildinfo.Date=%s", version, commit, date)
	ldflags += " -buildid="
	switch runtime.GOOS {
	case "linux":
		ldflags += " -extldflags=-Wl,--gc-sections,--build-id=none"
	case "darwin":
		ldflags += " -extldflags=-Wl,-dead_strip,-no_uuid"
	}
	// CGo does not include external archive mtimes in Go's build cache key.
	if err := sh.RunWithV(map[string]string{"CGO_ENABLED": "1"}, "go", "build", "-a", "-trimpath", "-tags", "native", "-ldflags", ldflags, "-o", output, "./cmd/bwkp"); err != nil {
		return err
	}
	return packBinary(output)
}

// stageRustLibrary gives cgo a target-independent archive path when Cargo is
// cross-compiling a Windows release.
func stageRustLibrary() error {
	target := os.Getenv("CARGO_BUILD_TARGET")
	if target == "" {
		return nil
	}
	destination := filepath.Join("target", "release", "libbwkp_native.a")
	for _, name := range []string{"libbwkp_native.a", "bwkp_native.lib"} {
		source := filepath.Join("target", target, "release", name)
		if _, err := os.Stat(source); err == nil {
			return copyFile(source, destination, 0o644)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	return fmt.Errorf("Cargo did not produce the bwkp-native static library for %s", target)
}

// Image builds the runtime container image, preferring Podman over Docker.
func Image() error {
	engine, err := containerEngine()
	if err != nil {
		return err
	}
	version := envOr("VERSION", "dev")
	image := envOr("BWKP_IMAGE", "bwkp:"+version)
	arguments := []string{
		"--target", "runtime",
		"--tag", image,
		"--build-arg", "VERSION=" + version,
		"--build-arg", "COMMIT=" + envOr("COMMIT", "unknown"),
		"--build-arg", "BUILD_DATE=" + envOr("BUILD_DATE", "unknown"),
		".",
	}
	if engine == "podman" {
		return sh.RunV(engine, append([]string{"build"}, arguments...)...)
	}
	if dockerBuildxAvailable() {
		return sh.RunV(engine, append([]string{"buildx", "build", "--load"}, arguments...)...)
	}
	return sh.RunV(engine, append([]string{"build"}, arguments...)...)
}

func containerEngine() (string, error) {
	if _, err := exec.LookPath("podman"); err == nil {
		return "podman", nil
	}
	if _, err := exec.LookPath("docker"); err == nil {
		return "docker", nil
	}
	return "", fmt.Errorf("container image build requires podman or docker in PATH")
}

func dockerBuildxAvailable() bool {
	return exec.Command("docker", "buildx", "version").Run() == nil
}

func buildKeePassXC() error {
	arguments := []string{"-S", "native/kpdb", "-B", "target/keepassxc", "-DCMAKE_BUILD_TYPE=Release"}
	if runtime.GOOS == "windows" {
		if windeployqt, err := exec.LookPath("windeployqt-qt5"); err == nil {
			arguments = append(arguments, "-DWINDEPLOYQT_EXE="+filepath.ToSlash(windeployqt))
		}
	}
	if err := sh.RunV("cmake", arguments...); err != nil {
		return err
	}
	return sh.RunV("cmake", "--build", "target/keepassxc", "--config", "Release", "--target", "bwkp_kpdb", "--parallel")
}

type Android mg.Namespace

// Arm64 builds an Android arm64 binary for Termux.
func (Android) Arm64() error { return buildAndroid("aarch64", "arm64") }

// Armv7 builds an Android 32-bit ARMv7 binary for Termux.
func (Android) Armv7() error { return buildAndroid("arm", "armv7") }

// All builds both supported Termux Android architectures.
func (Android) All() { mg.Deps(Android.Arm64, Android.Armv7) }

func buildAndroid(termuxArch, artifactArch string) error {
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("Android builds require Docker in PATH: %w", err)
	}
	repository, err := os.MkdirTemp("", "bwkp-termux-packages-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(repository)
	if err := os.Chmod(repository, 0o755); err != nil {
		return err
	}
	if err := sh.RunV("git", "-C", repository, "init", "--quiet"); err != nil {
		return err
	}
	if err := sh.RunV("git", "-C", repository, "remote", "add", "origin", "https://github.com/termux/termux-packages.git"); err != nil {
		return err
	}
	if err := sh.RunV("git", "-C", repository, "fetch", "--depth", "1", "origin", termuxPackagesCommit); err != nil {
		return err
	}
	if err := sh.RunV("git", "-C", repository, "checkout", "--quiet", "--detach", "FETCH_HEAD"); err != nil {
		return err
	}
	podman, err := configureTermuxRunner(repository)
	if err != nil {
		return err
	}
	if err := copyTree("build/termux", filepath.Join(repository, "packages", "bwkp")); err != nil {
		return err
	}
	if err := copySource(filepath.Join(repository, "sources", "bwkp")); err != nil {
		return err
	}
	containerName := "bwkp-termux-" + termuxArch
	if err := runTermuxBuilder(repository, containerName, termuxArch, podman); err != nil {
		return err
	}
	archives, err := filepath.Glob(filepath.Join(repository, "output", "bwkp_*.deb"))
	if err != nil {
		return err
	}
	if len(archives) != 1 {
		return fmt.Errorf("expected one Android package, found %d", len(archives))
	}
	packageRoot := filepath.Join(repository, "extracted")
	if err := extractDeb(archives[0], packageRoot); err != nil {
		return err
	}
	if err := os.MkdirAll("dist", 0o755); err != nil {
		return err
	}
	source := filepath.Join(packageRoot, "data", "data", "com.termux", "files", "usr", "bin", "bwkp")
	output := filepath.Join("dist", "bwkp-android-"+artifactArch)
	if err := copyFile(source, output, 0o755); err != nil {
		return err
	}
	return packBinary(output)
}

func packBinary(path string) error {
	if os.Getenv("BWKP_UPX") == "0" {
		return nil
	}
	upx, err := exec.LookPath("upx")
	if err != nil {
		fmt.Println("UPX not found; leaving binary unpacked")
		return nil
	}
	if err := sh.RunV(upx, "--best", "--lzma", path); err != nil {
		return fmt.Errorf("pack %s with UPX: %w", path, err)
	}
	if err := sh.RunV(upx, "--test", path); err != nil {
		return fmt.Errorf("test UPX-packed %s: %w", path, err)
	}
	return nil
}

func extractDeb(archive, destination string) error {
	if dpkgDeb, err := exec.LookPath("dpkg-deb"); err == nil {
		return sh.RunV(dpkgDeb, "--extract", archive, destination)
	}
	bsdtar, err := exec.LookPath("bsdtar")
	if err != nil {
		return fmt.Errorf("extract Android package: neither dpkg-deb nor bsdtar is available")
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return err
	}
	if err := sh.RunV(bsdtar, "--extract", "--file", archive, "--directory", destination); err != nil {
		return err
	}
	dataArchives, err := filepath.Glob(filepath.Join(destination, "data.tar*"))
	if err != nil {
		return err
	}
	if len(dataArchives) != 1 {
		return fmt.Errorf("expected one data archive in Android package, found %d", len(dataArchives))
	}
	return sh.RunV(bsdtar, "--extract", "--file", dataArchives[0], "--directory", destination)
}

func configureTermuxRunner(repository string) (bool, error) {
	path := filepath.Join(repository, "scripts", "run-docker.sh")
	content, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	text := string(content)
	if exec.Command("aa-status", "--enabled").Run() != nil {
		text = strings.ReplaceAll(text, " --security-opt apparmor=_custom-termux-package-builder-$CONTAINER_NAME", "")
	}
	dockerVersion, err := exec.Command("docker", "version").CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("inspect Docker-compatible engine: %w", err)
	}
	return strings.Contains(string(dockerVersion), "Podman Engine"), os.WriteFile(path, []byte(text), 0o755)
}

func runTermuxBuilder(repository, containerName, termuxArch string, podman bool) error {
	if !podman {
		defer exec.Command("docker", "rm", "--force", containerName).Run()
		cacheMount := "--volume " + containerName + "-cache:/home/builder/.termux-build"
		if os.Getenv("CI") == "true" {
			cacheMount = "--tmpfs /home/builder/.termux-build:exec,mode=1777"
		}
		environment := map[string]string{
			"CONTAINER_NAME":                                    containerName,
			"TERMUX_DOCKER_RUN_EXTRA_ARGS":                      cacheMount,
			"TERMUX_DOCKER_EXEC_EXTRA_ARGS":                     "--env VERSION --env COMMIT --env BUILD_DATE",
			"TERMUX_PKG_MAKE_PROCESSES":                         strconv.Itoa(runtime.NumCPU()),
			"TERMUX_RM_ALL_PKGS_BUILT_MARKER_AND_INSTALL_FILES": "false",
		}
		command := exec.Command(filepath.Join(repository, "scripts", "run-docker.sh"), "./build-package.sh", "-a", termuxArch, "-I", "bwkp")
		command.Dir = repository
		command.Env = os.Environ()
		for name, value := range environment {
			command.Env = append(command.Env, name+"="+value)
		}
		command.Stdin = os.Stdin
		command.Stdout = os.Stdout
		command.Stderr = os.Stderr
		if err := command.Run(); err != nil {
			return fmt.Errorf("run Termux builder: %w", err)
		}
		return nil
	}
	absoluteRepository, err := filepath.Abs(repository)
	if err != nil {
		return err
	}
	arguments := []string{
		"run", "--rm", "--init",
		"--volume", absoluteRepository + ":/home/builder/termux-packages",
		"--volume", containerName + "-cache:/home/builder/.termux-build",
		"--security-opt", "seccomp=" + filepath.Join(absoluteRepository, "scripts", "profile.json"),
		"--cap-add", "CAP_SYS_ADMIN", "--device", "/dev/fuse",
		"--user=0:0",
		"--env", "HOME=/home/builder", "--env", "CI=true", "--env", "VERSION", "--env", "COMMIT", "--env", "BUILD_DATE",
		"--mount", "type=tmpfs,destination=/data,tmpfs-mode=0777",
		"ghcr.io/termux/package-builder", "./build-package.sh", "-a", termuxArch, "-I", "bwkp",
	}
	return sh.RunWithV(map[string]string{"TERMUX_PKG_MAKE_PROCESSES": strconv.Itoa(runtime.NumCPU())}, "docker", arguments...)
}

func copySource(destination string) error {
	output, err := exec.Command("git", "ls-files", "--cached", "--others", "--exclude-standard", "-z").Output()
	if err != nil {
		return err
	}
	for name := range strings.SplitSeq(strings.TrimSuffix(string(output), "\x00"), "\x00") {
		if name == "" {
			continue
		}
		info, err := os.Stat(name)
		if err != nil {
			return err
		}
		if err := copyFile(name, filepath.Join(destination, name), info.Mode().Perm()); err != nil {
			return err
		}
	}
	return nil
}

func copyTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		return copyFile(path, filepath.Join(destination, relative), info.Mode().Perm())
	})
}

func copyFile(source, destination string, mode fs.FileMode) error {
	content, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	return os.WriteFile(destination, content, mode)
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
	return sh.RunV("go", "-C", "test/e2e", "test", "-count=1", "-v", ".")
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
