package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type LogConfig struct {
	Level     string `json:"level" yaml:"level"`           // debug, info, warn, error
	Format    string `json:"format" yaml:"format"`         // text, json
	Output    string `json:"output" yaml:"output"`         // file path, empty for stderr
	AddSource bool   `json:"add_source" yaml:"add_source"` // include source file:line
}

// yaml config
type Config struct {
	Listen    string `json:"listen" yaml:"listen"`
	BasePath  string `json:"base_path" yaml:"base_path"`
	AccessKey string `json:"access_key" yaml:"access_key"`
	Database  struct {
		Driver string `json:"driver" yaml:"driver"`
		Path   string `json:"path" yaml:"path"`
	} `json:"database" yaml:"database"`
	CollectionInterval int       `json:"collection_interval_seconds" yaml:"collection_interval_seconds"`
	Logger             LogConfig `json:"logger" yaml:"logger"`
	Docker             struct {
		RegistryMirror string `json:"registry_mirror" yaml:"registry_mirror"`
	} `json:"docker" yaml:"docker"`
}

var DefaultLogConfig = LogConfig{
	Level:  "info",
	Format: "text",
}

var ConfigDir = filepath.Join(os.Getenv("HOME"), ".ohmyserver")
var ConfigPath = filepath.Join(ConfigDir, "config.yaml")
var WorkspaceDir = filepath.Join(ConfigDir, "workspace")
var DockerDir = filepath.Join(WorkspaceDir, "docker")
var DockerComposeDir = filepath.Join(DockerDir, "compose")
var AppsDir = filepath.Join(ConfigDir, "apps")

func LoadConfig() (config *Config, err error) {
	config = &Config{
		Listen: "0.0.0.0:9000",
		Logger: DefaultLogConfig,
	}
	config.Database.Driver = "sqlite"
	config.Database.Path = filepath.Join(ConfigDir, "ohmyserver.db")
	data, err := os.ReadFile(ConfigPath)
	if err == nil {
		err = yaml.Unmarshal(data, &config)
	} else if os.IsNotExist(err) {
		err = nil
	}
	return
}
