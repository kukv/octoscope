# Security Policy

## Supported versions

Only the latest release is supported. Fixes are not backported to older versions;
update to the newest release from the
[releases page](https://github.com/kukv/octoscope/releases), mise or `go install`.

## Reporting a vulnerability

Report privately through GitHub: open the **Security** tab of this repository and
choose **Report a vulnerability**. Please do not open a public issue.

Include what `octoscope --version` prints, your operating system, the command line
you ran, and the steps or content that reproduce the problem.

## What counts as a security issue

- **A token leaking.** `GH_TOKEN`, `GITHUB_TOKEN` or a token from `gh` appearing on
  screen, in a log, in the settings file, or anywhere else octoscope writes.
- **Content affecting the terminal or the machine.** A crafted title, body, branch
  name or other text from GitHub that makes octoscope run a command, write outside its
  own files, or inject escape sequences that the terminal acts on.
- **A released binary that is not what this repository builds.**

Not a security issue:

- A crash, a wrong list or a broken layout without the effects above. Open an
  ordinary issue.
- Something a person can already see or do with their own token through `gh` or the
  GitHub web UI.

## Handling

Reports are acknowledged and triaged by the maintainer. Once a fix is released, the
advisory is published with credit to the reporter unless anonymity is requested.
