package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gen2brain/beeep"
	"github.com/getlantern/systray"
	"github.com/getlantern/systray/example/icon"

	"voice-assistant/config"
	"voice-assistant/internal/audio"
	"voice-assistant/internal/hotkey"
	"voice-assistant/internal/speech"
)

var (
	hotkeyListener *hotkey.Listener
	audioRecorder  *audio.Recorder
	speechService  *speech.AzureSpeechService
	appConfig      *config.Config
	currentStatus  = "Ready"
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
		log.Printf("✅ Claude API configured (Model: %s)", appConfig.Claude.Model)
	}

	// Set up graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	// Initialize audio recorder
	audioRecorder = audio.NewRecorder()
	err = audioRecorder.Initialize()
	if err != nil {
		log.Fatalf("Failed to initialize audio: %v", err)
	}
	defer audioRecorder.Cleanup()

	// Set audio callbacks
	audioRecorder.SetCallbacks(onRecordingComplete, onRecordingError)

	// Initialize Azure Speech Service
	if appConfig.Azure.IsConfigured() {
		speechConfig := speech.AzureConfig{
			SubscriptionKey: appConfig.Azure.SubscriptionKey,
			Region:          appConfig.Azure.Region,
			Language:        appConfig.Azure.Language,
		}
		speechService = speech.NewAzureSpeechService(speechConfig)

		// Test connection
		err = speechService.TestConnection()
		if err != nil {
			log.Printf("❌ Azure connection test failed: %v", err)
		} else {
			log.Println("✅ Azure Speech Services connection successful!")
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
		if audioRecorder != nil {
			audioRecorder.Cleanup()
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
	if audioRecorder.IsRecording() {
		// Stop recording
		log.Println("Stopping audio recording...")
		updateStatus("Processing")
		err := beeep.Notify("AI Assistant", "🔴 Stopping recording...", "")
		if err != nil {
			log.Printf("Failed to show notification: %v", err)
		}

		err = audioRecorder.StopRecording()
		if err != nil {
			log.Printf("Failed to stop recording: %v", err)
			updateStatus("Error")
			beeep.Notify("AI Assistant", "❌ Failed to stop recording", "")
		}
	} else {
		// Start recording
		log.Println("Starting audio recording...")
		updateStatus("Listening")
		err := beeep.Notify("AI Assistant", "🎤 Recording... Press F12 to stop.", "")
		if err != nil {
			log.Printf("Failed to show notification: %v", err)
		}

		err = audioRecorder.StartRecording()
		if err != nil {
			log.Printf("Failed to start recording: %v", err)
			updateStatus("Error")
			beeep.Notify("AI Assistant", "❌ Failed to start recording", "")
		}
	}
}

// Audio recording callbacks
func onRecordingComplete(filePath string, duration time.Duration) {
	log.Printf("Recording completed: %s (duration: %v)", filePath, duration)
	updateStatus("Processing")

	err := beeep.Notify("AI Assistant",
		fmt.Sprintf("🎤 Processing audio... Duration: %.1fs", duration.Seconds()), "")
	if err != nil {
		log.Printf("Failed to show notification: %v", err)
	}

	// Convert speech to text using Azure
	if speechService != nil {
		go func() {
			transcription, err := speechService.SpeechToText(filePath)
			if err != nil {
				log.Printf("Speech-to-text failed: %v", err)
				updateStatus("Error")
				beeep.Notify("AI Assistant", "❌ Speech recognition failed", "")
			} else {
				log.Printf("Transcription: %s", transcription)
				updateStatus("Ready")

				// Show transcription to user
				beeep.Notify("AI Assistant",
					fmt.Sprintf("✅ Transcription:\n%s", transcription), "")

				// TODO: Send to Claude API here
				log.Printf("TODO: Send to Claude: %s", transcription)
			}

			// Clean up the audio file
			audioRecorder.DeleteTempFile()
		}()
	} else {
		// No Azure configured, just clean up
		log.Println("Azure not configured - skipping speech recognition")
		updateStatus("Ready")
		beeep.Notify("AI Assistant", "⚠️ Azure Speech Services not configured", "")

		// Clean up after delay
		go func() {
			time.Sleep(2 * time.Second)
			audioRecorder.DeleteTempFile()
		}()
	}
}

func onRecordingError(err error) {
	log.Printf("Recording error: %v", err)
	updateStatus("Error")
	beeep.Notify("AI Assistant", "❌ Recording error occurred", "")
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
				err := beeep.Notify("About", "AI Desktop Assistant v1.0\nBuilt with Go", "")
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
