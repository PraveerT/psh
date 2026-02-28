package client

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Device holds stored connection info for a paired phone.
type Device struct {
	Name  string `json:"name"`
	Host  string `json:"host"`
	Port  int    `json:"port"`
	Token string `json:"token"`
}

// Config is the root config stored in ~/.config/psh/config.json
type Config struct {
	Devices       []Device `json:"devices"`
	DefaultDevice string   `json:"default_device"`
}

func ConfigDir() (string, error) {
	var base string
	switch runtime.GOOS {
	case "windows":
		base = os.Getenv("APPDATA")
		if base == "" {
			base = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Roaming")
		}
	default:
		base = os.Getenv("XDG_CONFIG_HOME")
		if base == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			base = filepath.Join(home, ".config")
		}
	}
	return filepath.Join(base, "psh"), nil
}

func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func LoadConfig() (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return &cfg, nil
}

func SaveConfig(cfg *Config) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func (cfg *Config) GetDevice(name string) (*Device, error) {
	if name == "" {
		name = cfg.DefaultDevice
	}
	if name == "" && len(cfg.Devices) == 1 {
		return &cfg.Devices[0], nil
	}
	for i := range cfg.Devices {
		if cfg.Devices[i].Name == name {
			return &cfg.Devices[i], nil
		}
	}
	if name == "" {
		return nil, fmt.Errorf("no devices paired — run: psh pair")
	}
	return nil, fmt.Errorf("device %q not found — run: psh devices", name)
}

func (cfg *Config) AddOrUpdateDevice(d Device) {
	for i := range cfg.Devices {
		if cfg.Devices[i].Name == d.Name || cfg.Devices[i].Host == d.Host {
			cfg.Devices[i] = d
			return
		}
	}
	cfg.Devices = append(cfg.Devices, d)
	if cfg.DefaultDevice == "" {
		cfg.DefaultDevice = d.Name
	}
}
