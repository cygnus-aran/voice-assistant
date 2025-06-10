package hotkey

import (
	"log"
	"syscall"
	"time"
)

const (
	VK_F12  = 0x7B
	VK_Q    = 0x51
	VK_CTRL = 0x11
)

var (
	user32           = syscall.NewLazyDLL("user32.dll")
	getAsyncKeyState = user32.NewProc("GetAsyncKeyState")
)

// Listener handles global hotkey detection
type Listener struct {
	onF12Pressed   func()
	onCtrlQPressed func()
	isRecording    bool
	stopChan       chan bool
	running        bool
}

// NewListener creates a new hotkey listener
func NewListener(onF12Pressed func(), onCtrlQPressed func()) *Listener {
	return &Listener{
		onF12Pressed:   onF12Pressed,
		onCtrlQPressed: onCtrlQPressed,
		isRecording:    false,
		stopChan:       make(chan bool, 1),
		running:        false,
	}
}

// isKeyPressed checks if a key is currently pressed
func isKeyPressed(vkCode int) bool {
	ret, _, _ := getAsyncKeyState.Call(uintptr(vkCode))
	return (ret & 0x8000) != 0
}

// Start begins listening for hotkeys
func (l *Listener) Start() {
	log.Println("Starting hotkey listener for F12 and Ctrl+Q...")
	l.running = true

	// Start the polling loop in a goroutine
	go func() {
		var lastF12State, lastCtrlQState bool

		for l.running {
			// Check F12 key
			currentF12State := isKeyPressed(VK_F12)
			if currentF12State && !lastF12State {
				log.Println("F12 key pressed!")

				// Always call the callback - let the main app decide what to do
				if l.onF12Pressed != nil {
					l.onF12Pressed()
				}
			}
			lastF12State = currentF12State

			// Check Ctrl+Q combination
			ctrlPressed := isKeyPressed(VK_CTRL)
			qPressed := isKeyPressed(VK_Q)
			currentCtrlQState := ctrlPressed && qPressed

			if currentCtrlQState && !lastCtrlQState {
				log.Println("Ctrl+Q pressed! Exiting application...")
				if l.onCtrlQPressed != nil {
					l.onCtrlQPressed()
				}
			}
			lastCtrlQState = currentCtrlQState

			time.Sleep(50 * time.Millisecond) // Poll every 50ms
		}
	}()
}

// Stop stops the hotkey listener
func (l *Listener) Stop() {
	log.Println("Stopping hotkey listener...")
	l.running = false
	l.stopChan <- true
}

// IsRecording returns the current recording state
func (l *Listener) IsRecording() bool {
	return l.isRecording
}

// SetRecording manually sets the recording state
func (l *Listener) SetRecording(recording bool) {
	l.isRecording = recording
}
