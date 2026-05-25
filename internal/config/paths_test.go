package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestResolveConfigPath_prefersCwd(t *testing.T) {
	dir := t.TempDir()
	cwdConfig := filepath.Join(dir, DefaultConfigFileName)
	if err := os.WriteFile(cwdConfig, []byte("server:\n  port: 9090\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })

	got, err := ResolveConfigPath("")
	if err != nil {
		t.Fatal(err)
	}
	if got != cwdConfig {
		// macOS: /var is a symlink to /private/var
		gotClean, _ := filepath.EvalSymlinks(got)
		wantClean, _ := filepath.EvalSymlinks(cwdConfig)
		if gotClean != wantClean {
			t.Fatalf("got %q, want %q", got, cwdConfig)
		}
	}
}

func TestResolveConfigPath_explicitPath(t *testing.T) {
	explicit := filepath.Join(t.TempDir(), "custom.yaml")
	got, err := ResolveConfigPath(explicit)
	if err != nil {
		t.Fatal(err)
	}
	if got != explicit {
		t.Fatalf("got %q, want %q", got, explicit)
	}
}

func TestResolveConfigPath_fallsBackToUserDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CG_CONFIG_HOME", dir)

	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	empty := filepath.Join(t.TempDir(), "empty-cwd")
	if err := os.MkdirAll(empty, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(empty); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })

	got, err := ResolveConfigPath("config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, DefaultConfigFileName)
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if !fileExists(want) {
		t.Fatalf("expected seeded config at %s", want)
	}
	if !fileExists(filepath.Join(dir, localConfigFileName)) {
		t.Fatal("expected seeded config.local.yaml")
	}
}

func TestEnsureUserConfigDir_idempotent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CG_CONFIG_HOME", dir)

	first, err := EnsureUserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	second, err := EnsureUserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("dir mismatch: %q vs %q", first, second)
	}

	data, err := os.ReadFile(filepath.Join(dir, DefaultConfigFileName))
	if err != nil {
		t.Fatal(err)
	}
	cfg := Default()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		t.Fatal(err)
	}
	wantSessions := filepath.Join(dir, "sessions.json")
	if cfg.Session.StoragePath != wantSessions {
		t.Fatalf("session storage %q, want %q", cfg.Session.StoragePath, wantSessions)
	}
}
