package llm

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"godict/internal/config"
)

func TestGenerateUsesResponsesAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer key" || r.Header.Get("OpenAI-Project") != "folder" {
			t.Errorf("headers = %#v", r.Header)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"model":"model"`) || !strings.Contains(string(body), `"input":"hello"`) {
			t.Errorf("body = %s", body)
		}
		_, _ = w.Write([]byte(`{"output":[{"content":[{"type":"output_text","text":"done"}]}]}`))
	}))
	defer server.Close()
	client := Client{Model: config.Model{ModelName: "model", BaseURL: server.URL + "/v1", APIKey: "key", Project: "folder", Params: map[string]any{"reasoning": map[string]any{"effort": "none"}}}}
	got, err := client.Generate(context.Background(), "hello")
	if err != nil || got != "done" {
		t.Fatalf("Generate() = %q, %v", got, err)
	}
}

func TestExtractTextTopLevel(t *testing.T) {
	got, err := ExtractText([]byte(`{"output_text":"plain"}`))
	if err != nil || got != "plain" {
		t.Fatalf("ExtractText() = %q, %v", got, err)
	}
}
