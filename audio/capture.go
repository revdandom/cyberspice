package audio

import (
	"cyberspec/viz"
	"encoding/binary"
	"fmt"
	"math"
	"os/exec"
	"strings"
	"sync"
	"time"

	pulse "github.com/mesilliac/pulse-simple"
)

// Capturer handles audio capture from PipeWire/PulseAudio
//
// LATENCY DESIGN:
//
// A dedicated goroutine (readLoop) does the blocking pulse reads back-to-back
// and publishes only the newest mono buffer. ReadSamples() returns that
// snapshot without blocking. This keeps the audio pipeline draining at full
// speed regardless of how long the render loop takes, so latency stays at
// ~one fragment instead of growing until it hits the server ring buffer.
//
// The capture stream is also created with an explicit pa_buffer_attr that
// caps Maxlength, so even a transient stall drops old samples (overrun)
// rather than accumulating multi-second lag.
type Capturer struct {
	stream *pulse.Stream

	mu       sync.Mutex
	historyL []float64 // rolling window, most recent viz.FFT_SIZE left samples
	historyR []float64 // ... right samples
	primed   bool      // true once at least one chunk has been read

	done      chan struct{} // closed by Close() to ask readLoop to stop
	stopped   chan struct{} // closed by readLoop when it has returned
	closeOnce sync.Once
}

// NewCapturer creates a new audio capturer
//
// AUDIO PIPELINE (on Linux with PipeWire):
//
//	Application Audio → PipeWire → PulseAudio Compatibility Layer
//	                              ↓
//	                        Monitor Source (capture copy of output)
//	                              ↓
//	                        Our Application (cyberspec)
//	                              ↓
//	                        FFT → Visualization
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
//
//	*Capturer - Initialized capturer ready to capture audio
//	error     - Error if initialization fails
func NewCapturer() (*Capturer, error) {
	// PulseAudio stream specification
	ss := pulse.SampleSpec{
		Format:   pulse.SAMPLE_FLOAT32LE,  // 32-bit float
		Rate:     uint32(viz.SAMPLE_RATE), // 48000 Hz
		Channels: 2,                       // Stereo
	}

	// Detect the monitor source of the current default sink so we capture
	// system playback rather than the microphone.
	monitorSource := detectMonitorSource()

	// One read chunk in bytes: BUFFER_SIZE samples × 2 channels × 4 bytes/float32.
	chunkBytes := viz.BUFFER_SIZE * 2 * 4

	// Explicit buffer attributes to keep capture latency bounded.
	// Fragsize: how much audio the server hands over per fragment (~16ms).
	// Maxlength: hard ceiling on the ring buffer (~130ms). Once full the
	//   server drops the oldest samples instead of letting lag accumulate.
	// Playback-only fields stay at the server default (^uint32(0)).
	battr := pulse.NewBufferAttr()
	battr.Fragsize = uint32(chunkBytes / 2)
	battr.Maxlength = uint32(chunkBytes * 4)

	// Create stream bound to the monitor source.
	// pulse-simple API:
	//   NewStream(server, clientName, direction, deviceName, streamName, spec, channelMap, bufferAttr)
	stream, err := pulse.NewStream(
		"",                  // server (empty = default)
		"CyberSpec",         // client name
		pulse.STREAM_RECORD, // direction: capture
		monitorSource,       // device name (monitor source)
		"Spectrum Analyzer", // stream description
		&ss,                 // sample specification
		nil,                 // channel map (default)
		battr,               // buffer attributes (latency cap)
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create PulseAudio stream: %w", err)
	}

	c := &Capturer{
		stream:   stream,
		historyL: make([]float64, viz.FFT_SIZE),
		historyR: make([]float64, viz.FFT_SIZE),
		done:     make(chan struct{}),
		stopped:  make(chan struct{}),
	}

	// Drain the capture stream continuously on its own goroutine.
	go c.readLoop(chunkBytes)

	return c, nil
}

// readLoop performs back-to-back blocking reads and slides each new chunk into
// the rolling history window. Runs until done is closed or the stream errors.
func (c *Capturer) readLoop(chunkBytes int) {
	defer close(c.stopped)

	byteBuffer := make([]byte, chunkBytes)
	chunkL := make([]float64, viz.BUFFER_SIZE)
	chunkR := make([]float64, viz.BUFFER_SIZE)

	for {
		select {
		case <-c.done:
			return
		default:
		}

		if _, err := c.stream.Read(byteBuffer); err != nil {
			// Stream freed or unrecoverable read error: stop the loop.
			// ReadSamples() will keep returning the last good window.
			return
		}

		// Split the interleaved stereo float32 stream into L / R.
		for i := 0; i < viz.BUFFER_SIZE; i++ {
			lo := i * 2 * 4
			ro := (i*2 + 1) * 4
			chunkL[i] = float64(math.Float32frombits(binary.LittleEndian.Uint32(byteBuffer[lo : lo+4])))
			chunkR[i] = float64(math.Float32frombits(binary.LittleEndian.Uint32(byteBuffer[ro : ro+4])))
		}

		// Slide both windows: drop the oldest chunk, append the newest.
		c.mu.Lock()
		kl := copy(c.historyL, c.historyL[len(chunkL):])
		copy(c.historyL[kl:], chunkL)
		kr := copy(c.historyR, c.historyR[len(chunkR):])
		copy(c.historyR[kr:], chunkR)
		c.primed = true
		c.mu.Unlock()
	}
}

// detectMonitorSource finds the monitor source of the current default sink
// by querying `pactl info`. Returns "" if detection fails (falls back to
// PulseAudio's default capture device).
func detectMonitorSource() string {
	out, err := exec.Command("pactl", "info").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "Default Sink:") {
			sink := strings.TrimSpace(strings.TrimPrefix(line, "Default Sink:"))
			return sink + ".monitor"
		}
	}
	return ""
}

// ReadSamples reads audio samples from the capture stream
//
// OPERATION:
//  1. Read stereo samples from PulseAudio as bytes
//  2. Convert bytes to float32 (little-endian)
//  3. Mix stereo to mono (average left and right)
//  4. Return mono samples for FFT processing
//
// WHY MIX TO MONO?
//   - Spectrum analyzer shows frequency content, not stereo position
//   - Mixing prevents phase cancellation issues
//   - Simpler FFT processing
//   - Still represents all frequencies from both channels
//
// ALTERNATIVE: Stereo visualization
//
//	Could process left/right separately for stereo spectrum
//	See docs/IMPLEMENTATION_PLAN.md "Future Enhancements"
//
// Returns:
//
//	[]float64 - Copy of the current analysis window (viz.FFT_SIZE samples),
//	            left and right averaged to mono
//	error     - Error if read fails
func (c *Capturer) ReadSamples() ([]float64, error) {
	out := make([]float64, viz.FFT_SIZE)

	c.mu.Lock()
	if c.primed {
		for i := range out {
			out[i] = (c.historyL[i] + c.historyR[i]) / 2.0
		}
	}
	c.mu.Unlock()

	// Before the first chunk arrives this is silence, which keeps the
	// pipeline running instead of erroring out.
	return out, nil
}

// ReadStereo returns copies of the current left and right analysis windows
// (each viz.FFT_SIZE samples). Used by the butterfly layout.
func (c *Capturer) ReadStereo() (left, right []float64) {
	left = make([]float64, viz.FFT_SIZE)
	right = make([]float64, viz.FFT_SIZE)

	c.mu.Lock()
	if c.primed {
		copy(left, c.historyL)
		copy(right, c.historyR)
	}
	c.mu.Unlock()

	return left, right
}

// Close stops the reader goroutine and frees the audio stream.
//
// It waits for readLoop to return before calling Free() so the C stream is
// never freed while a read is in flight. Fragsize is ~16ms, so the wait is
// short; the timeout is only a safety net for a wedged audio server.
func (c *Capturer) Close() {
	c.closeOnce.Do(func() { close(c.done) })

	select {
	case <-c.stopped:
	case <-time.After(200 * time.Millisecond):
	}

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
