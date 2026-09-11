# Repos の追加ダイアログ・削除・初回投入 実装計画（Phase 4 スライス 2-3）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Repos タブのリポジトリ一覧を、利用者が `a` で足し `x` で消し、設定ファイルに残せるようにする。

**Architecture:** 設定ファイルへの書き戻しは `internal/config` に `Store` を新設し、
`internal/usecase` 経由でビューに届ける（ビューは `internal/config` を import できない）。
ダイアログは `internal/tui/dialog` を新設し、`internal/tui/review` と同じ「自分の箱だけを
描き、持ち主が置く」流儀のポップアップにする。候補検索と初回投入の材料は
`internal/gh/cli` に足し、複数呼び出しになる初回投入だけ `internal/usecase` が順序を持つ。

**Tech Stack:** Go / Bubble Tea v2（`charm.land/*/v2`）/ `go.yaml.in/yaml/v3` / `gh` CLI

**Spec:**
- `docs/superpowers/specs/2026-09-08-phase4-design.md`（§3 設定ファイル、§4 Repos タブ、§7 境界、§8 テスト、§9 幅、§10 割り方、§11 完了条件）
- `docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md`（§4.2 画面、§5 設定ファイル）
- UI モックアップ: https://claude.ai/code/artifact/96b1dad5-ed75-4176-b110-20923b1b565e
  — **§4.2 についてはモックアップが正。** 文章だけ読んで実装すると
  「要素は揃っているがレイアウトが別物」になる（#52 で実際にそうなった）

---

## Global Constraints

- Bubble Tea 系の import は `charm.land/*/v2`。`github.com/charmbracelet/bubbletea/v2` は壊れている。`github.com/charmbracelet/x/ansi` は `github.com/` のままが正しい
- 画面に出す文字列は `internal/i18n` から引く。新しい ID は `active.en.yaml` と `active.ja.yaml` の**両方**に足す
- 桁は `ansi.StringWidth` で数える。`len` も `utf8.RuneCountInString` も使わない
- ネットワークも外部プロセスも実際には叩かない。`gh` の応答は実物を録って `testdata` に置き、録り方を `testdata/README.md` に残す
- 先に失敗するテストを書く。書いた直後に検証対象を一時的に壊し、**落ちることを目で見てから**コミットする
- コメントは英語。書くのは「外部の事情」「一見おかしいコードが正しい理由」「エクスポートした識別子の doc」の 3 つだけ。**実装計画や設計書への参照（`Task 7`、`spec §4.2`）をコードに書かない**（`.claude/rules/*.md` への参照は可）
- `internal/tui` は `internal/gh/cli` と `internal/config` を import しない。`internal/usecase` は `internal/tui` と `internal/i18n` を import しない。**パッケージを増やしたら `.golangci.yml` の depguard にその場で足す**
- 1 つの interface 宣言に直接並べるメソッドは 6 個まで（embed は数えない）
- 各タスクの終わりに `make check` が緑であること
- golden は en / ja × 80 / 120 / 160。`make golden` で録り直し、**diff を目で見てから**コミットする

## この計画が判断した前提（着手前に承認を取ること）

設計書にもモックアップにも答えが無く、この計画が決めたもの。**違うと思ったらここで止める。**

1. **ダイアログは画面を暗くして浮かせない。** モックアップは下の画面を暗転させて中央に
   モーダルを置くが、既存のポップアップ（`internal/tui/review`、`internal/tui/merge`）は
   どちらも画面を置き換える形で描いており、合成の仕組みがこのコードベースに無い。
   箱の中身（見出し・入力欄・ヒント・候補行・アクション）はモックアップどおりにし、
   置き方だけ既存のポップアップに揃える
2. **`x` は確認を取らない。** 消えるのは設定ファイルの 1 行で、`a` で足し直せる
3. **追加時に存在確認をしない。** 形（`owner/name`）だけ弾く。存在しない名前は
   サイドバーのバッジが「—」になって知らせる（既存の `RepoCounts` の挙動）
4. **候補はスター数つきで出す。** モックアップの候補行が `★ 9.1k` を出しているため。
   設計 §2 は `--json fullName,description,isPrivate` と書いているが、`description` は
   30 桁のダイアログに収まらず、モックアップにも無い。実際に使うのは
   `fullName,stargazersCount,isPrivate`
5. **保存に失敗しても行は画面に残す。** 通知行（既存の `notice`）で保存できなかったことを
   言う。行を巻き戻すと、利用者が今見ているリポジトリが目の前で消える
6. **初回投入は全件を自動で入れない。** 取得したものを**同じダイアログの候補として出し**、
   利用者が 1 つずつ `enter` で足す。実測（2026-09-11、この計画を書く際に計測）で
   `gh repo list --limit 100` は 45 件・6.7 秒、`gh api user/orgs` は 6.1 秒。
   45 件を黙って一覧に入れると、`RepoCounts` が 45 alias の 1 リクエストになる
   （30 alias で 8.1 秒の実測しかなく、45 は未計測）うえ、`x` で 1 つずつ消すしかなくなる
7. **`x` はサイドバーにフォーカスがあるときだけ効く。** 80 桁ではサイドバーが畳まれ
   （`sidebarCols() == 0`）、`h` も効かない。そこで `x` が効くと、画面に出ていない行が
   消える。`m.focus == paneSidebar` をゲートにする
8. **`a` は一時行を一覧に「昇格」させる。** カレントのリポジトリを永続化する経路は
   これしか無い（設計 §4「一覧に加えるのは利用者が `a` を押したときだけ」）。
   同名で既に一時行がある場合は「重複」ではなく昇格として扱う。ダイアログを開くとき、
   カーソルが一時行にあるならその名前を入力欄に入れておく
9. **サイドバー末尾の「＋ リポジトリを追加」にカーソルは止まらない。** クリックだけで
   反応する。`m.selected` は「今 `ListPRs` を引いているリポジトリ」の index であり、
   `rowNames()` と `repoCountsMsg` の index 対応もそこに乗っている。ボタンを行として
   混ぜるとその対応が全部崩れる。キーボードからは `a` で届く

---

## ファイル構成

| ファイル | 責務 |
|---|---|
| `internal/gh/gh.go`（変更） | `SplitRepo` — `owner/name` の形の判定。`cli` と `tui` の両方から使う |
| `internal/config/config.go`（変更） | `Store` — 設定ファイルの読み書き。`SaveRepositories` は temp + rename |
| `internal/gh/cli/search.go`（新規） | `SearchRepos` — 候補検索 |
| `internal/gh/cli/own_repos.go`（新規） | `ListOwnRepos` / `ListOrgs` — 初回投入の材料 |
| `internal/usecase/repos.go`（新規） | `SaveRepositories` / `SearchRepos` / `SeedCandidates` |
| `internal/tui/dialog/dialog.go`（新規） | ダイアログの状態と入力（`textinput`、候補、カーソル、フォーカス） |
| `internal/tui/dialog/render.go`（新規） | ダイアログの箱の描画 |
| `internal/tui/repo/repo.go`（変更） | `a` / `x`、保存、ダイアログの保持、候補取得のデバウンス |
| `internal/tui/repo/rows.go`（変更） | 行の追加・削除と、保存する名前の切り出し |
| `internal/tui/repo/sidebar.go`（変更） | ＋ 行、初回投入の導線 |
| `internal/tui/repo/mouse.go`（変更） | ＋ 行の当たり判定 |
| `internal/tui/app/app.go`（変更） | 現在リポジトリの確定を `repo` に伝える |

---

### Task 1: `owner/name` の判定を `internal/gh` に出す

ダイアログは保存前に入力の形を弾く必要があるが、その判定
（`internal/gh/cli/repo_counts.go` の `splitOwnerRepo`）は非公開で `internal/tui` から
見えない。同じ判定を 2 つ持つと、片方だけ直る日が来る。

**Files:**
- Modify: `internal/gh/gh.go`
- Modify: `internal/gh/cli/repo_counts.go:100-113`
- Test: `internal/gh/gh_test.go`

**Interfaces:**
- Produces: `func gh.SplitRepo(repo string) (owner, name string, ok bool)`

- [ ] **Step 1: 失敗するテストを書く**

`internal/gh/gh_test.go` に足す。パッケージ宣言は既存ファイルに合わせる。

```go
func TestSplitRepo(t *testing.T) {
	cases := []struct {
		name       string
		in         string
		wantOwner  string
		wantName   string
		wantOK     bool
	}{
		{"owner and name", "kukv/octoscope", "kukv", "octoscope", true},
		{"no slash", "octoscope", "", "", false},
		{"empty owner", "/octoscope", "", "", false},
		{"empty name", "kukv/", "", "", false},
		{"three parts", "github.com/kukv/octoscope", "", "", false},
		{"surrounding space", " kukv/octoscope ", "", "", false},
		{"empty", "", "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			owner, name, ok := gh.SplitRepo(c.in)
			if owner != c.wantOwner || name != c.wantName || ok != c.wantOK {
				t.Errorf("SplitRepo(%q) = (%q, %q, %v), want (%q, %q, %v)",
					c.in, owner, name, ok, c.wantOwner, c.wantName, c.wantOK)
			}
		})
	}
}
```

- [ ] **Step 2: 落ちることを確認する**

Run: `go test ./internal/gh/ -run TestSplitRepo`
Expected: `undefined: gh.SplitRepo` でコンパイルエラー

- [ ] **Step 3: `internal/gh/gh.go` に移す**

`internal/gh/cli/repo_counts.go` の `splitOwnerRepo` を丸ごと移し、名前と doc を変える。
doc コメントに残す外部の事情は「設定ファイルを手で編集したときに紛れ込む前後の空白を
弾く（設定の読み込みは trim しない）」の 1 点だけ。

```go
// SplitRepo reports whether repo has the shape "owner/name": both halves
// non-empty, no second "/", and no leading or trailing whitespace that a
// hand-edited settings file could carry in unnoticed.
func SplitRepo(repo string) (owner, name string, ok bool) {
	if repo != strings.TrimSpace(repo) {
		return "", "", false
	}
	owner, name, ok = strings.Cut(repo, "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
		return "", "", false
	}
	return owner, name, true
}
```

- [ ] **Step 4: `cli` 側の呼び出しを差し替え、重複を消す**

`internal/gh/cli/repo_counts.go` の `splitOwnerRepo(repo)` を `gh.SplitRepo(repo)` にし、
関数定義を削除する。`strings` の import が他で使われているかを確認してから消す。

- [ ] **Step 5: テストが通ることを確認する**

Run: `go test ./internal/gh/... && make check`
Expected: PASS

- [ ] **Step 6: 空振りしないことを確認する**

`SplitRepo` の `strings.Contains(name, "/")` を消して `go test ./internal/gh/ -run TestSplitRepo`
を走らせ、`three parts` が落ちることを見る。確認したら戻す。

- [ ] **Step 7: コミット**

```bash
git add internal/gh/gh.go internal/gh/gh_test.go internal/gh/cli/repo_counts.go
git commit -m "refactor: let the views check a repository name's shape"
```

---

### Task 2: 設定ファイルに書き戻す

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces:
  - `func config.NewStore(path string) *config.Store`
  - `func (s *config.Store) SaveRepositories(repos []string) error`

**この機能の肝は「壊れた設定ファイルを上書きしないこと」である。**
`Load` が失敗したファイルの上に書くと、`a` を 1 回押しただけで利用者の
`saved_queries` も `language` も既定値に潰れる。

- [ ] **Step 1: 失敗するテストを 4 本書く**

`internal/config/config_test.go` に足す。既存ファイルのパッケージ宣言に合わせる。

```go
// TestSaveRepositoriesKeepsTheOtherSettings is the whole reason Save reads
// before it writes: the sidebar knows only the repository list, and writing
// a Config built from that alone would drop everything else in the file.
func TestSaveRepositoriesKeepsTheOtherSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("language: ja\nicons: nerd\ndefault_tab: repos\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.NewStore(path).SaveRepositories([]string{"kukv/octoscope"}); err != nil {
		t.Fatalf("SaveRepositories: %v", err)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Language != "ja" || got.Icons != "nerd" || got.DefaultTab != "repos" {
		t.Errorf("save dropped the other settings: %+v", got)
	}
	if len(got.Repositories) != 1 || got.Repositories[0] != "kukv/octoscope" {
		t.Errorf("repositories = %v, want [kukv/octoscope]", got.Repositories)
	}
}

// TestSaveRepositoriesRefusesAFileItCannotParse guards the worst outcome
// this feature can have: one keypress flattening a settings file whose YAML
// the user is in the middle of hand-editing.
func TestSaveRepositoriesRefusesAFileItCannotParse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	const broken = "language: [ja\n"
	if err := os.WriteFile(path, []byte(broken), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.NewStore(path).SaveRepositories([]string{"kukv/octoscope"}); err == nil {
		t.Fatal("SaveRepositories overwrote a file it could not parse")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != broken {
		t.Errorf("file = %q, want it untouched %q", raw, broken)
	}
}

// TestSaveRepositoriesCreatesTheFileAndItsDirectory covers the first run:
// nothing under the OS config directory exists yet.
func TestSaveRepositoriesCreatesTheFileAndItsDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "octoscope", "config.yaml")
	if err := config.NewStore(path).SaveRepositories([]string{"kukv/koto"}); err != nil {
		t.Fatalf("SaveRepositories: %v", err)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Repositories) != 1 || got.Repositories[0] != "kukv/koto" {
		t.Errorf("repositories = %v, want [kukv/koto]", got.Repositories)
	}
}

// TestSaveRepositoriesLeavesNoTempBehind is what temp+rename is for: a
// half-written file must never be the one Load reads, and a successful save
// must not litter the config directory either.
func TestSaveRepositoriesLeavesNoTempBehind(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := config.NewStore(path).SaveRepositories([]string{"kukv/koto"}); err != nil {
		t.Fatalf("SaveRepositories: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "config.yaml" {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("directory holds %v, want only config.yaml", names)
	}
}
```

- [ ] **Step 2: 落ちることを確認する**

Run: `go test ./internal/config/`
Expected: `undefined: config.NewStore` でコンパイルエラー

- [ ] **Step 3: `Store` を実装する**

`internal/config/config.go` に足す。`os.CreateTemp` は同じディレクトリに作る
（別のファイルシステムをまたぐと `os.Rename` が失敗する）。

```go
// Store reads and writes one settings file.
type Store struct{ path string }

// NewStore returns a store for the settings file at path.
func NewStore(path string) *Store { return &Store{path: path} }

// SaveRepositories replaces the repository list and leaves every other
// setting as it was. A file that cannot be parsed is not written at all: a
// list is not worth flattening the rest of someone's settings for.
func (s *Store) SaveRepositories(repos []string) error {
	c, err := Load(s.path)
	if err != nil {
		return err
	}
	c.Repositories = repos
	return s.save(c)
}

// save writes through a temporary file in the same directory so that an
// interrupted write cannot leave a half-written settings file behind.
func (s *Store) save(c Config) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	raw, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("encode %s: %w", s.path, err)
	}
	tmp, err := os.CreateTemp(dir, "config-*.yaml")
	if err != nil {
		return fmt.Errorf("create a temporary file in %s: %w", dir, err)
	}
	defer os.Remove(tmp.Name()) // no-op once the rename below has succeeded
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return fmt.Errorf("write %s: %w", tmp.Name(), err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmp.Name(), err)
	}
	if err := os.Rename(tmp.Name(), s.path); err != nil {
		return fmt.Errorf("replace %s: %w", s.path, err)
	}
	return nil
}
```

`Config` の 4 つのフィールドに `omitempty` を足す。足さないと、リポジトリを 1 つ
追加しただけの設定ファイルに `language: ""` が並ぶ。

```go
type Config struct {
	Language   string `yaml:"language,omitempty"`
	Icons      string `yaml:"icons,omitempty"`
	DefaultTab string `yaml:"default_tab,omitempty"`

	// Repositories is the list the Repos tab shows, in the order it shows
	// them.
	Repositories []string `yaml:"repositories,omitempty"`
}
```

- [ ] **Step 4: テストが通ることを確認する**

Run: `go test ./internal/config/ -v`
Expected: 4 本とも PASS

- [ ] **Step 5: 空振りしないことを確認する**

`SaveRepositories` の `Load` のエラー処理を `c = Config{}` に変えて（エラーを握りつぶす形）
`TestSaveRepositoriesRefusesAFileItCannotParse` が落ちることを見る。確認したら戻す。

- [ ] **Step 6: コミット**

```bash
git add internal/config/
git commit -m "feat: write the repository list back to the settings file"
```

---

### Task 3: 候補検索

**Files:**
- Create: `internal/gh/cli/search.go`
- Create: `internal/gh/cli/search_test.go`
- Create: `internal/gh/cli/testdata/search_repos.json`
- Modify: `internal/gh/gh.go`（`RepoCandidate` 型）
- Modify: `internal/gh/cli/testdata/README.md`

**Interfaces:**
- Produces:
  - `type gh.RepoCandidate struct { Name string; Stars int; Private bool }`
  - `func (c *cli.Client) SearchRepos(ctx context.Context, query string, limit int) ([]gh.RepoCandidate, error)`

**実測（2026-09-11）:** `gh search repos lipgloss --limit 5 --json fullName,stargazersCount,isPrivate`
は 1.35 秒。**1 打鍵ごとに叩けない値であり、デバウンスは Task 7 の前提になる。**

- [ ] **Step 1: 実物を録る**

```bash
gh search repos lipgloss --limit 5 --json fullName,stargazersCount,isPrivate \
  > internal/gh/cli/testdata/search_repos.json
```

公開リポジトリの検索なので伏せるものは無い。`internal/gh/cli/testdata/README.md` に
録った日・コマンド・録った理由を既存の項目と同じ書式で足す。

- [ ] **Step 2: 失敗するテストを 3 本書く**

`internal/gh/cli/search_test.go`。既存の `cli_test.go` の差し替え方（`c.run` に関数を
代入する）に合わせる。

```go
// TestSearchReposAsksForTheFieldsTheDialogDraws pins the three fields the
// suggestion row is built from: a name to add, the stars that tell two
// similarly-named repositories apart, and whether it is private.
func TestSearchReposAsksForTheFieldsTheDialogDraws(t *testing.T) {
	var got []string
	c := New("/tmp", "")
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return []byte("[]"), nil
	}
	if _, err := c.SearchRepos(context.Background(), "lipgloss", 5); err != nil {
		t.Fatalf("SearchRepos: %v", err)
	}
	joined := strings.Join(got, " ")
	for _, field := range []string{"fullName", "stargazersCount", "isPrivate"} {
		if !strings.Contains(joined, field) {
			t.Errorf("args %v ask for no %s", got, field)
		}
	}
}

// TestSearchReposPassesTheQueryAsAValue is the shell-injection guard: what
// the user typed must reach gh as one argument and never be read as a flag.
// Measured 2026-09-11: gh search repos --limit 2 --json fullName -- -lipgloss
// returns charmbracelet/lipgloss rather than rejecting -lipgloss as a flag.
func TestSearchReposPassesTheQueryAsAValue(t *testing.T) {
	var got []string
	c := New("/tmp", "")
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return []byte("[]"), nil
	}
	if _, err := c.SearchRepos(context.Background(), "--limit=999", 5); err != nil {
		t.Fatalf("SearchRepos: %v", err)
	}
	for i, arg := range got {
		if arg == "--limit=999" && (i == 0 || got[i-1] != "--") {
			t.Errorf("the query reached gh as a flag: %v", got)
		}
	}
}

// TestSearchReposParsesWhatGitHubReturns reads the recording rather than a
// hand-written body, so a change in what gh prints is caught here.
func TestSearchReposParsesWhatGitHubReturns(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "search_repos.json"))
	if err != nil {
		t.Fatal(err)
	}
	c := New("/tmp", "")
	c.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) { return raw, nil }
	got, err := c.SearchRepos(context.Background(), "lipgloss", 5)
	if err != nil {
		t.Fatalf("SearchRepos: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("no candidates parsed from the recording")
	}
	if got[0].Name == "" {
		t.Errorf("first candidate has no name: %+v", got[0])
	}
	var anyStars bool
	for _, cand := range got {
		if cand.Stars > 0 {
			anyStars = true
		}
	}
	if !anyStars {
		t.Error("no candidate carried a star count")
	}
}
```

- [ ] **Step 3: 落ちることを確認する**

Run: `go test ./internal/gh/cli/ -run TestSearchRepos`
Expected: `c.SearchRepos undefined` でコンパイルエラー

- [ ] **Step 4: 型と実装を書く**

`internal/gh/gh.go` に足す。

```go
// RepoCandidate is one row of the add dialog's suggestions.
type RepoCandidate struct {
	Name    string
	Stars   int
	Private bool
}
```

`internal/gh/cli/search.go`。`--` の後ろに置く理由は外部の事情なのでコメントに残す。

```go
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/kukv/octoscope/internal/gh"
)

const searchRepoFields = "fullName,stargazersCount,isPrivate"

type searchRepoJSON struct {
	FullName        string `json:"fullName"`
	StargazersCount int    `json:"stargazersCount"`
	IsPrivate       bool   `json:"isPrivate"`
}

// SearchRepos looks for repositories whose name or description matches
// query. The query goes after "--" so that a word the user typed starting
// with a dash reaches gh as a search term and not as a flag.
func (c *Client) SearchRepos(ctx context.Context, query string, limit int) ([]gh.RepoCandidate, error) {
	args := []string{
		"search", "repos",
		"--json", searchRepoFields,
		"--limit", strconv.Itoa(limit),
		"--", query,
	}
	out, err := c.read(ctx, c.dir, args...)
	if err != nil {
		return nil, err
	}
	var found []searchRepoJSON
	if err := json.Unmarshal(out, &found); err != nil {
		return nil, fmt.Errorf("parse repo search: %w", err)
	}
	candidates := make([]gh.RepoCandidate, len(found))
	for i, f := range found {
		candidates[i] = gh.RepoCandidate{Name: f.FullName, Stars: f.StargazersCount, Private: f.IsPrivate}
	}
	return candidates, nil
}
```

- [ ] **Step 5: テストが通ることを確認する**

Run: `go test ./internal/gh/cli/ -run TestSearchRepos -v`
Expected: 3 本とも PASS

- [ ] **Step 6: 空振りしないことを確認する**

`args` の `"--"` を消して `TestSearchReposPassesTheQueryAsAValue` が落ちることを見る。
`searchRepoFields` から `stargazersCount` を消して 1 本目が落ちることを見る。
**3 本目は `Stars` のデコードを消して落ちるかを確かめる** — 落ちなければ、そのテストは
録画を読んでいるだけで何も守っていない。確認したら全部戻す。

- [ ] **Step 7: コミット**

```bash
git add internal/gh/gh.go internal/gh/cli/search.go internal/gh/cli/search_test.go internal/gh/cli/testdata/
git commit -m "feat: search GitHub for a repository to add"
```

---

### Task 4: 初回投入の材料

**Files:**
- Create: `internal/gh/cli/own_repos.go`
- Create: `internal/gh/cli/own_repos_test.go`
- Create: `internal/gh/cli/testdata/own_repos.json`
- Modify: `internal/gh/cli/testdata/README.md`

**Interfaces:**
- Consumes: `gh.RepoCandidate`（Task 3）
- Produces:
  - `func (c *cli.Client) ListOwnRepos(ctx context.Context, owner string, limit int) ([]gh.RepoCandidate, error)`
  - `func (c *cli.Client) ListOrgs(ctx context.Context) ([]string, error)`

**実測（2026-09-11）:** `gh repo list --limit 100 --json nameWithOwner,isPrivate` は
45 件・6.7 秒。`gh api user/orgs` は 6.1 秒。

- [ ] **Step 1: 実物を録る**

`gh repo list` は private リポジトリの名前を含むので、**公開ぶんだけを残す。**
これは秘密情報の除去であって、テストを通すための編集ではない。

```bash
gh repo list --limit 100 --json nameWithOwner,isPrivate \
  | jq '[.[] | select(.isPrivate == false)][:5]' \
  > internal/gh/cli/testdata/own_repos.json
```

`user/orgs` の録画は作らない。所属 Org 名は伏せる対象であり、`--jq '.[].login'` が
返すのは文字列の配列だけなので、テストは `[]byte("[\"charmbracelet\"]\n")` を
`c.run` から返せば足りる。この判断を `testdata/README.md` に 1 行書く。

- [ ] **Step 2: 失敗するテストを 3 本書く**

`internal/gh/cli/own_repos_test.go`。

```go
// TestListOwnReposAsksForMoreThanTheDefaultThirty: gh repo list fetches 30
// by default and says nothing about the rest, so an account with more
// repositories than that would silently lose them from the seeding list.
func TestListOwnReposAsksForMoreThanTheDefaultThirty(t *testing.T) {
	var got []string
	c := New("/tmp", "")
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return []byte("[]"), nil
	}
	if _, err := c.ListOwnRepos(context.Background(), "", 100); err != nil {
		t.Fatalf("ListOwnRepos: %v", err)
	}
	limit, ok := flagValue(got, "--limit")
	if !ok {
		t.Fatalf("args %v carry no --limit", got)
	}
	n, err := strconv.Atoi(limit)
	if err != nil || n <= 30 {
		t.Errorf("--limit = %q, want a number above gh's default of 30", limit)
	}
}

// TestListOwnReposNamesTheOwnerWhenGivenOne is the difference between "my
// repositories" and "the org's": gh repo list takes the owner as a
// positional argument and lists the authenticated user without one.
func TestListOwnReposNamesTheOwnerWhenGivenOne(t *testing.T) {
	var withOwner, without []string
	c := New("/tmp", "")
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if withOwner == nil && len(args) > 2 && args[2] == "charmbracelet" {
			withOwner = args
		} else {
			without = args
		}
		return []byte("[]"), nil
	}
	if _, err := c.ListOwnRepos(context.Background(), "charmbracelet", 100); err != nil {
		t.Fatalf("ListOwnRepos: %v", err)
	}
	if _, err := c.ListOwnRepos(context.Background(), "", 100); err != nil {
		t.Fatalf("ListOwnRepos: %v", err)
	}
	if withOwner == nil {
		t.Error("the owner never reached gh as a positional argument")
	}
	if slices.Contains(without, "charmbracelet") {
		t.Errorf("the ownerless call carried an owner: %v", without)
	}
}

// TestListOwnReposParsesWhatGitHubReturns reads the recording.
func TestListOwnReposParsesWhatGitHubReturns(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "own_repos.json"))
	if err != nil {
		t.Fatal(err)
	}
	c := New("/tmp", "")
	c.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) { return raw, nil }
	got, err := c.ListOwnRepos(context.Background(), "", 100)
	if err != nil {
		t.Fatalf("ListOwnRepos: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("no repositories parsed from the recording")
	}
	for _, r := range got {
		if _, _, ok := gh.SplitRepo(r.Name); !ok {
			t.Errorf("%q is not owner/name", r.Name)
		}
	}
}

// TestListOrgsReadsTheLoginsOnly pins the shape ListOrgs asks gh for: the
// org logins and nothing else, so no member list or billing detail travels
// through octoscope.
func TestListOrgsReadsTheLoginsOnly(t *testing.T) {
	var got []string
	c := New("/tmp", "")
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return []byte("[\"charmbracelet\"]\n"), nil
	}
	orgs, err := c.ListOrgs(context.Background())
	if err != nil {
		t.Fatalf("ListOrgs: %v", err)
	}
	if len(orgs) != 1 || orgs[0] != "charmbracelet" {
		t.Errorf("orgs = %v, want [charmbracelet]", orgs)
	}
	if !slices.Contains(got, ".[].login") {
		t.Errorf("args %v do not narrow the response to the logins", got)
	}
}
```

`flagValue` は `internal/gh/cli` のテストに既にあるか確認し、無ければこのファイルに置く。

```go
// flagValue returns the argument that follows name.
func flagValue(args []string, name string) (string, bool) {
	for i, a := range args {
		if a == name && i+1 < len(args) {
			return args[i+1], true
		}
	}
	return "", false
}
```

- [ ] **Step 3: 落ちることを確認する**

Run: `go test ./internal/gh/cli/ -run 'TestListOwnRepos|TestListOrgs'`
Expected: コンパイルエラー

- [ ] **Step 4: 実装する**

```go
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/kukv/octoscope/internal/gh"
)

type ownRepoJSON struct {
	NameWithOwner string `json:"nameWithOwner"`
	IsPrivate     bool   `json:"isPrivate"`
}

// ListOwnRepos lists the repositories of owner, or of the authenticated user
// when owner is empty. It carries no star count: gh repo list does not offer
// one, and the seeding list is read by name.
func (c *Client) ListOwnRepos(ctx context.Context, owner string, limit int) ([]gh.RepoCandidate, error) {
	args := []string{"repo", "list"}
	if owner != "" {
		args = append(args, owner)
	}
	args = append(args, "--json", "nameWithOwner,isPrivate", "--limit", strconv.Itoa(limit))
	out, err := c.read(ctx, c.dir, args...)
	if err != nil {
		return nil, err
	}
	var found []ownRepoJSON
	if err := json.Unmarshal(out, &found); err != nil {
		return nil, fmt.Errorf("parse repo list: %w", err)
	}
	repos := make([]gh.RepoCandidate, len(found))
	for i, f := range found {
		repos[i] = gh.RepoCandidate{Name: f.NameWithOwner, Private: f.IsPrivate}
	}
	return repos, nil
}

// ListOrgs names the organisations the authenticated user belongs to. --jq
// narrows the response to the logins in gh, so nothing else about an
// organisation is ever decoded here.
func (c *Client) ListOrgs(ctx context.Context) ([]string, error) {
	out, err := c.read(ctx, c.dir, "api", "user/orgs", "--jq", ".[].login")
	if err != nil {
		return nil, err
	}
	var logins []string
	if err := json.Unmarshal(out, &logins); err != nil {
		return nil, fmt.Errorf("parse orgs: %w", err)
	}
	return logins, nil
}
```

**注意:** `--jq '.[].login'` は JSON の配列ではなく 1 行 1 語を出す。
`json.Unmarshal` で読めるかは Step 5 で実際に確かめ、読めなければ
`strings.Fields(string(out))` に変え、その理由をコメントに残す。
（テストは `c.run` の戻り値を実装に合わせて書き換えるのではなく、
**先に `gh api user/orgs --jq '.[].login' | cat -A` の実出力を見てから**決める。）

- [ ] **Step 5: 実出力を確かめてからテストを通す**

```bash
gh api user/orgs --jq '.[].login' | head -3
```

見た形をテストの `c.run` の戻り値にする。

Run: `go test ./internal/gh/cli/ -run 'TestListOwnRepos|TestListOrgs' -v`
Expected: 4 本とも PASS

- [ ] **Step 6: 空振りしないことを確認する**

`--limit` を消して 1 本目が、`if owner != ""` の分岐を消して 2 本目が、
`--jq` を消して 4 本目が落ちることをそれぞれ見る。確認したら戻す。

- [ ] **Step 7: コミット**

```bash
git add internal/gh/cli/own_repos.go internal/gh/cli/own_repos_test.go internal/gh/cli/testdata/
git commit -m "feat: list the repositories a first run can be seeded from"
```

---

### Task 5: usecase に 3 つの操作を足す

**Files:**
- Create: `internal/usecase/repos.go`
- Create: `internal/usecase/repos_test.go`
- Modify: `internal/usecase/usecase.go`（`source` に `repoFinder` を embed、`New` に store を渡す）
- Modify: `cmd/octoscope/main.go`

**Interfaces:**
- Consumes: `cli.SearchRepos` / `cli.ListOwnRepos` / `cli.ListOrgs`（Task 3・4）、`config.Store.SaveRepositories`（Task 2）
- Produces:
  - `func usecase.New(src source, store repoStore) *Usecase`（署名が変わる）
  - `func (u *Usecase) SaveRepositories(repos []string) error`
  - `func (u *Usecase) SearchRepos(ctx context.Context, query string, limit int) ([]gh.RepoCandidate, error)`
  - `func (u *Usecase) SeedCandidates(ctx context.Context) ([]gh.RepoCandidate, error)`

**`SeedCandidates` が usecase にある理由:** `repo list` → `user/orgs` → Org ごとの
`repo list` で 3 種類・最低 2 回の呼び出しになる。`.claude/rules/architecture.md` は
「`tea.Cmd` のクロージャに 2 つ以上の API 呼び出しを並べない」と定めている。

- [ ] **Step 1: 失敗するテストを 4 本書く**

`internal/usecase/repos_test.go`。既存の `usecase_test.go` のフェイクの作り方に合わせる。

```go
// fakeRepoSource records what SeedCandidates asked for, in order.
type fakeRepoSource struct {
	orgs      []string
	byOwner   map[string][]gh.RepoCandidate
	owners    []string
	searchErr error
	orgsErr   error
}

func (f *fakeRepoSource) SearchRepos(_ context.Context, query string, limit int) ([]gh.RepoCandidate, error) {
	if f.searchErr != nil {
		return nil, f.searchErr
	}
	return []gh.RepoCandidate{{Name: "charmbracelet/" + query, Stars: limit}}, nil
}

func (f *fakeRepoSource) ListOwnRepos(_ context.Context, owner string, _ int) ([]gh.RepoCandidate, error) {
	f.owners = append(f.owners, owner)
	return f.byOwner[owner], nil
}

func (f *fakeRepoSource) ListOrgs(context.Context) ([]string, error) {
	return f.orgs, f.orgsErr
}

type fakeStore struct {
	saved []string
	err   error
}

func (f *fakeStore) SaveRepositories(repos []string) error {
	f.saved = repos
	return f.err
}

// TestSeedCandidatesCoversTheUserAndEveryOrg is the ordering this operation
// exists for: the sidebar is seeded from the user's own repositories and
// from each organisation they belong to, which no single call answers.
func TestSeedCandidatesCoversTheUserAndEveryOrg(t *testing.T) {
	src := &fakeRepoSource{
		orgs: []string{"charmbracelet", "kukv-org"},
		byOwner: map[string][]gh.RepoCandidate{
			"":              {{Name: "kukv/octoscope"}},
			"charmbracelet": {{Name: "charmbracelet/lipgloss"}},
			"kukv-org":      {{Name: "kukv-org/thing"}},
		},
	}
	got, err := newTestUsecase(src, &fakeStore{}).SeedCandidates(context.Background())
	if err != nil {
		t.Fatalf("SeedCandidates: %v", err)
	}
	var names []string
	for _, c := range got {
		names = append(names, c.Name)
	}
	for _, want := range []string{"kukv/octoscope", "charmbracelet/lipgloss", "kukv-org/thing"} {
		if !slices.Contains(names, want) {
			t.Errorf("candidates %v are missing %s", names, want)
		}
	}
}

// TestSeedCandidatesStillAnswersWhenTheOrgsAreUnreadable: a token without
// the org scope is common, and it must not cost the user their own
// repositories.
func TestSeedCandidatesStillAnswersWhenTheOrgsAreUnreadable(t *testing.T) {
	src := &fakeRepoSource{
		orgsErr: errors.New("HTTP 403"),
		byOwner: map[string][]gh.RepoCandidate{"": {{Name: "kukv/octoscope"}}},
	}
	got, err := newTestUsecase(src, &fakeStore{}).SeedCandidates(context.Background())
	if err != nil {
		t.Fatalf("SeedCandidates: %v", err)
	}
	if len(got) != 1 || got[0].Name != "kukv/octoscope" {
		t.Errorf("candidates = %+v, want the user's own repositories", got)
	}
}

// TestSeedCandidatesDropsDuplicates: a repository the user owns inside an
// organisation is returned by both calls.
func TestSeedCandidatesDropsDuplicates(t *testing.T) {
	src := &fakeRepoSource{
		orgs: []string{"kukv-org"},
		byOwner: map[string][]gh.RepoCandidate{
			"":         {{Name: "kukv-org/thing"}},
			"kukv-org": {{Name: "kukv-org/thing"}},
		},
	}
	got, err := newTestUsecase(src, &fakeStore{}).SeedCandidates(context.Background())
	if err != nil {
		t.Fatalf("SeedCandidates: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("candidates = %+v, want one", got)
	}
}

// TestSaveRepositoriesReachesTheStore is the wiring the sidebar depends on.
func TestSaveRepositoriesReachesTheStore(t *testing.T) {
	store := &fakeStore{}
	if err := newTestUsecase(&fakeRepoSource{}, store).SaveRepositories([]string{"kukv/koto"}); err != nil {
		t.Fatalf("SaveRepositories: %v", err)
	}
	if len(store.saved) != 1 || store.saved[0] != "kukv/koto" {
		t.Errorf("store holds %v, want [kukv/koto]", store.saved)
	}
}
```

`newTestUsecase` は既存の `usecase_test.go` にあるフェイクと合成する小さなヘルパー。
既存のフェイク（`source` の全メソッドを持つもの）を embed し、この 3 メソッドだけ
上書きする形にする。**既存ヘルパーの名前を先に読んでから書く。**

- [ ] **Step 2: 落ちることを確認する**

Run: `go test ./internal/usecase/`
Expected: コンパイルエラー

- [ ] **Step 3: `internal/usecase/repos.go` を書く**

```go
package usecase

import (
	"context"
	"fmt"

	"github.com/kukv/octoscope/internal/gh"
)

// seedLimit is how many repositories one owner contributes to the seeding
// list. gh repo list fetches 30 by default; an account or an organisation
// with more than that would lose the rest without a word.
const seedLimit = 100

// SaveRepositories writes the sidebar's list to the settings file.
func (u *Usecase) SaveRepositories(repos []string) error {
	return u.repoStore.SaveRepositories(repos)
}

// SearchRepos looks for repositories to offer while the add dialog is being
// typed into.
func (u *Usecase) SearchRepos(ctx context.Context, query string, limit int) ([]gh.RepoCandidate, error) {
	return u.repos.SearchRepos(ctx, query, limit)
}

// SeedCandidates is what a first run has to offer: the user's own
// repositories and those of every organisation they belong to. A token
// without the organisation scope is common enough that losing the user's own
// repositories over it would be the wrong trade, so an unreadable
// organisation list is passed over rather than returned.
func (u *Usecase) SeedCandidates(ctx context.Context) ([]gh.RepoCandidate, error) {
	own, err := u.repos.ListOwnRepos(ctx, "", seedLimit)
	if err != nil {
		return nil, fmt.Errorf("list own repos: %w", err)
	}
	seen := make(map[string]bool, len(own))
	var out []gh.RepoCandidate
	add := func(candidates []gh.RepoCandidate) {
		for _, c := range candidates {
			if seen[c.Name] {
				continue
			}
			seen[c.Name] = true
			out = append(out, c)
		}
	}
	add(own)
	orgs, err := u.repos.ListOrgs(ctx)
	if err != nil {
		return out, nil
	}
	for _, org := range orgs {
		repos, err := u.repos.ListOwnRepos(ctx, org, seedLimit)
		if err != nil {
			continue
		}
		add(repos)
	}
	return out, nil
}
```

- [ ] **Step 4: `usecase.go` の配線を変える**

`repoFinder` と `repoStore` を宣言し、`source` に embed し、`Usecase` にフィールドを
2 つ足し、`New` の署名を変える。

```go
type repoFinder interface {
	SearchRepos(ctx context.Context, query string, limit int) ([]gh.RepoCandidate, error)
	ListOwnRepos(ctx context.Context, owner string, limit int) ([]gh.RepoCandidate, error)
	ListOrgs(ctx context.Context) ([]string, error)
}

// repoStore is where the sidebar's list survives a restart.
type repoStore interface {
	SaveRepositories(repos []string) error
}
```

`source` に `repoFinder` を足し、`Usecase` に `repos repoFinder` と `repoStore repoStore`
を足す。`New` は `func New(src source, store repoStore) *Usecase` にし、
`repos: src, repoStore: store` を埋める。

- [ ] **Step 5: `cmd/octoscope/main.go` を直す**

`config.Path()` の戻り値は既に `path` として受けているが、スコープの外に出ていない。
`path` を関数スコープに引き上げ、`config.NewStore(path)` を `usecase.New` に渡す。
`config.Path()` が失敗したときは store を `nil` にせず、**失敗を覚えたままの store を
渡す**（保存しようとした時点で同じエラーを返す）。これは Step 6 のテストが守る。

```go
	var cfg config.Config
	var configErr string
	path, err := config.Path()
	if err != nil {
		configErr = err.Error()
	} else if cfg, err = config.Load(path); err != nil {
		configErr = err.Error()
	}
	...
	uc := usecase.New(client, config.NewStore(path))
```

`config.NewStore("")` が `SaveRepositories` でどうなるかを確かめ、エラーになることを
`internal/config/config_test.go` に 1 本足す。

```go
// TestSaveRepositoriesWithNoPathFails: main builds a store even when it
// could not locate the config directory, so the failure has to surface at
// the save rather than as a silent no-op.
func TestSaveRepositoriesWithNoPathFails(t *testing.T) {
	if err := config.NewStore("").SaveRepositories([]string{"kukv/koto"}); err == nil {
		t.Error("saving to an empty path reported success")
	}
}
```

- [ ] **Step 6: テストが通ることを確認する**

Run: `make check`
Expected: PASS（`usecase.New` の呼び出し元は `cmd/octoscope` と各テストにある。全部直す）

- [ ] **Step 7: 空振りしないことを確認する**

`SeedCandidates` の `seen` の判定を消して `TestSeedCandidatesDropsDuplicates` が、
`orgs` のループを消して 1 本目が落ちることを見る。確認したら戻す。

- [ ] **Step 8: コミット**

```bash
git add internal/usecase/ internal/config/ cmd/octoscope/main.go
git commit -m "feat: give the views a way to search, seed and save repositories"
```

---

### Task 6: `internal/tui/dialog` を新設する

**Files:**
- Create: `internal/tui/dialog/dialog.go`
- Create: `internal/tui/dialog/render.go`
- Create: `internal/tui/dialog/dialog_test.go`
- Modify: `.golangci.yml`（depguard に `tui-dialog`）
- Modify: `internal/i18n/locales/active.en.yaml`, `active.ja.yaml`

**Interfaces:**
- Produces:
  - `func dialog.New(title, hint string) dialog.Model`
  - `func (m dialog.Model) Update(msg tea.Msg) (dialog.Model, tea.Cmd)`
  - `func (m dialog.Model) View() string`
  - `func (m dialog.Model) Query() string` — 入力欄の中身
  - `func (m dialog.Model) Value() string` — 確定する名前（候補にカーソルがあればその名前）
  - `func (m dialog.Model) SetCandidates(c []gh.RepoCandidate) dialog.Model`
  - `func (m dialog.Model) SetValue(v string) dialog.Model` — 入力欄を埋めて開く
  - `func (m dialog.Model) SetWidth(w int) dialog.Model`
  - `func (m dialog.Model) SetError(text string) dialog.Model`

**このパッケージは取得をしない。** `internal/tui/review` と同じで、状態と描画だけを
持ち、候補を引くのは持ち主（`internal/tui/repo`）の仕事である。スライス 3 の
保存クエリのポップアップが 2 人目の利用者になる。

モックアップの箱（上から順に）:

```
リポジトリを追加
┌──────────────────────────────┐
│ charmbracelet/lipglos▌       │
└──────────────────────────────┘
owner/name の形式で入力。候補から選ぶこともできます。

  charmbracelet/lipgloss          ★ 9.1k
  charmbracelet/lipgloss-cli      ★ 42
  charmbracelet/glossary          ★ 18

enter:追加  tab:候補へ  esc:閉じる
```

- [ ] **Step 1: i18n の ID を両カタログに足す**

`active.en.yaml`:

```yaml
dialog:
  add_repo_title:
    other: "Add a repository"
  add_repo_hint:
    other: "Type owner/name, or pick one of the suggestions"
  searching:
    other: "searching..."
  no_candidates:
    other: "(no matches)"
  invalid_name:
    other: "Not an owner/name: "
  already_listed:
    other: "Already in the list: "
```

`active.ja.yaml` に同じ ID を足す。`invalid_name` / `already_listed` は末尾に
GitHub 由来ではない octoscope 自身の findings（入力した名前）が続くので、
前置きだけを翻訳する形にする。

- [ ] **Step 2: 失敗するテストを 5 本書く**

`internal/tui/dialog/dialog_test.go`。

```go
// TestTypingReachesTheQuery is the input path the debounce reads.
func TestTypingReachesTheQuery(t *testing.T) {
	m := dialog.New("Add a repository", "hint")
	for _, r := range "koto" {
		m, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	if m.Query() != "koto" {
		t.Errorf("Query() = %q, want \"koto\"", m.Query())
	}
}

// TestValueIsWhatWasTypedUntilACandidateIsPicked: the dialog adds what the
// user typed unless they moved onto a suggestion, which is the only way a
// name they never typed can be added.
func TestValueIsWhatWasTypedUntilACandidateIsPicked(t *testing.T) {
	m := dialog.New("t", "h")
	for _, r := range "koto" {
		m, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	m = m.SetCandidates([]gh.RepoCandidate{{Name: "kukv/koto", Stars: 3}})
	if m.Value() != "koto" {
		t.Errorf("Value() = %q before tab, want what was typed", m.Value())
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if m.Value() != "kukv/koto" {
		t.Errorf("Value() = %q after tab, want the candidate", m.Value())
	}
}

// TestMovingBackOffTheCandidatesRestoresWhatWasTyped: k past the first
// suggestion returns to the field rather than sticking.
func TestMovingBackOffTheCandidatesRestoresWhatWasTyped(t *testing.T) {
	m := dialog.New("t", "h")
	for _, r := range "koto" {
		m, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	m = m.SetCandidates([]gh.RepoCandidate{{Name: "kukv/koto"}})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if m.Value() != "koto" {
		t.Errorf("Value() = %q, want what was typed", m.Value())
	}
}

// TestNewCandidatesDoNotMoveTheCursorOffTheField: suggestions arrive a
// second after the keystroke that asked for them, and landing on one would
// change what enter adds while the user is still typing.
func TestNewCandidatesDoNotMoveTheCursorOffTheField(t *testing.T) {
	m := dialog.New("t", "h")
	for _, r := range "koto" {
		m, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	m = m.SetCandidates([]gh.RepoCandidate{{Name: "kukv/koto"}})
	if m.Value() != "koto" {
		t.Errorf("Value() = %q, want the field to keep the cursor", m.Value())
	}
}

// TestViewDrawsTheTitleHintAndCandidates covers the box's contents against
// the mockup: all three are what the user reads to know what to type.
func TestViewDrawsTheTitleHintAndCandidates(t *testing.T) {
	m := dialog.New("Add a repository", "Type owner/name").SetWidth(60)
	m = m.SetCandidates([]gh.RepoCandidate{{Name: "kukv/koto", Stars: 12}})
	view := m.View()
	for _, want := range []string{"Add a repository", "Type owner/name", "kukv/koto", "12"} {
		if !strings.Contains(view, want) {
			t.Errorf("view has no %q:\n%s", want, view)
		}
	}
}
```

**キーの表し方は既存のテストに合わせる。** `internal/tui/repo/repo_test.go` の
`key()` ヘルパーが `tea.KeyPressMsg` をどう組んでいるかを先に読み、同じ形にする。

- [ ] **Step 3: 落ちることを確認する**

Run: `go test ./internal/tui/dialog/`
Expected: パッケージが無い

- [ ] **Step 4: `dialog.go` を書く**

```go
// Package dialog is the popup that asks for one name, offering suggestions
// while it is typed into. It draws its own box; the holder places it.
package dialog

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/textinput"

	"github.com/kukv/octoscope/internal/gh"
)

// focus says whether the keys move within the field or down the
// suggestions. There is no third place, so the cursor's position in the
// suggestions doubles as the focus: see candidate.
type Model struct {
	title, hint string
	input       textinput.Model
	candidates  []gh.RepoCandidate

	// cursor is -1 while the field has the focus and indexes candidates
	// otherwise.
	cursor int

	width    int
	errText  string
	searching bool
}

func New(title, hint string) Model {
	in := textinput.New()
	in.Focus()
	return Model{title: title, hint: hint, input: in, cursor: -1}
}

// Query is what has been typed, which is what a search is run for.
func (m Model) Query() string { return m.input.Value() }

// Value is the name enter would add: the suggestion under the cursor, or
// what was typed when the field still has it.
func (m Model) Value() string {
	if c, ok := m.candidate(); ok {
		return c.Name
	}
	return m.input.Value()
}

func (m Model) candidate() (gh.RepoCandidate, bool) {
	if m.cursor < 0 || m.cursor >= len(m.candidates) {
		return gh.RepoCandidate{}, false
	}
	return m.candidates[m.cursor], true
}

// SetCandidates replaces the suggestions without moving the cursor off the
// field: they arrive a second after the keystroke that asked for them, and
// landing on one would change what enter adds mid-sentence.
func (m Model) SetCandidates(c []gh.RepoCandidate) Model {
	m.candidates = c
	m.searching = false
	if m.cursor >= len(c) {
		m.cursor = len(c) - 1
	}
	return m
}

func (m Model) SetWidth(w int) Model {
	m.width = w
	m.input.SetWidth(max(w-4, 10))
	return m
}

func (m Model) SetError(text string) Model { m.errText = text; return m }

// SetValue starts the field with v already in it, cursor at the end.
func (m Model) SetValue(v string) Model {
	m.input.SetValue(v)
	m.input.CursorEnd()
	return m
}

// Searching marks that a request is in flight, which the box says.
func (m Model) Searching() Model { m.searching = true; return m }

// Update handles only the keys that move between the field and the
// suggestions. enter and esc belong to the holder: it is what adds and what
// closes.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	switch key.String() {
	case "tab", "down":
		if m.cursor < len(m.candidates)-1 {
			m.cursor++
		}
		return m, nil
	case "shift+tab", "up":
		if m.cursor >= 0 {
			m.cursor--
		}
		return m, nil
	}
	m.errText = ""
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}
```

- [ ] **Step 5: `render.go` を書く**

```go
package dialog

import (
	"fmt"
	"strings"

	"github.com/kukv/octoscope/internal/i18n"
	"github.com/kukv/octoscope/internal/tui/layout"
	"github.com/kukv/octoscope/internal/tui/theme"
)

// starColumn holds the suggestion's star count, right-aligned.
const starColumn = 10

func (m Model) View() string {
	var b strings.Builder
	b.WriteString(theme.Title().Render(m.title) + "\n\n")
	b.WriteString(m.input.View() + "\n")
	b.WriteString(theme.Dim().Render(m.hint) + "\n")
	b.WriteString(m.candidateLines())
	if m.errText != "" {
		b.WriteString("\n" + theme.Error().Render(m.errText))
	}
	return theme.Popup().Width(m.boxWidth()).Render(layout.ClipLines(b.String(), m.boxWidth()))
}
```

```go
// candidateLines is the suggestion list under the field: what is running,
// what came back, or that nothing matched.
func (m Model) candidateLines() string {
	if m.searching {
		return "\n" + theme.Dim().Render(i18n.T("dialog.searching")) + "\n"
	}
	if len(m.candidates) == 0 {
		return "\n" + theme.Dim().Render(i18n.T("dialog.no_candidates")) + "\n"
	}
	var b strings.Builder
	b.WriteString("\n")
	nameWidth := max(m.boxWidth()-starColumn, 1)
	for i, c := range m.candidates {
		line := padTo(c.Name, nameWidth) + rightTo(theme.Dim().Render(stars(c.Stars)), starColumn)
		if i == m.cursor {
			line = theme.Selected().Render(line)
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

// stars is the count beside a suggestion, or nothing for a repository with
// none -- a bare zero reads as a measurement rather than as an absence.
func stars(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("★ %d", n)
}

// boxWidth keeps the popup readable at eighty columns while stopping it
// from running the full width of a wide terminal.
func (m Model) boxWidth() int {
	if m.width <= 0 {
		return 40
	}
	return max(min(m.width-4, 60), 20)
}

// padTo and rightTo count display columns, not bytes: a Japanese repository
// description takes two columns per character.
func padTo(s string, w int) string {
	s = ansi.Truncate(s, max(w-1, 0), "…")
	return s + strings.Repeat(" ", max(w-ansi.StringWidth(s), 0))
}

func rightTo(s string, w int) string {
	s = ansi.Truncate(s, w, "…")
	return strings.Repeat(" ", max(w-ansi.StringWidth(s), 0)) + s
}
```

`padTo` / `rightTo` は `internal/tui/repo/render.go` の `pad` / `right` と同じ計算だが、
パッケージが違うので写す。**3 人目が要ったら `internal/tui/layout` に出す** — 2 つでは
まだ出さない（`.claude/rules/architecture.md`「一度しか使わないコードに抽象化を
持ち込まない」）。

- [ ] **Step 6: depguard に足す**

`.golangci.yml` の `tui-review` の直後に、同じ形で足す。

```yaml
        # dialog is a popup, not a view: it is held by repo and, in a later
        # slice, by search.
        tui-dialog:
          files:
            - "**/internal/tui/dialog/**"
          deny:
            - pkg: github.com/kukv/octoscope/internal/tui/work
              desc: a popup must not depend on the views that hold it
            - pkg: github.com/kukv/octoscope/internal/tui/repo
              desc: a popup must not depend on the views that hold it
            - pkg: github.com/kukv/octoscope/internal/tui/detail
              desc: a popup must not depend on the views that hold it
            - pkg: github.com/kukv/octoscope/internal/tui/diff
              desc: a popup must not depend on the views that hold it
            - pkg: github.com/kukv/octoscope/internal/tui/app
              desc: a child must not import its parent
```

- [ ] **Step 7: depguard が実際に落ちることを確認する**

`internal/tui/dialog/dialog.go` に一時的に
`_ "github.com/kukv/octoscope/internal/tui/repo"` を足し、`make lint` が落ちることを
見る。確認したら消す。

- [ ] **Step 8: テストが通ることを確認する**

Run: `go test ./internal/tui/dialog/ -v && make check`
Expected: 5 本とも PASS

- [ ] **Step 9: 空振りしないことを確認する**

`SetCandidates` の中で `m.cursor = 0` にして
`TestNewCandidatesDoNotMoveTheCursorOffTheField` が落ちることを見る。確認したら戻す。

- [ ] **Step 10: コミット**

```bash
git add internal/tui/dialog/ .golangci.yml internal/i18n/locales/
git commit -m "feat: add the popup that asks for a repository name"
```

---

### Task 7: `a` でリポジトリを足す

**Files:**
- Modify: `internal/tui/repo/repo.go`
- Modify: `internal/tui/repo/rows.go`
- Modify: `internal/tui/repo/sidebar.go`
- Modify: `internal/tui/repo/mouse.go`
- Modify: `internal/tui/repo/render.go`
- Test: `internal/tui/repo/repo_test.go`, `rows_test.go`, `mouse_test.go`
- Modify: `internal/i18n/locales/active.en.yaml`, `active.ja.yaml`

**Interfaces:**
- Consumes: `usecase.SearchRepos` / `usecase.SaveRepositories`（Task 5）、`dialog.Model`（Task 6）
- Produces: `repo.Source` に `repoEditor` が embed される

**実測の帰結:** 候補検索は 1.35 秒（Task 3）。**1 打鍵ごとには叩かない。**
最後の打鍵から `searchDebounce` 経過した時点で 1 回だけ引く。

- [ ] **Step 1: interface とモードを足す前に、失敗するテストを 4 本書く**

`internal/tui/repo/repo_test.go`。

```go
// TestAOpensTheAddDialog
func TestAOpensTheAddDialog(t *testing.T) {
	m := sized(New(&fakeSource{}, Options{Repositories: []string{"kukv/octoscope"}}), 120)
	m, _ = m.Update(key("a"))
	if !strings.Contains(m.View(), i18n.T("dialog.add_repo_title")) {
		t.Errorf("a did not open the dialog:\n%s", m.View())
	}
}

// TestAddingARepositoryPutsItInTheListAndSavesIt is the whole feature: the
// row appears and survives the next start-up.
func TestAddingARepositoryPutsItInTheListAndSavesIt(t *testing.T) {
	f := &fakeSource{}
	m := sized(New(f, Options{Repositories: []string{"kukv/octoscope"}}), 120)
	m, _ = m.Update(key("a"))
	m = typeInto(m, "kukv/koto")
	m, cmd := m.Update(key("enter"))
	// addSelected returns a batch: the new row's fetch, its counts and the
	// save. drain runs every command in it.
	drain(t, cmd)
	if !slices.Contains(m.rowNames(), "kukv/koto") {
		t.Errorf("rows = %v, want kukv/koto among them", m.rowNames())
	}
	if !slices.Contains(f.saved, "kukv/koto") {
		t.Errorf("saved = %v, want kukv/koto among them", f.saved)
	}
}

// TestAddingPromotesTheTemporaryRow: the repository the user is standing in
// leads the list without being in it, and a is the only thing that can write
// it there. Reading the name already on screen as a duplicate would leave no
// way to keep it.
func TestAddingPromotesTheTemporaryRow(t *testing.T) {
	f := &fakeSource{}
	m := sized(New(f, Options{Repositories: []string{"kukv/octoscope"}, Current: "kukv/koto"}), 120)
	if slices.Contains(savedNames(m.rows), "kukv/koto") {
		t.Fatalf("setup: kukv/koto is already part of the saved list: %v", savedNames(m.rows))
	}
	m, _ = m.Update(key("a"))
	m = typeInto(m, "kukv/koto")
	m, cmd := m.Update(key("enter"))
	drain(t, cmd)
	if !slices.Contains(f.saved, "kukv/koto") {
		t.Errorf("saved = %v, want the temporary row written out", f.saved)
	}
	if n := strings.Count(strings.Join(m.rowNames(), " "), "kukv/koto"); n != 1 {
		t.Errorf("rows = %v, want kukv/koto exactly once", m.rowNames())
	}
}

// TestAddedRowsSurviveTheRepositoryLookup: without --repo the lookup answers
// up to twenty seconds after start-up, and rebuilding the rows from the
// settings file's list as it was read would undo anything added meanwhile.
func TestAddedRowsSurviveTheRepositoryLookup(t *testing.T) {
	f := &fakeSource{}
	m := sized(New(f, Options{Repositories: []string{"kukv/octoscope"}}), 120)
	m, _ = m.Update(key("a"))
	m = typeInto(m, "kukv/koto")
	m, cmd := m.Update(key("enter"))
	drain(t, cmd)
	m, _ = m.SetCurrent("kukv/structure")
	if !slices.Contains(m.rowNames(), "kukv/koto") {
		t.Errorf("rows = %v, want kukv/koto kept across the lookup", m.rowNames())
	}
}

// TestAddingRefusesANameThatIsNotOwnerSlashName: the settings file is
// hand-editable, and a malformed entry there is already read as an
// uncountable row. Writing one on purpose would be worse.
func TestAddingRefusesANameThatIsNotOwnerSlashName(t *testing.T) {
	f := &fakeSource{}
	m := sized(New(f, Options{}), 120)
	m, _ = m.Update(key("a"))
	m = typeInto(m, "octoscope")
	m, _ = m.Update(key("enter"))
	if f.saved != nil {
		t.Errorf("saved %v, want nothing written", f.saved)
	}
	if !strings.Contains(m.View(), i18n.T("dialog.invalid_name")) {
		t.Errorf("the dialog did not say why:\n%s", m.View())
	}
}

// TestAKeystrokeOnlyStartsTheTimer is the first half of the debounce: one
// search has been measured at over a second, so a letter must schedule the
// search rather than run it.
func TestAKeystrokeOnlyStartsTheTimer(t *testing.T) {
	f := &fakeSource{}
	m := sized(New(f, Options{}), 120)
	m, _ = m.Update(key("a"))
	m, cmd := m.Update(key("l"))
	for _, msg := range drain(t, cmd) {
		if _, ok := msg.(candidatesMsg); ok {
			t.Error("a keystroke searched instead of scheduling one")
		}
	}
	if f.searches != 0 {
		t.Errorf("%d searches ran on one keystroke, want none", f.searches)
	}
}

// TestTheTimerRunsTheSearch is the other half: once the pause has elapsed,
// exactly one search runs for everything typed into it.
func TestTheTimerRunsTheSearch(t *testing.T) {
	f := &fakeSource{}
	m := sized(New(f, Options{}), 120)
	m, _ = m.Update(key("a"))
	m = typeInto(m, "lipgloss")
	m, cmd := m.Update(searchTickMsg{gen: m.searchGen})
	drain(t, cmd)
	if f.searches != 1 {
		t.Errorf("%d searches ran after the pause, want exactly one", f.searches)
	}
	if f.lastQuery != "lipgloss" {
		t.Errorf("searched for %q, want everything that was typed", f.lastQuery)
	}
}

// TestAStaleTimerSearchesForNothing: every keystroke schedules a tick, so
// all but the last one arrive after the query has already moved on.
func TestAStaleTimerSearchesForNothing(t *testing.T) {
	f := &fakeSource{}
	m := sized(New(f, Options{}), 120)
	m, _ = m.Update(key("a"))
	m = typeInto(m, "lip")
	stale := m.searchGen - 1
	m, cmd := m.Update(searchTickMsg{gen: stale})
	drain(t, cmd)
	if f.searches != 0 {
		t.Errorf("%d searches ran from a stale timer, want none", f.searches)
	}
}
```

ヘルパーは既存のものを使う。`key(s)` は 1 文字なら
`tea.KeyPressMsg{Code: []rune(s)[0], Text: s}` を返すので、そのまま打鍵に使える。
`drain(t, cmd)` は `tea.Batch` を展開して全部走らせる既存のヘルパーで、
**`cmd()` を 1 回呼ぶ形では `tea.BatchMsg` が返るだけで中身が走らない。**

このファイルに足すのは `typeInto` だけ。

```go
// typeInto presses each character of s in turn, the way the dialog is
// actually filled in.
func typeInto(m Model, s string) Model {
	for _, r := range s {
		m, _ = m.Update(key(string(r)))
	}
	return m
}
```

`fakeSource` に `saved []string`、`searches int`、`lastQuery string`、
`seed []gh.RepoCandidate` を足す。`SearchRepos` は `searches++` と `lastQuery` を
記録して 1 件返す。

- [ ] **Step 2: 落ちることを確認する**

Run: `go test ./internal/tui/repo/ -run 'TestA(Opens|dding)|TestTyping'`
Expected: コンパイルエラー、または `a` が no-op で View にダイアログが出ない

- [ ] **Step 3: `repo.Source` に操作を足す**

`internal/tui/repo/repo.go`。**メソッドは 6 個までなので、新しい宣言に分ける。**

```go
// repoEditor is how the sidebar's own list changes: what to offer while the
// dialog is typed into, and where the result survives a restart.
type repoEditor interface {
	SearchRepos(ctx context.Context, query string, limit int) ([]gh.RepoCandidate, error)
	SaveRepositories(repos []string) error
}
```

`Source` に `repoEditor` を embed する。`internal/tui/app/app.go` の `Source` は
`repo.Source` を embed しているので自動で広がるが、**`app` 側と `repo` 側の
テスト用フェイクは両方コンパイルが割れる。このステップで両方直す。**

- [ ] **Step 4: モードを足す**

`.claude/rules/tui.md` は「並行する bool でモードを表現しない」。`repo` にはまだ
mode が無いので、ここで入れる。

```go
// mode says which overlay is up. The list is what is drawn when none is.
type mode uint8

const (
	modeList mode = iota
	modeAdd
)
```

`Model` に `mode mode`、`dlg dialog.Model`、`searchGen int` を足す。
`Update` の `tea.WindowSizeMsg` の枝で `m.dlg = m.dlg.SetWidth(msg.Width)` も通す。
通さないと、端末の幅が変わってもダイアログの箱が起動時の幅のままになる。

- [ ] **Step 5: キーを配線する**

`handleKey` の先頭で mode を見る。

```go
func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if m.mode == modeAdd {
		return m.handleAddKey(msg)
	}
	switch msg.String() {
	...
	case "a":
		return m.openAddDialog()
	}
}
```

```go
// searchDebounce is how long the dialog waits after the last keystroke
// before it asks GitHub. One search has been measured at over a second, so
// one per keystroke would queue a request behind every letter.
const searchDebounce = 400 * time.Millisecond

// searchLimit is how many suggestions fit the box without scrolling it.
const searchLimit = 5

type searchTickMsg struct{ gen int }

type candidatesMsg struct {
	gen        int
	candidates []gh.RepoCandidate
}
```

`candidatesMsg` も `searchTickMsg` も `gen` で古い答えを捨てる。既存の `m.gen`
（サイドバーのカーソル世代）とは別の数である: 片方はカーソル、もう片方は入力。
**同じ数を使い回すと、カーソルを動かしただけで入力中の候補が捨てられる。**

```go
func (m Model) openAddDialog() (Model, tea.Cmd) {
	m.mode = modeAdd
	m.dlg = dialog.New(i18n.T("dialog.add_repo_title"), i18n.T("dialog.add_repo_hint")).
		SetWidth(m.width)
	// Standing on the repository that leads the list without being in it,
	// a is almost always a request to keep that one, so it starts typed in.
	if i := m.selected; i < len(m.rows) && m.rows[i].temporary {
		m.dlg = m.dlg.SetValue(m.rows[i].name)
	}
	return m, nil
}

func (m Model) handleAddKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeList
		return m, nil
	case "enter":
		return m.addSelected()
	}
	before := m.dlg.Query()
	var cmd tea.Cmd
	m.dlg, cmd = m.dlg.Update(msg)
	if m.dlg.Query() == before {
		return m, cmd
	}
	m.searchGen++
	gen := m.searchGen
	return m, tea.Batch(cmd, tea.Tick(searchDebounce, func(time.Time) tea.Msg {
		return searchTickMsg{gen: gen}
	}))
}

// addSelected puts the dialog's answer in the list and writes the list out.
// The shape is checked here rather than on the way in: a suggestion is
// always well-formed, and rejecting a half-typed name mid-keystroke would
// blink an error at someone who is not finished.
func (m Model) addSelected() (Model, tea.Cmd) {
	name := m.dlg.Value()
	if _, _, ok := gh.SplitRepo(name); !ok {
		m.dlg = m.dlg.SetError(i18n.T("dialog.invalid_name") + name)
		return m, nil
	}
	rows, added := addRow(m.rows, name)
	if !added {
		m.dlg = m.dlg.SetError(i18n.T("dialog.already_listed") + name)
		return m, nil
	}
	m.rows = rows
	m.mode = modeList
	next, cmd := m.selectRow(len(rows) - 1)
	return next, tea.Batch(cmd, fetchCounts(next.src, next.rowNames()),
		saveRepos(next.src, savedNames(next.rows), next.tab))
}

func (m Model) runSearch(gen int) tea.Cmd {
	query := m.dlg.Query()
	if query == "" {
		return nil
	}
	return func() tea.Msg {
		found, err := m.src.SearchRepos(context.Background(), query, searchLimit)
		if err != nil {
			return candidatesMsg{gen: gen}
		}
		return candidatesMsg{gen: gen, candidates: found}
	}
}

func saveRepos(src repoEditor, names []string, t tabID) tea.Cmd {
	return func() tea.Msg {
		if err := src.SaveRepositories(names); err != nil {
			return errMsg{tab: t, kind: noticeSave, err: err}
		}
		return nil
	}
}
```

`Update` に 2 つの受け口を足す。`searchTickMsg` は `gen` が今のものと一致し、かつ
mode が `modeAdd` のときだけ検索を始める。`candidatesMsg` は `gen` が一致するときだけ
`m.dlg.SetCandidates` を通す。

**`errMsg` は `gen`（カーソル世代）で捨てられる。** 保存の失敗はカーソルと無関係なので、
`Update` の `errMsg` の枝で `msg.kind == noticeSave` のときは世代のガードを通さない
形にする。これは前スライスの見送り 9（`errMsg` が 2 つの失敗を兼ねている）に
3 つ目を乗せる形であり、Task 12 の積み残しに書く。

追加の本体は `rows.go` に置く。

```go
// addRow puts name in the list and reports whether anything changed. A row
// already there in any spelling is not repeated -- GitHub treats owner/name
// as case-insensitive -- but a temporary row bearing the name is promoted
// into the list, which is the only way the repository the user is standing
// in ever reaches the settings file.
func addRow(rows []row, name string) ([]row, bool) {
	for i, r := range rows {
		if !strings.EqualFold(r.name, name) {
			continue
		}
		if !r.temporary {
			return rows, false
		}
		rows = slices.Clone(rows)
		rows[i].temporary = false
		return rows, true
	}
	return append(slices.Clone(rows), row{name: name}), true
}

// savedNames is the list as it is written to the settings file: the
// temporary row is the repository the user happens to be standing in, not
// part of the list they are building.
func savedNames(rows []row) []string {
	names := make([]string, 0, len(rows))
	for _, r := range rows {
		if r.temporary {
			continue
		}
		names = append(names, r.name)
	}
	return names
}
```

**`m.opts.Repositories` を正のままにする。** `SetCurrent` は
`buildRows(m.opts.Repositories, name)` で行を作り直すので、`m.rows` だけを変えると
`--repo` 無しの起動で lookup が答えた瞬間（最長 20 秒後）に、足した行が消え
消した行が戻る。`addRow` / `removeRow` を通したあとは必ず
`m.opts.Repositories = savedNames(m.rows)` も更新する。

保存は `tea.Cmd` の中で行い、失敗を既存の通知の仕組みに載せる。`noticeKind` に
`noticeSave` を足し、`prefixID()` に `"notice.save_failed"` を返す枝を足す。
`answeredFetch` は `noticeFetch` しか消さないので、保存の失敗は取得が成功しても
消えない（`o` の失敗と同じ寿命）。

- [ ] **Step 6: ダイアログを描く**

`render.go` の `View()` の先頭に mode の分岐を足す。`internal/tui/detail` の
`View()` と同じ形で、ポップアップは画面を置き換える（前提 1）。

```go
func (m Model) View() string {
	if m.width <= 0 {
		return ""
	}
	if m.mode == modeAdd {
		return m.dlg.View() + "\n" +
			theme.Dim().Render(layout.FitKeyBar(addDialogHints(), m.width))
	}
	...
}

// addDialogHints is the dialog's key bar, most important first. esc leads:
// it is the only way out of the popup.
func addDialogHints() []string {
	return []string{
		i18n.T("footer.dialog.close"),
		i18n.T("footer.dialog.add"),
		i18n.T("footer.dialog.candidates"),
	}
}
```

`footer.dialog.close` / `add` / `candidates`（`esc:close` / `enter:add` /
`tab:suggestions`）を両カタログに足す。

- [ ] **Step 7: ＋ 行を描き、クリックで開く**

`sidebar.go`。**当たり判定を 2 度計算しないため、＋ 行の Y は描画と同じ関数から出す。**

```go
// addRowY is the line the "add a repository" button is drawn on: under the
// repositories on screen, with one blank line between. The hit-test reads
// this rather than counting the lines a second time.
func (m Model) addRowY() int {
	return sidebarTop + min(m.sidebarRows(), len(m.rows)) + 1
}
```

`sidebar()` の末尾に足す。カーソルは止まらない（前提 9）ので `theme.Dim()` で描く。

```go
	lines = append(lines, "", theme.Dim().Render(pad(i18n.T("repos.add_button"), sidebarWidth)))
```

`repos.add_button`（`＋ Add a repository` / `＋ リポジトリを追加`）を両カタログに足す。
全角の `＋` は 2 桁を占めるので、`pad` が `ansi.StringWidth` で数えていることが効く。

`mouse.go` の `handleMouseClick` のサイドバーの枝の先頭に足す。

```go
	if msg.Y == m.addRowY() {
		return m.openAddDialog()
	}
```

`mouse_test.go` に 1 本足す。`addRowY()` を実装から読んで期待値にすると
「実装の鏡」になるので、**行数から自分で数えた Y** をクリックする。

- [ ] **Step 8: キーバーに `a` を足す**

`render.go` の `footerHints()` に `i18n.T("footer.list.add")` を足す。
**位置は `footer.list.refresh` の直後**（`FitKeyBar` は末尾から落とすので、
`d` / `s` / `o` より先に残す。ja の 80 桁で消えないことを Task 8 の golden で確かめる）。

- [ ] **Step 9: テストが通ることを確認する**

Run: `go test ./internal/tui/repo/ -v && make check`
Expected: PASS

- [ ] **Step 10: 空振りしないことを確認する**

- `gh.SplitRepo` の判定を外して `TestAddingRefusesANameThatIsNotOwnerSlashName` が落ちるか
- `searchDebounce` のティックを飛ばして直接検索するようにして `TestTypingDoesNotSearchOnEveryKeystroke` が落ちるか
- `SaveRepositories` の呼び出しを消して `TestAddingARepositoryPutsItInTheListAndSavesIt` が落ちるか

3 つとも見てから戻す。

- [ ] **Step 11: コミット**

```bash
git add internal/tui/repo/ internal/tui/app/ internal/i18n/locales/
git commit -m "feat: add a repository to the sidebar with a"
```

---

### Task 8: `x` で消し、golden を録り直す

**Files:**
- Modify: `internal/tui/repo/repo.go`, `rows.go`, `render.go`
- Test: `internal/tui/repo/repo_test.go`, `rows_test.go`, `golden_test.go`
- Modify: `internal/tui/repo/testdata/*.golden`（再録）
- Modify: `internal/i18n/locales/active.en.yaml`, `active.ja.yaml`

- [ ] **Step 1: 失敗するテストを 5 本書く**

```go
// TestXRemovesTheRowAndSavesTheRest
func TestXRemovesTheRowAndSavesTheRest(t *testing.T) {
	f := &fakeSource{}
	m := sized(New(f, Options{Repositories: []string{"kukv/octoscope", "kukv/koto"}}), 120)
	m, _ = m.Update(key("h")) // focus the sidebar
	m, _ = m.Update(key("j")) // onto kukv/koto
	m, cmd := m.Update(key("x"))
	if cmd != nil {
		m, _ = m.Update(cmd())
	}
	if slices.Contains(m.rowNames(), "kukv/koto") {
		t.Errorf("rows = %v, want kukv/koto gone", m.rowNames())
	}
	if len(f.saved) != 1 || f.saved[0] != "kukv/octoscope" {
		t.Errorf("saved = %v, want [kukv/octoscope]", f.saved)
	}
}

// TestXOnTheTemporaryRowDoesNothing: the repository the user is standing in
// is not in the settings file, so there is nothing to remove -- and removing
// it from the screen would take away the row they came to look at.
func TestXOnTheTemporaryRowDoesNothing(t *testing.T) {
	f := &fakeSource{}
	m := sized(New(f, Options{Repositories: []string{"kukv/octoscope"}, Current: "kukv/koto"}), 120)
	m, _ = m.Update(key("h"))
	m, _ = m.Update(key("x"))
	if !slices.Contains(m.rowNames(), "kukv/koto") {
		t.Errorf("rows = %v, want the temporary row kept", m.rowNames())
	}
	if f.saved != nil {
		t.Errorf("saved %v, want nothing written", f.saved)
	}
}

// TestXDoesNothingWhileTheTableHasTheFocus: at eighty columns the sidebar
// is folded away and h cannot reach it, so an ungated x would remove a row
// that is not on screen.
func TestXDoesNothingWhileTheTableHasTheFocus(t *testing.T) {
	f := &fakeSource{}
	m := sized(New(f, Options{Repositories: []string{"kukv/octoscope", "kukv/koto"}}), 120)
	if m.focus != paneList {
		t.Fatalf("setup: the focus starts on %v, want the table", m.focus)
	}
	m, _ = m.Update(key("x"))
	if len(m.rowNames()) != 2 {
		t.Errorf("rows = %v, want both kept", m.rowNames())
	}
	if f.saved != nil {
		t.Errorf("saved %v, want nothing written", f.saved)
	}
}

// TestRemovedRowsStayRemovedAcrossTheLookup is the mirror of the add case:
// SetCurrent rebuilds the rows, and rebuilding them from the list as it was
// read at start-up would bring a removed repository back.
func TestRemovedRowsStayRemovedAcrossTheLookup(t *testing.T) {
	f := &fakeSource{}
	m := sized(New(f, Options{Repositories: []string{"kukv/octoscope", "kukv/koto"}}), 120)
	m, _ = m.Update(key("h"))
	m, _ = m.Update(key("j"))
	m, cmd := m.Update(key("x"))
	drain(t, cmd)
	m, _ = m.SetCurrent("kukv/structure")
	if slices.Contains(m.rowNames(), "kukv/koto") {
		t.Errorf("rows = %v, want kukv/koto still gone", m.rowNames())
	}
}

// TestRemovingTheSelectedRowFetchesWhatIsNowUnderTheCursor: the right pane
// must not keep showing a repository that is no longer in the list.
func TestRemovingTheSelectedRowFetchesWhatIsNowUnderTheCursor(t *testing.T) {
	f := &fakeSource{}
	m := sized(New(f, Options{Repositories: []string{"kukv/octoscope", "kukv/koto"}}), 120)
	m, _ = m.Update(key("h"))
	m, _ = m.Update(key("j"))
	before := m.selectedRepo()
	if before != "kukv/koto" {
		t.Fatalf("setup: selected %q, want kukv/koto", before)
	}
	m, _ = m.Update(key("x"))
	if m.selectedRepo() != "kukv/octoscope" {
		t.Errorf("selected %q after removal, want kukv/octoscope", m.selectedRepo())
	}
	if !m.loading[m.tab] {
		t.Error("the new row's list was not fetched")
	}
}
```

- [ ] **Step 2: 落ちることを確認する**

Run: `go test ./internal/tui/repo/ -run TestX`
Expected: FAIL（`x` が no-op）

- [ ] **Step 3: `rows.go` に削除を足す**

```go
// removeRow drops the row at i and reports whether it was dropped. The
// temporary row is not in the settings file, so there is nothing to remove.
func removeRow(rows []row, i int) ([]row, bool) {
	if i < 0 || i >= len(rows) || rows[i].temporary {
		return rows, false
	}
	return append(slices.Clone(rows[:i]), rows[i+1:]...), true
}
```

- [ ] **Step 4: `handleKey` に `x` を足す**

**`m.focus == paneSidebar` のときだけ効かせる**（前提 7）。80 桁ではサイドバーが
畳まれて `h` も効かないので、ゲートが無いと画面に出ていない行が消える。

削除したあとは `selectRow(min(m.selected, len(rows)-1))` を通す。`selectRow` が
世代を進め、前の行のリストを捨てて新しい行を引く一本道であり、ここを迂回すると
右ペインに消したリポジトリが残る。

- [ ] **Step 5: キーバーと文言を足す**

`footer.list.remove`（`x:remove` / `x:一覧から削除`）を両カタログに足し、
`footerHints()` の `add` の直後に置く。`notice.save_failed` も両方に足す。

- [ ] **Step 6: テストが通ることを確認する**

Run: `go test ./internal/tui/repo/ -v`
Expected: PASS

- [ ] **Step 7: golden を録り直す**

```bash
make golden
git diff --stat internal/tui/repo/testdata/
cat -v internal/tui/repo/testdata/repo_prs_ja_80.golden
```

**先に確かめること:** `textinput` のカーソルは点滅するので、そのまま録ると
golden が走るたびに変わりうる。`internal/tui/detail` と `internal/tui/diff` の
golden が `textarea` のカーソルをどう固定しているかを読み、同じ扱いにする。
固定の仕組みが無いなら、**録る前に 2 回続けて `make golden` を走らせて
差分が空になることを確かめる。**

**確認すること:**
- ja の 80 桁でキーバーが 80 桁に収まっている
- `a` と `x` のヒントが ja の 80 桁で落ちていない。落ちていたら `footerHints()` の
  並びを見直す（`d` / `s` / `o` より前に置く）
- サイドバー末尾の ＋ 行が 120 / 160 桁に出ており、80 桁ではサイドバーごと畳まれている

golden にダイアログを出した状態を 1 組足す。`golden_test.go` に
`repo_add_dialog_%s_%d` を録るブロックを書く。候補は `SetCandidates` ではなく
**`candidatesMsg` を `Update` に渡して**到達させる（フィールドを直接組み立てない）。

- [ ] **Step 8: 空振りしないことを確認する**

`removeRow` の `rows[i].temporary` の判定を消して `TestXOnTheTemporaryRowDoesNothing` が
落ちることを見る。`selectRow` を通す行を消して 3 本目が落ちることを見る。確認したら戻す。

- [ ] **Step 9: コミット**

```bash
git add internal/tui/repo/ internal/i18n/locales/
git commit -m "feat: remove a repository from the sidebar with x"
```

---

### Task 9: 現在リポジトリの確定を待つ

**Files:**
- Modify: `internal/tui/app/app.go`
- Modify: `internal/tui/repo/repo.go`, `sidebar.go`, `render.go`
- Test: `internal/tui/app/app_test.go`, `internal/tui/repo/repo_test.go`

これは `docs/superpowers/2026-09-09-phase4-repos-sidebar-followups.md` の見送り 5 番で、
**Task 10 の導線の出現条件がこれに乗る**ため、ここで回収する。

設定ファイルが空で `--repo` も無いとき、`repo.Model` は起動直後から
`repos.none` を描く。だが `app.resolveRepo` は最長 20 秒かかり（cold で 6 秒超の実測が
`app.go` の `repoLookupTimeout` のコメントにある）、その間「本当に空」なのか
「まだ調べている」のかを利用者は区別できない。

- [ ] **Step 1: 失敗するテストを 2 本書く**

```go
// TestTheEmptySidebarWaitsForTheLookup: with no settings file and no --repo,
// the list is empty for as long as the repository lookup takes -- measured
// at over six seconds cold. Saying "no repositories yet" during it would be
// an answer octoscope does not have.
func TestTheEmptySidebarWaitsForTheLookup(t *testing.T) {
	m := sized(New(&fakeSource{}, Options{}), 120)
	if strings.Contains(m.View(), i18n.T("repos.none")) {
		t.Errorf("the list answered before the lookup did:\n%s", m.View())
	}
}

// TestTheEmptySidebarSaysSoOnceTheLookupAnswers
func TestTheEmptySidebarSaysSoOnceTheLookupAnswers(t *testing.T) {
	m := sized(New(&fakeSource{}, Options{}), 120)
	m, _ = m.SetCurrent("")
	if !strings.Contains(m.View(), i18n.T("repos.none")) {
		t.Errorf("the list never answered:\n%s", m.View())
	}
}
```

`internal/tui/app/app_test.go` にも 1 本。**これが無いと、`repo` 側が確定を待てても
`app` がそれを伝えないままになる。**

```go
// TestTheReposTabWaitsForTheLookupBeforeCallingItEmpty: repoResolvedMsg is
// the only thing that can settle the question, so the root has to pass on
// an empty answer as well as a name.
func TestTheReposTabWaitsForTheLookupBeforeCallingItEmpty(t *testing.T) {
	m := sized(New(newFakeSource(), Options{}), 120, 40)
	m, _ = m.Update(key("2"))
	if strings.Contains(m.View(), i18n.T("repos.none")) {
		t.Errorf("the tab answered before the lookup did:\n%s", m.View())
	}
	m, _ = m.Update(repoResolvedMsg{})
	if !strings.Contains(m.View(), i18n.T("repos.none")) {
		t.Errorf("the tab never answered:\n%s", m.View())
	}
}
```

- [ ] **Step 2: 落ちることを確認する**

Run: `go test ./internal/tui/repo/ -run TestTheEmptySidebar`
Expected: 1 本目が FAIL

- [ ] **Step 3: `repo.Model` に確定の印を持たせる**

```go
	// currentSettled says the lookup that names the working directory's
	// repository has answered, whatever it answered. Before it has, an empty
	// list is not an answer: it is a question still open.
	currentSettled bool
```

`New` では `opts.Current != ""` のときだけ true にする（`--repo` は起動時に確定して
いる）。`SetCurrent` は名前が空でも true にする。

`body()` の `len(m.rows) == 0` の枝を、`currentSettled` が false ならスピナー行に
分ける。

- [ ] **Step 4: `app` 側から必ず伝える**

`repoResolved` は今、名前が空のときに `SetCurrent` を呼んでいない。空でも呼ぶ形にし、
呼ばない条件（あれば）をコメントに残す。**タイムアウトのときも呼ぶ**: 時間切れは
「まだ調べている」ではない。

- [ ] **Step 5: テストが通ることを確認する**

Run: `go test ./internal/tui/... && make check`
Expected: PASS

既存のテストが落ちたら、それは「どのタブから始まる前提か」と同じ種類の空振りである
（`docs/superpowers/plans/2026-09-07-phase2-followups.md` の教訓）。落ちたテストが
何を前提にしていたかを 1 つずつ確かめてから直す。

- [ ] **Step 6: golden を録り直す**

`repo_empty_*` は `Options{}` で組まれているので、**スピナー行に変わる。**
確定後の空の一覧を録る golden を別名（`repo_empty_settled_*`）で足す。

```bash
make golden && git diff internal/tui/repo/testdata/
```

- [ ] **Step 7: 空振りしないことを確認する**

`currentSettled` を常に true にして 1 本目が落ちることを見る。確認したら戻す。

- [ ] **Step 8: コミット**

```bash
git add internal/tui/repo/ internal/tui/app/
git commit -m "fix: do not call an unfinished lookup an empty list"
```

---

### Task 10: 初回投入の導線

**Files:**
- Modify: `internal/tui/repo/repo.go`, `sidebar.go`, `render.go`
- Test: `internal/tui/repo/repo_test.go`, `golden_test.go`
- Modify: `internal/i18n/locales/active.en.yaml`, `active.ja.yaml`

**前提 6 のとおり、取得したものは一覧に直接入れず、ダイアログの候補として出す。**
実測（2026-09-11）で `gh repo list` は 45 件・6.7 秒、`user/orgs` は 6.1 秒、
Org ごとにもう 1 回。**合計は 13 秒からで、Org の数だけ伸びる。**

- [ ] **Step 1: 失敗するテストを 3 本書く**

```go
// TestTheEmptySidebarOffersToSeedItself
func TestTheEmptySidebarOffersToSeedItself(t *testing.T) {
	m := sized(New(&fakeSource{}, Options{}), 120)
	m, _ = m.SetCurrent("")
	if !strings.Contains(m.View(), i18n.T("repos.seed_hint")) {
		t.Errorf("the empty list offers no way forward:\n%s", m.View())
	}
}

// TestSeedingOpensTheDialogWithWhatWasFound: the candidates are offered, not
// added. gh repo list has been measured at 45 repositories for one account,
// and adding all of them would make the count query 45 aliases wide and
// leave x as the only way back.
func TestSeedingOpensTheDialogWithWhatWasFound(t *testing.T) {
	f := &fakeSource{seed: []gh.RepoCandidate{{Name: "kukv/octoscope"}, {Name: "kukv/koto"}}}
	m := sized(New(f, Options{}), 120)
	m, _ = m.SetCurrent("")
	m, cmd := m.Update(key("g"))
	if cmd == nil {
		t.Fatal("g started nothing")
	}
	m, _ = m.Update(cmd())
	view := m.View()
	if !strings.Contains(view, "kukv/octoscope") || !strings.Contains(view, "kukv/koto") {
		t.Errorf("the dialog does not offer what was found:\n%s", view)
	}
	if slices.Contains(m.rowNames(), "kukv/koto") {
		t.Errorf("rows = %v, want nothing added without the user saying so", m.rowNames())
	}
}

// TestSeedingSaysWhenItIsStillRunning: the three calls behind it take over
// ten seconds together, and a dialog that opens empty reads as a failure.
func TestSeedingSaysWhenItIsStillRunning(t *testing.T) {
	m := sized(New(&fakeSource{}, Options{}), 120)
	m, _ = m.SetCurrent("")
	m, _ = m.Update(key("g"))
	if !strings.Contains(m.View(), i18n.T("dialog.searching")) {
		t.Errorf("nothing says the seeding is running:\n%s", m.View())
	}
}
```

- [ ] **Step 2: 落ちることを確認する**

Run: `go test ./internal/tui/repo/ -run 'TestTheEmptySidebarOffers|TestSeeding'`
Expected: コンパイルエラー

- [ ] **Step 3: `repoEditor` に `SeedCandidates` を足す**

3 メソッドになる（上限 6 以内）。`repo` と `app` のフェイクを両方直す。

- [ ] **Step 4: `g` を配線する**

`handleKey` の `modeList` の枝に `g` を足す。`repos.none` のときだけ効く形にはせず、
いつでも効く形にする（一覧が育ったあとに Org を足したくなる。`a` と同じ理由）。
`g` は `modeAdd` を開き、`m.dlg = m.dlg.Searching()` で走っていることを言ってから
`SeedCandidates` を走らせ、答えを `candidatesMsg` で受ける — 検索と同じ受け口を使うので、新しいメッセージ型は要らない。
**ただし世代は共有できない**（検索はデバウンスのティックから、投入はキーから始まる）ので、
`m.searchGen++` をどちらの入口でも通す。

`repos.seed_hint`（`g: import your repositories` / `g: 自分のリポジトリを取り込む`）と
`footer.list.seed` を両カタログに足す。空の一覧のときは `body()` の `repos.none` の
下にヒントを 1 行足す。

- [ ] **Step 5: テストが通ることを確認する**

Run: `go test ./internal/tui/repo/ -v && make check`
Expected: PASS

- [ ] **Step 6: golden**

```bash
make golden && git diff internal/tui/repo/testdata/
```

`repo_empty_settled_*` にヒントの行が増えているはず。ja の 80 桁で桁が溢れていないこと。

- [ ] **Step 7: 空振りしないことを確認する**

`g` の枝で `SeedCandidates` の代わりに空のスライスを返すようにして
`TestSeedingOpensTheDialogWithWhatWasFound` が落ちることを見る。確認したら戻す。

- [ ] **Step 8: コミット**

```bash
git add internal/tui/repo/ internal/i18n/locales/
git commit -m "feat: offer the user's own repositories on a first run"
```

---

### Task 11: キーだけで通すシナリオテスト

**Files:**
- Modify: `internal/tui/app/scenario_test.go`

`.claude/rules/testing.md`:「画面をまたぐ操作は `internal/tui/app` にキー入力だけの
シナリオテストを置く」。ここまでのタスクは `repo` の中だけを見ており、
**`a` が `app` のキー配分を通って `repo` に届くことは誰も確かめていない。**

- [ ] **Step 1: 失敗するテストを 2 本書く**

既存の 3 本と同じ形（`Update` に `tea.KeyPressMsg` を順に渡し、`View()` を見る）で書く。

```go
// TestAddingARepositoryFromTheReposTab walks the keys a user actually
// presses. It is not a second copy of the repo package's own test: what it
// covers is that a, the letters and enter reach the Repos tab through the
// root's key routing at all.
func TestAddingARepositoryFromTheReposTab(t *testing.T) {
	f := newFakeSource() // the app package's existing fake
	m := sized(New(f, Options{Repositories: []string{"kukv/octoscope"}}), 120, 40)

	m, _ = m.Update(key("2"))
	if !strings.Contains(m.View(), "kukv/octoscope") {
		t.Fatalf("setup: the Repos tab is not showing the settings file's list:\n%s", m.View())
	}
	if strings.Contains(m.View(), "kukv/koto") {
		t.Fatalf("setup: kukv/koto is listed before it was added:\n%s", m.View())
	}

	m, _ = m.Update(key("a"))
	for _, r := range "kukv/koto" {
		m, _ = m.Update(runeKey(r))
	}
	m, cmd := m.Update(key("enter"))
	if cmd != nil {
		m, _ = m.Update(cmd())
	}
	if !strings.Contains(m.View(), "kukv/koto") {
		t.Errorf("the added repository never reached the sidebar:\n%s", m.View())
	}
}

// TestRemovingARepositoryFromTheReposTab is the same route for x, including
// the pane move h that has to reach the sidebar first.
func TestRemovingARepositoryFromTheReposTab(t *testing.T) {
	f := newFakeSource()
	m := sized(New(f, Options{Repositories: []string{"kukv/octoscope", "kukv/koto"}}), 120, 40)

	m, _ = m.Update(key("2"))
	if !strings.Contains(m.View(), "kukv/koto") {
		t.Fatalf("setup: kukv/koto is not listed to begin with:\n%s", m.View())
	}

	m, _ = m.Update(key("h"))
	m, _ = m.Update(key("j"))
	m, cmd := m.Update(key("x"))
	if cmd != nil {
		m, _ = m.Update(cmd())
	}
	if strings.Contains(m.View(), "kukv/koto") {
		t.Errorf("x did not remove the row:\n%s", m.View())
	}
	if !strings.Contains(m.View(), "kukv/octoscope") {
		t.Errorf("x took the wrong row:\n%s", m.View())
	}
}
```

**ヘルパーの名前は既存の `scenario_test.go` を読んでから合わせる。** `sized` の引数
（幅だけか幅と高さか）、`key` / `runeKey` の有無、フェイクの作り方は既存に従い、
このファイルで新しく作らない。

**前提ガードに使う文字列に注意する。** `#12` のような番号は Repos の一覧も詳細も
描くので、ガードにすると空振りする（`docs/superpowers/plans/2026-09-07-phase2-followups.md`
の `TestClosingFromTheReposTabShowsTheNewState` で実際に起きた）。上の 2 本は
サイドバーにしか出ないリポジトリ名で確かめている。

- [ ] **Step 2: 落ちることを確認する**

Run: `go test ./internal/tui/app/ -run 'TestAddingARepository|TestRemovingARepository'`
Expected: FAIL

- [ ] **Step 3: 通るまで直す**

`app` が `a` を子に渡していない、フェイクが `SearchRepos` を持っていない、といった
配線の穴がここで出る。**`repo` のテストを直して通すのではなく、`app` 側の配線を直す。**

- [ ] **Step 4: 空振りしないことを確認する**

`app.Update` の Repos タブへのキー転送を切って、2 本とも落ちることを見る。確認したら戻す。

- [ ] **Step 5: コミット**

```bash
git add internal/tui/app/scenario_test.go
git commit -m "test: reach the add dialog through app's own key routing"
```

---

### Task 12: 積み残しを書き、実端末での確認を依頼する

**Files:**
- Create: `docs/superpowers/2026-09-11-phase4-repos-dialog-followups.md`
- Modify: `docs/superpowers/specs/2026-09-08-phase4-design.md`（設計と実装がずれた箇所）

- [ ] **Step 1: 設計書の訂正を入れる**

実装して初めて分かった差を設計書に戻す。**少なくとも次の 2 点はこの計画の時点で
分かっている。**

- §4 の「候補は `gh search repos <語> --limit N --json fullName,description,isPrivate`」
  → 実際に使うのは `fullName,stargazersCount,isPrivate`。理由はモックアップの候補行が
  スター数を出しており、`description` は箱の幅に収まらないこと
- §4 の「一覧が空でカレントも無いときは、自分と所属 Org のリポジトリを初期投入する」
  → 初期投入は一覧へ直接入れず、ダイアログの候補として出す。理由は実測
  （45 件・6.7 秒 + 6.1 秒 + Org ごと）

- [ ] **Step 2: 積み残しを書く**

`docs/superpowers/2026-09-09-phase4-repos-sidebar-followups.md` と同じ体裁で、
**直さないと決めた理由も書く。** 着手前から繰り越しが確定しているもの:

- サイドバーの連打が行数ぶんの `gh` を起こす（前スライスの見送り 7）。`gen` に
  `context.WithCancel` を紐づける土台はできているが、このスライスは触っていない
- `errMsg` が取得失敗とブラウザ失敗を兼ねている（見送り 9）。`noticeSave` を足して
  3 種類目が乗ったので、分ける判断はより急を要する
- `o` の失敗通知を消す手段が無い（見送り 2）
- `RepoCounts` が丸ごと失敗した理由が画面に出ない（見送り 6）
- サイドバーが畳まれていても `h/l:ペイン` がキーバーに出る（見送り 8）
- 区切り線ちょうどの桁のクリック（見送り 4）
- 実装中に見つかったものを足す

- [ ] **Step 3: 実端末での確認を依頼する**

TTY の無い環境では代行できないので、利用者に見てほしいものを列挙する。
**最低限これだけは書く。**

- 追加ダイアログ（120 桁以上と `--lang ja` の 80 桁）。入力欄・ヒント・候補・キーが読めること
- 候補が「打鍵を止めてから」出ること。打鍵中に画面が固まらないこと
- `a` → `enter` で行が増え、**octoscope を終了してもう一度起動しても残っていること**
- `x` で消え、同じく再起動後も消えたままであること
- 設定ファイルを手で壊した状態（`language: [ja`）で `a` を押し、
  **設定ファイルが潰れていないこと**（Task 2 が守っているが、実機で 1 度は見る）
- 空の設定・カレントが git リポジトリでない場所での起動 → スピナー → `repos.none` →
  `g` で候補が出ること
- サイドバー末尾の ＋ 行のクリック
- `--lang ja` の 80 桁でキーバーに `a` と `x` が残っていること

- [ ] **Step 4: コミット**

```bash
git add docs/superpowers/
git commit -m "docs: record what the Repos dialog slice left behind"
```

---

## 完了条件

設計 §11 のうち、このスライスが受け持つもの。

1. Repos タブの一覧の**追加と削除が次の起動でも残る**（§11 の 3）
2. カレントのリポジトリが一覧に無くても先頭に出て、**設定ファイルには書かれない**（§11 の 4）
3. golden が en / ja × 80 / 120 / 160 で録れており、**ja の 80 桁でキーバーが収まる**（§11 の 9）
4. `internal/tui` が `internal/gh/cli` も `internal/config` も import していない（§11 の 7）
5. `grep -rn 'TRANSIENT' .claude/rules .golangci.yml` が 0 件のまま
6. `make check` が緑
7. 非テストコードに `Task N` / `spec §N` の参照が無い

このスライスが受け持たないもの: `saved_queries`（スライス 3）、`internal/gh/api`（スライス 4）。
