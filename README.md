# scribin

A 100% local Go utility that scans a folder of already-recorded videos,
extracts the audio from each one (via ffmpeg), and generates a transcript
(plain text and SRT) using [whisper.cpp](https://github.com/ggerganov/whisper.cpp)
as the engine, with no cloud API dependency whatsoever.

## Prerequisites

- Go 1.22 or newer
- [ffmpeg](https://ffmpeg.org/) on the PATH
- cmake and a C/C++ compiler (gcc or clang), to build whisper.cpp
- git

With the prerequisites installed, run the setup script from the project root
to clone and build whisper.cpp and download the `small` model:

```sh
scripts/setup.sh
```

The script is idempotent (safe to run again without duplicating the clone or
the download) and, at the end, runs a test transcription to confirm the
ffmpeg + whisper.cpp chain is working. The whisper.cpp clone lives in
`third_party/whisper.cpp/` and downloaded models in `models/` — both outside
version control.

> Note: on some Windows environments with **Smart App Control** enabled, the
> build toolchain (MinGW/GCC, and even the Go assembler) can be silently
> blocked. In that case, run the script inside WSL (Windows Subsystem for
> Linux) instead.

## Building

```sh
go build -o scribin ./cmd/scribin
```

## Usage

```sh
./scribin \
  --input-dir ./videos \
  --output-dir ./transcripts \
  --model third_party/whisper.cpp/models/ggml-small.bin \
  --whisper-bin third_party/whisper.cpp/build/bin/whisper-cli \
  --lang en
```

For each video found recursively under `--input-dir` (extensions `.mp4`,
`.mkv`, `.mov`, `.avi`), the command extracts the audio, transcribes it, and
writes `<video-name>.txt` (plain text) and `<video-name>.srt` (with
timestamps) to `--output-dir`. Videos that already have a matching `.txt` in
`--output-dir` are skipped, so running the command again over the same
folder only processes what's new.

### Available flags

| Flag                   | Required |         Default        | Description                                                                 |
|-------------------------|:--------:|--------------------------|------------------------------------------------------------------------------|
| `--input-dir`            | yes      | —                        | Directory with the videos to transcribe (recursive scan)                   |
| `--output-dir`           | yes      | —                        | Directory where `.txt`/`.srt` files are written                             |
| `--model`                | yes      | —                        | Path to the ggml model (e.g. `ggml-small.bin`)                              |
| `--whisper-bin`          | yes      | —                        | Path to the `whisper-cli` binary                                            |
| `--lang`                 | no       | `pt`                     | Spoken language in the videos (e.g. `pt`, `en`, or `auto` for detection)    |
| `--workers`              | no       | half the available CPUs | How many videos to process in parallel                                      |
| `--per-video-timeout`    | no       | `30m`                    | Per-video timeout (extraction + transcription); prevents a corrupted video from hanging the whole batch |
| `--dry-run`              | no       | `false`                  | Only lists the videos that would be processed, without calling ffmpeg/whisper |
| `--tui`                  | no       | `false`                  | Shows an interactive terminal UI instead of plain logs                      |

The command keeps processing the remaining videos even if one fails
individually (the error is logged and the batch continues), but exits with a
non-zero status code if any failure occurred — useful for detecting problems
in scripts/CI.

### Terminal UI (`--tui`)

With `--tui`, the command shows the list of videos found along with each
one's status (pending, extracting audio, transcribing, done, skipped,
error), a per-video progress bar while it's being processed, an overall
progress bar, and a final summary (successes, failures, total time) once
everything finishes — press any key to exit after the summary, or `q`/`Ctrl+C`
at any time. If stdout isn't an interactive terminal (e.g. running inside a
script or with stdout redirected), `--tui` is ignored and the command falls
back to plain log output automatically.

### Running via WSL (Windows)

If Smart App Control blocks the native build on Windows (see note above),
build `scribin` inside WSL instead (e.g. `go build -o scribin_linux ./cmd/scribin`)
and run it from there — Windows paths are reachable under `/mnt/c/...`:

```sh
wsl -d Ubuntu -- bash -lc '
cd /mnt/c/DEV/scribin && \
./scribin_linux \
  --input-dir "/mnt/c/Videos" \
  --output-dir "/mnt/c/Videos/transcripts" \
  --model models/ggml-small.bin \
  --whisper-bin third_party/whisper.cpp/build/bin/whisper-cli \
  --lang en \
  --tui
'
```

## Tests

```sh
go test ./...
```

The end-to-end integration test in `cmd/scribin` uses the short video at
`testdata/sample.mp4` and only runs if `ffmpeg`, the `whisper-cli` binary, and
a ggml model are already available (i.e. after running `scripts/setup.sh`);
otherwise it's skipped automatically.

## Releases

Cutting a new version is automatic: just merge a PR from `develop` into
`main` using [Conventional Commits](https://www.conventionalcommits.org/)
(`feat:`, `fix:`, `feat!:`/`fix!:` or `BREAKING CHANGE` in the commit body for
breaking changes, etc.). The workflow in `.github/workflows/release.yml`
takes care of the rest:

1. Analyzes the commits since the last tag and decides the next semver
   version (`feat:` → minor, `fix:` → patch, breaking change → major),
   creating and pushing the tag automatically. If no relevant commit is
   found since the last tag, no new tag is created.
2. Builds `cmd/scribin` for Linux, macOS, and Windows (amd64 and arm64) with
   [GoReleaser](https://goreleaser.com/) (configured in `.goreleaser.yaml`)
   and publishes the binaries as attachments on a GitHub Release at the
   newly created tag.

Nothing needs to be run manually beyond the merge into `main`.

## License

Released under the [MIT License](LICENSE).
