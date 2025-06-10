package audio

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gordonklaus/portaudio"
)

const (
	// Audio settings
	SampleRate      = 16000 // 16kHz - optimal for speech recognition
	Channels        = 1     // Mono
	FramesPerBuffer = 1024

	// Memory management
	MaxRecordingDuration = 60 * time.Second // 60 second max recording
	TempFilePrefix       = "voice_assistant_"
	TempFileExt          = ".wav"
)

// Recorder handles audio recording with memory management
type Recorder struct {
	stream       *portaudio.Stream
	isRecording  bool
	mutex        sync.Mutex
	tempFilePath string
	audioBuffer  []int16
	file         *os.File
	startTime    time.Time
	onComplete   func(filePath string, duration time.Duration)
	onError      func(error)
}

// NewRecorder creates a new audio recorder
func NewRecorder() *Recorder {
	return &Recorder{
		isRecording: false,
		audioBuffer: make([]int16, FramesPerBuffer),
	}
}

// Initialize sets up PortAudio
func (r *Recorder) Initialize() error {
	log.Println("Initializing audio system...")
	return portaudio.Initialize()
}

// Cleanup shuts down PortAudio
func (r *Recorder) Cleanup() {
	log.Println("Cleaning up audio system...")
	portaudio.Terminate()
}

// SetCallbacks sets the completion and error callbacks
func (r *Recorder) SetCallbacks(onComplete func(string, time.Duration), onError func(error)) {
	r.onComplete = onComplete
	r.onError = onError
}

// StartRecording begins audio recording to a temporary file
func (r *Recorder) StartRecording() error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.isRecording {
		return fmt.Errorf("already recording")
	}

	// Create temporary file
	tempDir := os.TempDir()
	timestamp := time.Now().Format("20060102_150405")
	r.tempFilePath = filepath.Join(tempDir, fmt.Sprintf("%s%s%s", TempFilePrefix, timestamp, TempFileExt))

	// Create and open the temp file
	file, err := os.Create(r.tempFilePath)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %v", err)
	}
	r.file = file

	// Write WAV header (we'll update it when recording stops)
	err = r.writeWAVHeader(r.file, 0) // 0 size for now
	if err != nil {
		r.file.Close()
		os.Remove(r.tempFilePath)
		return fmt.Errorf("failed to write WAV header: %v", err)
	}

	// Set up PortAudio stream
	stream, err := portaudio.OpenDefaultStream(
		Channels, // input channels
		0,        // output channels
		float64(SampleRate),
		FramesPerBuffer,
		r.processAudio,
	)
	if err != nil {
		r.file.Close()
		os.Remove(r.tempFilePath)
		return fmt.Errorf("failed to open audio stream: %v", err)
	}

	r.stream = stream
	r.isRecording = true
	r.startTime = time.Now()

	// Start the stream
	err = stream.Start()
	if err != nil {
		r.cleanup()
		return fmt.Errorf("failed to start audio stream: %v", err)
	}

	log.Printf("Started recording to: %s", r.tempFilePath)

	// Start monitoring for max duration
	go r.monitorDuration()

	return nil
}

// StopRecording stops audio recording and finalizes the file
func (r *Recorder) StopRecording() error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if !r.isRecording {
		return fmt.Errorf("not currently recording")
	}

	duration := time.Since(r.startTime)
	log.Printf("Stopping recording after %v", duration)

	// Stop and cleanup
	err := r.cleanup()
	if err != nil {
		return err
	}

	// Finalize the WAV file with correct header
	err = r.finalizeWAVFile()
	if err != nil {
		os.Remove(r.tempFilePath)
		return fmt.Errorf("failed to finalize WAV file: %v", err)
	}

	// Call completion callback
	if r.onComplete != nil {
		go r.onComplete(r.tempFilePath, duration)
	}

	return nil
}

// IsRecording returns current recording state
func (r *Recorder) IsRecording() bool {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return r.isRecording
}

// processAudio handles incoming audio data
func (r *Recorder) processAudio(in []int16) {
	if !r.isRecording || r.file == nil {
		return
	}

	// Write audio data directly to file (streaming approach)
	// Convert int16 to bytes and write
	for _, sample := range in {
		// Write little-endian 16-bit samples
		r.file.Write([]byte{byte(sample), byte(sample >> 8)})
	}
}

// monitorDuration stops recording if it exceeds max duration
func (r *Recorder) monitorDuration() {
	time.Sleep(MaxRecordingDuration)

	if r.IsRecording() {
		log.Println("Recording exceeded max duration, stopping...")
		err := r.StopRecording()
		if err != nil && r.onError != nil {
			r.onError(fmt.Errorf("failed to stop recording after timeout: %v", err))
		}
	}
}

// cleanup handles stream and file cleanup
func (r *Recorder) cleanup() error {
	if r.stream != nil {
		r.stream.Stop()
		r.stream.Close()
		r.stream = nil
	}

	if r.file != nil {
		r.file.Close()
		r.file = nil
	}

	r.isRecording = false
	return nil
}

// finalizeWAVFile updates the WAV header with correct file size
func (r *Recorder) finalizeWAVFile() error {
	// Get file size
	fileInfo, err := os.Stat(r.tempFilePath)
	if err != nil {
		return err
	}

	dataSize := int(fileInfo.Size()) - 44 // Subtract WAV header size

	// Rewrite the file with correct header
	file, err := os.OpenFile(r.tempFilePath, os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	// Seek to beginning and write correct header
	file.Seek(0, 0)
	return r.writeWAVHeader(file, dataSize)
}

// writeWAVHeader writes a WAV file header
func (r *Recorder) writeWAVHeader(file *os.File, dataSize int) error {
	fileSize := dataSize + 36

	// Helper function to convert int32 to little-endian bytes
	int32ToBytes := func(val int32) []byte {
		return []byte{
			byte(val),
			byte(val >> 8),
			byte(val >> 16),
			byte(val >> 24),
		}
	}

	// Helper function to convert int16 to little-endian bytes
	int16ToBytes := func(val int16) []byte {
		return []byte{
			byte(val),
			byte(val >> 8),
		}
	}

	// Build header piece by piece
	var header []byte

	// RIFF header
	header = append(header, 'R', 'I', 'F', 'F')
	header = append(header, int32ToBytes(int32(fileSize))...)
	header = append(header, 'W', 'A', 'V', 'E')

	// fmt chunk
	header = append(header, 'f', 'm', 't', ' ')
	header = append(header, int32ToBytes(16)...)                  // chunk size
	header = append(header, int16ToBytes(1)...)                   // PCM format
	header = append(header, int16ToBytes(int16(Channels))...)     // channels
	header = append(header, int32ToBytes(int32(SampleRate))...)   // sample rate
	header = append(header, int32ToBytes(int32(SampleRate*2))...) // byte rate
	header = append(header, int16ToBytes(2)...)                   // block align
	header = append(header, int16ToBytes(16)...)                  // bits per sample

	// data chunk
	header = append(header, 'd', 'a', 't', 'a')
	header = append(header, int32ToBytes(int32(dataSize))...)

	_, err := file.Write(header)
	return err
}

// DeleteTempFile safely removes the temporary audio file
func (r *Recorder) DeleteTempFile() error {
	if r.tempFilePath == "" {
		return nil
	}

	err := os.Remove(r.tempFilePath)
	if err != nil {
		log.Printf("Warning: Failed to delete temp file %s: %v", r.tempFilePath, err)
		return err
	}

	log.Printf("Deleted temp file: %s", r.tempFilePath)
	r.tempFilePath = ""
	return nil
}
