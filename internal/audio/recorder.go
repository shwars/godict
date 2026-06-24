package audio

import (
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	"github.com/gen2brain/malgo"
)

// Beep plays a short completion tone asynchronously. Failure to open an output
// device is intentionally ignored so notifications never affect dictation.
func Beep() { go playBeep() }

func playBeep() {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, func(string) {})
	if err != nil { return }
	defer func() { _ = ctx.Uninit(); ctx.Free() }()
	config := malgo.DefaultDeviceConfig(malgo.Playback)
	config.Playback.Format, config.Playback.Channels, config.SampleRate = malgo.FormatS16, 1, 44100
	var sample uint32
	device, err := malgo.InitDevice(ctx.Context, config, malgo.DeviceCallbacks{Data: func(output, _ []byte, frames uint32) {
		for i := uint32(0); i < frames; i++ { var value int16; if (sample/50)%2 == 0 { value = 9000 }; binary.LittleEndian.PutUint16(output[i*2:], uint16(value)); sample++ }
	}})
	if err != nil { return }
	defer device.Uninit()
	if device.Start() != nil { return }
	time.Sleep(100 * time.Millisecond)
}

// ChunkHandler receives a copy of each PCM16 mono audio chunk. Implementations
// must return quickly: it is called from the microphone callback.
type ChunkHandler func([]byte) error

// Recorder streams microphone samples as raw PCM16 mono at 16 kHz. It never
// accumulates a recording in memory, which keeps long dictations bounded.
type Recorder struct {
	mu          sync.Mutex
	ctx         *malgo.AllocatedContext
	device      *malgo.Device
	handler     ChunkHandler
	active      bool
	chunks      uint64
	callbackErr error
}

// Start opens the default microphone and begins delivering chunks to handler.
func (r *Recorder) Start(handler ChunkHandler) error {
	if handler == nil {
		return fmt.Errorf("audio chunk handler is required")
	}
	r.mu.Lock()
	if r.active {
		r.mu.Unlock()
		return fmt.Errorf("recording is already active")
	}
	r.mu.Unlock()

	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, func(string) {})
	if err != nil {
		return fmt.Errorf("initialize microphone: %w", err)
	}
	config := malgo.DefaultDeviceConfig(malgo.Capture)
	config.Capture.Format = malgo.FormatS16
	config.Capture.Channels = 1
	config.SampleRate = SampleRate
	callbacks := malgo.DeviceCallbacks{Data: func(_, input []byte, _ uint32) {
		// malgo owns input after this callback returns, so make an independent
		// copy before handing it to the streaming transport.
		chunk := append([]byte(nil), input...)
		r.mu.Lock()
		if !r.active || r.callbackErr != nil {
			r.mu.Unlock()
			return
		}
		h := r.handler
		r.mu.Unlock()
		if err := h(chunk); err != nil {
			r.mu.Lock()
			if r.callbackErr == nil {
				r.callbackErr = err
			}
			r.mu.Unlock()
			return
		}
		r.mu.Lock()
		r.chunks++
		r.mu.Unlock()
	}}
	device, err := malgo.InitDevice(ctx.Context, config, callbacks)
	if err != nil {
		ctx.Uninit()
		ctx.Free()
		return fmt.Errorf("open microphone: %w", err)
	}

	r.mu.Lock()
	if r.active {
		r.mu.Unlock()
		device.Uninit()
		ctx.Uninit()
		ctx.Free()
		return fmt.Errorf("recording is already active")
	}
	r.ctx, r.device, r.handler = ctx, device, handler
	r.active, r.chunks, r.callbackErr = true, 0, nil
	r.mu.Unlock()
	if err := device.Start(); err != nil {
		r.release(device, ctx)
		return fmt.Errorf("start microphone: %w", err)
	}
	return nil
}

// Stop stops capture and waits for malgo to release its callback. Chunks already
// accepted by the handler are owned by the downstream streaming session.
func (r *Recorder) Stop() error {
	r.mu.Lock()
	if !r.active {
		r.mu.Unlock()
		return fmt.Errorf("recording is not active")
	}
	device, ctx, chunks, callbackErr := r.device, r.ctx, r.chunks, r.callbackErr
	r.device, r.ctx, r.handler = nil, nil, nil
	r.active = false
	r.mu.Unlock()
	r.cleanup(device, ctx)
	if callbackErr != nil {
		return fmt.Errorf("stream microphone audio: %w", callbackErr)
	}
	if chunks == 0 {
		return fmt.Errorf("no audio was recorded")
	}
	return nil
}

func (r *Recorder) release(device *malgo.Device, ctx *malgo.AllocatedContext) {
	r.mu.Lock()
	if r.device == device {
		r.device, r.ctx, r.handler, r.active = nil, nil, nil, false
	}
	r.mu.Unlock()
	r.cleanup(device, ctx)
}

func (r *Recorder) cleanup(device *malgo.Device, ctx *malgo.AllocatedContext) {
	if device != nil {
		_ = device.Stop()
		device.Uninit()
	}
	if ctx != nil {
		_ = ctx.Uninit()
		ctx.Free()
	}
}
