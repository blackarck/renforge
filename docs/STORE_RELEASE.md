# Releasing on the Mac App Store and Microsoft Store

Plan for selling RenForge as a paid app. Status: **not started**. Decide the open questions at the bottom first.

## Summary

| | Mac App Store | Microsoft Store |
|---|---|---|
| Account | Apple Developer Program (yearly fee) | Microsoft Partner Center developer account (one-time fee) |
| Package | signed `.pkg` built with `fyne release -os darwin` | `.appx` built with `fyne release -os windows` (needs the Windows SDK `makeappx.exe`) |
| Sandbox | **Mandatory.** Biggest technical risk, see below | Desktop apps run as "full trust", no file sandbox |
| Signing | "Apple Distribution" + "3rd Party Mac Developer Installer" certificates and a provisioning profile | Microsoft re-signs on submission; local signing needs the publisher identity from Partner Center |
| Notarization | Not used (App Review replaces it) | Not used |
| Review | Apple App Review, typically days | Microsoft certification, typically days |

Fees and policies change: check the current numbers on the Apple and Microsoft developer sites.

## Blocker to solve first: the macOS sandbox

The Mac App Store requires App Sandbox. Fyne's own release template (`entitlements-darwin.plist`) turns the sandbox on and grants **no file access at all**. RenForge renames files in folders the user picks, so as it stands a sandboxed build would not be able to read or rename anything. What is needed:

1. **Custom entitlements**, at minimum `com.apple.security.app-sandbox` plus `com.apple.security.files.user-selected.read-write` (access only to folders/files the user chooses). Fyne does not let you supply your own plist, so the final signing step (`codesign --entitlements`, `productbuild`) has to be scripted by hand instead of `fyne release`.
2. **A native folder picker.** The sandbox only grants access to what the user picks in Apple's own open panel (`NSOpenPanel`). Fyne's built-in folder dialog is not that panel, so access would be denied. Options: a native-dialog library (for example `github.com/ncruces/zenity` or `github.com/sqweek/dialog`), or a small cgo wrapper around `NSOpenPanel`.
3. **Drag and drop** onto the window does grant access, so it keeps working.
4. **"Recent…" folders** stop working across launches unless the app stores **security-scoped bookmarks**, because the sandbox forgets access when the app quits. Without them, Recent must ask the user to pick the folder again.
5. **Undo logs and presets**: the log is written where the user chooses (fine); presets and preferences live in the app's container (fine, but the config path changes).
6. **Photo metadata** is only read from files the user selected, so EXIF reading is fine.

Do a one-day spike before anything else: build a sandboxed test build, pick a folder with the native panel, rename files, undo. If that works, the rest is paperwork.

If the sandbox proves too costly, sell the Mac version directly instead (signed and notarized build from the website with a licence key or a payment link), and use the Microsoft Store for Windows only.

## Microsoft Store

1. Create the Partner Center account and **reserve the product name** "RenForge".
2. Note the **Publisher** identity (looks like `CN=XXXXXXXX-XXXX-...`) and the **Package identity name** shown in Partner Center.
3. Build on a Windows machine with the Windows 10/11 SDK installed:
   ```powershell
   fyne release -os windows -app-id com.blackarck.renforge -app-version 1.0.0 -app-build 1 `
     -developer "CN=<publisher from Partner Center>" -certificate <pfx file> -password <pfx password>
   ```
   Check the generated manifest declares the `runFullTrust` capability (needed for a normal desktop app); edit the manifest if it does not.
4. Upload the package to Partner Center, fill in the listing, submit.

## Store listing checklist (both stores)

- App name, subtitle, description, keywords (reuse the website text, written without "open source")
- **Price** and territories (decision below); optional free trial on the Microsoft Store
- **Screenshots**: `go test -tags screenshots` already renders 2880×1800 PNGs, which fit the Mac App Store size list; Microsoft accepts 1366×768 and larger. Add a Windows-themed run if desired.
- App icon 1024×1024 (`icon-macos.png` for Mac, `icon.png` for Windows)
- **Privacy policy URL**: publish `PRIVACY.md` as a page on the website (for example `/privacy.html`) and update it: RenForge reads photo EXIF locally and stores presets/preferences on the device, no data is collected
- **Support URL**: https://renforgeapp.web.app/#support
- Age rating questionnaire, category (Utilities / Productivity), export compliance (no encryption beyond the standard OS libraries)
- Version number scheme: `1.0.0` and an incrementing build number for every upload

## Licensing and legal

- Third-party notices: the app bundles Fyne (BSD-3), goexif (BSD-2), golang.org/x/* (BSD-3) and others. Include their licence texts (for example a `THIRD_PARTY_LICENSES.md` and a line in About).
- The source is published under CC BY-NC 4.0 with a separate commercial licence. Selling the app yourself is fine because you hold the copyright. If you accept outside contributions, get a **contributor licence agreement** so you keep the right to sell the combined work; the paragraph in `CONTRIBUTING.md` is only a stopgap.
- Keep website wording consistent with the pricing (see questions).

## Engineering tasks

- [ ] Sandbox spike (native folder picker, security-scoped bookmarks) — decides whether the Mac App Store is viable
- [ ] Scripted Mac packaging with custom entitlements, `productbuild`, upload with Transporter
- [ ] Windows `.appx` build in CI on `windows-latest` (install the Windows SDK step, sign with a secret)
- [ ] Version numbers wired into `FyneApp.toml` (`Version`, `Build`)
- [ ] About dialog shows the version and a link to third-party licences
- [ ] Publish privacy page on the website

## Open questions

1. **Price** (and whether to offer a trial).
2. **Do free builds stay on GitHub?** Options: (a) paid in stores, free builds removed, source still visible; (b) paid in stores for convenience and updates, free non-commercial builds stay on GitHub; (c) stores only. This decides the website download buttons and the "free for non-commercial use" wording.
3. **Mac App Store or direct sale** if the sandbox spike is expensive.
4. **Updates and support promise** (free updates for the major version?).
