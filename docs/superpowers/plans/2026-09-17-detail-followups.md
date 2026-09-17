# 詳細画面の申し送り 2 件 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 詳細画面の再設計で積み残した 2 件を片づける。`detail.go` を責務ごとに分け、
端末が低いときにキーバーが画面外へ押し出されるのを止める。

**Architecture:** どちらも `internal/app/presentation/tui/detail` の中で閉じる。分割は
関数を動かすだけで中身を変えない。高さの修正は `View()` の最後に 1 か所加えるだけで、
`relayout` の予算計算には触らない。

**Tech Stack:** Go 1.25 / Bubble Tea v2 / bubbles viewport v2 / golden テスト

**Spec:** `docs/superpowers/specs/2026-09-16-detail-redesign-design.md`
（この計画は §5.1 のファイル構成を更新する。タスク 3 で設計書を追従させる）

**Branch:** `feat/detail-redesign`（続き。`e595502` の上に積む）

---

## なぜ（実測 2026-09-17）

### 1. `detail.go` が 925 行ある

```
detail.go  925    ← Model・取得・Update・キー処理が 1 ファイルに同居
render.go  230
meta.go    217
body.go    118
```

`.claude/rules/architecture.md` は「ファイルが 300 行を超えたら、責務が増えていないか
疑う」と言っている。今回の再設計で増やしたのは 40 行ほどで、大半は元からだが、
中を見ると責務は 4 つに分かれている。

| 責務 | 現在の行 | 関数 |
|---|---|---|
| 型と状態 | 1-260, 880-925 | Model、4 つの interface、メッセージ型、mode / phase、`New`、`newBody`、`relayout`、`setBodyContent`、`stateAction`、`canMerge`、`labelNames`、`authorLogins` |
| 取得と送信 | 261-367 | `fetch` `fetchReviewContext` `openWeb` `postComment` `setState` `fetchLabelPicker` `fetchAssigneePicker` `applyPicker` |
| メッセージの処理 | 369-670 | `Update` と 20 個のハンドラ、`wheel`、`stillLoading` |
| キー入力 | 672-879 | mode ごとの 6 つの `handle*Key` |

### 2. 低い端末でキーバーが画面外へ出る

`View()` は組み立てた行数を端末の高さと突き合わせていない。2 ペインでは
`layout.JoinPanes` が `max(len(left), len(right))` 行を返し、左ペインは PR で 10 行ある。
`relayout` は本文の高さを `max(h-chromeLines, 5)` で下限 5 に丸めるので、高さ 12 行あたりから
左ペインの方が高くなり、キーバーが下へ押し出される。

詳細画面から出る唯一の手段は `esc` で、それを知らせているのがキーバーである。
高さが足りないときに失ってよいのはメタ行の末尾であって、出口の案内ではない。

## Global Constraints

- 各タスクの末尾で `make check`。緑でない状態でコミットしない
  （`internal/app/config` の `TestPathPutsTheFileUnderTheConfigDirectory` は
  この変更の前から落ちている環境依存。これだけは無視する）
- 桁数は `ansi.StringWidth` で数える（`.claude/rules/tui.md`）
- 新しい文字列は足さない。カタログは触らない

---

### Task 1: 端末の高さに収める

分割より先にやる。分割は「中身を変えない」ことが値打ちなので、振る舞いを変える修正を
混ぜない。

**Files:**
- Modify: `internal/app/presentation/tui/detail/render.go`
- Test: `internal/app/presentation/tui/detail/render_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`render_test.go` の末尾に足す。

```go
// TestTheKeyBarSurvivesAShortTerminal covers the one line the view cannot
// afford to lose: esc is the only way out of the detail view, and the key bar
// is what says so. A pull request has ten meta rows, which is taller than a
// short terminal's body, and JoinPanes runs to whichever pane is taller.
func TestTheKeyBarSurvivesAShortTerminal(t *testing.T) {
	for _, h := range []int{8, 12, 16, 24} {
		t.Run(fmt.Sprintf("height_%d", h), func(t *testing.T) {
			m := goldenModel(120)
			m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: h})

			lines := strings.Split(m.View(), "\n")
			if len(lines) > h {
				t.Errorf("the view is %d lines at a height of %d", len(lines), h)
			}
			last := ansi.Strip(lines[len(lines)-1])
			if !strings.Contains(last, "esc") {
				t.Errorf("the last line is %q, want the key bar", last)
			}
		})
	}
}
```

`render_test.go` の import に `"fmt"` が無ければ足す。

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/app/presentation/tui/detail/ -run TestTheKeyBarSurvivesAShortTerminal -v`
Expected: 高さ 8 と 12 で FAIL（`the view is 13 lines at a height of 8` のような形）。
高さ 24 は元から通る。

- [ ] **Step 3: `View` の最後で高さに収める**

`render.go` の `View()` の `return b.String() + footer` を次で置き換える。

```go
	return fitHeight(b.String(), m.height) + footer
```

同じファイルに足す。

```go
// fitHeight cuts what is drawn above the key bar down to the lines the
// terminal has, so that the key bar is still on the screen. The meta pane is
// as tall as the item has facts and does not scroll, and on a short terminal
// it would otherwise push the bar off the bottom -- taking with it the only
// notice that esc is the way out. What is lost instead is the tail of the
// pane, which the reader can get back by making the window taller.
//
// A height of zero or less means no size has arrived yet, and nothing is cut.
func fitHeight(s string, height int) string {
	if height <= 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	// The key bar is appended after this and takes the last line; s ends in a
	// newline, so its final element is the empty string before it.
	if budget := height - 1; len(lines) > budget {
		lines = append(lines[:budget], "")
	}
	return strings.Join(lines, "\n")
}
```

- [ ] **Step 4: 通ることを確かめる**

Run: `go test ./internal/app/presentation/tui/detail/ -run TestTheKeyBarSurvivesAShortTerminal -v`
Expected: PASS（4 つの高さすべて）

- [ ] **Step 5: 他が壊れていないことを確かめる**

Run: `go test ./internal/app/presentation/tui/detail/ -v`
Expected: PASS。**ゴールデンが落ちたら、それは高さ 40 の記録まで切っているということ**なので、
`budget` の計算を疑う。`make golden` で塗り潰さない。

- [ ] **Step 6: 空振りしないことを確かめる**

`fitHeight` の `if budget := ...` の中身を消して（何も切らないようにして）テストが落ちることを
見る。見たら戻す。

- [ ] **Step 7: `make check` とコミット**

```bash
git add internal/app/presentation/tui/detail/render.go internal/app/presentation/tui/detail/render_test.go
git commit -m "fix(detail): keep the key bar on screen in a short terminal

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: `detail.go` を責務ごとに分ける

**関数の中身を 1 文字も変えない。** 動かすだけ。import は各ファイルが使う分だけになる。

**Files:**
- Modify: `internal/app/presentation/tui/detail/detail.go`
- Create: `internal/app/presentation/tui/detail/commands.go`
- Create: `internal/app/presentation/tui/detail/update.go`
- Create: `internal/app/presentation/tui/detail/keys.go`

- [ ] **Step 1: `commands.go` を作る**

`detail.go` から次の 8 つを、doc コメントごと切り取って移す。

`fetch` / `fetchReviewContext` / `openWeb` / `postComment` / `setState` /
`fetchLabelPicker` / `fetchAssigneePicker` / `applyPicker`

ファイルの先頭に置く doc コメント:

```go
// The commands in this file are what the detail view asks the outside world
// to do: fetch an item, post a comment, change a state, open a page. Each
// returns a tea.Cmd that runs off the update loop and reports back as a
// message, which is the only way a view is allowed to do I/O
// (.claude/rules/tui.md).
```

- [ ] **Step 2: ビルドが通ることを確かめる**

Run: `go build ./... && go test ./internal/app/presentation/tui/detail/`
Expected: PASS。未使用 import が残っていれば `make lint` が教える

- [ ] **Step 3: `update.go` を作る**

`detail.go` から `Update` と、それが呼ぶメッセージのハンドラを移す。

`Update` / `resize` / `tick` / `itemArrived` / `commentPosted` / `commentFailed` /
`stateChanged` / `stateFailed` / `candidatesArrived` / `pickApplied` / `pickFailed` /
`reviewContextArrived` / `reviewContextFailed` / `submitCancelled` / `submitDone` /
`submitFailed` / `mergeCancelled` / `mergeDone` / `mergeFailed` / `fetchFailed` /
`wheel` / `stillLoading`

ファイルの先頭に置く doc コメント:

```go
// Update and the handlers it dispatches to. Each message type has a handler
// of its own rather than a branch inside Update, so that what happens when a
// fetch answers can be read without reading what happens when a key is
// pressed.
```

`relayout` と `setBodyContent` は**動かさない**（`detail.go` に残す）。寸法は状態であって、
メッセージの処理ではない。

- [ ] **Step 4: ビルドが通ることを確かめる**

Run: `go build ./... && go test ./internal/app/presentation/tui/detail/`
Expected: PASS

- [ ] **Step 5: `keys.go` を作る**

`detail.go` から 6 つを移す。

`handleKey` / `handlePickerKey` / `handleSubmitKey` / `handleMergeKey` /
`handleConfirmKey` / `handleComposeKey`

ファイルの先頭に置く doc コメント:

```go
// Key handling, one function per mode. Which keys do anything depends on what
// is on the screen, and keeping the modes apart is what stops a key meant for
// the composer from reaching the body underneath it (.claude/rules/tui.md).
```

- [ ] **Step 6: 全部通ることを確かめる**

Run: `go test ./internal/app/presentation/tui/detail/ -v`
Expected: PASS（ゴールデンを含めて全部。**振る舞いは 1 つも変えていない**ので、
ゴールデンが 1 バイトでも動いたら何かを書き換えている）

- [ ] **Step 7: 行数を見る**

Run: `wc -l internal/app/presentation/tui/detail/*.go`
Expected: `detail.go` が 300 行前後まで下がり、新しい 3 ファイルもそれぞれ 300 行以下。
どれかが 300 を大きく超えていたら、切り方を見直してから次へ進む。

- [ ] **Step 8: `make check` とコミット**

```bash
git add internal/app/presentation/tui/detail/
git commit -m "refactor(detail): split the model, the commands, the updates and the keys

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: 設計書を追従させる

**Files:**
- Modify: `docs/superpowers/specs/2026-09-16-detail-redesign-design.md`

- [ ] **Step 1: §5.1 のファイル構成を実際のものに直す**

今の §5.1 は `render.go` / `meta.go` / `body.go` の 3 つしか挙げていない。
`detail.go` / `commands.go` / `update.go` / `keys.go` を足し、それぞれの責務を 1 行で書く。

- [ ] **Step 2: 高さの扱いを書き足す**

§3.1（2 ペイン）に、端末が低いときは左ペインの末尾が切れてキーバーが残ることと、
その理由（`esc` が唯一の出口）を 2〜3 行で書く。§7「やらないこと」に入れていた
「端末が低いときの挙動」の記述があれば、解決済みとして消す。

- [ ] **Step 3: コミット**

```bash
git add docs/superpowers/specs/2026-09-16-detail-redesign-design.md
git commit -m "docs: follow the detail view's files and short-terminal behaviour

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## 仕上げ

- [ ] `make check` が緑（既知の `internal/app/config` の 1 件を除く）
- [ ] `wc -l internal/app/presentation/tui/detail/*.go` でどのファイルも 300 行前後
- [ ] `git diff e595502 -- internal/app/presentation/tui/detail/testdata/` が空
      （分割で描画が変わっていないこと。Task 1 の高さの修正は高さ 40 の記録に影響しない）
- [ ] 実機で `go run ./cmd/octoscope --lang ja` を、端末を低くした状態でも見る
