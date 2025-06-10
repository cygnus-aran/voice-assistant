package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Config holds all application configuration from params.json
type Config struct {
	Azure  AzureConfig  `json:"azure"`
	Claude ClaudeConfig `json:"claude"`
}

// Configuration errors
var (
	ErrMissingAzureKey    = errors.New("Azure subscription key is required")
	ErrMissingAzureRegion = errors.New("Azure region is required")
	ErrMissingClaudeKey   = errors.New("Claude API key is required")
)

// LoadConfig loads the entire configuration from params.json
func LoadConfig() (*Config, error) {
	configPath := getConfigPath()

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Create default config file
		defaultConfig := DefaultConfig()
		err := defaultConfig.Save()
		if err != nil {
			return nil, fmt.Errorf("failed to create default config: %v", err)
		}
		return defaultConfig, nil
	}

	// Read existing config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %v", err)
	}

	var config Config
	err = json.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config file: %v", err)
	}

	return &config, nil
}

// DefaultConfig returns a config with default values
func DefaultConfig() *Config {
	return &Config{
		Azure:  DefaultAzureConfig(),
		Claude: DefaultClaudeConfig(),
	}
}

// Save saves the configuration to params.json
func (c *Config) Save() error {
	configPath := getConfigPath()

	// Create config directory if it doesn't exist
	configDir := filepath.Dir(configPath)
	err := os.MkdirAll(configDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create config directory: %v", err)
	}

	// Marshal config to JSON with nice formatting
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %v", err)
	}

	// Write to file
	err = os.WriteFile(configPath, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write config file: %v", err)
	}

	return nil
}

// ValidateAll validates all configurations
func (c *Config) ValidateAll() []error {
	var errors []error

	if err := c.Azure.Validate(); err != nil {
		errors = append(errors, fmt.Errorf("Azure config: %v", err))
	}

	if err := c.Claude.Validate(); err != nil {
		errors = append(errors, fmt.Errorf("Claude config: %v", err))
	}

	return errors
}

// GetAzureConfig returns the Azure configuration
func (c *Config) GetAzureConfig() *AzureConfig {
	return &c.Azure
}

// GetClaudeConfig returns the Claude configuration
func (c *Config) GetClaudeConfig() *ClaudeConfig {
	return &c.Claude
}

// UpdateAzureConfig updates the Azure configuration and saves
func (c *Config) UpdateAzureConfig(azure AzureConfig) error {
	c.Azure = azure
	return c.Save()
}

// UpdateClaudeConfig updates the Claude configuration and saves
func (c *Config) UpdateClaudeConfig(claude ClaudeConfig) error {
	c.Claude = claude
	return c.Save()
}

// getConfigPath returns the path to params.json
func getConfigPath() string {
	// For development, always check local params.json first
	localPath := "params.json"
	if _, err := os.Stat(localPath); err == nil {
		return localPath
	}

	// Fallback to user config directory
	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		// Final fallback to current directory
		return "params.json"
	}
	return filepath.Join(userConfigDir, "voice-assistant", "params.json")
}

// GetConfigPath returns the config file path (for display to user)
func GetConfigPath() string {
	return getConfigPath()
}

// GetConfigDir returns the config directory path
func GetConfigDir() string {
	return filepath.Dir(getConfigPath())
}
