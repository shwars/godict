package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testConfig = `speech {
  api_key = "%TEST_API_KEY%"
  folder_id = "folder"
  default_language = "Auto"
  languages = { Auto = "auto", English = "en-US" }
}
model "one" {
  model_name = "test-model"
  base_url = "https://example.test/v1"
  api_key = "%TEST_API_KEY%"
  params = { reasoning = { effort = "none" } }
  reasoning = "low"
  default = true
}
template "edit" { text = "Edit {recognized_text}" }
`

func TestLoadExpandsEnvironmentAndParams(t *testing.T) {
	t.Setenv("TEST_API_KEY", "secret")
	path := filepath.Join(t.TempDir(), "godict.config")
	if err := os.WriteFile(path, []byte(testConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Speech.APIKey != "secret" || cfg.DefaultModel().APIKey != "secret" {
		t.Fatal("environment value was not expanded")
	}
	if got := cfg.DefaultModel().Params["reasoning"].(map[string]any)["effort"]; got != "none" {
		t.Fatalf("params = %#v", cfg.DefaultModel().Params)
	}
	if got := cfg.DefaultModel().Reasoning; got != "low" {
		t.Fatalf("reasoning = %q", got)
	}
	if got := cfg.LanguageNames(); strings.Join(got, ",") != "Auto,English" {
		t.Fatalf("languages = %v", got)
	}
}

func TestLoadSkipsModelWithMissingEnvironment(t *testing.T) {
	t.Setenv("TEST_API_KEY", "available")
	path := filepath.Join(t.TempDir(), "godict.config")
	configWithFallback := testConfig + `
model "unavailable" {
  model_name = "unavailable"
  base_url = "https://example.test/v1"
  api_key = "%MISSING_MODEL_KEY%"
}
`
	if err := os.WriteFile(path, []byte(configWithFallback), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Models) != 1 || cfg.Models[0].Name != "one" {
		t.Fatalf("available models = %#v", cfg.Models)
	}
}

func TestBundledConfigParses(t *testing.T) {
	t.Setenv("api_key", "key")
	t.Setenv("folder_id", "folder")
	t.Setenv("OPENAI_API_KEY", "openai-key")
	cfg, err := Load(filepath.Join("..", "..", "godict.config"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Models) != 10 || len(cfg.Templates) != 4 {
		t.Fatalf("models=%d templates=%d", len(cfg.Models), len(cfg.Templates))
	}
}
