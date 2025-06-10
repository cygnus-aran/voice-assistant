package config

// ClaudeConfig holds Claude API settings
type ClaudeConfig struct {
	APIKey string `json:"api_key"`
	Model  string `json:"model"`
}

// DefaultClaudeConfig returns default Claude configuration
func DefaultClaudeConfig() ClaudeConfig {
	return ClaudeConfig{
		Model: "claude-3-sonnet-20240229",
		// APIKey needs to be set by user
	}
}

// IsConfigured checks if Claude credentials are set
func (c *ClaudeConfig) IsConfigured() bool {
	return c.APIKey != ""
}

// Validate checks if the Claude configuration is valid
func (c *ClaudeConfig) Validate() error {
	if c.APIKey == "" {
		return ErrMissingClaudeKey
	}
	if c.Model == "" {
		c.Model = "claude-3-sonnet-20240229" // Set default
	}
	return nil
}
