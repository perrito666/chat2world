package config

import (
	"encoding/json"
	"fmt"
	"os"
)

type AvailableIM string

const (
	IMTelegram AvailableIM = "telegram"
	IMSignal   AvailableIM = "signal"
)

type AvailableBloggingPlatform string

const (
	MBPMastodon AvailableBloggingPlatform = "mastodon"
	MBPBsky     AvailableBloggingPlatform = "bluesky"
	BPHugo      AvailableBloggingPlatform = "hugo.io"
)

type Config struct {
	EnabledUIDs              map[AvailableIM][]uint64
	EnabledIMs               []AvailableIM
	EnabledBloggingPlatforms []AvailableBloggingPlatform
	AvailableInteractions    map[AvailableIM][]AvailableBloggingPlatform
	BPAuth                   map[AvailableBloggingPlatform]map[string]string
	IMAuth                   map[AvailableIM]map[string]string
	PerUserBloggingConfig    map[uint64]map[AvailableBloggingPlatform]map[string]string
}

func NewConfig() *Config {
	return &Config{
		EnabledUIDs:              map[AvailableIM][]uint64{},
		EnabledIMs:               []AvailableIM{IMTelegram},
		EnabledBloggingPlatforms: []AvailableBloggingPlatform{MBPMastodon},
		AvailableInteractions: map[AvailableIM][]AvailableBloggingPlatform{
			IMTelegram: {MBPMastodon},
		},
		BPAuth:                map[AvailableBloggingPlatform]map[string]string{},
		IMAuth:                map[AvailableIM]map[string]string{},
		PerUserBloggingConfig: map[uint64]map[AvailableBloggingPlatform]map[string]string{},
	}
}

// LoadFromFile reads a json serialized version of a config from a file
func (c *Config) LoadFromFile(path string) error {
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("checking readability of file: %w", err)
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("opening file: %w", err)
	}
	defer f.Close()
	json.NewDecoder(f)
	if err := json.NewDecoder(f).Decode(c); err != nil {
		return fmt.Errorf("decoding config: %w", err)
	}
	return nil
}

func (c *Config) SaveToFile(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating file: %w", err)
	}
	defer f.Close()
	if err := json.NewEncoder(f).Encode(c); err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}
	return nil
}
