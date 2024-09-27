package reverseproxy

import (
	"errors"
	"gopkg.in/yaml.v2"
	"net/http"
	"os"
)

type Route struct {
	Path       string `yaml:"path"`
	Host       string `yaml:"host"`
	TargetPath string `yaml:"target_path"`
}

type ServerConfig struct {
	Port   int     `yaml:"port"`
	Routes []Route `yaml:"routes"`
}

type CachingConfig struct {
	Enabled bool `yaml:"enabled"`
	TTL     int  `yaml:"ttl"`
}

type LoggerConfig struct {
	Level string `yaml:"level"`
	File  string `yaml:"file"`
}

type Config struct {
	Server    ServerConfig  `yaml:"server"`
	AddHeader http.Header   `yaml:"add_header"`
	Caching   CachingConfig `yaml:"caching"`
	Blacklist []string      `yaml:"blacklist"`
	Logger    LoggerConfig  `yaml:"logger"`
}

func (c *Config) Validate() error {
	if c.Server.Port == 0 {
		return errors.New("server port is not set")
	}
	if len(c.Server.Routes) == 0 {
		return errors.New("no server routes are defined")
	}
	for _, route := range c.Server.Routes {
		if route.Path == "" {
			return errors.New("route path is not set")
		}
		if route.TargetPath == "" {
			return errors.New("route target path is not set")
		}
		if route.Host == "" {
			return errors.New("route host is not set")
		}
	}
	if c.Logger.Level == "" {
		return errors.New("logger level is not set")
	}
	if c.Logger.File == "" {
		return errors.New("logger file is not set")
	}
	return nil
}

func LoadConfig(configFileName string) (*Config, error) {
	data, err := os.ReadFile(configFileName)
	if err != nil {
		return nil, err
	}

	var config Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}

	if err := config.Validate(); err != nil {
		return nil, err
	}

	return &config, nil
}
