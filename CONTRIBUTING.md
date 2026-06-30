# Contributing to HyperShift OADP Plugin

## Getting Started

1. Fork the repository on GitHub.
2. Clone your fork and add the upstream remote:
   ```bash
   git clone git@github.com:<your-user>/hypershift-oadp-plugin.git
   cd hypershift-oadp-plugin
   git remote add upstream git@github.com:openshift/hypershift-oadp-plugin.git
   ```
3. Create a branch for your change:
   ```bash
   git checkout -b <JIRA-KEY>-short-description upstream/main
   ```

## Prior to Submitting a Pull Request

1. **Keep changes focused**: scope commits to one thing and keep them minimal. Separate refactoring from logic changes, and save additional improvements for separate PRs.

2. **Test your changes**: run `make verify` to update dependencies, build code, and run tests. This prevents CI failures on your PR.

3. **Review before submitting**: look at your changes from a reviewer's perspective and explain anything that might not be immediately clear in your PR description.

4. **Use proper commit format**:
   - Write commit subjects in [imperative mood](https://en.wikipedia.org/wiki/Imperative_mood) (e.g., `Fix backup status` not `Fixed backup status`).
   - Every commit must have a **subject** (what) and a **body** (why and how). GitHub automatically uses the commit body as PR description for single-commit PRs.
   - Reference the Jira issue in the subject: `OCPBUGS-12345: fix backup status reporting`.
   - Use `NO-JIRA:` prefix for changes without an associated issue (dependency bumps, CI fixes). Use sparingly — have a Jira issue whenever possible.
   - Sign your commits: `git commit -s`.

Example:
```bash
git commit -s -m "OCPBUGS-12345: cap HCPEtcdBackup CR name at 63 bytes" \
              -m "Kubernetes rejects names longer than 63 characters. The backup
orchestrator was generating names from the HostedCluster name without
truncation, causing backup failures for clusters with long names."
```

## Creating a Pull Request

1. **For small changes** (under 200 lines): create your change and submit a pull request directly.

2. **For larger changes** (200+ lines): get feedback on your approach first by opening a GitHub issue or posting in the team Slack channel. This prevents situations where large changes get declined after significant work.

3. **Write a clear PR title**: prefix with your Jira ticket number (e.g., `OCPBUGS-12345: fix memory leak in restore plugin`).

4. **Explain the value**: always describe how your change improves the project in the PR description. Fill in the [PR template](.github/PULL_REQUEST_TEMPLATE.md) completely — do not leave it empty.

5. **PR checklist** must be satisfied:
   - Subject and description in both commit and PR.
   - Relevant issues referenced.
   - Documentation included if applicable.
   - Unit tests included.

### CI

This project uses [Prow](https://docs.ci.openshift.org/) for CI. Tests run automatically on pull requests.

Useful Prow commands:

- `/test <job-name>` — run a specific CI job.
- `/test all` — run all CI jobs.
- `/retest` — re-run failed CI jobs.
- `/retest-required` — re-run failed required CI jobs.

### Required Labels to Merge

- `approved` — from an approver via `/approve`.
- `lgtm` — from a reviewer via `/lgtm`.
- `jira/valid-reference` — PR title contains a valid Jira ticket reference (or `NO-JIRA`).

### Area Labels

PRs can be labeled with area labels using Prow commands:
- `/area backup-plugin` — changes to backup logic.
- `/area restore-plugin` — changes to restore logic.

## Review Process

- Reviewers and approvers are listed in the [OWNERS](./OWNERS) file. Any reviewer can provide feedback; approvers have merge authority.
- CI must pass before merge (see [CI Pipeline](#ci-pipeline) below).
- Review turnaround: aim to provide initial review feedback within **2 business days**.
- If assigned as a reviewer and you cannot review, hand over to another reviewer using `/un-cc` yourself and `/cc` a replacement.
- If assigned as an approver and you cannot approve, reassign using `/unassign` yourself and `/assign` a replacement.

## Coding Conventions

- **Language:** Go. Follow standard Go idioms and the [Effective Go](https://go.dev/doc/effective_go) guidelines.
- **Formatting:** `gofmt` / `goimports`. CI enforces this.
- **Naming:** use descriptive names. Avoid abbreviations except well-known ones (`ctx`, `err`, `req`).
- **Error handling:** always wrap errors with context using `fmt.Errorf("operation: %w", err)`.
- **Dependencies:** vendored. After changing `go.mod`, run `make deps` to tidy and vendor.
- **No global mutable state.** Pass dependencies explicitly (logger, client, context).

## Testing Requirements

### What to Test

- Every new function or method should have unit tests.
- Bug fixes must include a regression test that fails without the fix.
- Platform-specific logic (AWS, Azure, Agent) must be tested with mock clients.

### How to Run Tests

```bash
# Unit and integration tests
make test

# Tests with coverage
make cover

# Full verification (module check + tests)
make verify
```

### Test Style

- Use **table-driven tests**.
- Name test cases with descriptive strings that explain the scenario.
- Tests live alongside the code they test (`*_test.go` in the same package).
- Integration tests are in `tests/integration/`.

## CI Pipeline

The project uses OpenShift CI (Prow). The following checks must pass before merge:

| Check | What It Verifies |
|-------|------------------|
| `verify-modules` | `go.mod` and `go.sum` are up to date |
| `test` | All unit and integration tests pass |
| `verify` | Combined module verification + tests |

### Debugging CI Failures

1. Reproduce locally with `make verify`.
2. If the dependency validation test fails, run `make update-deps` and commit the result.
3. For module drift, run `make deps` and commit `go.mod`, `go.sum`, and `vendor/`.

## Dependency Updates

Automated dependency PRs come from Dependabot and Konflux (Renovate). These are handled by maintainers. If you need to update a dependency manually:

```bash
go get <module>@<version>
make deps
```

For watched upstream dependencies (e.g., `github.com/openshift/hypershift/api`):

```bash
make update-deps
```

## Project Structure

See the [Project Layout section in README.md](./README.md#project-layout) for the full directory structure.

Key areas for contributors:
- `pkg/core/` — backup and restore plugin logic (start here for most changes).
- `pkg/platform/` — platform-specific implementations.
- `pkg/common/` — shared utilities.
- `docs/` — technical reference documentation.
- `examples/` — OADP CR samples per platform.

## License

By contributing, you agree that your contributions will be licensed under the [Apache License 2.0](./LICENSE).
