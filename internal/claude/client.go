package claude

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"voice-assistant/config"
)

// Claude API configuration
type Config struct {
	APIKey       string
	Model        string
	SystemPrompt string
}

// Message represents a single message in the conversation
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Request represents the Claude API request structure
type Request struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	Messages  []Message `json:"messages"`
	System    string    `json:"system,omitempty"`
}

// Response represents the Claude API response structure
type Response struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Role    string `json:"role"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Model        string `json:"model"`
	StopReason   string `json:"stop_reason"`
	StopSequence string `json:"stop_sequence"`
	Usage        struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// Client handles communication with Claude API
type Client struct {
	config          Config
	httpClient      *http.Client
	baseURL         string
	conversationLog []Message // Store conversation history
}

// NewClientFromConfig creates a new Claude API client from app config
func NewClientFromConfig(cfg *config.Config) *Client {
	return NewClient(Config{
		APIKey:       cfg.Claude.APIKey,
		Model:        cfg.Claude.Model,
		SystemPrompt: cfg.Claude.SystemPrompt,
	})
}

// NewClient creates a new Claude API client
func NewClient(config Config) *Client {
	return &Client{
		config: config,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL:         "https://api.anthropic.com/v1",
		conversationLog: make([]Message, 0), // Initialize empty conversation
	}
}

// SendMessage sends a message to Claude and returns the response
func (c *Client) SendMessage(userMessage string) (string, error) {
	log.Printf("Sending message to Claude: %s", userMessage)

	// Add user message to conversation log
	userMsg := Message{
		Role:    "user",
		Content: userMessage,
	}
	c.conversationLog = append(c.conversationLog, userMsg)

	// Prepare the request payload with full conversation history
	request := Request{
		Model:     c.config.Model,
		MaxTokens: 1000,
		Messages:  c.conversationLog,
	}

	// Only include system prompt if this is the first message
	if len(c.conversationLog) == 1 {
		request.System = c.config.SystemPrompt
		log.Printf("Including system prompt (first message)")
	}

	// Marshal request to JSON
	requestBody, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %v", err)
	}

	// Create HTTP request
	url := fmt.Sprintf("%s/messages", c.baseURL)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(requestBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}

	// Set required headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.config.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	// Execute request
	log.Printf("Sending request to Claude API...")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute request: %v", err)
	}
	defer resp.Body.Close()

	// Read response body
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %v", err)
	}

	// Check for HTTP errors
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Claude API error: %s - %s", resp.Status, string(responseBody))
	}

	// Parse response
	var claudeResponse Response
	err = json.Unmarshal(responseBody, &claudeResponse)
	if err != nil {
		return "", fmt.Errorf("failed to parse response: %v", err)
	}

	// Extract text content from response
	if len(claudeResponse.Content) == 0 {
		return "", fmt.Errorf("no content in Claude response")
	}

	responseText := claudeResponse.Content[0].Text

	// Add Claude's response to conversation log
	assistantMsg := Message{
		Role:    "assistant",
		Content: responseText,
	}
	c.conversationLog = append(c.conversationLog, assistantMsg)

	return responseText, nil
}

// SendConversation sends a multi-turn conversation to Claude
func (c *Client) SendConversation(messages []Message) (string, error) {
	log.Printf("Sending conversation with %d messages to Claude", len(messages))

	// Prepare the request payload
	request := Request{
		Model:     c.config.Model,
		MaxTokens: 1000,
		System:    c.config.SystemPrompt,
		Messages:  messages,
	}

	// Marshal request to JSON
	requestBody, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %v", err)
	}

	// Create HTTP request
	url := fmt.Sprintf("%s/messages", c.baseURL)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(requestBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}

	// Set required headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.config.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute request: %v", err)
	}
	defer resp.Body.Close()

	// Read response body
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %v", err)
	}

	// Check for HTTP errors
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Claude API error: %s - %s", resp.Status, string(responseBody))
	}

	// Parse response
	var claudeResponse Response
	err = json.Unmarshal(responseBody, &claudeResponse)
	if err != nil {
		return "", fmt.Errorf("failed to parse response: %v", err)
	}

	// Extract text content from response
	if len(claudeResponse.Content) == 0 {
		return "", fmt.Errorf("no content in Claude response")
	}

	responseText := claudeResponse.Content[0].Text
	log.Printf("Claude conversation response: %s", responseText)

	return responseText, nil
}

// ValidateConfig checks if the Claude configuration is valid
func (c *Client) ValidateConfig() error {
	if c.config.APIKey == "" {
		return fmt.Errorf("Claude API key is required")
	}
	if c.config.Model == "" {
		return fmt.Errorf("Claude model is required")
	}
	return nil
}

// TestConnection tests the connection to Claude API
func (c *Client) TestConnection() error {
	log.Println("Testing Claude API connection...")

	// Send a simple test message
	response, err := c.SendMessage("Hello! Can you respond with just 'API connection successful'?")
	if err != nil {
		return fmt.Errorf("Claude API test failed: %v", err)
	}

	log.Printf("Claude API test successful. Response: %s", response)
	return nil
}
