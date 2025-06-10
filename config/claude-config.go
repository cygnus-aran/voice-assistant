package config

// ClaudeConfig holds Claude API settings
type ClaudeConfig struct {
	APIKey       string `json:"api_key"`
	Model        string `json:"model"`
	SystemPrompt string `json:"system_prompt"`
}

// DefaultClaudeConfig returns default Claude configuration
func DefaultClaudeConfig() ClaudeConfig {
	return ClaudeConfig{
		Model:        "claude-sonnet-4-20250514",
		SystemPrompt: "You are a helpful AI assistant. Respond concisely and naturally for voice conversations.",
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
		c.Model = "claude-sonnet-4-20250514" // Set default
	}
	if c.SystemPrompt == "" {
		c.SystemPrompt = "You are a helpful AI assistant. Respond concisely and naturally for voice conversations."
	}
	return nil
}
