# Module catalog schema

`catalog.json` is an index of pointers to third-party
`push-tethered-app` process modules (Python, Node.js, or any other
language — see [docs/guides/writing-a-process-module.md](../docs/guides/writing-a-process-module.md)),
not a host: it names a GitHub repo and a release asset filename, and
module authors own their own releases and versioning.

## Trust model

There is no checksum, hash, or signature check on a downloaded release
asset. Installing from the catalog carries the same trust as running any
open-source release binary directly: you are trusting the repo owner and
GitHub's own hosting. `pushapp -catalog-install`/`-catalog-update` and
`pushapp-ui`'s "Browse Catalog" flow both download and extract a tarball
and run whatever `manifest.json`'s `exec` says, exactly like a manual
`-install` would for a hand-downloaded module. Only add an entry for a
repo you trust.

This mirrors the equivalent decision in the sibling `ableton-push-hack`
project's own Push Catalog (see its `catalog/ARCHITECTURE.md`): the
catalog is an index, not a host, and adds no integrity guarantees beyond
what GitHub itself provides.

## Top-level shape

```json
{
  "catalog_version": 1,
  "entries": [ ... ]
}
```

`catalog_version` must be `1`. `pushapp` and `pushapp-ui` refuse to read
a catalog with any other value, so a future breaking change to this
schema can ship as `2` without silently misparsing on older clients.

## Entry fields

| Field         | Required | Meaning                                                                 |
|---------------|----------|--------------------------------------------------------------------------|
| `id`          | yes      | Must match the `id` field in the module's own `manifest.json`.          |
| `name`        | yes      | Display name.                                                            |
| `description` | no       | One line, shown in `-catalog-list` and the UI's catalog browser.        |
| `author`      | no       | Free text.                                                               |
| `homepage`    | no       | URL to the module's own repo or docs.                                   |
| `github_repo` | yes      | `owner/repo`, used to resolve `GET /repos/<github_repo>/releases/latest`.|
| `asset_name`  | yes      | Exact filename of the `.tar.gz`/`.tgz` release asset to download.       |

Example:

```json
{
  "id": "hello-py",
  "name": "Hello (Python)",
  "description": "Minimal Python process module example.",
  "author": "Jane Doe",
  "homepage": "https://github.com/janedoe/hello-py-pushapp",
  "github_repo": "janedoe/hello-py-pushapp",
  "asset_name": "hello-py.tar.gz"
}
```

## Version resolution

There is no separate `release.json` convention — the catalog resolves a
module's current version directly from GitHub's
`releases/latest` API response (`tag_name`), and finds the download URL
from that same response's asset list by matching `asset_name` exactly.
An update check (`-catalog-check-updates`, `-catalog-update`) compares
that resolved version against the installed module's own
`manifest.json` `version` field using this project's own version scheme
(`vMAJOR.MINOR.PATCH[-alpha|...]`, see the root `CLAUDE.md`'s Releases
section) — module authors should tag releases the same way.

## Publishing an entry

1. Write your module as a normal process module (see
   [docs/guides/writing-a-process-module.md](../docs/guides/writing-a-process-module.md)) —
   `manifest.json` at the archive root plus your script and any assets
   (including a `palette.json` if you use one, from
   `go run ./cmd/genpalette`).
2. Tag a release in your own repo and attach a `.tar.gz` of your module
   directory as a release asset. A GitHub Actions workflow that runs on
   a `v*` tag and uploads the tarball via `softprops/action-gh-release`
   works well — see this repo's own `.github/workflows/build.yml` for
   the pattern.
3. Open a pull request against this repo adding one entry to
   `catalog/catalog.json` with your module's `id`, `github_repo`, and
   `asset_name`.

Archive layout: either the archive's root directly contains
`manifest.json`, or the whole tree is wrapped in a single top-level
directory (the shape `git archive` and GitHub's own auto-generated
source tarballs produce) — both are handled automatically.
