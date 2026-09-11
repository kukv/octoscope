# Search タブの土台 実装計画（Phase 4 スライス 3-1）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Search タブが載る土台を作る。任意の検索クエリで `search(type: ISSUE)` を
1 発叩く口を `internal/gh/cli` に作り、結果が open / closed / merged のどれかを
ドメイン型に持たせ、Repos の右ペインが持っている桁の道具を
`internal/tui/layout` に出して 2 つ目のタブから使えるようにする。

**Architecture:** 検索は Work 板と同じ `work.graphql` をそのまま使う。列の検索文字列が
固定テキストか利用者のクエリかの違いしか無いので、**文書は 1 つのまま**にして
`schema_test.go` の検証を 1 組に保つ（設計 §6 と同じ理由）。描画の共通化は
行の描画関数ごとではなく、桁を数える道具（`Clip` / `Pad` / `Right` / `JoinPanes`）
だけを `internal/tui/layout` に出す。このスライスは画面を 1 桁も変えない
（golden が変わらないことが検証になる）。

**Tech Stack:** Go / Bubble Tea v2（`charm.land/*/v2`）/ `gh` CLI

**Spec:**
- `docs/superpowers/specs/2026-09-08-phase4-design.md`（§5 Search タブ、§7 境界、§8 テスト、§10 割り方）
- `docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md`（§4.3 画面）
- UI モックアップ: https://claude.ai/code/artifact/96b1dad5-ed75-4176-b110-20923b1b565e
  — **§4.3 についてはモックアップ（採用案 S2）が正。** 文章だけ読んで実装すると
  「要素は揃っているがレイアウトが別物」になる（#52 で実際にそうなった）
- 前スライスの積み残し: `docs/superpowers/2026-09-11-phase4-repos-dialog-followups.md`

---

## Global Constraints

- Bubble Tea 系の import は `charm.land/*/v2`。`github.com/charmbracelet/bubbletea/v2` は壊れている。`github.com/charmbracelet/x/ansi` は `github.com/` のままが正しい
- 画面に出す文字列は `internal/i18n` から引く。新しい ID は `active.en.yaml` と `active.ja.yaml` の**両方**に足す
- 桁は `ansi.StringWidth` で数える。`len` も `utf8.RuneCountInString` も使わない
- ネットワークも外部プロセスも実際には叩かない。`gh` の応答は実物を録って `testdata` に置き、録り方を `testdata/README.md` に残す。**録る対象は公開リポジトリだけ**
- 先に失敗するテストを書く。書いた直後に検証対象を一時的に壊し、**落ちることを目で見てから**コミットする
- コメントは英語。書くのは「外部の事情」「一見おかしいコードが正しい理由」「エクスポートした識別子の doc」の 3 つだけ。**実装計画や設計書への参照（`Task 3`、`spec §5`）をコードに書かない**（`.claude/rules/*.md` への参照は可）
- `internal/tui` は `internal/gh/cli` と `internal/config` を import しない。`internal/usecase` は `internal/tui` と `internal/i18n` を import しない。**パッケージを増やしたら `.golangci.yml` の depguard にその場で足す**（このスライスは増やさない）
- 1 つの interface 宣言に直接並べるメソッドは 6 個まで（embed は数えない）
- 各タスクの終わりに `make check` が緑であること

## このスライスに入れないもの

順番を間違えると二重実装になるので、明示しておく。

- **フィルタからクエリを組み立てる関数**。状態を持つのは `internal/tui/search` で、
  その形が決まるのは 3-2 である。呼び出し元の無いまま先に書かない
- **`saved_queries` の読み書き**。書き込む呼び出し元（`s`）が生まれるのは 3-3。
  スライス 2 で「書き込みは書き込む呼び出し元が生まれるスライスに置く」と決めた形に従う
- **`internal/usecase` への配線**。interface は利用側（`internal/tui/search`）で
  宣言するので、宣言が生まれる 3-2 で一緒に足す
- **画面の変更**。このスライスで golden は 1 行も変わらない

## この計画が判断した前提（着手前に承認を取ること）

設計書にもモックアップにも答えが無く、この計画が決めたもの。**違うと思ったらここで止める。**

1. **設計 §5 の「`internal/tui/repo` の表の部分を切り出して両方から使う」は、
   行の描画関数ではなく桁の道具だけを切り出す形にする。** モックアップの S2 は
   結果行に**リポジトリ名の列を足し、checks の列を持たない**（`trow` の中身が
   `st / rp / num / ttl / ag`。Repos の右ペインは `st / num / ttl / ck / ag`）。
   共通化できるのは `clip` / `pad` / `right` と 2 ペインの連結であって、行そのものではない。
   行まで共通化すると「どちらの列構成も描ける関数」になり、引数で分岐するだけの
   抽象が 1 つ増える
2. **`gh.WorkItem` に `State` を足すのはこのスライスに置く。** Search には
   `state` フィルタがあり、結果に closed / merged が混ざる。`.graphql` 文書・
   `schema_test.go`・録りものは一度に触るのが安全で、3-2 に持ち越すと
   「画面を作る PR で文書と fixture も録り直す」ことになる。
   **Work 板の見た目は変えない**（Work は常に `is:open` なので出番が無い）
3. **検索は部分成功を救わない。** `RepoCounts` は alias ごとに独立した結果を持つので
   半分でも意味があるが、検索は結果集合が 1 つしか無い。半分の結果を黙って出すより、
   GitHub が言ったことをそのまま呼び出し元に返すほうが直せる。
   失敗は `gh.IsFatal` が偽であること（= エラー画面ではなくタブの通知行に出る）を
   テストで固定する
4. **`first: 50` の上限とページングの無さを Work 板から引き継ぐ。** 設計 §1 は
   Work 板の切り詰めを「別の機会」に送っており、Search も同じ文書を使う以上同じ上限になる。
   ここで直さないことを積み残しに書く
5. **`dialog` の `padTo` / `rightTo` は `layout` のものに置き換えて消す。** 前スライスの
   積み残しが「2 人目の利用者が付いた時点で決める」と書いた件で、Search が 2 人目である
6. **`layout` が `internal/tui/theme` を import する。** `JoinPanes` はペインの間に
   `theme.Rule()` で色をつけた罫線を引くため。`theme` は `layout` を import していないので
   循環しない

---

## ファイル構成

| ファイル | 責務 |
|---|---|
| `internal/tui/layout/columns.go`（新規） | `Clip` / `Pad` / `Right` — 桁を数えて 1 つの欄に収める |
| `internal/tui/layout/panes.go`（新規） | `JoinPanes` — サイドバーと本体を横に並べる |
| `internal/tui/repo/render.go`（変更） | `clip` / `pad` / `right` を消して `layout` を呼ぶ |
| `internal/tui/repo/sidebar.go`（変更） | `joinPanes` / `padExact` を消して `layout` を呼ぶ |
| `internal/tui/dialog/render.go`（変更） | `padTo` / `rightTo` を消して `layout` を呼ぶ |
| `internal/gh/gh.go`（変更） | `WorkItem.State` |
| `internal/gh/cli/work.graphql`（変更） | `state` を PullRequest と Issue に足す |
| `internal/gh/cli/graphql.go`（変更） | `searchItems` — 検索 1 発の共通経路。`ListWorkSection` と `SearchItems` がここに乗る |
| `internal/gh/cli/testdata/work_section.json`（録り直し） | `state` を含む Work 列の実レスポンス |
| `internal/gh/cli/testdata/search_items.json`（新規） | open / closed / merged が混ざる検索の実レスポンス |
| `internal/gh/cli/testdata/README.md`（変更） | 上 2 つの録り方 |

---

### Task 1: 桁の道具を `internal/tui/layout` に出す

`internal/tui/repo/render.go` の `clip` / `pad` / `right` と、
`internal/tui/dialog/render.go` の `padTo` / `rightTo` は同じものが 2 つある。
Search の結果表が 3 つ目になる前に 1 つにする。

**Files:**
- Create: `internal/tui/layout/columns.go`
- Create: `internal/tui/layout/columns_test.go`
- Modify: `internal/tui/repo/render.go`（`clip` / `pad` / `right` の定義と呼び出し）
- Modify: `internal/tui/repo/sidebar.go`（`pad` / `right` の呼び出し）
- Modify: `internal/tui/dialog/render.go`（`padTo` / `rightTo` の定義と呼び出し）

**Interfaces:**
- Produces:
  - `func layout.Clip(s string, w int) string`
  - `func layout.Pad(s string, w int) string`
  - `func layout.Right(s string, w int) string`

- [ ] **Step 1: 失敗するテストを書く**

`internal/tui/layout/columns_test.go` を新規に作る。

```go
package layout_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/tui/layout"
)

func TestPadLeavesAColumnBetweenFields(t *testing.T) {
	t.Parallel()

	// A name that exactly fills the field would run into the next one, so
	// Pad clips to one column short of the width.
	got := layout.Pad("abcdefgh", 8)
	if ansi.StringWidth(got) != 8 {
		t.Errorf("Pad width = %d, want 8: %q", ansi.StringWidth(got), got)
	}
	if got[len(got)-1] != ' ' {
		t.Errorf("Pad = %q, want it to end in a space", got)
	}
}

func TestPadCountsJapaneseAsTwoColumns(t *testing.T) {
	t.Parallel()

	got := layout.Pad("あいうえお", 8)
	if ansi.StringWidth(got) != 8 {
		t.Errorf("Pad width = %d, want 8: %q", ansi.StringWidth(got), got)
	}
}

func TestRightEndsFlush(t *testing.T) {
	t.Parallel()

	got := layout.Right("2h", 8)
	if ansi.StringWidth(got) != 8 {
		t.Errorf("Right width = %d, want 8: %q", ansi.StringWidth(got), got)
	}
	if got[len(got)-1] != 'h' {
		t.Errorf("Right = %q, want it to end in the text", got)
	}
}

func TestClipMarksWhatItCut(t *testing.T) {
	t.Parallel()

	got := layout.Clip("a very long title indeed", 10)
	if ansi.StringWidth(got) > 10 {
		t.Errorf("Clip width = %d, want at most 10: %q", ansi.StringWidth(got), got)
	}
	if !strings.Contains(got, "…") {
		t.Errorf("Clip = %q, want an ellipsis where it cut", got)
	}
}
```

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/tui/layout/`
Expected: FAIL — `undefined: layout.Pad` ほか

- [ ] **Step 3: `columns.go` を書く**

`internal/tui/repo/render.go` の 3 つの関数を**中身を変えずに**移す。

```go
package layout

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Clip cuts s to w display columns. Japanese takes two columns per
// character, so the count is never a byte or a rune count.
func Clip(s string, w int) string { return ansi.Truncate(s, w, "…") }

// Pad clips s to one column short of w and pads it out, so two fields never
// run into each other.
func Pad(s string, w int) string {
	s = Clip(s, max(w-1, 0))
	return s + strings.Repeat(" ", max(w-ansi.StringWidth(s), 0))
}

// Right pads s on the left instead, so a column of ages ends flush.
func Right(s string, w int) string {
	s = Clip(s, w)
	return strings.Repeat(" ", max(w-ansi.StringWidth(s), 0)) + s
}
```

- [ ] **Step 4: 通ることを確かめる**

Run: `go test ./internal/tui/layout/`
Expected: PASS

- [ ] **Step 5: 呼び出し側を 1 つにする**

`internal/tui/repo/render.go` から `clip` / `pad` / `right` の**定義を消し**、
呼び出しを `layout.Clip` / `layout.Pad` / `layout.Right` に置き換える
（`sidebar.go` にも呼び出しがある。`go build ./...` が通るまで潰す）。

`internal/tui/dialog/render.go` からは `padTo` / `rightTo` の定義を消し、
`candidateLines` の呼び出しを `layout.Pad` / `layout.Right` に置き換える。
`dialog` は `internal/tui/layout` を import していないので追加する。

**中身を変えない。** `padTo` は `ansi.Truncate(s, max(w-1,0), "…")` で、
`Pad` と同じ式である。違いがあったらそれは移植ミスなので、書き換える前に
両方の式を並べて確かめること。

- [ ] **Step 6: 画面が 1 桁も変わっていないことを確かめる**

Run: `go test ./internal/tui/...`
Expected: PASS。**golden テストが落ちたら移植が間違っている。**
`make golden` で録り直して黙らせない。

- [ ] **Step 7: テストが空振りでないことを確かめる**

`layout.Pad` の `max(w-1, 0)` を一時的に `w` に変え、
`TestPadLeavesAColumnBetweenFields` が落ちることを目で見る。戻す。

- [ ] **Step 8: `make check`**

Run: `make check`
Expected: 緑

- [ ] **Step 9: Commit**

```bash
git add internal/tui/layout internal/tui/repo internal/tui/dialog
git commit -m "refactor: keep one way to fit text into a column

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: 2 ペインの連結を `internal/tui/layout` に出す

Search タブも左にフィルタ、右に結果の 2 ペインで、間の罫線の引き方は Repos と同じ
（モックアップの `side2`）。`joinPanes` は `internal/tui/repo` の非公開関数なので、
そのままでは 2 つ目のタブから使えない。

**Files:**
- Create: `internal/tui/layout/panes.go`
- Create: `internal/tui/layout/panes_test.go`
- Modify: `internal/tui/repo/sidebar.go:118-145`（`joinPanes` と `padExact` を消す）
- Modify: `internal/tui/repo/render.go`（`joinPanes` の呼び出し）

**Interfaces:**
- Consumes: `layout.Clip` / `layout.Pad`（Task 1）
- Produces: `func layout.JoinPanes(left, right []string, leftWidth int) []string`

- [ ] **Step 1: 失敗するテストを書く**

`internal/tui/layout/panes_test.go` を新規に作る。

```go
package layout_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/tui/layout"
)

func TestJoinPanesRunsTheRuleDownTheTallerPane(t *testing.T) {
	t.Parallel()

	got := layout.JoinPanes([]string{"a"}, []string{"x", "y", "z"}, 10)
	if len(got) != 3 {
		t.Fatalf("got %d lines, want 3: %q", len(got), got)
	}
	for i, line := range got {
		if !strings.Contains(line, "│") {
			t.Errorf("line %d has no rule: %q", i, line)
		}
	}
}

func TestJoinPanesPutsTheRuleInTheSameColumnOnEveryLine(t *testing.T) {
	t.Parallel()

	// A short left line and a long one must not move the rule: the eye
	// reads the boundary as one straight line down the page.
	got := layout.JoinPanes([]string{"a", "あいうえ"}, []string{"x", "y"}, 10)
	want := -1
	for i, line := range got {
		at := ansi.StringWidth(line[:strings.Index(line, "│")])
		if want == -1 {
			want = at
			continue
		}
		if at != want {
			t.Errorf("line %d puts the rule at column %d, want %d", i, at, want)
		}
	}
	if want != 10 {
		t.Errorf("the rule sits at column %d, want 10", want)
	}
}
```

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/tui/layout/`
Expected: FAIL — `undefined: layout.JoinPanes`

- [ ] **Step 3: `panes.go` を書く**

`internal/tui/repo/sidebar.go` の `joinPanes` と `padExact` を中身を変えずに移す。
`padExact` は非公開のまま（`Pad` との違いを外に見せる必要が無い）。

```go
package layout

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/tui/theme"
)

// JoinPanes puts the left pane beside the right one, padding whichever is
// shorter so the rule runs the full height of the taller one.
func JoinPanes(left, right []string, leftWidth int) []string {
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
		out[i] = padExact(l, leftWidth) + theme.Rule().Render("│") + r
	}
	return out
}

// padExact pads s out to exactly w columns with no ellipsis. A caller has
// already fitted its lines to leftWidth; running them through Pad here would
// reserve its usual one-column margin and clip a name that already fits.
func padExact(s string, w int) string {
	return s + strings.Repeat(" ", max(w-ansi.StringWidth(s), 0))
}
```

- [ ] **Step 4: 通ることを確かめる**

Run: `go test ./internal/tui/layout/`
Expected: PASS

- [ ] **Step 5: 呼び出し側を移す**

`internal/tui/repo/sidebar.go` から `joinPanes` と `padExact` の定義を消し、
`render.go` の `joinPanes(m.sidebar(), lines, sidebarWidth)` を
`layout.JoinPanes(...)` にする。`sidebar.go` が `ansi` と `strings` を
使わなくなるなら import も消す（自分の変更で不要になったものは消す）。

- [ ] **Step 6: 画面が変わっていないことを確かめる**

Run: `go test ./internal/tui/...`
Expected: PASS。golden が落ちたら移植が間違っている。

- [ ] **Step 7: テストが空振りでないことを確かめる**

`padExact` の `max(w-ansi.StringWidth(s), 0)` を `0` に変え、
`TestJoinPanesPutsTheRuleInTheSameColumnOnEveryLine` が落ちることを目で見る。戻す。

- [ ] **Step 8: `make check`**

Run: `make check`
Expected: 緑

- [ ] **Step 9: Commit**

```bash
git add internal/tui/layout internal/tui/repo
git commit -m "refactor: let a second tab put two panes side by side

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: 検索結果が open / closed / merged のどれかを持つ

Work 板は常に `is:open` なので状態を引いていない。Search には `state` フィルタが
あり、結果に closed と merged が混ざる。`.graphql` 文書・スキーマ検証・録りものを
一度に触る。

**Files:**
- Modify: `internal/gh/cli/work.graphql`
- Modify: `internal/gh/gh.go:214-231`（`WorkItem`）
- Modify: `internal/gh/cli/graphql.go`（`searchNode` と `toWorkItem`）
- Modify: `internal/gh/cli/testdata/work_section.json`（録り直し）
- Modify: `internal/gh/cli/testdata/README.md`
- Test: `internal/gh/cli/graphql_test.go`

**Interfaces:**
- Produces: `gh.WorkItem` に `State gh.ItemState` が増える（`gh.ParseItemState` で変換済み）

- [ ] **Step 1: 失敗するテストを書く**

`internal/gh/cli/graphql_test.go` に足す。ファイル先頭の `workJSON` には
手で `"state"` を足さない（あれは手書きの定数で、文書が何を選ぶかの証拠にならない）。
文書が `state` を選んでいること自体と、値の変換の 2 つを分けて書く。

```go
// The Work board is always is:open, but the same document answers the Search
// tab, whose results carry closed and merged items.
func TestTheSearchDocumentSelectsTheState(t *testing.T) {
	t.Parallel()

	prBlock, issueBlock := onTypeBlocks(t, workQuery)
	if !strings.Contains(prBlock, "state") {
		t.Errorf("the PullRequest selection does not ask for state:\n%s", prBlock)
	}
	if !strings.Contains(issueBlock, "state") {
		t.Errorf("the Issue selection does not ask for state:\n%s", issueBlock)
	}
}

func TestAMergedPullRequestComesBackMerged(t *testing.T) {
	t.Parallel()

	const merged = `{"data":{"results":{"nodes":[
	  {"__typename":"PullRequest","number":9,"title":"merged one","state":"MERGED",
	   "url":"https://github.com/kukv/octoscope/pull/9",
	   "updatedAt":"2026-09-06T12:00:00Z","author":{"login":"kukv"},
	   "repository":{"nameWithOwner":"kukv/octoscope"}}
	]}}}`

	c, _ := newTestClient(merged, nil)
	items, err := c.ListWorkSection(t.Context(), gh.SectionAssigned)
	if err != nil {
		t.Fatalf("ListWorkSection: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	if items[0].State != gh.StateMerged {
		t.Errorf("State = %v, want StateMerged", items[0].State)
	}
}
```

`onTypeBlocks` は無いので書く。同じファイルの下のほうに置く。

```go
// onTypeBlocks cuts the document into what it selects for a pull request and
// what it selects for an issue. Searching the whole document for "state"
// would pass while only one of the two branches asked for it.
func onTypeBlocks(t *testing.T, doc string) (pr, issue string) {
	t.Helper()

	prAt := strings.Index(doc, "... on PullRequest")
	issueAt := strings.Index(doc, "... on Issue")
	if prAt < 0 || issueAt < 0 || prAt > issueAt {
		t.Fatalf("the document does not hold a PullRequest block before an Issue block:\n%s", doc)
	}
	// The fragments below the query select a field called "state" of their
	// own (StatusContext). An Issue block that ran to the end of the
	// document would find it and pass whatever the Issue itself selects.
	end := strings.Index(doc[issueAt:], "\nfragment ")
	if end < 0 {
		t.Fatalf("the document has no fragment after the Issue block:\n%s", doc)
	}
	return doc[prAt:issueAt], doc[issueAt : issueAt+end]
}
```

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/gh/cli/ -run 'TheSearchDocumentSelectsTheState|AMergedPullRequest'`
Expected: FAIL — 文書に `state` が無く、`items[0].State` のフィールドも無い

- [ ] **Step 3: 文書とドメイン型に `state` を足す**

`internal/gh/cli/work.graphql` の `... on PullRequest` と `... on Issue` の
どちらにも 1 行足す。`number` の隣に置く。

```graphql
  ... on PullRequest {
    number
    title
    state
```

```graphql
  ... on Issue {
    number
    title
    state
```

`internal/gh/gh.go` の `WorkItem` に足す。位置は `IsDraft` の隣。

```go
	IsDraft bool
	// State is open, closed or merged. The Work board's own searches are
	// all is:open; a search the user wrote is not.
	State ItemState
```

`internal/gh/cli/graphql.go` の `searchNode` に `State string \`json:"state"\`` を足し、
`toWorkItem` で変換する。

```go
		State: gh.ParseItemState(n.State),
```

- [ ] **Step 4: 通ることを確かめる**

Run: `go test ./internal/gh/cli/ -run 'TheSearchDocumentSelectsTheState|AMergedPullRequest'`
Expected: PASS

Run: `go test ./internal/gh/cli/ -run TestEveryFieldTheDocumentsSelect`
Expected: PASS（`schema.json` に `PullRequest.state` と `Issue.state` は既にある）

- [ ] **Step 5: `work_section.json` を録り直す**

録りものは `state` を含まないので、パースすると全部 closed になる。
**fixture を手で編集しない。** `testdata/README.md` の手順で録り直す。

```bash
gh api graphql -F query=@internal/gh/cli/work.graphql \
  -f search='is:open assignee:@me' > /tmp/work-raw.json
jq -r '[.data.results.nodes[]?.repository.nameWithOwner] | unique[]' /tmp/work-raw.json
# 出てきたリポジトリを 1 つずつ確認する
gh repo view <owner>/<repo> --json nameWithOwner,isPrivate
# isPrivate:false だったものだけを allowlist に入れ、それだけを残す
```

`README.md` の `work_section.json` の節に、録り直した日付と
「`state` を含むようになった」ことを 1 行足す。

**`gh` が使えない環境で実装している場合はここで止めて人に頼む。**
手で `"state"` を書き足すのは README が禁じている編集である。

- [ ] **Step 6: 録り直しを強制するアサーションを足す**

`state` の無い古い録りものは `ParseItemState("")` が closed を返すので、
足さないと「録り直し忘れ」が緑のまま通る。`graphql_test.go:423` からの
録りものを読むテスト（`items` を回している箇所）に足す。

```go
		// The recording is is:open assignee:@me, so anything else means the
		// fixture predates the state field and was not re-recorded.
		if item.State != gh.StateOpen {
			t.Errorf("%q came back %v; re-record work_section.json", item.Title, item.State)
		}
```

Run: `go test ./internal/gh/cli/`
Expected: PASS（録り直していれば緑。古い録りもののままなら赤で、それが正しい）

- [ ] **Step 7: テストが空振りでないことを確かめる**

2 つ壊して 2 つとも落ちることを目で見る。1 つずつ戻す。

- `toWorkItem` の `State:` の行を消す → `TestAMergedPullRequestComesBackMerged` が落ちる
- `work.graphql` の **Issue 側だけ**から `state` を消す →
  `TestTheSearchDocumentSelectsTheState` が落ちる。片側だけの検査になっていないことを
  ここで確かめる

- [ ] **Step 8: `make check`**

Run: `make check`
Expected: 緑

- [ ] **Step 9: Commit**

```bash
git add internal/gh internal/gh/cli
git commit -m "feat: carry whether a search result is open, closed or merged

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: 任意のクエリで検索する口を開ける

Work 板の 4 列は固定の検索文字列を持った `searchItems` の呼び出しに過ぎない。
利用者のクエリを渡せるようにし、失敗したときに GitHub が言ったことが
呼び出し元まで届くことを固定する。

**Files:**
- Modify: `internal/gh/cli/graphql.go:89-110`（`ListWorkSection`）
- Create: `internal/gh/cli/testdata/search_items.json`
- Modify: `internal/gh/cli/testdata/README.md`
- Test: `internal/gh/cli/graphql_test.go`

**Interfaces:**
- Consumes: `gh.WorkItem.State`（Task 3）
- Produces: `func (c *Client) SearchItems(ctx context.Context, query string) ([]gh.WorkItem, error)`

- [ ] **Step 1: 失敗するテストを書く**

`internal/gh/cli/graphql_test.go` に足す。

```go
func TestSearchItemsSendsTheQueryItWasGiven(t *testing.T) {
	t.Parallel()

	c := New("/tmp", "")
	var got []string
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return []byte(emptyColumnJSON), nil
	}

	if _, err := c.SearchItems(context.Background(), "is:pr org:kukv label:renovate"); err != nil {
		t.Fatalf("SearchItems: %v", err)
	}

	joined := strings.Join(got, " ")
	if !strings.Contains(joined, "search=is:pr org:kukv label:renovate") {
		t.Errorf("the query never reached gh:\n%s", joined)
	}
	// The query travels as a variable. A query that became part of the
	// document could not hold a quote or a brace.
	if strings.Contains(joined, `search(type: ISSUE, first: 50, query: "is:pr`) {
		t.Errorf("the query was pasted into the document:\n%s", joined)
	}
}

// A query the user typed can be one GitHub rejects. What it said is the only
// thing that tells them how to fix it, so it must not be swallowed.
func TestSearchItemsReportsWhatGitHubSaidAboutABadQuery(t *testing.T) {
	t.Parallel()

	const rejected = `{"data":null,"errors":[{"message":"Invalid search query"}]}`
	c, _ := newTestClient(rejected, errors.New(`gh api: {"message":"Invalid search query"}`))

	_, err := c.SearchItems(t.Context(), "is:nonsense")
	if err == nil {
		t.Fatal("SearchItems succeeded on a query GitHub rejected")
	}
	if !strings.Contains(err.Error(), "Invalid search query") {
		t.Errorf("err = %v, want it to carry what GitHub said", err)
	}
	// The Search tab shows this on its notice line. A fatal error would
	// replace the whole screen over one mistyped query.
	if gh.IsFatal(err) {
		t.Errorf("err = %v, want it not to be fatal", err)
	}
}

func TestSearchItemsParsesARecordedSearch(t *testing.T) {
	t.Parallel()

	c, _ := newTestClient(readTestdata(t, "search_items.json"), nil)
	items, err := c.SearchItems(t.Context(), "repo:kukv/octoscope is:pr")
	if err != nil {
		t.Fatalf("SearchItems: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("no items parsed out of the recording")
	}
	states := map[gh.ItemState]bool{}
	for _, item := range items {
		states[item.State] = true
		if item.Ref.Repo == "" {
			t.Errorf("%q has no repo; the row cannot be opened", item.Title)
		}
	}
	// The recording was taken with no state qualifier, so it holds more than
	// open ones. A recording that lost that would stop testing the state.
	if len(states) < 2 {
		t.Errorf("the recording holds only %v; re-record it over open and closed items", states)
	}
}
```

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/gh/cli/ -run TestSearchItems`
Expected: FAIL — `c.SearchItems undefined`

- [ ] **Step 3: `ListWorkSection` を共通経路の上に載せ替える**

`internal/gh/cli/graphql.go` の `ListWorkSection` の中身を `searchItems` に移し、
両方から呼ぶ。既存のコメント（固定テキストだから `errors` は文書の壊れ）は
**`ListWorkSection` 側には残らない**: 同じ経路を利用者のクエリも通るので、
理由を書き直す。`work.graphql` の冒頭の `# One column of the Work board.` も
同じ理由で事実でなくなるので直す（`# One GitHub issue search: a column of the
Work board, or a query the user built.`）。

```go
// ListWorkSection fetches one column of the Work board. Its search string is
// fixed text embedded at build time, not anything the user typed.
func (c *Client) ListWorkSection(ctx context.Context, s gh.WorkSection) ([]gh.WorkItem, error) {
	if s < 0 || int(s) >= len(workSearches) {
		return nil, fmt.Errorf("unknown work section %d", s)
	}
	return c.searchItems(ctx, workSearches[s])
}

// SearchItems runs one GitHub issue search and returns what it found. The
// query is the user's, so a rejected one is an ordinary failure to report
// rather than a broken document.
func (c *Client) SearchItems(ctx context.Context, query string) ([]gh.WorkItem, error) {
	return c.searchItems(ctx, query)
}

// searchItems is the one call behind both. The document itself travels as
// gh's own "query" parameter, so the search string has to go under a
// different name.
//
// Unlike RepoCounts there is no partial body worth salvaging: a search has
// one result set, and half of one would be read as "that is all there is".
// The error carries what GitHub said, which is what a user has to act on.
func (c *Client) searchItems(ctx context.Context, search string) ([]gh.WorkItem, error) {
	out, err := c.read(ctx, c.dir, "api", "graphql",
		"-f", "query="+workQuery, "-f", "search="+search)
	if err != nil {
		return nil, err
	}
	var resp workResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("parse search: %w", err)
	}
	nodes := resp.Data.Results.Nodes
	items := make([]gh.WorkItem, 0, len(nodes))
	for _, n := range nodes {
		items = append(items, n.toWorkItem())
	}
	return items, nil
}
```

- [ ] **Step 4: 録りものを録る**

```bash
D=internal/gh/cli/testdata
gh api graphql -F query=@internal/gh/cli/work.graphql \
  -f search='repo:kukv/octoscope is:pr sort:updated-desc' | jq . > $D/search_items.json
```

`kukv/octoscope` は公開リポジトリなので伏せるものは無い。
**`state` 修飾子を付けない**こと: 付けると open だけになり、
`TestSearchItemsParsesARecordedSearch` の状態の検査が意味を失う。
録れた JSON の `state` を見て、`OPEN` と `MERGED`（または `CLOSED`）が
両方入っていることを確かめてからコミットする。

```bash
jq -r '[.data.results.nodes[].state] | unique' $D/search_items.json
```

`README.md` に節を足す。

```markdown
## `search_items.json`

`work.graphql` を利用者のクエリで叩いた実レスポンス。録った日: <実際の日付>、
対象: `repo:kukv/octoscope is:pr sort:updated-desc`。

`state` 修飾子を付けずに録ってある。Work 板は常に `is:open` なので、
open 以外が入っている録りものはこれしか無い。

```bash
gh api graphql -F query=@internal/gh/cli/work.graphql \
  -f search='repo:kukv/octoscope is:pr sort:updated-desc' | jq . > $D/search_items.json
```
```

- [ ] **Step 5: 通ることを確かめる**

Run: `go test ./internal/gh/cli/`
Expected: PASS（`TestListWorkSectionSendsOneSearch` も含めて全部）

- [ ] **Step 6: テストが空振りでないことを確かめる**

`searchItems` の `if err != nil { return nil, err }` を
`if err != nil { return nil, nil }` に変え、
`TestSearchItemsReportsWhatGitHubSaidAboutABadQuery` が落ちることを目で見る。戻す。

- [ ] **Step 7: `make check`**

Run: `make check`
Expected: 緑

- [ ] **Step 8: Commit**

```bash
git add internal/gh/cli
git commit -m "feat: search GitHub with a query the caller chose

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: このスライスの積み残しを書く

前のスライスと同じ受け渡しを残す。**直さないと決めた理由も書く。**

**Files:**
- Create: `docs/superpowers/2026-09-11-phase4-search-foundation-followups.md`

- [ ] **Step 1: 積み残しを書く**

最低限、次を含めること。実装中に見つかったものがあれば足す。

- **`first: 50` の上限とページングの無さ。** Work 板から引き継いだ（設計 §1 が
  Work の切り詰めを別の機会に送っている）。Search は 1 リポジトリに絞らない
  クエリを投げられるので、Work 板より当たりやすい。3-2 で「50 件で切れている」ことを
  画面に出すかどうかを決める
- **`internal/usecase` への配線が無い。** interface は利用側で宣言するので 3-2 で足す
- **3-2 に渡すもの**: `layout.Clip` / `Pad` / `Right` / `JoinPanes` と
  `gh.WorkItem.State`。結果行の列構成はモックアップの S2
  （`st / rp / num / ttl / ag`、checks 列は無い）で、Repos の右ペインとは違う
- **3-3 に渡すもの**: `config.Store` には `SaveRepositories` しか無い。
  `saved_queries` を書くときに `SaveQueries` を足す。`save(Config)` は共通化済み。
  ダイアログの候補にスクロールが無い件（前スライスの積み残し 8 番）は、
  保存クエリのポップアップが 2 人目の利用者になる 3-3 で決める

- [ ] **Step 2: Commit**

```bash
git add docs/superpowers/2026-09-11-phase4-search-foundation-followups.md
git commit -m "docs: record what the Search foundation left behind

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## 完了条件

1. `internal/tui/layout` に `Clip` / `Pad` / `Right` / `JoinPanes` があり、
   `repo` と `dialog` が自前の同じ関数を持っていない
2. **golden が 1 行も変わっていない**（このスライスは画面を変えない）
3. `work.graphql` が `state` を PullRequest と Issue の両方で選び、
   `schema_test.go` がそれを検証している
4. `gh.WorkItem.State` が `open` / `closed` / `merged` を持ち、録りもので検証されている
5. `SearchItems` が任意のクエリで検索でき、GitHub が拒んだクエリのメッセージが
   呼び出し元に届き、それが `gh.IsFatal` ではない
6. `ListWorkSection` と `SearchItems` が同じ 1 つの経路を通り、`.graphql` 文書も 1 つのまま
7. `make check` が緑
