# GoDict

[Русская версия](README.ru.md)

GoDict is a small cross-platform desktop dictator. Choose a prompt template, LLM, and recognition language; press **Start recording**, dictate, then press **Stop recording**. GoDict streams raw 16-bit PCM microphone audio to Yandex SpeechKit v3 while you dictate, then sends the final recognized text to an OpenAI Responses-compatible LLM and copies the generated result to the clipboard. It does not use SpeechKit's 1 MB short-audio HTTP endpoint, so long recordings are governed by the active streaming session and account limits rather than a 30-second client upload limit.

## Setup

1. Place `godict.config` beside the executable. Release archives already include it.
2. Set the environment variables referenced by `%NAME%` values in that file, or replace them with literal credentials. Do not commit real credentials.
3. Allow microphone access when your operating system asks.
4. Start the application.

The supplied configuration contains the model entries from the existing `gogpt.config` example and two templates: `correct` and `professional`. The default SpeechKit language list is `Auto`, Russian, and English; edit `speech.languages` to add other SpeechKit language codes.

## Installation

Unpack the release archive, choose a starter configuration, and keep it beside the executable under the name `godict.config`. On macOS, `/usr/local/bin` is normally already on your `PATH`, so you can install both files there:

```sh
sudo cp godict godict.config /usr/local/bin/
```

The macOS binaries are unsigned. If macOS blocks the downloaded executable, remove its quarantine attribute and then start it from Terminal:

```sh
sudo xattr -d com.apple.quarantine /usr/local/bin/godict
godict
```

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

`{recognized_text}` is replaced with the SpeechKit result. If it is absent, the result is appended to the template. `{language}` is replaced with the selected SpeechKit language code, including `auto` for the Auto selection. `{clipboard}` is replaced with the current clipboard text only when that exact placeholder is present. An empty clipboard becomes an empty string; templates without `{clipboard}` never trigger a clipboard read. Set `default = true` on one template block to pre-select it when GoDict starts.

`project` is optional on a model and is sent as the `OpenAI-Project` header, which is useful for Yandex AI Studio’s OpenAI-compatible API.

`reasoning` is optional on a model. When set, GoDict sends it as the Responses API `reasoning.effort`; when omitted, it does not send a reasoning field. Define duplicate model blocks with distinct names when you need the same model at different reasoning levels. The **Web search** checkbox sends `tools: [{"type":"web_search"}]`. **Display result** reveals a read-only result panel while continuing to copy successful output to the clipboard.

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
.\scripts\build-release.ps1 -Version 0.1.0 -Targets darwin-amd64
```

The local release script requires Docker/Fyne-cross. By default it creates standalone binaries for Windows x64, Windows ARM64, Linux x64, and macOS ARM64. macOS x64 remains available as the explicit `darwin-amd64` target, but is excluded by default. Output ZIP files are written to `releases/`, which is ignored by Git. macOS builds are unsigned, so users may need to approve them in System Settings before launching from Terminal.

The manually triggered GitHub Actions workflow builds Windows x64, Linux x64, and macOS ARM64 on their native hosted runners. `build_windows_arm64` optionally adds the Docker cross-build. Set `create_release` to `false` to keep the ZIPs as workflow artifacts only; set it to `true` and provide a title and description to publish a GitHub Release.
