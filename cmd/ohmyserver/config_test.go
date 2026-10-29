package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestLoadConfigDefaults(t *testing.T) {
	t.Setenv("OHMYSERVER_PASSWORD", "")
	dir := t.TempDir()
	cfg, err := loadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := defaultConfig()
	want.Data = dir
	if !reflect.DeepEqual(cfg, want) {
		t.Fatalf("got %+v, want %+v", cfg, want)
	}
}

func TestLoadConfigFile(t *testing.T) {
	t.Setenv("OHMYSERVER_PASSWORD", "")
	dir := t.TempDir()
	write(t, dir, "listen: 0.0.0.0:9000\ndata: data\ndns:\n  listen: 127.0.0.1:5300\nacme:\n  retry_delay: 2m\n")
	cfg, err := loadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != "0.0.0.0:9000" || cfg.Data != filepath.Join(dir, "data") || cfg.DNS.Listen != "127.0.0.1:5300" {
		t.Fatalf("got %+v", cfg)
	}
	if cfg.ACME.RetryDelay != 2*time.Minute || cfg.ACME.OrderTimeout != 10*time.Minute || cfg.Gateway.HTTP != ":80" {
		t.Fatalf("settings left out lost their defaults: %+v", cfg)
	}
}

func TestLoadConfigRejectsUnknownSettings(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "listn: :8080\n")
	if _, err := loadConfig(dir); err == nil || !strings.Contains(err.Error(), "listn") {
		t.Fatalf("got %v, want an error naming the setting", err)
	}
}

func TestLoadConfigPasswordFromEnvironment(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "password: file\n")
	t.Setenv("OHMYSERVER_PASSWORD", "env")
	cfg, err := loadConfig(dir)
	if err != nil || cfg.Password != "env" {
		t.Fatalf("got %q, %v", cfg.Password, err)
	}
}

// The example documents the defaults, so it must parse to exactly them.
func TestConfigExampleMatchesDefaults(t *testing.T) {
	data, err := os.ReadFile("../../config.yaml.example")
	if err != nil {
		t.Fatal(err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	if want := defaultConfig(); !reflect.DeepEqual(cfg, want) {
		t.Fatalf("example %+v, defaults %+v", cfg, want)
	}
}

func write(t *testing.T, dir, config string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
}
