# octoscope

[日本語](README.ja.md)

A terminal dashboard for GitHub pull requests and issues.

octoscope = **Octo**cat + **-scope**: a telescope for looking over your GitHub work.

## Requirements

- [GitHub CLI](https://cli.github.com/) (`gh`), authenticated via `gh auth login`

## Install

### mise

[mise](https://mise.jdx.dev/) installs the released binary from GitHub Releases:

    mise use -g github:kukv/octoscope@latest

Or pin it per project in `mise.toml`:

    [tools]
    "github:kukv/octoscope" = "latest"

### Manual download

Grab the archive for your platform from the
[releases page](https://github.com/kukv/octoscope/releases), extract it, and put
`octoscope` somewhere on your `PATH`:

    tar xzf octoscope_<version>_linux_amd64.tar.gz
    install -m 0755 octoscope ~/.local/bin/

On Windows, unzip the archive and place `octoscope.exe` in a directory on `PATH`.

`checksums.txt` is published alongside the archives:

    sha256sum -c checksums.txt --ignore-missing

### go install

    go install github.com/kukv/octoscope/cmd/octoscope@latest

`--version` prints `dev` with this method; the version string is stamped in at
release build time.

### Build from source

    git clone https://github.com/kukv/octoscope.git
    cd octoscope
    go build -o octoscope ./cmd/octoscope

## Usage

Run it inside a git repository:

    octoscope

Or point it at any repository:

    octoscope --repo kukv/octoscope

### Flags

| Flag | Description |
|---|---|
| `--repo owner/name` | Target repository. Defaults to the repository of the current directory. |
| `--lang en\|ja` | Display language. Defaults to the operating system locale. |
| `--icons unicode\|nerd\|ascii` | Glyph set. Defaults to `unicode`; `OCTOSCOPE_ICONS` sets it permanently. |
| `--version` | Print the version and exit. |

Pass `--icons nerd` if you have a [Nerd Font](https://www.nerdfonts.com/)
patched font installed, or `--icons ascii` if the Unicode symbols do not draw.
There is no reliable way to detect a patched font — a terminal reports neither
the font in use nor its coverage — so the default is the set that needs none.

### Keys

| Key | List | Detail |
|---|---|---|
| `j` / `k` | move cursor | scroll |
| `enter` | open detail | — |
| `tab` | switch PRs / Issues | — |
| `r` | refresh | refresh |
| `o` | open in browser | open in browser |
| `c` | — | comment (`Ctrl+S` send / `Esc` cancel) |
| `x` | — | close / reopen (`y` confirm / `n` cancel) |
| `l` | — | edit labels (`space` toggle / `enter` apply) |
| `a` | — | edit assignees (`space` toggle / `enter` apply) |
| `esc` | — | back to list |
| `q` | quit | back to list |

### Mouse

| Action | Effect |
|---|---|
| Click a tab | Switch to it |
| Click a card or row | Select it |
| Click the selected card or row | Open its detail |
| Wheel | Move the cursor in the column under the pointer; scroll the body in the detail view |

## Localization

octoscope speaks English and Japanese. The language is chosen from `--lang`
first, then the operating system locale, and falls back to English.
