# Search タブ本体 実装計画（Phase 4 スライス 3-2）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 3 つ目のタブとして Search を出し、左のフィルタで組んだクエリで GitHub を検索して
右に結果を並べる。生クエリは上段に常時表示し、`e` で直接編集できる。

**Architecture:** `internal/tui/search` を新設する。フィルタの状態からクエリ文字列を組む部分は
純粋関数として切り出し（`query.go`）、画面と分けてテストする。検索は 3-1 で開けた
`SearchItems` を `internal/usecase` 経由で呼ぶ。2 ペインの連結と桁の道具は 3-1 で
`internal/tui/layout` に出した `JoinPanes` / `Clip` / `Pad` / `Right` を使う。

**Tech Stack:** Go / Bubble Tea v2（`charm.land/*/v2`）/ `gh` CLI

**Spec:**
- `docs/superpowers/specs/2026-09-08-phase4-design.md`（§5 Search タブ、§7 境界、§8 テスト、§9 幅）
- `docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md`（§4.3 画面、§4.6 幅の劣化）
- UI モックアップ: https://claude.ai/code/artifact/96b1dad5-ed75-4176-b110-20923b1b565e
  — **採用案は S2「クエリビルダー」。§4.3 についてはモックアップが正。**
  文章だけ読んで実装すると「要素は揃っているがレイアウトが別物」になる（#52 で実際にそうなった）
- 前スライスの積み残し: `docs/superpowers/2026-09-11-phase4-search-foundation-followups.md`

---

## Global Constraints

- Bubble Tea 系の import は `charm.land/*/v2`。`github.com/charmbracelet/bubbletea/v2` は壊れている。`github.com/charmbracelet/x/ansi` は `github.com/` のままが正しい
- 画面に出す文字列は `internal/i18n` から引く。新しい ID は `active.en.yaml` と `active.ja.yaml` の**両方**に足す。**GitHub の検索構文（`is:open`、`label:`、`sort:`）は翻訳しない**
- 桁は `ansi.StringWidth` で数える。`len` も `utf8.RuneCountInString` も使わない
- 色は `internal/tui/theme` からだけ引く。16 進の色をビューに書かない
- ネットワークも外部プロセスも実際には叩かない。`gh` の応答は 3-1 で録った `internal/gh/cli/testdata/search_items.json` がある
- 先に失敗するテストを書く。書いた直後に検証対象を一時的に壊し、**落ちることを目で見てから**コミットする
- コメントは英語。書くのは「外部の事情」「一見おかしいコードが正しい理由」「エクスポートした識別子の doc」の 3 つだけ。**実装計画や設計書への参照（`Task 4`、`spec §5`）をコードに書かない**（`.claude/rules/*.md` への参照は可）
- `internal/tui` は `internal/gh/cli` も `internal/config` も import しない。interface は**利用側**（`internal/tui/search`）で宣言する。1 つの interface 宣言に直接並べるメソッドは 6 個まで
- **`.golangci.yml` の depguard は `**/internal/tui/**` で効くので、`internal/tui/search` を足しても設定変更は要らない**（確認済み）
- `View()` は副作用を持たない。時計を読まない（相対時刻は `Update` で読んだ時刻を使う）
- 各タスクの終わりに `make check` が緑であること
- golden は en / ja × 80 / 120 / 160。`make golden` で録り直し、**diff を目で見てから**コミットする

## このスライスに入れないもの

- **保存クエリ**（`s` 保存 / `Ctrl+O` ポップアップ / `saved_queries` / `default_tab: search`）は 3-3
- **生クエリからフィルタへの逆解析**。設計 §5 が「編集は一方向」と決めている
- **マウス**。Repos タブは `mouse.go` を持つが、Search は持たずに出す（積み残しに書く）
- **ページング**。`first: 50` の上限は Work 板から引き継ぐ（下の前提 5 を見る）

## この計画が判断した前提（着手前に承認を取ること）

1. **`s` は Search タブで未割当にする**（利用者が選択済み）。Repos の `s` は checks だが、
   Search の `s` は 3-3 の保存で使う。結果行から使えるキーは `enter`（詳細）/ `d`（PR の diff）/
   `o`（ブラウザ）の 3 つで、checks は詳細ビュー経由で届く
2. **候補チップは最後のタスク（Task 8）に置く**（利用者が選択済み）。PR が大きくなりすぎたら
   そこだけ切り離して 3-2b にできる形にする
3. **フィルタは 8 項目で、種類は 2 つだけ。**「選ぶ項目」（type / state / review / sort）は
   `space` で値を回し、「打つ項目」（org / repo / author / label）は `enter` で入力欄になる。
   モックアップのキーバーが `space 切替` しか言っていないのは選ぶ項目の話で、
   `org: kukv` が入っている以上、打つ手段は要る
4. **検索はフィルタを確定した時点で 1 回走る**（`space` で値を回している間は走らせない。
   打つ項目は `enter` で確定したとき）。1 回が数秒かかるため
5. **`first: 50` に届いたら件数を `50+` と描く。** 「ちょうど 50 件」と「50 件で切られた」を
   利用者が区別できないのは嘘に近い。ページングはしない（積み残しのまま）
6. **フィルタが全部既定のときのクエリは `is:open`。** 空文字は GitHub に拒まれる。
   `type` の既定は `all`（`is:pr` も `is:issue` も付けない）、`state` の既定は `open`
7. **結果行は `st / rp / num / ttl / ag`**（state / repo / number / title / age）。
   モックアップ S2 のとおりで、Repos の右ペイン（`st / num / ttl / ck / ag`）とは違う。
   checks 列は持たない（ペインが狭い）。`rp` はオーナーを落とした短い名前
8. **`app.handleKey` と `app.handleMouseClick` の「Work か、さもなくば Repos」を
   3 分岐に直す。** 今は `if m.tab == tabWork { work } else { repo }` なので、
   タブを足すと Search のキーとクリックが Repos に届く

---

## ファイル構成

| ファイル | 責務 |
|---|---|
| `internal/tui/search/query.go`（新規） | フィルタの値と、それを GitHub の検索クエリ文字列に組む関数 |
| `internal/tui/search/search.go`（新規） | Model / Update / 取得。フィルタのカーソル、入力欄、生クエリ、結果 |
| `internal/tui/search/render.go`（新規） | 生クエリの行、フィルタペイン、結果表、キーバー |
| `internal/tui/search/golden_test.go`（新規） | en / ja × 80 / 120 / 160 |
| `internal/usecase/search.go`（新規） | `SearchItems` の 1 行の配線 |
| `internal/usecase/usecase.go`（変更） | `crossRepoLister` に `SearchItems` を足す |
| `internal/tui/app/app.go`（変更） | タブ 3、`3` キー、`Capturing()` の委譲、Source に search.Source |
| `internal/tui/app/render.go`（変更） | タブ行のラベルと `activeTab()` |
| `internal/tui/app/mouse.go`（変更） | クリックの振り分けを 3 分岐に |
| `internal/i18n/locales/active.{en,ja}.yaml`（変更） | Search タブの文言 |

---

### Task 1: フィルタからクエリ文字列を組む

画面より先に、組み立てだけを純粋関数で作る。ここが正しければ、あとは描いて配るだけになる。

**Files:**
- Create: `internal/tui/search/query.go`
- Create: `internal/tui/search/query_test.go`

**Interfaces:**
- Produces:
  - `type FilterID int` と定数 `FilterType` / `FilterState` / `FilterOrg` / `FilterRepo` / `FilterAuthor` / `FilterLabel` / `FilterReview` / `FilterSort`（この順で画面に並ぶ）
  - `type Filters struct { ... }` — 8 項目の値を持つ
  - `func (f Filters) Query() string` — GitHub の検索クエリ文字列
  - `func (f Filters) Value(id FilterID) string` — 画面に出す今の値（未設定は空文字）
  - `func (f Filters) Cycle(id FilterID) Filters` — 選ぶ項目の値を 1 つ進める（打つ項目では何もしない）
  - `func (f Filters) Set(id FilterID, v string) Filters` — 打つ項目の値を入れる
  - `func (id FilterID) Choices() []string` — その項目が選ぶ項目なら候補、打つ項目なら nil

- [ ] **Step 1: 失敗するテストを書く**

`internal/tui/search/query_test.go` を新規に作る。外部テストパッケージ（`package search_test`）。

```go
package search_test

import (
	"testing"

	"github.com/kukv/octoscope/internal/tui/search"
)

// GitHub rejects an empty search, so the untouched form still has to mean
// something. It means what the mockup shows selected: everything still open.
func TestTheUntouchedFormSearchesForWhatIsOpen(t *testing.T) {
	t.Parallel()

	var f search.Filters
	if got := f.Query(); got != "is:open" {
		t.Errorf("Query() = %q, want %q", got, "is:open")
	}
}

func TestEachFilterAddsItsOwnQualifier(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		set  func(search.Filters) search.Filters
		want string
	}{
		{
			"type",
			func(f search.Filters) search.Filters { return f.Cycle(search.FilterType) },
			"is:open is:pr",
		},
		{
			"org",
			func(f search.Filters) search.Filters { return f.Set(search.FilterOrg, "kukv") },
			"is:open org:kukv",
		},
		{
			"repo",
			func(f search.Filters) search.Filters { return f.Set(search.FilterRepo, "kukv/octoscope") },
			"is:open repo:kukv/octoscope",
		},
		{
			"author",
			func(f search.Filters) search.Filters { return f.Set(search.FilterAuthor, "kukv") },
			"is:open author:kukv",
		},
		{
			"label",
			func(f search.Filters) search.Filters { return f.Set(search.FilterLabel, "bug") },
			"is:open label:bug",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			if got := c.set(search.Filters{}).Query(); got != c.want {
				t.Errorf("Query() = %q, want %q", got, c.want)
			}
		})
	}
}

// A label with a space in it is one qualifier, not two.
func TestALabelWithASpaceIsQuoted(t *testing.T) {
	t.Parallel()

	f := search.Filters{}.Set(search.FilterLabel, "good first issue")
	if got := f.Query(); got != `is:open label:"good first issue"` {
		t.Errorf("Query() = %q, want the label quoted", got)
	}
}

// state:all means the user asked for both, so neither is:open nor is:closed
// belongs in the query.
func TestStateAllDropsTheStateQualifier(t *testing.T) {
	t.Parallel()

	f := search.Filters{}
	for range 2 { // open -> closed -> all
		f = f.Cycle(search.FilterState)
	}
	if got := f.Query(); got != "" {
		t.Errorf("Query() = %q, want it empty once nothing is being filtered", got)
	}
}

func TestCyclingATypedFilterDoesNothing(t *testing.T) {
	t.Parallel()

	f := search.Filters{}.Set(search.FilterOrg, "kukv")
	if got := f.Cycle(search.FilterOrg); got.Value(search.FilterOrg) != "kukv" {
		t.Errorf("Value() = %q, want cycling to leave a typed filter alone", got.Value(search.FilterOrg))
	}
}

func TestCyclingComesBackAround(t *testing.T) {
	t.Parallel()

	f := search.Filters{}
	first := f.Value(search.FilterState)
	for range len(search.FilterState.Choices()) {
		f = f.Cycle(search.FilterState)
	}
	if f.Value(search.FilterState) != first {
		t.Errorf("Value() = %q after a full turn, want %q", f.Value(search.FilterState), first)
	}
}
```

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/tui/search/`
Expected: FAIL — パッケージが無い

- [ ] **Step 3: `query.go` を書く**

```go
// Package search is the tab that searches GitHub from filters the user
// builds, with the query it built on show above them.
package search

import (
	"strings"
)

// FilterID names one row of the filter pane, in the order they are drawn.
type FilterID int

const (
	FilterType FilterID = iota
	FilterState
	FilterOrg
	FilterRepo
	FilterAuthor
	FilterLabel
	FilterReview
	FilterSort
	filterCount
)

// choices is what a picked filter offers, in the order space walks them. A
// filter with no choices is typed into instead. The first entry is the
// default, and an empty first entry means the filter adds nothing until it
// is moved off it.
var choices = [filterCount][]string{
	FilterType:   {"all", "pr", "issue"},
	FilterState:  {"open", "closed", "all"},
	FilterReview: {"", "none", "required", "approved", "changes-requested"},
	FilterSort:   {"", "updated", "created", "comments"},
}

// Choices is what space walks through for a picked filter, and nil for one
// that is typed into.
func (id FilterID) Choices() []string { return choices[id] }

// Filters is the state of the filter pane: which choice each picked filter
// is on, and what was typed into each of the others.
type Filters struct {
	picked [filterCount]int
	typed  [filterCount]string
}

// Value is what the pane draws for one filter, and what the query is built
// from. It is empty when the filter is adding nothing.
func (f Filters) Value(id FilterID) string {
	if c := choices[id]; c != nil {
		return c[f.picked[id]]
	}
	return f.typed[id]
}

// Cycle moves a picked filter onto its next choice and comes back around at
// the end. A typed filter has nothing to cycle through and is left alone.
func (f Filters) Cycle(id FilterID) Filters {
	c := choices[id]
	if c == nil {
		return f
	}
	f.picked[id] = (f.picked[id] + 1) % len(c)
	return f
}

// Set puts v into a typed filter. A picked one is left alone: its values are
// the only ones GitHub understands for it.
func (f Filters) Set(id FilterID, v string) Filters {
	if choices[id] != nil {
		return f
	}
	f.typed[id] = strings.TrimSpace(v)
	return f
}

// Query is the GitHub search these filters stand for. The qualifiers are not
// translated: they are GitHub's own syntax (.claude/rules/tui.md).
func (f Filters) Query() string {
	var parts []string
	switch f.Value(FilterState) {
	case "open":
		parts = append(parts, "is:open")
	case "closed":
		parts = append(parts, "is:closed")
	}
	switch f.Value(FilterType) {
	case "pr":
		parts = append(parts, "is:pr")
	case "issue":
		parts = append(parts, "is:issue")
	}
	for _, q := range []struct {
		id     FilterID
		prefix string
	}{
		{FilterOrg, "org:"},
		{FilterRepo, "repo:"},
		{FilterAuthor, "author:"},
		{FilterLabel, "label:"},
		{FilterReview, "review:"},
		{FilterSort, "sort:"},
	} {
		if v := f.Value(q.id); v != "" {
			parts = append(parts, q.prefix+quote(v))
		}
	}
	return strings.Join(parts, " ")
}

// quote wraps a value that carries a space, which GitHub would otherwise
// read as the end of the qualifier.
func quote(v string) string {
	if !strings.Contains(v, " ") {
		return v
	}
	return `"` + v + `"`
}
```

- [ ] **Step 4: 通ることを確かめる**

Run: `go test ./internal/tui/search/`
Expected: PASS

- [ ] **Step 5: テストが空振りでないことを確かめる**

`quote` を `return v` に変え、`TestALabelWithASpaceIsQuoted` が落ちることを目で見る。戻す。
`Query()` の `case "open":` の行を消し、
`TestTheUntouchedFormSearchesForWhatIsOpen` が落ちることを目で見る。戻す。

- [ ] **Step 6: `make check`**

Run: `make check`
Expected: 緑

- [ ] **Step 7: Commit**

```bash
git add internal/tui/search
git commit -m "feat: build a GitHub search out of the filters a user picked

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: 検索を配線する

クエリが組めるので、それで GitHub を叩く口をビューに用意する。interface は利用側で宣言し、
`internal/usecase` がそれを満たす。

**Files:**
- Create: `internal/usecase/search.go`
- Modify: `internal/usecase/usecase.go`（`crossRepoLister`）
- Create: `internal/tui/search/search.go`
- Create: `internal/tui/search/search_test.go`
- Test: `internal/usecase/usecase_test.go`

**Interfaces:**
- Consumes: `search.Filters`（Task 1）
- Produces:
  - `type search.Source interface { SearchItems(ctx context.Context, query string) ([]gh.WorkItem, error); OpenWeb(url string) error }`
  - `func search.New(src Source) Model`
  - `func (m Model) Update(msg tea.Msg) (Model, tea.Cmd)` / `func (m Model) Init() tea.Cmd`
  - `func (u *Usecase) SearchItems(ctx context.Context, query string) ([]gh.WorkItem, error)`

- [ ] **Step 1: 失敗するテストを書く**

`internal/tui/search/search_test.go`（内部テストパッケージ `package search`。
非公開のフィールドを読むため）。

```go
package search

import (
	"context"
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kukv/octoscope/internal/gh"
)

type fakeSource struct {
	query string
	items []gh.WorkItem
	err   error
}

func (f *fakeSource) SearchItems(_ context.Context, query string) ([]gh.WorkItem, error) {
	f.query = query
	return f.items, f.err
}

func (f *fakeSource) OpenWeb(string) error { return nil }

// resolve runs a command the model handed back and feeds its message in, the
// way Bubble Tea would.
func resolve(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()

	if cmd == nil {
		t.Fatal("no command to run")
	}
	next, _ := m.Update(cmd())
	return next
}

func TestTheFirstSearchAsksForWhatTheFormMeans(t *testing.T) {
	t.Parallel()

	src := &fakeSource{}
	m := New(src)
	resolve(t, m, m.Init())

	if src.query != "is:open" {
		t.Errorf("searched for %q, want %q", src.query, "is:open")
	}
}

func TestTheResultsArriveOnTheModel(t *testing.T) {
	t.Parallel()

	src := &fakeSource{items: []gh.WorkItem{{Title: "fix the thing"}}}
	m := New(src)
	m = resolve(t, m, m.Init())

	if len(m.items) != 1 || m.items[0].Title != "fix the thing" {
		t.Errorf("items = %v, want the one the source returned", m.items)
	}
	if m.loading {
		t.Error("still loading after the answer arrived")
	}
}

// A query GitHub rejects is the user's to fix, so what it said has to reach
// the screen -- and the tab must keep working.
func TestARejectedQueryIsReportedOnTheTab(t *testing.T) {
	t.Parallel()

	src := &fakeSource{err: errors.New("gh api: Invalid search query")}
	m := New(src)
	m = resolve(t, m, m.Init())

	if m.notice == "" {
		t.Fatal("nothing to show the user about a rejected query")
	}
	if !strings.Contains(m.notice, "Invalid search query") {
		t.Errorf("notice = %q, want what GitHub said", m.notice)
	}
	if m.loading {
		t.Error("still loading after the failure arrived")
	}
}

// gh missing or a signed-out user is not something the tab can carry on
// past: the root shows its own screen for those.
func TestAFatalFailureGoesToTheRoot(t *testing.T) {
	t.Parallel()

	src := &fakeSource{err: gh.ErrGhNotFound}
	m := New(src)
	_, cmd := m.Update(searchDone(m.gen, nil, gh.ErrGhNotFound))
	if cmd == nil {
		t.Fatal("no message went to the root")
	}
	if _, ok := cmd().(FatalMsg); !ok {
		t.Errorf("sent %T, want FatalMsg", cmd())
	}
}

// An answer to a search the user has already typed past must not land.
func TestAnOldAnswerIsDropped(t *testing.T) {
	t.Parallel()

	src := &fakeSource{}
	m := New(src)
	m.gen = 2
	m.items = []gh.WorkItem{{Title: "current"}}
	next, _ := m.Update(itemsMsg{gen: 1, items: []gh.WorkItem{{Title: "stale"}}})
	if next.items[0].Title != "current" {
		t.Errorf("items = %v, want the stale answer dropped", next.items)
	}
}
```

`strings` の import を足す。`searchDone` は実装側のヘルパー名なので、
Step 3 の実装に合わせて書くこと（そこで `itemsMsg` / `errMsg` を組む形にする）。

`internal/usecase/usecase_test.go` には、配線の 1 行を守るテストを足す。

```go
func TestSearchItemsReachesTheGitHubLayer(t *testing.T) {
	t.Parallel()

	f := &fakeGitHub{}
	u := New(f, nil)
	if _, err := u.SearchItems(t.Context(), "is:open is:pr"); err != nil {
		t.Fatalf("SearchItems: %v", err)
	}
	if f.searchQuery != "is:open is:pr" {
		t.Errorf("the query reached the layer as %q", f.searchQuery)
	}
}
```

`fakeGitHub` は既存のフェイクの名前に合わせること（`internal/usecase/usecase_test.go` を読む）。
`New` の引数の形も既存に合わせる。

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/tui/search/ ./internal/usecase/`
Expected: FAIL — `New` も `SearchItems` も無い

- [ ] **Step 3: `search.go` を書く**

```go
package search

import (
	"context"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/kukv/octoscope/internal/gh"
)

// searcher runs one GitHub search. The query is built here and means
// nothing to the layer below, which only sends it.
type searcher interface {
	SearchItems(ctx context.Context, query string) ([]gh.WorkItem, error)
}

// webOpener shows an item in a browser. It takes the URL GitHub gave the
// item rather than a reference to it: building the address by hand would put
// GitHub's URL layout in the UI.
type webOpener interface {
	OpenWeb(url string) error
}

// Source is what the Search tab needs from the GitHub layer.
type Source interface {
	searcher
	webOpener
}

// OpenDetailMsg asks the parent to show the detail view for one result.
type OpenDetailMsg struct{ Ref gh.ItemRef }

// OpenDiffMsg asks the parent to show the diff of the selected pull request.
type OpenDiffMsg struct{ Ref gh.ItemRef }

// FatalMsg carries a failure the parent shows on its error screen. Only what
// the user has to act on travels this way; a rejected query stays here as a
// notice (see gh.IsFatal).
type FatalMsg struct{ Err error }

type itemsMsg struct {
	gen   int
	items []gh.WorkItem
}

type errMsg struct {
	gen int
	err error
}

type Model struct {
	src Source

	width, height int
	spin          spinner.Model

	filters Filters
	cursor  FilterID
	items   []gh.WorkItem
	sel     int

	// gen counts the searches started. An answer that names an older one is
	// dropped, which is what a query the user has since changed means.
	gen     int
	loading bool

	// notice is what GitHub said about a query it would not run. The tab
	// keeps its filters and its last results; the user edits and tries again.
	notice string

	// fetchedAt is when the results arrived. The rows carry relative times
	// and View may not read a clock, so it is read here.
	fetchedAt time.Time
}

func New(src Source) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return Model{src: src, spin: s, loading: true}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spin.Tick, runSearch(m.src, m.filters.Query(), m.gen))
}

// search starts one, counting it so a late answer to the last one can be
// told apart from this one's.
func (m Model) search() (Model, tea.Cmd) {
	m.gen++
	m.loading = true
	m.notice = ""
	return m, runSearch(m.src, m.filters.Query(), m.gen)
}

func runSearch(src searcher, query string, gen int) tea.Cmd {
	return func() tea.Msg { return searchDone(gen, src, query) }
}

// searchDone runs the search and names which of the two messages it turned
// into, so a test can build either without a source.
func searchDone(gen int, src searcher, query string) tea.Msg {
	items, err := src.SearchItems(context.Background(), query)
	if err != nil {
		return errMsg{gen: gen, err: err}
	}
	return itemsMsg{gen: gen, items: items}
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case itemsMsg:
		if msg.gen != m.gen {
			return m, nil
		}
		m.items = msg.items
		m.loading = false
		m.fetchedAt = time.Now()
		if m.sel >= len(m.items) {
			m.sel = max(len(m.items)-1, 0)
		}
		return m, nil
	case errMsg:
		if msg.gen != m.gen {
			return m, nil
		}
		m.loading = false
		if gh.IsFatal(msg.err) {
			err := msg.err
			return m, func() tea.Msg { return FatalMsg{err} }
		}
		m.notice = msg.err.Error()
		return m, nil
	}
	return m, nil
}
```

**`searchDone` のシグネチャはテストと揃えること。** 上のテストは
`searchDone(m.gen, nil, gh.ErrGhNotFound)` の形を仮に書いているので、
実装がこの形でないならテスト側をこの実装に合わせて直す（実装を曲げない）。

`internal/usecase/search.go`:

```go
package usecase

import (
	"context"

	"github.com/kukv/octoscope/internal/gh"
)

// SearchItems runs the Search tab's query.
func (u *Usecase) SearchItems(ctx context.Context, query string) ([]gh.WorkItem, error) {
	return u.cross.SearchItems(ctx, query)
}
```

**フィールド名（`u.cross`）は既存の `internal/usecase/usecase.go` に合わせること。**
`crossRepoLister` に `SearchItems(ctx context.Context, query string) ([]gh.WorkItem, error)` を足す
（これで 3 メソッド、上限の 6 以内）。

- [ ] **Step 4: 通ることを確かめる**

Run: `go test ./internal/tui/search/ ./internal/usecase/`
Expected: PASS

- [ ] **Step 5: テストが空振りでないことを確かめる**

`itemsMsg` の `if msg.gen != m.gen` を消し、`TestAnOldAnswerIsDropped` が落ちることを目で見る。戻す。

- [ ] **Step 6: `make check`**

Run: `make check`
Expected: 緑

- [ ] **Step 7: Commit**

```bash
git add internal/tui/search internal/usecase
git commit -m "feat: run the search the filters describe

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: 画面を描く

モックアップ S2 のとおりに描く。上段に生クエリと件数、左にフィルタ、右に結果、下にキーバー。

**Files:**
- Create: `internal/tui/search/render.go`
- Create: `internal/tui/search/render_test.go`
- Modify: `internal/i18n/locales/active.en.yaml`
- Modify: `internal/i18n/locales/active.ja.yaml`

**Interfaces:**
- Consumes: `Model`（Task 2）、`layout.Clip` / `Pad` / `Right` / `JoinPanes`
- Produces: `func (m Model) View() string`

**モックアップの寸法**（そのまま使う）:

- 生クエリの行: `q` のプロンプト、クエリ本体（淡色）、右端に件数（`7 件` / `7 results`）
- フィルタペイン: 見出し `Filters`、項目名は 10 桁、値がその右。選択中の行は反転
- 結果ペイン: 見出し `Results` と件数、行は `st(2) / rp(16) / num(6) / ttl(残り) / ag(7)`
- キーバー: `j/k 項目` `space 切替` `l 結果へ` `e 生クエリ編集` `s 保存`
  — **`s 保存` は 3-3 で足す。このスライスのキーバーには出さない**

**新しい文言**（`active.en.yaml` と `active.ja.yaml` の両方に足す。ID は既存の並びに合わせる）:

| ID | en | ja |
|---|---|---|
| `tab.search` | `Search` | `Search` |
| `search.filters` | `Filters` | `フィルタ` |
| `search.results` | `Results` | `結果` |
| `search.no_results` | `Nothing matched this search` | `一致するものがありません` |
| `search.truncated` | `50+` | `50+` |
| `search.filter.type` | `type` | `type` |
| `search.filter.state` | `state` | `state` |
| `search.filter.org` | `org` | `org` |
| `search.filter.repo` | `repo` | `repo` |
| `search.filter.author` | `author` | `author` |
| `search.filter.label` | `label` | `label` |
| `search.filter.review` | `review` | `review` |
| `search.filter.sort` | `sort` | `sort` |
| `search.unset` | `—` | `—` |
| `footer.search.field` | `j/k: field` | `j/k:項目` |
| `footer.search.cycle` | `space: change` | `space:切替` |
| `footer.search.edit_field` | `enter: type` | `enter:入力` |
| `footer.search.results` | `l: results` | `l:結果へ` |
| `footer.search.raw` | `e: raw query` | `e:生クエリ` |
| `footer.search.open` | `enter: detail` | `enter:詳細` |
| `footer.search.quit` | `q: quit` | `q:終了` |

**フィルタ名（`type` / `state` / ...）は両言語で同じ。** GitHub の検索構文そのものだからである
（`.claude/rules/tui.md` の「翻訳しないもの」）。

- [ ] **Step 1: 失敗するテストを書く**

`internal/tui/search/render_test.go`（内部テストパッケージ）。

```go
package search

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/gh"
)

// sized is a model with results in it, at the width the test cares about.
func sized(t *testing.T, width int, items []gh.WorkItem) Model {
	t.Helper()

	m := New(&fakeSource{items: items})
	next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 40})
	m = next
	next, _ = m.Update(itemsMsg{gen: m.gen, items: items})
	return next
}

func TestTheQueryIsOnShowAboveTheFilters(t *testing.T) {
	t.Parallel()

	m := sized(t, 120, nil)
	if !strings.Contains(m.View(), "is:open") {
		t.Errorf("the query the filters mean is not on screen:\n%s", m.View())
	}
}

func TestAResultRowShowsItsRepositoryAndNumber(t *testing.T) {
	t.Parallel()

	m := sized(t, 120, []gh.WorkItem{{
		Ref:   gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/octoscope", Number: 41},
		Title: "chore(deps): golangci-lint",
	}})
	view := m.View()
	// The repository column carries the name without its owner: every row
	// can come from a different one, and the owner is usually the same.
	for _, want := range []string{"octoscope", "#41", "chore(deps)"} {
		if !strings.Contains(view, want) {
			t.Errorf("the row does not carry %q:\n%s", want, view)
		}
	}
}

func TestEveryLineFitsTheTerminal(t *testing.T) {
	t.Parallel()

	items := make([]gh.WorkItem, 60)
	for i := range items {
		items[i] = gh.WorkItem{
			Ref:   gh.ItemRef{Repo: "kukv/a-repository-with-a-long-name", Number: i},
			Title: strings.Repeat("long title ", 20),
		}
	}
	for _, w := range []int{80, 120, 160} {
		m := sized(t, w, items)
		for i, line := range strings.Split(m.View(), "\n") {
			if got := ansi.StringWidth(line); got > w {
				t.Errorf("width %d: line %d is %d columns:\n%s", w, i, got, line)
			}
		}
	}
}

// The terminal is 40 rows; the view must not draw more than that.
func TestTheViewFitsTheHeight(t *testing.T) {
	t.Parallel()

	items := make([]gh.WorkItem, 60)
	m := sized(t, 120, items)
	if got := len(strings.Split(m.View(), "\n")); got > 40 {
		t.Errorf("the view is %d rows, want at most 40", got)
	}
}

// Fifty is the cap the search asks for, so a full page may have been cut
// short. Saying "50" would claim there are exactly fifty.
func TestAFullPageSaysItMayHaveBeenCutShort(t *testing.T) {
	t.Parallel()

	m := sized(t, 120, make([]gh.WorkItem, 50))
	if !strings.Contains(m.View(), "50+") {
		t.Errorf("a full page does not say it may be cut short:\n%s", m.View())
	}
}

// Under a hundred columns the filter pane folds away and the query and the
// results take the whole width.
func TestTheFilterPaneFoldsAwayWhenNarrow(t *testing.T) {
	t.Parallel()

	wide := sized(t, 120, nil).View()
	narrow := sized(t, 80, nil).View()
	if !strings.Contains(wide, "│") {
		t.Error("the wide view has no rule between the panes")
	}
	if strings.Contains(narrow, "│") {
		t.Errorf("the narrow view still draws two panes:\n%s", narrow)
	}
	if !strings.Contains(narrow, "is:open") {
		t.Errorf("the narrow view lost the query:\n%s", narrow)
	}
}
```

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/tui/search/ -run 'TestThe|TestA|TestEvery'`
Expected: FAIL — `View` が無い

- [ ] **Step 3: `render.go` を書く**

モックアップの寸法で組む。骨は Repos の `render.go` と同じ形にする
（`m.width` ではなく本文の幅を各列が読む、`layout.Pad` / `layout.Right` で桁を作る、
`layout.JoinPanes` で 2 ペインにする、`layout.FitKeyBar` でキーバーを畳む）。

```go
package search

import (
	"strconv"
	"strings"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/i18n"
	"github.com/kukv/octoscope/internal/tui/icon"
	"github.com/kukv/octoscope/internal/tui/layout"
	"github.com/kukv/octoscope/internal/tui/theme"
)

// The panes and the result table's columns, in display columns. The mockup
// puts the filter names in a ten-column field and the results beside them.
const (
	filterPaneWidth = 30
	paneRule        = 1
	filterNameWidth = 10

	// minPaneWidth is where the filter pane folds away (spec section 4.6).
	minPaneWidth = 100

	stateColumn  = 2
	repoColumn   = 16
	numberColumn = 6
	ageColumn    = 7

	// queryRowHeight is the query line and the blank line under it;
	// footerHeight is the blank line and the key bar.
	queryRowHeight = 2
	footerHeight   = 2

	// searchCap is what one search asks GitHub for. A page of exactly this
	// many may have been cut short.
	searchCap = 50
)
```

以下は書き下ろし。**次を守ること。**

- `View()` は `queryRow()` + 本体 + 通知行 + キーバー
- 本体は `layout.JoinPanes(m.filterPane(), m.resultPane(), filterPaneWidth)`。
  `m.paneCols() == 0`（幅 100 未満）のときは結果だけ
- 結果の行数は `m.height - queryRowHeight - footerHeight`（通知行があればもう 1 行引く）から求める。
  **Repos の `visibleRows()` の定数をそのまま写さない**。Search にはサブタブも要約ブロックも無い
- 件数は `len(m.items)`。`searchCap` に達していたら `i18n.T("search.truncated")`
- 結果が空で読み込み中でないときは `search.no_results`
- 状態のアイコンは `internal/tui/icon` と `internal/tui/theme` から引く。
  PR は `icon.Review(...)`、Issue は `icon.Issue()`（Repos の `row()` を見る）
- リポジトリ列はオーナーを落とす（`gh.SplitRepo` の 2 つ目）

- [ ] **Step 4: 通ることを確かめる**

Run: `go test ./internal/tui/search/`
Expected: PASS

- [ ] **Step 5: 目で見る**

`go test ./internal/tui/search/ -run TestTheQueryIsOnShow -v` の出力ではなく、
テストの中で `fmt.Println(m.View())` を一時的に足して**画面を目で見る**
（`.claude/rules/tui.md` と `tui-never-ship-without-rendering`）。
桁がずれていないこと、罫線が縦に通っていることを確かめてから消す。

- [ ] **Step 6: テストが空振りでないことを確かめる**

結果の行数計算から `- footerHeight` を消し、`TestTheViewFitsTheHeight` が落ちることを目で見る。戻す。

- [ ] **Step 7: `make check`**

Run: `make check`
Expected: 緑

- [ ] **Step 8: Commit**

```bash
git add internal/tui/search internal/i18n
git commit -m "feat: draw the query, the filters and what they found

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: キーを効かせる

フィルタを動かし、値を変え、打ち、結果に移り、生クエリを編集する。

**Files:**
- Modify: `internal/tui/search/search.go`
- Modify: `internal/tui/search/render.go`（入力中の描き分け）
- Test: `internal/tui/search/search_test.go`

**Interfaces:**
- Consumes: `Filters.Cycle` / `Set`（Task 1）、`Model`（Task 2）
- Produces:
  - `func (m Model) Capturing() bool` — 入力欄が開いている間は全キーがこのタブのもの
  - `OpenDetailMsg` / `OpenDiffMsg` を返すキー

**キー割り当て**（モックアップのキーバーと前提 1）:

| キー | フィルタペインで | 結果ペインで |
|---|---|---|
| `j` / `k` | 項目を上下 | 結果を上下 |
| `space` | 選ぶ項目の値を次へ（**検索は走らない**） | — |
| `enter` | 打つ項目なら入力欄を開く。選ぶ項目なら確定して検索 | 詳細を開く |
| `l` / `right` | 結果ペインへ | — |
| `h` / `left` | — | フィルタペインへ |
| `e` | 生クエリの編集を開く | 同じ |
| `r` | 今のクエリでもう一度検索 | 同じ |
| `d` | — | PR なら diff |
| `o` | — | ブラウザで開く |
| `s` | **未割当**（3-3 の保存で使う） | 未割当 |

入力欄（打つ項目・生クエリ）の中では `enter` で確定、`esc` で取り消し。
確定したときだけ検索が走る。

- [ ] **Step 1: 失敗するテストを書く**

```go
// press sends one key the way the terminal would.
func press(m Model, key string) (Model, tea.Cmd) {
	return m.Update(tea.KeyPressMsg{Code: []rune(key)[0], Text: key})
}
```

**`tea.KeyPressMsg` の組み方は既存のテストに合わせること**
（`internal/tui/repo/repo_test.go` と `internal/tui/app/scenario_test.go` にある形を写す。
`space` や `enter` のような名前のあるキーは `Code` が違う）。

```go
func TestSpaceChangesAFilterWithoutSearching(t *testing.T) {
	t.Parallel()

	src := &fakeSource{}
	m := New(src)
	m = resolve(t, m, m.Init()) // the first search
	src.query = ""

	m, cmd := press(m, " ")
	if m.filters.Value(FilterType) == "all" {
		t.Error("space did not move the filter")
	}
	if cmd != nil {
		t.Error("space started a search; the user is still choosing")
	}
}

func TestEnterOnAFilterRunsTheSearchItBuilt(t *testing.T) {
	t.Parallel()

	src := &fakeSource{}
	m := New(src)
	m = resolve(t, m, m.Init())

	m, _ = press(m, " ") // type: all -> pr
	m, cmd := press(m, "enter")
	m = resolve(t, m, cmd)

	if src.query != "is:open is:pr" {
		t.Errorf("searched for %q, want the filters' query", src.query)
	}
}

// A typed filter takes a value the way the add dialog does: the field opens,
// what is typed lands in it, and enter commits.
func TestTypingIntoAFilterReachesTheQuery(t *testing.T) {
	t.Parallel()

	src := &fakeSource{}
	m := New(src)
	m = resolve(t, m, m.Init())

	m, _ = press(m, "j") // type -> state
	m, _ = press(m, "j") // state -> org
	m, _ = press(m, "enter")
	if !m.Capturing() {
		t.Fatal("the field is not open, so the root would act on q and 1")
	}
	for _, r := range "kukv" {
		m, _ = press(m, string(r))
	}
	m, cmd := press(m, "enter")
	m = resolve(t, m, cmd)

	if src.query != "is:open org:kukv" {
		t.Errorf("searched for %q, want the typed org in it", src.query)
	}
	if m.Capturing() {
		t.Error("the field is still open after enter")
	}
}

// The keys the root acts on before the tabs see them must reach the field.
func TestTypingAQuitKeyIntoAFieldTypesIt(t *testing.T) {
	t.Parallel()

	m := New(&fakeSource{})
	m, _ = press(m, "e") // the raw query editor
	for _, r := range "q1q23" {
		m, _ = press(m, string(r))
	}
	if !strings.Contains(m.View(), "q1q23") {
		t.Errorf("the typed query is not on screen:\n%s", m.View())
	}
}

// e edits the query the filters built; the filters are not parsed back out
// of what the user writes (spec section 4.3).
func TestTheEditedQueryIsWhatIsSearchedFor(t *testing.T) {
	t.Parallel()

	src := &fakeSource{}
	m := New(src)
	m = resolve(t, m, m.Init())

	m, _ = press(m, "e")
	for _, r := range " draft:false" {
		m, _ = press(m, string(r))
	}
	m, cmd := press(m, "enter")
	m = resolve(t, m, cmd)

	if src.query != "is:open draft:false" {
		t.Errorf("searched for %q, want what the user edited", src.query)
	}
}

func TestEnterOnAResultOpensIt(t *testing.T) {
	t.Parallel()

	m := sized(t, 120, []gh.WorkItem{{
		Ref: gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/octoscope", Number: 41},
	}})
	m, _ = press(m, "l")
	_, cmd := press(m, "enter")
	if cmd == nil {
		t.Fatal("enter on a result did nothing")
	}
	msg, ok := cmd().(OpenDetailMsg)
	if !ok {
		t.Fatalf("sent %T, want OpenDetailMsg", cmd())
	}
	if msg.Ref.Number != 41 {
		t.Errorf("opened #%d, want #41", msg.Ref.Number)
	}
}

// s belongs to saving a query, which this slice does not have yet. Binding
// it to anything else now would have to be taken back.
func TestSDoesNothingYet(t *testing.T) {
	t.Parallel()

	m := sized(t, 120, []gh.WorkItem{{Ref: gh.ItemRef{Repo: "kukv/octoscope", Number: 1}}})
	m, _ = press(m, "l")
	before := m.View()
	m, cmd := press(m, "s")
	if cmd != nil || m.View() != before {
		t.Error("s did something; it is reserved for saving a query")
	}
}
```

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/tui/search/`
Expected: FAIL

- [ ] **Step 3: キーを実装する**

`internal/tui/search/search.go` に足す。

- `pane`（`paneFilters` / `paneResults`）と `mode`（`modeBrowse` / `modeField` / `modeRaw`）を
  それぞれ enum で持つ。**並行する bool にしない**（`.claude/rules/tui.md`）
- 入力欄は `charm.land/bubbles/v2/textinput`。`dialog` と同じ使い方
- `Capturing()` は `m.mode != modeBrowse`
- 生クエリを編集したら `m.raw` に入れ、以後の検索はそれを使う。**フィルタを触ったら
  組み立て直しに戻る**（`m.raw = ""`）。編集中はフィルタ側を `theme.Dim()` で淡く描く（設計 §5）
- 幅 100 未満（`paneCols() == 0`）では `h` / `l` を効かせない。フィルタペインが画面に無いのに
  カーソルがそこへ行くと、何も起きないキーになる

- [ ] **Step 4: 通ることを確かめる**

Run: `go test ./internal/tui/search/`
Expected: PASS

- [ ] **Step 5: テストが空振りでないことを確かめる**

`Capturing()` を `return false` に変え、`TestTypingAQuitKeyIntoAFieldTypesIt` ではなく
`TestTypingIntoAFilterReachesTheQuery` が落ちることを目で見る
（`Capturing()` は app を通らないと効かないので、このタスクのテストで落ちるのは後者）。戻す。

- [ ] **Step 6: `make check`**

Run: `make check`
Expected: 緑

- [ ] **Step 7: Commit**

```bash
git add internal/tui/search
git commit -m "feat: build, edit and run a search from the keyboard

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: 3 つ目のタブとして出す

**Files:**
- Modify: `internal/tui/app/app.go`
- Modify: `internal/tui/app/render.go`
- Modify: `internal/tui/app/mouse.go`
- Test: `internal/tui/app/app_test.go`
- Test: `internal/tui/app/scenario_test.go`

**Interfaces:**
- Consumes: `search.New` / `Model` / `Capturing` / `OpenDetailMsg` / `OpenDiffMsg` / `FatalMsg`
- Produces: タブ `tabSearch`、キー `3`

**`app` 側で直すところ**（前提 8）:

- `Source` に `search.Source` を足す
- `tabID` に `tabSearch` を足し、`tabLabels()` に `"3 " + i18n.T("tab.search")` を足す
- `activeTab()` を `switch m.tab` にする
- `handleKey` の `case "3":` と、`m.tab == tabRepos && m.repo.Capturing()` の判定を
  **タブごとの `capturing()` に畳む**
- `handleKey` 末尾と `handleMouseClick` の `if m.tab == tabWork { ... } else { ... }` を
  **3 分岐にする**。今のままだと Search のキーとクリックが Repos に届く
- `broadcast` に `m.search` を足す（画面を離れても答えが着く）
- `search.OpenDetailMsg` / `search.OpenDiffMsg` / `search.FatalMsg` を `Update` で受ける
- `resize` と最初の取得（`started`）に `m.search` を入れる

- [ ] **Step 1: 失敗するテストを書く**

`internal/tui/app/app_test.go` に足す。

```go
func TestThreeShowsTheSearchTab(t *testing.T) {
	t.Parallel()

	m := started(t, 120)
	m = press(m, "3")
	if !strings.Contains(m.View().Content, i18n.T("search.filters")) {
		t.Errorf("3 did not reach the Search tab:\n%s", m.View().Content)
	}
}

// The root acts on q, 1, 2 and 3 before the tabs see them. A query with one
// of those in it must still reach the field.
func TestTypingTheTabKeysIntoTheSearchFieldTypesThem(t *testing.T) {
	t.Parallel()

	m := started(t, 120)
	m = press(m, "3")
	m = press(m, "e")
	for _, key := range []string{"q", "1", "2", "3"} {
		m = press(m, key)
	}
	if !strings.Contains(m.View().Content, "q123") {
		t.Errorf("the tab keys did not reach the field:\n%s", m.View().Content)
	}
}

func TestASearchResultOpensTheDetailView(t *testing.T) {
	t.Parallel()

	m := started(t, 120)
	m = press(m, "3")
	m = press(m, "l")
	m = press(m, "enter")
	if len(m.stack) == 0 {
		t.Error("enter on a result opened nothing")
	}
}
```

`started` / `press` は既存のヘルパー（`app_test.go` / `scenario_test.go`）に合わせること。
`fakeSource` に `SearchItems` を足す（結果を 1 件返す）。

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/tui/app/`
Expected: FAIL

- [ ] **Step 3: 配線する**

上の「`app` 側で直すところ」を順に。

- [ ] **Step 4: 通ることを確かめる**

Run: `go test ./internal/tui/app/`
Expected: PASS（golden は次のタスクで録り直すので、ここで落ちてよいのは golden だけ）

- [ ] **Step 5: テストが空振りでないことを確かめる**

`capturing()` の Search の分岐を落とし、
`TestTypingTheTabKeysIntoTheSearchFieldTypesThem` が落ちることを目で見る。戻す。

- [ ] **Step 6: Commit**

```bash
git add internal/tui/app
git commit -m "feat: reach the Search tab with 3

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: golden を録る

**Files:**
- Create: `internal/tui/search/golden_test.go`
- Create: `internal/tui/search/testdata/*.golden`
- Modify: `internal/tui/app/testdata/*.golden`（タブ行に Search が増える）

- [ ] **Step 1: golden テストを書く**

`internal/tui/repo/golden_test.go` を写して、Search 用に直す。録るのは:

- `search_<lang>_<width>` — フィルタと結果が入った画面
- `search_editing_<lang>_<width>` — 生クエリを編集中（フィルタが淡い）
- `search_empty_<lang>_<width>` — 一致なし

en / ja × 80 / 120 / 160 で 18 枚。

- [ ] **Step 2: 録る**

Run: `make golden`

- [ ] **Step 3: 目で見る**

```bash
cat -v internal/tui/search/testdata/search_ja_80.golden
cat -v internal/tui/search/testdata/search_ja_120.golden
cat -v internal/tui/app/testdata/app_tabs_ja_80.golden
```

**確かめること:**
- ja の 80 桁でキーバーが収まっていること（`FitKeyBar` は末尾から落とす）
- フィルタペインが 80 桁で畳まれていること
- 罫線が縦に通っていること、結果行の桁が揃っていること
- タブ行に `3 Search` が出ていること

- [ ] **Step 4: `make check`**

Run: `make check`
Expected: 緑

- [ ] **Step 5: Commit**

```bash
git add internal/tui/search internal/tui/app
git commit -m "test: record the Search tab at three widths in both languages

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 7: 積み残しを書く

**Files:**
- Create: `docs/superpowers/2026-09-12-phase4-search-tab-followups.md`

前例（`docs/superpowers/2026-09-11-phase4-repos-dialog-followups.md`）に合わせる。
**実端末での確認の依頼**を必ず入れる（このスライスは画面を作っているため）。最低限:

- Search タブ（120 桁以上と `--lang ja` の 80 桁）。フィルタ・生クエリ・結果・キーバーが読めること
- `space` で値が変わり、`enter` で検索が走ること。**`space` の連打で `gh` が何本も起きないこと**
- 生クエリに `q` や `3` を含む語を打っても octoscope が終了・タブ移動しないこと
- 一致なしのときの見え方、GitHub にクエリを拒まれたときの通知行
- 80 桁でフィルタペインが畳まれ、120 桁で戻ること

直さなかったこととして最低限:

- **マウスを持たない**（Repos は持つ）。理由を書く
- **`first: 50` のページングが無い**。`50+` と描くだけ
- **`s` は未割当**。3-3 の保存で使う
- **候補チップ**（Task 8 を切った場合）

- [ ] **Step 1: 書いてコミット**

```bash
git add docs/superpowers/2026-09-12-phase4-search-tab-followups.md
git commit -m "docs: record what the Search tab left behind

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 8: label と author の候補チップ（切り離し可能）

**このタスクは単独で切り離せる。** ここまでで Search タブは動く。PR が大きいと判断したら、
このタスクを 3-2b として別の PR に回してよい（そのときは Task 7 の積み残しに書く）。

設計 §5: 候補は既存の `ListLabels` / `ListAssignees` を使う。
**`repo:` が定まっていないときは候補を出さない** — GitHub にリポジトリ横断のラベル一覧は無い。

**Files:**
- Modify: `internal/tui/search/search.go`（`Source` に 2 メソッド、取得と保持）
- Modify: `internal/tui/search/render.go`（チップの行）
- Test: `internal/tui/search/search_test.go`
- Modify: `internal/i18n/locales/active.{en,ja}.yaml`（`search.candidates`）

**Interfaces:**
- Produces: `Source` に `ListLabels(ctx, repo) ([]gh.Label, error)` と
  `ListAssignees(ctx, repo) ([]string, error)` が増える（`Source` は 4 メソッドで上限内）

- [ ] **Step 1: 失敗するテストを書く**

```go
// GitHub has no cross-repository list of labels, so there is nothing to
// offer until the search names one repository.
func TestNoCandidatesUntilARepositoryIsNamed(t *testing.T) {
	t.Parallel()

	src := &fakeSource{labels: []gh.Label{{Name: "bug"}}}
	m := New(src)
	m = resolve(t, m, m.Init())
	m = onFilter(t, m, FilterLabel)

	if strings.Contains(m.View(), "bug") {
		t.Errorf("candidates were offered with no repo: in the query:\n%s", m.View())
	}
	if src.labelRepo != "" {
		t.Errorf("asked for the labels of %q", src.labelRepo)
	}
}

func TestTheLabelsOfTheNamedRepositoryAreOffered(t *testing.T) {
	t.Parallel()

	src := &fakeSource{labels: []gh.Label{{Name: "bug"}, {Name: "enhancement"}}}
	m := New(src)
	m = resolve(t, m, m.Init())
	m = withRepo(t, m, "kukv/octoscope")
	m = onFilter(t, m, FilterLabel)
	m = resolveCandidates(t, m)

	if !strings.Contains(m.View(), "bug") {
		t.Errorf("the repository's labels are not offered:\n%s", m.View())
	}
}
```

`onFilter` / `withRepo` / `resolveCandidates` はこのテストのためのヘルパーとして書く
（**キーを順に送って到達させる。フィールドを直接組み立てない** — `tests-that-cannot-fail`）。

- [ ] **Step 2〜5**: 落ちることを確かめる → 実装 → 通ることを確かめる → 変異で確かめる

実装で守ること:

- 候補はカーソルが `FilterLabel` / `FilterAuthor` にあるときだけ引く。
  **1 打鍵ごとに引かない**（`repo:` が変わったときに 1 回）
- 引いた結果はリポジトリ名とともに覚え、同じリポジトリでは引き直さない
- 失敗したら**黙って候補を出さない**。候補が出ないことは検索の妨げにならないので、
  通知行を占領しない

- [ ] **Step 6: `make golden`** — チップの行が増えるので録り直し、目で見る

- [ ] **Step 7: `make check` → Commit**

```bash
git add internal/tui/search internal/i18n
git commit -m "feat: offer the labels and authors of the repository in the query

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## 完了条件

1. `3` で Search タブが出て、フィルタで組んだクエリで検索でき、結果が並ぶ
2. 生クエリが上段に常時出ており、`e` で編集できる。編集した内容はフィルタに戻らない
3. `space` では検索が走らず、確定した時点で 1 回走る
4. GitHub が拒んだクエリは通知行に出る。タブは動き続ける
5. 結果から `enter` で詳細、`d` で diff、`o` でブラウザが開く。**`s` は何もしない**
6. 幅 100 未満でフィルタペインが畳まれ、ja の 80 桁でキーバーが収まる
7. `internal/tui/search` が `internal/gh/cli` も `internal/config` も import していない
8. golden が en / ja × 80 / 120 / 160 で録れている
9. `make check` が緑
