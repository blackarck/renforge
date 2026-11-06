# Releasing and code signing

The workflow `.github/workflows/go.yml` builds RenForge for Windows, Linux and macOS (Intel and Apple Silicon) and publishes it to GitHub Releases: every push to `main` updates the **latest** pre-release, and a tag `v*` creates a normal release.

Signing is **optional and driven by repository secrets**. With no secrets the build still works and falls back to the behaviour before: macOS gets an ad-hoc signature, Windows stays unsigned (the workflow prints a warning).

Add secrets under *Settings → Secrets and variables → Actions*.

## macOS: Developer ID signing and notarization

What the job does (`Sign and notarize (macOS)`): imports your certificate into a temporary keychain, signs `renforge.app` with the hardened runtime and a secure timestamp, submits it to Apple's notary service, waits for **Accepted**, staples the ticket (so it opens offline) and checks it with `spctl`. The keychain and key files are deleted at the end.

### Secrets

| Secret | Required | Value |
|---|---|---|
| `MACOS_CERTIFICATE` | yes | Base64 of your **Developer ID Application** certificate exported as `.p12` (with its private key) |
| `MACOS_CERTIFICATE_PASSWORD` | yes | The password you set when exporting the `.p12` |
| `MACOS_SIGN_IDENTITY` | no | Exact identity, e.g. `Developer ID Application: Your Name (TEAMID)`. Detected automatically if omitted |

Notarization credentials: set **one** of the two groups.

| Secret | Value |
|---|---|
| `APPLE_API_KEY`, `APPLE_API_KEY_ID`, `APPLE_API_ISSUER` | App Store Connect API key: base64 of `AuthKey_XXXX.p8`, its key ID, and the issuer ID (preferred for CI) |
| `APPLE_ID`, `APPLE_APP_PASSWORD`, `APPLE_TEAM_ID` | Apple ID e-mail, an [app-specific password](https://support.apple.com/102654), and the 10-character team ID |

If you already sign another app, reuse the same certificate and credentials: the secrets above are the same values.

### Exporting the certificate

On the Mac that has the Developer ID certificate: *Keychain Access → My Certificates → right-click "Developer ID Application: …" → Export…* and choose `.p12`. Then:

```bash
base64 -i DeveloperID.p12 | pbcopy        # paste into MACOS_CERTIFICATE
base64 -i AuthKey_XXXX.p8 | pbcopy        # paste into APPLE_API_KEY (if using an API key)
```

### Checking a downloaded build

```bash
codesign --verify --deep --strict --verbose=2 renforge.app
spctl --assess --type execute --verbose=2 renforge.app    # "accepted, source=Notarized Developer ID"
xcrun stapler validate renforge.app
```

### If notarization fails

The job prints Apple's log. Common causes: the certificate is not a *Developer ID Application* one, the app was not signed with the hardened runtime (the job does this), or the Apple ID/team does not match the certificate.

## Windows: code signing

The job `Sign (Windows)` signs `renforge.exe` with `signtool` (SHA-256, DigiCert timestamp server) when these secrets exist:

| Secret | Value |
|---|---|
| `WINDOWS_CERTIFICATE` | Base64 of a code-signing certificate `.pfx` (PowerShell: `[Convert]::ToBase64String([IO.File]::ReadAllBytes("cert.pfx"))`) |
| `WINDOWS_CERTIFICATE_PASSWORD` | Its password |

**Getting a certificate (none exists yet):**

- **Azure Artifact Signing** (formerly Trusted Signing): low monthly cost, no hardware token, and certificates are managed by Microsoft. Needs a verified identity. Requires replacing the `signtool` step with Microsoft's signing action.
- **A commercial OV or EV certificate** (DigiCert, Sectigo, SSL.com, …): EV gives immediate SmartScreen reputation. Since 2023 the private key must live on a hardware token or in the CA's cloud signing service, so a plain `.pfx` file is usually *not* issued; ask the CA for a cloud-signing option and swap the step accordingly.
- **SignPath Foundation** offers free signing for *open-source* projects, but RenForge's non-commercial licence (CC BY-NC) is not an OSI open-source licence, so it probably does not qualify.

Until then, Windows downloads are unsigned: SmartScreen shows "Windows protected your PC". Users can click *More info → Run anyway*. A signed binary from a new OV certificate may still warn until it builds reputation.

## Linux

Not signed. (Optionally publish SHA-256 checksums next to the release files.)

## Releasing

```bash
git tag v1.2.0 && git push origin v1.2.0   # builds, signs and publishes a normal release
```
Pushes to `main` refresh the *latest* pre-release instead.
