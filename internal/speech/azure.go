package speech

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// AzureConfig holds Azure Speech Service configuration
type AzureConfig struct {
	SubscriptionKey string
	Region          string
	Language        string // e.g., "en-US"
}

// SpeechToTextResponse represents Azure STT API response
type SpeechToTextResponse struct {
	RecognitionStatus string `json:"RecognitionStatus"`
	DisplayText       string `json:"DisplayText"`
	Offset            int64  `json:"Offset"`
	Duration          int64  `json:"Duration"`
}

// TextToSpeechRequest represents Azure TTS API request
type TextToSpeechRequest struct {
	Text      string
	VoiceName string
	Language  string
}

// AzureSpeechService handles Azure Speech Services integration
type AzureSpeechService struct {
	config     AzureConfig
	httpClient *http.Client
}

// NewAzureSpeechService creates a new Azure Speech Service client
func NewAzureSpeechService(config AzureConfig) *AzureSpeechService {
	return &AzureSpeechService{
		config: config,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SpeechToText converts audio file to text using Azure STT
func (a *AzureSpeechService) SpeechToText(audioFilePath string) (string, error) {
	log.Printf("Converting audio to text: %s", audioFilePath)

	// Read audio file
	audioFile, err := os.Open(audioFilePath)
	if err != nil {
		return "", fmt.Errorf("failed to open audio file: %v", err)
	}
	defer audioFile.Close()

	// Create multipart form data
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)

	// Add audio file to form
	part, err := writer.CreateFormFile("audio", filepath.Base(audioFilePath))
	if err != nil {
		return "", fmt.Errorf("failed to create form file: %v", err)
	}

	_, err = io.Copy(part, audioFile)
	if err != nil {
		return "", fmt.Errorf("failed to copy audio data: %v", err)
	}

	err = writer.Close()
	if err != nil {
		return "", fmt.Errorf("failed to close multipart writer: %v", err)
	}

	// Build API URL
	apiURL := fmt.Sprintf("https://%s.stt.speech.microsoft.com/speech/recognition/conversation/cognitiveservices/v1?language=%s",
		a.config.Region, a.config.Language)

	// Create HTTP request
	req, err := http.NewRequest("POST", apiURL, &requestBody)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}

	// Set headers
	req.Header.Set("Ocp-Apim-Subscription-Key", a.config.SubscriptionKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")

	// Execute request
	log.Printf("Sending STT request to Azure...")
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute STT request: %v", err)
	}
	defer resp.Body.Close()

	// Read response
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %v", err)
	}

	// Check for HTTP errors
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Azure STT API error: %s - %s", resp.Status, string(responseBody))
	}

	// Parse JSON response
	var sttResponse SpeechToTextResponse
	err = json.Unmarshal(responseBody, &sttResponse)
	if err != nil {
		return "", fmt.Errorf("failed to parse STT response: %v", err)
	}

	// Check recognition status
	if sttResponse.RecognitionStatus != "Success" {
		return "", fmt.Errorf("speech recognition failed: %s", sttResponse.RecognitionStatus)
	}

	log.Printf("STT Success: %s", sttResponse.DisplayText)
	return sttResponse.DisplayText, nil
}

// TextToSpeech converts text to speech using Azure TTS
func (a *AzureSpeechService) TextToSpeech(text string, outputPath string) error {
	log.Printf("Converting text to speech: %s", text)

	// Build SSML (Speech Synthesis Markup Language)
	ssml := fmt.Sprintf(`<speak version='1.0' xml:lang='%s'>
		<voice xml:lang='%s' xml:gender='Female' name='en-US-JennyNeural'>
			%s
		</voice>
	</speak>`, a.config.Language, a.config.Language, text)

	// Build API URL
	apiURL := fmt.Sprintf("https://%s.tts.speech.microsoft.com/cognitiveservices/v1", a.config.Region)

	// Create HTTP request
	req, err := http.NewRequest("POST", apiURL, bytes.NewBufferString(ssml))
	if err != nil {
		return fmt.Errorf("failed to create TTS request: %v", err)
	}

	// Set headers
	req.Header.Set("Ocp-Apim-Subscription-Key", a.config.SubscriptionKey)
	req.Header.Set("Content-Type", "application/ssml+xml")
	req.Header.Set("X-Microsoft-OutputFormat", "riff-24khz-16bit-mono-pcm")
	req.Header.Set("User-Agent", "VoiceAssistant")

	// Execute request
	log.Printf("Sending TTS request to Azure...")
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute TTS request: %v", err)
	}
	defer resp.Body.Close()

	// Check for HTTP errors
	if resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Azure TTS API error: %s - %s", resp.Status, string(responseBody))
	}

	// Create output file
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %v", err)
	}
	defer outputFile.Close()

	// Copy audio data to file
	_, err = io.Copy(outputFile, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write audio data: %v", err)
	}

	log.Printf("TTS Success: Audio saved to %s", outputPath)
	return nil
}

// ValidateConfig checks if the Azure configuration is valid
func (a *AzureSpeechService) ValidateConfig() error {
	if a.config.SubscriptionKey == "" {
		return fmt.Errorf("Azure subscription key is required")
	}
	if a.config.Region == "" {
		return fmt.Errorf("Azure region is required")
	}
	if a.config.Language == "" {
		return fmt.Errorf("language is required")
	}
	return nil
}

// TestConnection tests the connection to Azure Speech Services
func (a *AzureSpeechService) TestConnection() error {
	log.Println("Testing Azure Speech Services connection...")

	// Create a simple test request to verify credentials
	apiURL := fmt.Sprintf("https://%s.stt.speech.microsoft.com/speech/recognition/conversation/cognitiveservices/v1?language=%s",
		a.config.Region, a.config.Language)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create test request: %v", err)
	}

	req.Header.Set("Ocp-Apim-Subscription-Key", a.config.SubscriptionKey)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to Azure: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("invalid Azure subscription key")
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("Azure API error: %s", resp.Status)
	}

	log.Println("Azure Speech Services connection successful!")
	return nil
}
