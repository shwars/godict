# GoDict

GoDict is a small cross-platform desktop dictator. Choose a prompt template, LLM, and recognition language; press **Start recording**, dictate, then press **Stop recording**. GoDict sends raw 16-bit PCM microphone audio to Yandex SpeechKit, sends the recognized text to an OpenAI Responses-compatible LLM, and copies the generated result to the clipboard.

## Setup

1. Place `godict.config` beside the executable. Release archives already include it.
2. Set the environment variables referenced by `%NAME%` values in that file, or replace them with literal credentials. Do not commit real credentials.
3. Allow microphone access when your operating system asks.
4. Start the application.

The supplied configuration contains the model entries from the existing `gogpt.config` example and two templates: `correct` and `professional`. The default SpeechKit language list is `Auto`, Russian, and English; edit `speech.languages` to add other SpeechKit language codes.

## Configuration format

`godict.config` is HCL. Models and templates are named blocks:

```hcl
speech {
  api_key          = "%api_key%"
  folder_id        = "%folder_id%"
  default_language = "Auto"
  languages = { Auto = "auto", Russian = "ru-RU" }
}

model "example" {
  model_name = "example-model"
  base_url   = "https://example.com/v1"
  api_key    = "%EXAMPLE_API_KEY%"
  default    = true
  params     = { reasoning = { effort = "none" } }
}

template "my-template" {
  text = <<-PROMPT
Use this dictated text: {recognized_text}
Optional existing context: {clipboard}
PROMPT
}
```

`{recognized_text}` is replaced with the SpeechKit result. If it is absent, the result is appended to the template. `{clipboard}` is replaced with the current clipboard text only when that exact placeholder is present. An empty clipboard becomes an empty string; templates without `{clipboard}` never trigger a clipboard read.

`project` is optional on a model and is sent as the `OpenAI-Project` header, which is useful for Yandex AI Studio’s OpenAI-compatible API.

## Local development and releases

```powershell
go test ./...
go run .
.\scripts\build-release.ps1 -Version 0.1.0
```

During `go run`, GoDict falls back to `godict.config` in the current directory; packaged binaries always use the sidecar config next to the executable.

Build a subset when you only need one platform, for example:

```powershell
.\scripts\build-release.ps1 -Version 0.1.0 -Targets windows-amd64
.\scripts\build-release.ps1 -Version 0.1.0 -Targets windows-amd64,darwin-arm64
```

The release script requires Docker. It uses Fyne’s cross-build image to create standalone binaries for Windows x64, Linux x64, macOS x64, and macOS ARM64. Output ZIP files are written to `releases/`, which is ignored by Git. macOS builds are unsigned, so users may need to approve them in System Settings before launching from Terminal.

The GitHub Actions **Release** workflow is manually triggered with a version and publishes the same archives as a GitHub Release.
