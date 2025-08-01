package e2e

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/go-github/v63/github"
)

//go:embed all:templates
var templates embed.FS

func GetClient() (*github.Client, error) {
	token := os.Getenv("GITHUB_PERSONAL_ACCESS_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("GITHUB_PERSONAL_ACCESS_TOKEN is not set")
	}
	return github.NewClient(nil).WithAuthToken(token), nil
}

func GetTemplate(path string) (string, error) {
	data, err := templates.ReadFile(filepath.Join("templates", path))
	if err != nil {
		return "", err
	}
	return string(data), nil
}
