package audio

import (
	"cyberspec/viz"
	"encoding/binary"
	"fmt"
	"math"

	pulse "github.com/mesilliac/pulse-simple"
)

// Capturer handles audio capture from PipeWire/PulseAudio
type Capturer struct {
	stream     *pulse.Stream
	byteBuffer []byte
}

// NewCapturer creates a new audio capturer
//
// AUDIO PIPELINE (on Linux with PipeWire):
//
//   Application Audio → PipeWire → PulseAudio Compatibility Layer
//                                 ↓
//                           Monitor Source (capture copy of output)
//                                 ↓
//                           Our Application (cyberspec)
//                                 ↓
//                           FFT → Visualization
//
// WHY MONITOR SOURCE?
//   - "Monitor" = copy of audio being played
//   - Captures system audio output (music, videos, games)
//   - Non-intrusive (doesn't affect playback)
//   - Standard method for spectrum analyzers
//
// PULSEAUDIO API:
//   - PipeWire provides PulseAudio compatibility
//   - We use pulse-simple (simplified API)
//   - Works transparently with PipeWire
//
// SAMPLE FORMAT:
//   - Float32LE: 32-bit float, little-endian
//   - Stereo: 2 channels (left, right)
//   - We'll mix to mono for visualization
//
// Returns:
//   *Capturer - Initialized capturer ready to capture audio
//   error     - Error if initialization fails
func NewCapturer() (*Capturer, error) {
	// PulseAudio stream specification
	ss := pulse.SampleSpec{
		Format:   pulse.SAMPLE_FLOAT32LE,  // 32-bit float
		Rate:     uint32(viz.SAMPLE_RATE), // 48000 Hz
		Channels: 2,                       // Stereo
	}

	// Create stream
	// pulse-simple API: Capture(appName, streamName, sampleSpec)
	stream, err := pulse.Capture(
		"CyberSpec",         // Application name
		"Spectrum Analyzer", // Stream description
		&ss,                 // Sample specification
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create PulseAudio stream: %w", err)
	}

	// Create byte buffer for captured samples
	// Size: BUFFER_SIZE samples × 2 channels × 4 bytes per float32
	bufferSize := viz.BUFFER_SIZE * 2 * 4
	byteBuffer := make([]byte, bufferSize)

	return &Capturer{
		stream:     stream,
		byteBuffer: byteBuffer,
	}, nil
}

// ReadSamples reads audio samples from the capture stream
//
// OPERATION:
//   1. Read stereo samples from PulseAudio as bytes
//   2. Convert bytes to float32 (little-endian)
//   3. Mix stereo to mono (average left and right)
//   4. Return mono samples for FFT processing
//
// WHY MIX TO MONO?
//   - Spectrum analyzer shows frequency content, not stereo position
//   - Mixing prevents phase cancellation issues
//   - Simpler FFT processing
//   - Still represents all frequencies from both channels
//
// ALTERNATIVE: Stereo visualization
//   Could process left/right separately for stereo spectrum
//   See docs/IMPLEMENTATION_PLAN.md "Future Enhancements"
//
// Returns:
//   []float64 - Mono audio samples (BUFFER_SIZE samples)
//   error     - Error if read fails
func (c *Capturer) ReadSamples() ([]float64, error) {
	// Read bytes from stream
	_, err := c.stream.Read(c.byteBuffer)
	if err != nil {
		return nil, fmt.Errorf("failed to read audio: %w", err)
	}

	// Convert bytes to float32 samples and mix to mono
	mono := make([]float64, viz.BUFFER_SIZE)

	for i := 0; i < viz.BUFFER_SIZE; i++ {
		// Each sample is 4 bytes (float32), stereo has 2 channels
		// Byte positions: [L0L0L0L0 R0R0R0R0 L1L1L1L1 R1R1R1R1 ...]
		leftOffset := i * 2 * 4       // Left channel byte offset
		rightOffset := (i*2 + 1) * 4 // Right channel byte offset

		// Convert bytes to float32 (little-endian)
		left := math.Float32frombits(binary.LittleEndian.Uint32(c.byteBuffer[leftOffset : leftOffset+4]))
		right := math.Float32frombits(binary.LittleEndian.Uint32(c.byteBuffer[rightOffset : rightOffset+4]))

		// Mix to mono (average left and right)
		mono[i] = (float64(left) + float64(right)) / 2.0
	}

	return mono, nil
}

// Close closes the audio stream
func (c *Capturer) Close() {
	if c.stream != nil {
		c.stream.Free()
	}
}

// TROUBLESHOOTING AUDIO ISSUES:
//
// Problem: No audio captured
// Solutions:
//   1. Check PipeWire is running:
//      ps aux | grep pipewire
//
//   2. Check default sink has a monitor:
//      pactl list sources | grep monitor
//
//   3. Check audio is actually playing:
//      paplay /usr/share/sounds/alsa/Front_Center.wav
//
//   4. List all sources (find monitor source name):
//      pactl list sources short
//
//   5. Specify monitor source explicitly:
//      In NewCapturer(), replace nil with source name:
//      pulse.Capture("CyberSpec", "...", "alsa_output.pci-0000_00_1f.3.analog-stereo.monitor", ...)
//
// Problem: Choppy/stuttering audio
// Solutions:
//   1. Increase buffer size (trade latency for stability)
//   2. Check CPU usage (spectrum analyzer shouldn't use >5%)
//   3. Check for buffer underruns in PipeWire logs
//
// Problem: Low/no visualization despite audio playing
// Solutions:
//   1. Increase gain with + key
//   2. Check A-weighting isn't over-reducing (disable in config)
//   3. Verify audio stream is actually captured (add debug logging)
//
// ADVANCED: SPECIFYING AUDIO SOURCE
//
// To capture from specific device instead of default monitor:
//
// 1. Find available sources:
//    pactl list sources short
//
// 2. Modify NewCapturer():
//    sourceName := "alsa_output.usb-Device_Name.monitor"
//    stream, err := pulse.Capture(
//        "CyberSpec",
//        "Spectrum Analyzer",
//        sourceName,  // Specify source
//        &ss,
//        nil,
//        &attr,
//    )
//
// ALTERNATIVE LIBRARIES:
//
// If pulse-simple doesn't work:
//
// 1. github.com/lawl/pulseaudio
//    - More features
//    - More complex API
//
// 2. github.com/jfreymuth/pulse
//    - Modern, well-maintained
//    - Good documentation
//
// 3. Direct PipeWire API
//    - Most control
//    - Most complex
//    - Requires pipewire-go bindings
//
// PERFORMANCE NOTES:
//
// Audio capture is typically very cheap:
//   - <1% CPU usage
//   - Minimal memory (just buffer)
//   - PipeWire/PulseAudio handles the heavy lifting
//
// Buffer size affects latency:
//   - 1600 samples at 48kHz = ~33ms latency
//   - Acceptable for visualization
//   - Could reduce for lower latency (trade stability)
//
// FUTURE ENHANCEMENTS:
//
// 1. Stereo visualization:
//    Process left/right channels separately
//    Display side-by-side or overlapped
//
// 2. Multiple input sources:
//    Allow user to select audio source
//    Switch between devices
//
// 3. Recording mode:
//    Capture audio file instead of live stream
//    Process offline, generate video
//
// 4. Loopback mode:
//    Capture from specific application
//    Use PipeWire's advanced routing
//
// See docs/IMPLEMENTATION_PLAN.md for more ideas
