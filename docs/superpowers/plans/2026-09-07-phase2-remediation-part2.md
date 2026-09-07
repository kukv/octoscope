# Phase 2 立て直し Part 2 実装計画（作業順 9 / 6 / 7 / 8）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `detail` と `diff` の並行する bool を mode/phase の enum に畳み、`Update` の各 case を名前のある method に出し、非テストコードから実装計画・設計書への参照を消す。その前に、書き換えを守る回帰ネット（キー入力だけのシナリオテストと、未収録の golden）を現行コードの上で用意する。

**Architecture:** 表示は 1 ピクセルも変えない。`detail.Model` の bool 10 個 + エラー文字列 3 本（+ `picker.err`）を `mode` / `phase` / `errText` に、`diff.Model` の bool 5 個 + エラー文字列 3 本を同じ形に畳む。`internal/usecase` と `internal/gh` には触らない（Part 1 で終わっている）。

**Tech Stack:** Go 1.27.1 / Bubble Tea v2 (`charm.land/*/v2`) / `internal/golden`（`OCTOSCOPE_UPDATE_GOLDEN=1`）

**Spec:** `docs/superpowers/specs/2026-09-06-phase2-remediation-design.md`（§4.4 / §4.5 / §4.8 / §4.7(c) / §6 の作業順 6〜9）

**前提:** 作業順 0 / 1 / 1.5 は PR #56、作業順 2 / 5 / 3 / 4 は Part 1（`docs/superpowers/plans/2026-09-07-phase2-remediation-part1.md`）で完了済み。`internal/usecase` は既にあり、`detail.Source` は分割済み。

---

## Global Constraints

- **`make check` が通らない状態でコミットしない。** 各タスクの最後は必ず `make check`
- **表示を変えない。** 作業順 6 と 7 の検証は「golden が変わらないこと」。`git diff --stat -- '*.golden'` が空であること。golden を録り直したくなったら、それは表示を変えてしまった合図なので止めて相談する
- **`internal/gh` / `internal/gh/cli` / `internal/usecase` に触らない。** この計画は `internal/tui` と `.claude/rules` とコメントだけを扱う
- **`internal/tui` は `internal/gh/cli` を import しない**（depguard が落とす）
- **テストでネットワークも外部プロセスも叩かない**（`.claude/rules/testing.md`）
- **`//nolint` を新しく足さない**（`.claude/rules/go-style.md`）
- **コードのコメントは英語。** 画面に出す文字列は `internal/i18n` のカタログから引き、`en` と `ja` の両方に足す（この計画で新しい文字列は足さない）
- **テストは状態を直接組み立てず、`Update` にメッセージやキーを渡して到達させる**（`.claude/rules/testing.md`）
- **新しく書いたテストは、検証対象を一時的に壊して落ちることを確認してからコミットする**
- **Phase 3 以降の機能（checks / merge / Search タブ）を入れない**

---

## Decisions（この計画で確定させたこと）

**実装者はここを勝手に読み替えない。** 変えたくなったら止めて相談する。

### D1. 作業順を 9 → golden 追録 → 6 → 7 → 8 に並べ替える

設計書 §6 は 6 → 7 → 8 → 9 と並べているが、理由は書かれていない。作業順 9 の
シナリオテスト（`c` → `ctrl+s` / `x` → `y` / `l` → `space` → `enter`）は、
作業順 6 が書き換えるまさにその遷移を通る。**現行コードの上で先に書けば回帰ネットになる。**

**2026-09-07 に利用者の承認を得た。** Task 8 Step 4 で設計書 §6 に並べ替えを追記する。

（選ばなかった案: 設計書どおり 6 → 9。enum 化の間、シナリオの担保が無い。）

### D2. golden を enum 化の前に追録する

現在の golden は `detail` が既定 / confirm / submit の 3 状態、`diff` が既定 1 状態しかない。
「golden 不変で等価性を確認する」は、**録れているセルにしか効かない**。
作業順 6 の前に、未収録のセルを現行コードの出力で録る（Task 2 / Task 3）。

**2026-09-07 に利用者の承認を得た。**

あわせて `internal/tui/detail/golden_test.go:69` の `confirming.confirming = true`
（フィールド直書き）を `x` キー経由に直す。enum 化で確実に壊れるうえ、
到達不能な状態を録っていないことを示せない。

### D3. `diff.loading` は enum に畳まない

`diff` の `c` / `v` / `X` は `m.loading`（差分の取得中）ではなく
**レビューコンテキストの到着**でゲートされている（`comment.go` の `startComposing`、
`review.go` の `openSubmit` / `startDiscard`）。差分の取得中でもオーバーレイは開ける。
`loading` を `phase` に畳むと、オーバーレイを閉じた時点で「まだ取得中」を失う。

**確定:** `diff` の mode/phase は**オーバーレイにだけ**適用し、`loading` は
「本文がまだ来ていない」という別の軸として `bool` のまま残す。
`detail` は `c` / `x` / `v` / `l` / `a` が `m.loading` でゲートされているので、
設計書の表どおり `modeView` + `phaseLoading` に畳む。

**2026-09-07 に利用者の承認を得た。** 完了条件 4「bool の mode フラグが無い」は
「**並行するオーバーレイのフラグ**が無い」と読む。`diff.loading` / `sidebar` /
`expanded` / `work` と `repo` の `loading` は mode フラグではないので対象外。

### D4. 送信中の `phase` は `review.Model` を鏡写しにしない

設計書 §4.4 の表は Submit + Working を「`submitting` + `submit.sending`」としているが、
`sending` は `review.Model` 自身のフィールドで、ポップアップが持つのが正しい
（`internal/tui/review/review.go:58`）。`phase` に写すと真実の出所が 2 つになる。

**確定:** ポップアップが出ている間は `mode == modeSubmit` / `phase == phaseIdle` とし、
送信中かどうかは `review.Model` に任せる。`detail` の `modeSubmit` + `phaseLoading` は
`v` を押してからレビューコンテキストが届くまで（旧 `openingReview`）だけに使う。

### D5. エラー文字列は 1 本。`picker.err` も畳む

設計書 §4.4 は `postErr` / `actionErr` / `submitErr` の 3 本を 1 本にするとしているが、
`picker` は自分の中に 4 本目（`internal/tui/detail/picker.go:31`）を持っている。
どこに描くかは `mode` が決めるので、これも `Model.errText` に畳む。
`picker.listView` は err を引数で受け取る形にする。

安全性の確認: 現行コードで mode に入る全ての入口が、入る直前にその mode の
エラー文字列を空にしている（`c` は `postErr`、`x` / `l` / `a` / `v` は `actionErr`）。
よって 1 本にしても、別の mode の古いエラーが残って描かれることはない。

### D6. `diff` の `declined` と `reviewErr` は畳まない

どちらもフッター行に出る注記で、mode のエラーではない。
`declined` は「`c` / `v` / `X` が何もしなかった理由」、`reviewErr` は
「レビューコンテキストの取得が失敗した」で、**どの mode でも下敷きの画面に出る**。
`errText` に混ぜると、コメント送信の失敗が消えたあとも取得失敗が消えてしまう。

### D7. enum 化で変わる 2 つの挙動（意図的、golden では見えない）

| 場所 | 現行 | enum 後 | 理由 |
|---|---|---|---|
| `detail` の `tea.MouseWheelMsg` | `composing / confirming / picking / loading / submitting` の 5 つでだけ止まる。`pickerLoading` と `openingReview` では本文が動く | `mode != modeView \|\| phase != phaseIdle` で止まる | `render.go:30` は `pickerLoading` / `openingReview` でも本文を描いていない。見えていない本文をホイールで動かしていた |
| `detail` の `pickErrorMsg` | `m.picking` で分岐 | `phase == phaseWorking`（適用の失敗）か `phaseLoading`（候補取得の失敗）かで分岐 | 同じ分岐を状態の名前で書き直したもの。振る舞いは変わらない |

1 つ目は**振る舞いの変更**である。Task 4 のテストで明示的に固定する。

---

## ファイル構成

**新規**

| ファイル | 責務 |
|---|---|
| `internal/tui/app/scenario_test.go` | キー入力だけで画面をまたぐシナリオテスト 3 本と、状態が変わるフェイク |
| `internal/tui/detail/testdata/detail_{compose,picker,picker_loading,loading,error}_*.golden` | 未収録セルの録り |
| `internal/tui/diff/testdata/diff_{compose,submit,discard,loading}_*.golden` | 同上 |

**変更**

| ファイル | 変更 |
|---|---|
| `internal/tui/detail/detail.go` | bool 10 個 → `mode` / `phase` / `errText`。`Update` の case を名前つき method へ |
| `internal/tui/detail/render.go` | `View` / `pickerView` / `confirmView` / `composeView` の分岐を mode/phase へ |
| `internal/tui/detail/picker.go` | `err` フィールドを外し、`listView` が err を引数で受ける |
| `internal/tui/detail/golden_test.go` | フィールド直書きをキー経由へ。未収録セルを追加 |
| `internal/tui/detail/detail_test.go` / `picker_test.go` | bool を見ているアサーションを mode/phase へ |
| `internal/tui/diff/diff.go` | bool 5 個 → `mode` / `phase` / `errText`。`Update` の case を名前つき method へ |
| `internal/tui/diff/render.go` / `comment.go` / `review.go` / `mouse.go` | 同上の追随 |
| `internal/tui/diff/golden_test.go` / `*_test.go` | 同上 |
| `internal/tui/app/app.go` | `Update` の case を名前つき method へ |
| `internal/tui/**` / `internal/gh/review.go`（非テスト） | `spec N` / `Task N` / `Phase N` の参照 27 箇所を書き換えまたは削除 |
| `.claude/rules/tui.md` / `go-style.md` | `TRANSIENT` 注記の削除 |
| `docs/superpowers/specs/2026-09-06-phase2-remediation-design.md` | §6 に D1 の並べ替えを追記 |

---

## Task 1: 画面をまたぐシナリオテスト（作業順 9）

**Files:**
- Create: `internal/tui/app/scenario_test.go`
- Reference: `internal/tui/app/app_test.go:29-96`（既存の `fakeSource`）、`:104-140`（`key` ヘルパー）

**Interfaces:**
- Consumes: `app.New` / `app.Model.Update` と、`app_test.go` の既存ヘルパー
  `key` / `pressCmd` / `resolve` / `content`
- Produces: `scenarioSource`（状態が変わるフェイク）、`scenarioModel` / `run`。
  以降のタスクはこれを壊さない

既存の `fakeSource` は変更しない。ミューテーションを反映しない（`SetState` が `nil` を
返すだけ）ので、シナリオの「状態が変わる」を見られない。**隣に別のフェイクを足す。**

- [ ] **Step 1: 状態が変わるフェイクを書く**

`internal/tui/app/scenario_test.go` を作る。

```go
package app

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/text/language"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/i18n"
	"github.com/kukv/octoscope/internal/usecase"
)

// scenarioSource answers like the real thing does: a change lands, and the
// next fetch sees it. app_test.go's fakeSource returns fixed values, which is
// enough for routing but cannot show a scenario reaching its end.
type scenarioSource struct {
	pr      gh.PR
	files   []gh.FileDiff
	labels  []gh.Label
	threads []gh.ReviewThread

	pendingID string
	posted    []gh.PendingComment
}

func (f *scenarioSource) ListWork(context.Context) (gh.Work, error) {
	var w gh.Work
	w[gh.SectionReviewRequested] = []gh.WorkItem{{
		Kind: gh.ItemPR, Number: f.pr.Number, Title: f.pr.Title,
		Author: f.pr.Author, State: f.pr.State, UpdatedAt: f.pr.UpdatedAt,
	}}
	return w, nil
}

func (f *scenarioSource) ListPRs(context.Context) ([]gh.PR, error)       { return []gh.PR{f.pr}, nil }
func (f *scenarioSource) ListIssues(context.Context) ([]gh.Issue, error) { return nil, nil }
func (f *scenarioSource) RepoName(context.Context) (string, error)       { return "kukv/demo", nil }

func (f *scenarioSource) GetItem(context.Context, gh.ItemRef) (usecase.Item, error) {
	pr := f.pr
	return usecase.Item{
		Kind: gh.ItemPR, Number: pr.Number, Title: pr.Title, Author: pr.Author,
		State: pr.State, Body: pr.Body, URL: pr.URL, Labels: pr.Labels,
		Assignees: pr.Assignees, Comments: pr.Comments, UpdatedAt: pr.UpdatedAt,
		PR: &pr,
	}, nil
}

func (f *scenarioSource) OpenWeb(string) error { return nil }

func (f *scenarioSource) AddComment(_ gh.ItemRef, body string) error {
	f.pr.Comments = append(f.pr.Comments, gh.Comment{
		Author: gh.Author{Login: "kukv"}, Body: body, CreatedAt: scenarioAt,
	})
	return nil
}

func (f *scenarioSource) SetState(_ gh.ItemRef, closing bool) error {
	if closing {
		f.pr.State = gh.StateClosed
	} else {
		f.pr.State = gh.StateOpen
	}
	return nil
}

func (f *scenarioSource) EditLabels(_ gh.ItemRef, add, remove []string) error {
	for _, name := range add {
		f.pr.Labels = append(f.pr.Labels, gh.Label{Name: name})
	}
	for _, name := range remove {
		kept := f.pr.Labels[:0]
		for _, l := range f.pr.Labels {
			if l.Name != name {
				kept = append(kept, l)
			}
		}
		f.pr.Labels = kept
	}
	return nil
}

func (f *scenarioSource) EditAssignees(gh.ItemRef, []string, []string) error { return nil }

func (f *scenarioSource) ListLabels(context.Context, string) ([]gh.Label, error) {
	return f.labels, nil
}
func (f *scenarioSource) ListAssignees(context.Context, string) ([]string, error) { return nil, nil }

func (f *scenarioSource) PRDiff(context.Context, string, int) ([]gh.FileDiff, error) {
	return f.files, nil
}

func (f *scenarioSource) PRReviewContext(context.Context, string, int) (gh.ReviewContext, error) {
	return gh.ReviewContext{
		PullRequestID: "PR_1", PendingID: f.pendingID, Threads: f.threads,
	}, nil
}

// PostLineComment starts the pending review the first time, the way the
// usecase does, and the thread it created shows up in the next fetch.
func (f *scenarioSource) PostLineComment(_ usecase.ReviewTarget, c gh.PendingComment) (string, error) {
	f.posted = append(f.posted, c)
	f.pendingID = "PRR_1"
	f.threads = append(f.threads, gh.ReviewThread{
		Path: c.Path, Line: c.Line, Side: c.Side, Pending: true,
		Comments: []gh.ThreadComment{{Author: gh.Author{Login: "kukv"}, Body: c.Body}},
	})
	return f.pendingID, nil
}

func (f *scenarioSource) DiscardReview(string) error { return nil }

func (f *scenarioSource) SubmitReview(usecase.ReviewTarget, gh.ReviewEvent, string) error {
	return nil
}

var scenarioAt = time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)

func scenarioPR() gh.PR {
	return gh.PR{
		Number: 12, Title: "replace the renderer",
		Author: gh.Author{Login: "kukv"}, State: gh.StateOpen,
		UpdatedAt: scenarioAt, Body: "body",
	}
}
```

`gh.ReviewThread` / `gh.ThreadComment` / `gh.PendingComment` / `gh.FileDiff` /
`gh.WorkItem` / `gh.SectionReviewRequested` のフィールド名と綴りは
`internal/gh/*.go` で確かめてから書く。合わないときは**この計画のコードではなく
実際の型に合わせる**。

- [ ] **Step 2: シナリオを走らせるヘルパーを書く（既存のものを使う）**

`internal/tui/app/app_test.go` には**既に**次の 4 つがある。同じパッケージなので
そのまま使える。**同じものを書き直さない。**

| ヘルパー | 何をするか |
|---|---|
| `key(string) tea.KeyPressMsg`（`:107`） | キー 1 つ。1 文字は `Code` と `Text` の両方に入る |
| `pressCmd(m, k) (Model, tea.Cmd)`（`:133`） | キーを 1 つ押し、返ってきたコマンドも受け取る |
| `resolve(t, m, cmd) Model`（`:141`） | コマンドを走らせて答えを戻す。**`tea.BatchMsg` を開き、`spinner.TickMsg` はそこで止める**（ティックは自分を無限に再スケジュールする） |
| `content(m) string`（`:167`） | `m.View().Content` から ANSI を剥がしたもの。**`m.View()` は文字列ではない** |

足すのは、キー列をまとめて押す 1 つだけ。

```go
// run presses each key and lets whatever it started finish, so a scenario
// reads as the keys a user types. resolve is what makes that safe: it opens
// the batches the fetches come wrapped in and stops at the spinner's tick.
func run(t *testing.T, m Model, keys ...string) Model {
	t.Helper()
	for _, k := range keys {
		next, cmd := pressCmd(m, k)
		m = resolve(t, next, cmd)
	}
	return m
}

// scenarioModel drives the root the way the terminal does: a size first, and
// then everything the first size started.
func scenarioModel(t *testing.T, f *scenarioSource) Model {
	t.Helper()
	i18n.SetLanguage(language.English)
	t.Cleanup(func() { i18n.SetLanguage(language.English) })

	m := New(f, Options{HasRepo: true})
	next, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return resolve(t, next.(Model), cmd)
}
```

- [ ] **Step 3: シナリオ 1 — Repos で `2` → `enter` → 詳細で `x` → `y`**

タブの切り替えは `tab` キーではなく `2`（`internal/tui/app/app.go:455`）。
設計書 §4.7(c) と `.claude/rules/testing.md` の「Repos で tab →」はキーの綴りが
古い。**実装に合わせる。**

```go
// TestClosingFromTheReposTabShowsTheNewState walks the keys a user actually
// presses: switch to Repos, open the row, ask to close it, confirm. Nothing
// but the key sequence drives it, so the list, the detail view and the root
// all have to agree for this to pass.
func TestClosingFromTheReposTabShowsTheNewState(t *testing.T) {
	f := &scenarioSource{pr: scenarioPR()}
	m := scenarioModel(t, f)

	m = run(t, m, "2", "enter") // the Repos row -> the detail view
	if !strings.Contains(content(m), "#12") {
		t.Fatalf("the detail view did not open:\n%s", content(m))
	}

	m = run(t, m, "x", "y")

	if f.pr.State != gh.StateClosed {
		t.Fatalf("state = %v, want closed", f.pr.State)
	}
	if got := content(m); !strings.Contains(got, "Closed") {
		t.Errorf("the view does not show the new state:\n%s", got)
	}
}
```

`"Closed"` の綴りは英語カタログ（`internal/i18n` の `md.state` / `state.closed`）の
値に合わせる。**当てずっぽうで書かない。**

カーソルが 1 行目に無い場合は `"j"` を足す。**`content(m)` を見て決める。**

- [ ] **Step 4: シナリオ 1 が通ることを確認**

```bash
go test ./internal/tui/app/ -run TestClosingFromTheReposTabShowsTheNewState -v
```

Expected: PASS。落ちる場合は**フェイクかキー列が実物と違う**ので、
`t.Log(content(m))` を挟んで、どの画面で止まっているかを見る。

- [ ] **Step 5: シナリオ 1 の空振りを確認**

`internal/tui/detail/detail.go` の `handleConfirmKey` の `case "y":` を
`return m, nil` に一時的に書き換える。

```bash
go test ./internal/tui/app/ -run TestClosingFromTheReposTabShowsTheNewState
```

Expected: FAIL（`state = open, want closed`）。確認したら書き換えを戻す。

- [ ] **Step 6: シナリオ 2 — 詳細で `l` → `space` → `enter` でラベルが変わる**

```go
// TestPickingALabelFromTheDetailViewAppliesIt covers the picker end to end:
// the candidates are fetched, one is toggled, enter applies it, and the
// refetch that follows is what puts it on screen.
func TestPickingALabelFromTheDetailViewAppliesIt(t *testing.T) {
	f := &scenarioSource{pr: scenarioPR(), labels: []gh.Label{{Name: "bug", Color: "d73a4a"}}}
	m := scenarioModel(t, f)

	m = run(t, m, "2", "enter", "l", "space", "enter")

	if len(f.pr.Labels) != 1 || f.pr.Labels[0].Name != "bug" {
		t.Fatalf("labels = %+v, want bug applied", f.pr.Labels)
	}
	if got := content(m); !strings.Contains(got, "bug") {
		t.Errorf("the view does not show the new label:\n%s", got)
	}
}
```

- [ ] **Step 7: シナリオ 2 が通ること・空振りしないことを確認**

```bash
go test ./internal/tui/app/ -run TestPickingALabelFromTheDetailViewAppliesIt -v
```

Expected: PASS。空振り確認は `internal/tui/detail/picker.go` の `toggle()` の中身を
一時的に空にして、`labels = [], want bug applied` で落ちることを見る。

- [ ] **Step 8: シナリオ 3 — 詳細 → `d` → diff → `c` → 入力 → `ctrl+s` でスレッドが出る**

```go
// TestCommentingOnADiffLineShowsTheThread is the scenario the whole
// remediation started from: the line comment that never worked. It crosses
// three views (board, detail, diff) on key presses alone.
func TestCommentingOnADiffLineShowsTheThread(t *testing.T) {
	f := &scenarioSource{
		pr: scenarioPR(),
		files: []gh.FileDiff{{
			Path: "main.go", Status: gh.FileModified, Additions: 1,
			Lines: []gh.DiffLine{
				{Kind: gh.DiffHunk, Text: "@@ -1,1 +1,1 @@"},
				{Kind: gh.DiffAdd, Text: "+package main", NewLine: 1},
			},
		}},
	}
	m := scenarioModel(t, f)

	m = run(t, m, "2", "enter", "d") // Repos -> the detail view -> the diff
	m = run(t, m, "j")              // onto a line that can carry a comment
	// "n" is one character of body: an empty one is not sent. key() puts a
	// single character in Text, which is what the textarea inserts.
	m = run(t, m, "c", "n", "ctrl+s")

	if len(f.posted) != 1 {
		t.Fatalf("posted = %+v, want one comment", f.posted)
	}
	if f.posted[0].Path != "main.go" || f.posted[0].Line != 1 {
		t.Errorf("posted at %s:%d, want main.go:1", f.posted[0].Path, f.posted[0].Line)
	}
	// The thread row carries its author, which nothing else on this screen
	// does -- a one-character body would match anywhere.
	if got := content(m); !strings.Contains(got, "kukv") {
		t.Errorf("the view does not show the posted thread:\n%s", got)
	}
}
```

`gh.DiffLine` / `gh.FileDiff` のフィールド名と定数（`gh.DiffHunk` / `gh.DiffAdd` /
`gh.FileModified`）は `internal/gh/diff.go` で確かめる。
カーソルがコメントできる行に乗るまでに必要な `j` の回数は `content(m)` を見て合わせる。
**当てずっぽうに増やさない。**

`"kukv"` が下敷きの画面（diff のヘッダ）にも出ているなら、このアサーションは
スレッドが出ていなくても通ってしまう。Step 9 の空振り確認でそれが分かるので、
落ちなければ**スレッド行にしか出ないもの**に差し替えてから先へ進む。

- [ ] **Step 9: シナリオ 3 が通ること・空振りしないことを確認**

```bash
go test ./internal/tui/app/ -run TestCommentingOnADiffLineShowsTheThread -v
```

Expected: PASS。空振り確認は `internal/tui/diff/comment.go` の `post()` の
`return m, func() tea.Msg {...}` を `return m, nil` に一時的に書き換え、
`posted = [], want one comment` で落ちることを見る。

- [ ] **Step 10: `make check`**

```bash
make check
git diff --stat -- '*.golden'
```

Expected: 全部通る。golden の diff は空。

- [ ] **Step 11: コミット**

```bash
git add internal/tui/app/scenario_test.go
git commit -m "test: walk the board, the detail view and the diff on key presses alone"
```

コミットメッセージの末尾には次の行を足す。

```
Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
```

---

## Task 2: `detail` の未収録 golden を録る

**Files:**
- Modify: `internal/tui/detail/golden_test.go`
- Create: `internal/tui/detail/testdata/detail_{compose,picker,picker_loading,loading,error}_{en,ja}_{160,120,80}.golden`

**Interfaces:**
- Consumes: 既存の `goldenModel(width)` / `goldenPR()` / `key(string)` / `golden.Assert`
- Produces: 追録された golden。Task 4 はこれを 1 バイトも変えずに通す

- [ ] **Step 1: `goldenModel` の fake にラベル候補を持たせる**

`internal/tui/detail/golden_test.go:46` の `fakeSource` リテラルに足す。

```go
	f := &fakeSource{
		pr:        goldenPR(),
		labels:    []gh.Label{{Name: "bug", Color: "d73a4a"}},
		reviewCtx: gh.ReviewContext{PullRequestID: "PR_128", PendingID: "PRR_1"},
	}
```

`fakeSource` の候補フィールドの名前は `detail_test.go:17` の宣言で確かめる。

- [ ] **Step 2: フィールド直書きをキー経由に直し、4 つのセルを足す**

`TestGolden` の `t.Run` の中身を置き換える。

```go
				m := goldenModel(w)
				golden.Assert(t, fmt.Sprintf("detail_%s_%d", lang.name, w), m.View())

				// Every state below is reached by pressing the key that
				// opens it, not by setting the field behind it: a recording
				// of a state the keys cannot reach guards nothing.
				confirming, _ := m.Update(key("x"))
				golden.Assert(t, fmt.Sprintf("detail_confirm_%s_%d", lang.name, w), confirming.View())

				composing, _ := m.Update(key("c"))
				golden.Assert(t, fmt.Sprintf("detail_compose_%s_%d", lang.name, w), composing.View())

				opening, cmd := m.Update(key("l"))
				golden.Assert(t, fmt.Sprintf("detail_picker_loading_%s_%d", lang.name, w), opening.View())
				picking, _ := opening.Update(cmd())
				golden.Assert(t, fmt.Sprintf("detail_picker_%s_%d", lang.name, w), picking.View())

				openingReview, cmd := m.Update(key("v"))
				submitting, _ := openingReview.Update(cmd())
				golden.Assert(t, fmt.Sprintf("detail_submit_%s_%d", lang.name, w), submitting.View())

				// The first frame: nothing has arrived yet.
				loading := New(&fakeSource{pr: goldenPR()}, prRef())
				loading, _ = loading.Update(tea.WindowSizeMsg{Width: w, Height: 40})
				golden.Assert(t, fmt.Sprintf("detail_loading_%s_%d", lang.name, w), loading.View())

				// A failed action is drawn under the body, and no other
				// recording covers that line.
				failed, _ := m.Update(stateErrorMsg{err: errors.New("boom")})
				golden.Assert(t, fmt.Sprintf("detail_error_%s_%d", lang.name, w), failed.View())
```

`errors` の import を足す。

- [ ] **Step 3: 録る**

```bash
OCTOSCOPE_UPDATE_GOLDEN=1 go test ./internal/tui/detail/ -run TestGolden
```

- [ ] **Step 4: 録ったものを目で見る**

```bash
git status --short internal/tui/detail/testdata
git diff --stat -- internal/tui/detail/testdata
cat internal/tui/detail/testdata/detail_picker_ja_80.golden
cat internal/tui/detail/testdata/detail_compose_ja_80.golden
cat internal/tui/detail/testdata/detail_error_ja_80.golden
```

**既存の 18 本が 1 本も変わっていないこと。** `git diff --stat` に
`detail_en_*` / `detail_ja_*` / `detail_confirm_*` / `detail_submit_*` が出たら、
キー経由への書き換えが到達している状態を変えてしまっている。止めて原因を確かめる。

新しい golden が空だったりスピナー 1 行だけだったりしないかを見る。
`detail_picker_*` にはラベル名 `bug` が、`detail_compose_*` には入力欄が、
`detail_error_*` には `boom` が出ているはずである。

- [ ] **Step 5: 録り直しなしで通ることを確認**

```bash
go test ./internal/tui/detail/ -run TestGolden -v
```

Expected: PASS

- [ ] **Step 6: `make check` してコミット**

```bash
make check
git add internal/tui/detail/golden_test.go internal/tui/detail/testdata
git commit -m "test: record the detail view's composer, picker, first frame and error line"
```

`Co-Authored-By:` の行を足すのは Task 1 Step 11 と同じ。

---

## Task 3: `diff` の未収録 golden を録る

**Files:**
- Modify: `internal/tui/diff/golden_test.go`
- Create: `internal/tui/diff/testdata/diff_{compose,submit,discard,loading}_{en,ja}_{160,120,80}.golden`

**Interfaces:**
- Consumes: `internal/tui/diff/golden_test.go` の既存のモデル組み立て（実装前に読む）
- Produces: 追録された golden。Task 5 はこれを 1 バイトも変えずに通す

- [ ] **Step 1: 既存の golden テストを読む**

```bash
cat internal/tui/diff/golden_test.go
```

確かめること:

1. `diff_en_*` / `diff_ja_*` を組み立てているヘルパーの名前
2. そのモデルにレビューコンテキスト（`PullRequestID` と `PendingID`）が入っているか
3. カーソルがコメントできる行に乗っているか

**2 が入っていなければ `c` / `v` / `X` は `declined` を出して何も開かない。**
その場合はモデルに `reviewMsg` を渡す形をこのテストの中で足す
（既存の `diff_*` の録りは変えない — 別の変数で組み立てる）。

- [ ] **Step 2: 4 つのセルを足す**

`TestGolden` の中、既存の録りのあとに足す。**キー経由で到達させる。**

```go
				// Each overlay is opened with the key that opens it. The
				// review context has to be in place first: c, v and X all
				// decline without it.
				ready := m
				ready, _ = ready.Update(reviewMsg{ref: prRef(), ctx: gh.ReviewContext{
					PullRequestID: "PR_1", PendingID: "PRR_1",
				}})

				composing, _ := ready.Update(key("c"))
				golden.Assert(t, fmt.Sprintf("diff_compose_%s_%d", lang.name, w), composing.View())

				submitting, _ := ready.Update(key("v"))
				golden.Assert(t, fmt.Sprintf("diff_submit_%s_%d", lang.name, w), submitting.View())

				discarding, _ := ready.Update(key("X"))
				golden.Assert(t, fmt.Sprintf("diff_discard_%s_%d", lang.name, w), discarding.View())

				loading := New(f, prRef())
				loading, _ = loading.Update(tea.WindowSizeMsg{Width: w, Height: 40})
				golden.Assert(t, fmt.Sprintf("diff_loading_%s_%d", lang.name, w), loading.View())
```

`reviewMsg` / `prRef` / `f`（fake）の名前は既存のテストに合わせる。
既存のモデルが既にレビューコンテキストを持っているなら `ready := m` だけでよい。

`c` は「カーソルが行の上にある」ことも要る。`declined` の注記が録れてしまったら
（`diff_compose_*` に入力欄が無い）、`j` を足してカーソルを動かしてから `c` を押す。

- [ ] **Step 3: 録って目で見る**

```bash
OCTOSCOPE_UPDATE_GOLDEN=1 go test ./internal/tui/diff/ -run TestGolden
git diff --stat -- internal/tui/diff/testdata
cat internal/tui/diff/testdata/diff_compose_ja_80.golden
cat internal/tui/diff/testdata/diff_discard_ja_80.golden
cat internal/tui/diff/testdata/diff_submit_ja_80.golden
```

既存の 8 本が変わっていないこと。新しい 4 セルが**注記ではなく実際のオーバーレイ**を
写していることを確認する。

- [ ] **Step 4: 録り直しなしで通ることを確認 → `make check` → コミット**

```bash
go test ./internal/tui/diff/ -run TestGolden -v
make check
git add internal/tui/diff/golden_test.go internal/tui/diff/testdata
git commit -m "test: record the diff's composer, submit popup, discard prompt and first frame"
```

---

## Task 4: `detail` の bool を mode/phase に畳む（作業順 6a）

**Files:**
- Modify: `internal/tui/detail/detail.go:105-137`（`Model`）ほか全体
- Modify: `internal/tui/detail/render.go:17-38` ほか
- Modify: `internal/tui/detail/picker.go:31`、`listView`
- Test: `internal/tui/detail/detail_test.go` / `picker_test.go`

**Interfaces:**
- Produces: 非公開の `mode` / `phase` 型。`Model` は `mode mode` / `phase phase` /
  `errText string` を持つ。`picker` は `err` を持たなくなり、
  `listView(height, width int, errText string) string` になる
- 外から見える型（`Source` / `ClosedMsg` / `OpenDiffMsg` / `ErrorMsg` / `New`）は変えない

- [ ] **Step 1: D7 の挙動を固定するテストを先に書く**

`internal/tui/detail/detail_test.go` の末尾に足す。

```go
// TestTheWheelDoesNotScrollWhatTheSpinnerHides fixes the one behaviour this
// fold changes on purpose: while the picker's candidates are in flight the
// view draws a spinner, not the body, and a wheel that moved the text
// underneath was scrolling what nobody can see.
func TestTheWheelDoesNotScrollWhatTheSpinnerHides(t *testing.T) {
	f := &fakeSource{pr: goldenPR(), labels: []gh.Label{{Name: "bug"}}}
	m := loaded(f, prRef())
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = m.Update(key("l")) // the candidates are now in flight

	before := m.body.YOffset
	m, _ = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})

	if m.body.YOffset != before {
		t.Errorf("the body scrolled to %d while the fetch was in flight, want %d",
			m.body.YOffset, before)
	}
}
```

`tea.MouseWheelMsg` の組み立てと `m.body.YOffset` の綴りは、
`internal/tui/detail` に既にあるホイールのテスト（`grep -n MouseWheel
internal/tui/detail/*_test.go`）に合わせる。本文が 1 画面に収まっていて
スクロールの余地が無いと、この差は出ない。`goldenPR()` の本文が短いなら
`f.pr.Body` を 100 行にしてから測る。

- [ ] **Step 2: このテストが現行コードで落ちることを確認**

```bash
go test ./internal/tui/detail/ -run TestTheWheelDoesNotScrollWhatTheSpinnerHides -v
```

Expected: FAIL（`the body scrolled to 1 ... want 0`）。
**落ちなければテストが何も守っていない。** スクロールの余地を作ってから測り直す。
落ちない状態で先へ進まない。

- [ ] **Step 3: enum を定義する**

`internal/tui/detail/detail.go` の `type Model struct` の直前に足す。

```go
// mode is which overlay is on screen. Parallel bools made 2^n nominal states
// out of the eight this view actually has, and Update, handleKey and View
// each assumed a different subset of them (.claude/rules/tui.md).
type mode uint8

const (
	modeView    mode = iota // the body on its own
	modeCompose             // the comment composer
	modeConfirm             // the close/reopen confirmation
	modePick                // the label/assignee picker
	modeSubmit              // the review submission popup
)

// phase is where the current mode is in its own round trip.
type phase uint8

const (
	phaseIdle    phase = iota
	phaseLoading             // fetching what the mode needs to open
	phaseWorking             // sending
)
```

- [ ] **Step 4: `Model` のフィールドを畳む**

`detail.go:105-137` の 10 個の bool と 3 本のエラー文字列を置き換える。
**`spin` / `body` / `title` / `state` / `url` / `textarea` / `picker` / `labels` /
`assignees` / `submit` は残す。**

```go
	mode  mode
	phase phase

	// errText is the last failure, whatever produced it. Which mode is on
	// screen decides where it is drawn, so there is nothing to gain from
	// keeping one string per kind.
	errText string
```

`New` の `loading: true` は `phase: phaseLoading` になる（`mode` はゼロ値の `modeView`）。

- [ ] **Step 5: 対応表のとおりに全ての読み書きを置き換える**

| 旧 | 新 |
|---|---|
| `m.loading = true` | `m.mode, m.phase = modeView, phaseLoading` |
| `m.loading = false` | `m.phase = phaseIdle` |
| `if m.loading`（`stateAction` / `handleKey`） | `if m.phase == phaseLoading` |
| `m.composing = true` | `m.mode, m.phase = modeCompose, phaseIdle` |
| `m.composing = false` | `m.mode, m.phase = modeView, phaseIdle` |
| `m.posting = true` / `m.working = true` / `m.applying = true` | `m.phase = phaseWorking` |
| `m.posting = false` / `m.working = false` / `m.applying = false` | `m.phase = phaseIdle` |
| `m.confirming = true` | `m.mode, m.phase = modeConfirm, phaseIdle` |
| `m.confirming = false` | `m.mode, m.phase = modeView, phaseIdle` |
| `m.pickerLoading = true` | `m.mode, m.phase = modePick, phaseLoading` |
| `m.picking = true` | `m.phase = phaseIdle` |
| `m.picking = false` | `m.mode, m.phase = modeView, phaseIdle` |
| `m.openingReview = true` | `m.mode, m.phase = modeSubmit, phaseLoading` |
| `m.openingReview = false`（`reviewContextErrMsg`、ポップアップは開かない） | `m.mode, m.phase = modeView, phaseIdle` |
| `m.openingReview = false`（`reviewContextMsg`、ポップアップが開く） | 下の `m.submitting = true` と同じ行で `m.phase = phaseIdle` |
| `m.submitting = true` | `m.phase = phaseIdle` |
| `m.submitting = false` | `m.mode, m.phase = modeView, phaseIdle` |
| `if !m.submitting`（`review.*Msg`） | `if m.mode != modeSubmit` |
| `m.postErr` / `m.actionErr` / `m.submitErr` / `m.picker.err` | `m.errText` |

`errMsg`（本文の取得が失敗して親にエラー画面を出させる case）は
`m.loading = false` を `m.phase = phaseIdle` にするだけ。**この case だけは
コマンドを返す**ので、分割（Task 6）でもその形を保つ。

`pickErrorMsg` は D7 のとおり:

```go
	case pickErrorMsg:
		if m.phase == phaseWorking { // the apply failed; the picker stays up
			m.phase = phaseIdle
		} else { // the candidates never arrived; there is no picker to show
			m.mode, m.phase = modeView, phaseIdle
		}
		m.errText = msg.err.Error()
		return m, nil
```

- [ ] **Step 6: `handleKey` の入口を mode で書き直す**

```go
func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	// Nothing an overlay's keys could act on is on screen until what it was
	// opened for arrives. modeView is the exception: the body is drawn, and
	// q still leaves.
	if m.mode != modeView && m.phase == phaseLoading {
		return m, nil
	}
	switch m.mode {
	case modeCompose:
		return m.handleComposeKey(msg)
	case modeConfirm:
		return m.handleConfirmKey(msg)
	case modePick:
		return m.handlePickerKey(msg)
	case modeSubmit:
		return m.handleSubmitKey(msg)
	}
	switch msg.String() {
	// ... modeView's own keys, unchanged apart from the table above
	}
}
```

各ハンドラ冒頭の `if m.posting` / `if m.working` / `if m.applying` は
`if m.phase == phaseWorking` になる。

- [ ] **Step 7: `MouseWheelMsg` を D7 のとおりにする**

```go
	case tea.MouseWheelMsg:
		// The body is the only thing here that scrolls, and it is only on
		// screen with nothing over it and nothing in flight.
		if m.mode != modeView || m.phase != phaseIdle {
			return m, nil
		}
```

- [ ] **Step 8: `View` を mode/phase で書き直す**

`internal/tui/detail/render.go:17-38`。

```go
func (m Model) View() string {
	// Every mode's fetch draws the same line: there is nothing of that mode
	// to show yet.
	if m.phase == phaseLoading {
		return layout.ClipLines(m.spin.View()+" "+i18n.T("common.loading")+"\n", m.width)
	}
	switch m.mode {
	case modeCompose:
		return m.composeView()
	case modeConfirm:
		return m.confirmView()
	case modeSubmit:
		return m.submitView()
	case modePick:
		return m.pickerView()
	}
	// ... modeView's body, unchanged apart from actionErr -> errText
}
```

**順序が旧コードと違う。** 旧コードは `composing` → `confirming` → `submitting` →
`picking` → `loading` の順に見ていた。`phaseLoading` に入る前に必ず mode を
移しているので、どの mode でも `phaseLoading` の画面はスピナー 1 行であり、
差は出ない。golden がそれを確かめる。

`composeView` / `confirmView` / `pickerView` / `submitView` の中の
`m.postErr` / `m.actionErr` / `m.submitErr` / `m.posting` / `m.working` /
`m.applying` も対応表に従って置き換える。

- [ ] **Step 9: `picker.err` を外す**

`internal/tui/detail/picker.go:31` の `err` フィールドを削除し、
`listView` を `func (p picker) listView(height, width int, errText string) string` にする。
`pickerView` は `m.picker.listView(m.height, m.width, m.errText)` を呼ぶ。

- [ ] **Step 10: テストのアサーションを直す**

`detail_test.go:418` / `:712`、`picker_test.go:314` は bool を名指ししている。
**状態の名前ではなく、そこで守りたいこと**に書き直す。

```go
	if m.mode != modeCompose || m.phase != phaseWorking {
		t.Errorf("mode/phase = %v/%v, want compose/working while the post is in flight",
			m.mode, m.phase)
	}
```

- [ ] **Step 11: テストと golden が通ることを確認**

```bash
go test ./internal/tui/detail/ -v
git diff --stat -- 'internal/tui/detail/testdata/*.golden'
```

Expected: PASS、golden の diff は**空**。1 本でも変わっていたら表示を変えている。
その golden を diff で読んで原因を直す。**録り直して先へ進まない。**

- [ ] **Step 12: Step 1 のテストが今度は通ることを確認**

```bash
go test ./internal/tui/detail/ -run TestTheWheelDoesNotScrollWhatTheSpinnerHides -v
```

Expected: PASS

- [ ] **Step 13: 並行する bool が消えたことを機械的に確認**

```bash
grep -nE '\b(loading|composing|posting|confirming|working|picking|pickerLoading|applying|submitting|openingReview) +bool' internal/tui/detail/*.go
grep -nE '\b(postErr|actionErr|submitErr|err) +string' internal/tui/detail/*.go
```

Expected: どちらも出力なし。

- [ ] **Step 14: シナリオテストが通ることを確認 → `make check` → コミット**

```bash
go test ./internal/tui/app/ -v
make check
git add internal/tui/detail
git commit -m "refactor: fold the detail view's ten flags into a mode and a phase"
```

---

## Task 5: `diff` の bool を mode/phase に畳む（作業順 6b）

**Files:**
- Modify: `internal/tui/diff/diff.go:85-155`（`Model`）ほか
- Modify: `internal/tui/diff/render.go` / `comment.go` / `review.go` / `mouse.go`
- Test: `internal/tui/diff/*_test.go`

**Interfaces:**
- Produces: `diff` パッケージの `mode` / `phase`（`detail` のものとは別の型）。
  `Model` は `mode` / `phase` / `errText` を持ち、`loading` / `sidebar` /
  `expanded` / `declined` / `reviewErr` / `target` はそのまま（D3 / D6）

- [ ] **Step 1: enum を定義する**

`internal/tui/diff/diff.go` の `type Model struct` の直前。

```go
// mode is which overlay is on screen. loading is not one of these: the diff
// and the review context arrive separately, and c, v and X are gated on the
// context, not on the diff -- an overlay can be open while the files are
// still on their way (.claude/rules/tui.md).
type mode uint8

const (
	modeView mode = iota
	modeCompose
	modeSubmit
	modeDiscard
)

// phase is where the current mode is in its own round trip. There is no
// loading phase here: what an overlay needs is already on the model by the
// time the key that opens it is accepted.
type phase uint8

const (
	phaseIdle phase = iota
	phaseWorking
)
```

- [ ] **Step 2: `Model` のフィールドを畳む**

`composing` / `posting` / `submitting` / `discarding` / `discardWorking` と
`postErr` / `submitErr` / `discardErr` を `mode` / `phase` / `errText` に置き換える。
**`loading` / `sidebar` / `expanded` / `declined` / `reviewErr` / `target` は残す。**

| 旧 | 新 |
|---|---|
| `m.composing = true` | `m.mode, m.phase = modeCompose, phaseIdle` |
| `m.composing = false`（esc） | `m.mode, m.phase = modeView, phaseIdle` |
| `m.composing = false; m.posting = true`（`comment.go:87-88`） | `m.phase = phaseWorking` |
| `m.posting = false; m.composing = true`（`diff.go:277-278`） | `m.phase = phaseIdle` |
| `m.posting = false`（送信成功） | `m.mode, m.phase = modeView, phaseIdle` |
| `m.submitting = true` | `m.mode, m.phase = modeSubmit, phaseIdle` |
| `m.submitting = false` | `m.mode, m.phase = modeView, phaseIdle` |
| `if !m.submitting` | `if m.mode != modeSubmit` |
| `m.discarding = true` | `m.mode, m.phase = modeDiscard, phaseIdle` |
| `m.discarding = false` | `m.mode, m.phase = modeView, phaseIdle` |
| `m.discardWorking = true` | `m.phase = phaseWorking` |
| `m.discardWorking = false` | `m.phase = phaseIdle` |
| `m.composing \|\| m.posting` | `m.mode == modeCompose` |
| `m.postErr` / `m.submitErr` / `m.discardErr` | `m.errText` |

`commentPostedMsg`（`diff.go:267-272`）は送信が成功して composer を閉じる箇所なので、
`m.mode, m.phase = modeView, phaseIdle` になる。**旧コードは `posting = false` だけで
`composing` は既に false だった**ことを確かめてから書く。

- [ ] **Step 3: `handleKey` を mode で書き直す**

```go
func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch m.mode {
	case modeCompose:
		return m.handleComposeKey(msg)
	case modeSubmit:
		return m.handleSubmitKey(msg)
	case modeDiscard:
		return m.handleDiscardKey(msg)
	}
	switch msg.String() {
	// ... modeView's own keys, unchanged
	}
}
```

`handleComposeKey` / `handleDiscardKey` 冒頭の `if m.posting` / `if m.discardWorking` は
`if m.phase == phaseWorking` になる。

- [ ] **Step 4: `View` / `keyBar` / 高さ計算 / マウスを書き直す**

- `mouse.go:17` / `:54` の 4 つ並んだ条件 → `if m.mode != modeView { return m, nil }`
- `render.go:69` `m.composing || m.posting` → `m.mode == modeCompose`
- `render.go:72` `m.submitting` → `m.mode == modeSubmit`
- `render.go:75` `m.discarding` → `m.mode == modeDiscard`
- `render.go:83-88` の `keyBar` の `switch` → 同じく mode で
- `composerHeight` の `if !m.composing && !m.posting` → `if m.mode != modeCompose`
- `submitHeight` の `if !m.submitting` → `if m.mode != modeSubmit`
- `discardHeight` の `if !m.discarding` → `if m.mode != modeDiscard`
- `composerLines` / `discardLines` の `case m.postErr != "":` / `case m.discardErr != "":`
  → `case m.errText != "":`、`case m.posting:` / `case m.discardWorking:`
  → `case m.phase == phaseWorking:`

`render.go:58` の `if m.loading` は**そのまま**（D3）。

- [ ] **Step 5: テストのアサーションを直す**

bool を名指ししている箇所（`comment_test.go:625` のコメントを含む）を mode/phase へ。

- [ ] **Step 6: テストと golden が通ることを確認**

```bash
go test ./internal/tui/diff/ -v
git diff --stat -- 'internal/tui/diff/testdata/*.golden'
```

Expected: PASS、golden の diff は空。

- [ ] **Step 7: 並行する bool が消えたことを機械的に確認**

```bash
grep -nE '\b(composing|posting|submitting|discarding|discardWorking) +bool' internal/tui/diff/*.go
grep -nE '\b(postErr|submitErr|discardErr) +string' internal/tui/diff/*.go
```

Expected: どちらも出力なし。`loading bool` / `sidebar bool` /
`expanded map[string]bool` は D3 のとおり残るので、この grep には出ない。

- [ ] **Step 8: シナリオテストが通ることを確認 → `make check` → コミット**

```bash
go test ./internal/tui/app/ -v
make check
git add internal/tui/diff
git commit -m "refactor: fold the diff's overlay flags into a mode and a phase"
```

---

## Task 6: `Update` の case を名前つき method に出す（作業順 7）

**Files:**
- Modify: `internal/tui/detail/detail.go`（`Update`、現在 153 行）
- Modify: `internal/tui/diff/diff.go`（`Update`、現在 122 行）
- Modify: `internal/tui/app/app.go`（`Update`、現在 82 行）

**Interfaces:**
- Produces: 非公開 method のみ。外から見える型・関数は 1 つも変えない
- **`handleKey` と各 `handle*Key` は対象外。** 設計書 §4.5 は `Update` の case を言っている

**目安:** 1 つの `case` は 3 行以内（呼び出しと `return` のみ）。
行数のための規則ではなく、「関数名を読めば何をするか分かる」ための形である。

- [ ] **Step 1: `detail.Update` を分割する**

各 case の中身を、`Update` の直後に置いた method へ出す。
**ref のガード（`if msg.ref != m.ref`）も method の中に入れる。**

```go
	case tea.WindowSizeMsg:
		return m.resize(msg), nil
	case spinner.TickMsg:
		return m.tick(msg)
	case itemMsg:
		return m.itemArrived(msg), nil
	case commentPostedMsg:
		return m.commentPosted()
	case commentErrorMsg:
		return m.commentFailed(msg.err), nil
	case stateChangedMsg:
		return m.stateChanged()
	case stateErrorMsg:
		return m.stateFailed(msg.err), nil
	case pickerCandidatesMsg:
		return m.candidatesArrived(msg), nil
	case pickerAppliedMsg:
		return m.pickApplied()
	case pickErrorMsg:
		return m.pickFailed(msg.err), nil
	case reviewContextMsg:
		return m.reviewContextArrived(msg), nil
	case reviewContextErrMsg:
		return m.reviewContextFailed(msg), nil
	case review.CancelledMsg:
		return m.submitCancelled(), nil
	case review.SubmittedMsg:
		return m.submitDone()
	case review.ErrorMsg:
		return m.submitFailed(msg)
	case errMsg:
		return m.fetchFailed(msg)
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case tea.MouseWheelMsg:
		return m.wheel(msg)
```

例:

```go
// itemArrived puts a fetched item on screen. An answer for an item the user
// has already left is dropped: the request for the last one is still running
// while the view is rebuilt for the new one.
func (m Model) itemArrived(msg itemMsg) Model {
	if msg.ref != m.ref {
		return m
	}
	it := msg.item
	m.phase = phaseIdle
	m.state = it.State
	m.errText = ""
	// ... the rest of the old case body, unchanged
	return m
}
```

戻り値の形（`Model` だけか `(Model, tea.Cmd)` か）は、その case が
コマンドを返すかどうかで決める。**返さない case に `nil` を返させない。**

- [ ] **Step 2: 分割の前後でテストと golden が変わらないことを確認**

```bash
go test ./internal/tui/detail/ ./internal/tui/app/ -v
git diff --stat -- '*.golden'
```

Expected: PASS、golden の diff は空。

- [ ] **Step 3: `detail.Update` が短くなったことを確認**

```bash
awk '/^func \(m Model\) Update\(/{s=NR} /^}$/{if(s){print NR-s+1" lines"; s=0}}' internal/tui/detail/detail.go
```

Expected: 45 行以下。**行数は目安であって目的ではない。**
3 行を超える case が残っていないことを目で見る。

- [ ] **Step 4: `diff.Update` を同じ形にする**

```go
	case diffMsg:
		return m.filesArrived(msg), nil
	case reviewMsg:
		return m.reviewArrived(msg), nil
	case reviewErrMsg:
		return m.reviewFailed(msg), nil
	case errMsg:
		return m.fetchFailed(msg)
	case commentPostedMsg:
		return m.commentPosted(msg)
	case commentErrorMsg:
		return m.commentFailed(msg), nil
	case review.CancelledMsg:
		return m.submitCancelled(), nil
	case review.SubmittedMsg:
		return m.submitDone()
	case review.ErrorMsg:
		return m.submitFailed(msg)
	case discardedMsg:
		return m.discarded(msg)
```

`tea.WindowSizeMsg` は `m.resize(msg)`、`spinner.TickMsg` は `m.tick(msg)` へ。

- [ ] **Step 5: `app.Update` を同じ形にする**

3 行を超えているのは `repoResolvedMsg` と `review.SubmittedMsg` の 2 つ。
`m.repoResolved(msg)` と `m.reviewSubmitted(msg)` へ出す。
`work.OpenDetailMsg` / `repo.OpenDetailMsg` のように既に 1 行のものは触らない。

- [ ] **Step 6: 全パッケージのテストと golden**

```bash
go test ./...
git diff --stat -- '*.golden'
make check
```

Expected: PASS、golden の diff は空。

- [ ] **Step 7: コミット**

```bash
git add internal/tui
git commit -m "refactor: give every Update case a name instead of a body"
```

---

## Task 7: 実装計画・設計書への参照を消す（作業順 8）

**Files:** 下表の非テストコード 26 箇所（27 行）

**Interfaces:** コードの振る舞いは変えない。コメントだけ。

**ここで sed を使わない。** 参照の多くは意味のある文の一部で、
`(spec 3.4)` を落とすだけで済むものと、参照が理由そのものを指していて
書き足さないと文が意味を失うものが混ざっている。**1 箇所ずつ読んで判断する。**

- [ ] **Step 1: 現状を数える**

```bash
grep -rnE "spec [0-9§]|§[0-9]|Task [0-9]|Phase [0-9]" --include='*.go' internal cmd | grep -v _test | tee /tmp/specrefs-before.txt | wc -l
```

Expected: 27 行（`internal/tui/diff/render.go:486` と `:487` は 1 つのコメント）。
数が違う場合はコード側が動いているので、**この計画の表ではなく実際の出力に従う。**

- [ ] **Step 2: 1 箇所ずつ書き換えるか消すかを決める**

判断の基準（`.claude/rules/go-style.md`）:

- **参照を消すと文が意味を失う** → その参照が指していた**理由そのもの**を書く
- **参照が飾り** → 参照だけ消す
- **コメント全体が名前の言い直し** → コメントごと消す

| 場所 | 扱い |
|---|---|
| `internal/tui/app/render.go:29` | 「クリックだけでホイールは扱わない」理由を残し、参照を消す |
| `internal/tui/app/render.go:42` | 参照だけ消す（理由は文にある） |
| `internal/tui/app/app.go:39` | 参照だけ消す |
| `internal/tui/app/app.go:221` | 参照だけ消す（「板と Repos は自分のポップアップを持たない」が理由） |
| `internal/tui/work/mouse.go:26` | 参照だけ消す |
| `internal/tui/work/drawer.go:17` | 参照だけ消す |
| `internal/tui/work/render.go:25,121,218,302` | 参照だけ消す。文が「そう決めたから」しか言っていないものは、決めた理由（幅・可読性）を 1 行で書く |
| `internal/tui/repo/render.go:17,150` | 参照だけ消す |
| `internal/tui/detail/render.go:197` | 参照だけ消す |
| `internal/tui/icon/icon.go:6,153,157` | 参照だけ消す |
| `internal/tui/diff/render.go:18,26,31,48,486-487` | `Task 10` / `Task 11` は完了した計画の番号。**「hit-test も同じ値を読む」という今も真の事実**に書き換える |
| `internal/tui/diff/render.go:44,347` | 参照だけ消す |
| `internal/tui/diff/diff.go:163` | `(Task 7)` を消す。文の残りは今も真 |
| `internal/tui/diff/mouse.go:39` | 参照だけ消す |
| `internal/tui/theme/theme.go:86` | 「Phase 4 の Repos の追加ダイアログ」は**まだ無い機能への言及**。その節を消す |
| `internal/gh/review.go:41` | 参照だけ消す |

書き換えの例（`diff/render.go:26`）:

```go
	// Model.gutter, which both the drawing and the hit-test read.
```

- [ ] **Step 3: 0 になったことを確認**

```bash
grep -rnE "spec [0-9§]|§[0-9]|Task [0-9]|Phase [0-9]" --include='*.go' internal cmd | grep -v _test
```

Expected: 出力なし。

- [ ] **Step 4: 消しすぎていないことを確認**

```bash
git diff --stat
git diff -- internal/tui/diff/render.go
```

**コメントの行数が大きく減っていないこと。** 参照だけを外したなら、
消えるのは括弧の中だけである。理由ごと消した箇所があれば戻す。

- [ ] **Step 5: `make check` してコミット**

```bash
make check
git diff --stat -- '*.golden'
git add internal
git commit -m "docs: say why the code is the way it is, not which plan asked for it"
```

Expected: golden の diff は空（コメントは表示に出ない）。

---

## Task 8: `TRANSIENT` 注記を消し、設計書に並べ替えを追記する

**Files:**
- Modify: `.claude/rules/tui.md:69` の注記
- Modify: `.claude/rules/go-style.md:70` の注記
- Modify: `docs/superpowers/specs/2026-09-06-phase2-remediation-design.md`（§6）

- [ ] **Step 1: 残っている `TRANSIENT` を数える**

```bash
grep -rn 'TRANSIENT' .claude/rules .golangci.yml
```

Expected: 2 件（`tui.md` の `Part2 作業順 6` と `go-style.md` の `Part2 作業順 8`）。
Part 1 のものが残っていたら Part 1 が終わっていないので、止めて確認する。

- [ ] **Step 2: 2 つの注記を消す**

`TRANSIENT` のコメント行と、それに続く引用ブロック（`>` で始まる行）を消す。
**規約の本体は残す。**

- [ ] **Step 3: 0 になったことを確認**

```bash
grep -rn 'TRANSIENT' .claude/rules .golangci.yml
```

Expected: 出力なし。

- [ ] **Step 4: 設計書 §6 に並べ替えを追記する**

`docs/superpowers/specs/2026-09-06-phase2-remediation-design.md` の §6、
「4 と 6 は同じファイルを触るため…」の段落の隣に足す。

```markdown
> **2026-09-07 追記。** Part 2（作業順 6〜9）は **9 → 6 → 7 → 8** の順で実施した。
> 9 のシナリオテストは 6 が書き換える遷移をそのまま通るので、現行コードの上で
> 先に書けば 6 と 7 の回帰ネットになる。あわせて、6 の前に未収録の golden
> （`detail` の compose / picker / loading / エラー行、`diff` の各オーバーレイ）を
> 録った。「golden 不変で等価性を確認する」は録れているセルにしか効かないため。
>
> §4.4 の表から 3 点変えた。`diff` の `loading` は畳まない（`c` / `v` / `X` は
> レビューコンテキストでゲートされており、差分の取得中でもオーバーレイは開ける）。
> 送信中は `review.Model` の `sending` に任せ、`phase` に写さない。
> `picker` が持っていた 4 本目のエラー文字列も 1 本に畳んだ。
```

- [ ] **Step 5: `make check` してコミット**

```bash
make check
git add .claude/rules docs/superpowers/specs
git commit -m "docs: drop the notes about what the code had not caught up to yet"
```

---

## Task 9: 実機で全機能を確認し、完了条件を数える

**Files:** なし（確認のみ。直すものが出たらそのタスクに戻る）

設計書 §8 の完了条件 1 は「実機で全機能を 1 つずつ確認し、動かないものが無い」である。
テストが通っただけで完了にしない（`.claude/rules/tui.md`）。

- [ ] **Step 1: 英語で全 mode を通す**

```bash
go run ./cmd/octoscope
```

1 つでも動かなければ止めて直す。

- [ ] Work タブ: `j` / `k` / `enter` でカードが開く
- [ ] `tab` で Repos タブ、`j` / `enter` で詳細
- [ ] 詳細: 本文が出る / `j` `k` でスクロール / ホイールでスクロール
- [ ] 詳細 `c`: 入力欄が開く → 本文を書く → `ctrl+s` → コメントが付く
- [ ] 詳細 `c` → `esc`: 下書きが消えて本文に戻る
- [ ] 詳細 `x` → `y`: 状態が変わる / `x` → `n`: 何も起きない
- [ ] 詳細 `l`: スピナー → ピッカー → `space` → `enter` → ラベルが変わる
- [ ] 詳細 `l` のスピナー中にホイール: 何も動かない（D7 の変更）
- [ ] 詳細 `a`: アサイニーのピッカーが同じように動く
- [ ] 詳細 `v`: スピナー → 提出ポップアップ → `esc` で戻る
- [ ] 詳細 `o`: ブラウザが開く
- [ ] 詳細 `r`: 再取得のスピナー → 本文
- [ ] 詳細 `d`: diff が開く / `esc` で詳細に戻る
- [ ] diff: `j` `k` `[` `]` `{` `}` `h` `l` / `enter` でスレッド開閉
- [ ] diff `c`: 入力欄 → `ctrl+s` → スレッドが出る
- [ ] diff `v`: ポップアップ → コメントを付けて提出 → 反映される
- [ ] diff `X` → `y`: pending review が破棄される / `X` → `n`: 残る
- [ ] diff で差分の取得中に `c`: レビューコンテキスト待ちの注記が出る（D3 の挙動）

- [ ] **Step 2: 日本語でもう一度通す**

```bash
go run ./cmd/octoscope --lang ja
```

全角で桁が 2 つになるため、幅の崩れは日本語でしか出ない。
**特にピッカー・確認・提出ポップアップ・フッターの折返し**を見る。

- [ ] **Step 3: 完了条件を数える**

```bash
# 条件 2
make check

# 条件 3: 1 つの interface 宣言に直接並ぶメソッドが 6 個まで
grep -n -A20 'interface {' internal/tui/detail/detail.go

# 条件 4: 並行する mode の bool が無い
grep -rnE '\b(composing|posting|confirming|working|picking|pickerLoading|applying|submitting|openingReview|discarding|discardWorking) +bool' internal/tui

# 条件 5: 非テストコードに実装計画・spec への参照が 0
grep -rnE "spec [0-9§]|§[0-9]|Task [0-9]|Phase [0-9]" --include='*.go' internal cmd | grep -v _test

# 条件 7: キー入力だけのシナリオテストが 3 本以上
grep -c '^func Test' internal/tui/app/scenario_test.go

# 条件 8 の残り
grep -rn 'TRANSIENT' .claude/rules .golangci.yml
```

Expected: 条件 4 / 5 / 8 は出力なし。条件 7 は 3 以上。条件 3 は
`itemSource` 6 / `candidateSource` 2 / `reviewOpener` 1 で、`Source` は embed のみ。

- [ ] **Step 4: 見つかったものを直す**

実機で動かないものが出たら、**それを再現するテストを書いてから直す**
（`.claude/rules/testing.md`）。直したら Step 1 からやり直す。

- [ ] **Step 5: PR**

```bash
make check
git log --oneline main..HEAD
```

PR の説明には、この計画で扱った作業順（9 / 6 / 7 / 8）と、
D1〜D7 の判断、特に **D7 の振る舞いの変更 1 件**（ピッカー取得中のホイール）を書く。
末尾に次の行を足す。

```
🤖 Generated with [Claude Code](https://claude.com/claude-code)
```

---

## Self-Review

**設計書のカバレッジ:**

| 設計書 | この計画 |
|---|---|
| §4.4 UI の状態を enum にする | Task 4（detail）/ Task 5（diff）。表は D3 / D4 / D5 で 3 点だけ変えた |
| §4.5 関数の分割 | Task 6 |
| §4.7(c) シナリオテスト | Task 1 |
| §4.7(e) 空振りの確認 | Task 1 Step 5 / 7 / 9、Task 4 Step 2 |
| §4.8 コメント | Task 7 |
| §6 作業順 6〜9 | Task 1〜9（順序は D1 で並べ替え、Task 8 Step 4 で設計書に追記） |
| §8 完了条件 1〜7 | Task 9 Step 1〜3。条件 8（rules と depguard）は Part 1 で済み、`TRANSIENT` の削除だけ Task 8 |

**この計画が扱わないもの:**

- 設計書 §3.4（詳細画面の `c` が反応しないという未再現の報告、`--debug-keys`）。
  作業順 6〜9 に無く、Part 1 でも扱っていない。**Part 2 に持ち込まない。**
- `internal/gh` / `internal/gh/cli` / `internal/usecase`（Part 1 で完了）
- `work` / `repo` / `review` の `Model`。並行する mode の bool を持っていない
