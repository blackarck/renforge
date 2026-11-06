# Signing migration instructions

Checklist for moving code signing from the other app's setup to RenForge's GitHub Actions workflow (`.github/workflows/go.yml`). Details and background: [RELEASING.md](RELEASING.md).

## A. macOS (reuse the existing Developer ID setup)

1. **Collect what the other app already uses**: the Developer ID Application certificate (+ private key), its password, the Apple team ID, and how it notarizes (Apple ID + app-specific password, or an App Store Connect API key).
2. **Export the certificate** from Keychain Access (*My Certificates → Developer ID Application: … → Export…* as `.p12`), then:
   ```bash
   base64 -i DeveloperID.p12 | pbcopy
   ```
3. **Add repository secrets** (*Settings → Secrets and variables → Actions*):
   - `MACOS_CERTIFICATE` = the base64 text, `MACOS_CERTIFICATE_PASSWORD` = the `.p12` password
   - `MACOS_SIGN_IDENTITY` (optional) = e.g. `Developer ID Application: Your Name (TEAMID)`
   - **Either** `APPLE_API_KEY` (base64 of `AuthKey_XXXX.p8`), `APPLE_API_KEY_ID`, `APPLE_API_ISSUER` **or** `APPLE_ID`, `APPLE_APP_PASSWORD`, `APPLE_TEAM_ID`
4. **Compare with the other app's commands.** The workflow signs with `codesign --force --deep --options runtime --timestamp`, notarizes with `xcrun notarytool submit --wait`, then runs `xcrun stapler staple`. If the other app also uses entitlements, a DMG or a different bundle ID, add that to the *Sign and notarize (macOS)* step.
5. **Run the workflow** (push to `main` or *Actions → Run workflow*) and check the macOS job log for "Signed, notarized and stapled."
6. **Verify a downloaded build** on a Mac that has never seen it:
   ```bash
   codesign --verify --deep --strict --verbose=2 renforge.app
   spctl --assess --type execute --verbose=2 renforge.app   # expect: Notarized Developer ID
   xcrun stapler validate renforge.app
   ```
7. Remove the "ad-hoc only" warning from your mind: it only appears when `MACOS_CERTIFICATE` is missing.

## B. Windows (no certificate yet)

1. **Choose a signing route** (see RELEASING.md): Azure Artifact Signing, or a CA's cloud/EV signing service. A plain `.pfx` is rarely issued for new certificates.
2. **If you get a `.pfx`**: add `WINDOWS_CERTIFICATE` (base64) and `WINDOWS_CERTIFICATE_PASSWORD`. The *Sign (Windows)* step then signs `renforge.exe` automatically.
3. **If you use a cloud service** (Azure Artifact Signing, SSL.com eSigner, DigiCert KeyLocker, …): replace the `signtool` command in *Sign (Windows)* with that provider's action or CLI, keeping the step's position (after *Package (Windows)*, before *Create artifacts (Windows)*).
4. **Verify**: right-click `renforge.exe` → *Properties → Digital Signatures*, or `signtool verify /pa /v renforge.exe`. SmartScreen may still warn until the certificate builds reputation (EV avoids this).

## C. Finish

- Tag a release (`git tag v1.x.y && git push origin v1.x.y`) once both platforms verify.
- Update the *Signing status* note in `readme.md` when Windows is signed.
