package audio

import (
	"fmt"
	"sync"

	"github.com/gen2brain/malgo"
)

// Recorder captures a single microphone recording in raw PCM16 mono at 16 kHz.
type Recorder struct {
	mu      sync.Mutex
	ctx     *malgo.AllocatedContext
	device  *malgo.Device
	samples []byte
	active  bool
}

func (r *Recorder) Start() error {
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
	r.samples = nil
	callbacks := malgo.DeviceCallbacks{Data: func(_, input []byte, _ uint32) {
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.active {
			r.samples = append(r.samples, input...)
		}
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
	r.ctx, r.device, r.active = ctx, device, true
	r.mu.Unlock()
	if err := device.Start(); err != nil {
		r.release(device, ctx)
		return fmt.Errorf("start microphone: %w", err)
	}
	return nil
}

func (r *Recorder) Stop() ([]byte, error) {
	r.mu.Lock()
	if !r.active {
		r.mu.Unlock()
		return nil, fmt.Errorf("recording is not active")
	}
	pcm := append([]byte(nil), r.samples...)
	device, ctx := r.device, r.ctx
	r.device, r.ctx, r.active = nil, nil, false
	r.mu.Unlock()
	r.cleanup(device, ctx)
	if len(pcm) == 0 {
		return nil, fmt.Errorf("no audio was recorded")
	}
	return pcm, nil
}

func (r *Recorder) release(device *malgo.Device, ctx *malgo.AllocatedContext) {
	r.mu.Lock()
	if r.device == device {
		r.device, r.ctx, r.active = nil, nil, false
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
