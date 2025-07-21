package main_test

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestNoEnvironmentVariableAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode: skipping environment variable access check")
	}

	violations := checkCodeViolations(t, "environment variable access", func(call *ast.CallExpr) (bool, string) {
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "os" {
				switch sel.Sel.Name {
				case "Getenv", "LookupEnv", "Environ", "UserHomeDir", "Getwd":
					return true, fmt.Sprintf("os.%s()", sel.Sel.Name)
				}
			}
		}
		return false, ""
	})

	if len(violations) > 0 {
		t.Errorf("Found environment variable access outside of cmd packages:\n%s", strings.Join(violations, "\n"))
	}
}

func TestNoFlagParsingAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode: skipping flag parsing access check")
	}

	violations := checkCodeViolations(t, "flag parsing", func(call *ast.CallExpr) (bool, string) {
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			if ident, ok := sel.X.(*ast.Ident); ok {
				switch ident.Name {
				case "cobra", "pflag", "viper":
					return true, fmt.Sprintf("%s.%s()", ident.Name, sel.Sel.Name)
				case "flag":
					// Only flag parsing functions, not flag.Set or other non-parsing functions
					switch sel.Sel.Name {
					case "Parse", "Bool", "String", "Int", "Float64", "Duration", "Var":
						return true, fmt.Sprintf("flag.%s()", sel.Sel.Name)
					}
				}
			}
		}
		return false, ""
	})

	if len(violations) > 0 {
		t.Errorf("Found flag parsing outside of cmd packages:\n%s", strings.Join(violations, "\n"))
	}
}

func checkCodeViolations(t *testing.T, checkType string, checkFunc func(*ast.CallExpr) (bool, string)) []string {
	violations := []string{}

	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip test files, cmd packages, and test directories
		if strings.HasSuffix(path, "_test.go") ||
			strings.HasPrefix(path, "cmd/") ||
			strings.Contains(path, "/cmd/") ||
			path == "e2e_test.go" ||
			strings.HasPrefix(path, "test/") ||
			strings.Contains(path, "/test/") {
			return nil
		}

		// Only check Go files
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		// Skip vendor and hidden directories
		if strings.Contains(path, "/vendor/") || strings.Contains(path, "/.") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		// Parse the Go file
		fset := token.NewFileSet()
		node, err := parser.ParseFile(fset, path, content, parser.ParseComments)
		if err != nil {
			return err
		}

		// Check for violations
		ast.Inspect(node, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				if isViolation, funcName := checkFunc(call); isViolation {
					pos := fset.Position(call.Pos())
					violations = append(violations, fmt.Sprintf("%s:%d:%d: found %s call outside of cmd package",
						path, pos.Line, pos.Column, funcName))
				}
			}
			return true
		})

		return nil
	})

	if err != nil {
		t.Fatalf("Failed to walk directory for %s check: %v", checkType, err)
	}

	return violations
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
