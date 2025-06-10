package speech

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gordonklaus/portaudio"
	"github.com/gorilla/websocket"
)

// WebSocket message types for Azure Speech Service
const (
	AudioMessageType     = "audio"
	SpeechMessageType    = "speech.endDetected"
	SpeechStartedType    = "speech.startDetected"
	SpeechHypothesisType = "speech.hypothesis"
	SpeechPhraseType     = "speech.phrase"
	TurnEndType          = "turn.end"
)

// AzureWebSocketSpeechService handles real-time speech recognition via WebSocket
type AzureWebSocketSpeechService struct {
	subscriptionKey string
	region          string
	language        string

	// WebSocket connection
	conn           *websocket.Conn
	isConnected    bool
	isListening    bool
	isShuttingDown bool // Flag to prevent error logging during shutdown
	mutex          sync.Mutex

	// Audio recording
	stream       *portaudio.Stream
	audioBuffer  []int16
	onRecognized func(text string)
	onError      func(error)

	// Audio settings
	sampleRate      int
	channels        int
	framesPerBuffer int

	// Connection tracking
	requestId    string
	connectionId string
}

// Audio configuration
const (
	SampleRate      = 16000 // 16kHz for speech recognition
	Channels        = 1     // Mono
	FramesPerBuffer = 1024
	MaxDuration     = 60 * time.Second // Max recording duration
)

// Azure WebSocket protocol messages
type SpeechConfigMessage struct {
	Context struct {
		System struct {
			Version string `json:"version"`
		} `json:"system"`
		OS struct {
			Platform string `json:"platform"`
			Name     string `json:"name"`
			Version  string `json:"version"`
		} `json:"os"`
		Device struct {
			Manufacturer string `json:"manufacturer"`
			Model        string `json:"model"`
			Version      string `json:"version"`
		} `json:"device"`
	} `json:"context"`
}

type SpeechResultMessage struct {
	Path              string `json:"Path"`
	RequestId         string `json:"RequestId"`
	RecognitionStatus string `json:"RecognitionStatus"`
	DisplayText       string `json:"DisplayText"` // Top-level DisplayText field
	Text              string `json:"Text"`        // For hypothesis messages
	Offset            int64  `json:"Offset"`
	Duration          int64  `json:"Duration"`
	NBest             []struct {
		Display string `json:"Display"`
	} `json:"NBest"`
}

// NewAzureWebSocketSpeechService creates a new WebSocket-based speech service
func NewAzureWebSocketSpeechService(subscriptionKey, region, language string) (*AzureWebSocketSpeechService, error) {
	service := &AzureWebSocketSpeechService{
		subscriptionKey: subscriptionKey,
		region:          region,
		language:        language,
		sampleRate:      SampleRate,
		channels:        Channels,
		framesPerBuffer: FramesPerBuffer,
		requestId:       generateRequestId(),
	}

	// Initialize PortAudio
	err := portaudio.Initialize()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize PortAudio: %v", err)
	}

	log.Printf("🌐 WebSocket Speech Service initialized")
	log.Printf("   🔧 Sample Rate: %d Hz", SampleRate)
	log.Printf("   🎙️  Channels: %d (Mono)", Channels)
	log.Printf("   📡 Region: %s", region)
	log.Printf("   🗣️  Language: %s", language)

	return service, nil
}

// SetCallbacks sets the recognition and error callbacks
func (a *AzureWebSocketSpeechService) SetCallbacks(onRecognized func(string), onError func(error)) {
	a.onRecognized = onRecognized
	a.onError = onError
}

// StartContinuousRecognition starts WebSocket connection and live audio streaming
func (a *AzureWebSocketSpeechService) StartContinuousRecognition() error {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if a.isListening {
		log.Printf("⚠️  Already listening - ignoring start request")
		return nil
	}

	log.Printf("🔌 CONNECTING TO AZURE WEBSOCKET...")

	// Connect to Azure WebSocket
	err := a.connectWebSocket()
	if err != nil {
		return fmt.Errorf("failed to connect to WebSocket: %v", err)
	}

	// Start audio capture
	err = a.startAudioCapture()
	if err != nil {
		a.disconnectWebSocket()
		return fmt.Errorf("failed to start audio capture: %v", err)
	}

	a.isListening = true
	log.Printf("🟢 LIVE STREAMING ACTIVE - Speak now!")
	log.Printf("   💡 Audio is being streamed in real-time to Azure")
	log.Printf("   💡 You should see recognition results as you speak")

	// Start goroutines for message handling and audio streaming
	go a.handleWebSocketMessages()
	go a.handleAudioStreaming()

	return nil
}

// connectWebSocket establishes WebSocket connection to Azure
func (a *AzureWebSocketSpeechService) connectWebSocket() error {
	// Build WebSocket URL
	u := url.URL{
		Scheme: "wss",
		Host:   fmt.Sprintf("%s.stt.speech.microsoft.com", a.region),
		Path:   "/speech/recognition/conversation/cognitiveservices/v1",
		RawQuery: fmt.Sprintf("language=%s&format=detailed&Ocp-Apim-Subscription-Key=%s",
			url.QueryEscape(a.language), url.QueryEscape(a.subscriptionKey)),
	}

	log.Printf("📡 Connecting to: %s", u.String())

	// Set up headers
	headers := http.Header{}
	headers.Set("Ocp-Apim-Subscription-Key", a.subscriptionKey)

	// Connect
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), headers)
	if err != nil {
		return fmt.Errorf("WebSocket dial failed: %v", err)
	}

	a.conn = conn
	a.isConnected = true
	a.connectionId = generateRequestId()

	log.Printf("✅ WebSocket connected successfully")
	log.Printf("   🔗 Connection ID: %s", a.connectionId)

	// Send initial configuration
	return a.sendSpeechConfig()
}

// sendSpeechConfig sends initial configuration to Azure
func (a *AzureWebSocketSpeechService) sendSpeechConfig() error {
	config := SpeechConfigMessage{}
	config.Context.System.Version = "1.0.0"
	config.Context.OS.Platform = "Windows"
	config.Context.OS.Name = "Windows"
	config.Context.OS.Version = "10.0"
	config.Context.Device.Manufacturer = "GoApp"
	config.Context.Device.Model = "VoiceAssistant"
	config.Context.Device.Version = "1.0"

	configBytes, err := json.Marshal(config)
	if err != nil {
		return err
	}

	// Send as text message with headers
	message := fmt.Sprintf("Path: speech.config\r\nContent-Type: application/json; charset=utf-8\r\nX-RequestId: %s\r\nX-Timestamp: %s\r\n\r\n%s",
		a.requestId, time.Now().UTC().Format("2006-01-02T15:04:05.000Z"), string(configBytes))

	log.Printf("📤 Sending speech config...")
	return a.conn.WriteMessage(websocket.TextMessage, []byte(message))
}

// startAudioCapture begins capturing audio from microphone
func (a *AzureWebSocketSpeechService) startAudioCapture() error {
	// Reset audio buffer
	a.audioBuffer = make([]int16, 0)

	// Set up PortAudio stream
	stream, err := portaudio.OpenDefaultStream(
		a.channels, // input channels
		0,          // output channels (no output)
		float64(a.sampleRate),
		a.framesPerBuffer,
		a.processAudio,
	)
	if err != nil {
		return fmt.Errorf("failed to open audio stream: %v", err)
	}

	a.stream = stream

	// Start the stream
	err = stream.Start()
	if err != nil {
		a.cleanup()
		return fmt.Errorf("failed to start audio stream: %v", err)
	}

	log.Printf("🎤 Audio capture started")
	return nil
}

// processAudio handles incoming audio data from microphone
func (a *AzureWebSocketSpeechService) processAudio(in []int16) {
	if !a.isListening || !a.isConnected {
		return
	}

	// Add incoming audio to buffer
	a.audioBuffer = append(a.audioBuffer, in...)

	// Log audio activity
	var sum int64
	for _, sample := range in {
		if sample < 0 {
			sum += int64(-sample)
		} else {
			sum += int64(sample)
		}
	}
	avgAmplitude := sum / int64(len(in))

	if avgAmplitude > 800 { // Threshold for speech detection
		log.Printf("🔊 Audio detected (amplitude: %d)", avgAmplitude)
	}
}

// handleAudioStreaming sends audio chunks to Azure via WebSocket
func (a *AzureWebSocketSpeechService) handleAudioStreaming() {
	log.Printf("🎵 Starting audio streaming handler...")

	ticker := time.NewTicker(100 * time.Millisecond) // Send audio every 100ms
	defer ticker.Stop()

	maxDuration := time.NewTimer(MaxDuration)
	defer maxDuration.Stop()

	for {
		select {
		case <-ticker.C:
			if !a.isListening || !a.isConnected {
				log.Printf("🛑 Audio streaming stopped")
				return
			}

			// Send accumulated audio
			if len(a.audioBuffer) > 0 {
				err := a.sendAudioChunk(a.audioBuffer)
				if err != nil {
					log.Printf("❌ Failed to send audio chunk: %v", err)
					if a.onError != nil {
						a.onError(err)
					}
					return
				}
				a.audioBuffer = make([]int16, 0) // Reset buffer
			}

		case <-maxDuration.C:
			log.Printf("⏰ Max streaming duration reached, stopping...")
			a.StopContinuousRecognition()
			return
		}
	}
}

// sendAudioChunk sends audio data to Azure via WebSocket
func (a *AzureWebSocketSpeechService) sendAudioChunk(audioData []int16) error {
	if len(audioData) == 0 {
		return nil
	}

	// Convert to bytes (16-bit PCM, little-endian)
	audioBytes := make([]byte, len(audioData)*2)
	for i, sample := range audioData {
		binary.LittleEndian.PutUint16(audioBytes[i*2:], uint16(sample))
	}

	// Create proper headers for Azure WebSocket protocol
	// Headers must be lowercase and follow exact format
	headers := fmt.Sprintf("path:audio\r\nx-requestid:%s\r\nx-timestamp:%s\r\n\r\n",
		a.requestId, time.Now().UTC().Format("2006-01-02T15:04:05.000Z"))

	headerBytes := []byte(headers)
	headerLength := uint16(len(headerBytes))

	// Build the complete message: [2-byte header length BIG-ENDIAN][header][audio data]
	message := make([]byte, 2+len(headerBytes)+len(audioBytes))

	// Write header length in BIG-ENDIAN format (this was the issue!)
	binary.BigEndian.PutUint16(message[0:2], headerLength)

	// Write header
	copy(message[2:2+len(headerBytes)], headerBytes)

	// Write audio data
	copy(message[2+len(headerBytes):], audioBytes)

	// Send as binary message
	return a.conn.WriteMessage(websocket.BinaryMessage, message)
}

// handleWebSocketMessages processes incoming messages from Azure
func (a *AzureWebSocketSpeechService) handleWebSocketMessages() {
	log.Printf("📬 Starting WebSocket message handler...")

	for a.isConnected {
		messageType, data, err := a.conn.ReadMessage()
		if err != nil {
			// Only log errors if we're not intentionally shutting down
			if !a.isShuttingDown && !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Printf("❌ WebSocket read error: %v", err)
				if a.onError != nil {
					a.onError(err)
				}
			}
			break
		}

		switch messageType {
		case websocket.TextMessage:
			a.handleTextMessage(data)
		case websocket.BinaryMessage:
			log.Printf("📦 Received binary message (%d bytes)", len(data))
		}
	}

	log.Printf("📬 Message handler stopped")
}

// handleTextMessage processes text messages from Azure
func (a *AzureWebSocketSpeechService) handleTextMessage(data []byte) {
	//message := string(data)

	// Parse message headers and body
	parts := bytes.Split(data, []byte("\r\n\r\n"))
	if len(parts) < 2 {
		return
	}

	headers := string(parts[0])
	body := parts[1]

	// Only process speech.phrase messages (final results)
	if bytes.Contains([]byte(headers), []byte("Path:speech.phrase")) {
		// Parse the JSON body
		var result SpeechResultMessage
		err := json.Unmarshal(body, &result)
		if err != nil {
			log.Printf("❌ Failed to parse speech phrase: %v", err)
			return
		}

		// Check if recognition was successful and we have text
		if result.RecognitionStatus == "Success" {
			var finalText string

			// Try DisplayText first (top-level field)
			if result.DisplayText != "" {
				finalText = result.DisplayText
			} else if len(result.NBest) > 0 && result.NBest[0].Display != "" {
				// Fallback to NBest[0].Display
				finalText = result.NBest[0].Display
			}

			if finalText != "" {
				log.Printf("🎯 FINAL RESULT: '%s'", finalText)
				log.Printf("   📤 Sending to Claude API...")

				// Call the recognition callback with the final text
				if a.onRecognized != nil {
					a.onRecognized(finalText)
				}
			}
		} else {
			log.Printf("🔇 No speech recognized (status: %s)", result.RecognitionStatus)
		}
	}
	// Ignore all other message types (hypothesis, speech start/end, etc.)
}

// StopContinuousRecognition stops WebSocket connection and audio capture
func (a *AzureWebSocketSpeechService) StopContinuousRecognition() error {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if !a.isListening {
		log.Printf("⚠️  Not currently listening - ignoring stop request")
		return nil
	}

	log.Printf("🛑 STOPPING LIVE STREAMING...")
	a.isShuttingDown = true // Set flag to prevent error logging during shutdown

	// Send end of audio signal (empty audio chunk with proper format)
	if a.isConnected && a.conn != nil {
		headers := fmt.Sprintf("path:audio\r\nx-requestid:%s\r\nx-timestamp:%s\r\n\r\n",
			a.requestId, time.Now().UTC().Format("2006-01-02T15:04:05.000Z"))

		headerBytes := []byte(headers)
		headerLength := uint16(len(headerBytes))

		// Create end-of-stream message (header only, no audio data)
		message := make([]byte, 2+len(headerBytes))
		binary.BigEndian.PutUint16(message[0:2], headerLength) // BIG-ENDIAN
		copy(message[2:], headerBytes)

		a.conn.WriteMessage(websocket.BinaryMessage, message)
	}

	// Stop audio capture first
	err := a.cleanup()
	if err != nil {
		log.Printf("❌ Error during cleanup: %v", err)
	}

	// Close WebSocket connection
	a.disconnectWebSocket()
	a.isListening = false
	a.isShuttingDown = false // Reset flag

	log.Printf("🔴 STREAMING STOPPED")
	return nil
}

// cleanup handles audio stream cleanup
func (a *AzureWebSocketSpeechService) cleanup() error {
	if a.stream != nil {
		a.stream.Stop()
		a.stream.Close()
		a.stream = nil
	}
	return nil
}

// disconnectWebSocket closes the WebSocket connection
func (a *AzureWebSocketSpeechService) disconnectWebSocket() {
	if a.conn != nil {
		a.conn.Close()
		a.conn = nil
	}
	a.isConnected = false
	log.Printf("🔌 WebSocket disconnected")
}

// IsListening returns whether continuous recognition is active
func (a *AzureWebSocketSpeechService) IsListening() bool {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	return a.isListening
}

// TestConnection tests the Azure Speech Services connection
func (a *AzureWebSocketSpeechService) TestConnection() error {
	log.Printf("🧪 TESTING AZURE WEBSOCKET CONNECTION...")
	log.Printf("   🌐 Region: %s", a.region)
	log.Printf("   🗣️  Language: %s", a.language)
	log.Printf("   🔑 Key configured: %t", a.subscriptionKey != "")

	// Try to establish WebSocket connection
	err := a.connectWebSocket()
	if err != nil {
		return fmt.Errorf("connection test failed: %v", err)
	}

	// Close test connection
	a.disconnectWebSocket()

	log.Printf("   ✅ Connection test successful!")
	log.Printf("   🎉 Azure WebSocket Speech Services is ready!")
	return nil
}

// Close releases all resources
func (a *AzureWebSocketSpeechService) Close() {
	log.Printf("🧹 Cleaning up WebSocket Speech Service...")

	if a.isListening {
		a.StopContinuousRecognition()
	}

	a.disconnectWebSocket()
	portaudio.Terminate()
	log.Printf("✅ Cleanup completed")
}

// generateRequestId creates a proper UUID/GUID for Azure
func generateRequestId() string {
	// Generate a proper UUID v4 format: xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
	b := make([]byte, 16)
	rand.Read(b)

	// Set version (4) and variant bits according to RFC 4122
	b[6] = (b[6] & 0x0f) | 0x40 // Version 4
	b[8] = (b[8] & 0x3f) | 0x80 // Variant 10

	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
