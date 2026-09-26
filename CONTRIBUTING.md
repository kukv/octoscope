# Contributing

Thanks for taking the time to improve octoscope.

## Getting started

octoscope is a single Go binary for Linux, macOS and Windows. You need the Go version
in `go.mod` and [golangci-lint](https://golangci-lint.run/) v2 (the version CI uses is
pinned in `.github/workflows/ci.yaml`).

```bash
make check                              # everything CI checks: tidy, lint, fmt, test
make test                               # tests with the race detector and coverage
make fmt                                # format with gofumpt + goimports
make golden                             # regenerate golden files after an intended UI change
make release-check                      # goreleaser config and cross-compiling for 3 OSes

go run ./cmd/octoscope                  # the repository of the current directory
go run ./cmd/octoscope --repo kukv/koto # any repository
go run ./cmd/octoscope --lang ja        # Japanese display
```

Do not commit while `make check` fails.

## Before you change code

Changes to `internal/` and `cmd/` follow a design and an implementation plan:

- Designs: `docs/superpowers/specs/`
- Plans: `docs/superpowers/plans/`

Read the ones that cover your change first. If none does, open an issue to agree on
the design before writing code — a pull request that restructures packages without one
is closed.

Conventions for Go style, errors, tests, architecture and the TUI live in
`.claude/rules/`. If a rule gets in the way, propose changing the rule rather than
working around it.

## Checking a TUI change

Passing tests are not enough for a change to what appears on screen. Run it and look:

- with `--lang ja` as well as English — full-width characters take two columns, and a
  layout that fits in English can overflow in Japanese;
- on a narrow terminal as well as a wide one;
- on Windows, if you can — CI runs the tests on all three operating systems, but only a
  person can see how a terminal renders.

## Pull requests

- CI runs on every pull request: `ci.yaml` (lint, format, and the tests on Linux, macOS
  and Windows) and `security.yml` (hidden-content scan, gitleaks, dependency scanning,
  zizmor, actionlint). All of them must pass.
- Pin any GitHub Action you add to a full commit SHA with a `# vX.Y.Z` comment. Renovate
  follows them.
- Commit messages use a `feat:` / `fix:` / `docs:` / `refactor:` / `test:` / `chore:` prefix.
- Link the issue the pull request resolves (`Closes #123`).
- Label the pull request so it lands in the right section of the release notes and the
  right version bump — see [Releases](#releases).
- Review by the maintainer (`.github/CODEOWNERS`) is required before merge.

## Releases

Work is planned with issues and milestones. Each milestone is named after the version
it is expected to ship as (`v0.9.0`), and the issues and pull requests for that version
are assigned to it.

The labels on the merged pull requests decide the version and the release notes:

| Label                | Bump                                   | Release notes section |
| -------------------- | -------------------------------------- | --------------------- |
| `Impact: Breaking`   | major (minor while the version is 0.x) | Breaking Changes      |
| `Kind: Feature`      | minor                                  | New Features          |
| `Kind: Enhancement`  | minor                                  | Enhancement Updates   |
| `Kind: Bug Fix`      | patch                                  | Bug Fix               |
| `Kind: Dependencies` | patch                                  | dependency updates    |
| anything else        | patch                                  | Other Changes         |

The highest bump among the pull requests wins. `Meta: Release note ignored` keeps a pull
request out of the notes.

To release:

1. Check that every pull request in the milestone is labelled, and that the milestone's
   name still matches the bump the labels call for. If a `v0.9.1` milestone picked up a
   `Kind: Feature`, rename it to `v0.10.0`.
2. Push the tag on `main`:

   ```bash
   git switch main && git pull
   git tag v0.10.0 && git push origin v0.10.0
   ```

3. `.github/workflows/release.yaml` runs GoReleaser, which builds the binaries for
   Linux, macOS and Windows and creates the GitHub Release. Its notes are generated from
   the pull request labels, following `.github/release.yaml`.
4. Close the milestone.

Tags are never moved or reused once pushed: `go install` and mise resolve versions from
them. Going to v2 or later also means changing the module path to
`github.com/kukv/octoscope/v2`, as Go modules require.

## Use of AI

AI assistance is fine. Submitting what an AI produced without understanding it is not.

Generating a submission takes seconds; verifying one takes a person's time. Sending
unverified output moves that cost onto the maintainer and takes time away from the review
this project actually needs.

Before you open an issue or a pull request, you are expected to have read the output,
verified it against this repository, and be able to explain and defend it. You are the
author of what you submit, whatever tool helped you write it.

Issues and pull requests that appear to be unreviewed AI output — invented options,
files or APIs that do not exist, a diff that does not follow from the description,
boilerplate that does not engage with this project — are **closed without notice and
without individual explanation**. That judgment is the maintainer's, and there is no
appeal process; you are welcome to open a new issue or pull request that shows your own
reasoning.

Closing one does not mean the underlying point was worthless. If a closed issue or pull
request contains something useful, the maintainer may take it up — as an issue raised by
the maintainer, or by merging or rewriting the change — without notice and without credit
to the original submitter. Anything you submit is licensed under the
[MIT License](LICENSE), and opening an issue or pull request here means you accept this
handling.

## Reporting problems

- A security issue: see [SECURITY.md](SECURITY.md).
- A bug or a feature request: open an issue from the templates.

By contributing you agree that your contributions are licensed under the
[MIT License](LICENSE). Everyone taking part is expected to follow the
[Code of Conduct](CODE_OF_CONDUCT.md).
