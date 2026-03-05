# Desktop Release: Signing and Configuration Guide

This document covers the one-time setup for desktop release signing (macOS notarization, Tauri update signing) and the GitHub secrets required by the `desktop-release.yml` workflow.

## Overview

The desktop release workflow (`.github/workflows/desktop-release.yml`) triggers on `v*` tag pushes and produces:

- **macOS**: Signed and notarized `.dmg` installer + `.app.tar.gz` updater bundle
- **Windows**: `.exe` NSIS installer + `.nsis.zip` updater bundle
- **Updater manifest**: `latest.json` with platform URLs and Ed25519 signatures

Three separate credential sets are needed:

| Credential Set | Purpose | Platforms |
|----------------|---------|-----------|
| Apple Developer certificate | Code signing | macOS |
| Apple App Store Connect API key | Notarization | macOS |
| Tauri signing key | Update signature verification | macOS + Windows |

## 1. Apple Developer Certificate (macOS code signing)

### Prerequisites

- An [Apple Developer Program](https://developer.apple.com/programs/) membership ($99/year)
- A "Developer ID Application" certificate (not "Mac App Distribution")

### Generate the certificate

1. Open **Keychain Access** on your Mac
2. Go to **Keychain Access > Certificate Assistant > Request a Certificate from a Certificate Authority**
3. Enter your email, select "Saved to disk", click Continue
4. Go to [Apple Developer > Certificates](https://developer.apple.com/account/resources/certificates/list)
5. Click **+**, select **Developer ID Application**, upload the CSR
6. Download the `.cer` file and double-click to install in Keychain Access

### Export as .p12

1. In **Keychain Access**, find the certificate under "My Certificates"
2. Right-click > **Export** as `.p12`
3. Set a strong password (you'll need it for the GitHub secret)
4. Base64-encode the file:

```bash
base64 -i DeveloperIDApplication.p12 | pbcopy
```

### Find the signing identity

```bash
security find-identity -v -p codesigning
```

Look for the line containing "Developer ID Application: Your Name (TEAM_ID)". The full string in quotes is your signing identity.

### GitHub secrets

| Secret | Value |
|--------|-------|
| `APPLE_CERTIFICATE` | Base64-encoded `.p12` file content |
| `APPLE_CERTIFICATE_PASSWORD` | Password used when exporting the `.p12` |
| `APPLE_SIGNING_IDENTITY` | Full identity string, e.g. `Developer ID Application: Your Name (ABC123XYZ)` |

## 2. Apple App Store Connect API Key (notarization)

Notarization sends the built app to Apple for malware scanning. It requires an API key from App Store Connect.

### Generate the API key

1. Go to [App Store Connect > Users and Access > Integrations > App Store Connect API](https://appstoreconnect.apple.com/access/integrations/api)
2. Click **+** to create a new key
3. Name: `AgentsView Notarization` (or similar)
4. Access: **Developer** role is sufficient
5. Download the `.p8` key file (you can only download it once)
6. Note the **Key ID** (shown in the table, e.g. `ABC123DEF`)
7. Note the **Issuer ID** (shown at the top of the page, UUID format)

### Base64-encode the key

```bash
base64 -i AuthKey_ABC123DEF.p8 | pbcopy
```

### GitHub secrets

| Secret | Value |
|--------|-------|
| `APPLE_API_KEY_CONTENT` | Base64-encoded `.p8` key file content |
| `APPLE_API_KEY` | Key ID (e.g. `ABC123DEF`) |
| `APPLE_API_ISSUER` | Issuer ID (UUID from App Store Connect) |

## 3. Tauri Update Signing Key (auto-updater)

The Tauri updater uses Ed25519 signatures to verify that update bundles are authentic. A keypair is generated once; the private key signs bundles during CI, and the public key is compiled into the app binary.

### Generate the keypair

```bash
npx @tauri-apps/cli signer generate -w ~/.tauri/agentsview.key
```

This creates two files:

- `~/.tauri/agentsview.key` -- the private key (keep secret)
- `~/.tauri/agentsview.key.pub` -- the public key

The command will prompt for a password. You can leave it empty for an unencrypted key, or set one (you'll need to provide it as a GitHub secret).

### Configure the public key

The public key needs to go in **two** places:

**Option A (recommended):** Add `AGENTSVIEW_UPDATER_PUBKEY` as a GitHub Actions secret containing the public key string. The release workflow passes it as an env var to both Tauri build steps, and the Rust code reads it at compile time via `option_env!("AGENTSVIEW_UPDATER_PUBKEY")` to override the placeholder in `tauri.conf.json`. The relevant workflow lines look like:

```yaml
env:
  AGENTSVIEW_UPDATER_PUBKEY: ${{ secrets.AGENTSVIEW_UPDATER_PUBKEY }}
  TAURI_SIGNING_PRIVATE_KEY: ${{ secrets.TAURI_SIGNING_PRIVATE_KEY }}
  TAURI_SIGNING_PRIVATE_KEY_PASSWORD: ${{ secrets.TAURI_SIGNING_PRIVATE_KEY_PASSWORD }}
```

If this secret is missing or empty, the app compiles but the updater falls back to the `"NOT_SET"` placeholder and shows "updater is not configured" at runtime.

**Option B:** Replace `"NOT_SET"` in `desktop/src-tauri/tauri.conf.json` directly:

```json
"plugins": {
  "updater": {
    "pubkey": "<paste contents of agentsview.key.pub here>",
    "endpoints": [
      "https://github.com/wesm/agentsview/releases/latest/download/latest.json"
    ]
  }
}
```

### GitHub secrets

| Secret | Value |
|--------|-------|
| `TAURI_SIGNING_PRIVATE_KEY` | Contents of `~/.tauri/agentsview.key` |
| `TAURI_SIGNING_PRIVATE_KEY_PASSWORD` | Password (empty string if unencrypted) |

If using Option A for the public key:

| Secret | Value |
|--------|-------|
| `AGENTSVIEW_UPDATER_PUBKEY` | Contents of `~/.tauri/agentsview.key.pub` |

## Complete GitHub Secrets Reference

All secrets are configured at **Settings > Secrets and variables > Actions** in the GitHub repository.

| Secret | Used By | Purpose |
|--------|---------|---------|
| `APPLE_CERTIFICATE` | macOS build | Signing certificate (.p12, base64) |
| `APPLE_CERTIFICATE_PASSWORD` | macOS build | Certificate password |
| `APPLE_SIGNING_IDENTITY` | macOS build | Certificate CN identity string |
| `APPLE_API_KEY_CONTENT` | macOS build | Notarization API key (.p8, base64) |
| `APPLE_API_KEY` | macOS build | API key ID |
| `APPLE_API_ISSUER` | macOS build | API issuer ID |
| `TAURI_SIGNING_PRIVATE_KEY` | Both platforms | Tauri updater signing key |
| `TAURI_SIGNING_PRIVATE_KEY_PASSWORD` | Both platforms | Signing key password |
| `AGENTSVIEW_UPDATER_PUBKEY` | Both platforms | Updater public key (Option A) |

## Key Rotation

### Rotating the Apple certificate

Apple Developer ID Application certificates are valid for 5 years. To rotate:

1. Generate a new certificate following section 1 above
2. Export as `.p12` and base64-encode
3. Update `APPLE_CERTIFICATE` and `APPLE_CERTIFICATE_PASSWORD` in GitHub secrets
4. Update `APPLE_SIGNING_IDENTITY` if the identity string changed
5. The old certificate can be revoked in Apple Developer portal after confirming new builds work

### Rotating the Apple API key

API keys don't expire, but can be revoked. To rotate:

1. Generate a new key in App Store Connect
2. Base64-encode the new `.p8` file
3. Update `APPLE_API_KEY_CONTENT` and `APPLE_API_KEY` in GitHub secrets
4. `APPLE_API_ISSUER` doesn't change (it's per-organization)
5. Revoke the old key in App Store Connect

### Rotating the Tauri signing key

Changing the signing key means existing app installations cannot verify updates signed with the new key. Plan for this:

1. Generate a new keypair: `npx @tauri-apps/cli signer generate -w ~/.tauri/agentsview-v2.key`
2. Update `TAURI_SIGNING_PRIVATE_KEY` and `TAURI_SIGNING_PRIVATE_KEY_PASSWORD` in GitHub secrets
3. Update the public key in `tauri.conf.json` or `AGENTSVIEW_UPDATER_PUBKEY`
4. Release a version with the new public key compiled in
5. Users on older versions will see update verification fail and need to download the new version manually from the GitHub releases page

## Build Artifacts

Each release produces these artifacts:

| File | Description |
|------|-------------|
| `AgentsView_x.y.z_aarch64.dmg` | macOS installer (signed + notarized) |
| `AgentsView.app.tar.gz` | macOS updater bundle |
| `AgentsView.app.tar.gz.sig` | macOS updater signature |
| `AgentsView_x.y.z_x64-setup.exe` | Windows NSIS installer |
| `AgentsView_x.y.z_x64-setup.nsis.zip` | Windows updater bundle |
| `AgentsView_x.y.z_x64-setup.nsis.zip.sig` | Windows updater signature |
| `latest.json` | Updater manifest (version, URLs, signatures) |
| `SHA256SUMS-desktop` | Checksums for all desktop artifacts |

## Runtime Configuration

These environment variables affect the desktop app at runtime (not build time):

| Variable | Default | Purpose |
|----------|---------|---------|
| `AGENTSVIEW_DESKTOP_AUTOUPDATE` | enabled | Set to `0` to disable automatic update check on startup |
| `AGENTSVIEW_DESKTOP_SKIP_LOGIN_SHELL_ENV` | unset | Set to skip inheriting login shell environment |
| `AGENTSVIEW_DESKTOP_PATH` | unset | Override PATH passed to the Go backend sidecar |

Users can also set environment overrides in `~/.agentsview/desktop.env` (KEY=VALUE format, one per line).

## Troubleshooting

### "The updater is not configured"

The `AGENTSVIEW_UPDATER_PUBKEY` env var was not set at compile time, or the pubkey in `tauri.conf.json` is still `"NOT_SET"`. Rebuild with the correct public key.

### Notarization fails with "invalid credentials"

Verify the API key hasn't been revoked and that `APPLE_API_KEY`, `APPLE_API_ISSUER`, and `APPLE_API_KEY_CONTENT` are all correct. The `.p8` file can only be downloaded once from App Store Connect.

### "Developer ID Application" certificate not found

The signing identity in `APPLE_SIGNING_IDENTITY` must exactly match the identity in the keychain. Run `security find-identity -v -p codesigning` to see available identities.

### Update verification fails after key rotation

Expected. Users on versions compiled with the old public key cannot verify signatures from the new private key. They must download the new version manually from the releases page.
