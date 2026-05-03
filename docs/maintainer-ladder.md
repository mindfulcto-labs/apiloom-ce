# Maintainer Ladder

This document defines the contributor roles in Apiloom Gateway CE, the criteria for moving between them, and the process for nominations, votes, and removals. It is the operational complement to [GOVERNANCE.md](../GOVERNANCE.md), which establishes the project's vision and decision-making norms.

The ladder has three rungs: **Contributor**, **Reviewer**, and **Maintainer**. The path is open. Anyone who shows up consistently and does the work can move up.

## Contributor

A Contributor is anyone who has engaged constructively with the project. You do not need a merged pull request to be a Contributor.

**You are a Contributor if you have done any of the following:**
- Filed a substantive issue (a real bug report, a thoughtful feature request, a documentation gap with context).
- Reviewed a pull request.
- Helped another user in GitHub Discussions or the project chat.
- Submitted a pull request -- merged or not.

**Responsibilities:**
- Follow the [Code of Conduct](../CODE_OF_CONDUCT.md).
- Read [CONTRIBUTING.md](../CONTRIBUTING.md) before opening a PR.
- Sign your commits (`git commit -s`) once we adopt DCO. Tracked separately.

**Recognition:** Contributors are listed automatically in the GitHub contributor graph. We do not maintain a separate file. People who contribute substantially over time get nominated to Reviewer.

## Reviewer

A Reviewer has demonstrated they understand a meaningful slice of the codebase well enough to give useful feedback on pull requests. Reviewers do not have merge rights, but their `LGTM` carries weight, and a maintainer will usually defer to a Reviewer on changes inside their area.

### Criteria

To be eligible for nomination, you should typically have:

- **5+ merged pull requests** of non-trivial scope (a typo fix does not count; a new test for an existing block does).
- **A clear area of focus** -- e.g., the `auth` block, the `ratelimit` block, the control plane API, the Helm chart, the docs site. Reviewers are scoped; you are not expected to know the whole codebase.
- **Demonstrated good judgment in PR review:** comments that are technical, specific, and actionable. Reviewers are nominated as much for *how* they review as for *whether* they ship code.
- **Sustained activity:** consistent presence over at least 8 weeks. We are looking for people who will still be here in six months.

### Nomination and confirmation

1. Any Maintainer may nominate a Contributor by opening an issue titled `Reviewer nomination: <name>` with a short justification (link to PRs, areas of focus, examples of good review feedback).
2. The nomination is open for **5 business days** for other Maintainers to comment.
3. Confirmation is by **lazy consensus**: if no Maintainer objects within 5 business days, the nomination passes. One objection from a Maintainer triggers a discussion; if it cannot be resolved, the nomination fails for now and can be re-raised in 90 days.
4. On passing, the new Reviewer is added to `MAINTAINERS.md` (in a "Reviewers" section), granted the GitHub `triage` permission on the repository, and announced in the next release notes.

### Responsibilities

- Approve pull requests within their area (`LGTM` from a Reviewer counts toward Maintainer review).
- Triage and label issues.
- Mentor first-time contributors.
- Help maintain `good first issue` candidates.

### Stepping down

A Reviewer can step down at any time by opening a PR removing themselves from `MAINTAINERS.md`. There is no stigma. If you need a break, take one.

## Maintainer

A Maintainer has merge rights, can cut releases, and votes on governance changes. Maintainers are the people the project ultimately relies on; the bar is correspondingly higher.

### Criteria

To be eligible for nomination, you should typically have:

- **Served as a Reviewer for at least 3 months** with consistent activity.
- **Deep familiarity with the codebase** beyond a single area. Maintainers are expected to be able to triage issues and reason about cross-cutting concerns (e.g., "does this PR affect the LISTEN/NOTIFY hot-reload path?").
- **A track record of good architectural judgment**, demonstrated in PR review, design discussions, or RFCs.
- **Public alignment with the project's vision** as expressed in [GOVERNANCE.md](../GOVERNANCE.md) -- in particular, the free-core commitment.
- **A real time commitment.** Maintainers are expected to be reachable on issues and PRs within a few business days, on average.

### Nomination and confirmation

1. Any existing Maintainer may nominate a Reviewer by opening a private discussion among the current Maintainers (not a public issue, to avoid putting the candidate on the spot if it does not pass).
2. Maintainers discuss for up to **10 business days**.
3. Confirmation requires **unanimous approval** from current Maintainers.
4. On passing, the candidate is asked privately whether they accept. If they accept, they are added to `MAINTAINERS.md`, granted GitHub `maintain` permission on the repository, added to the security disclosure list, and announced in the next release notes.

A failed nomination is communicated privately to the nominee with constructive feedback. A nomination can be re-raised after 90 days.

### Responsibilities

- Merge pull requests after appropriate review.
- Cut releases per [docs/release-process.md](release-process.md).
- Vote on governance changes (see [GOVERNANCE.md](../GOVERNANCE.md)).
- Triage security reports sent to `security@apiloom.io`.
- Set roadmap priorities and accept/reject design proposals.
- Enforce the Code of Conduct in the spaces they moderate.

### Maintainer diversity

CNCF Incubating requires maintainer diversity across organizations. We treat this as a hard requirement: **no more than 50% of active Maintainers may be employed by the same organization** at any time. If a hire or departure would put the project over that line, the project pauses new Maintainer additions from that organization until balance is restored, and prioritizes recruitment from underrepresented organizations.

### Stepping down

A Maintainer can step down voluntarily at any time by opening a PR moving themselves to the "Emeritus Maintainers" section of `MAINTAINERS.md`. Emeritus Maintainers retain credit but no merge rights or votes.

### Inactivity

A Maintainer who has not reviewed a PR, merged a PR, voted on a governance issue, or otherwise engaged with the project for **6 consecutive months** will be moved to Emeritus by lazy consensus among the active Maintainers, after a private check-in. This is a housekeeping action, not a punishment. The path back is a fresh nomination at the Reviewer or Maintainer level, depending on circumstances.

### Removal for cause

A Maintainer may be removed for serious violations of the Code of Conduct, repeated breaches of governance norms, or actions taken in bad faith against the project. Removal requires a **two-thirds vote** of all current Maintainers excluding the subject of the vote, and a public statement of the cause once the process concludes. This has never been needed and we hope it never is.

## How decisions get made

Quick reference. See [GOVERNANCE.md](../GOVERNANCE.md) for the canonical version.

| Decision type | Who decides | Mechanism |
|---|---|---|
| Merge a PR | One Maintainer (or one Reviewer in their area + one Maintainer) | Lazy consensus, 48-hour window for objections from other Maintainers |
| Cut a patch release (`vX.Y.Z+1`) | Any Maintainer | No vote |
| Cut a minor release (`vX.Y+1.0`) | Maintainer consensus on readiness | Lazy consensus on the release issue |
| Cut a major release (`vX+1.0.0`) | Maintainer vote + migration guide | Two-thirds majority, 7-day open window |
| Promote Contributor → Reviewer | Maintainer nomination | Lazy consensus, 5 business days |
| Promote Reviewer → Maintainer | Maintainer nomination | Unanimous Maintainer approval |
| Governance changes | Maintainer vote | Two-thirds majority, 7-day open window |
| Architectural decisions | Maintainer consensus | ADR PR, discussed publicly |
| Code of Conduct enforcement | CoC committee (currently the Maintainers) | Per the Code of Conduct |

## Questions

If anything here is unclear, open a discussion or send a note to the Maintainers via `hello@apiloom.io`. The point of writing this down is to make the path legible. If it is not, that is a bug in this document.
