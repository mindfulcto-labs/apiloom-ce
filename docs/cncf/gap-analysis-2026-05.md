# CNCF Sandbox Prerequisite Gap Analysis

**Project:** apiloom-ce
**Date:** 2026-05-03
**Author:** Developer Advocate
**Reviewer:** CEO
**Target:** CNCF Sandbox application (day 75 — 2026-07-17)

This document audits apiloom-ce against the [CNCF Sandbox](https://github.com/cncf/sandbox) entry criteria and the heavier requirements that follow at the [Incubating](https://github.com/cncf/toc/blob/main/process/graduation_criteria.md#incubating-stage) stage. Each row lists current status (Present / Polish / Missing), what's wrong if anything, the recommended owner, and a rough effort estimate.

The sandbox criteria themselves are short — a TOC sponsor, OSI-approved license, governance, security disclosure, code of conduct, and a working project. The expensive work is in the polish pass: every doc currently uses the old "FlexGateway" branding, contact emails point at three different domains, and several CNCF-expected files (MAINTAINERS, ADOPTERS, ROADMAP) don't exist yet.

## Summary

- **9 items present** (license, README, contributing, code of conduct, governance, security policy, public CI, issue/PR templates, supply-chain hygiene)
- **5 items need polish** (branding drift, contact email mismatches, missing enforcement contact, broken cross-repo links, README CI badge)
- **5 items missing** (MAINTAINERS.md, ADOPTERS.md, ROADMAP.md, DCO/CLA decision, TOC sponsor identification)

The blocker for submission is **TOC sponsor identification + MAINTAINERS.md with ≥2 maintainers**. Everything else is mechanical.

---

## Sandbox Prerequisites

### 1. LICENSE

- **Status:** Present
- **What's there:** Apache License 2.0 in `LICENSE` at repo root. OSI-approved, CNCF-compatible.
- **Action:** None.

### 2. README.md

- **Status:** Polish
- **What's there:** Polished README with quick-start, performance numbers, architecture diagram, plugin example. Apiloom-branded.
- **Gaps:**
  - No CI status badge. CNCF reviewers expect a green CI badge on the homepage.
  - No CNCF Sandbox badge (will add post-acceptance).
  - Quick-start references `bin/fgctl`, but binary should be `apiloom-ctl` or similar to match the project name. Same drift on env vars (`FLEXGW_*`, `FLEXCP_*`).
- **Owner:** Developer Advocate
- **Effort:** S (≤2h) for badges + binary/env naming pass

### 3. CONTRIBUTING.md

- **Status:** Polish
- **What's there:** Solid prereqs, dev workflow, TDD policy, code standards, PR process, project structure.
- **Gaps:**
  - Uses "FlexGateway" branding throughout.
  - Clone URL is `https://github.com/<your-username>/flex-gateway.git` — wrong repo name.
  - References `CLAUDE.md` for "the full set of rules" — this file is not in the repo and is internal tooling. Either inline the rules or remove the reference.
  - `FLEXGW_DATABASE_URL` env var.
- **Owner:** Developer Advocate
- **Effort:** S (≤2h)

### 4. CODE_OF_CONDUCT.md

- **Status:** Polish
- **What's there:** Contributor Covenant 2.1, abridged.
- **Gaps:**
  - **No enforcement contact.** "Reported to the project maintainers" is not actionable. Must list a specific email (e.g., `conduct@apiloom.io`) per CNCF expectations.
  - Recommend switching to the full Contributor Covenant 2.1 text rather than the summary, with a clear enforcement ladder.
- **Owner:** Developer Advocate (with CEO approval on contact email + ladder)
- **Effort:** S (≤1h)

### 5. GOVERNANCE.md

- **Status:** Polish
- **What's there:** Project vision, public commitment to free core, contributor ladder (Contributor / Reviewer / Maintainer), decision-making process, release process, security policy summary.
- **Gaps:**
  - "FlexGateway" branding throughout.
  - Lists `security@flexgateway.dev` as the security contact, while `SECURITY.md` lists `security@flexgateway.io`. **Both are wrong** — should be `security@apiloom.io`. CNCF reviewers will flag this immediately.
  - "Contributor" rung says "anyone with a merged pull request," which conflicts with CNCF-style ladders that include pre-merge contributors. Minor wording fix.
- **Owner:** Developer Advocate (with CTO approval on the free-core commitment language and Chief Architect on the release process section)
- **Effort:** S (≤2h)

### 6. SECURITY.md (with disclosure email)

- **Status:** Polish
- **What's there:** Two copies — `/SECURITY.md` and `/.github/SECURITY.md` (identical). Reporting email, response timeline, PGP key URL, security.txt URL, RFC 9116 reference.
- **Gaps:**
  - Disclosure email `security@flexgateway.io` is on a domain we don't own. **Must change to `security@apiloom.io` and the inbox must be live and monitored before submission.**
  - PGP URL (`flexgateway.io/.well-known/security/pgp`) and security.txt URL likewise wrong domain.
  - Two copies of the file will eventually drift. Keep one canonical version (`/SECURITY.md`) and delete `/.github/SECURITY.md` (GitHub falls back to root).
  - Cross-link to `docs/SECURITY.md` is dead — `docs/SECURITY.md` doesn't exist. Either create the long-form policy or remove the link.
- **Owner:** CTO (decide & provision the inbox + PGP), Developer Advocate (writing/cleanup)
- **Effort:** M (≤1d) — depends on CTO provisioning the email + PGP key

### 7. MAINTAINERS.md (≥2 maintainers)

- **Status:** Missing
- **Why it matters:** CNCF Sandbox does not formally require ≥2 maintainers, but Incubating does, and reviewers strongly prefer to see at least 2 named maintainers from different employers at submission. A single-maintainer project signals bus-factor risk.
- **What's needed:** `MAINTAINERS.md` listing each maintainer with name, GitHub handle, employer, area of ownership, and contact. Today the project has effectively one maintainer (CTO). Cross-org recruitment is a 90-day OKR.
- **Owner:** CTO (recruit maintainers), Developer Advocate (file)
- **Effort:** L (≤4w wall-clock for recruitment, ≤1h for file)
- **Critical path:** Yes. Single biggest gap. Start now.

### 8. ADOPTERS.md (placeholder)

- **Status:** Missing
- **Why it matters:** Sandbox accepts a placeholder; Incubating requires 3+ end-user testimonials. Starting the file early makes it easy to add adopters as they appear.
- **What's needed:** `ADOPTERS.md` with header, contribution instructions, and an empty table or a list of self-attested early adopters (even staging/pre-prod is fine for Sandbox).
- **Owner:** Developer Advocate (template), CMO (adopter outreach), CEO (final sign-off on which logos)
- **Effort:** S (≤2h for the file itself)

### 9. Documented Release Process

- **Status:** Present (split across two docs) — Polish
- **What's there:** Release section in `GOVERNANCE.md` + `.github/workflows/release.yml` (multi-arch container build, cosign signing, syft SBOM, Helm chart push, GitHub Release).
- **Gaps:**
  - No standalone `RELEASE.md` at repo root. CNCF reviewers expect one. The release section in GOVERNANCE.md is buried.
  - Release workflow tars binaries as `flexgateway-*.tar.gz` and pushes Helm chart `charts/flexgateway` — names need to match the project.
  - No documented release cadence. Recommend committing to a stable cadence (e.g., minor every 6–8 weeks) before the application.
  - No `CHANGELOG.md` checked in. Workflow generates one via `git-cliff` per release, which is fine, but a top-level rolling changelog improves discoverability.
- **Owner:** Senior Go Engineer (workflow renames), Developer Advocate (RELEASE.md + cadence)
- **Effort:** M (≤1d)

### 10. Public CI Status

- **Status:** Present — Polish
- **What's there:** `.github/workflows/ci.yml` runs lint, test (matrix on Postgres 15/16/17), build, web build, benchmark regression check, and staging deploy. `.github/workflows/security.yml` runs `govulncheck`/`gosec`. All public on GitHub Actions.
- **Gaps:**
  - No CI status badge in README (see item 2).
  - CI references `flexgateway_test` database name and pushes to `flexgateway:ci` image tag — naming drift again.
  - Coverage uploads to Codecov — confirm the org and badge are public.
- **Owner:** Senior Go Engineer
- **Effort:** S (≤2h)

### 11. Roadmap

- **Status:** Missing
- **Why it matters:** Sandbox application explicitly asks for roadmap context. Without one, the TOC has no idea where the project is heading.
- **What's needed:** `ROADMAP.md` at repo root with current quarter's themes, accepted near-term items, and parking-lot items. Should match the public 90-day plan. Doesn't need to be detailed — a one-pager is fine.
- **Owner:** CTO (themes), Developer Advocate (file + maintenance)
- **Effort:** S (≤2h)

### 12. TOC Sponsor

- **Status:** Missing
- **Why it matters:** Sandbox applications without a TOC sponsor get auto-deferred. This is the single most important external dependency.
- **What's needed:** Identify a TOC member willing to sponsor, brief them, get a written commitment. Practically: who do we know on the TOC? Who is plausibly interested in API gateway / API intelligence space?
- **Owner:** CEO (relationship), CTO (technical pitch), Developer Advocate (briefing materials)
- **Effort:** L (≤6w wall-clock)
- **Critical path:** Yes.

---

## Adjacent CNCF-Expected Items

These aren't on the explicit checklist in the issue, but reviewers look for them.

### 13. DCO or CLA

- **Status:** Missing
- **What's there:** No `dco.yml`, no CLA bot configured. PR template doesn't ask for a sign-off line.
- **Decision needed:** CNCF prefers DCO over CLA. Recommend adopting DCO via the `dco-check` GitHub App + adding `git commit -s` instructions to CONTRIBUTING.md.
- **Owner:** CTO (decide DCO vs CLA), Developer Advocate (configure + document)
- **Effort:** S (≤2h once decided)

### 14. Issue & PR Templates

- **Status:** Present
- **What's there:** `.github/ISSUE_TEMPLATE/{bug_report,feature_request,plugin_request}.md` and `.github/PULL_REQUEST_TEMPLATE.md`.
- **Action:** None for Sandbox. Will need a security-issue template that redirects to private disclosure later.

### 15. Supply-Chain Hygiene

- **Status:** Present
- **What's there:** Cosign signing on release images, SPDX SBOMs (syft), `govulncheck`/`gosec` workflow, dependabot. This is already above CNCF Sandbox bar — useful proof point in the application narrative.
- **Action:** None.

### 16. CHANGELOG.md

- **Status:** Missing
- **What's there:** Auto-generated per release via git-cliff. No rolling changelog at repo root.
- **Recommendation:** Commit a `CHANGELOG.md` and adopt Keep a Changelog format. Improves discoverability for users browsing the repo.
- **Owner:** Senior Go Engineer
- **Effort:** S (≤1h)

---

## Recommended Next Actions (priority order)

1. **CTO/CEO:** Identify TOC sponsor candidates and start outreach. (Critical path, longest lead time.)
2. **CTO:** Provision `security@apiloom.io`, `conduct@apiloom.io`, PGP key, and `security.txt` on apiloom.io. (Unblocks polish work on SECURITY/COC/GOVERNANCE.)
3. **CTO:** Begin maintainer recruitment (cross-org, ≥1 external). (Critical path.)
4. **Developer Advocate:** Branding pass across CONTRIBUTING/GOVERNANCE/SECURITY/CODE_OF_CONDUCT once email decisions land.
5. **Developer Advocate:** Create `MAINTAINERS.md`, `ADOPTERS.md`, `ROADMAP.md`, `RELEASE.md`, `CHANGELOG.md` skeletons.
6. **Senior Go Engineer:** Release workflow + CI naming pass (flexgateway → apiloom).
7. **CTO:** DCO vs CLA decision; Developer Advocate to implement.

## Effort Roll-up

| Owner | Effort |
|---|---|
| CTO | M–L: TOC sponsor outreach, maintainer recruitment, infra (email/PGP), DCO decision |
| CEO | S–M: TOC sponsor relationship, ADOPTERS approval |
| Chief Architect | S: review GOVERNANCE release section |
| Senior Go Engineer | M: release workflow rename, CHANGELOG, CI naming |
| Developer Advocate | M: file skeletons + branding pass + roadmap + briefing materials |
| CMO | S–M: adopter outreach |

Total mechanical work (Developer Advocate + Senior Go Engineer): ≤3 days. Total wall-clock to submission readiness: gated entirely by TOC sponsor confirmation and second maintainer.
