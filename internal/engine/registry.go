package engine

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// AuthConfig represents registry authentication configuration.
type AuthConfig struct {
	Username      string `json:"username,omitempty"`
	Password      string `json:"password,omitempty"`
	Auth          string `json:"auth,omitempty"`
	ServerAddress string `json:"serveraddress,omitempty"`
	IdentityToken string `json:"identitytoken,omitempty"`
	RegistryToken string `json:"registrytoken,omitempty"`
}

// DockerConfig represents the structure of ~/.docker/config.json.
type DockerConfig struct {
	Auths       map[string]AuthConfig `json:"auths"`
	CredHelpers map[string]string     `json:"credHelpers,omitempty"`
}

// RegistryCredentials stores registry authentication information.
type RegistryCredentials struct {
	Registry string
	Username string
	Password string
	Token    string
}

// ImageCache manages cached container images.
type ImageCache struct {
	cacheDir      string
	maxSize       int64 // Maximum cache size in bytes
	currentSize   int64
	entries       map[string]*ImageCacheEntry
	mu            sync.RWMutex
	cleanupPeriod time.Duration
	debug         bool
}

// ImageCacheEntry represents a cached image.
type ImageCacheEntry struct {
	Image     string
	Registry  string
	Tag       string
	Digest    string
	Size      int64
	LastUsed  time.Time
	PullTime  time.Time
	LocalPath string
}

// RegistryManager handles private registry authentication and image caching.
type RegistryManager struct {
	credentials  map[string]*RegistryCredentials
	dockerConfig *DockerConfig
	imageCache   *ImageCache
	configPath   string
	mu           sync.RWMutex
	debug        bool
}

// NewRegistryManager creates a new registry manager.
func NewRegistryManager(cacheDir string, debug bool) (*RegistryManager, error) {
	return NewRegistryManagerWithConfig(cacheDir, "", debug)
}

// NewRegistryManagerWithConfig creates a new registry manager with custom docker config path.
func NewRegistryManagerWithConfig(cacheDir, dockerConfigPath string, debug bool) (*RegistryManager, error) {
	// Use default docker config path if not provided
	configPath := dockerConfigPath
	if configPath == "" {
		// Default path - this will be overridden by cmd packages when needed
		configPath = filepath.Join("~", ".docker", "config.json")
	}

	// Create image cache
	imageCache, err := NewImageCache(cacheDir, debug)
	if err != nil {
		return nil, fmt.Errorf("failed to create image cache: %w", err)
	}

	rm := &RegistryManager{
		credentials: make(map[string]*RegistryCredentials),
		configPath:  configPath,
		imageCache:  imageCache,
		debug:       debug,
	}

	// Load docker config if it exists
	if err := rm.LoadDockerConfig(); err != nil && debug {
		fmt.Printf("Warning: failed to load docker config: %v\n", err)
	}

	return rm, nil
}

// NewImageCache creates a new image cache.
func NewImageCache(cacheDir string, debug bool) (*ImageCache, error) {
	// Ensure cache directory exists
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	return &ImageCache{
		cacheDir:      cacheDir,
		maxSize:       10 * 1024 * 1024 * 1024, // 10GB default
		entries:       make(map[string]*ImageCacheEntry),
		cleanupPeriod: 24 * time.Hour,
		debug:         debug,
	}, nil
}

// LoadDockerConfig loads docker authentication configuration.
func (rm *RegistryManager) LoadDockerConfig() error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Skip loading if config path starts with ~ (requires home dir expansion by caller)
	if strings.HasPrefix(rm.configPath, "~") {
		if rm.debug {
			fmt.Printf("Skipping docker config load - path needs home dir expansion: %s\n", rm.configPath)
		}
		return nil
	}

	data, err := os.ReadFile(rm.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// No config file, that's ok
			return nil
		}
		return fmt.Errorf("failed to read docker config: %w", err)
	}

	var config DockerConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse docker config: %w", err)
	}

	rm.dockerConfig = &config

	// Load credentials from auths
	for registry, auth := range config.Auths {
		creds := &RegistryCredentials{
			Registry: registry,
		}

		// Decode base64 auth if present
		if auth.Auth != "" {
			decoded, err := base64.StdEncoding.DecodeString(auth.Auth)
			if err == nil {
				parts := strings.SplitN(string(decoded), ":", 2)
				if len(parts) == 2 {
					creds.Username = parts[0]
					creds.Password = parts[1]
				}
			}
		} else {
			creds.Username = auth.Username
			creds.Password = auth.Password
		}

		if auth.IdentityToken != "" {
			creds.Token = auth.IdentityToken
		}

		rm.credentials[registry] = creds
	}

	return nil
}

// GetCredentials returns credentials for a registry.
func (rm *RegistryManager) GetCredentials(registry string) (*RegistryCredentials, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	// Normalize registry URL
	registry = normalizeRegistry(registry)

	// Check direct match
	if creds, ok := rm.credentials[registry]; ok {
		return creds, nil
	}

	// Check for Docker Hub aliases
	if registry == "docker.io" || registry == "registry-1.docker.io" {
		// Try various Docker Hub registry names
		for _, alias := range []string{"docker.io", "https://index.docker.io/v1/", "index.docker.io"} {
			if creds, ok := rm.credentials[alias]; ok {
				return creds, nil
			}
		}
	}

	return nil, fmt.Errorf("no credentials found for registry: %s", registry)
}

// AddCredentials adds credentials for a registry.
func (rm *RegistryManager) AddCredentials(registry, username, password string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	registry = normalizeRegistry(registry)

	rm.credentials[registry] = &RegistryCredentials{
		Registry: registry,
		Username: username,
		Password: password,
	}

	// Update docker config
	if rm.dockerConfig == nil {
		rm.dockerConfig = &DockerConfig{
			Auths: make(map[string]AuthConfig),
		}
	}

	// Encode credentials
	auth := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))

	rm.dockerConfig.Auths[registry] = AuthConfig{
		Username:      username,
		Password:      password,
		Auth:          auth,
		ServerAddress: registry,
	}

	return nil
}

// SaveDockerConfig saves the docker configuration.
func (rm *RegistryManager) SaveDockerConfig() error {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	if rm.dockerConfig == nil {
		return nil
	}

	// Ensure config directory exists
	configDir := filepath.Dir(rm.configPath)
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Marshal config
	data, err := json.MarshalIndent(rm.dockerConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal docker config: %w", err)
	}

	// Write with secure permissions
	if err := os.WriteFile(rm.configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write docker config: %w", err)
	}

	return nil
}

// GetAuthString returns the base64 encoded auth string for docker login.
func (rm *RegistryManager) GetAuthString(registry string) (string, error) {
	creds, err := rm.GetCredentials(registry)
	if err != nil {
		return "", err
	}

	if creds.Token != "" {
		return creds.Token, nil
	}

	if creds.Username != "" && creds.Password != "" {
		auth := base64.StdEncoding.EncodeToString([]byte(creds.Username + ":" + creds.Password))
		return auth, nil
	}

	return "", fmt.Errorf("no valid authentication found for registry: %s", registry)
}

// normalizeRegistry normalizes registry URLs.
func normalizeRegistry(registry string) string {
	// Remove protocol
	registry = strings.TrimPrefix(registry, "https://")
	registry = strings.TrimPrefix(registry, "http://")

	// Remove trailing slash
	registry = strings.TrimSuffix(registry, "/")

	// Handle Docker Hub default
	if registry == "" || registry == "index.docker.io" {
		return "docker.io"
	}

	return registry
}

// ParseImageName parses a full image name into components.
func ParseImageName(image string) (registry, namespace, name, tag string) {
	// Default values
	registry = "docker.io"
	namespace = "library"
	tag = "latest"

	// First split by "/" to get path components
	components := strings.Split(image, "/")

	var imagePart string

	switch len(components) {
	case 1:
		// Just image name (e.g., "alpine" or "alpine:tag")
		imagePart = components[0]
	case 2:
		// Could be namespace/image or registry/image
		if strings.Contains(components[0], ".") || strings.Contains(components[0], ":") {
			// It's a registry
			registry = components[0]
			imagePart = components[1]
			namespace = ""
		} else {
			// It's a namespace
			namespace = components[0]
			imagePart = components[1]
		}
	case 3:
		// Full format: registry/namespace/image
		registry = components[0]
		namespace = components[1]
		imagePart = components[2]
	default:
		// More than 3 components, treat first as registry, second as namespace, rest as name
		registry = components[0]
		namespace = components[1]
		imagePart = strings.Join(components[2:], "/")
	}

	// Now split the image part to extract tag
	tagParts := strings.Split(imagePart, ":")
	if len(tagParts) == 2 {
		name = tagParts[0]
		tag = tagParts[1]
	} else {
		name = imagePart
	}

	return
}

// CacheImage adds an image to the cache.
func (ic *ImageCache) CacheImage(entry *ImageCacheEntry) error {
	ic.mu.Lock()
	defer ic.mu.Unlock()

	// Check if we need to clean up
	if ic.currentSize+entry.Size > ic.maxSize {
		neededSpace := (ic.currentSize + entry.Size) - ic.maxSize
		if err := ic.cleanup(neededSpace); err != nil {
			return fmt.Errorf("failed to make space in cache: %w", err)
		}
	}

	key := fmt.Sprintf("%s:%s", entry.Image, entry.Tag)
	ic.entries[key] = entry
	ic.currentSize += entry.Size

	return nil
}

// GetCachedImage retrieves a cached image if available.
func (ic *ImageCache) GetCachedImage(image, tag string) (*ImageCacheEntry, bool) {
	ic.mu.Lock()
	defer ic.mu.Unlock()

	key := fmt.Sprintf("%s:%s", image, tag)
	entry, exists := ic.entries[key]

	if exists {
		// Update last used time
		entry.LastUsed = time.Now()
	}

	return entry, exists
}

// cleanup removes old entries to make space.
// Returns error for interface consistency (currently always nil).
//
//nolint:unparam // Error return maintained for interface consistency
func (ic *ImageCache) cleanup(neededSpace int64) error {
	// Sort entries by last used time
	type entrySort struct {
		key   string
		entry *ImageCacheEntry
	}

	var sortedEntries []entrySort
	for k, v := range ic.entries {
		sortedEntries = append(sortedEntries, entrySort{k, v})
	}

	// Sort by last used (oldest first)
	for i := 0; i < len(sortedEntries); i++ {
		for j := i + 1; j < len(sortedEntries); j++ {
			if sortedEntries[i].entry.LastUsed.After(sortedEntries[j].entry.LastUsed) {
				sortedEntries[i], sortedEntries[j] = sortedEntries[j], sortedEntries[i]
			}
		}
	}

	// Remove entries until we have enough space
	freedSpace := int64(0)
	for _, es := range sortedEntries {
		// Check if we already have enough space
		if freedSpace >= neededSpace {
			break
		}

		delete(ic.entries, es.key)
		ic.currentSize -= es.entry.Size
		freedSpace += es.entry.Size

		// Remove from disk if needed
		if es.entry.LocalPath != "" {
			os.RemoveAll(es.entry.LocalPath)
		}
	}

	return nil
}

// GetCacheStats returns cache statistics.
func (ic *ImageCache) GetCacheStats() (totalSize int64, entryCount int, oldestEntry time.Time) {
	ic.mu.RLock()
	defer ic.mu.RUnlock()

	totalSize = ic.currentSize
	entryCount = len(ic.entries)

	for _, entry := range ic.entries {
		if oldestEntry.IsZero() || entry.LastUsed.Before(oldestEntry) {
			oldestEntry = entry.LastUsed
		}
	}

	return
}
