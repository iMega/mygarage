package config

import (
	"encoding/json"
	"os"
)

type Config struct {
	Port        int    `json:"port"`
	OpenBrowser bool   `json:"openBrowser"`
	DBPath      string `json:"dbPath"`
	// PublicURL is the external base URL (https://garage.example.com) used
	// for OAuth callbacks when the server sits behind a reverse proxy.
	PublicURL string `json:"publicURL"`
}

func Load(path string) (*Config, error) {
	cfg := &Config{
		Port:        8765,
		OpenBrowser: true,
	}

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}
	defer f.Close()

	if err := json.NewDecoder(f).Decode(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}
