# Update check for pushapp-ui

Status: Stage 1 built 2026-10-06 (code and docs done, not yet tested in a
released build). Stages 1.5 and 2 not started.

## Goal

Tell the user when a newer release exists on GitHub. Later, optionally
download and install it.

## Stage 1 (MVP): banner with a link

Effort: about half a day.

What exists already:

- `internal/catalog` fetches data from GitHub releases.
- `internal/version.Version` holds the running version. It is "dev" unless
  the build sets it with `-ldflags`.
- `PushService.OpenMirror` already opens a URL in the system browser
  through Wails' `Browser.OpenURL`.

What to add:

- A small `internal/updatecheck` package:
  - Call `GET /repos/federico-pepe/push-tethered-app/releases`. Do not use
    `/releases/latest`: it ignores pre-releases, and every current tag is
    `-alpha` or `-beta`.
  - Pick the release with the highest version, not the first one in the
    list. GitHub orders the list by creation date, so a hotfix to an older
    line could come first. Skip drafts.
  - Write a new version comparison in `internal/updatecheck`. Do not reuse
    `catalog.CompareVersions`: it drops the pre-release suffix, so it
    treats `0.1.5-alpha` and `0.1.5-beta` as equal. The new comparison
    follows semver: `alpha < beta < rc.1 < rc.2 < (no suffix)`, with
    numeric parts compared as numbers.
  - Pre-release policy: a user on a stable build sees only stable
    releases. A user on a pre-release build sees all releases.
  - Return the newer version and its release page URL.
- A `PushService` method that runs the check.
- A banner in `cmd/pushapp-ui/frontend/src/main.ts` with an "Open release
  page" button and a dismiss button.
- Run the check in the background with a short timeout. If the network is
  down, show nothing. Skip the check when `Version` is "dev".
- A setting to turn the check off. It is stored in `settings.json` in the
  app's config directory, and a checkbox in the Settings panel changes it.
- A short section in `MANUAL.md`: what the check does, and how to turn it
  off.
- Unit tests for version comparison, and for the HTTP call with a mocked
  server.

Open items:

- **Done in Stage 1:** the UI build did not set `Version`. Only the
  `cmd/pushapp` step in `build.yml` passed `-X ...version.Version`. The
  `pushapp-ui` Taskfiles used `-ldflags="-w -s"`, so a release build
  reported "dev" and the check would never run. The Taskfiles now add the
  `-X` flag from the `APP_VERSION` environment variable, which `build.yml`
  already sets from the tag.
- Decided: check at start only. The banner stays until dismissed, and
  dismiss lasts until the next start.
- GitHub's unauthenticated API allows 60 requests per hour per IP. One
  check per start is well inside that limit.

## Stage 1.5 (suggested): download button

Add a "Download" button that fetches the right release asset for the
user's OS and architecture into the Downloads folder, then shows it in the
file manager. The user installs it by hand. This gives most of the value
of auto-install without code signing or helper programs.

- Release assets are zips that contain the `.dmg`, the NSIS `.exe`, or the
  AppImage. Pick the asset by OS and architecture (the Raspberry Pi
  `linux-arm64` asset is separate).
- Publish SHA-256 checksums in the release job, and verify them after the
  download. The release job does not publish checksums today.

## Stage 2: automatic install

Effort: days to weeks. Do this only after Stage 1 and after code signing
is in place.

Blockers, per platform:

- **macOS:** the app is ad-hoc signed and not notarized. A replaced `.app`
  hits Gatekeeper and quarantine. This needs a Developer ID certificate
  and notarization, or a Sparkle-style flow. A running app cannot replace
  itself in `/Applications`, so a helper must swap it after quit and
  relaunch.
- **Windows:** run the NSIS installer silently, or use a helper that
  replaces the exe after exit. Admin rights depend on the install
  location. An unsigned exe also has a SmartScreen reputation problem.
- **Linux:** an AppImage can replace itself (`appimageupdate`, or swap the
  file). The Raspberry Pi arm64 build needs its own asset.
- **Hardware safety:** the app owns USB devices. An update must stop all
  sessions cleanly first and clear the LEDs on the way out, as the safety
  rules in `CLAUDE.md` require.
- **Supply chain:** installing downloaded code needs signed releases and
  signature verification, not only a checksum from the same release page.

## Decision

Build Stage 1 first. Add Stage 1.5 if users ask for it. Leave Stage 2
until signing and notarization exist.
