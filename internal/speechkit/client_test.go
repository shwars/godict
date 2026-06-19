package speechkit

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
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
