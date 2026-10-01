# Session expiry demo

A small access-control change makes a useful example: a session must be rejected when its expiry is **at or before** the current time.

The [example](../examples/session-expiry/session.go) starts with `expiresAt >= now`. Its existing tests cover the future and the past, so they pass while missing the equality boundary. The intended change is `expiresAt > now`. A useful challenge tests `Valid(100, 100)` and expects `false`.

Tarigato gives the builder the requirement and lets a separate challenger look for a counterexample. The controller runs the submitted test twice and preserves it for the final checks. A correct first patch needs no repair; a reproduced failure permits one repair. Real agents may choose a different test or report no finding.

## Run it

From the Tarigato checkout, with Go, Git, and an authenticated Codex CLI available:

```sh
demo_dir="$(mktemp -d)"
go build -o "$demo_dir/tarigato" ./cmd/tarigato
cp -R examples/session-expiry "$demo_dir/session-expiry"
cd "$demo_dir/session-expiry"
git init -q
git add .
git commit -qm "Add session expiry example"
go test ./...
"$demo_dir/tarigato" "Reject sessions when expiry is at or before now"
```

Copying the example into its own repository gives Tarigato a clean, committed root module. The run uses your configured provider account. Its patches and report are saved under `~/.tarigato/runs/`; your example checkout stays unchanged.

Open the printed `report.md`, check the source diff and the challenger's expectation, then follow the report's replay commands in a fresh checkout. `ready_for_review` means the recorded checks passed; it does not establish complete coverage.

## Recording

[Watch the 24-second video](assets/terminal-demo.mp4) or inspect the [original terminal capture](assets/terminal-demo.cast). The [animated preview](assets/terminal-demo.gif) is embedded in the README. **Actual CLI output; playback accelerated.** The 91.023-second run plays at approximately 4.8× speed, with a final hold.

Recorded on 2026-10-01 with Codex CLI 0.159.2 and Go 1.27.1 on macOS arm64. The builder changed `>=` to `>`. The challenger submitted one test; both challenge runs and the final checks passed. No repair was needed.

### Render the existing capture

From the Tarigato checkout on macOS, use Python 3, the bundled Menlo and Apple Symbols fonts, and Swift's native video encoder. This uses the saved capture and makes no provider calls:

```sh
render_dir="$(mktemp -d)"
python3 -m venv "$render_dir/venv"
"$render_dir/venv/bin/pip" install Pillow pyte
"$render_dir/venv/bin/python" scripts/demo/record.py render \
  docs/assets/terminal-demo.cast --frames "$render_dir/frames" \
  --output "$render_dir/terminal-demo"
swift scripts/demo/encode.swift "$render_dir/frames" "$render_dir/terminal-demo.mp4" 10
```

The output directory contains the GIF, PNG, MP4, and source frames. The frame directory must be empty; the encoder refuses to overwrite an existing video.

### Capture a new run

Set up the example using the commands above, then return to the Tarigato checkout. Capture requires only Python's standard library on macOS or Linux; the Tarigato run uses your provider account:

```sh
python3 scripts/demo/record.py capture \
  --cwd "$demo_dir/session-expiry" --output "$demo_dir/new-demo.cast" -- \
  "$demo_dir/tarigato" --timeout 4m "Reject sessions when expiry is at or before now"
```

Review the new recording before sharing it, then render its `.cast` file with the commands above.
