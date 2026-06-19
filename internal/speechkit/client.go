package speechkit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const endpoint = "https://stt.api.cloud.yandex.net/speech/v1/stt:recognize"

type Client struct {
	APIKey, FolderID, Endpoint string
	HTTPClient                 *http.Client
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
