// Licensed Materials - Property of IBM
// Copyright IBM Corp. 2023.
// US Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule Contract with IBM Corp.

package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/zosopentools/wharf/internal/port2"
	"github.com/zosopentools/wharf/internal/porting"
	"golang.org/x/tools/go/vcs"
	"gopkg.in/yaml.v3"
)

type jsonOut struct {
	Modules  []porting.ModuleAction
	Packages []porting.PackageAction
}

const WHARF_TEST_RUN = "WHARF_TEST_RUN"

//go:embed test/expected/*.json
var expectedFS embed.FS

//go:embed test/modules.yaml
var config []byte

var doLongTests = flag.Bool("long", false, "run long tests")
var doLatest = flag.Bool("latest", false, "run tests on latest versions of modules")

var goVersionRx = regexp.MustCompile(`go1.([0-9]+)(?:.([0-9]+))?`)
var moduleNameRx = regexp.MustCompile(`/([a-zA-Z0-9.\-_~]+)(?:/v([0-9]+))?$`)

var testBin string

type Test struct {
	Version string
	Paths   []string

	GoVersion string `yaml:"go_version"` // TODO: remove by parsing go.mod

	Name     string // Test name, if omitted will use version
	Expected string // If omitted will use module@name.json
	simple   bool   // If true, doesn't compare against the expected result

	Long bool
}

type Module struct {
	Default []string // Default path to use for building, otherwise will use the module path
	Name    string   // Custom short name, otherwise will use the last segment of the module path
	// TODO: implement when we need it
	Url string // Custom URL for cloning

	Tests []Test

	SkipLatest bool `yaml:"skip_latest"`
	Long       bool
}

func getDefaultShortName(path string) string {
	info := moduleNameRx.FindStringSubmatch(path)
	if info == nil {
		info = strings.Split(path, "/")
		return info[len(info)-1]
	}

	if info[2] != "" {
		return fmt.Sprintf("%v_v%v", info[1], info[2])
	}
	return info[1]
}

func satisfyGoVersion(ver string) (bool, error) {
	var major, minor, rmajor, rminor int
	info := goVersionRx.FindStringSubmatch(ver)
	if info == nil {
		return false, fmt.Errorf("invalid go version: %v", ver)
	}
	major, _ = strconv.Atoi(info[1])
	if len(info) > 2 {
		minor, _ = strconv.Atoi(info[2])
	}

	rinfo := goVersionRx.FindStringSubmatch(runtime.Version())
	if rinfo == nil {
		return false, fmt.Errorf("unable to parse runtime version: %v", ver)
	}
	rmajor, _ = strconv.Atoi(info[1])
	if len(info) > 2 {
		rminor, _ = strconv.Atoi(info[2])
	}

	return rmajor > major || (rmajor == major && rminor >= minor), nil
}

func TestMain(m *testing.M) {
	flag.Parse()

	if _, ranAsWharf := os.LookupEnv(WHARF_TEST_RUN); ranAsWharf {
		porting.SuppressOutput = true
		port2.SuppressOutput = true
		if err := main2(os.Args[1:], false); err == nil {
			// TODO: make this shared behaviour within main1
			outjson := make(map[string]any, 2)
			outjson["modules"] = port2.ModuleActions
			outjson["packages"] = port2.PackageActions
			if outstrm, err := json.MarshalIndent(outjson, "", "\t"); err == nil {
				fmt.Println(string(outstrm))
			} else {
				fmt.Println(err.Error())
			}
		} else {
			fmt.Printf("unable to port: %v", err)
		}
	} else {
		os.Exit(m.Run())
	}
}

func TestModules(t *testing.T) {
	var err error
	var modules map[string]Module

	if testBin, err = os.Executable(); err != nil {
		t.Fatalf("unable to get test executable: %v", err)
	}

	// Parse Configs
	err = yaml.Unmarshal(config, &modules)
	if err != nil {
		t.Fatalf("unable to parse configs: %v", err)
	}

	for mpath, module := range modules {
		// This could be re-written to be parallelized
		t.Run(mpath, func(t *testing.T) {
			if module.Long && !*doLongTests {
				t.Skip("skipping long tests")
			}

			if len(module.Default) == 0 {
				module.Default = []string{mpath}
			}

			if *doLatest && !module.SkipLatest {
				module.Tests = append(module.Tests, Test{
					Name:   "latest",
					Paths:  module.Default,
					simple: true,
				})
			}

			if len(module.Tests) == 0 {
				t.Skip("no tests to run")
			}

			if module.Name == "" {
				module.Name = getDefaultShortName(mpath)
			}

			// TODO: add support for module.Url
			repo, err := vcs.RepoRootForImportPath(mpath, false)
			if err != nil {
				t.Fatalf("unable to find repository: %v", err)
			}

			for _, test := range module.Tests {
				if test.Name == "" {
					test.Name = test.Version
				}

				t.Run(test.Name, func(t *testing.T) {
					if test.Long && !*doLongTests {
						t.Skip("skipping long tests")
					}

					if test.GoVersion != "" {
						if valid, err := satisfyGoVersion(test.GoVersion); err != nil {
							t.Fatalf("unable to check version requirements: %v", err)
						} else if !valid {
							t.Skipf("unable to satisfy version requirements: wanted %v or higher, using %v", test.GoVersion, runtime.Version())
						}
					}

					rpath := test.Expected
					if rpath == "" {
						rpath = fmt.Sprintf("%v@%v.json", module.Name, test.Name)
					}

					var expect porting.ActionList // TODO: make concrete
					if !test.simple {
						if b, err := expectedFS.ReadFile(filepath.Join("test/expected", rpath)); err != nil {
							t.Fatalf("unable to read test data %v: %v", rpath, err)
						} else if err := json.Unmarshal(b, &expect); err != nil {
							t.Fatalf("unable to parse test data %v: %v", rpath, err)
						}
					}

					testRoot := t.TempDir()
					repoRoot := filepath.Join(testRoot, module.Name)

					if test.Version != "" {
						if err := repo.VCS.CreateAtRev(repoRoot, repo.Repo, test.Version); err != nil {
							t.Fatalf("cannot clone repo: %v", err)
						}
					} else if err := repo.VCS.Create(repoRoot, repo.Repo); err != nil {
						t.Fatalf("cannot clone repo: %v", err)
					}

					if len(test.Paths) == 0 {
						if len(module.Default) > 0 {
							test.Paths = module.Default
						} else {
							test.Paths = []string{mpath + "/..."}
						}
					}

					if _, err := os.Stat(filepath.Join(repoRoot, "go.mod")); err != nil {
						if errors.Is(err, fs.ErrNotExist) {
							cmd := exec.Command("go", "mod", "init", mpath)
							cmd.Dir = repoRoot
							if err := cmd.Run(); err != nil {
								t.Fatalf("go mod init: %v", err)
							}

							cmd = exec.Command("go", "mod", "tidy")
							cmd.Dir = repoRoot
							stderr := bytes.Buffer{}
							cmd.Stderr = &stderr
							if err := cmd.Run(); err != nil {
								t.Fatalf("go mod tidy: %v: %v", err, stderr.String())
							}
						} else {
							t.Fatalf("stat go.mod: %v", err)
						}
					}

					cmd := exec.Command("go", "work", "init", repoRoot)
					cmd.Dir = testRoot
					if err := cmd.Run(); err != nil {
						t.Fatalf("go work init: %v", err)
					} else if _, err = os.Stat(filepath.Join(testRoot, "go.work")); err != nil {
						t.Fatalf("go.work not created: unable to stat go.work: %v", err)
					}
					// cpcmd := exec.Command("cp", "-r", testRoot, "/home/macmala/wharf-wk/testout")
					// cpcmd.Run()
					cmd = exec.Command(testBin, test.Paths...)
					cmd.Dir = testRoot
					cmd.Env = append(os.Environ(), "WHARF_TEST_RUN=1")
					var stdout bytes.Buffer
					var stderr bytes.Buffer
					cmd.Stdout = &stdout
					cmd.Stderr = &stderr
					if err := cmd.Run(); err != nil {
						// fmt.Println(stderr.String())
						// fmt.Println(stdout.String())
						t.Fatalf("wharf failure: %v", err)
					}
					// fmt.Println(stderr.String())
					// fmt.Println(stdout.String())
					out := jsonOut{}
					if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
						t.Fatalf("unable to parse output: %v", err)
					}

					if len(expect.Modules) != len(out.Modules) {
						t.FailNow()
					}
					moduleSet := make(map[string]*porting.ModuleAction, len(expect.Modules))
					for i := range expect.Modules {
						mod := &expect.Modules[i]
						moduleSet[mod.Path] = mod
					}
					for _, oMod := range out.Modules {
						if eMod, ok := moduleSet[oMod.Path]; !ok {
							t.FailNow()
						} else if !compareModules(eMod, &oMod) {
							t.FailNow()
						}
					}

					if len(expect.Packages) != len(out.Packages) {
						t.FailNow()
					}
					packageSet := make(map[string]*porting.PackageAction, len(expect.Packages))
					for i := range expect.Packages {
						pkg := &expect.Packages[i]
						packageSet[pkg.Path] = pkg
					}
					for _, oPkg := range out.Packages {
						if ePkg, ok := packageSet[oPkg.Path]; !ok {
							t.FailNow()
						} else if !comparePackages(ePkg, &oPkg) {
							t.FailNow()
						}
					}
				})
			}
		})
	}
}

func compareModules(a *porting.ModuleAction, b *porting.ModuleAction) bool {
	if a.Path != b.Path || a.Version != b.Version {
		return false
	}

	aVerChanged := a.Fixed != a.Version
	bVerChanged := b.Fixed != b.Version
	if aVerChanged != bVerChanged {
		return false
	}

	if a.Imported != b.Imported {
		return false
	}

	return true
}

func comparePackages(a *porting.PackageAction, b *porting.PackageAction) bool {
	if a.Path != b.Path || a.Module != b.Module {
		return false
	}

	if len(a.Tags) != len(b.Tags) {
		return false
	}
	tags := make(map[string]bool, len(a.Tags))
	for _, aTag := range a.Tags {
		tags[aTag] = true
	}
	for _, bTag := range b.Tags {
		delete(tags, bTag)
	}
	if len(tags) > 0 {
		return false
	}

	if len(a.Tokens) != len(b.Tokens) {
		return false
	}
	tokens := make(map[string]*porting.TokenAction, len(a.Tokens))
	for i := range a.Tokens {
		aToken := &a.Tokens[i]
		tokens[aToken.File+aToken.Token] = aToken
	}
	for _, bToken := range b.Tokens {
		if aToken, ok := tokens[bToken.File+bToken.Token]; !ok {
			return false
		} else if aToken.File != bToken.File || aToken.Token != bToken.Token || aToken.Change != bToken.Change {
			return false
		}
	}

	if len(a.Files) != len(b.Files) {
		return false
	}
	files := make(map[string]*porting.FileAction, len(a.Files))
	for i := range a.Files {
		aFile := &a.Files[i]
		files[aFile.Name] = aFile
	}
	for _, bFile := range b.Files {
		if aFile, ok := files[bFile.Name]; !ok {
			return false
		} else if aFile.Build != bFile.Build || aFile.BaseFile != bFile.BaseFile {
			return false
		}
	}

	return true
}
