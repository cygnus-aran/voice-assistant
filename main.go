package main

import (
	_ "fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
	"voice-assistant/internal/claude"

	"github.com/gen2brain/beeep"
	"github.com/getlantern/systray"
	"github.com/getlantern/systray/example/icon"

	"voice-assistant/config"
	"voice-assistant/internal/hotkey"
	"voice-assistant/internal/speech"
)

var (
	hotkeyListener       *hotkey.Listener
	azureSpeechWebSocket *speech.AzureWebSocketSpeechService
	appConfig            *config.Config
	claudeClient         *claude.Client
	currentStatus        = "Ready"
	isRecording          = false
)

func main() {
	// Load configuration from params.json
	var err error
	appConfig, err = config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Display config status
	log.Printf("📁 Config file: %s", config.GetConfigPath())

	// Check Azure configuration
	if !appConfig.Azure.IsConfigured() {
		log.Printf("⚠️  Azure Speech Services not configured")
		log.Printf("   Add your subscription_key and region to: %s", config.GetConfigPath())
	} else {
		log.Printf("✅ Azure Speech Services configured (Region: %s)", appConfig.Azure.Region)
	}

	// Check Claude configuration
	if !appConfig.Claude.IsConfigured() {
		log.Printf("⚠️  Claude API not configured")
		log.Printf("   Add your api_key to: %s", config.GetConfigPath())
	} else {
		claudeClient = claude.NewClientFromConfig(appConfig)

		// Test connection
		err = claudeClient.TestConnection()
		if err != nil {
			log.Printf("❌ Claude connection test failed: %v", err)
		} else {
			log.Println("✅ Claude API connection successful!")
		}
	}

	// Set up graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	// Initialize Azure WebSocket Speech Service
	if appConfig.Azure.IsConfigured() {
		azureSpeechWebSocket, err = speech.NewAzureWebSocketSpeechService(
			appConfig.Azure.SubscriptionKey,
			appConfig.Azure.Region,
			appConfig.Azure.Language,
		)
		if err != nil {
			log.Printf("❌ Failed to initialize Azure WebSocket Speech: %v", err)
		} else {
			// Set callbacks for speech recognition
			azureSpeechWebSocket.SetCallbacks(onSpeechRecognized, onSpeechError)

			// Test connection
			err = azureSpeechWebSocket.TestConnection()
			if err != nil {
				log.Printf("❌ Azure WebSocket connection test failed: %v", err)
			} else {
				log.Println("✅ Azure WebSocket Speech Service connection successful!")
			}
		}
	}

	// Initialize hotkey listener
	hotkeyListener = hotkey.NewListener(onF12Pressed, onCtrlQPressed)

	// Start hotkey listener
	hotkeyListener.Start()

	// Handle shutdown
	go func() {
		<-c
		log.Println("Shutting down...")
		if hotkeyListener != nil {
			hotkeyListener.Stop()
		}
		if azureSpeechWebSocket != nil {
			azureSpeechWebSocket.Close()
		}
		systray.Quit()
	}()

	// Start the system tray
	systray.Run(onReady, onExit)
}

// onCtrlQPressed handles Ctrl+Q key combination for graceful exit
func onCtrlQPressed() {
	err := beeep.Notify("AI Assistant", "👋 Shutting down...", "")
	if err != nil {
		log.Printf("Failed to show notification: %v", err)
	}

	// Give time for notification to show
	go func() {
		// Small delay to show the notification
		time.Sleep(500 * time.Millisecond)
		systray.Quit()
	}()
}

// onF12Pressed handles F12 key press events
func onF12Pressed() {
	log.Printf("🔑 F12 KEY PRESSED - Current recording state: %v", isRecording)

	if azureSpeechWebSocket == nil {
		log.Printf("❌ Azure WebSocket Speech not available")
		beeep.Notify("AI Assistant", "❌ Azure Speech Services not configured", "")
		return
	}

	if isRecording {
		// Stop recording
		log.Printf("🛑 USER REQUESTED STOP")
		updateStatus("Processing")
		err := beeep.Notify("AI Assistant", "🔴 Stopping recognition...", "")
		if err != nil {
			log.Printf("Failed to show notification: %v", err)
		}

		err = azureSpeechWebSocket.StopContinuousRecognition()
		if err != nil {
			log.Printf("❌ Failed to stop recognition: %v", err)
			updateStatus("Error")
			beeep.Notify("AI Assistant", "❌ Failed to stop recognition", "")
		} else {
			isRecording = false
			updateStatus("Ready")
			log.Printf("✅ Recording stopped successfully")
		}
	} else {
		// Start recording
		log.Printf("🎤 USER REQUESTED START")
		updateStatus("Listening")
		err := beeep.Notify("AI Assistant", "🎤 Streaming live... Press F12 to stop.", "")
		if err != nil {
			log.Printf("Failed to show notification: %v", err)
		}

		err = azureSpeechWebSocket.StartContinuousRecognition()
		if err != nil {
			log.Printf("❌ Failed to start recognition: %v", err)
			updateStatus("Error")
			beeep.Notify("AI Assistant", "❌ Failed to start recognition", "")
		} else {
			isRecording = true
			log.Printf("✅ Live streaming started successfully")
			log.Printf("💡 Now speak clearly - audio is streaming to Azure in real-time!")
		}
	}
	err := azureSpeechWebSocket.StartContinuousRecognition()
	if err != nil {
		log.Printf("❌ Failed to start recognition: %v", err)
		updateStatus("Error")
		beeep.Notify("AI Assistant", "❌ Failed to start recognition", "")
	} else {
		isRecording = true
		log.Printf("✅ Live streaming started successfully")
		log.Printf("💡 Now speak clearly - audio is streaming to Azure in real-time!")
	}
}

// Speech recognition callbacks
func onSpeechRecognized(text string) {
	log.Printf("🎉 SPEECH CALLBACK TRIGGERED")
	log.Printf("   📝 Recognized text: '%s'", text)
	log.Printf("   📏 Text length: %d characters", len(text))
	updateStatus("Processing")

	// Send transcription to Claude API
	if claudeClient != nil {
		updateStatus("Thinking")

		claudeResponse, err := claudeClient.SendMessage(text)
		if err != nil {
			log.Printf("Claude API failed: %v", err)
			updateStatus("Error")
			beeep.Notify("AI Assistant", "❌ Claude API failed", "")
		} else {
			log.Printf("Claude response: %s", claudeResponse)
			updateStatus("Ready")

			// TODO: Convert Claude's response to speech using TTS
			log.Printf("Converting to speech...")
		}
	} else {
		log.Println("Claude not configured - skipping AI processing")
		beeep.Notify("AI Assistant", "⚠️ Claude API not configured", "")
	}

	// Auto-stop after recognition for now
	go func() {
		log.Printf("⏰ Auto-stopping in 3 seconds...")
		time.Sleep(3 * time.Second)
		if isRecording {
			log.Printf("🔄 Auto-stopping recognition...")
			azureSpeechWebSocket.StopContinuousRecognition()
			isRecording = false
			updateStatus("Ready")
			log.Printf("✅ Auto-stop completed")
		}
	}()
}

func onSpeechError(err error) {
	log.Printf("🚨 SPEECH ERROR CALLBACK TRIGGERED")
	log.Printf("   ❌ Error details: %v", err)
	log.Printf("   💡 Check your microphone, internet connection, and Azure credentials")
	updateStatus("Error")
	beeep.Notify("AI Assistant", "❌ Speech recognition error", "")
	isRecording = false
}

func onReady() {
	// Set the system tray icon and tooltip
	systray.SetIcon(icon.Data) // Using example icon for now
	systray.SetTitle("AI Assistant")
	systray.SetTooltip("AI Desktop Assistant - Press F12 to start")

	// Create menu items
	mStatus := systray.AddMenuItem("Status: "+currentStatus, "Current assistant status")
	mStatus.Disable() // Make it non-clickable, just for display

	systray.AddSeparator()

	mSettings := systray.AddMenuItem("Settings", "Configure the assistant")
	mAbout := systray.AddMenuItem("About", "About AI Assistant")

	systray.AddSeparator()

	mQuit := systray.AddMenuItem("Quit", "Quit the assistant")

	// Show startup notification
	err := beeep.Notify("AI Assistant", "Assistant is ready!\nF12: Start/Stop recording\nCtrl+Q: Exit", "")
	if err != nil {
		log.Printf("Failed to show notification: %v", err)
	}

	// Handle menu clicks
	go func() {
		for {
			select {
			case <-mSettings.ClickedCh:
				err := beeep.Notify("Settings", "Settings panel would open here", "")
				if err != nil {
					log.Printf("Failed to show notification: %v", err)
				}

			case <-mAbout.ClickedCh:
				err := beeep.Notify("About", "AI Desktop Assistant v1.0\nBuilt with Go + Azure WebSocket Speech", "")
				if err != nil {
					log.Printf("Failed to show notification: %v", err)
				}

			case <-mQuit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
}

func onExit() {
	// Cleanup when the application exits
	log.Println("AI Assistant shutting down...")
}

// Helper function to update status
func updateStatus(status string) {
	currentStatus = status
	log.Printf("Status: %s", status)
	// TODO: Update the menu item text dynamically
}
