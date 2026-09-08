# Phase 4 スライス 1: 設定ファイル 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `os.UserConfigDir()/octoscope/config.yaml` を読み、表示言語・グリフ・起動タブの決定に組み込む。設定が無くても壊れていても全機能が動く。

**Architecture:** `internal/config` は読み込みだけを持つ葉のパッケージ。既定値の解決（フラグ・環境変数との優先順）は呼び出し側（`cmd/octoscope` と `i18n.Resolve` / `icon.Resolve`）に置き、config 自身は「どこから来た値か」を知らない。書き込み（`repositories` の追加・`saved_queries` の保存）は、その呼び出し元が生まれるスライス 2・3 で足す。

**Tech Stack:** Go 1.25、`go.yaml.in/yaml/v3`（i18n と同じ YAML 実装）、`charm.land/bubbletea/v2`

**Spec:** `docs/superpowers/specs/2026-09-08-phase4-design.md`（§3）。画面と設定項目は `docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md` §5・§6.3 が正。

## Global Constraints

- **依存の向き**（`.claude/rules/architecture.md`）: `internal/config` は他の internal パッケージを import しない葉。`internal/tui` は `internal/config` を import しない。設定値は `app.Options` に載せて渡す。**パッケージを増やしたら `.golangci.yml` の depguard にも足す**
- **テスト**（`.claude/rules/testing.md`）: 失敗するテストから書く。書いた直後に検証対象を壊して落ちることを確かめる。ファイルは `t.TempDir()` に置く。言語を切り替えるテストは `t.Parallel()` にせず `t.Cleanup` で戻す
- **コメント**（`.claude/rules/go-style.md`）: 基本は書かない。書くのは外部の事情・一見おかしいコードが正しい理由・エクスポートした識別子の doc の 3 つだけ。**実装計画や設計書への参照をコードに書かない**（`Task 3`、`spec §5` などを書かない）
- **`make check` が緑でないコミットを作らない**
- 新しい文字列は `internal/i18n/locales/active.en.yaml` と `active.ja.yaml` の**両方**に足す
- golden は `en` / `ja` × 80 / 120 / 160。録り直しは `make golden` で、diff を目で見てからコミットする

## 設定ファイルの形

```yaml
language: ja
icons: nerd
default_tab: repos
```

**`icons` は spec §5 の表で `nerd_font` と呼ばれている項目である。** 名前を変える:
グリフの集合は `unicode` / `nerd` / `ascii` の 3 つあり（`internal/tui/icon`）、
`nerd_font` という真偽値の名前では `ascii` を指定できない。Task 4 で spec §5 の表も
直す。

`repositories` と `saved_queries` はスライス 2・3 で足す。**このスライスでは
型に持たない。** 読む側も書く側もまだ無い。YAML の未知のキーは
`yaml.v3` が既定で無視するので、先に手で書いた設定が壊れることはない。

## File Structure

| ファイル | 責務 |
|---|---|
| `internal/config/config.go`（新規） | `Config` 型、`Path()`、`Load()` |
| `internal/config/config_test.go`（新規） | 無いファイル・壊れた YAML・値の読み取り |
| `.golangci.yml`（変更） | depguard に `config-layer` と `tui-layer` の deny を足す |
| `internal/i18n/i18n.go`（変更） | `Resolve` が設定ファイルの候補を受け取る |
| `internal/tui/icon/icon.go`（変更） | `Resolve` が設定ファイルの候補を受け取る |
| `internal/tui/app/app.go`（変更） | `Options.DefaultRepos`、起動タブの選択 |
| `internal/tui/app/render.go`（変更） | 設定を読めなかったことをタブ行に出す |
| `cmd/octoscope/main.go`（変更） | 設定を読み、優先順を組んで各所に渡す |

---

### Task 1: `internal/config` の読み込み

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`
- Modify: `.golangci.yml`（depguard に `config-layer`、`tui-layer` に config の deny）

**Interfaces:**
- Consumes: なし
- Produces:
  - `type Config struct { Language string; Icons string; DefaultTab string }`
  - `func Path() (string, error)` — `<os.UserConfigDir()>/octoscope/config.yaml`
  - `func Load(path string) (Config, error)` — ファイルが無ければ**ゼロ値と nil**。
    解析に失敗したらゼロ値と error（呼び出し側は続行する）

- [ ] **Step 1: 失敗するテストを書く**

`internal/config/config_test.go`:

```go
package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kukv/octoscope/internal/config"
)

// A missing file is the default, not a failure: octoscope must run for
// someone who has never written a config.
func TestLoadTreatsAMissingFileAsDefaults(t *testing.T) {
	t.Parallel()

	got, err := config.Load(filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatalf("Load of a missing file: %v", err)
	}
	if got != (config.Config{}) {
		t.Errorf("Load of a missing file = %+v, want the zero Config", got)
	}
}

func TestLoadReadsEveryField(t *testing.T) {
	t.Parallel()

	path := write(t, "language: ja\nicons: nerd\ndefault_tab: repos\n")

	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := config.Config{Language: "ja", Icons: "nerd", DefaultTab: "repos"}
	if got != want {
		t.Errorf("Load = %+v, want %+v", got, want)
	}
}

// A broken config must not keep the user from reaching GitHub: Load reports
// the failure, and the caller carries on with defaults.
func TestLoadReportsBrokenYAMLAndStillReturnsDefaults(t *testing.T) {
	t.Parallel()

	path := write(t, "language: [ja\n")

	got, err := config.Load(path)
	if err == nil {
		t.Fatal("Load of broken YAML returned no error")
	}
	if got != (config.Config{}) {
		t.Errorf("Load of broken YAML = %+v, want the zero Config", got)
	}
}

// Keys this version does not know belong to a later one; they must not stop
// it from reading the keys it does know.
func TestLoadIgnoresKeysItDoesNotKnow(t *testing.T) {
	t.Parallel()

	path := write(t, "language: ja\nrepositories:\n  - kukv/octoscope\n")

	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Language != "ja" {
		t.Errorf("Language = %q, want %q", got.Language, "ja")
	}
}

func write(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
```

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/config/`
Expected: FAIL（`internal/config` がまだ無い）

- [ ] **Step 3: 最小限の実装を書く**

`internal/config/config.go`:

```go
// Package config reads the settings file that persists what a flag cannot:
// the repositories the Repos tab lists, the saved search queries, and the
// choices a user makes once rather than every run.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	yaml "go.yaml.in/yaml/v3"
)

// Config is the settings file. Every field is optional; the zero Config is
// what someone with no settings file gets, and every feature works from it.
type Config struct {
	Language   string `yaml:"language"`
	Icons      string `yaml:"icons"`
	DefaultTab string `yaml:"default_tab"`
}

// Path is where the settings file lives: octoscope/config.yaml under the
// directory the operating system keeps configuration in (%AppData% on
// Windows, ~/Library/Application Support on macOS, ~/.config on Linux).
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate the config directory: %w", err)
	}
	return filepath.Join(dir, "octoscope", "config.yaml"), nil
}

// Load reads the settings file. A file that is not there is not a failure:
// the answer is the zero Config. A file that is there but unreadable returns
// the zero Config alongside the error, so a caller can report it and carry
// on with defaults rather than refusing to start.
func Load(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read %s: %w", path, err)
	}
	var c Config
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return c, nil
}
```

- [ ] **Step 4: 通ることを確かめる**

Run: `go test ./internal/config/`
Expected: PASS（4 本）

- [ ] **Step 5: アサーションが空振りしていないことを確かめる**

`Load` の `errors.Is(err, fs.ErrNotExist)` の分岐を一時的に消して
`TestLoadTreatsAMissingFileAsDefaults` が落ちること、`yaml.Unmarshal` の
エラーを握りつぶす形に一時的に変えて
`TestLoadReportsBrokenYAMLAndStillReturnsDefaults` が落ちることを目で見る。
確かめたら元に戻す。

- [ ] **Step 6: depguard に足す**

`.golangci.yml` の `depguard.rules` に足す。`internal/config` は葉なので何も
import しない側であり、`internal/tui` は設定を直接読まない側である。

```yaml
        # Reading the settings file is a leaf: it parses YAML and knows
        # nothing of the screens or of GitHub.
        config-layer:
          files:
            - "**/internal/config/**"
          deny:
            - pkg: github.com/kukv/octoscope/internal/gh
              desc: internal/config must not depend on other internal packages
            - pkg: github.com/kukv/octoscope/internal/tui
              desc: internal/config must not depend on other internal packages
            - pkg: github.com/kukv/octoscope/internal/i18n
              desc: internal/config must not depend on other internal packages
            - pkg: github.com/kukv/octoscope/internal/usecase
              desc: internal/config must not depend on other internal packages
```

`tui-layer` の `deny` にも 1 行足す:

```yaml
            - pkg: github.com/kukv/octoscope/internal/config
              desc: settings reach the views through app.Options; cmd/octoscope reads them
```

- [ ] **Step 7: 検査してコミット**

```bash
make check
git add internal/config .golangci.yml
git commit -m "feat: read the settings file"
```

---

### Task 2: 言語の決定順に設定ファイルを挟む

`--lang` → 設定ファイルの `language` → OS ロケール → en（standalone spec §6.3）。
今の `i18n.Resolve` は 2 番目を持っていない。

**Files:**
- Modify: `internal/i18n/i18n.go`（`Resolve`）
- Modify: `internal/i18n/i18n_test.go`
- Modify: `cmd/octoscope/main.go`（呼び出し側。Task 5 で設定を読むまでは空文字を渡す）

**Interfaces:**
- Consumes: `config.Config.Language`（Task 1）
- Produces: `func Resolve(flagLang, configLang, osLocale string) language.Tag`

- [ ] **Step 1: 失敗するテストを書く**

`internal/i18n/i18n_test.go` の `Resolve` のテストの並びに足す:

```go
// The flag is a one-off override; the settings file is the standing choice.
// A user who set language: ja and passes --lang en wants English this once.
func TestResolvePrefersTheFlagOverTheSettingsFile(t *testing.T) {
	if got := i18n.Resolve("en", "ja", "ja_JP.UTF-8"); got != language.English {
		t.Errorf("Resolve = %v, want English", got)
	}
}

// The settings file is a deliberate choice; the operating system locale is
// only a guess at one.
func TestResolvePrefersTheSettingsFileOverTheOSLocale(t *testing.T) {
	if got := i18n.Resolve("", "ja", "en_US.UTF-8"); got != language.Japanese {
		t.Errorf("Resolve = %v, want Japanese", got)
	}
}

// A language with no catalog is not a reason to fall all the way back to
// English: the next candidate still gets its turn.
func TestResolveSkipsASettingsFileLanguageWithNoCatalog(t *testing.T) {
	if got := i18n.Resolve("", "fr", "ja_JP.UTF-8"); got != language.Japanese {
		t.Errorf("Resolve = %v, want Japanese", got)
	}
}
```

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/i18n/ -run TestResolve`
Expected: FAIL（`Resolve` の引数が 2 つ、コンパイルエラー）

- [ ] **Step 3: 実装を直す**

`internal/i18n/i18n.go`:

```go
// Resolve picks the display language: the --lang flag first, then the
// settings file, then the locale reported by the operating system, then
// English.
func Resolve(flagLang, configLang, osLocale string) language.Tag {
	matcher := language.NewMatcher(supported)
	for _, candidate := range []string{flagLang, configLang, osLocale} {
		if candidate == "" {
			continue
		}
		tag, err := language.Parse(candidate)
		if err != nil {
			continue
		}
		if _, index, conf := matcher.Match(tag); conf != language.No {
			return supported[index]
		}
	}
	return language.English
}
```

既存の呼び出し（`cmd/octoscope/main.go`、既存テスト）を 3 引数に直す。
main では今のところ 2 番目に `""` を渡す（Task 5 で設定の値に変わる）。

- [ ] **Step 4: 通ることを確かめる**

Run: `go test ./internal/i18n/`
Expected: PASS

- [ ] **Step 5: 空振りしていないことを確かめる**

`Resolve` の候補の並びを一時的に `{flagLang, osLocale, configLang}` に入れ替え、
`TestResolvePrefersTheSettingsFileOverTheOSLocale` が落ちることを見る。戻す。

- [ ] **Step 6: 検査してコミット**

```bash
make check
git add internal/i18n cmd/octoscope
git commit -m "feat: let the settings file choose the language"
```

---

### Task 3: グリフの決定順に設定ファイルを挟む

`--icons` → `OCTOSCOPE_ICONS` → 設定ファイルの `icons` → 自動判定（既定の
`Unicode`）。standalone spec §5.1 は「環境変数に逃がしてある恒久指定を、設定ファイルを
実装した時点でそちらへ寄せる」と書いている。**環境変数は消さない**（すでに使っている
人の設定を黙って無効にしないため）が、順は環境変数が先で設定ファイルが後になる。

**Files:**
- Modify: `internal/tui/icon/icon.go`（`Resolve`）
- Modify: `internal/tui/icon/icon_test.go`
- Modify: `cmd/octoscope/main.go`（Task 5 まで空文字）

**Interfaces:**
- Consumes: `config.Config.Icons`（Task 1）
- Produces: `func Resolve(flag, configured string) Set`

- [ ] **Step 1: 失敗するテストを書く**

`internal/tui/icon/icon_test.go` に足す:

```go
// The settings file is the standing choice; the flag and the environment
// variable are ways to override it for one run.
func TestResolveTakesTheSettingsFileWhenNothingOverridesIt(t *testing.T) {
	t.Setenv(icon.EnvVar, "")

	if got := icon.Resolve("", "ascii"); got != icon.ASCII {
		t.Errorf("Resolve = %v, want ASCII", got)
	}
}

func TestResolvePrefersTheEnvironmentOverTheSettingsFile(t *testing.T) {
	t.Setenv(icon.EnvVar, "nerd")

	if got := icon.Resolve("", "ascii"); got != icon.Nerd {
		t.Errorf("Resolve = %v, want Nerd", got)
	}
}

// A name no set answers to is worth ignoring rather than refusing to start
// over: the glyphs a terminal draws are not worth an error screen.
func TestResolveFallsBackWhenTheSettingsFileNamesNoSet(t *testing.T) {
	t.Setenv(icon.EnvVar, "")

	if got := icon.Resolve("", "emoji"); got != icon.Unicode {
		t.Errorf("Resolve = %v, want Unicode", got)
	}
}
```

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/tui/icon/ -run TestResolve`
Expected: FAIL（`Resolve` の引数が 1 つ、コンパイルエラー）

- [ ] **Step 3: 実装を直す**

```go
// Resolve picks the glyph set: the --icons flag first, then OCTOSCOPE_ICONS,
// then the settings file, then the set that needs no font installed.
func Resolve(flag, configured string) Set {
	for _, candidate := range []string{flag, os.Getenv(EnvVar), configured} {
		switch strings.ToLower(strings.TrimSpace(candidate)) {
		case "nerd":
			return Nerd
		case "ascii":
			return ASCII
		case "unicode":
			return Unicode
		}
	}
	return Unicode
}
```

既存の呼び出しを 2 引数に直す（main では Task 5 まで `""`）。

- [ ] **Step 4: 通ることを確かめる**

Run: `go test ./internal/tui/icon/`
Expected: PASS

- [ ] **Step 5: 空振りしていないことを確かめる**

候補の並びを一時的に `{flag, configured, os.Getenv(EnvVar)}` に入れ替え、
`TestResolvePrefersTheEnvironmentOverTheSettingsFile` が落ちることを見る。戻す。

- [ ] **Step 6: 検査してコミット**

```bash
make check
git add internal/tui/icon cmd/octoscope
git commit -m "feat: let the settings file choose the glyph set"
```

---

### Task 4: `default_tab` で起動タブを選ぶ

今は `--repo` があれば Repos タブから始まる。設定の `default_tab: repos` は、
**カレントディレクトリのリポジトリが後から見つかったときにも** Repos から始めたい、
という指定である。`--repo` は「今回はこれを見に来た」という宣言なので設定より強い
（standalone spec §5.1）。

Repos タブは今、リポジトリが見つかるまで存在しない。設定が `repos` でも、
タブが無いうちは Work を出し、`repoResolved` で見つかった時点で移る。

**Files:**
- Modify: `internal/tui/app/app.go`（`Options`、`New`、`repoResolved`）
- Modify: `internal/tui/app/app_test.go`
- Modify: `cmd/octoscope/main.go`（Task 5 まで false）

**Interfaces:**
- Consumes: `config.Config.DefaultTab`（Task 1）
- Produces: `app.Options.DefaultRepos bool` — 設定が Repos タブを既定にしているか。
  `--repo` の有無は今までどおり `Options.HasRepo`

- [ ] **Step 1: 失敗するテストを書く**

`internal/tui/app/app_test.go` に足す:

```go
// Someone who set default_tab: repos wants the Repos tab even when the
// repository is found from the working directory rather than named on the
// command line -- but the tab does not exist until it is found.
func TestDefaultReposWaitsForTheRepositoryToBeFound(t *testing.T) {
	m := New(&fakeSource{}, Options{DefaultRepos: true})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	if m.tab != tabWork {
		t.Fatalf("tab before the lookup answered = %v, want tabWork", m.tab)
	}

	next, _ = m.Update(repoResolvedMsg{found: true})
	if got := next.(Model).tab; got != tabRepos {
		t.Errorf("tab after the repository was found = %v, want tabRepos", got)
	}
}

// Without the setting, a repository found from the working directory does not
// move the user off the board they started on.
func TestAFoundRepositoryDoesNotMoveTheUserByItself(t *testing.T) {
	m := New(&fakeSource{}, Options{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	next, _ = next.(Model).Update(repoResolvedMsg{found: true})

	if got := next.(Model).tab; got != tabWork {
		t.Errorf("tab = %v, want tabWork", got)
	}
}

// The setting must not pull the user back after they have moved: it chooses
// where the run starts, not where it stays.
func TestDefaultReposDoesNotPullTheUserBackAfterTheyMove(t *testing.T) {
	m := New(&fakeSource{}, Options{DefaultRepos: true})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	next, _ = next.(Model).Update(repoResolvedMsg{found: true})
	onWork := press(next.(Model), "1")

	after, _ := onWork.Update(repoResolvedMsg{found: true})
	if got := after.(Model).tab; got != tabWork {
		t.Errorf("tab = %v, want tabWork", got)
	}
}
```

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/tui/app/ -run TestDefaultRepos`
Expected: FAIL（`Options` に `DefaultRepos` が無い、コンパイルエラー）

- [ ] **Step 3: 実装を書く**

`Options` に足す:

```go
	// DefaultRepos says the settings file asks for the Repos tab at start-up.
	// It cannot be honoured until the tab exists, so it is remembered here
	// and spent when the lookup finds a repository. It is weaker than --repo,
	// which is a statement about this run.
	DefaultRepos bool
```

`Model` に、まだ使っていない指定を覚えるフィールドを足す:

```go
	// wantRepos holds the settings file's opening tab until the Repos tab
	// exists. It is cleared once spent, so a later answer to the same lookup
	// does not pull the user back off the tab they moved to.
	wantRepos bool
```

`New` で `m.wantRepos = opts.DefaultRepos && !opts.HasRepo` を立て、
`repoResolved`（`repoResolvedMsg` を処理している箇所）で、見つかったときに:

```go
	if m.wantRepos {
		m.wantRepos = false
		m.tab = tabRepos
	}
```

**`press` は既存のテストヘルパー**（`internal/tui/app/app_test.go`）。新しく書かない。

- [ ] **Step 4: 通ることを確かめる**

Run: `go test ./internal/tui/app/`
Expected: PASS

- [ ] **Step 5: 空振りしていないことを確かめる**

`m.wantRepos = false` の 1 行を一時的に消して
`TestDefaultReposDoesNotPullTheUserBackAfterTheyMove` が落ちることを見る。戻す。

- [ ] **Step 6: spec の設定項目の名前を直す**

`docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md` §5 の表の
`nerd_font` の行を `icons` に直し、内容を
「グリフの集合（`unicode` / `nerd` / `ascii`、既定は `unicode`）」にする。
理由（真偽値の名前では `ascii` を指定できない）を §5.1 の `--icons` の段落に 1 文で足す。

- [ ] **Step 7: 検査してコミット**

```bash
make check
git add internal/tui/app cmd/octoscope docs/superpowers/specs
git commit -m "feat: let the settings file choose the opening tab"
```

---

### Task 5: main で設定を読み、読めなければ画面に出す

**Files:**
- Modify: `cmd/octoscope/main.go`
- Modify: `internal/tui/app/app.go`（`Options.ConfigError`）
- Modify: `internal/tui/app/render.go`（`tabRow`）
- Modify: `internal/tui/app/app_test.go`、`internal/tui/app/golden_test.go`
- Modify: `internal/i18n/locales/active.en.yaml`、`active.ja.yaml`
- Create: `internal/tui/app/testdata/app_config_error_{en,ja}_{80,120,160}.golden`

**Interfaces:**
- Consumes: `config.Path()`、`config.Load()`（Task 1）、`i18n.Resolve`（Task 2）、
  `icon.Resolve`（Task 3）、`Options.DefaultRepos`（Task 4）
- Produces: `app.Options.ConfigError string` — 空でなければタブ行に警告を出す

- [ ] **Step 1: 失敗するテストを書く**

`internal/tui/app/app_test.go` に足す:

```go
// A settings file that cannot be read must not be silent: the user is running
// with defaults and has no other way to find out.
func TestTheTabRowSaysTheSettingsFileCouldNotBeRead(t *testing.T) {
	m := New(&fakeSource{}, Options{ConfigError: "parse config.yaml: yaml: line 1: did not find expected node content"})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	if !strings.Contains(next.(Model).View().Content, i18n.T("tab.config_unreadable")) {
		t.Error("the tab row does not report the unreadable settings file")
	}
}

func TestTheTabRowIsQuietWhenTheSettingsFileIsFine(t *testing.T) {
	m := New(&fakeSource{}, Options{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	if strings.Contains(next.(Model).View().Content, i18n.T("tab.config_unreadable")) {
		t.Error("the tab row reports an unreadable settings file that was fine")
	}
}
```

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/tui/app/ -run TestTheTabRow`
Expected: FAIL（`Options` に `ConfigError` が無い、コンパイルエラー）

- [ ] **Step 3: カタログに文言を足す**

`internal/i18n/locales/active.en.yaml` の `tab:` の下（`repo_lookup_timeout` の隣）:

```yaml
  config_unreadable:
    other: "settings file unreadable; using defaults"
```

`internal/i18n/locales/active.ja.yaml`:

```yaml
  config_unreadable:
    other: "設定ファイルを読めません。既定値で動いています"
```

- [ ] **Step 4: 実装を書く**

`Options` に足す:

```go
	// ConfigError is why the settings file could not be read, if it could
	// not. Empty means it was read, or was not there at all -- which is not
	// a failure. The run carries on with defaults either way, so the tab row
	// is the only place the user learns of it.
	ConfigError string
```

`render.go` の `tabRow` で、`repoLookupTimedOut` の隣に足す:

```go
	if m.opts.ConfigError != "" {
		row += tabGap + theme.Error().Render(i18n.T("tab.config_unreadable"))
	}
```

- [ ] **Step 5: 通ることを確かめる**

Run: `go test ./internal/tui/app/ -run TestTheTabRow`
Expected: PASS

- [ ] **Step 6: main を組む**

`cmd/octoscope/main.go` の `flag.Parse()` の後、`i18n.SetLanguage` の前に:

```go
	var cfg config.Config
	var configErr string
	if path, err := config.Path(); err != nil {
		configErr = err.Error()
	} else if cfg, err = config.Load(path); err != nil {
		configErr = err.Error()
	}

	osLocale, _ := locale.GetLocale() // an error here just means "unknown"
	i18n.SetLanguage(i18n.Resolve(*lang, cfg.Language, osLocale))
	icon.Use(icon.Resolve(*icons, cfg.Icons))
```

`app.New` に渡す `Options`:

```go
	p := tea.NewProgram(app.New(uc, app.Options{
		HasRepo:      *repoFlag != "",
		DefaultRepos: cfg.DefaultTab == "repos",
		ConfigError:  configErr,
	}))
```

- [ ] **Step 7: golden を録る**

`golden_test.go` の `TestGolden` に、設定を読めなかった形を 1 つ足す:

```go
				badConfig := goldenModel(w, Options{HasRepo: true, ConfigError: "parse config.yaml: yaml: line 1: did not find expected node content"})
				golden.Assert(t, fmt.Sprintf("app_config_error_%s_%d", lang.name, w), badConfig.View().Content)
```

```bash
make golden
cat -v internal/tui/app/testdata/app_config_error_ja_80.golden
```

**ja の 80 桁でタブ行が溢れていないことを目で見る。** 溢れていたら文言を縮める
（カタログ側を直す。描画側で切らない）。

- [ ] **Step 8: 空振りしていないことを確かめる**

`tabRow` に足した 3 行を一時的に消して
`TestTheTabRowSaysTheSettingsFileCouldNotBeRead` が落ちることを見る。戻す。

- [ ] **Step 9: 手で動かす**

```bash
go run ./cmd/octoscope --lang ja     # 設定が無い状態。今までどおり動く
```

設定ファイルを実際に置いて確かめる（Linux なら `~/.config/octoscope/config.yaml`）。

```bash
printf 'language: ja\nicons: ascii\n' > "${XDG_CONFIG_HOME:-$HOME/.config}/octoscope/config.yaml"
go run ./cmd/octoscope            # 日本語・ASCII グリフで出る
printf 'language: [ja\n' > "${XDG_CONFIG_HOME:-$HOME/.config}/octoscope/config.yaml"
go run ./cmd/octoscope            # 英語・既定グリフで動き、タブ行に警告が出る
```

確認が終わったら置いたファイルを消す。

- [ ] **Step 10: 検査してコミット**

```bash
make check
git add cmd/octoscope internal/tui/app internal/i18n
git commit -m "feat: read the settings file at start-up"
```

---

## 完了条件

1. `config.yaml` の `language` / `icons` / `default_tab` が効く
2. ファイルが無くても、壊れていても起動し、壊れているときはタブ行に警告が出る
3. 言語の決定順が `--lang` → `language` → OS ロケール → en
4. グリフの決定順が `--icons` → `OCTOSCOPE_ICONS` → `icons` → `unicode`
5. `default_tab: repos` が、後から見つかったリポジトリでも効く。ただし利用者が
   タブを移った後には効かない
6. `internal/tui` が `internal/config` を import していない（depguard が守る）
7. golden が en / ja × 80 / 120 / 160 で録れており、ja の 80 桁でタブ行が溢れない
8. `make check` が緑
