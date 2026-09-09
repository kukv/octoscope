# Phase 4 スライス 2-2: Repos サイドバー本体 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Repos タブを常設にし、設定ファイルの一覧を左ペインのサイドバーとして描き、選択した行のリポジトリで右ペインを引き直せるようにする。

**Architecture:** 2-1 が用意した口（`config.Repositories`、`ListPRs(ctx, repo)`、`RepoCounts`）を画面につなぐ。リポジトリ名は `internal/config` から `app.Options` に載って `repo.Model` に渡る（`internal/tui` は `internal/config` を import できない）。`repo.Model` は「行の一覧」と「どちらのペインにフォーカスがあるか」を持ち、行が変わったら右ペインを引き直す。**追加ダイアログ・削除・初回投入・設定への書き戻しは 2-3。このスライスは読むだけである。**

**Tech Stack:** Go 1.25、charm.land/bubbletea/v2、`internal/golden`、`internal/i18n`

**Spec:** `docs/superpowers/specs/2026-09-08-phase4-design.md` §4・§8・§9。画面は
`docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md` §4.2（サイドバー）・
§4.0（マウス）・§4.6（幅の劣化）が正。

**前のスライスの積み残し**（`docs/superpowers/2026-09-08-phase4-repos-backend-followups.md`）は
このスライスで片付ける: 重複したリポジトリ名の扱い（Task 1）、`fetchList` が渡す
`repo` を fake に主張させる（Task 5）、20〜50 件での実測（Task 8）。

## 承認が要る判断

**ペインの移動キーに `h` / `l` を提案する。** spec §4.2 はサイドバーの操作キーを
書いていない。`tab` はサブタブ、`a` / `x` は 2-3 で予約済み。Work タブが `h` / `l`
を「列の移動」に使っているので、Repos で「ペインの移動」に使うと横移動の意味が
そろう。Repos タブでは今どちらも未使用である。**この計画の承認をもって採用とする。**

## Global Constraints

- **依存の向き**（`.claude/rules/architecture.md`）: `internal/tui` は `internal/gh/cli` も
  `internal/config` も import しない。**利用側が自分の使う分だけ interface を宣言する**
  （1 宣言に直接並べるメソッドは 6 個まで）
- **GitHub API 固有の文字列を層の外に出さない**
- **`View` は時計を読まない**（`.claude/rules/tui.md`）。時刻は `Update` で拾う
- **当たり判定は描画と同じ関数を読む**（spec §4.0）。レイアウトを 2 度計算しない
- **テスト**（`.claude/rules/testing.md`）: ネットワークもサブプロセスも叩かない。
  **状態は `Update` にメッセージとキーを渡して到達させる。フィールドを直接組み立てない。**
  **実装が組み立てた引数をコピーした期待値を書かない。** 書いた直後に検証対象を
  壊して落ちることを確かめる
- **新しい文字列は `active.en.yaml` と `active.ja.yaml` の両方に足す**
- **コメント**（`.claude/rules/go-style.md`）: 基本は書かない。書くのは外部の事情・
  一見おかしいコードが正しい理由・エクスポートした識別子の doc の 3 つだけ。
  実装計画や設計書への参照をコードに書かない。コメントは英語
- **`make check` が緑でないコミットを作らない**

## File Structure

| ファイル | 責務 |
|---|---|
| `internal/tui/repo/rows.go`（新規） | 行の一覧の組み立て（重複の畳み込み、一時行、選択の追従） |
| `internal/tui/repo/rows_test.go`（新規） | 上の単体テスト |
| `internal/tui/repo/repo.go`（変更） | `Options`、行と focus の状態、選択行でのフェッチ、古い応答の破棄、件数の取り込み |
| `internal/tui/repo/sidebar.go`（新規） | サイドバーの幅・行・バッジの描画 |
| `internal/tui/repo/render.go`（変更） | 右ペインをサイドバーの右に寄せる。ヘッダとキーバー |
| `internal/tui/repo/mouse.go`（変更） | サイドバーの当たり判定、右ペインの X オフセット |
| `internal/tui/repo/repo_test.go` ほか（変更） | fake が `repo` 引数を記録する |
| `internal/tui/repo/golden_test.go`（変更）・`testdata/*`（再録・新規） | サイドバー付き / 畳んだとき |
| `internal/tui/app/app.go`（変更） | `Options` に `Repo` / `Repositories`。Repos タブ常設。`repoResolvedMsg` が名前を運ぶ |
| `internal/tui/app/render.go`（変更） | タブ行から `HasRepo` の分岐を外す |
| `internal/tui/app/*_test.go`・`testdata/*`（変更・再録） | 「リポジトリが無いと Repos タブが無い」を反転 |
| `internal/i18n/locales/active.{en,ja}.yaml`（変更） | サイドバーの見出し・空の文言・キーバーの分解 |
| `cmd/octoscope/main.go`（変更） | `cfg.Repositories` と `--repo` を `Options` に載せる |
| `docs/superpowers/2026-09-09-phase4-repos-sidebar-followups.md`（新規） | 実端末での確認の受け渡しと、残した判断 |

---

### Task 1: 行の一覧を組み立てる

設定ファイルの名前の並びと「今いるリポジトリ」から、サイドバーに出す行を作る。
**このタスクで画面は変わらない**（`View` はまだ行を読まない）。

**Files:**
- Create: `internal/tui/repo/rows.go`
- Create: `internal/tui/repo/rows_test.go`
- Modify: `internal/tui/repo/repo.go`
- Modify: `internal/tui/app/app.go`, `cmd/octoscope/main.go`
- Modify: `internal/tui/repo/repo_test.go`, `internal/tui/repo/golden_test.go`,
  `internal/tui/repo/mouse_test.go`, `internal/tui/app/*_test.go`（`New` の呼び出し）

**Interfaces:**
- Produces:
  - `type Options struct { Repositories []string; Current string }`
  - `func New(src Source, opts Options) Model`
  - `func (m Model) SetCurrent(name string) (Model, tea.Cmd)`
    （Task 1 では `cmd` は常に `nil`。Task 5 が `selectRow` に再取得を入れると値を返すようになる）
  - `type row struct { name string; temporary bool }`
    （`prs` / `issues` / `counted` は Task 3 が足す。このリポジトリの lint は
    未使用フィールドを赤にするので、読む側と同じコミットで生まれる必要がある）
  - `func buildRows(repositories []string, current string) (rows []row, selected int)`
  - `func (m Model) selectRow(i int) (Model, tea.Cmd)`（カーソルが動く唯一の口）
- Consumes: `app.Options`（Task 2 で `Repo` / `Repositories` を足す。ここでは
  `main.go` から `--repo` の値と `cfg.Repositories` を渡すところまで）

- [ ] **Step 1: 失敗するテストを書く**

`internal/tui/repo/rows_test.go`:

```go
package repo

import "testing"

func names(rows []row) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.name
	}
	return out
}

func TestBuildRowsKeepsSettingsOrder(t *testing.T) {
	rows, selected := buildRows([]string{"kukv/octoscope", "kukv/koto"}, "")
	if got := names(rows); got[0] != "kukv/octoscope" || got[1] != "kukv/koto" || len(got) != 2 {
		t.Errorf("rows = %v, want the settings order", got)
	}
	if selected != 0 {
		t.Errorf("selected = %d, want 0", selected)
	}
}

// The settings file is hand-edited, and GitHub treats owner/name as
// case-insensitive: two spellings of one repository must not become two rows.
func TestBuildRowsFoldsDuplicates(t *testing.T) {
	rows, _ := buildRows([]string{"kukv/octoscope", "KUKV/Octoscope", "kukv/koto"}, "")
	if got := names(rows); len(got) != 2 || got[1] != "kukv/koto" {
		t.Errorf("rows = %v, want the duplicate folded away", got)
	}
}

// The current repository is not written to the settings file: it leads the
// list as a temporary row until the user adds it.
func TestBuildRowsPrependsCurrentWhenAbsent(t *testing.T) {
	rows, selected := buildRows([]string{"kukv/koto"}, "kukv/octoscope")
	if got := names(rows); len(got) != 2 || got[0] != "kukv/octoscope" {
		t.Fatalf("rows = %v, want the current repository first", got)
	}
	if !rows[0].temporary {
		t.Error("the current repository was recorded as part of the saved list")
	}
	if selected != 0 {
		t.Errorf("selected = %d, want the current repository selected", selected)
	}
}

func TestBuildRowsSelectsCurrentAlreadyInList(t *testing.T) {
	rows, selected := buildRows([]string{"kukv/koto", "KUKV/Octoscope"}, "kukv/octoscope")
	if len(rows) != 2 {
		t.Fatalf("rows = %v, want no extra row", names(rows))
	}
	if selected != 1 {
		t.Errorf("selected = %d, want the row already in the list", selected)
	}
	if rows[1].temporary {
		t.Error("a row from the settings file was marked temporary")
	}
}

func TestBuildRowsWithoutCurrent(t *testing.T) {
	rows, selected := buildRows(nil, "")
	if len(rows) != 0 || selected != 0 {
		t.Errorf("rows = %v, selected = %d, want nothing to show", names(rows), selected)
	}
}
```

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/tui/repo/ -run TestBuildRows -v`
Expected: FAIL（`undefined: buildRows`、`undefined: row`）

- [ ] **Step 3: `rows.go` を書く**

```go
package repo

import "strings"

// row is one line of the sidebar: a repository and how much is open in it.
type row struct {
	name string

	// temporary marks the repository the user is standing in when it is not
	// in the settings file. It leads the list but is not part of it: only
	// adding it explicitly writes it there.
	temporary bool
}

// buildRows turns the settings file's list into the sidebar's rows and picks
// the one to start on. GitHub treats owner/name as case-insensitive, so two
// spellings of one repository fold into the first one written.
func buildRows(repositories []string, current string) ([]row, int) {
	var rows []row
	seen := make(map[string]int, len(repositories))
	for _, name := range repositories {
		key := strings.ToLower(name)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = len(rows)
		rows = append(rows, row{name: name})
	}
	if current == "" {
		return rows, 0
	}
	if i, ok := seen[strings.ToLower(current)]; ok {
		return rows, i
	}
	return append([]row{{name: current, temporary: true}}, rows...), 0
}
```

- [ ] **Step 4: 通ることを確かめる**

Run: `go test ./internal/tui/repo/ -run TestBuildRows -v`
Expected: PASS

- [ ] **Step 5: 検証対象を壊して落ちることを確かめる**

`buildRows` の `strings.ToLower(name)` を `name` に変えて
`TestBuildRowsFoldsDuplicates` が落ち、`temporary: true` を `false` に変えて
`TestBuildRowsPrependsCurrentWhenAbsent` が落ちることを見てから戻す。

- [ ] **Step 6: `Model` に持たせ、`New` の口を変える**

`internal/tui/repo/repo.go`:

```go
// Options is what the list needs to know before it can draw: the
// repositories the settings file holds, and the one the user is standing in.
type Options struct {
	Repositories []string

	// Current is the repository --repo named, or the one the working
	// directory belongs to. It may be empty, and it may arrive after the
	// model was built: see SetCurrent.
	Current string
}

func New(src Source, opts Options) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	m := Model{src: src, opts: opts}
	m.spin = s
	m.rows, m.selected = buildRows(opts.Repositories, opts.Current)
	m.loading[m.tab] = true
	return m
}

// SetCurrent names the repository the user is standing in once the lookup
// that finds it answers, which is seconds after the model was built. The
// answer can put a new row at the top of the list and select it, so it
// returns the fetch that row needs.
func (m Model) SetCurrent(name string) (Model, tea.Cmd) {
	m.opts.Current = name
	rows, selected := buildRows(m.opts.Repositories, name)
	m.rows = rows
	return m.selectRow(selected)
}

// selectRow is the single way the sidebar's cursor moves: from a key, from
// the mouse, and from the lookup that names the current repository.
func (m Model) selectRow(i int) (Model, tea.Cmd) {
	m.selected = i
	return m, nil
}
```

**コード中に "Task N" や設計書への参照を書かないこと**
（`.claude/rules/go-style.md`）。上のコード片はその規約に従って書いてある。
`selectRow` がまだクリアも再取得もしないのは、それを使う側がまだ無いからで、
コメントで説明することではない。

`Model` に `opts Options`、`rows []row`、`selected int` を足す。

- [ ] **Step 7: 呼び出しを追従させる**

`app.New` は `repo.New(src, repo.Options{})` を渡す（設定の受け渡しは Task 2）。
テストの `New(f)` はすべて `New(f, Options{})` にする。

- [ ] **Step 8: `make check`**

Run: `make check`
Expected: すべて緑（golden は変わらない — 描画に手を入れていない）

- [ ] **Step 9: コミット**

```bash
git add internal/tui/repo internal/tui/app cmd/octoscope
git commit -m "feat: build the Repos sidebar's rows from the settings file

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: Repos タブを常設にする

一覧を設定ファイルに持つ以上、Repos タブはカレントのリポジトリが見つかったかどうかに
関わらず存在する（設計 §4）。`Options.HasRepo` が「タブがあるか」を意味するのをやめる。

**Files:**
- Modify: `internal/tui/app/app.go`, `internal/tui/app/render.go`
- Modify: `cmd/octoscope/main.go`
- Modify: `internal/tui/app/app_test.go`, `mouse_test.go`, `golden_test.go`
- Modify: `internal/i18n/locales/active.{en,ja}.yaml`
- Update: `internal/tui/app/testdata/*.golden`

**Interfaces:**
- Produces: `app.Options{ Repo string; Repositories []string; DefaultRepos bool; ConfigError string }`
  （`HasRepo` は消える）、`repoResolvedMsg{ name string; timedOut bool }`
- Consumes: `repo.Options`, `repo.Model.SetCurrent`（Task 1）

- [ ] **Step 1: 失敗するテストを書く**

`internal/tui/app/app_test.go`（既存の「リポジトリが無いと 2 が効かない」テストを
差し替える）:

```go
// The Repos tab lists what the settings file holds, so it is there whether or
// not the working directory is a repository.
func TestReposTabExistsWithoutACurrentRepository(t *testing.T) {
	m := press(newTestModel(Options{}), "2")
	if m.tab != tabRepos {
		t.Error("2 did not reach the Repos tab")
	}
	if !strings.Contains(content(m), i18n.T("tab.repos")) {
		t.Errorf("the tab row does not offer Repos:\n%s", content(m))
	}
}

// The lookup's answer names the repository, which is what the list needs to
// put a temporary row at the top of the sidebar.
func TestResolvedRepositoryReachesTheList(t *testing.T) {
	m := newTestModel(Options{})
	next, _ := m.Update(repoResolvedMsg{name: "kukv/demo"})
	if got := next.(Model).repo.Current(); got != "kukv/demo" {
		t.Errorf("the list's current repository = %q, want kukv/demo", got)
	}
}

// --repo is a statement about this run: its repository is current from the
// start, without waiting for a lookup.
func TestRepoFlagIsCurrentFromTheStart(t *testing.T) {
	m := New(&fakeSource{}, Options{Repo: "kukv/flagged"})
	if got := m.repo.Current(); got != "kukv/flagged" {
		t.Errorf("the list's current repository = %q, want kukv/flagged", got)
	}
	if m.tab != tabRepos {
		t.Error("--repo did not land on the Repos tab")
	}
}
```

`repo.Model` に `func (m Model) Current() string { return m.opts.Current }` を足す
（テストが読むためだけの getter ではなく、`app` が今のリポジトリを問う唯一の口）。
`content(m)` は `app_test.go:192` の既存ヘルパー（`ansi.Strip(m.View().Content)`）。
**`app.Model.View()` は `tea.View` を返すので、`strings.Contains(m.View(), …)` は
コンパイルできない。**

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/tui/app/ -run 'TestReposTab|TestResolved|TestRepoFlag' -v`
Expected: FAIL（`Options` に `Repo` が無い、`repoResolvedMsg` に `name` が無い）

- [ ] **Step 3: `app.go` を書き換える**

```go
// Options carries what main determined before the UI started.
type Options struct {
	// Repo is what --repo named, if anything. Without the flag the
	// repository of the working directory is not known yet: asking the
	// GitHub layer is a subprocess, and doing it before the UI started left
	// the terminal blank for as long as it took. The root asks as soon as it
	// has a size, and the answer reaches the list then.
	Repo string

	// Repositories is the settings file's list, which the Repos tab shows
	// whether or not the working directory is a repository. internal/tui
	// cannot read internal/config, so the list travels here.
	Repositories []string

	// DefaultRepos says the settings file asks for the Repos tab at
	// start-up. It is weaker than --repo, which is a statement about this run.
	DefaultRepos bool

	// ConfigError is why the settings file could not be read, if it could
	// not. ...（既存のまま）
	ConfigError string
}

type repoResolvedMsg struct {
	name     string
	timedOut bool
}

func resolved(ctx context.Context, name string, err error) repoResolvedMsg {
	if err != nil {
		return repoResolvedMsg{timedOut: ctx.Err() != nil}
	}
	return repoResolvedMsg{name: name}
}

func (m Model) repoResolved(msg repoResolvedMsg) (tea.Model, tea.Cmd) {
	m.repoLookupTimedOut = msg.timedOut
	if msg.name == "" {
		return m, nil
	}
	var cmd tea.Cmd
	m.repo, cmd = m.repo.SetCurrent(msg.name)
	if m.wantRepos {
		m.wantRepos = false
		m.tab = tabRepos
	}
	return m, cmd
}
```

`New` は次のようにする:

```go
func New(src Source, opts Options) Model {
	m := Model{
		src:  src,
		opts: opts,
		work: work.New(src),
		repo: repo.New(src, repo.Options{
			Repositories: opts.Repositories,
			Current:      opts.Repo,
		}),
	}
	if opts.Repo != "" {
		m.tab = tabRepos
	}
	m.wantRepos = opts.DefaultRepos
	return m
}
```

`m.opts.HasRepo` の分岐をすべて外す:

| 場所 | 変更 |
|---|---|
| `render.go` `tabLabels` | Repos のラベルを常に足す |
| `handleKey` の `"2"` | 条件なしで `m.tab = tabRepos` |
| `broadcast` | `m.repo` に常に配る |
| `refreshLists` | `m.repo.Refresh()` を常に呼ぶ |
| `resize` の初回分岐 | `m.repo.Init()` を常に呼び、`--repo` が無いときは加えて `resolveRepo(m.src)` |
| `repoResolved` | 上のとおり。サイズは `broadcast` が最初から配っているので、ここでの `WindowSizeMsg` の後追いは要らない — **消す** |

- [ ] **Step 4: 文言を直す**

`tab.repo_lookup_timeout` は「タブが出ない」ではなく「今いるリポジトリが分からない」
になった。両方の locale で書き換える:

- en: `could not tell which repository this is`
- ja: `現在のリポジトリを特定できませんでした`

- [ ] **Step 5: `main.go` を配線する**

```go
p := tea.NewProgram(app.New(uc, app.Options{
	Repo:         *repoFlag,
	Repositories: cfg.Repositories,
	DefaultRepos: cfg.WantsRepos(),
	ConfigError:  configError,
}), ...)
```

- [ ] **Step 6: テストを通す**

Run: `go test ./internal/tui/app/ ./cmd/... -v`
Expected: PASS。`Options{HasRepo: true}` は `Options{Repo: "kukv/demo"}` に、
`Options{HasRepo: false}` は `Options{}` に読み替える。
`TestReposTabHiddenWithoutRepo` の類は消し、Step 1 のテストが置き換える。

- [ ] **Step 7: golden を録り直す**

Run: `OCTOSCOPE_UPDATE_GOLDEN=1 go test ./internal/tui/app/`
`git diff` を読み、**タブ行に `2 Repos` が増えた以外の差が無いこと**を目で確かめる。

- [ ] **Step 8: `make check` してコミット**

```bash
git add internal/tui/app internal/i18n cmd/octoscope internal/tui/repo
git commit -m "feat: keep the Repos tab whether or not this is a repository

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: サイドバーを描き、フォーカスを動かす

**Files:**
- Create: `internal/tui/repo/sidebar.go`
- Modify: `internal/tui/repo/repo.go`, `internal/tui/repo/render.go`
- Modify: `internal/i18n/locales/active.{en,ja}.yaml`
- Modify: `internal/tui/repo/repo_test.go`, `internal/tui/repo/golden_test.go`
- Update/Create: `internal/tui/repo/testdata/*.golden`

**Interfaces:**
- Produces: `func (m Model) sidebarCols() int`、`func (m Model) sidebar() []string`、
  `type pane int` (`paneSidebar`, `paneList`)、`m.focus pane`
- Consumes: `row`, `m.rows`, `m.selected`（Task 1）

- [ ] **Step 1: 失敗するテストを書く**

`internal/tui/repo/repo_test.go`:

```go
func sidebarModel(f *fakeSource, width int) Model {
	m := sized(New(f, Options{
		Repositories: []string{"kukv/octoscope", "kukv/koto"},
		Current:      "kukv/octoscope",
	}), width)
	m, _ = m.Update(prListMsg{repo: "kukv/octoscope", prs: f.prs})
	return m
}

func TestSidebarListsEveryRepository(t *testing.T) {
	m := sidebarModel(&fakeSource{prs: samplePRs()}, 120)
	view := m.View()
	for _, want := range []string{"kukv/octoscope", "kukv/koto"} {
		if !strings.Contains(view, want) {
			t.Errorf("the sidebar is missing %q:\n%s", want, view)
		}
	}
}

// Design §9: under 100 columns the sidebar folds away and the header keeps
// the name of the repository being shown.
func TestSidebarFoldsAwayWhenNarrow(t *testing.T) {
	m := sidebarModel(&fakeSource{prs: samplePRs()}, 80)
	if strings.Contains(m.View(), "kukv/koto") {
		t.Errorf("the sidebar was drawn at 80 columns:\n%s", m.View())
	}
	if !strings.Contains(m.View(), "kukv/octoscope") {
		t.Errorf("the header lost the current repository:\n%s", m.View())
	}
}

// Every line must fit the terminal: a sidebar that does not subtract itself
// from the table's width runs off the right edge.
func TestEveryLineFitsTheWidth(t *testing.T) {
	for _, w := range []int{80, 100, 120, 160} {
		m := sidebarModel(&fakeSource{prs: samplePRs()}, w)
		for _, line := range strings.Split(m.View(), "\n") {
			if got := ansi.StringWidth(line); got > w {
				t.Errorf("at %d columns a line is %d wide: %q", w, got, line)
			}
		}
	}
}

func TestFocusMovesBetweenPanes(t *testing.T) {
	f := &fakeSource{prs: samplePRs()}
	m := sidebarModel(f, 120)
	if m.focus != paneList {
		t.Fatal("the list did not start focused")
	}
	m, _ = m.Update(key("h"))
	if m.focus != paneSidebar {
		t.Error("h did not move the focus to the sidebar")
	}
	m, _ = m.Update(key("j"))
	if m.selected != 1 {
		t.Errorf("selected = %d, want j to move the sidebar's cursor", m.selected)
	}
	if m.cursors[m.tab] != 0 {
		t.Error("j moved the table's cursor while the sidebar had the focus")
	}
	m, _ = m.Update(key("l"))
	if m.focus != paneList {
		t.Error("l did not move the focus back to the list")
	}
}

// Design §8: the key bar must fit ja at 80 columns. FitKeyBar guarantees the
// width on its own -- it drops hints until they fit -- so measuring the width
// would assert nothing (see docs: the seven tests that could not fail). What
// is worth holding is which hints survive the drop.
func TestKeyBarKeepsTheEssentialKeysInJapaneseAt80(t *testing.T) {
	i18n.SetLanguage(language.Japanese)
	t.Cleanup(func() { i18n.SetLanguage(language.English) })
	bar := sidebarModel(&fakeSource{prs: samplePRs()}, 80).keyBar()
	for _, want := range []string{
		i18n.T("footer.list.move"),
		i18n.T("footer.list.open"),
		i18n.T("footer.list.pane"),
		i18n.T("footer.list.kind"),
		i18n.T("footer.list.quit"),
	} {
		if !strings.Contains(bar, want) {
			t.Errorf("the key bar dropped %q at ja/80: %q", want, bar)
		}
	}
}

// 20-50 repositories is the realistic size of the list (design §2). The
// sidebar must scroll rather than run off the bottom of the terminal.
func TestSidebarScrollsRatherThanOverflowing(t *testing.T) {
	var many []string
	for i := range 50 {
		many = append(many, fmt.Sprintf("kukv/repo-%02d", i))
	}
	f := &fakeSource{prs: samplePRs()}
	m := sized(New(f, Options{Repositories: many}), 120)
	if got := len(strings.Split(m.View(), "\n")); got > 40 {
		t.Errorf("the view is %d lines tall in a 40-line terminal", got)
	}
	m, _ = m.Update(key("h"))
	for range 49 {
		m, _ = m.Update(key("j"))
	}
	if !strings.Contains(m.View(), "kukv/repo-49") {
		t.Errorf("the last row is off screen:\n%s", m.View())
	}
}
```

`sized` は高さ 40 で組む既存のヘルパーに合わせる（`golden_test.go` と同じ）。

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/tui/repo/ -v`
Expected: FAIL（`paneList` が未定義、`prListMsg` がまだ struct でない）

`prListMsg` / `issueListMsg` を struct にするのは Task 5 だが、テストが先に
その形を使うため **このタスクで型だけ先に変える**:

```go
type (
	prListMsg    struct{ repo string; prs []gh.PR }
	issueListMsg struct{ repo string; issues []gh.Issue }
)
```

`Update` の受け口は `m.prs = msg.prs` のように読み替えるだけにし、`repo` を見る
のは Task 5 に置く。

- [ ] **Step 3: `sidebar.go` を書く**

まず `rows.go` の `row` にバッジのフィールドを足す（Task 1 では読む側が無く、
lint の `unused` に落ちるので置けなかった）:

```go
type row struct {
	name      string
	temporary bool

	prs, issues int

	// counted says the badge has an answer. A repository GitHub could not
	// resolve stays uncounted, and the row is drawn without numbers rather
	// than with zeroes.
	counted bool
}
```

```go
package repo

import (
	"fmt"
	"strings"

	"github.com/kukv/octoscope/internal/i18n"
	"github.com/kukv/octoscope/internal/theme"
)

// The sidebar's geometry, read by both View and the hit-test: a fixed column
// of names with their badges, then the rule between the panes.
const (
	sidebarWidth = 30
	sidebarRule  = 1

	// sidebarTop is the first line a repository is drawn on: the heading and
	// the blank line under it. The hit-test reads the same constant.
	sidebarTop = 2

	// badgeColumn holds "12/3" right-aligned, or the dash a repository that
	// could not be counted gets.
	badgeColumn = 8

	// minSidebarWidth is where the sidebar folds away (spec §4.6, design §9).
	minSidebarWidth = 100
)

// sidebarCols is how much of the terminal the sidebar takes, rule included.
// Zero means it is folded away.
func (m Model) sidebarCols() int {
	if m.width < minSidebarWidth || len(m.rows) == 0 {
		return 0
	}
	return sidebarWidth + sidebarRule
}

// sidebarRows is how many repositories fit under the heading, and
// sidebarWindow is the first one drawn, chosen to keep the cursor in view.
// The list is realistically 20-50 long and the terminal is not (design §2),
// so it scrolls exactly as the table does.
func (m Model) sidebarRows() int {
	if m.height <= 0 {
		return len(m.rows)
	}
	return max(m.height-sidebarTop-footerHeight, 1)
}

func (m Model) sidebarWindow() int {
	if m.selected < m.sidebarRows() {
		return 0
	}
	return m.selected - m.sidebarRows() + 1
}

// sidebar draws one line per repository on screen: its name, and how much is
// open in it. A repository whose count could not be fetched keeps its row and
// loses its numbers.
func (m Model) sidebar() []string {
	lines := []string{theme.Dim().Render(pad(i18n.T("repos.sidebar"), sidebarWidth)), ""}
	first := m.sidebarWindow()
	for i := first; i < min(first+m.sidebarRows(), len(m.rows)); i++ {
		r := m.rows[i]
		badge := "—"
		if r.counted {
			badge = fmt.Sprintf("%d/%d", r.prs, r.issues)
		}
		line := pad(r.name, sidebarWidth-badgeColumn) +
			right(theme.Dim().Render(badge), badgeColumn)
		switch {
		case i == m.selected && m.focus == paneSidebar:
			line = theme.Selected().Render(line)
		case i == m.selected:
			line = theme.Accent().Render(line)
		}
		lines = append(lines, line)
	}
	return lines
}

// joinPanes puts the sidebar beside the body, padding whichever is shorter so
// the rule runs the full height of the taller one.
func joinPanes(left, right []string, leftWidth int) []string {
	n := max(len(left), len(right))
	out := make([]string, n)
	for i := range n {
		l, r := "", ""
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		out[i] = pad(l, leftWidth) + theme.Rule().Render("│") + r
	}
	return out
}
```

`theme` の import パスは既存の `github.com/kukv/octoscope/internal/tui/theme` に
合わせること（上の擬似コードは短縮している）。

- [ ] **Step 4: `render.go` を右に寄せる**

- `View` は `m.sidebarCols() > 0` のとき `joinPanes(m.sidebar(), body, sidebarWidth)`
- 右ペインが使える幅を `m.bodyWidth() = m.width - m.sidebarCols()` にし、
  `row` の `titleWidth`・`clip`・`summary`・ヘッダはすべてこれを読む
- ヘッダの名前は `m.repoName` ではなく `m.selectedRepo()`（選択行の名前。
  行が無ければ `i18n.T("app.name")`）
- キーバーは `layout.FitKeyBar(m.footerHints(), m.width)` に変える

`footerHints` は重要な順に並べる:

```go
func (m Model) footerHints() []string {
	return []string{
		i18n.T("footer.list.move"),
		i18n.T("footer.list.open"),
		i18n.T("footer.list.pane"),
		i18n.T("footer.list.kind"),
		i18n.T("footer.list.refresh"),
		i18n.T("footer.list.diff"),
		i18n.T("footer.list.checks"),
		i18n.T("footer.list.web"),
		i18n.T("footer.list.quit"),
	}
}
```

- [ ] **Step 5: 文言を足す**

`active.en.yaml` / `active.ja.yaml`。`footer.list` の 1 本の文字列を分解する
（`footer.detail` と同じ形。並びの意図をコメントに 1 つ書く）:

| キー | en | ja |
|---|---|---|
| `repos.sidebar` | `Repositories` | `リポジトリ` |
| `repos.none` | `No repositories yet` | `リポジトリがまだありません` |
| `footer.list.move` | `j/k:move` | `j/k:移動` |
| `footer.list.open` | `enter:detail` | `enter:詳細` |
| `footer.list.pane` | `h/l:pane` | `h/l:ペイン` |
| `footer.list.kind` | `tab:PR/Issue` | `tab:PR/Issue` |
| `footer.list.refresh` | `r:refresh` | `r:更新` |
| `footer.list.diff` | `d:diff` | `d:diff` |
| `footer.list.checks` | `s:checks` | `s:checks` |
| `footer.list.web` | `o:web` | `o:Web` |
| `footer.list.quit` | `q:quit` | `q:終了` |

古い `footer.list.other` は消す。`internal/i18n/unresolved*.go` が未使用キーや
未定義キーを見ているなら、その検査に従う。

- [ ] **Step 6: フォーカスとキーを実装する**

```go
type pane int

const (
	paneList pane = iota
	paneSidebar
)
```

`handleKey` の頭に:

```go
case "h", "left":
	if m.sidebarCols() > 0 {
		m.focus = paneSidebar
	}
	return m, nil
case "l", "right":
	m.focus = paneList
	return m, nil
```

`j` / `k` は `m.focus == paneSidebar` のとき `m.selectRow(...)` を通す
（Task 1 で作った唯一の口）。**行が変わったときのクリアと再取得は Task 5** が
`selectRow` の中に足す。ここでは選択が動くだけでよい。

サイドバーが畳まれている（`sidebarCols() == 0`）ときは `focus` を `paneList` に
落とす — 見えないペインにフォーカスが残ると `j` が何も動かさないように見える。

- [ ] **Step 7: テストを通す**

Run: `go test ./internal/tui/repo/ -v`
Expected: PASS

- [ ] **Step 8: 検証対象を壊して落ちることを確かめる**

`bodyWidth` から `sidebarCols()` の減算を外して `TestEveryLineFitsTheWidth` が
落ちること、`minSidebarWidth` を `0` にして `TestSidebarFoldsAwayWhenNarrow` が
落ちることを見てから戻す。

- [ ] **Step 9: golden を録り直す**

`goldenModel` に `Options{Repositories: []string{"kukv/octoscope", "kukv/koto"}, Current: "kukv/octoscope"}` を渡すよう直す。

**`goldenModel` は `loadedModel` の上に建っており、`loadedModel` は
`prListMsg{repo: ""}` を配る。Task 5 で `repo` の食い違う応答を捨てるように
なると、この golden は空の表を録ることになる。** `goldenModel` は
`sidebarModel` と同じ形（応答の `repo` に選択行の名前を入れる）で組み直し、
`Options` に値を渡している他のテストも同じく直す。

Run: `OCTOSCOPE_UPDATE_GOLDEN=1 go test ./internal/tui/repo/`
`git diff` を全幅・両言語で読む。**ja の 80 桁でキーバーが 1 行に収まっていること**、
**160 桁でサイドバーの右に表がそろっていること**を目で確かめる。

- [ ] **Step 10: `make check` してコミット**

```bash
git add internal/tui/repo internal/i18n
git commit -m "feat: draw the Repos sidebar beside the list

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: 件数バッジを引く

**Files:**
- Modify: `internal/tui/repo/repo.go`
- Modify: `internal/tui/repo/repo_test.go`, `internal/tui/app/app_test.go`,
  `internal/tui/app/scenario_test.go`（fake に `RepoCounts` を足す）
- Update: `internal/tui/repo/testdata/*.golden`

**Interfaces:**
- Produces: `type repoCounter interface { RepoCounts(ctx context.Context, repos []string) ([]gh.RepoCount, error) }`、
  `Source` に `repoCounter` を足す、`repoCountsMsg []gh.RepoCount`
- Consumes: `gh.RepoCount{Repo string; PRs, Issues int; Unavailable bool}`（2-1）、
  `usecase.RepoCounts`

- [ ] **Step 1: 失敗するテストを書く**

```go
func TestBadgesShowTheCounts(t *testing.T) {
	f := &fakeSource{prs: samplePRs(), counts: []gh.RepoCount{
		{Repo: "kukv/octoscope", PRs: 12, Issues: 3},
		{Repo: "kukv/koto", Unavailable: true},
	}}
	m := sidebarModel(f, 120)
	m, _ = m.Update(repoCountsMsg(f.counts))
	view := m.View()
	if !strings.Contains(view, "12/3") {
		t.Errorf("the badge is missing:\n%s", view)
	}
	if !strings.Contains(view, "—") {
		t.Errorf("a repository that could not be counted lost its row or got a zero:\n%s", view)
	}
}

// RepoCounts answers positionally and rewrites each name to the spelling
// GitHub resolved. Matching by name would drop the answer for a row the user
// spelled differently.
func TestCountsMatchByPositionAndTakeTheResolvedName(t *testing.T) {
	f := &fakeSource{prs: samplePRs()}
	m := sized(New(f, Options{Repositories: []string{"KUKV/Octoscope"}}), 120)
	m, _ = m.Update(repoCountsMsg([]gh.RepoCount{{Repo: "kukv/octoscope", PRs: 1, Issues: 2}}))
	if m.rows[0].name != "kukv/octoscope" {
		t.Errorf("row name = %q, want the spelling GitHub resolved", m.rows[0].name)
	}
}

// A shorter or longer answer than there are rows must not panic.
func TestCountsOfADifferentLengthAreIgnored(t *testing.T) {
	f := &fakeSource{prs: samplePRs()}
	m := sidebarModel(f, 120)
	before := m.rows
	m, _ = m.Update(repoCountsMsg([]gh.RepoCount{{Repo: "kukv/octoscope"}}))
	if m.rows[0].counted != before[0].counted {
		t.Error("a mismatched answer was taken")
	}
}

func TestCountsAreFetchedOnRefresh(t *testing.T) {
	f := &fakeSource{prs: samplePRs()}
	m := sidebarModel(f, 120)
	f.countCalls = nil
	_, cmd := m.Update(key("r"))
	drain(t, cmd)
	if len(f.countCalls) != 1 {
		t.Errorf("RepoCounts called %d times on r, want 1", len(f.countCalls))
	}
	if got := f.countCalls[0]; len(got) != 2 || got[0] != "kukv/octoscope" {
		t.Errorf("RepoCounts got %v, want every row's name", got)
	}
}
```

`fakeSource` に `counts []gh.RepoCount`、`countErr error`、`countCalls [][]string`
と `RepoCounts` を足す。`drain` は `tea.Cmd` を実行して `tea.BatchMsg` を展開する
既存のヘルパーがあればそれを使い、無ければこのファイルに書く。

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/tui/repo/ -run TestCounts -v` / `-run TestBadges -v`
Expected: FAIL

- [ ] **Step 3: 実装する**

```go
// repoCounter is how many pull requests and issues are open in each
// repository of the sidebar. It stands alone because it is the only call
// that looks past the repository on screen.
type repoCounter interface {
	RepoCounts(ctx context.Context, repos []string) ([]gh.RepoCount, error)
}
```

`Source` に足す。`Init` と `Refresh` が `fetchCounts(m.src, m.rowNames())` を
併せて返す。`rowNames` はここで足す（Task 1 では呼び出し元が無く、lint の
`unused` に落ちる）:

```go
func (m Model) rowNames() []string {
	names := make([]string, len(m.rows))
	for i, r := range m.rows {
		names[i] = r.name
	}
	return names
}
```

```go
func fetchCounts(src repoCounter, repos []string) tea.Cmd {
	if len(repos) == 0 {
		return nil
	}
	return func() tea.Msg {
		counts, err := src.RepoCounts(context.Background(), repos)
		if err != nil {
			return repoCountsMsg(nil) // badges stay blank; the lists still work
		}
		return repoCountsMsg(counts)
	}
}
```

`Update`:

```go
case repoCountsMsg:
	if len(msg) != len(m.rows) {
		return m, nil
	}
	for i, c := range msg {
		if c.Unavailable {
			continue
		}
		m.rows[i].name = c.Repo
		m.rows[i].prs, m.rows[i].issues = c.PRs, c.Issues
		m.rows[i].counted = true
	}
	return m, nil
```

**`m.rows` は値レシーバのモデルが持つ slice なので、書き換える前に
`m.rows = slices.Clone(m.rows)` する。** 元の slice を共有したままだと、
Bubble Tea が持っている前のモデルの行まで書き換わる。

件数の取得が失敗しても行は消さない。バッジが「—」のままになるだけである。

- [ ] **Step 4: 通ることを確かめる**

Run: `go test ./internal/tui/repo/ -v`
Expected: PASS

- [ ] **Step 5: 検証対象を壊して落ちることを確かめる**

`m.rows[i].name = c.Repo` を消して
`TestCountsMatchByPositionAndTakeTheResolvedName` が落ちること、
長さの一致判定を外して `TestCountsOfADifferentLengthAreIgnored` が
panic か失敗することを見てから戻す。

- [ ] **Step 6: golden を録り直し、`make check` してコミット**

`goldenModel` に `repoCountsMsg` を配る（バッジが「—」のままの録画は、
バッジの桁を守らない）。1 行は数字、1 行は `Unavailable` にする。

```bash
OCTOSCOPE_UPDATE_GOLDEN=1 go test ./internal/tui/repo/ && make check
git add internal/tui internal/i18n
git commit -m "feat: badge each sidebar row with what is open in it

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: 選択した行のリポジトリを引く

サイドバーの行を移ると、右ペインがその行のリポジトリの PR / Issue になる。
**2-1 の積み残しがここに当たる: fake が受け取った `repo` を記録し、テストが
それを主張しないと、このタスクは「通り続けるが何も守らない」テストになる。**

**Files:**
- Modify: `internal/tui/repo/repo.go`
- Modify: `internal/tui/repo/repo_test.go`, `internal/tui/app/app_test.go`,
  `internal/tui/app/scenario_test.go`

**Interfaces:**
- Produces: `func fetchList(src Source, t tabID, repo string) tea.Cmd`、
  `func (m Model) selectedRepo() string`
- Consumes: `prListMsg{repo, prs}` / `issueListMsg{repo, issues}`（Task 3 で型は導入済み）

- [ ] **Step 1: fake に `repo` を記録させる**

`internal/tui/repo/repo_test.go`・`internal/tui/app/app_test.go`・
`internal/tui/app/scenario_test.go` の 4 つの fake（`fakeSource.ListPRs` /
`ListIssues`、`scenarioSource.ListPRs` / `ListIssues`）に:

```go
func (f *fakeSource) ListPRs(ctx context.Context, repo string) ([]gh.PR, error) {
	f.prRepos = append(f.prRepos, repo)
	return f.prs, f.err
}
```

- [ ] **Step 2: 失敗するテストを書く**

```go
func TestMovingTheSidebarFetchesThatRepository(t *testing.T) {
	f := &fakeSource{prs: samplePRs()}
	m := sidebarModel(f, 120)
	f.prRepos = nil
	m, _ = m.Update(key("h"))
	m, cmd := m.Update(key("j")) // onto kukv/koto
	drain(t, cmd)
	if len(f.prRepos) != 1 || f.prRepos[0] != "kukv/koto" {
		t.Errorf("ListPRs got %v, want the row the cursor moved onto", f.prRepos)
	}
}

// The rows the previous repository's answer would fill must not be shown
// under the new one's name.
func TestMovingTheSidebarClearsTheOldList(t *testing.T) {
	f := &fakeSource{prs: samplePRs()}
	m := sidebarModel(f, 120)
	m, _ = m.Update(key("h"))
	m, _ = m.Update(key("j"))
	if strings.Contains(m.View(), "first pr") {
		t.Errorf("the previous repository's rows are still on screen:\n%s", m.View())
	}
}

// A fetch outlives the row that started it. Its answer must not land under
// another repository's name.
func TestAnAnswerForAnotherRepositoryIsDropped(t *testing.T) {
	f := &fakeSource{prs: samplePRs()}
	m := sidebarModel(f, 120)
	m, _ = m.Update(key("h"))
	m, _ = m.Update(key("j")) // now on kukv/koto
	m, _ = m.Update(prListMsg{repo: "kukv/octoscope", prs: samplePRs()})
	if strings.Contains(m.View(), "first pr") {
		t.Errorf("a stale answer was shown:\n%s", m.View())
	}
}

// The ref that travels to the detail, diff and checks views names the
// repository the row belongs to, not the one the process started in.
func TestSelectedRefNamesTheSelectedRepository(t *testing.T) {
	f := &fakeSource{prs: samplePRs()}
	m := sidebarModel(f, 120)
	m, _ = m.Update(key("h"))
	m, cmd := m.Update(key("j"))
	drain(t, cmd)
	m, _ = m.Update(prListMsg{repo: "kukv/koto", prs: samplePRs()})
	ref, ok := m.SelectedRef()
	if !ok || ref.Repo != "kukv/koto" {
		t.Errorf("ref = %+v, want kukv/koto", ref)
	}
}

// The lookup that names the working directory answers seconds after the
// model was built, and can put a temporary row above the one already loaded.
// What is on screen belongs to the row that was selected, so it must go.
func TestSetCurrentClearsAndRefetches(t *testing.T) {
	f := &fakeSource{prs: samplePRs()}
	m := sidebarModel(f, 120) // showing kukv/octoscope's pull requests
	f.prRepos = nil
	m, cmd := m.SetCurrent("kukv/elsewhere")
	drain(t, cmd)
	if strings.Contains(m.View(), "first pr") {
		t.Errorf("the previous row's rows survived:\n%s", m.View())
	}
	if len(f.prRepos) != 1 || f.prRepos[0] != "kukv/elsewhere" {
		t.Errorf("ListPRs got %v, want the new row", f.prRepos)
	}
}
```

- [ ] **Step 3: 落ちることを確かめる**

Run: `go test ./internal/tui/repo/ -v`
Expected: FAIL

- [ ] **Step 4: 実装する**

- `fetchList(src, t, repo)` は答えに `repo` を載せて返す
- `Init` / `Refresh` / `showTab` は `m.selectedRepo()` を渡す
- `prListMsg` / `issueListMsg` の受け口は `msg.repo != m.selectedRepo()` なら捨てる
- **クリアと再取得は `selectRow` の中だけに置く**。キーもマウスも、
  `SetCurrent`（一時行が先頭に割り込んで選択が動く）も、そこを通る:

```go
func (m Model) selectRow(i int) (Model, tea.Cmd) {
	if i == m.selected && m.loaded[m.tab] {
		return m, nil
	}
	m.selected = i
	m.prs, m.issues = nil, nil
	m.loaded, m.cursors = [2]bool{}, [2]int{}
	if len(m.rows) == 0 {
		return m, nil
	}
	m.loading[m.tab] = true
	return m, fetchList(m.src, m.tab, m.selectedRepo())
}
```
- `selectedRepo()` は `m.rows[m.selected].name`。行が無ければ空文字
  （`ListPRs` の空文字はクライアントのリポジトリに落ちる — 2-1 の `effectiveRepo`）
- `SelectedRef` の `Repo` と、ヘッダの名前を `selectedRepo()` にする
- `repoNamer` と `fetchRepoName` と `repoNameMsg` と `m.repoName` を
  `internal/tui/repo` から**消す**。`app.resolveRepo` はこれの唯一の残りの
  呼び出し元なので、`app` に自分用の 1 メソッド interface を宣言する:

```go
// repoNamer names the repository of the working directory. It is the root's
// alone: the tabs are told which repository they show.
type repoNamer interface {
	RepoName(ctx context.Context) (string, error)
}
```

`app.Source` から `repo.Source` の `repoNamer` が抜けるので、`app.Source` に
`repoNamer` を足す。

- [ ] **Step 5: 通ることを確かめ、壊して落ちることも確かめる**

Run: `go test ./internal/tui/... -v`
`fetchList` に渡す `m.selectedRepo()` を `""` に戻して
`TestMovingTheSidebarFetchesThatRepository` が落ちること、
`msg.repo != m.selectedRepo()` の破棄を外して
`TestAnAnswerForAnotherRepositoryIsDropped` が落ちることを見てから戻す。

- [ ] **Step 6: `make check` してコミット**

```bash
git add internal/tui
git commit -m "feat: show the repository the sidebar's cursor is on

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: マウス

**Files:**
- Modify: `internal/tui/repo/mouse.go`
- Modify: `internal/tui/repo/mouse_test.go`

**Interfaces:**
- Consumes: `m.sidebarCols()`, `m.rows`, `m.selected`, `m.focus`（Task 3）
- Produces: `func (m Model) sidebarRowAt(y int) (int, bool)`

- [ ] **Step 1: 失敗するテストを書く**

```go
// A click in the sidebar selects that repository and moves the focus there.
func TestClickingASidebarRowSelectsIt(t *testing.T) {
	f := &fakeSource{prs: samplePRs()}
	m := sidebarModel(f, 120)
	m, cmd := m.Update(click(2, sidebarTop+1)) // the second repository
	drain(t, cmd)
	if m.selected != 1 || m.focus != paneSidebar {
		t.Errorf("selected = %d focus = %v, want the clicked row focused", m.selected, m.focus)
	}
}

// The sub-tab row starts at the sidebar's right edge, not at column zero: a
// hit-test that forgot the offset would switch tabs on a sidebar click.
func TestSubTabHitTestIsOffsetByTheSidebar(t *testing.T) {
	f := &fakeSource{prs: samplePRs()}
	m := sidebarModel(f, 120)
	before := m.tab
	m, _ = m.Update(click(1, subTabRow))
	if m.tab != before {
		t.Error("a click inside the sidebar switched the sub-tab")
	}
	m, _ = m.Update(click(m.sidebarCols()+1, subTabRow))
	if m.tab != tabPRs {
		t.Error("a click on the first sub-tab missed it")
	}
}

// The wheel moves whichever pane the pointer is over.
func TestWheelOverTheSidebarMovesTheSidebar(t *testing.T) {
	f := &fakeSource{prs: samplePRs()}
	m := sidebarModel(f, 120)
	m, cmd := m.Update(wheelDown(2, sidebarTop))
	drain(t, cmd)
	if m.selected != 1 {
		t.Errorf("selected = %d, want the wheel to move the sidebar", m.selected)
	}
	if m.cursors[m.tab] != 0 {
		t.Error("the wheel moved the table too")
	}
}
```

`click` / `wheelDown` は `mouse_test.go` の既存のヘルパーに合わせる
（無ければ `tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y}` を返す関数を書く）。

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/tui/repo/ -run 'Click|SubTab|Wheel' -v`
Expected: FAIL

- [ ] **Step 3: 実装する**

- `sidebarTop` を `sidebar()` が使う定数として `sidebar.go` に置き、
  当たり判定はそれを読む（描画と同じ関数を読む — spec §4.0）
- `handleMouseClick`: `m.sidebarCols() > 0 && msg.X < sidebarWidth` なら
  `sidebarRowAt(msg.Y)` で行を選び（**`selectRow` を通す** — Task 5 の
  クリアと再取得はそこにしかない）、focus をサイドバーに移す。
  選択済みの行の再クリックは何もしない（開く先が無い — 右ペインは既にその
  リポジトリを出している）
- `sidebarRowAt(y)` は `y - sidebarTop + m.sidebarWindow()` を返し、
  範囲外なら `ok` が false（サイドバーもスクロールする — Task 3）
- それ以外は `msg.X -= m.sidebarCols()` してから今の判定に入り、focus を
  `paneList` に移す
- `handleMouseWheel`: `msg.X < sidebarWidth` かつサイドバーがあるならサイドバーの
  カーソル（同じく `selectRow` を通すので、再取得は自然に付いてくる）、
  そうでなければ表のカーソル

- [ ] **Step 4: 通ることを確かめ、壊して落ちることも確かめる**

`msg.X -= m.sidebarCols()` を外して `TestSubTabHitTestIsOffsetByTheSidebar` が
落ちることを見てから戻す。

- [ ] **Step 5: `make check` してコミット**

```bash
git add internal/tui/repo
git commit -m "feat: reach the sidebar with the mouse

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 7: 一覧が空のとき

設定ファイルが空でカレントのリポジトリも無いとき、Repos タブは開ける。
**初期投入の導線は 2-3。** ここでは何が起きているかを言うだけである。

**Files:**
- Modify: `internal/tui/repo/render.go`
- Modify: `internal/tui/repo/repo_test.go`, `internal/tui/repo/golden_test.go`
- Create: `internal/tui/repo/testdata/repo_empty_{en,ja}_{80,120,160}.golden`

- [ ] **Step 1: 失敗するテストを書く**

```go
func TestEmptyListSaysSo(t *testing.T) {
	m := sized(New(&fakeSource{}, Options{}), 120)
	view := m.View()
	if !strings.Contains(view, i18n.T("repos.none")) {
		t.Errorf("an empty Repos tab says nothing:\n%s", view)
	}
	if strings.Contains(view, i18n.T("common.loading")) {
		t.Errorf("an empty Repos tab spins forever:\n%s", view)
	}
}
```

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/tui/repo/ -run TestEmptyList -v`
Expected: FAIL（`New` が `loading[tab]` を立てるので、いま出るのはスピナー）

- [ ] **Step 3: 実装する**

`New` は行が無ければ `loading` を立てない。`View` の `body` は行が無ければ
`repos.none` を描く（`Init` も、行が無ければ `fetchList` / `fetchCounts` を
返さない — 引くものが無い）。

- [ ] **Step 4: 通ることを確かめ、golden を録る**

`golden_test.go` に `repo_empty_*` を足す。

Run: `OCTOSCOPE_UPDATE_GOLDEN=1 go test ./internal/tui/repo/ && go test ./internal/tui/repo/`

- [ ] **Step 5: `make check` してコミット**

```bash
git add internal/tui/repo
git commit -m "fix: say when the Repos tab has nothing to list

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 8: 実測と受け渡し

**Files:**
- Modify: `docs/superpowers/specs/2026-09-08-phase4-design.md`（§2 の表）
- Create: `docs/superpowers/2026-09-09-phase4-repos-sidebar-followups.md`

- [ ] **Step 1: 20〜50 件での件数取得を 1 回だけ実測する**

前スライスの積み残しが求めている数字である。`gh auth status` が通るなら:

```bash
gh repo list --limit 30 --json nameWithOwner --jq '.[].nameWithOwner' > "$SCRATCH/repos.txt"
# repo_counts.graphql と同じ選択を 30 リポジトリ分並べたクエリを組み、time で計る
```

**`gh auth status` が通らない環境なら、実測は利用者への受け渡しに回す**
（推測の数字を設計書に書かない — `justify-with-measurements`）。

- [ ] **Step 2: 設計 §2 の表に行を足す**

計った件数・所要時間・録った日を書く。1 リクエストで足りるか、分割が要るかの
判断もそこに 1 行で書く。

- [ ] **Step 3: 積み残しを書く**

`docs/superpowers/2026-09-09-phase4-repos-sidebar-followups.md` に、
前 2 スライスと同じ形で:

- **実端末での確認の依頼**（TTY が無い環境では代行できない）: サイドバー付きの
  Repos タブ、`--lang ja` での 80 桁、`h`/`l` の移動、マウスでのサイドバー選択
- 見つかったが直さなかったこと、およびその理由。**少なくとも 1 つある:
  `RepoCounts` が丸ごと失敗したとき（トークン切れ、レート制限）バッジが
  「—」になるだけで、なぜかが画面に出ない。** 一覧そのものは動くので
  2-2 では受け入れたが、書き残す
- 2-3 に渡すもの: 追加ダイアログ（`a`）、削除（`x`）、初回投入、`config.Save`。
  **一時行は `Save` の対象外**であること

- [ ] **Step 4: コミット**

```bash
git add docs
git commit -m "docs: record what the Repos sidebar slice left behind

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## 完了条件（このスライス）

設計 §11 の 3・4 のうち、**読む側**が満たされる:

1. Repos タブがカレントのリポジトリの有無によらず常に存在する
2. 設定ファイルの一覧がサイドバーに並び、重複した綴りは 1 行に畳まれる
3. カレントのリポジトリが一覧に無いとき先頭に並び、選択済みで始まる
4. 件数バッジが 1 リクエストで引け、引けなかった行は「—」で残る
5. 行を移ると右ペインがその行のリポジトリになり、古い応答は捨てられる
6. 100 桁未満でサイドバーが畳まれ、ja の 80 桁でキーバーが 1 行に収まる
7. `internal/tui` が `internal/gh/cli` も `internal/config` も import していない
8. `make check` が緑

**このスライスに無いもの**: 追加ダイアログ、削除、初回投入の導線、
設定ファイルへの書き戻し（すべて 2-3）。
