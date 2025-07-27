package engine

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewRegistryManager(t *testing.T) {
	tmpDir := t.TempDir()

	rm, err := NewRegistryManager(tmpDir, true)
	if err != nil {
		t.Fatalf("NewRegistryManager() failed: %v", err)
	}

	if rm.imageCache == nil {
		t.Error("Image cache should be initialized")
	}

	if rm.credentials == nil {
		t.Error("Credentials map should be initialized")
	}
}

func TestRegistryManager_LoadDockerConfig(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test docker config
	dockerDir := filepath.Join(tmpDir, ".docker")
	os.MkdirAll(dockerDir, 0700)

	testConfig := DockerConfig{
		Auths: map[string]AuthConfig{
			"docker.io": {
				Username: "testuser",
				Password: "testpass",
				Auth:     base64.StdEncoding.EncodeToString([]byte("testuser:testpass")),
			},
			"ghcr.io": {
				Auth: base64.StdEncoding.EncodeToString([]byte("ghuser:ghtoken")),
			},
		},
	}

	configData, _ := json.Marshal(testConfig)
	configPath := filepath.Join(dockerDir, "config.json")
	os.WriteFile(configPath, configData, 0600)

	// Create registry manager with custom config path
	rm := &RegistryManager{
		credentials: make(map[string]*RegistryCredentials),
		configPath:  configPath,
		debug:       true,
	}

	err := rm.LoadDockerConfig()
	if err != nil {
		t.Fatalf("LoadDockerConfig() failed: %v", err)
	}

	// Verify credentials were loaded
	dockerCreds, err := rm.GetCredentials("docker.io")
	if err != nil {
		t.Fatalf("GetCredentials(docker.io) failed: %v", err)
	}

	if dockerCreds.Username != "testuser" {
		t.Errorf("Username = %s, want testuser", dockerCreds.Username)
	}
	if dockerCreds.Password != "testpass" {
		t.Errorf("Password = %s, want testpass", dockerCreds.Password)
	}

	// Test decoded auth
	ghcrCreds, err := rm.GetCredentials("ghcr.io")
	if err != nil {
		t.Fatalf("GetCredentials(ghcr.io) failed: %v", err)
	}

	if ghcrCreds.Username != "ghuser" {
		t.Errorf("Username = %s, want ghuser", ghcrCreds.Username)
	}
	if ghcrCreds.Password != "ghtoken" {
		t.Errorf("Password = %s, want ghtoken", ghcrCreds.Password)
	}
}

func TestRegistryManager_AddCredentials(t *testing.T) {
	tmpDir := t.TempDir()
	rm, err := NewRegistryManager(tmpDir, false)
	if err != nil {
		t.Fatalf("NewRegistryManager() failed: %v", err)
	}

	// Add credentials
	err = rm.AddCredentials("myregistry.io", "myuser", "mypass")
	if err != nil {
		t.Fatalf("AddCredentials() failed: %v", err)
	}

	// Verify credentials
	creds, err := rm.GetCredentials("myregistry.io")
	if err != nil {
		t.Fatalf("GetCredentials() failed: %v", err)
	}

	if creds.Username != "myuser" {
		t.Errorf("Username = %s, want myuser", creds.Username)
	}
	if creds.Password != "mypass" {
		t.Errorf("Password = %s, want mypass", creds.Password)
	}

	// Verify docker config was updated
	if rm.dockerConfig == nil {
		t.Fatal("Docker config should be created")
	}

	auth, exists := rm.dockerConfig.Auths["myregistry.io"]
	if !exists {
		t.Fatal("Registry should exist in docker config")
	}

	expectedAuth := base64.StdEncoding.EncodeToString([]byte("myuser:mypass"))
	if auth.Auth != expectedAuth {
		t.Errorf("Auth = %s, want %s", auth.Auth, expectedAuth)
	}
}

func TestRegistryManager_GetAuthString(t *testing.T) {
	tmpDir := t.TempDir()
	rm, err := NewRegistryManager(tmpDir, false)
	if err != nil {
		t.Fatalf("NewRegistryManager() failed: %v", err)
	}

	// Add credentials
	rm.AddCredentials("test.io", "user", "pass")

	// Get auth string
	authStr, err := rm.GetAuthString("test.io")
	if err != nil {
		t.Fatalf("GetAuthString() failed: %v", err)
	}

	expectedAuth := base64.StdEncoding.EncodeToString([]byte("user:pass"))
	if authStr != expectedAuth {
		t.Errorf("GetAuthString() = %s, want %s", authStr, expectedAuth)
	}

	// Test with token
	rm.credentials["token.io"] = &RegistryCredentials{
		Registry: "token.io",
		Token:    "mytoken",
	}

	authStr, err = rm.GetAuthString("token.io")
	if err != nil {
		t.Fatalf("GetAuthString() with token failed: %v", err)
	}

	if authStr != "mytoken" {
		t.Errorf("GetAuthString() with token = %s, want mytoken", authStr)
	}
}

func TestNormalizeRegistry(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://docker.io", "docker.io"},
		{"http://ghcr.io/", "ghcr.io"},
		{"registry.example.com", "registry.example.com"},
		{"", "docker.io"},
		{"index.docker.io", "docker.io"},
		{"https://index.docker.io/v1/", "index.docker.io/v1"},
	}

	for _, tt := range tests {
		got := normalizeRegistry(tt.input)
		if got != tt.want {
			t.Errorf("normalizeRegistry(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseImageName(t *testing.T) {
	tests := []struct {
		image     string
		registry  string
		namespace string
		name      string
		tag       string
	}{
		{
			image:     "alpine",
			registry:  "docker.io",
			namespace: "library",
			name:      "alpine",
			tag:       "latest",
		},
		{
			image:     "alpine:3.14",
			registry:  "docker.io",
			namespace: "library",
			name:      "alpine",
			tag:       "3.14",
		},
		{
			image:     "myuser/myapp",
			registry:  "docker.io",
			namespace: "myuser",
			name:      "myapp",
			tag:       "latest",
		},
		{
			image:     "ghcr.io/owner/image:v1.0",
			registry:  "ghcr.io",
			namespace: "owner",
			name:      "image",
			tag:       "v1.0",
		},
		{
			image:     "registry.example.com:5000/myimage",
			registry:  "registry.example.com:5000",
			namespace: "",
			name:      "myimage",
			tag:       "latest",
		},
		{
			image:     "localhost:5000/test/app:dev",
			registry:  "localhost:5000",
			namespace: "test",
			name:      "app",
			tag:       "dev",
		},
	}

	for _, tt := range tests {
		registry, namespace, name, tag := ParseImageName(tt.image)

		if registry != tt.registry {
			t.Errorf("ParseImageName(%q) registry = %q, want %q", tt.image, registry, tt.registry)
		}
		if namespace != tt.namespace {
			t.Errorf("ParseImageName(%q) namespace = %q, want %q", tt.image, namespace, tt.namespace)
		}
		if name != tt.name {
			t.Errorf("ParseImageName(%q) name = %q, want %q", tt.image, name, tt.name)
		}
		if tag != tt.tag {
			t.Errorf("ParseImageName(%q) tag = %q, want %q", tt.image, tag, tt.tag)
		}
	}
}

func TestImageCache_CacheAndRetrieve(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewImageCache(tmpDir, false)
	if err != nil {
		t.Fatalf("NewImageCache() failed: %v", err)
	}

	// Add entry to cache
	entry := &ImageCacheEntry{
		Image:     "test/image",
		Registry:  "docker.io",
		Tag:       "latest",
		Digest:    "sha256:abc123",
		Size:      1024 * 1024, // 1MB
		LastUsed:  time.Now(),
		PullTime:  time.Now(),
		LocalPath: filepath.Join(tmpDir, "test-image.tar"),
	}

	err = cache.CacheImage(entry)
	if err != nil {
		t.Fatalf("CacheImage() failed: %v", err)
	}

	// Save original timestamp before retrieval
	originalLastUsed := entry.LastUsed

	// Small delay to ensure timestamp changes
	time.Sleep(time.Millisecond)

	// Retrieve from cache
	cached, exists := cache.GetCachedImage("test/image", "latest")
	if !exists {
		t.Fatal("Image should exist in cache")
	}

	if cached.Digest != "sha256:abc123" {
		t.Errorf("Cached digest = %s, want sha256:abc123", cached.Digest)
	}

	// Check that last used was updated
	if !cached.LastUsed.After(originalLastUsed) {
		t.Errorf("LastUsed should be updated when retrieved. Original: %v, Cached: %v", originalLastUsed, cached.LastUsed)
	}
}

func TestImageCache_Cleanup(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewImageCache(tmpDir, false)
	if err != nil {
		t.Fatalf("NewImageCache() failed: %v", err)
	}

	// Set small max size for testing
	cache.maxSize = 3 * 1024 * 1024 // 3MB

	// Add multiple entries
	entries := []*ImageCacheEntry{
		{
			Image:    "image1",
			Tag:      "latest",
			Size:     1024 * 1024, // 1MB
			LastUsed: time.Now().Add(-2 * time.Hour),
		},
		{
			Image:    "image2",
			Tag:      "latest",
			Size:     1024 * 1024, // 1MB
			LastUsed: time.Now().Add(-1 * time.Hour),
		},
	}

	for _, entry := range entries {
		cache.CacheImage(entry)
	}

	// Add a third entry that should trigger cleanup
	newEntry := &ImageCacheEntry{
		Image:    "image3",
		Tag:      "latest",
		Size:     1500 * 1024, // 1.5MB
		LastUsed: time.Now(),
	}

	err = cache.CacheImage(newEntry)
	if err != nil {
		t.Fatalf("CacheImage() failed: %v", err)
	}

	// Verify oldest entry was removed
	_, exists := cache.GetCachedImage("image1", "latest")
	if exists {
		t.Error("Oldest image should have been removed")
	}

	// Verify newer entries still exist
	_, exists = cache.GetCachedImage("image2", "latest")
	if !exists {
		t.Error("image2 should still exist")
	}

	_, exists = cache.GetCachedImage("image3", "latest")
	if !exists {
		t.Error("image3 should exist")
	}
}

func TestImageCache_GetStats(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewImageCache(tmpDir, false)
	if err != nil {
		t.Fatalf("NewImageCache() failed: %v", err)
	}

	// Add entries
	now := time.Now()
	entries := []*ImageCacheEntry{
		{
			Image:    "image1",
			Tag:      "latest",
			Size:     1024 * 1024,
			LastUsed: now.Add(-2 * time.Hour),
		},
		{
			Image:    "image2",
			Tag:      "latest",
			Size:     2 * 1024 * 1024,
			LastUsed: now.Add(-1 * time.Hour),
		},
	}

	for _, entry := range entries {
		cache.CacheImage(entry)
	}

	// Get stats
	totalSize, count, oldest := cache.GetCacheStats()

	if totalSize != 3*1024*1024 {
		t.Errorf("Total size = %d, want %d", totalSize, 3*1024*1024)
	}

	if count != 2 {
		t.Errorf("Entry count = %d, want 2", count)
	}

	if !oldest.Equal(now.Add(-2 * time.Hour)) {
		t.Errorf("Oldest entry time mismatch")
	}
}

func TestRegistryManager_SaveDockerConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".docker", "config.json")

	rm := &RegistryManager{
		credentials: make(map[string]*RegistryCredentials),
		configPath:  configPath,
		dockerConfig: &DockerConfig{
			Auths: map[string]AuthConfig{
				"test.io": {
					Username: "user",
					Password: "pass",
					Auth:     base64.StdEncoding.EncodeToString([]byte("user:pass")),
				},
			},
		},
	}

	err := rm.SaveDockerConfig()
	if err != nil {
		t.Fatalf("SaveDockerConfig() failed: %v", err)
	}

	// Verify file was created with correct permissions
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("Config file not created: %v", err)
	}

	if info.Mode().Perm() != 0600 {
		t.Errorf("Config file permissions = %v, want 0600", info.Mode().Perm())
	}

	// Verify content
	data, _ := os.ReadFile(configPath)
	var config DockerConfig
	json.Unmarshal(data, &config)

	if auth, ok := config.Auths["test.io"]; !ok || auth.Username != "user" {
		t.Error("Saved config does not contain expected credentials")
	}
}
