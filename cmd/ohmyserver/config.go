package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is <dir>/config.yaml (config.yaml.example documents every
// setting). A missing file, or a setting left out, takes the default.
type Config struct {
	// Listen is the panel's address.
	Listen string `yaml:"listen"`
	// Data holds the shared database and one subdirectory per module.
	// Relative paths are relative to the configuration directory, which
	// is also the default.
	Data string `yaml:"data"`
	// Password requires signing in; OHMYSERVER_PASSWORD overrides it.
	Password string `yaml:"password"`
	// Host is the address users reach installed apps at (default: derived
	// from Listen).
	Host string `yaml:"host"`
	// Catalog is the app catalog git repository ("-": use the local copy
	// as is).
	Catalog string `yaml:"catalog"`
	// Gateway and DNS listeners apply to a new installation only; they are
	// then edited in the panel.
	Gateway GatewayConfig `yaml:"gateway"`
	DNS     DNSConfig     `yaml:"dns"`
	ACME    ACMEConfig    `yaml:"acme"`
}

type GatewayConfig struct {
	HTTP  string `yaml:"http"`
	HTTPS string `yaml:"https"`
}

type DNSConfig struct {
	// Listen is the UDP and TCP address.
	Listen string `yaml:"listen"`
}

// ACMEConfig holds acme-go's network and recovery settings. An attempt
// limit of 1 disables retries at that layer.
type ACMEConfig struct {
	RequestTimeout    time.Duration `yaml:"request_timeout"`
	TLSTimeout        time.Duration `yaml:"tls_timeout"`
	RequestAttempts   int           `yaml:"request_attempts"`
	IssuanceAttempts  int           `yaml:"issuance_attempts"`
	RetryDelay        time.Duration `yaml:"retry_delay"`
	ValidationTimeout time.Duration `yaml:"validation_timeout"`
	OrderTimeout      time.Duration `yaml:"order_timeout"`
}

func defaultConfig() Config {
	return Config{
		Listen:  "127.0.0.1:8080",
		Data:    ".",
		Catalog: defaultCatalog,
		Gateway: GatewayConfig{HTTP: ":80", HTTPS: ":443"},
		DNS:     DNSConfig{Listen: ":53"},
		ACME: ACMEConfig{
			RequestTimeout: time.Minute, TLSTimeout: 30 * time.Second,
			RequestAttempts: 4, IssuanceAttempts: 4, RetryDelay: time.Minute,
			ValidationTimeout: 5 * time.Minute, OrderTimeout: 10 * time.Minute,
		},
	}
}

// loadConfig reads dir/config.yaml over the defaults and resolves Data
// against dir. Unknown settings are errors, so a misspelt one is not
// silently ignored.
func loadConfig(dir string) (Config, error) {
	cfg := defaultConfig()
	dir, err := filepath.Abs(dir)
	if err != nil {
		return cfg, err
	}
	path := filepath.Join(dir, "config.yaml")
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return cfg, err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) {
		return cfg, fmt.Errorf("%s: %w", path, err)
	}
	if cfg.Data == "" {
		cfg.Data = "."
	}
	if !filepath.IsAbs(cfg.Data) {
		cfg.Data = filepath.Join(dir, cfg.Data)
	}
	if password := os.Getenv("OHMYSERVER_PASSWORD"); password != "" {
		cfg.Password = password
	}
	return cfg, nil
}
