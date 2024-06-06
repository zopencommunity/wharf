// Licensed Materials - Property of IBM
// Copyright IBM Corp. 2023.
// US Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule Contract with IBM Corp.

// This package is dedicated to exec.Command related calls
package util

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

/////////////////////
// SCRIPT COMMANDS //
/////////////////////

// Run the patch command
func Patch(target string, output string, diff []byte) error {
	cmd := exec.Command("patch", target, "-o", output)
	cmd.Stdin = &bytes.Buffer{}
	cmd.Stdin.Read(diff)
	return run(cmd)
}

// Git clone a repository
func GitClone(repo string, dest string) error {
	cmd := exec.Command("git", "clone", repo, dest)
	return run(cmd)
}

func GitDiff(target string, output string) error {
	cmd := exec.Command("git", "diff", "--output="+output)
	cmd.Dir = target
	return run(cmd)
}

// Run go env and return all it's contents
func GoEnv() (map[string]string, error) {
	cmd := exec.Command("go", "env", "-json")
	out, err := runout(cmd)
	if err != nil {
		return nil, err
	}

	var env map[string]string
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		return nil, err
	}

	return env, nil
}

// Run go build on the command line
func GoBuild(paths []string) (string, error) {
	// TODO: pass in additional build flags
	cmd := exec.Command("go", append([]string{"build", "-mod=readonly"}, paths...)...)
	return runout(cmd)
}

// Run tests on a package
func GoTest(paths []string) (string, error) {
	cmd := exec.Command("go", append([]string{"test"}, paths...)...)
	return runout(cmd)
}

////////////////////////
// WORKSPACE COMMANDS //
////////////////////////

func GoWorkUse(path string) error {
	cmd := exec.Command("go", "work", "use", path)
	return run(cmd)
}

// Replace entry in go.mod
func GoWorkEditReplaceVersion(path string, version string) error {
	cmd := exec.Command("go", "work", "edit", "-replace",
		path+"="+path+"@"+version,
	)
	return run(cmd)
}

// Drop replace entry in go.mod
func GoWorkEditDropReplace(path string) error {
	cmd := exec.Command("go", "work", "edit", "-dropreplace", path)
	return run(cmd)
}

/////////////////////////////////
// GO MODULE SPECIFIC COMMANDS //
/////////////////////////////////

// Tidy go.mod
func GoModTidy() error {
	cmd := exec.Command("go", "mod", "tidy")
	return run(cmd)
}

// Init go.mod
func GoModInit(dir string, path string) error {
	cmd := exec.Command("go", "mod", "init", path)
	cmd.Dir = dir
	return run(cmd)
}

// Run go list -m -u
func GoListModUpdate(mod string) (string, error) {
	cmd := exec.Command("go", "list", "-f", "{{if .Update}}{{.Update.Version}}{{else}}{{.Version}}{{end}}", "-m", "-u", "-mod=readonly", mod)
	return runout(cmd)
}

// Run go list
func GoList(pkgs []string) (string, error) {
	cmd := exec.Command("go", append([]string{"list", "-json", "-e", "-deps", "-mod=readonly"}, pkgs...)...)
	return runout(cmd)
}

// Run go list -find
func GoListPkgDir(pkg string) (string, error) {
	cmd := exec.Command("go", "list", "-f", "{{.Dir}}", "-find", "-mod=readonly", pkg)
	out, err := runout(cmd)
	if err != nil {
		return "", fmt.Errorf("%v\n %w", out, err)
	}
	return out, err
}

func GoListModMain(mod string) error {
	cmd := exec.Command("go", "list", "-m", "-f", "{{.Main}}", "-mod=readonly", mod)
	out, err := runout(cmd)
	if err != nil {
		return err
	}
	if out != "true" {
		return fmt.Errorf("%v: not main module", mod)
	}
	return nil
}

// Run a command, return stdout
func runout(cmd *exec.Cmd) (string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	if err == nil {
		return strings.TrimSpace(stdout.String()), nil
	} else {
		return strings.TrimSpace(stderr.String()), fmt.Errorf("cmd: %v: %w", strings.Join(cmd.Args, " "), err)
	}
}

// Run a command, ignoring stdout
func run(cmd *exec.Cmd) error {
	_, err := runout(cmd)
	return err
}
