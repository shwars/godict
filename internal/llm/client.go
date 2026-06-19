package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"godict/internal/config"
)

type Client struct {
	Model      config.Model
	HTTPClient *http.Client
}

func (c Client) Generate(ctx context.Context, input string) (string, error) {
	payload := make(map[string]any, len(c.Model.Params)+2)
	for key, value := range c.Model.Params {
		payload[key] = value
	}
	payload["model"] = c.Model.ModelName
	payload["input"] = input
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode Responses request: %w", err)
	}
	url := strings.TrimRight(c.Model.BaseURL, "/") + "/responses"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.Model.APIKey)
	req.Header.Set("Content-Type", "application/json")
	if c.Model.Project != "" {
		req.Header.Set("OpenAI-Project", c.Model.Project)
	}
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("Responses request: %w", err)
	}
	defer resp.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("Responses API returned %s: %s", resp.Status, strings.TrimSpace(string(responseBody)))
	}
	text, err := ExtractText(responseBody)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("Responses API returned no text")
	}
	return text, nil
}

func ExtractText(body []byte) (string, error) {
	var response struct {
		OutputText string `json:"output_text"`
		Output     []struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("decode Responses response: %w", err)
	}
	if response.OutputText != "" {
		return response.OutputText, nil
	}
	var texts []string
	for _, item := range response.Output {
		for _, content := range item.Content {
			if content.Type == "output_text" && content.Text != "" {
				texts = append(texts, content.Text)
			}
		}
	}
	return strings.Join(texts, ""), nil
}
