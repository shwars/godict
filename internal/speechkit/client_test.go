package speechkit

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	sttv3 "github.com/yandex-cloud/go-genproto/yandex/cloud/ai/stt/v3"
)

func TestRecognizeBuildsSpeechKitRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Api-Key key" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		if r.URL.Query().Get("folderId") != "folder" || r.URL.Query().Get("lang") != "ru-RU" {
			t.Errorf("query = %s", r.URL.RawQuery)
		}
		if r.URL.Query().Get("format") != "lpcm" || r.Header.Get("Content-Type") != "application/octet-stream" {
			t.Errorf("format=%q content-type=%q", r.URL.Query().Get("format"), r.Header.Get("Content-Type"))
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != "wav" {
			t.Errorf("body = %q", body)
		}
		_, _ = w.Write([]byte(`{"result":"recognized"}`))
	}))
	defer server.Close()
	got, err := (Client{APIKey: "key", FolderID: "folder", Endpoint: server.URL}).Recognize(context.Background(), []byte("wav"), "ru-RU")
	if err != nil || got != "recognized" {
		t.Fatalf("Recognize() = %q, %v", got, err)
	}
}

func TestSessionOptionsConfiguresRawPCMAndLanguage(t *testing.T) {
	options := sessionOptions("ru-RU").GetSessionOptions().GetRecognitionModel()
	if options.GetModel() != "general" {
		t.Fatalf("model = %q", options.GetModel())
	}
	raw := options.GetAudioFormat().GetRawAudio()
	if raw == nil || raw.GetSampleRateHertz() != 16000 || raw.GetAudioChannelCount() != 1 {
		t.Fatalf("raw audio = %#v", raw)
	}
	if got := options.GetLanguageRestriction().GetLanguageCode(); len(got) != 1 || got[0] != "ru-RU" {
		t.Fatalf("language restriction = %#v", got)
	}
}

func TestSessionOptionsLeavesAutoLanguageUnrestricted(t *testing.T) {
	if restriction := sessionOptions("auto").GetSessionOptions().GetRecognitionModel().GetLanguageRestriction(); restriction != nil {
		t.Fatalf("unexpected language restriction: %#v", restriction)
	}
}

func TestUpdateTextUsesTopAlternative(t *testing.T) {
	// Keep text extraction independent from streaming I/O: the server's top
	// alternative is the one sent into the prompt.
	update := &sttv3.AlternativeUpdate{Alternatives: []*sttv3.Alternative{{Text: "  hello  "}, {Text: "other"}}}
	if got := updateText(update); got != "hello" {
		t.Fatalf("updateText() = %q", got)
	}
}

func TestStreamSessionForwardsChunksAndReturnsFinalTranscript(t *testing.T) {
	transport := &fakeRecognitionStream{
		responses: []*sttv3.StreamingResponse{{Event: &sttv3.StreamingResponse_Final{
			Final: &sttv3.AlternativeUpdate{Alternatives: []*sttv3.Alternative{{Text: "final transcript"}}},
		}}},
	}
	session := &streamSession{
		cancel:       func() {},
		stream:       transport,
		chunks:       make(chan []byte, 2),
		senderDone:   make(chan struct{}),
		receiverDone: make(chan struct{}),
	}
	go session.sendLoop()
	go session.receiveLoop()
	if err := session.Send([]byte{1, 2, 3}); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	transcript, err := session.Close()
	if err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if transcript != "final transcript" {
		t.Fatalf("transcript = %q", transcript)
	}
	requests := transport.Requests()
	if len(requests) != 1 || string(requests[0].GetChunk().GetData()) != string([]byte{1, 2, 3}) {
		t.Fatalf("audio requests = %#v", requests)
	}
	if !transport.closed {
		t.Fatal("stream input was not closed")
	}
}

func TestStreamSessionReportsBackpressureWithoutBlocking(t *testing.T) {
	session := &streamSession{
		cancel:       func() {},
		chunks:       make(chan []byte),
		senderDone:   make(chan struct{}),
		receiverDone: make(chan struct{}),
	}
	err := session.Send([]byte{1})
	if !errors.Is(err, errChunkQueueFull) {
		t.Fatalf("Send() error = %v, want queue-full error", err)
	}
}

func TestStreamSessionCancelClosesInputWithoutConnection(t *testing.T) {
	session := &streamSession{
		cancel:       func() {},
		chunks:       make(chan []byte, 1),
		senderDone:   make(chan struct{}),
		receiverDone: make(chan struct{}),
	}
	session.Cancel()
	if err := session.Send([]byte{1}); err == nil {
		t.Fatal("Send() succeeded after Cancel()")
	}
}

type fakeRecognitionStream struct {
	mu        sync.Mutex
	requests  []*sttv3.StreamingRequest
	responses []*sttv3.StreamingResponse
	closed    bool
}

func (s *fakeRecognitionStream) Send(request *sttv3.StreamingRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, request)
	return nil
}

func (s *fakeRecognitionStream) Recv() (*sttv3.StreamingResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.responses) == 0 {
		return nil, io.EOF
	}
	response := s.responses[0]
	s.responses = s.responses[1:]
	return response, nil
}

func (s *fakeRecognitionStream) CloseSend() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

func (s *fakeRecognitionStream) Requests() []*sttv3.StreamingRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*sttv3.StreamingRequest(nil), s.requests...)
}
