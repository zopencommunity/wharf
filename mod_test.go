// Licensed Materials - Property of IBM
// Copyright IBM Corp. 2023.
// US Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule Contract with IBM Corp.

package main

import (
	"bytes"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"strconv"
	"testing"
	"regexp"
	"runtime"

	"github.com/zosopentools/wharf/internal/util"
	"golang.org/x/tools/go/vcs"
	"gopkg.in/yaml.v3"
)

//go:embed internal/modtest/*.yaml
var configsDir embed.FS

var longTests *bool

var goVersionRegxp = regexp.MustCompile("go1.[0-9]+(.[0-9]+)?")

type testConfig struct {
	Module  string
	Paths   []string
	Version string

	// required go version to run the test
	// skip the test if current go version does not have the require go version
	GoVersion string `yaml:"go_version"`

	// Test needs more time to complete (also won't run under normal conditions)
	Long bool
	// Test is known to fail (still runs but doesn't report a failed port as a failed test)
	Fails bool

	// TODO: add support for partial fixes / intermediate steps
}

func TestMain(m *testing.M) {
	// This is currently useless, but for parsing args this will be useful
	longTests = flag.Bool("long", false, "run long tests")

	os.Exit(m.Run())
}

func TestModules(t *testing.T) {
	// Parse Configs
	entries, err := configsDir.ReadDir("internal/modtest")
	if err != nil {
		t.Fatalf("unable to parse configs: %v", err)
	}
	for _, entry := range entries {
		t.Run(strings.TrimSuffix(entry.Name(), ".yaml"), func(t *testing.T) {
			var cfg testConfig
			if b, err := configsDir.ReadFile(filepath.Join("internal/modtest", entry.Name())); err != nil {
				t.Fatalf("unable to open config: %v", err)
			} else if err := yaml.Unmarshal(b, &cfg); err != nil {
				t.Fatalf("unable to parse config: %v", err)
			}

			if cfg.Long && !*longTests {
				t.Skip("disabled long tests")
			}

			if cfg.GoVersion != "" {
				if !goVersionRegxp.MatchString(cfg.GoVersion) {
					t.Fatal("incorrect go version formation: ", cfg.GoVersion)
				}
				currentVersionNums := strings.Split(cfg.GoVersion, ".")
				runtimeVersionNums := strings.Split(runtime.Version(), ".")
				//go1.21.2 will become ["go1", "21", "2"]
				if len(currentVersionNums) >= 2 {
					currentGoVersion, _ := strconv.Atoi(currentVersionNums[1])
					runtimeGoVersion, _ := strconv.Atoi(runtimeVersionNums[1])
					if runtimeGoVersion < currentGoVersion {
						t.Skip("test require ", cfg.GoVersion, " current go version is ", runtime.Version())
					}

					if currentGoVersion == runtimeGoVersion && 
					len(currentVersionNums) > 2 && len(runtimeVersionNums) > 2 {
						currentGoVersion, _ = strconv.Atoi(currentVersionNums[2])
						runtimeGoVersion, _ = strconv.Atoi(runtimeVersionNums[2])
						if runtimeGoVersion < currentGoVersion {
							t.Skip("test require ", cfg.GoVersion, " current go version is ", runtime.Version())
						}
					}
				}
			}

			if cfg.Paths == nil {
				cfg.Paths = []string{cfg.Module}
			}

			repo, err := vcs.RepoRootForImportPath(cfg.Module, false)
			if err != nil {
				t.Fatalf("cannot resolve repo: %v", err)
			}

			if cfg.Version != "" {
				t.Run(cfg.Version, func(t *testing.T) {
					run(repo, cfg.Module, cfg.Paths, cfg.Version, !cfg.Fails, t)
				})
			}

			t.Run("latest", func(t *testing.T) {
				run(repo, cfg.Module, cfg.Paths, "", !cfg.Fails, t)
			})
		})
	}
}

func run(repo *vcs.RepoRoot, module string, paths []string, version string, expectSuccess bool, t *testing.T) {
	// dir := t.TempDir()
	lastIndex := strings.LastIndex(module, "/")
	dir, _ := os.MkdirTemp("", module[lastIndex + 1:])

	targetDir := filepath.Join(dir, "target")

	// Download module and set up the environment
	if version != "" {
		if err := repo.VCS.CreateAtRev(targetDir, repo.Repo, version); err != nil {
			t.Fatal(err)
		}
	} else if err := repo.VCS.Create(targetDir, repo.Repo); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(targetDir, "go.mod")); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			cmd := exec.Command("go", "mod", "init", module)
			cmd.Dir = targetDir
			if err := cmd.Run(); err != nil {
				t.Fatalf("go mod init: %v", err)
			}

			cmd = exec.Command("go", "mod", "tidy")
			cmd.Dir = targetDir
			stderr := bytes.Buffer{}
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("go mod tidy: %v: %v", err, stderr.String())
			}
		} else {
			t.Fatalf("stat go.mod: %v", err)
		}
	}

	cmd := exec.Command("go", "work", "init", targetDir)
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("go work init: %v", err)
	} else if _, err = os.Stat(filepath.Join(dir, "go.work")); err != nil {
		t.Fatalf("go.work not created")
	}

	if err := os.Setenv("GOWORK", filepath.Join(dir, "go.work")); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		// TODO: handle err
		os.Unsetenv("GOWORK")
	})

	goenv, err := util.GoEnv()
	if err != nil {
		t.Fatal("Unable to read 'go env':", err)
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatal(r)
		}
	}()

	// TODO: set up a test build for this that runs in a child executable
	err = main1(paths, []string{}, false, false, false, filepath.Join(dir, "wharf_port"), goenv)
	if err != nil {
		if expectSuccess {
			t.Fatal(err)
		}
	} else {
		if !expectSuccess {
			t.Fatal("Expecting an error but did not receive one")
		}
	}
}

func cleanup(dir string) error {
	err := os.RemoveAll(dir)
	if err != nil {
		return fmt.Errorf("rm: %w", err)
	}

	return nil
}

func setup() (string, error) {
	workdir, err := os.MkdirTemp("", "")
	if err != nil {
		return workdir, fmt.Errorf("mkdirtemp: %w", err)
	}

	return workdir, nil
}
