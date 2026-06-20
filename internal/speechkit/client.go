package speechkit

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	sttv3 "github.com/yandex-cloud/go-genproto/yandex/cloud/ai/stt/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
)

const endpoint = "https://stt.api.cloud.yandex.net/speech/v1/stt:recognize"
const streamingEndpoint = "stt.api.cloud.yandex.net:443"

var errChunkQueueFull = fmt.Errorf("SpeechKit audio queue is full; check the network connection")

// Stream accepts microphone PCM chunks and eventually returns the final
// transcript. Send is non-blocking so it is safe to call from an audio driver
// callback. Close flushes queued chunks and waits for SpeechKit's final result.
type Stream interface {
	Send([]byte) error
	Close() (string, error)
	Cancel()
}

// StreamingClient is intentionally small so the desktop workflow can be tested
// without opening a real microphone or gRPC connection.
type StreamingClient interface {
	Start(context.Context, string) (Stream, error)
}

type Client struct {
	APIKey, FolderID, Endpoint string
	HTTPClient                 *http.Client
	// StreamingEndpoint is the host:port of the v3 gRPC API. It is primarily
	// useful for tests; production uses stt.api.cloud.yandex.net:443.
	StreamingEndpoint string
}

// Start creates a SpeechKit v3 bidirectional recognition session. The first
// stream message contains the audio format; all subsequent messages carry raw
// signed 16-bit PCM microphone chunks.
func (c Client) Start(ctx context.Context, language string) (Stream, error) {
	if strings.TrimSpace(c.APIKey) == "" {
		return nil, fmt.Errorf("SpeechKit API key is required")
	}
	if strings.TrimSpace(c.FolderID) == "" {
		return nil, fmt.Errorf("SpeechKit folder ID is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	endpoint := c.StreamingEndpoint
	if endpoint == "" {
		endpoint = streamingEndpoint
	}
	connectionCtx, cancel := context.WithCancel(ctx)
	dialCtx, cancelDial := context.WithTimeout(connectionCtx, 15*time.Second)
	conn, err := grpc.DialContext(dialCtx, endpoint,
		grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})),
		grpc.WithBlock(),
	)
	cancelDial()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("connect to SpeechKit: %w", err)
	}
	requestCtx := metadata.AppendToOutgoingContext(connectionCtx,
		"authorization", "Api-Key "+c.APIKey,
		"x-folder-id", c.FolderID,
	)
	grpcStream, err := sttv3.NewRecognizerClient(conn).RecognizeStreaming(requestCtx)
	if err != nil {
		_ = conn.Close()
		cancel()
		return nil, fmt.Errorf("open SpeechKit recognition stream: %w", err)
	}
	session := &streamSession{
		cancel: cancel, conn: conn, stream: grpcStream,
		chunks: make(chan []byte, 64), senderDone: make(chan struct{}), receiverDone: make(chan struct{}),
	}
	if err := grpcStream.Send(sessionOptions(language)); err != nil {
		session.Cancel()
		return nil, fmt.Errorf("configure SpeechKit recognition stream: %w", err)
	}
	go session.sendLoop()
	go session.receiveLoop()
	return session, nil
}

func sessionOptions(language string) *sttv3.StreamingRequest {
	model := &sttv3.RecognitionModelOptions{
		Model:               "general",
		AudioProcessingType: sttv3.RecognitionModelOptions_FULL_DATA,
		AudioFormat: &sttv3.AudioFormatOptions{AudioFormat: &sttv3.AudioFormatOptions_RawAudio{RawAudio: &sttv3.RawAudio{
			AudioEncoding:     sttv3.RawAudio_LINEAR16_PCM,
			SampleRateHertz:   16000,
			AudioChannelCount: 1,
		}}},
	}
	if language = strings.TrimSpace(language); language != "" && !strings.EqualFold(language, "auto") {
		model.LanguageRestriction = &sttv3.LanguageRestrictionOptions{
			RestrictionType: sttv3.LanguageRestrictionOptions_WHITELIST,
			LanguageCode:    []string{language},
		}
	}
	return &sttv3.StreamingRequest{Event: &sttv3.StreamingRequest_SessionOptions{
		SessionOptions: &sttv3.StreamingOptions{RecognitionModel: model},
	}}
}

type streamSession struct {
	cancel context.CancelFunc
	conn   *grpc.ClientConn
	stream recognitionStream

	chunks       chan []byte
	closeOnce    sync.Once
	cancelOnce   sync.Once
	sendMu       sync.Mutex
	closed       bool
	senderDone   chan struct{}
	receiverDone chan struct{}

	mu      sync.Mutex
	err     error
	finals  []string
	partial string
}

// recognitionStream is the small part of the generated gRPC stream used by a
// session. Keeping the boundary narrow lets the transport lifecycle be tested
// without a real microphone or SpeechKit connection.
type recognitionStream interface {
	Send(*sttv3.StreamingRequest) error
	Recv() (*sttv3.StreamingResponse, error)
	CloseSend() error
}

func (s *streamSession) Send(chunk []byte) error {
	if len(chunk) == 0 {
		return nil
	}
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	if s.closed {
		return fmt.Errorf("SpeechKit stream is closed")
	}
	s.mu.Lock()
	err := s.err
	s.mu.Unlock()
	if err != nil {
		return err
	}
	copyChunk := append([]byte(nil), chunk...)
	select {
	case s.chunks <- copyChunk:
		return nil
	default:
		s.setError(errChunkQueueFull)
		return errChunkQueueFull
	}
}

func (s *streamSession) sendLoop() {
	defer close(s.senderDone)
	for chunk := range s.chunks {
		if err := s.stream.Send(&sttv3.StreamingRequest{Event: &sttv3.StreamingRequest_Chunk{Chunk: &sttv3.AudioChunk{Data: chunk}}}); err != nil {
			s.setError(fmt.Errorf("send audio to SpeechKit: %w", err))
			s.cancel()
			return
		}
	}
	if err := s.stream.CloseSend(); err != nil {
		s.setError(fmt.Errorf("finish SpeechKit audio stream: %w", err))
		s.cancel()
	}
}

func (s *streamSession) receiveLoop() {
	defer close(s.receiverDone)
	for {
		response, err := s.stream.Recv()
		if err == io.EOF {
			return
		}
		if err != nil {
			s.setError(fmt.Errorf("receive SpeechKit recognition: %w", err))
			s.cancel()
			return
		}
		// Status updates include normal WORKING/CLOSED and non-fatal WARNING
		// messages. RPC failures are reported by Recv, so they must not turn a
		// valid transcript into an error merely because a status has text.
		if final := response.GetFinal(); final != nil {
			if text := updateText(final); text != "" {
				s.mu.Lock()
				s.finals = append(s.finals, text)
				s.mu.Unlock()
			}
		}
		if refinement := response.GetFinalRefinement(); refinement != nil {
			if text := updateText(refinement.GetNormalizedText()); text != "" {
				s.mu.Lock()
				// SpeechKit sends a normalized replacement for the matching final.
				index := int(refinement.GetFinalIndex())
				if index >= 0 && index < len(s.finals) {
					s.finals[index] = text
				}
				s.mu.Unlock()
			}
		}
		if partial := response.GetPartial(); partial != nil {
			if text := updateText(partial); text != "" {
				s.mu.Lock()
				s.partial = text
				s.mu.Unlock()
			}
		}
	}
}

func updateText(update *sttv3.AlternativeUpdate) string {
	if update == nil || len(update.GetAlternatives()) == 0 {
		return ""
	}
	return strings.TrimSpace(update.GetAlternatives()[0].GetText())
}

func (s *streamSession) Close() (string, error) {
	s.finishInput()
	<-s.senderDone
	<-s.receiverDone
	defer s.Cancel()
	s.mu.Lock()
	err := s.err
	text := strings.TrimSpace(strings.Join(s.finals, " "))
	if text == "" {
		text = strings.TrimSpace(s.partial)
	}
	s.mu.Unlock()
	if err != nil {
		return "", err
	}
	if text == "" {
		return "", fmt.Errorf("SpeechKit returned empty recognition")
	}
	return text, nil
}

func (s *streamSession) Cancel() {
	s.cancelOnce.Do(func() {
		s.finishInput()
		if s.cancel != nil {
			s.cancel()
		}
		if s.conn != nil {
			_ = s.conn.Close()
		}
	})
}

func (s *streamSession) finishInput() {
	s.closeOnce.Do(func() {
		s.sendMu.Lock()
		s.closed = true
		close(s.chunks)
		s.sendMu.Unlock()
	})
}

func (s *streamSession) setError(err error) {
	if err == nil {
		return
	}
	s.mu.Lock()
	if s.err == nil {
		s.err = err
	}
	s.mu.Unlock()
}

// Recognize sends raw signed 16-bit PCM to the SpeechKit short-audio endpoint.
// That endpoint rejects WAV containers; format and sample rate are supplied in the query.
func (c Client) Recognize(ctx context.Context, pcm []byte, language string) (string, error) {
	query := url.Values{"folderId": []string{c.FolderID}, "format": []string{"lpcm"}, "sampleRateHertz": []string{"16000"}}
	if strings.TrimSpace(language) != "" {
		query.Set("lang", language)
	}
	requestEndpoint := c.Endpoint
	if requestEndpoint == "" {
		requestEndpoint = endpoint
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestEndpoint+"?"+query.Encode(), bytes.NewReader(pcm))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Api-Key "+c.APIKey)
	req.Header.Set("Content-Type", "application/octet-stream")
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("SpeechKit request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("SpeechKit returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	// SpeechKit's short-audio endpoint returns {"result":"..."}.
	result := struct {
		Result       string `json:"result"`
		ErrorCode    string `json:"error_code"`
		ErrorMessage string `json:"error_message"`
	}{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("decode SpeechKit response: %w", err)
	}
	if result.ErrorCode != "" {
		return "", fmt.Errorf("SpeechKit: %s", result.ErrorMessage)
	}
	if strings.TrimSpace(result.Result) == "" {
		return "", fmt.Errorf("SpeechKit returned empty recognition")
	}
	return result.Result, nil
}
