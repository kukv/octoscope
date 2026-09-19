# Repos / Search のキーバーを最下行に固定する 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Repos タブと Search タブのキーバーを、件数によらず常に画面の最下行に置く。

**Problem:** Work は `board()` が `max(height, filled)` 行を必ず作るのでキーバーが動かない
（`docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md` §「画面は必ず端末に収まる」）。
一方 Repos の `body()` と Search の `resultPane()` は**実際にある行しか返さない**ので、
ヒット件数が少ないとキーバーがせり上がる。タブを切り替えるたびにヒントの位置が変わる。

**Architecture:** どちらのタブも「1 画面に何行入るか」はすでに計算している
（`repo.visibleRows()` / `search.resultRows()`）。その予算まで空行で埋めてから
フッターを付けるだけで、`height` の内訳と実際の行数が一致する。
埋める処理は `layout` に 1 つ置いて両方で使う。

Repos のサマリ欄（4 行）はキーバーの直上に固定する。Work のドロワーと同じ扱いで、
表もサマリも件数で動かない。サマリを出さない場面（読み込み中・0 件）では
その 4 行は空行になる。

**方針の境界:** 詰め物は**切り詰めない**。`visibleRows()` は下限 1 なので、
3 行ある「リポジトリが無い」案内を切り詰める実装にすると案内が消える。
ポップアップ（Repos の追加ダイアログ、Search の保存クエリピッカー）は
画面ではなく箱なので対象外。

**Tech Stack:** Go, charm.land/bubbletea/v2（既存）

---

## ファイル構成

- 変更: `internal/app/presentation/tui/layout/layout.go` — `PadLines`
- 変更: `internal/app/presentation/tui/layout/layout_test.go` — `PadLines` のテスト
- 変更: `internal/app/presentation/tui/repo/render.go` — `View` の組み立て
- 変更: `internal/app/presentation/tui/repo/render_test.go`（無ければ `repo_test.go`）
- 変更: `internal/app/presentation/tui/search/render.go` — `View` の組み立て
- 変更: `internal/app/presentation/tui/search/render_test.go`
- 再生成: `internal/app/presentation/tui/{repo,search}/testdata/*.golden`

---

### Task 1: layout に行を埋めるヘルパを足す

**Files:**
- Modify: `internal/app/presentation/tui/layout/layout.go`
- Test: `internal/app/presentation/tui/layout/layout_test.go`

- [ ] **Step 1: 失敗するテストを書く**

```go
// TestPadLinesFillsUpToTheBudget is what pins a key bar: the block above it
// has to be as tall as the budget the height was divided into, however few
// rows there were to draw.
func TestPadLinesFillsUpToTheBudget(t *testing.T) {
	got := layout.PadLines([]string{"a"}, 3)
	if len(got) != 3 {
		t.Errorf("the block is %d lines, want 3: %q", len(got), got)
	}
	if got[0] != "a" {
		t.Errorf("the drawn line was lost: %q", got)
	}
}

// TestPadLinesKeepsWhatOverflows guards the empty states. visibleRows bottoms
// out at one, and a helper that cut to the budget would drop two of the three
// lines that tell the user there is no repository yet.
func TestPadLinesKeepsWhatOverflows(t *testing.T) {
	got := layout.PadLines([]string{"a", "b", "c"}, 1)
	if len(got) != 3 {
		t.Errorf("the block is %d lines, want 3: %q", len(got), got)
	}
}
```

- [ ] **Step 2: 失敗することを確かめる**

Run: `go test ./internal/app/presentation/tui/layout/ -run TestPadLines`
Expected: FAIL — `undefined: layout.PadLines`

- [ ] **Step 3: 通す最小の実装**

```go
// PadLines appends blank lines until the block is n tall, so that whatever
// is drawn under it lands on the same row however few lines there were.
// A block already taller than n is returned as it is: n is a budget for what
// fits, not a limit to cut to, and an empty state that runs past it is still
// worth reading (repo.visibleRows bottoms out at one).
func PadLines(lines []string, n int) []string {
	for len(lines) < n {
		lines = append(lines, "")
	}
	return lines
}
```

- [ ] **Step 4: 通ることを確かめる**

Run: `go test ./internal/app/presentation/tui/layout/`
Expected: PASS

- [ ] **Step 5: コミット**

メッセージ: `feat(layout): pad a block of lines out to its budget`

---

### Task 2: Search の結果ペインを予算まで埋める

**Files:**
- Modify: `internal/app/presentation/tui/search/render.go`
- Test: `internal/app/presentation/tui/search/render_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`TestTheViewFitsTheHeight` の隣に足す。

```go
// TestTheKeyBarSitsOnTheLastRow is the complaint this change answers: the
// bar used to ride up with the results, so switching tabs moved the hints
// under the user's eyes. The board's bar has always been pinned.
func TestTheKeyBarSitsOnTheLastRow(t *testing.T) {
	t.Parallel()

	for _, n := range []int{0, 1, 3, 60} {
		m := sized(t, 120, make([]domain.WorkItem, n))
		if got := len(strings.Split(m.View(), "\n")); got != 40 {
			t.Errorf("%d results: the view is %d rows, want 40", n, got)
		}
	}
}
```

（`sized` が 0 件を扱えるかを先に確かめる。扱えなければヘルパ側を直さず、
このテストだけ `sized` と同じ組み立てをその場で書く。）

- [ ] **Step 2: 失敗することを確かめる**

Run: `go test ./internal/app/presentation/tui/search/ -run TestTheKeyBarSitsOnTheLastRow`
Expected: FAIL — 0 / 1 / 3 件で 40 行に足りない

- [ ] **Step 3: 通す最小の実装**

`resultPane()` の末尾で、見出し 1 行と行の予算まで埋めて返す。
`m.height <= 0`（まだサイズが来ていない）ときは埋めない。

```go
	if m.height <= 0 {
		return lines
	}
	return layout.PadLines(lines, 1+m.resultRows()) // 1: the pane's own heading
```

**ペインを結合する前に埋める。** `JoinPanes` は短いほうを補って縦罫を引くので、
埋めるのが後だと罫線が結果の途中で止まる。

`loading` と 0 件の早期 return もこの埋めを通るように、`return append(lines, ...)` を
いったん `lines = append(lines, ...)` にして最後の 1 か所に集約する。

- [ ] **Step 4: 通ることを確かめる**

Run: `go test ./internal/app/presentation/tui/search/ -run 'TestTheKeyBar|TestTheViewFits'`
Expected: PASS（`TestTheViewFitsTheHeight` は「40 行以下」なので引き続き通る）

- [ ] **Step 5: コミット**

golden はまだ落ちる（Task 4）。`render.go` と `render_test.go` だけを add する。
メッセージ: `fix(search): pin the key bar to the last row`

---

### Task 3: Repos の表とサマリを予算まで埋める

**Files:**
- Modify: `internal/app/presentation/tui/repo/render.go`
- Test: `internal/app/presentation/tui/repo/render_test.go`（無ければ `repo_test.go`）

- [ ] **Step 1: 失敗するテストを書く**

```go
// TestTheKeyBarSitsOnTheLastRow is the complaint this change answers: with
// two open pull requests the bar used to sit halfway up the terminal.
func TestTheKeyBarSitsOnTheLastRow(t *testing.T) {
	// 0 件 / 少数 / 画面に収まらない数 の 3 通りで、View() が height 行ちょうど
	// であることを見る。既存のヘルパ（repo_test.go:475 あたりの sized）を使う。
}

// TestTheSummaryBlockSitsAboveTheKeyBar is the other half of the choice: the
// block is anchored to the bottom like the board's drawer, so neither it nor
// the table above it moves as rows arrive.
func TestTheSummaryBlockSitsAboveTheKeyBar(t *testing.T) {
	// 2 件のときと 20 件のときで、サマリの 1 行目が同じ行番号に出ることを見る。
}
```

- [ ] **Step 2: 失敗することを確かめる**

Run: `go test ./internal/app/presentation/tui/repo/ -run 'TestTheKeyBar|TestTheSummaryBlock'`
Expected: FAIL

- [ ] **Step 3: 通す最小の実装**

`View()` の組み立てを変える。サマリは出す出さないにかかわらず `summaryHeight` 行を
占め、表はその上を予算いっぱいまで埋める。

```go
	lines := m.header()
	if m.height > 0 {
		lines = append(lines, layout.PadLines(m.body(), m.visibleRows())...)
		summary := []string{}
		if m.itemCount() > 0 && !m.loading[m.tab] {
			summary = m.summary()
		}
		lines = append(lines, layout.PadLines(summary, summaryHeight)...)
	} else {
		// 以下は現行どおり
	}
```

`fill()`（切り詰める既存ヘルパ）は使わない。`summary()` は
`TestTheSummaryBlockIsAlwaysTheSameHeight` が 4 行を保証しているのでそのまま通る。

**サイドバーと結合する前に埋める**（Search と同じ理由）。

- [ ] **Step 4: 通ることを確かめる**

Run: `go test ./internal/app/presentation/tui/repo/`
Expected: PASS。落ちるのは golden だけ。

- [ ] **Step 5: コミット**

メッセージ: `fix(repos): pin the key bar and the summary to the bottom`

---

### Task 4: golden を更新し、差分を読む

**Files:**
- Modify: `internal/app/presentation/tui/{repo,search}/testdata/*.golden`
- Modify: `internal/app/presentation/tui/root/testdata/*.golden`（root にも録画があれば）

- [ ] **Step 1: 何が落ちるかを見る**

Run: `go test ./internal/app/presentation/tui/... -run TestGolden`

- [ ] **Step 2: 再生成する**

Run: `OCTOSCOPE_UPDATE_GOLDEN=1 go test ./internal/app/presentation/tui/... -run TestGolden`

- [ ] **Step 3: 差分を目視で確かめる**

**受け入れる前に読む。** 増えたのが末尾寄りの空行だけで、
文字の出ている行が消えたり動いたりしていないことを見る。
特に `repo_empty_*` と `repos_none_*`（案内が 3 行のもの）で
案内が残っていることを確かめる。

- [ ] **Step 4: 全部通ることを確かめる**

Run: `make check`
Expected: tidy / lint / fmt / test すべて通る

- [ ] **Step 5: コミット**

メッセージ: `test(tui): record the pinned key bar`

---

### Task 5: 実際に起動して見る

**Files:** なし（CLAUDE.md の「TUI の変更は起動して見る」）

- [ ] **Step 1: 英語で見る**

Run: `go run ./cmd/octoscope --repo kukv/koto`

確かめること:
- 1 / 2 / 3 をタブで往復して、ヒントが同じ行に居続ける
- Repos で PR が数件のとき、サマリが最下行の 1 つ上に居る
- Search で 0 件 / 数件 / 50+ のどれでもヒントが動かない
- サイドバーと結果ペインの縦罫が画面の下まで引かれている

- [ ] **Step 2: 日本語で見る**

Run: `go run ./cmd/octoscope --repo kukv/koto --lang ja`

- [ ] **Step 3: 端末を縦に縮めて見る**

10 行程度まで縮め、ヒントが画面外に押し出されないことを見る。
