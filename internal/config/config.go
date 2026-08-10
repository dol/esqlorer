package config

import (
	"os"
	"path/filepath"

	"github.com/dominicluechinger/esqlorer/internal/paths"
	"gopkg.in/yaml.v3"
)

type Server struct {
	Name     string `yaml:"name"`
	URL      string `yaml:"url"`
	APIKey   string `yaml:"api_key,omitempty"`
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`
}

type Config struct {
	CurrentContext string   `yaml:"current_context"`
	Servers        []Server `yaml:"servers"`
}

func DefaultConfigDir() string {
	return paths.ConfigDir()
}

func DefaultConfigPath() string {
	return paths.ConfigPath()
}

func DefaultConfig() *Config {
	return &Config{
		CurrentContext: "",
		Servers:        []Server{},
	}
}

func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	if path == "" {
		path = DefaultConfigPath()
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) Save(path string) error {
	if path == "" {
		path = DefaultConfigPath()
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

func (c *Config) GetCurrentServer() *Server {
	for i := range c.Servers {
		if c.Servers[i].Name == c.CurrentContext {
			return &c.Servers[i]
		}
	}
	return nil
}

func (c *Config) GetServer(name string) *Server {
	for i := range c.Servers {
		if c.Servers[i].Name == name {
			return &c.Servers[i]
		}
	}
	return nil
}

func (c *Config) AddServer(server Server) error {
	if c.GetServer(server.Name) != nil {
		return ErrServerAlreadyExists
	}
	c.Servers = append(c.Servers, server)
	return nil
}

func (c *Config) RemoveServer(name string) error {
	for i, s := range c.Servers {
		if s.Name == name {
			c.Servers = append(c.Servers[:i], c.Servers[i+1:]...)
			if c.CurrentContext == name {
				c.CurrentContext = ""
			}
			return nil
		}
	}
	return ErrServerNotFound
}

func (c *Config) SetCurrentContext(name string) error {
	if c.GetServer(name) == nil {
		return ErrServerNotFound
	}
	c.CurrentContext = name
	return nil
}

var (
	ErrServerAlreadyExists = &ConfigError{"server already exists"}
	ErrServerNotFound      = &ConfigError{"server not found"}
)

type ConfigError struct {
	msg string
}

func (e *ConfigError) Error() string {
	return e.msg
}
