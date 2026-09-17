# Issue / PR 詳細画面 再設計 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 詳細画面のメタ情報を本文から空間的に分離し、「どこからどこまでが何の情報か」を
一目で分かるようにする。

**Architecture:** 変更は `internal/app/presentation/tui/detail` の描画の中だけで閉じる。
メタ情報は左の固定ペイン（100 桁未満では上の 1 段落）、説明とコメントは右の viewport。
Markdown を 1 本に連結するのをやめ、説明とコメントを別々に glamour へ渡して、
出来上がった行を viewport に入れる。usecase / domain / gateway には触らない。

**Tech Stack:** Go 1.25 / Bubble Tea v2 / bubbles viewport v2 / glamour v2 / golden テスト

**Spec:** `docs/superpowers/specs/2026-09-16-detail-redesign-design.md`

**Branch:** `feat/detail-redesign`（作成済み。spec の 2 コミットが載っている）

---

## Global Constraints

- 各タスクの末尾で `make check` が緑。緑でない状態でコミットしない
- 文字列は `en` / `ja` 両方のカタログへ。片方だけだと `TestCatalogsHaveTheSameIDs` が落ちる
- 桁数は `ansi.StringWidth`（`github.com/charmbracelet/x/ansi`）で数える。`len` も
  `utf8.RuneCountInString` も使わない（`.claude/rules/tui.md`）
- 色は `theme` から引く。16 進をビューのファイルに書かない
- `work` パッケージを import しない。画面同士の依存になる（spec §2）
- ゴールデンの記録は `make golden`。**diff を読んでからコミットする**

## 既存コードの前提（2026-09-16 に確認）

- `detail.Model` は取得したアイテムを**保持していない**。`itemArrived`
  (`detail.go:427`) が title / labels / assignees / url / state を取り出し、本文は
  Markdown にして `setContent` で viewport に入れて捨てている。今回の左ペインは
  アイテムそのものを読むので、Model に持たせる
- 寸法は `resize`（`detail.go:407`）でしか決まらない。`m.body.SetWidth(msg.Width)` /
  `SetHeight(max(msg.Height-4, 5))`
- `layout.JoinPanes(left, right []string, leftWidth int) []string` が縦罫つきの
  ペイン連結を持っている（Work ボードのドロワーが使っている）
- `layout.Pad(s, w)` は **w-1 桁に切ってから**詰める。ラベル列の幅はラベルの実幅 +1 以上要る
- `icon.CommentBar()` は `▌`（ASCII セットでは `|`）。diff のスレッド表示が使っている
- `golden.Assert` は `OCTOSCOPE_UPDATE_GOLDEN=1` で記録し直す。`make golden` がそれ

---

### Task 1: Model にアイテムを持たせ、寸法計算を 1 か所に集める

見た目はまだ変えない。以降のタスクが乗る土台を作る。

**Files:**
- Modify: `internal/app/presentation/tui/detail/detail.go`
- Test: `internal/app/presentation/tui/detail/detail_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`detail_test.go` の末尾に足す。

```go
// TestTheModelKeepsTheItem is what the meta pane will read. Until now the
// view kept only what it had already turned into a string.
func TestTheModelKeepsTheItem(t *testing.T) {
	f := &fakeSource{pr: domain.PR{
		Number: 1, Title: "first pr", Author: domain.Author{Login: "kukv"},
		Head: "feat/x", Base: "main", Additions: 10, Deletions: 2,
	}}
	m := loaded(f, prRef())
	if m.item.PR == nil {
		t.Fatal("the model did not keep the pull request")
	}
	if got := m.item.PR.Head; got != "feat/x" {
		t.Errorf("Head = %q, want %q", got, "feat/x")
	}
}

// TestResizeKeepsThePlaceInTheBody covers the re-render a resize now needs:
// the body is laid out for a width that changed, and the reader should not be
// thrown back to the top of a long description.
func TestResizeKeepsThePlaceInTheBody(t *testing.T) {
	m := loaded(&fakeSource{pr: longPR()}, prRef())
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	m, _ = m.Update(wheelDown())
	before := m.body.YOffset()
	if before == 0 {
		t.Fatal("the wheel did not scroll the body")
	}

	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})

	if got := m.body.YOffset(); got != before {
		t.Errorf("YOffset = %d after a resize, want the place to be kept at %d", got, before)
	}
}
```

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/app/presentation/tui/detail/ -run 'TestTheModelKeepsTheItem|TestResizeKeepsThePlaceInTheBody' -v`
Expected: FAIL — `m.item undefined` でコンパイルが通らない

- [ ] **Step 3: Model にフィールドを足す**

`detail.go` の Model に、`state domain.ItemState` の次へ足す。

```go
	// item is the whole of what was fetched. The meta pane reads it at draw
	// time, so unlike title and state it cannot be turned into a string once
	// and thrown away: its layout changes with the terminal's width.
	item   usecase.Item
	loaded bool
```

- [ ] **Step 4: itemArrived に保存させる**

`detail.go:427` の `itemArrived` を書き換える。`setContent` の呼び出しは
`m.relayout()` に置き換わる。

```go
func (m Model) itemArrived(msg itemMsg) Model {
	if msg.ref != m.ref {
		return m
	}
	it := msg.item
	m.phase = phaseIdle
	m.state = it.State
	m.errText = ""
	m.declined = ""
	m.labels = labelNames(it.Labels)
	m.assignees = authorLogins(it.Assignees)
	m.url = it.URL
	m.item, m.loaded = it, true
	if it.Kind == domain.ItemPR {
		m.title = i18n.Tf("detail.pr_title", map[string]any{"Number": it.Number, "Title": it.Title})
	} else {
		m.title = i18n.Tf("detail.issue_title", map[string]any{"Number": it.Number, "Title": it.Title})
	}
	m.relayout()
	m.body.GotoTop()
	return m
}
```

- [ ] **Step 5: relayout を書き、resize から呼ぶ**

`setContent`（`detail.go:862`）を次の 2 つで置き換える。`prMarkdown` /
`issueMarkdown` はまだ残っているので、このタスクでは中身をそのまま使う。

```go
// relayout gives the body its width and height and lays the content out
// again. It runs both on a resize and when the item arrives: what the body
// is worth depends on the width, and — once the meta block sits above it —
// how tall that block turned out.
func (m *Model) relayout() {
	w := m.width
	if w <= 0 {
		w = 80
	}
	h := m.height
	if h <= 0 {
		h = 24
	}
	m.body.SetWidth(max(w, 1))
	m.body.SetHeight(max(h-chromeLines, 5))
	m.setBodyContent(max(w, 1))
}

// setBodyContent re-renders the body at w, keeping the reader's place. The
// content is laid out for a width, so every width change rebuilds it.
func (m *Model) setBodyContent(w int) {
	if !m.loaded {
		return
	}
	var md string
	if m.item.Kind == domain.ItemPR {
		md = prMarkdown(*m.item.PR)
	} else {
		md = issueMarkdown(m.item)
	}
	content := md
	if r, err := glamour.NewTermRenderer(glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(w-2)); err == nil {
		if out, err := r.Render(md); err == nil {
			content = out
		}
	}
	at := m.body.YOffset()
	m.body.SetContent(content)
	m.body.SetYOffset(at)
}
```

同じファイルの const ブロック（無ければ import の下に作る）へ足す。

```go
// chromeLines is what the body never gets: the title line, the key bar, and
// one line held back for the error, which must not push the key bar off the
// bottom of the screen.
const chromeLines = 4
```

`resize`（`detail.go:407`）の body の 2 行を `m.relayout()` に置き換える。

```go
func (m Model) resize(msg tea.WindowSizeMsg) Model {
	m.width, m.height = msg.Width, msg.Height
	m.relayout()
	m.textarea.SetWidth(msg.Width)
	m.textarea.SetHeight(max(msg.Height-6, 3))
	if m.submit.Active() {
		m.submit, _ = m.submit.Update(msg)
	}
	if m.merge.Active() {
		m.merge, _ = m.merge.Update(msg)
	}
	return m
}
```

- [ ] **Step 6: テストが通ることを確かめる**

Run: `go test ./internal/app/presentation/tui/detail/ -v`
Expected: PASS（ゴールデンも含めて全部。この時点では描画は変わっていない）

- [ ] **Step 7: コミット**

```bash
git add internal/app/presentation/tui/detail/detail.go internal/app/presentation/tui/detail/detail_test.go
git commit -m "refactor(detail): keep the item and lay the body out in one place

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: カタログに新しいキーを足す

**Files:**
- Modify: `internal/i18n/locales/active.ja.yaml`
- Modify: `internal/i18n/locales/active.en.yaml`
- Test: `internal/app/presentation/tui/detail/render_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`render_test.go` の末尾に足す。

```go
// TestTheNewMetaKeysResolve catches a key that was added to one catalog and
// not the other before it reaches a view.
func TestTheNewMetaKeysResolve(t *testing.T) {
	ids := []string{
		"detail.meta.repo", "detail.meta.author", "detail.meta.state",
		"detail.meta.review", "detail.meta.checks", "detail.meta.branch",
		"detail.meta.changes", "detail.meta.assignees", "detail.meta.labels",
		"detail.meta.updated",
		"detail.section.description", "detail.section.comments",
		"detail.no_description", "state.draft_suffix",
	}
	for _, lang := range []language.Tag{language.English, language.Japanese} {
		i18n.SetLanguage(lang)
		t.Cleanup(func() { i18n.SetLanguage(language.English) })
		for _, id := range ids {
			i18n.AssertNoUnresolvedIDs(t, i18n.T(id))
		}
	}
	i18n.AssertNoUnresolvedIDs(t, i18n.Tn("detail.section.comments", 2))
}
```

`render_test.go` の import に `"golang.org/x/text/language"` と
`"github.com/kukv/octoscope/internal/i18n"` が無ければ足す。

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/app/presentation/tui/detail/ -run TestTheNewMetaKeysResolve -v`
Expected: FAIL — `unresolved message ID "detail.meta.repo"` など 14 件

- [ ] **Step 3: ja のカタログに足す**

`active.ja.yaml` の `detail:` ブロック（126 行目付近、`decline_loading` の下）に足す。

```yaml
  no_description:
    other: "_説明はありません_"
  meta:
    repo:
      other: "リポジトリ"
    author:
      other: "作成者"
    state:
      other: "状態"
    review:
      other: "レビュー"
    checks:
      other: "checks"
    branch:
      other: "ブランチ"
    changes:
      other: "変更"
    assignees:
      other: "担当"
    labels:
      other: "ラベル"
    updated:
      other: "更新"
  section:
    description:
      other: "説明"
    comments:
      other: "コメント {{.Count}} 件"
```

同じファイルの `state:` ブロックに足す。

```yaml
  draft_suffix:
    other: "（下書き）"
```

- [ ] **Step 4: en のカタログに足す**

`active.en.yaml` の `detail:` ブロックに足す。

```yaml
  no_description:
    other: "_no description_"
  meta:
    repo:
      other: "repository"
    author:
      other: "author"
    state:
      other: "state"
    review:
      other: "review"
    checks:
      other: "checks"
    branch:
      other: "branch"
    changes:
      other: "changes"
    assignees:
      other: "assignees"
    labels:
      other: "labels"
    updated:
      other: "updated"
  section:
    description:
      other: "Description"
    comments:
      one: "{{.Count}} comment"
      other: "{{.Count}} comments"
```

同じファイルの `state:` ブロックに足す。

```yaml
  draft_suffix:
    other: " (draft)"
```

- [ ] **Step 5: 通ることを確かめる**

Run: `go test ./internal/i18n/... ./internal/app/presentation/tui/detail/ -run 'TestCatalogsHaveTheSameIDs|TestTheNewMetaKeysResolve' -v`
Expected: PASS

- [ ] **Step 6: コミット**

```bash
git add internal/i18n/locales/active.ja.yaml internal/i18n/locales/active.en.yaml internal/app/presentation/tui/detail/render_test.go
git commit -m "i18n: add the detail view's meta and section strings

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: メタ行を組み立てる（`meta.go`）

**Files:**
- Create: `internal/app/presentation/tui/detail/meta.go`
- Create: `internal/app/presentation/tui/detail/meta_test.go`
- Modify: `internal/app/presentation/tui/detail/render.go`（`stateText` / `reviewText` を移す）

- [ ] **Step 1: 失敗するテストを書く**

`meta_test.go` を新規に作る。

```go
package detail

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/app/usecase"
)

func metaAt() time.Time { return time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC) }

func fullPRItem() usecase.Item {
	pr := domain.PR{
		Number: 12, Title: "a pr", Author: domain.Author{Login: "kukv"},
		State: domain.StateOpen, Review: domain.ReviewApproved, UpdatedAt: metaAt(),
		Labels:    []domain.Label{{Name: "bug", Color: "d73a4a"}},
		Assignees: []domain.Author{{Login: "alice"}},
		Checks:    domain.Checks{Total: 3, Passed: 1, Failed: 1, Running: 1},
		Head:      "feat/x", Base: "main", Additions: 218, Deletions: 31,
	}
	return usecase.Item{
		Kind: domain.ItemPR, Number: pr.Number, Title: pr.Title, Author: pr.Author,
		State: pr.State, Labels: pr.Labels, Assignees: pr.Assignees,
		UpdatedAt: pr.UpdatedAt, PR: &pr,
	}
}

func fullRef() domain.ItemRef {
	return domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/octoscope", Number: 12}
}

// labelsOf is what the row order is checked against: the values carry ANSI,
// the labels are what names the row.
func labelsOf(rows []metaRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.label
	}
	return out
}

func TestAPullRequestGetsEveryRow(t *testing.T) {
	rows := metaRows(fullRef(), fullPRItem())

	want := []string{
		"repository", "author", "state", "review", "checks",
		"branch", "changes", "assignees", "labels", "updated",
	}
	if got := labelsOf(rows); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("rows = %v, want %v", got, want)
	}
}

func TestAnIssueHasNoPullRequestRows(t *testing.T) {
	it := usecase.Item{
		Kind: domain.ItemIssue, Number: 7, Title: "an issue",
		Author: domain.Author{Login: "kukv"}, State: domain.StateOpen, UpdatedAt: metaAt(),
	}
	ref := domain.ItemRef{Kind: domain.ItemIssue, Repo: "kukv/octoscope", Number: 7}

	for _, gone := range []string{"review", "checks", "branch", "changes"} {
		for _, label := range labelsOf(metaRows(ref, it)) {
			if label == gone {
				t.Errorf("an issue was given the %q row", gone)
			}
		}
	}
}

// TestAnEmptyValueTakesNoRow keeps the pane from drawing labels with nothing
// after them.
func TestAnEmptyValueTakesNoRow(t *testing.T) {
	pr := domain.PR{
		Number: 3, Author: domain.Author{Login: "kukv"}, State: domain.StateOpen,
		Review: domain.ReviewNone, UpdatedAt: metaAt(),
	}
	it := usecase.Item{
		Kind: domain.ItemPR, Number: pr.Number, Author: pr.Author,
		State: pr.State, UpdatedAt: pr.UpdatedAt, PR: &pr,
	}

	want := []string{"repository", "author", "state", "updated"}
	got := labelsOf(metaRows(fullRef(), it))
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("rows = %v, want %v", got, want)
	}
}

func TestTheMetaPaneFitsItsWidth(t *testing.T) {
	lines := metaPaneLines(metaRows(fullRef(), fullPRItem()), 28)

	for _, l := range lines {
		if w := ansi.StringWidth(l); w > 28 {
			t.Errorf("line %q is %d columns wide, want at most 28", ansi.Strip(l), w)
		}
	}
}

// TestTheInlineMetaNamesTheAssignees covers the one label the single-column
// layout keeps: "@alice" on its own reads as the author.
func TestTheInlineMetaNamesTheAssignees(t *testing.T) {
	lines := metaInlineLines(metaRows(fullRef(), fullPRItem()), 80)

	joined := ansi.Strip(strings.Join(lines, " "))
	if !strings.Contains(joined, "assignees @alice") {
		t.Errorf("the inline meta does not name the assignees:\n%s", joined)
	}
	for _, l := range lines {
		if w := ansi.StringWidth(l); w > 80 {
			t.Errorf("line %q is %d columns wide, want at most 80", ansi.Strip(l), w)
		}
	}
}
```

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/app/presentation/tui/detail/ -run 'TestAPullRequestGetsEveryRow|TestAnIssueHasNoPullRequestRows|TestAnEmptyValueTakesNoRow|TestTheMetaPaneFitsItsWidth|TestTheInlineMetaNamesTheAssignees' -v`
Expected: FAIL — `undefined: metaRow` / `metaRows` / `metaPaneLines` / `metaInlineLines`

- [ ] **Step 3: `meta.go` を書く**

```go
package detail

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/app/presentation/tui/icon"
	"github.com/kukv/octoscope/internal/app/presentation/tui/layout"
	"github.com/kukv/octoscope/internal/app/presentation/tui/theme"
	"github.com/kukv/octoscope/internal/app/usecase"
	"github.com/kukv/octoscope/internal/i18n"
)

// metaLabelWidth is the label column of the two-pane layout. layout.Pad cuts
// to one column short of what it is given, so this is the longest label
// ("リポジトリ", ten columns) plus two.
const metaLabelWidth = 12

// metaRow is one fact about the item. The value is already styled; the label
// is drawn by whichever layout is in use.
type metaRow struct {
	label string
	value string

	// inlineLabel keeps the label when the rows are joined into a single
	// paragraph. Only the assignees need it: "@alice" on its own reads as
	// the author.
	inlineLabel bool
}

// metaRows is everything the meta pane says about the item, in the order the
// design puts it. A row whose value is empty is left out rather than drawn
// with nothing after its label.
func metaRows(ref domain.ItemRef, it usecase.Item) []metaRow {
	var rows []metaRow
	if ref.Repo != "" {
		rows = append(rows, metaRow{
			label: i18n.T("detail.meta.repo"),
			value: theme.Dim().Render(fmt.Sprintf("%s #%d", ref.Repo, it.Number)),
		})
	}
	rows = append(rows,
		metaRow{label: i18n.T("detail.meta.author"), value: "@" + it.Author.Login},
		metaRow{label: i18n.T("detail.meta.state"), value: itemStateText(it)},
	)
	if pr := it.PR; pr != nil {
		if pr.Review != domain.ReviewNone {
			rows = append(rows, metaRow{
				label: i18n.T("detail.meta.review"),
				value: theme.Review(pr.Review, pr.IsDraft).Render(reviewText(pr.Review)),
			})
		}
		if pr.Checks.Total > 0 {
			rows = append(rows, metaRow{
				label: i18n.T("detail.meta.checks"),
				value: checksText(pr.Checks),
			})
		}
		if pr.Head != "" && pr.Base != "" {
			rows = append(rows, metaRow{
				label: i18n.T("detail.meta.branch"),
				value: theme.Accent().Render(pr.Head) +
					theme.Dim().Render(" → ") + theme.Accent().Render(pr.Base),
			})
		}
		if pr.Additions > 0 || pr.Deletions > 0 {
			rows = append(rows, metaRow{
				label: i18n.T("detail.meta.changes"),
				value: theme.Added().Render("+"+strconv.Itoa(pr.Additions)) + " " +
					theme.Removed().Render("−"+strconv.Itoa(pr.Deletions)),
			})
		}
	}
	if len(it.Assignees) > 0 {
		logins := make([]string, len(it.Assignees))
		for i, a := range it.Assignees {
			logins[i] = "@" + a.Login
		}
		rows = append(rows, metaRow{
			label:       i18n.T("detail.meta.assignees"),
			value:       strings.Join(logins, " "),
			inlineLabel: true,
		})
	}
	if len(it.Labels) > 0 {
		names := make([]string, len(it.Labels))
		for i, l := range it.Labels {
			names[i] = theme.Badge(l.Color).Render(" " + l.Name + " ")
		}
		rows = append(rows, metaRow{
			label: i18n.T("detail.meta.labels"),
			value: strings.Join(names, " "),
		})
	}
	return append(rows, metaRow{
		label: i18n.T("detail.meta.updated"),
		value: theme.Dim().Render(i18n.DateTime(it.UpdatedAt)),
	})
}

// itemStateText names the state, with the draft suffix a pull request can
// carry. A draft is open, and the state word alone would not say so.
func itemStateText(it usecase.Item) string {
	s := stateText(it.State)
	if it.PR != nil && it.PR.IsDraft {
		s += i18n.T("state.draft_suffix")
	}
	return s
}

// checksText is the summary the detail view has room for: how many passed,
// failed and are still running. Which check is which is what the s key shows.
func checksText(c domain.Checks) string {
	var parts []string
	for _, p := range []struct {
		state domain.CheckState
		n     int
	}{
		{domain.CheckSuccess, c.Passed},
		{domain.CheckFailure, c.Failed},
		{domain.CheckRunning, c.Running},
	} {
		if p.n > 0 {
			parts = append(parts, theme.Check(p.state).Render(icon.Check(p.state)+strconv.Itoa(p.n)))
		}
	}
	return strings.Join(parts, " ")
}

// metaPaneLines draws the rows as the left pane: a dimmed label column and
// the value beside it, cut to whatever the pane was given.
func metaPaneLines(rows []metaRow, w int) []string {
	lines := make([]string, len(rows))
	for i, r := range rows {
		lines[i] = theme.Dim().Render(layout.Pad(r.label, metaLabelWidth)) +
			layout.Clip(r.value, max(w-metaLabelWidth, 0))
	}
	return lines
}

// metaInlineLines draws the same rows as one wrapped paragraph, for a
// terminal too narrow to give the pane a column of its own. Nothing is
// dropped: the paragraph grows by a line instead.
func metaInlineLines(rows []metaRow, w int) []string {
	parts := make([]string, len(rows))
	for i, r := range rows {
		parts[i] = r.value
		if r.inlineLabel {
			parts[i] = theme.Dim().Render(r.label+" ") + r.value
		}
	}
	joined := strings.Join(parts, theme.Dim().Render(" · "))
	if w <= 0 {
		return []string{joined}
	}
	return strings.Split(ansi.Wrap(joined, w, ""), "\n")
}
```

- [ ] **Step 4: `stateText` と `reviewText` を `meta.go` へ移す**

`render.go` から次の 2 つの関数（コメントごと）を切り取り、`meta.go` の末尾へ貼る。
呼ぶのはメタ行だけになるため。中身は変えない。

```go
// stateText and reviewText name a state in the reader's language. GitHub's
// own spelling stopped at the access layer (.claude/rules/architecture.md),
// and a state word is ours to translate.
func stateText(s domain.ItemState) string {
	switch s {
	case domain.StateOpen:
		return i18n.T("state.open")
	case domain.StateMerged:
		return i18n.T("state.merged")
	default:
		return i18n.T("state.closed")
	}
}

func reviewText(r domain.ReviewState) string {
	switch r {
	case domain.ReviewApproved:
		return i18n.T("review.approved")
	case domain.ReviewChangesRequested:
		return i18n.T("review.changes_requested")
	case domain.ReviewRequired:
		return i18n.T("review.required")
	default:
		return i18n.T("review.none")
	}
}
```

- [ ] **Step 5: 通ることを確かめる**

Run: `go test ./internal/app/presentation/tui/detail/ -v`
Expected: PASS（ゴールデンを含めて全部。View はまだ meta.go を呼んでいない）

- [ ] **Step 6: `make check`**

Run: `make check`
Expected: 緑

- [ ] **Step 7: コミット**

```bash
git add internal/app/presentation/tui/detail/meta.go internal/app/presentation/tui/detail/meta_test.go internal/app/presentation/tui/detail/render.go
git commit -m "feat(detail): build the meta rows the panes will draw

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: 説明とコメントのブロックを描く（`body.go`）

**Files:**
- Create: `internal/app/presentation/tui/detail/body.go`
- Create: `internal/app/presentation/tui/detail/body_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`body_test.go` を新規に作る。

```go
package detail

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/app/presentation/tui/icon"
	"github.com/kukv/octoscope/internal/app/usecase"
)

func withComments() usecase.Item {
	it := fullPRItem()
	it.Body = "This replaces the renderer.\n\n- one\n- two"
	it.Comments = []domain.Comment{
		{Author: domain.Author{Login: "bob"}, Body: "見た目が良い", CreatedAt: metaAt()},
		{Author: domain.Author{Login: "alice"}, Body: "LGTM\n\nand a second paragraph", CreatedAt: metaAt()},
	}
	return it
}

// TestEveryCommentLineCarriesTheBar is the whole point of the bar: a comment
// that runs past one line has to stay visibly one comment.
func TestEveryCommentLineCarriesTheBar(t *testing.T) {
	lines := commentLines(withComments().Comments[1], 40)

	if len(lines) < 3 {
		t.Fatalf("the comment came out as %d lines, want the header and two paragraphs", len(lines))
	}
	for _, l := range lines {
		if !strings.HasPrefix(ansi.Strip(l), icon.CommentBar()) {
			t.Errorf("line %q does not start with the comment bar", ansi.Strip(l))
		}
	}
}

// TestTheBodyDoesNotRepeatTheTitle covers the H1 that used to be generated:
// the title now has a line of its own above the panes.
func TestTheBodyDoesNotRepeatTheTitle(t *testing.T) {
	text := ansi.Strip(strings.Join(bodyLines(withComments(), 60), "\n"))

	if strings.Contains(text, "a pr") {
		t.Errorf("the body repeats the title:\n%s", text)
	}
}

func TestTheBodyNamesItsSections(t *testing.T) {
	text := ansi.Strip(strings.Join(bodyLines(withComments(), 60), "\n"))

	for _, want := range []string{"Description", "2 comments", "This replaces the renderer."} {
		if !strings.Contains(text, want) {
			t.Errorf("the body is missing %q:\n%s", want, text)
		}
	}
}

// TestAnItemWithNoCommentsHasNoCommentHeading keeps an empty heading off the
// screen.
func TestAnItemWithNoCommentsHasNoCommentHeading(t *testing.T) {
	it := fullPRItem()
	it.Body = "just the description"

	text := ansi.Strip(strings.Join(bodyLines(it, 60), "\n"))

	if strings.Contains(text, "comment") {
		t.Errorf("a heading was drawn for comments that do not exist:\n%s", text)
	}
}

func TestAnEmptyDescriptionSaysSo(t *testing.T) {
	it := fullPRItem()
	it.Body = "   "

	text := ansi.Strip(strings.Join(bodyLines(it, 60), "\n"))

	if !strings.Contains(text, "no description") {
		t.Errorf("an empty description was drawn as nothing:\n%s", text)
	}
}

func TestTheBodyFitsItsWidth(t *testing.T) {
	for _, l := range bodyLines(withComments(), 40) {
		if w := ansi.StringWidth(l); w > 40 {
			t.Errorf("line %q is %d columns wide, want at most 40", ansi.Strip(l), w)
		}
	}
}
```

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/app/presentation/tui/detail/ -run 'TestEveryCommentLineCarriesTheBar|TestTheBody|TestAnItemWithNoComments|TestAnEmptyDescription' -v`
Expected: FAIL — `undefined: commentLines` / `bodyLines`

- [ ] **Step 3: `body.go` を書く**

```go
package detail

import (
	"strings"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/styles"
	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/app/presentation/tui/icon"
	"github.com/kukv/octoscope/internal/app/presentation/tui/layout"
	"github.com/kukv/octoscope/internal/app/presentation/tui/theme"
	"github.com/kukv/octoscope/internal/app/usecase"
	"github.com/kukv/octoscope/internal/i18n"
)

// bodyIndent is how far the description sits in from the section heading. It
// replaces the margin glamour would otherwise add, which is turned off in
// markdownLines so that a comment's bar has nothing between it and the text.
const bodyIndent = "  "

// bodyLines is what scrolls: the description under its heading, then every
// comment behind its own bar.
func bodyLines(it usecase.Item, w int) []string {
	lines := []string{theme.Heading().Render(i18n.T("detail.section.description"))}

	body := it.Body
	if strings.TrimSpace(body) == "" {
		body = i18n.T("detail.no_description")
	}
	for _, l := range markdownLines(body, max(w-len(bodyIndent), 1)) {
		lines = append(lines, bodyIndent+l)
	}

	if len(it.Comments) == 0 {
		return lines
	}
	lines = append(lines, "",
		theme.Heading().Render(i18n.Tn("detail.section.comments", len(it.Comments))))
	for _, c := range it.Comments {
		lines = append(lines, "")
		lines = append(lines, commentLines(c, w)...)
	}
	return fit(lines, w)
}

// fit cuts every line to the pane's width. glamour wraps prose, but a long
// URL or a line of code has nowhere to wrap, and one line past the edge
// pushes the rule between the panes sideways for its whole height.
func fit(lines []string, w int) []string {
	for i, l := range lines {
		lines[i] = layout.Clip(l, w)
	}
	return lines
}

// commentLines draws one comment: who wrote it and when, then the body, with
// a bar down the left of every line. The bar is what says where one comment
// ends and the next begins, so it cannot be left off a wrapped line.
func commentLines(c domain.Comment, w int) []string {
	bar := theme.Dim().Render(icon.CommentBar() + " ")
	lines := []string{bar + theme.Dim().Render("@"+c.Author.Login+" · "+i18n.DateTime(c.CreatedAt))}
	for _, l := range markdownLines(c.Body, max(w-2, 1)) {
		lines = append(lines, bar+l)
	}
	return lines
}

// markdownLines renders one block of GitHub markdown to w columns.
//
// The document margin is turned off: glamour would indent every line by two
// columns, which inside a comment would open a gap between the bar and the
// text. The blank lines glamour puts around a document are dropped for the
// same reason — a bar with nothing beside it reads as a break in the comment.
func markdownLines(src string, w int) []string {
	// A GitHub body carries whatever line endings its author used. A stray
	// carriage return inside a drawn line sends the cursor back to the start
	// of it, which shifts everything after it sideways.
	src = strings.ReplaceAll(src, "\r", "")

	out := src
	cfg := styles.DarkStyleConfig
	var noMargin uint
	cfg.Document.Margin = &noMargin
	if r, err := glamour.NewTermRenderer(glamour.WithStyles(cfg),
		glamour.WithWordWrap(max(w, 1))); err == nil {
		if rendered, err := r.Render(src); err == nil {
			out = rendered
		}
	}
	return trimBlankEdges(strings.Split(out, "\n"))
}

// trimBlankEdges drops the blank lines at either end of a rendered block.
func trimBlankEdges(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(ansi.Strip(lines[0])) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(ansi.Strip(lines[len(lines)-1])) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
```

- [ ] **Step 4: 通ることを確かめる**

Run: `go test ./internal/app/presentation/tui/detail/ -run 'TestEveryCommentLineCarriesTheBar|TestTheBody|TestAnItemWithNoComments|TestAnEmptyDescription' -v`
Expected: PASS

- [ ] **Step 5: `make check`**

Run: `make check`
Expected: 緑

- [ ] **Step 6: コミット**

```bash
git add internal/app/presentation/tui/detail/body.go internal/app/presentation/tui/detail/body_test.go
git commit -m "feat(detail): draw the description and each comment as its own block

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: View を 2 ペイン / 1 カラムに組み替える

ここで画面が変わる。ゴールデンは Task 6 で記録し直すので、このタスクの終わりでは
ゴールデンテストが**落ちたままでよい**（それ以外は緑であること）。

**Files:**
- Modify: `internal/app/presentation/tui/detail/render.go`
- Modify: `internal/app/presentation/tui/detail/detail.go`
- Test: `internal/app/presentation/tui/detail/render_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`render_test.go` の末尾に足す。

```go
// TestTheViewSplitsInTwoWhenItCan covers the threshold: the rule between the
// panes is what tells the two layouts apart.
func TestTheViewSplitsInTwoWhenItCan(t *testing.T) {
	f := &fakeSource{pr: domain.PR{
		Number: 12, Title: "a pr", Author: domain.Author{Login: "kukv"},
		State: domain.StateOpen, Body: "the description",
	}}

	for _, tc := range []struct {
		name  string
		width int
		split bool
	}{
		{"a wide terminal splits", 120, true},
		{"the threshold itself splits", 100, true},
		{"one column short does not", 99, false},
		{"eighty does not", 80, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := loaded(f, domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/octoscope", Number: 12})
			m, _ = m.Update(tea.WindowSizeMsg{Width: tc.width, Height: 24})

			got := strings.Contains(ansi.Strip(m.View()), "repository  kukv/octoscope #12")
			if got != tc.split {
				t.Errorf("split = %v at %d columns, want %v:\n%s",
					got, tc.width, tc.split, ansi.Strip(m.View()))
			}
		})
	}
}

// TestTheSingleColumnKeepsEveryFact is why the narrow layout joins the rows
// rather than dropping them.
func TestTheSingleColumnKeepsEveryFact(t *testing.T) {
	pr := domain.PR{
		Number: 12, Title: "a pr", Author: domain.Author{Login: "kukv"},
		State: domain.StateOpen, Review: domain.ReviewApproved, Body: "the description",
		Assignees: []domain.Author{{Login: "alice"}},
		Checks:    domain.Checks{Total: 2, Passed: 1, Failed: 1},
		Head:      "feat/x", Base: "main", Additions: 218, Deletions: 31,
	}
	m := loaded(&fakeSource{pr: pr}, domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/octoscope", Number: 12})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	view := ansi.Strip(m.View())
	for _, want := range []string{"kukv/octoscope #12", "@kukv", "feat/x → main", "+218", "−31", "@alice"} {
		if !strings.Contains(view, want) {
			t.Errorf("the single-column header dropped %q:\n%s", want, view)
		}
	}
}

// TestNoLineOverrunsTheTerminal is the check a two-pane layout most easily
// fails, and the one a Japanese terminal fails first.
func TestNoLineOverrunsTheTerminal(t *testing.T) {
	for _, w := range []int{80, 100, 120, 160} {
		m := goldenModel(w)
		for _, l := range strings.Split(m.View(), "\n") {
			if got := ansi.StringWidth(l); got > w {
				t.Errorf("at %d columns a line is %d wide: %q", w, got, ansi.Strip(l))
			}
		}
	}
}
```

`render_test.go` の import に `tea "charm.land/bubbletea/v2"`、`"strings"`、
`"github.com/charmbracelet/x/ansi"`、`"github.com/kukv/octoscope/internal/app/domain"`
が無ければ足す。

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/app/presentation/tui/detail/ -run 'TestTheViewSplitsInTwoWhenItCan|TestTheSingleColumnKeepsEveryFact|TestNoLineOverrunsTheTerminal' -v`
Expected: FAIL — 画面はまだ 1 本の Markdown で、`repository  kukv/octoscope #12` も
`feat/x → main` も出ていない

- [ ] **Step 3: しきい値と幅の計算を `render.go` に足す**

`render.go` の import の下に足す。

```go
const (
	// twoPaneMinColumns is where the view splits in two. It is the Work
	// board's own threshold for its drawer, which is the same shape of
	// problem: a hundred columns leave the meta pane thirty-three and the
	// body sixty-six.
	twoPaneMinColumns = 100

	// metaPaneMaxColumns caps the meta pane. Its values are short, and past
	// forty columns the width would be spent on nothing.
	metaPaneMaxColumns = 40
)

func twoPane(w int) bool { return w >= twoPaneMinColumns }

// metaPaneWidth is the left pane's share. The floor is the threshold: at a
// hundred columns this is already thirty-three.
func metaPaneWidth(w int) int { return min(w/3, metaPaneMaxColumns) }
```

- [ ] **Step 4: `View` の modeView の枝を書き換える**

`render.go:36-43` の 4 行（`header := ...` から `return body + ...` まで）を
次で置き換える。

```go
	header := layout.ClipLines(theme.Title().Render(m.title), m.width)
	footer := layout.ClipLines(theme.Dim().Render(m.footer()), m.width)

	var b strings.Builder
	b.WriteString(header + "\n")
	if twoPane(m.width) {
		left := metaPaneLines(m.rows(), metaPaneWidth(m.width))
		right := strings.Split(m.body.View(), "\n")
		b.WriteString(strings.Join(layout.JoinPanes(left, right, metaPaneWidth(m.width)), "\n") + "\n")
	} else {
		for _, l := range m.headerLines() {
			b.WriteString(l + "\n")
		}
		b.WriteString(theme.Rule().Render(strings.Repeat("─", max(m.width, 0))) + "\n")
		b.WriteString(m.body.View() + "\n")
	}
	if m.errText != "" {
		b.WriteString(wrapErr(m.errText, m.width) + "\n")
	}
	return b.String() + footer
```

同じファイルに足す。

```go
// rows is the meta pane's content. It is worked out at draw time rather than
// kept, because its layout follows the terminal's width.
func (m Model) rows() []metaRow {
	if !m.loaded {
		return nil
	}
	return metaRows(m.ref, m.item)
}

// headerLines is the meta block of the single-column layout: the same rows,
// joined into one wrapped paragraph above the body.
func (m Model) headerLines() []string {
	if !m.loaded {
		return nil
	}
	return metaInlineLines(m.rows(), m.width)
}
```

- [ ] **Step 5: 寸法計算を新しいレイアウトに合わせる**

`detail.go` の `relayout`（Task 1 で作ったもの）の body の 2 行を置き換える。

```go
	bodyW, bodyH := w, h-chromeLines
	if twoPane(w) {
		bodyW = w - metaPaneWidth(w) - 1 // the rule JoinPanes draws between them
	} else {
		bodyH -= len(m.headerLines()) + 1 // the paragraph and its rule
	}
	m.body.SetWidth(max(bodyW, 1))
	m.body.SetHeight(max(bodyH, 5))
	m.setBodyContent(max(bodyW, 1))
```

`relayout` は `m.headerLines()` を読むので、**`m.width` を先に代入してから**呼ぶこと
（`resize` はすでにそうなっている）。

- [ ] **Step 6: `setBodyContent` を新しい描画に差し替える**

`detail.go` の `setBodyContent` を置き換える。glamour の呼び出しは `body.go` に移った。

```go
func (m *Model) setBodyContent(w int) {
	if !m.loaded {
		return
	}
	at := m.body.YOffset()
	m.body.SetContentLines(bodyLines(m.item, w))
	m.body.SetYOffset(at)
}
```

`detail.go` の import から `"charm.land/glamour/v2"` を外す（使うのは `body.go` だけ）。

- [ ] **Step 7: 罫がペインの高さいっぱいに走るようにする**

`detail.go` の `newBody` に 1 行足す。viewport が足りない行を空行で埋めないと、
`JoinPanes` の縦罫が本文の終わりで途切れる。

```go
func newBody() viewport.Model {
	v := viewport.New(viewport.WithWidth(80), viewport.WithHeight(20))
	v.MouseWheelEnabled = true
	// The rule between the panes runs as far as the body does, so the body
	// has to be as tall as the pane even when the item is short.
	v.FillHeight = true
	return v
}
```

- [ ] **Step 8: 古い Markdown の組み立てを消す**

`render.go` から次の 5 つを削除する。どこからも呼ばれなくなる。

- `prMarkdown`
- `issueMarkdown`
- `writeCommonMeta`
- `writeBody`
- `writeComments`

削除後、`render.go` の import から `"fmt"`、`"time"`、
`"github.com/kukv/octoscope/internal/app/usecase"` が未使用になっていれば外す
(`make fmt` では消えない。`make lint` が教える)。

- [ ] **Step 9: 新しいテストが通ることを確かめる**

Run: `go test ./internal/app/presentation/tui/detail/ -run 'TestTheViewSplitsInTwoWhenItCan|TestTheSingleColumnKeepsEveryFact|TestNoLineOverrunsTheTerminal' -v`
Expected: PASS

- [ ] **Step 10: ゴールデン以外が通ることを確かめる**

Run: `go test ./internal/app/presentation/tui/detail/ -v`
Expected: `TestGolden` だけが FAIL。他は PASS。
（`TestGolden` は Task 6 で記録し直す）

- [ ] **Step 11: コミット**

```bash
git add internal/app/presentation/tui/detail/render.go internal/app/presentation/tui/detail/detail.go internal/app/presentation/tui/detail/render_test.go
git commit -m "feat(detail): put the meta beside the body instead of inside it

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: ゴールデンの fixture を太らせ、記録し直す

**Files:**
- Modify: `internal/app/presentation/tui/detail/golden_test.go`
- Modify: `internal/app/presentation/tui/detail/testdata/*.golden`

- [ ] **Step 1: fixture に足りないものを入れる**

今の `goldenPR()` は `Head` `Base` `Additions` `Deletions` `Assignees` `Checks` が
すべて空で、新しい行が 1 行も記録されない。`golden_test.go` の `goldenPR` を置き換える。

```go
func goldenPR() domain.PR {
	return domain.PR{
		Number: 12, Title: "レンダリングのパイプラインを置き換える",
		Author: domain.Author{Login: "kukv"}, State: domain.StateOpen,
		Review: domain.ReviewApproved, UpdatedAt: goldenAt,
		Body:   "This replaces the renderer.\n\n- one\n- two",
		Labels: []domain.Label{{Name: "enhancement", Color: "a2eeef"}},
		// Everything the meta pane can draw is filled in: a recording made
		// against an item with no branches, no checks and no assignee would
		// never show those rows at all.
		Assignees: []domain.Author{{Login: "alice"}},
		Checks:    domain.Checks{Total: 3, Passed: 1, Failed: 1, Running: 1},
		Head:      "feat/graph", Base: "main", Additions: 218, Deletions: 31,
		Comments: []domain.Comment{
			{Author: domain.Author{Login: "bob"}, Body: "見た目が良い", CreatedAt: goldenAt},
			// A second comment is what shows the bar starting over.
			{
				Author: domain.Author{Login: "alice"},
				Body:   "ここは次の PR で直すつもり。\n\n長さのある段落をもう一つ置いて、折り返した行にも罫がつくことを記録に残す。",
				CreatedAt: goldenAt,
			},
		},
	}
}
```

- [ ] **Step 2: ゴールデンのモデルにリポジトリ名を持たせる**

`prRef()` の `Repo` は空で、リポジトリ行が出ない。`golden_test.go` に足す。

```go
// goldenRef names a repository, unlike prRef: the meta pane's first row is
// the reference, and an empty repository would leave it out of every
// recording.
func goldenRef() domain.ItemRef {
	return domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/octoscope", Number: 12}
}
```

`goldenModel` の中の `prRef()` を 2 か所とも `goldenRef()` に替える
（`New(f, prRef())` と `fetch(f, prRef())()`）。`TestGolden` の中の
`New(&fakeSource{pr: goldenPR()}, prRef())` も `goldenRef()` に替える。

- [ ] **Step 3: 記録し直す**

Run: `make golden`
Expected: `testdata/detail_*.golden` が書き換わる

- [ ] **Step 4: diff を読む**

Run: `git diff --stat internal/app/presentation/tui/detail/testdata/`

次を目で確かめる。**確かめずにコミットしない。**

```bash
sed 's/\x1b\[[0-9;]*m//g' internal/app/presentation/tui/detail/testdata/detail_ja_120.golden
sed 's/\x1b\[[0-9;]*m//g' internal/app/presentation/tui/detail/testdata/detail_ja_80.golden
sed 's/\x1b\[[0-9;]*m//g' internal/app/presentation/tui/detail/testdata/detail_en_160.golden
```

- `detail_ja_120` と `detail_en_160` は 2 ペイン。左にラベルと値、縦罫が本文の高さまで走る
- `detail_ja_80` は 1 カラム。メタが ` · ` で連結されて折り返り、その下に全幅の罫
- タイトルが 2 回出ていない
- コメントが 2 つとも `▌` で始まり、折り返した行にも `▌` がある
- 日本語の行が 120 / 80 桁を超えていない

- [ ] **Step 5: 記録が通ることを確かめる**

Run: `go test ./internal/app/presentation/tui/detail/ -v`
Expected: PASS（`TestGolden` を含めて全部）

- [ ] **Step 6: `make check`**

Run: `make check`
Expected: 緑

- [ ] **Step 7: コミット**

```bash
git add internal/app/presentation/tui/detail/golden_test.go internal/app/presentation/tui/detail/testdata
git commit -m "test(detail): record the redesigned view against a full item

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 7: 使われなくなったカタログのキーを消す

**Files:**
- Modify: `internal/i18n/locales/active.ja.yaml`
- Modify: `internal/i18n/locales/active.en.yaml`

- [ ] **Step 1: 参照が無いことを確かめる**

Run: `grep -rn '"md\.' --include='*.go' internal cmd`
Expected: 出力なし（Task 5 で `writeBody` ごと消えている）

出力があれば、それを消してから先に進む。

- [ ] **Step 2: 両方のカタログから `md:` ブロックを消す**

`active.ja.yaml`（446 行目付近）と `active.en.yaml`（460 行目付近）の `md:` ブロックを、
`author` から `no_description` まで丸ごと削除する。中身は
`detail.meta.*` / `detail.no_description` / `state.draft_suffix` に移っている。

- [ ] **Step 3: 通ることを確かめる**

Run: `go test ./internal/i18n/... ./internal/app/presentation/tui/...`
Expected: PASS（`TestCatalogsHaveTheSameIDs` を含む）

- [ ] **Step 4: `make check`**

Run: `make check`
Expected: 緑

- [ ] **Step 5: 目で見る（`.claude/rules/tui.md`）**

テストが通っただけでは完了にしない。次の 4 つを実際に起動して見る。

```bash
go run ./cmd/octoscope --repo kukv/octoscope            # 広い端末
go run ./cmd/octoscope --repo kukv/octoscope --lang ja  # 同じものを日本語で
```

端末を 80 桁に狭めて同じことをする。見るのは次の 4 点。

1. 2 ペインで左の値が切れすぎていないか（ブランチ名、日時）
2. 80 桁でメタの段落が崩れず、本文が読める高さを保っているか
3. コメントの罫が途切れず、コメントどうしの境界が分かるか
4. Issue を開いたとき（`i` で一覧から）、レビュー / checks / ブランチ / 変更の
   4 行が出ていないか

- [ ] **Step 6: コミット**

```bash
git add internal/i18n/locales/active.ja.yaml internal/i18n/locales/active.en.yaml
git commit -m "i18n: drop the markdown strings the detail view no longer builds

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## 仕上げ

- [ ] `make check` が緑
- [ ] `git log --oneline main..` が 7 コミット（spec の 2 つを含めて 9 つ)
- [ ] spec の §5.1 が言うファイル分割になっている（`render.go` / `meta.go` / `body.go`）
- [ ] `wc -l internal/app/presentation/tui/detail/*.go` で、どのファイルも 300 行前後に
      収まっている。超えていたら責務が増えていないか疑う（`.claude/rules/architecture.md`）
