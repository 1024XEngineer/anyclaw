# macOS Release

This project can build a signed and notarized macOS desktop app from the Tauri workspace in `src-tauri`.

## Prerequisites

- Apple Developer account
- A valid `Developer ID Application` certificate installed in Keychain
- Xcode command line tools
- `xcrun notarytool` available
- Rust, Cargo, Node.js, and Tauri CLI installed

## Required Environment Variables

For code signing:

```bash
export MACOS_SIGNING_IDENTITY='Developer ID Application: Your Name (TEAMID)'
```

For notarization:

```bash
export APPLE_ID='your-apple-id@example.com'
export APPLE_PASSWORD='app-specific-password'
export APPLE_TEAM_ID='TEAMID'
```

## Build Signed Release

```bash
make release-tauri-macos
```

This command:

- builds the embedded dashboard UI
- prepares the sidecar binary
- runs `cargo tauri build`
- injects the signing identity through a temporary config merge

Artifacts are created under:

```bash
src-tauri/target/release/bundle/
```

Typical outputs:

- `src-tauri/target/release/bundle/macos/AnyClaw.app`
- `src-tauri/target/release/bundle/dmg/*.dmg`

## Verify Signature

```bash
make verify-tauri-macos
```

This runs:

- `codesign -dv --verbose=4`
- `spctl -a -vv`

## Notarize DMG

```bash
make notarize-tauri-macos
```

This submits the generated DMG to Apple and waits for completion.

## Staple Ticket

```bash
make staple-tauri-macos
```

This staples the notarization ticket to both the `.app` and `.dmg`.

## Recommended Release Flow

```bash
export MACOS_SIGNING_IDENTITY='Developer ID Application: Your Name (TEAMID)'
export APPLE_ID='your-apple-id@example.com'
export APPLE_PASSWORD='app-specific-password'
export APPLE_TEAM_ID='TEAMID'

make release-tauri-macos
make verify-tauri-macos
make notarize-tauri-macos
make staple-tauri-macos
make verify-tauri-macos
```

## Manual Acceptance Checklist

- Launch the generated `.app`
- Confirm it auto-opens the dashboard
- Verify `Sessions`, `Channels`, `Cron`, `Chat`, and `Logs`
- Quit the app and confirm the sidecar process exits cleanly
- Test the notarized DMG on a separate macOS machine if possible
