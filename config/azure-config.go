package config

// AzureConfig holds Azure Speech Service settings
type AzureConfig struct {
	SubscriptionKey string `json:"subscription_key"`
	Region          string `json:"region"`
	Language        string `json:"language"`
}

// DefaultAzureConfig returns default Azure configuration
func DefaultAzureConfig() AzureConfig {
	return AzureConfig{
		Language: "en-US",
		// SubscriptionKey and Region need to be set by user
	}
}

// IsConfigured checks if Azure credentials are set
func (c *AzureConfig) IsConfigured() bool {
	return c.SubscriptionKey != "" && c.Region != ""
}

// Validate checks if the Azure configuration is valid
func (c *AzureConfig) Validate() error {
	if c.SubscriptionKey == "" {
		return ErrMissingAzureKey
	}
	if c.Region == "" {
		return ErrMissingAzureRegion
	}
	if c.Language == "" {
		c.Language = "en-US" // Set default
	}
	return nil
}
