// Package config loads the portable GoDict configuration file.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

var envReference = regexp.MustCompile(`%([A-Za-z_][A-Za-z0-9_]*)%`)

// Config is the complete application configuration.
type Config struct {
	Settings  Settings
	Speech    Speech
	Models    []Model
	Templates []Template
}
type Settings struct { BeepOnCompletion bool }

type Speech struct {
	APIKey          string
	FolderID        string
	Languages       map[string]string // display name -> SpeechKit language code; "auto" selects auto detection.
	DefaultLanguage string
}

type Model struct {
	Name      string
	ModelName string
	BaseURL   string
	APIKey    string
	Project   string
	Params    map[string]any
	Reasoning string
	Default   bool
}

type Template struct {
	Name    string
	Text    string
	Default bool
}

func Load(path string) (*Config, error) {
	parser := hclparse.NewParser()
	file, diags := parser.ParseHCLFile(path)
	if diags.HasErrors() {
		return nil, fmt.Errorf("parse configuration: %s", diags.Error())
	}
	content, diags := file.Body.Content(&hcl.BodySchema{Blocks: []hcl.BlockHeaderSchema{
		{Type: "settings"}, {Type: "speech"}, {Type: "model", LabelNames: []string{"name"}}, {Type: "template", LabelNames: []string{"name"}},
	}})
	if diags.HasErrors() {
		return nil, fmt.Errorf("read configuration: %s", diags.Error())
	}

	cfg := &Config{Settings: Settings{BeepOnCompletion: true}}
	seenSettings := false
	for _, block := range content.Blocks {
		switch block.Type {
		case "settings":
			if seenSettings { return nil, fmt.Errorf("configuration may contain only one settings block") }
			settings, err := decodeSettings(block.Body)
			if err != nil { return nil, err }
			cfg.Settings = settings
			seenSettings = true
		case "speech":
			if cfg.Speech.APIKey != "" {
				return nil, fmt.Errorf("configuration may contain only one speech block")
			}
			speech, err := decodeSpeech(block.Body)
			if err != nil {
				return nil, err
			}
			cfg.Speech = speech
		case "model":
			model, err := decodeModel(block.Labels[0], block.Body)
			if err != nil {
				return nil, err
			}
			if err := expandModel(&model); err != nil {
				var missingEnv *missingEnvironmentError
				if errors.As(err, &missingEnv) {
					// A model may rely on credentials that are intentionally absent on
					// this machine. Do not expose an unusable choice in the UI.
					continue
				}
				return nil, fmt.Errorf("model %q: %w", model.Name, err)
			}
			cfg.Models = append(cfg.Models, model)
		case "template":
			template, err := decodeTemplate(block.Labels[0], block.Body)
			if err != nil {
				return nil, err
			}
			cfg.Templates = append(cfg.Templates, template)
		}
	}
	if err := expandConfig(cfg); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func decodeSettings(body hcl.Body) (Settings, error) {
	attrs, diags := body.JustAttributes()
	if diags.HasErrors() { return Settings{}, fmt.Errorf("read settings block: %s", diags.Error()) }
	beep, err := optionalBool(attrs, "beep_on_completion")
	if err != nil { return Settings{}, err }
	if _, ok := attrs["beep_on_completion"]; !ok { beep = true }
	return Settings{BeepOnCompletion: beep}, nil
}

func (c *Config) Validate() error {
	if c.Speech.APIKey == "" {
		return fmt.Errorf("speech.api_key is required")
	}
	if c.Speech.FolderID == "" {
		return fmt.Errorf("speech.folder_id is required")
	}
	if len(c.Speech.Languages) == 0 {
		return fmt.Errorf("speech.languages must define at least Auto")
	}
	if _, ok := c.Speech.Languages[c.Speech.DefaultLanguage]; !ok {
		return fmt.Errorf("speech.default_language %q is not in speech.languages", c.Speech.DefaultLanguage)
	}
	if len(c.Models) == 0 {
		return fmt.Errorf("at least one model block is required")
	}
	defaults := 0
	for _, model := range c.Models {
		if model.Name == "" || model.ModelName == "" || model.BaseURL == "" || model.APIKey == "" {
			return fmt.Errorf("model %q requires model_name, base_url, and api_key", model.Name)
		}
		if model.Default {
			defaults++
		}
	}
	if defaults > 1 {
		return fmt.Errorf("only one model may be marked default")
	}
	if len(c.Templates) == 0 {
		return fmt.Errorf("at least one template block is required")
	}
	for _, template := range c.Templates {
		if template.Name == "" || strings.TrimSpace(template.Text) == "" {
			return fmt.Errorf("every template requires text")
		}
	}
	templateDefaults := 0
	for _, template := range c.Templates {
		if template.Default {
			templateDefaults++
		}
	}
	if templateDefaults > 1 {
		return fmt.Errorf("only one template may be marked default")
	}
	return nil
}

func (c *Config) DefaultModel() Model {
	for _, model := range c.Models {
		if model.Default {
			return model
		}
	}
	return c.Models[0]
}

func (c *Config) DefaultTemplate() Template {
	for _, template := range c.Templates {
		if template.Default {
			return template
		}
	}
	return c.Templates[0]
}

func (c *Config) LanguageNames() []string {
	names := make([]string, 0, len(c.Speech.Languages))
	for name := range c.Speech.Languages {
		names = append(names, name)
	}
	sort.Strings(names)
	// Auto is always first when supplied.
	for i, name := range names {
		if name == "Auto" {
			return append([]string{"Auto"}, append(names[:i], names[i+1:]...)...)
		}
	}
	return names
}

func decodeSpeech(body hcl.Body) (Speech, error) {
	attrs, diags := body.JustAttributes()
	if diags.HasErrors() {
		return Speech{}, fmt.Errorf("read speech block: %s", diags.Error())
	}
	apiKey, err := requiredString(attrs, "api_key")
	if err != nil {
		return Speech{}, err
	}
	folderID, err := requiredString(attrs, "folder_id")
	if err != nil {
		return Speech{}, err
	}
	defaultLanguage, err := requiredString(attrs, "default_language")
	if err != nil {
		return Speech{}, err
	}
	languagesValue, err := requiredValue(attrs, "languages")
	if err != nil {
		return Speech{}, err
	}
	if !languagesValue.Type().IsObjectType() && !languagesValue.Type().IsMapType() {
		return Speech{}, fmt.Errorf("speech.languages must be an object")
	}
	languages := map[string]string{}
	it := languagesValue.ElementIterator()
	for it.Next() {
		key, value := it.Element()
		if value.Type() != cty.String {
			return Speech{}, fmt.Errorf("speech.languages values must be strings")
		}
		languages[key.AsString()] = value.AsString()
	}
	return Speech{APIKey: apiKey, FolderID: folderID, Languages: languages, DefaultLanguage: defaultLanguage}, nil
}

func decodeModel(name string, body hcl.Body) (Model, error) {
	attrs, diags := body.JustAttributes()
	if diags.HasErrors() {
		return Model{}, fmt.Errorf("read model %q: %s", name, diags.Error())
	}
	modelName, err := requiredString(attrs, "model_name")
	if err != nil {
		return Model{}, fmt.Errorf("model %q: %w", name, err)
	}
	baseURL, err := requiredString(attrs, "base_url")
	if err != nil {
		return Model{}, fmt.Errorf("model %q: %w", name, err)
	}
	apiKey, err := requiredString(attrs, "api_key")
	if err != nil {
		return Model{}, fmt.Errorf("model %q: %w", name, err)
	}
	project, err := optionalString(attrs, "project")
	if err != nil {
		return Model{}, fmt.Errorf("model %q: %w", name, err)
	}
	defaultValue, err := optionalBool(attrs, "default")
	if err != nil {
		return Model{}, fmt.Errorf("model %q: %w", name, err)
	}
	reasoning, err := optionalString(attrs, "reasoning")
	if err != nil {
		return Model{}, fmt.Errorf("model %q: %w", name, err)
	}
	params := map[string]any{}
	if attr, ok := attrs["params"]; ok {
		value, diags := attr.Expr.Value(nil)
		if diags.HasErrors() {
			return Model{}, fmt.Errorf("model %q params: %s", name, diags.Error())
		}
		encoded, err := ctyjson.Marshal(value, value.Type())
		if err != nil {
			return Model{}, fmt.Errorf("model %q params: %w", name, err)
		}
		if err := json.Unmarshal(encoded, &params); err != nil {
			return Model{}, fmt.Errorf("model %q params: %w", name, err)
		}
	}
	return Model{Name: name, ModelName: modelName, BaseURL: baseURL, APIKey: apiKey, Project: project, Params: params, Reasoning: reasoning, Default: defaultValue}, nil
}

func decodeTemplate(name string, body hcl.Body) (Template, error) {
	attrs, diags := body.JustAttributes()
	if diags.HasErrors() {
		return Template{}, fmt.Errorf("read template %q: %s", name, diags.Error())
	}
	text, err := requiredString(attrs, "text")
	if err != nil {
		return Template{}, fmt.Errorf("template %q: %w", name, err)
	}
	defaultValue, err := optionalBool(attrs, "default")
	if err != nil {
		return Template{}, fmt.Errorf("template %q: %w", name, err)
	}
	return Template{Name: name, Text: text, Default: defaultValue}, nil
}

func requiredValue(attrs hcl.Attributes, name string) (cty.Value, error) {
	attr, ok := attrs[name]
	if !ok {
		return cty.NilVal, fmt.Errorf("%s is required", name)
	}
	value, diags := attr.Expr.Value(nil)
	if diags.HasErrors() {
		return cty.NilVal, fmt.Errorf("%s: %s", name, diags.Error())
	}
	return value, nil
}
func requiredString(attrs hcl.Attributes, name string) (string, error) {
	value, err := requiredValue(attrs, name)
	if err != nil {
		return "", err
	}
	if value.Type() != cty.String {
		return "", fmt.Errorf("%s must be a string", name)
	}
	return value.AsString(), nil
}
func optionalString(attrs hcl.Attributes, name string) (string, error) {
	if _, ok := attrs[name]; !ok {
		return "", nil
	}
	return requiredString(attrs, name)
}
func optionalBool(attrs hcl.Attributes, name string) (bool, error) {
	if _, ok := attrs[name]; !ok {
		return false, nil
	}
	value, err := requiredValue(attrs, name)
	if err != nil {
		return false, err
	}
	if value.Type() != cty.Bool {
		return false, fmt.Errorf("%s must be a boolean", name)
	}
	return value.True(), nil
}

func expandConfig(cfg *Config) error {
	var err error
	if cfg.Speech.APIKey, err = expand(cfg.Speech.APIKey); err != nil {
		return fmt.Errorf("speech.api_key: %w", err)
	}
	if cfg.Speech.FolderID, err = expand(cfg.Speech.FolderID); err != nil {
		return fmt.Errorf("speech.folder_id: %w", err)
	}
	for i := range cfg.Templates {
		if cfg.Templates[i].Text, err = expand(cfg.Templates[i].Text); err != nil {
			return fmt.Errorf("template %q: %w", cfg.Templates[i].Name, err)
		}
	}
	return nil
}

func expandModel(model *Model) error {
	var err error
	for _, field := range []*string{&model.ModelName, &model.BaseURL, &model.APIKey, &model.Project} {
		if *field, err = expand(*field); err != nil {
			return err
		}
	}
	model.Params, err = expandAny(model.Params)
	return err
}

type missingEnvironmentError struct{ name string }

func (e *missingEnvironmentError) Error() string {
	return fmt.Sprintf("environment variable %%%s%% is not set", e.name)
}

func expand(value string) (string, error) {
	missing := ""
	result := envReference.ReplaceAllStringFunc(value, func(match string) string {
		name := envReference.FindStringSubmatch(match)[1]
		env, ok := os.LookupEnv(name)
		if !ok {
			missing = name
			return match
		}
		return env
	})
	if missing != "" {
		return "", &missingEnvironmentError{name: missing}
	}
	return result, nil
}
func expandAny(value map[string]any) (map[string]any, error) {
	for key, item := range value {
		switch typed := item.(type) {
		case string:
			expanded, err := expand(typed)
			if err != nil {
				return nil, err
			}
			value[key] = expanded
		case map[string]any:
			expanded, err := expandAny(typed)
			if err != nil {
				return nil, err
			}
			value[key] = expanded
		case []any:
			for i, child := range typed {
				if stringChild, ok := child.(string); ok {
					expanded, err := expand(stringChild)
					if err != nil {
						return nil, err
					}
					typed[i] = expanded
				}
			}
		}
	}
	return value, nil
}
