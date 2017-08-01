package version

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gravitational/version/pkg/tool"
)

const testPackage = "github.com/gravitational/version/test"

const testBinary = "test"

func TestAutoBuildVersion(t *testing.T) {
	if _, err := exec.LookPath("linkflags"); err != nil {
		t.Skip("skipping because linkflags binary not found")
	}

	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	testPackagePath := filepath.Join(dir, "test")

	git := newGit(testPackagePath)

	_, err = git.RawExec("init", testPackagePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err = os.RemoveAll(filepath.Join(testPackagePath, ".git")); err != nil {
			t.Fatalf("failed to clean up test repository: %v", err)
		}
	}()

	_, err = git.Exec("add", filepath.Join(testPackagePath, "main.go"))
	if err != nil {
		t.Fatal(err)
	}

	_, err = git.Exec("commit", "-m", "Initial commit")
	if err != nil {
		t.Fatal(err)
	}

	commitID, err := git.Exec("rev-parse", "HEAD^{commit}")
	if err != nil {
		t.Fatal(err)
	}

	linkFlags, err := exec.Command("linkflags", "-pkg", testPackagePath).CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}

	goTool := newGoTool()
	_, err = goTool.Exec("build", "-o", filepath.Join(testPackagePath, testBinary), "-ldflags", string(linkFlags), testPackage)
	if err != nil {
		t.Fatal(err)
	}

	payload, err := exec.Command(filepath.Join(testPackagePath, testBinary)).CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}

	var info Info
	if err = json.Unmarshal(payload, &info); err != nil {
		t.Fatal(err)
	}

	if info.GitCommit != commitID {
		t.Fatalf("expected git commit `%s` but got `%s`", commitID, info.GitCommit)
	}
}

func newGit(pkg string) *tool.T {
	args := []string{"--git-dir", filepath.Join(pkg, ".git")}
	return &tool.T{Cmd: "git", Args: args}
}

func newGoTool() *tool.T {
	return &tool.T{Cmd: "go"}
}
