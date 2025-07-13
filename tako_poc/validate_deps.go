package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type TakoConfig struct {
	Dependents []string `yaml:"dependents"`
}

func getRepoName(path string) string {
	absPath, err := filepath.Abs(path)
	if err != nil {
		fmt.Printf("Error getting absolute path for %s: %v\n", path, err)
		return ""
	}
	return filepath.Base(absPath)
}

func buildDependencyGraph(rootDir string, graph map[string][]string, visiting map[string]bool, visited map[string]bool) {
	repoName := getRepoName(rootDir)
	if repoName == "" {
		return
	}

	visiting[repoName] = true

	takoFile := filepath.Join(rootDir, "tako.yml")
	if _, err := os.Stat(takoFile); !os.IsNotExist(err) {
		data, err := ioutil.ReadFile(takoFile)
		if err == nil {
			var config TakoConfig
			if err := yaml.Unmarshal(data, &config); err == nil {
				if config.Dependents != nil {
					for _, dependentPath := range config.Dependents {
						dependentFullPath := filepath.Join(rootDir, dependentPath)
						dependentName := getRepoName(dependentFullPath)
						if dependentName != "" {
							if visiting[dependentName] {
								fmt.Printf("Circular dependency detected involving: %s\n", dependentName)
								continue
							}
							if !visited[dependentName] {
								buildDependencyGraph(dependentFullPath, graph, visiting, visited)
							}
							graph[repoName] = append(graph[repoName], dependentName)
						}
					}
				}
			}
		}
	}

	visiting[repoName] = false
	visited[repoName] = true
}

func printGraph(graph map[string][]string, rootName string, prefix string) {
	dependents, ok := graph[rootName]
	if !ok {
		return
	}

	for i, dependent := range dependents {
		isLast := i == len(dependents)-1
		connector := "├── "
		if isLast {
			connector = "└── "
		}
		fmt.Printf("%s%s%s\n", prefix, connector, dependent)

		newPrefix := prefix
		if isLast {
			newPrefix += "    "
		} else {
			newPrefix += "│   "
		}
		printGraph(graph, dependent, newPrefix)
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run validate_deps.go <root_dir>")
		os.Exit(1)
	}
	rootDir := os.Args[1]

	info, err := os.Stat(rootDir)
	if os.IsNotExist(err) || !info.IsDir() {
		fmt.Printf("Error: Directory not found at %s\n", rootDir)
		os.Exit(1)
	}

	graph := make(map[string][]string)
	visiting := make(map[string]bool)
	visited := make(map[string]bool)

	rootName := getRepoName(rootDir)
	fmt.Println(rootName)
	buildDependencyGraph(rootDir, graph, visiting, visited)
	printGraph(graph, rootName, "")
}
