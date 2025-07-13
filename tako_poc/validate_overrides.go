package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	sourceRepo := "repo-a"
	dependentRepo := "repo-d"
	configFile := filepath.Join(dependentRepo, "go.mod")

	// Create a dummy artifact
	if err := os.MkdirAll(sourceRepo, 0755); err != nil {
		fmt.Printf("Error creating source repo dir: %v\n", err)
		os.Exit(1)
	}
	dummyArtifact := filepath.Join(sourceRepo, "repo-a.zip")
	if err := ioutil.WriteFile(dummyArtifact, []byte("dummy artifact"), 0644); err != nil {
		fmt.Printf("Error creating dummy artifact: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(sourceRepo)

	// Read original content
	originalContent, err := ioutil.ReadFile(configFile)
	if err != nil {
		fmt.Printf("Error reading config file: %v\n", err)
		os.Exit(1)
	}

	// Defer restoration
	defer func() {
		fmt.Println("Restoring config file...")
		if err := ioutil.WriteFile(configFile, originalContent, 0644); err != nil {
			fmt.Printf("Error restoring config file: %v\n", err)
		}
	}()

	// Modify config
	newContent := strings.Replace(string(originalContent), "github.com/some/dependency v1.2.3", "github.com/some/dependency v1.2.4", 1)
	if err := ioutil.WriteFile(configFile, []byte(newContent), 0644); err != nil {
		fmt.Printf("Error writing modified config: %v\n", err)
		os.Exit(1)
	}

	// Simulate successful test
	fmt.Println("--- Running successful test ---")
	fmt.Println("Tests passed")

	// Simulate failed test
	fmt.Println("\n--- Running failed test ---")
	// The deferred function will still run, even with a panic
	panic("Simulating a test failure")
}
