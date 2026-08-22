# Release Process

This document describes how releases of Apiloom Gateway CE are produced, signed, and published. It is the operational reference for Maintainers cutting a release and for users reasoning about supply-chain provenance.

## Versioning

Apiloom Gateway CE follows [Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html).

- **Patch (`vX.Y.Z` → `vX.Y.Z+1`):** Bug fixes and security patches. No new features. No changes to public APIs (REST, gRPC, plugin SDK, config schema, CLI surface).
- **Minor (`vX.Y.0` → `vX.Y+1.0`):** Backward-compatible additions. New blocks, new endpoints, new CLI flags. Existing config files continue to load. Plugin SDK consumers do not need to recompile.
- **Major (`vX.0.0` → `vX+1.0.0`):** Breaking changes. Requires a migration guide checked in under `docs/migrations/`, a deprecation period of at least one minor release where possible, and a governance vote per [GOVERNANCE.md](../GOVERNANCE.md).

We are pre-1.0. The 0.x line means we reserve the right to break things on minor releases when it is the right call, but we will not do so casually, and every break will be documented in `CHANGELOG.md` and called out in the release notes.

## Cadence

Target cadence:

- **Minor releases:** every 6–8 weeks. The cadence is a target, not a contract; we will slip if a release is not ready.
- **Patch releases:** as needed. Security patches ship within the SLAs in [docs/SECURITY.md](SECURITY.md).
- **Major releases:** when the project demands it. No fixed schedule.

A release is "ready" when:
- All issues in the release milestone are closed or moved.
- CI is green on `main` for at least 24 hours.
- The benchmark suite (`make bench`) shows no unexplained regression vs. the previous release baseline.
- `CHANGELOG.md` is updated and reviewed by at least one Maintainer other than the release manager.
- The release notes draft has been posted to a release issue and left open for at least 48 hours for Maintainer feedback.

## Who can cut a release

| Release type | Who | Mechanism |
|---|---|---|
| Patch (`vX.Y.Z+1`) | Any Maintainer | Push the tag |
| Minor (`vX.Y+1.0`) | Any Maintainer, after lazy consensus on the release issue | Push the tag |
| Major (`vX+1.0.0`) | After a successful governance vote (two-thirds, 7-day window) | Push the tag |

Reviewers do not cut releases. Contributors do not cut releases. The release manager for a given release is the Maintainer who pushes the tag; that person owns shepherding the release through to published artifacts.

## Mechanical steps

1. **Create the release branch** (minor and major only):
   ```bash
   git checkout -b release/vX.Y.0
   git push -u origin release/vX.Y.0
   ```
   For patch releases, work directly on `main` or cherry-pick to a maintenance branch (`release/vX.Y`).

2. **Update `CHANGELOG.md`:**
   - Move items from `[Unreleased]` into a new `[X.Y.Z] -- YYYY-MM-DD` section.
   - Add a `[X.Y.Z]` link reference at the bottom.
   - Open a PR titled `chore(release): vX.Y.Z`. Get one Maintainer review.

3. **Tag and push:**
   ```bash
   git checkout main && git pull
   git tag -a vX.Y.Z -m "Apiloom Gateway CE vX.Y.Z"
   git push origin vX.Y.Z
   ```
   The tag must follow `vX.Y.Z` exactly (lowercase `v`, no suffixes for stable releases). Pre-releases use `vX.Y.Z-rc.N`, `vX.Y.Z-beta.N`, etc., per semver.

4. **Watch the release workflow.** Pushing the tag triggers `.github/workflows/release.yml`. The workflow runs the test suite, then builds and publishes:
   - Multi-architecture (amd64, arm64) container images for the gateway and control plane to `ghcr.io/mindfulcto-labs/apiloom-ce/{gateway,controlplane}:vX.Y.Z` and `:latest`.
   - Cosign keyless signatures (Sigstore OIDC) on each image.
   - SPDX SBOMs for each image, generated with `syft`, attached to the GitHub Release.
   - Pre-built binaries for `linux/amd64`, `linux/arm64`, and `darwin/arm64` as `.tar.gz` archives.
   - The Helm chart packaged and pushed to `oci://ghcr.io/mindfulcto-labs/apiloom-ce/charts`.
   - A GitHub Release with the changelog snippet rendered by `git-cliff`.

5. **Verify artifacts:**
   ```bash
   # Image signatures (replace with real digest)
   cosign verify ghcr.io/mindfulcto-labs/apiloom-ce/gateway:vX.Y.Z \
     --certificate-identity-regexp "https://github.com/mindfulcto-labs/apiloom-ce" \
     --certificate-oidc-issuer "https://token.actions.githubusercontent.com"

   # Pull and smoke test
   docker pull ghcr.io/mindfulcto-labs/apiloom-ce/gateway:vX.Y.Z
   docker run --rm ghcr.io/mindfulcto-labs/apiloom-ce/gateway:vX.Y.Z --version
   ```

6. **Publish the announcement:**
   - GitHub Release: workflow auto-creates this; the release manager edits the body to highlight the 3–5 most important changes.
   - Discord/Slack `#announcements`: short summary + link to the release.
   - Blog post (`apiloom.io/blog`): for minor and major releases only. The Developer Advocate is responsible.
   - Social: optional and tasteful.

7. **Bump `[Unreleased]` in `CHANGELOG.md`** if not already done. Open a small PR.

## Supply-chain provenance

Every release ships with:

- **Signed container images.** Cosign keyless signing using GitHub OIDC. Verifiable with the `cosign verify` command above. The signing identity is the release workflow run, not a long-lived key, which means there is no key for an attacker to steal.
- **SPDX SBOM.** One per image, attached to the GitHub Release. Generated by `syft` from the built image. Includes every Go module in the binary.
- **Vulnerability scan history.** `govulncheck` runs on every CI build (`.github/workflows/security.yml`). Releases do not ship if the scan flags a known unfixed vulnerability in a direct dependency.
- **Reproducible builds.** All builds use `CGO_ENABLED=0` and `-trimpath`. Given the same source commit and Go toolchain version, two builds produce byte-identical binaries.

We do not (yet) publish in-toto attestations or SLSA provenance. That is on the roadmap; tracked in the CNCF gap analysis.

## Backports and security patches

- The current minor release receives security patches.
- The previous minor release receives security patches on a best-effort basis for 90 days after the next minor ships.
- Older minor releases do not receive patches. Users on older versions are expected to upgrade.

Security patches follow the timeline in [docs/SECURITY.md](SECURITY.md). The release process for a security patch is the same as for a normal patch release, but coordinated with the reporter and (if applicable) downstream distributors under embargo.

## Yanking a release

A release can be yanked from the GitHub Releases page (set to draft) and from container registries (delete the tag, retain the digest). We do not delete the git tag itself, because tags are part of the public history.

Yanking is reserved for releases that:
- Contain a critical security regression that cannot be fixed forward immediately.
- Were tagged from the wrong commit.
- Contain credentials or other secrets accidentally included in artifacts.

A yank is announced in the affected release's GitHub Release body, in `#announcements`, and in `CHANGELOG.md` under a new patch release entry that explains the yank.

## Changelog format

`CHANGELOG.md` follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Each release section uses the standard headings (`Added`, `Changed`, `Deprecated`, `Removed`, `Fixed`, `Security`) where they apply. Commits should follow [Conventional Commits](https://www.conventionalcommits.org/) so that `git-cliff` can render an accurate first draft for the release notes.

The release manager is expected to edit the auto-rendered notes. A release-notes summary that reads like a list of commits is a release-notes summary that the release manager did not finish.

## What this document does not cover

- The **Apiloom Platform** (the enterprise product) has a separate release process. This document is scoped to Apiloom Gateway CE.
- The **docs site** (`docs/docs/`) ships continuously from `main`. There is no docs versioning yet; we will add it when there is a real reason to.
- The **Helm chart** is currently versioned in lockstep with the gateway. We may decouple this in the future.
