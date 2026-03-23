# FlexGateway Governance

## Project Vision

FlexGateway is an open-source API intelligence platform. It provides a high-performance gateway core, AI-native routing, plugin extensibility, and a control plane for managing API infrastructure.

## Public Commitment: Free Core

Authentication, rate limiting, and mTLS remain in the free core permanently. This boundary is defined publicly and will not change.

These capabilities are foundational to running a secure API gateway. Gating them behind a paid tier would undermine the usefulness of the open-source project. They ship with the core binary and always will.

## Contributor Ladder

### Contributor

Anyone with a merged pull request.

**Responsibilities:**
- Submit pull requests following the [contribution guide](CONTRIBUTING.md)
- Participate in issue discussions and code reviews
- Report bugs and suggest improvements

### Reviewer

5+ merged pull requests, nominated by a maintainer.

**Responsibilities:**
- Approve pull requests
- Triage and label issues
- Mentor new contributors
- Maintain code quality in their area of expertise

**How to become a Reviewer:** A maintainer nominates you after observing consistent, quality contributions. The nomination is discussed among maintainers and decided by lazy consensus.

### Maintainer

Demonstrated sustained contribution and deep understanding of the codebase and project direction.

**Responsibilities:**
- Merge pull requests
- Cut releases
- Vote on governance changes
- Define roadmap priorities
- Enforce code standards and architectural decisions

**How to become a Maintainer:** Existing maintainers nominate candidates. Requires unanimous approval from current maintainers.

## Decision-Making

**Code changes:** Lazy consensus. A pull request can be merged after one maintainer approval and passing CI, unless another maintainer raises an objection within 48 hours.

**Governance changes:** Require a formal vote among maintainers. Changes pass with a two-thirds majority. The vote must remain open for at least 7 days to allow asynchronous participation.

**Architectural decisions:** Documented as ADRs (Architecture Decision Records) in the repository. Proposed via pull request, discussed openly, decided by maintainer consensus.

## Release Process

FlexGateway follows semantic versioning (semver.org).

- **Patch releases** (v0.1.1): Bug fixes, no API changes. Any maintainer can cut these.
- **Minor releases** (v0.2.0): New features, backward-compatible. Require maintainer consensus on readiness.
- **Major releases** (v1.0.0+): Breaking changes. Require governance vote and migration guide.

Releases are created by pushing a version tag (`v*.*.*`). The release workflow builds container images, generates a changelog, and publishes a GitHub Release.

## Security Policy

**Reporting vulnerabilities:** Send reports to security@flexgateway.dev. Do not open public issues for security vulnerabilities.

**Disclosure timeline:**
1. Report received -- acknowledgment within 48 hours.
2. Assessment and fix development -- target within 30 days.
3. Coordinated disclosure at 90 days, regardless of fix status.
4. CVE assigned if applicable.

**Supported versions:** The latest minor release receives security patches. Older versions receive patches on a best-effort basis.

## Code of Conduct

All participants are expected to follow the [Code of Conduct](CODE_OF_CONDUCT.md).

## License

FlexGateway is licensed under the Apache License 2.0.
