package main_test

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"testing"
)

func TestGolangCILint(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode: skipping golangci-lint")
	}
	rungo(t, "run", "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest", "run")
}

func TestGoFmt(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode: skipping gofmt")
	}
	cmd := exec.Command("gofmt", "-l", ".")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		t.Fatalf("gofmt failed to run: %v\nOutput:\n%s", err, out.String())
	}
	if out.Len() > 0 {
		t.Errorf("gofmt found unformatted files:\n%s", out.String())
	}
}

func TestGoModTidy(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode: skipping go mod tidy")
	}
	rungo(t, "mod", "tidy", "-diff")
}

func TestGovulncheck(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode: skipping govulncheck")
	}
	rungo(t, "run", "golang.org/x/vuln/cmd/govulncheck@latest", "./...")
}

func TestGodocLint(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode: skipping godoc-lint")
	}
	rungo(t, "run", "github.com/godoc-lint/godoc-lint/cmd/godoclint@latest", "./...")
}

func TestCoverage(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode: skipping coverage")
	}
	rungo(t, "test", "-coverprofile=coverage.out", "./internal/...", "./cmd/tako/...")

	// Check coverage
	out, err := exec.Command("go", "tool", "cover", "-func=coverage.out").Output()
	if err != nil {
		t.Fatalf("failed to get coverage: %v", err)
	}

	if testing.Verbose() {
		t.Logf("Coverage per function:\n%s", out)
	}

	// Get total coverage
	totalCoverage := 0.0
	lines := bytes.Split(out, []byte("\n"))
	for _, line := range lines {
		if bytes.HasPrefix(line, []byte("total:")) {
			fields := bytes.Fields(line)
			if len(fields) > 2 {
				coverageStr := bytes.TrimSuffix(fields[2], []byte("%"))
				_, err := sscanf(string(coverageStr), "%f", &totalCoverage)
				if err != nil {
					t.Fatalf("failed to parse total coverage: %v", err)
				}
				break
			}
		}
	}

	if totalCoverage < 70.0 {
		t.Errorf("expected coverage to be at least 70.0%%, got %.1f%%", totalCoverage)
	}
}

// sscanf is a simple implementation of sscanf.
func sscanf(str, format string, a ...interface{}) (int, error) {
	n, err := fmt.Sscanf(str, format, a...)
	return n, err
}

func rungo(t *testing.T, args ...string) {
	t.Helper()

	cmd := exec.Command("go", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		if ee := (*exec.ExitError)(nil); errors.As(err, &ee) && len(ee.Stderr) > 0 {
			t.Fatalf("%v: %v\n%s", cmd, err, ee.Stderr)
		}
		t.Fatalf("%v: %v\n%s", cmd, err, output)
	}
}
