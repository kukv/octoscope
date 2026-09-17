# diff 画面で行あたりに払っている総量比例のコストを外す 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** diff 画面を 1 フレーム描くコストを、ファイルの行数にもファイルの数にも比例しない形にする。見た目は変えない。

**Architecture:** 前の計画（`2026-09-18-render-perf.md`）でレキサ検索とログ行の組み直しを外した。残っていたのは**見えている行数ではなく総量に比例する**2 箇所で、どちらも「画面に出ないものまで数えている / 描いている」という同じ形をしている。

- **行番号の桁数:** `lineNumberWidth()` は全 `m.rows` を走査して `strconv.Itoa` を 2 回ずつ呼ぶ。これが 1 行描くたびに **2 回** 呼ばれる（`diffTextLine` の直接呼び出しと、同じ行の `m.gutter()` 経由）。5,000 行のファイルを 50 行の画面に描くと 50 万回。答えは `m.rows` が変わるまで変わらないので、`buildRows` と同時に 1 回数えて Model に持つ。
- **サイドバー:** `sidebarLines()` は `fileTop` から**全ファイル末尾まで**行を作り、1 ファイルにつき lipgloss の `Render` を 3〜4 回呼ぶ。中で呼ぶ `threadCount` はスレッド全件を走査するので、実質 O(ファイル数 × スレッド数)。`body` は `paneHeight` 行しか使わないので、そこで打ち切る。

前の計画で学んだこと（`logLines` が `log` と食い違いうる問題）をそのまま適用する: **導出値と元データは、同じ 1 箇所で更新する。** `m.rows = m.buildRows()` が 5 箇所（`diff.go` に 4 つ、`mouse.go` に 1 つ）に散っているので、行と桁数を一緒に入れ替える 1 つのメソッドに畳む。

**Tech Stack:** Go / Bubble Tea v2 (`charm.land/bubbletea/v2`) / `internal/golden` によるゴールデンテスト / `go test -bench`

**設計:** 設計文書は無い。挙動を変えない内部最適化で、画面仕様・データの流れ・パッケージ境界のいずれも動かさないため。前段の計画は `docs/superpowers/plans/2026-09-18-render-perf.md`。

**前提:** このブランチは `perf/render-cost`（PR #103）の上に積んである。`BenchmarkView` はそちらで入ったもので、この計画はそれを土台に使う。#103 が main に入るまでマージしない。

---

## 実測した現状（2026-09-18, Apple M5, darwin/arm64）

`perf/render-cost` の HEAD（レキサキャッシュ済み）で、既存の `BenchmarkView`（ゴールデンの小さな fixture、2 ファイル・十数行）は **0.83 ms/op**。この fixture では上の 2 箇所はどちらもほとんど効かないので、**現状のベンチはこの問題を測れていない。** 大きな入力のベンチを置くところから始める。

見積もり（実測ではなく、走査回数からの概算）:

| 箇所 | 1 フレームあたりの仕事 |
|---|---|
| `lineNumberWidth` | 2 × 画面行数 × 全 rows 回の `strconv.Itoa` |
| `sidebarLines` | 全ファイル数 × `Render` 3〜4 回 + 全スレッド走査 |

## ファイル構成

| ファイル | 役割 | 変更 |
|---|---|---|
| `internal/app/presentation/tui/diff/diff.go` | Model と行の組み立て | `numWidth` フィールドを追加、`m.rows = m.buildRows()` の 4 箇所を 1 メソッドに畳む |
| `internal/app/presentation/tui/diff/render.go` | 描画 | `lineNumberWidth` が保持した値を返す、`sidebarLines` を画面の高さで打ち切る |
| `internal/app/presentation/tui/diff/mouse.go` | マウスの当たり判定 | `m.rows = m.buildRows()` の 1 箇所を同じメソッドに置き換える |
| `internal/app/presentation/tui/diff/bench_test.go` | ベンチ | 大きな入力のベンチを 1 本追加 |
| `internal/app/presentation/tui/diff/diff_test.go` | テスト | 退行テストを 2 本追加 |

`Model` の公開 API、`buildRows` のシグネチャ、ゴールデンファイルは**変更しない**。

---

### Task 1: 問題が見えるベンチを置く

既存の `BenchmarkView` は小さすぎて、この計画が直すものを測れない。大きい入力のベンチを先に置く。

**Files:**
- Modify: `internal/app/presentation/tui/diff/bench_test.go`

- [ ] **Step 1: 大きな diff の fixture とベンチを書く**

`bench_test.go` の末尾に追記する。`fakeSource` は `diff_test.go` に、`diffMsg` / `reviewMsg` は `diff.go` にある。

```go
// hugeDiff is a pull request of the size that made the diff view feel slow:
// many files, and a first file long enough that its line numbers run past
// four digits, which is what widens the gutter.
func hugeDiff(files, linesPerFile int) []domain.FileDiff {
	out := make([]domain.FileDiff, 0, files)
	for f := range files {
		lines := make([]domain.DiffLine, 0, linesPerFile)
		for i := range linesPerFile {
			lines = append(lines, domain.DiffLine{
				Kind:    domain.LineContext,
				OldLine: i + 1,
				NewLine: i + 1,
				Text:    "\tif err := walk(ctx, node, depth+1); err != nil {",
			})
		}
		out = append(out, domain.FileDiff{
			Path:      fmt.Sprintf("internal/app/pkg%d/file%d.go", f/10, f),
			Additions: linesPerFile / 2,
			Deletions: linesPerFile / 4,
			Hunks:     []domain.Hunk{{Header: "@@ -1,1 +1,1 @@", Lines: lines}},
		})
	}
	return out
}

// hugeModel is the diff view opened on hugeDiff, with a review context
// carrying a thread per file: threadCount walks every thread for every file
// the sidebar draws, so the threads have to be there to measure it.
func hugeModel(width, files, linesPerFile int) Model {
	diff := hugeDiff(files, linesPerFile)
	threads := make([]domain.ReviewThread, 0, len(diff))
	for _, f := range diff {
		threads = append(threads, domain.ReviewThread{
			Path: f.Path, Line: 1, Side: domain.SideRight,
			Comments: []domain.ThreadComment{
				{Author: domain.Author{Login: "kukv"}, Body: "ここは 2 が既定ではないでしょうか"},
			},
		})
	}
	m := New(&fakeSource{files: diff}, domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/octoscope", Number: 128})
	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 40})
	m, _ = m.Update(diffMsg{ref: m.ref, files: diff})
	m, _ = m.Update(reviewMsg{ref: m.ref, ctx: domain.ReviewContext{Threads: threads}})
	return m
}

// BenchmarkViewHugeDiff is one frame of a 300-file pull request whose open
// file runs to 5,000 lines. Only a screenful is drawn, so neither number
// should reach the cost.
func BenchmarkViewHugeDiff(b *testing.B) {
	m := hugeModel(160, 300, 5000)
	if len(m.rows) < 5000 {
		b.Fatalf("the diff did not land: %d rows", len(m.rows))
	}
	for b.Loop() {
		benchSink = m.View()
	}
}
```

import に `fmt`、`tea "charm.land/bubbletea/v2"`、`"github.com/kukv/octoscope/internal/app/domain"` が要る。

`len(m.rows) < 5000` のガードは、前の計画で checks のベンチが静かに空を測りかけた失敗と同じ形を防ぐためのもの。`diffMsg` が捨てられればベンチは一瞬で終わり、何もゲートしなくなる。

- [ ] **Step 2: 走らせて現状値を記録する**

```bash
go test ./internal/app/presentation/tui/diff/ -bench=View -run=XXX -benchtime=20x
```

期待: `BenchmarkViewHugeDiff` が `BenchmarkView`（0.83 ms）より桁で遅い。**両方の数値を控える。** もし差が小さければ、見立てが外れているので Task 2・3 に進む前に報告すること（走査回数の見積もりであって実測ではない）。

- [ ] **Step 3: コミット**

```bash
git add internal/app/presentation/tui/diff/bench_test.go
git commit -m "test(diff): measure a frame of a pull request that is actually big"
```

---

### Task 2: 行番号の桁数を、行を組むときに 1 回だけ数える

**Files:**
- Modify: `internal/app/presentation/tui/diff/diff.go`
- Modify: `internal/app/presentation/tui/diff/mouse.go`
- Modify: `internal/app/presentation/tui/diff/render.go`
- Modify: `internal/app/presentation/tui/diff/diff_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`diff_test.go` に追記する。桁数はキャッシュしても値が変わってはいけない — そこが壊れるとガター幅が狂って全行がずれる。

```go
// TestLineNumberWidthFollowsTheRows locks the one thing caching the gutter
// width can break: it must still answer for the file on screen. A file
// whose numbers run past four digits needs a wider gutter than the floor,
// and moving to a short file must give the floor back.
func TestLineNumberWidthFollowsTheRows(t *testing.T) {
	long := domain.FileDiff{Path: "long.go", Hunks: []domain.Hunk{{
		Header: "@@ -12000,1 +12000,1 @@",
		Lines:  []domain.DiffLine{{Kind: domain.LineContext, OldLine: 12000, NewLine: 12000, Text: "x"}},
	}}}
	short := domain.FileDiff{Path: "short.go", Hunks: []domain.Hunk{{
		Header: "@@ -1,1 +1,1 @@",
		Lines:  []domain.DiffLine{{Kind: domain.LineContext, OldLine: 1, NewLine: 1, Text: "x"}},
	}}}

	m := New(&fakeSource{files: []domain.FileDiff{long, short}},
		domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/koto", Number: 1})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 160, Height: 30})
	m, _ = m.Update(diffMsg{ref: m.ref, files: []domain.FileDiff{long, short}})

	if got := m.lineNumberWidth(); got != 5 {
		t.Fatalf("lineNumberWidth() = %d on a five-digit file, want 5", got)
	}

	m = m.moveFile(1)

	if got := m.lineNumberWidth(); got != 4 {
		t.Fatalf("lineNumberWidth() = %d after moving to a short file, want 4 (the floor)", got)
	}
}

// TestLineNumberWidthIsCounted is why the width is kept at all: it is asked
// for twice per drawn row, and the answer only changes when the rows do.
// Counting it on every ask walked every row of the file each time.
func TestLineNumberWidthIsCounted(t *testing.T) {
	m := goldenModel(160)

	if m.numWidth == 0 {
		t.Fatal("the rows were built without counting the width")
	}
	if got, want := m.lineNumberWidth(), m.numWidth; got != want {
		t.Fatalf("lineNumberWidth() = %d, want the counted %d", got, want)
	}
}
```

- [ ] **Step 2: 走らせて失敗を確かめる**

```bash
go test ./internal/app/presentation/tui/diff/ -run 'TestLineNumberWidth' -v
```

期待: `TestLineNumberWidthIsCounted` が `m.numWidth` という未定義フィールドでコンパイルできず失敗する。`TestLineNumberWidthFollowsTheRows` は、コンパイルが通れば今も PASS する回帰テスト。

- [ ] **Step 3: 行と桁数を一緒に入れ替える**

`diff.go` の Model に足す。`rows` の宣言の直後に置く。

```go
	// numWidth is how many columns the widest line number in rows needs,
	// counted when the rows are built. The gutter is asked for it twice for
	// every row drawn, and counting it there walked the whole file each
	// time. withRows is the only place the two are set, so they cannot fall
	// out of step.
	numWidth int
```

同じく `diff.go` に、`buildRows` の直後へ足す。

```go
// withRows rebuilds the rows and everything counted from them. Every place
// that changes what the diff pane shows goes through this rather than
// assigning rows directly: a count left behind would draw the previous
// file's gutter.
func (m Model) withRows() Model {
	m.rows = m.buildRows()
	m.numWidth = countLineNumberWidth(m.rows)
	return m
}

// countLineNumberWidth is how many columns the widest line number in rows
// needs, floored at the four digits the gutter reserves. The format that
// draws a line number pads to a minimum rather than truncating, so a file
// whose numbers run past four digits must widen the gutter, or the row it
// draws runs past the budget the rest of the layout assumes.
func countLineNumberWidth(rows []row) int {
	w := (gutterWidth - 3) / 2
	for _, r := range rows {
		if r.kind != rowLine {
			continue
		}
		w = max(w, len(strconv.Itoa(r.line.OldLine)), len(strconv.Itoa(r.line.NewLine)))
	}
	return w
}
```

`countLineNumberWidth` を `diff.go` に置くので、`diff.go` の import に `"strconv"` を足す
（今は入っていない）。

`m.rows = m.buildRows()` と書いてある**全 5 箇所**を `m = m.withRows()` に置き換える。`diff.go` に 4 箇所（現状 308・323・492・550 行付近）、`mouse.go` に 1 箇所（29 行付近）。置き換えたあと `grep -rn 'm.rows = m.buildRows()' internal/` が何も出ないことを確かめる。

`render.go` の `lineNumberWidth` を、保持した値を返すだけにする。

```go
// lineNumberWidth is the gutter's number field, counted when the rows were
// built (countLineNumberWidth). The floor is applied here too, so a Model
// whose rows have not been built yet still measures the same as an empty
// file rather than zero.
func (m Model) lineNumberWidth() int { return max(m.numWidth, (gutterWidth-3)/2) }
```

`render.go` から `strconv` が使われなくなっていないか確かめる（`sidebarLines` がまだ使っているので残るはず）。

- [ ] **Step 4: テストとゴールデンを走らせる**

```bash
go test ./internal/app/presentation/tui/diff/ -run 'TestLineNumberWidth' -v
go test ./internal/app/presentation/tui/diff/ -run TestGolden
go test ./internal/app/presentation/tui/root/ -run TestGolden
```

期待: すべて PASS。ゴールデンは `-update` を付けずに通ること。**ここでゴールデンが落ちたら、桁が動いている。** 更新ではなく原因を直す。

- [ ] **Step 5: ベンチを見る**

```bash
go test ./internal/app/presentation/tui/diff/ -bench=View -run=XXX -benchtime=20x
```

期待: `BenchmarkViewHugeDiff` が Task 1 の値から落ちていること。まだサイドバーぶんが残っているので、ここで目標値には届かなくてよい。

- [ ] **Step 6: コミット**

```bash
git add internal/app/presentation/tui/diff/diff.go internal/app/presentation/tui/diff/mouse.go internal/app/presentation/tui/diff/render.go internal/app/presentation/tui/diff/diff_test.go
git commit -m "perf(diff): count the gutter's width once a file, not twice a row"
```

---

### Task 3: サイドバーを画面に入る分で打ち切る

**Files:**
- Modify: `internal/app/presentation/tui/diff/render.go`
- Modify: `internal/app/presentation/tui/diff/diff_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`diff_test.go` に追記する。

```go
// TestSidebarStopsAtTheBottom is why the file list is cut: body only ever
// uses paneHeight of its lines, and a pull request with hundreds of files
// paid to draw every one of them on every frame. Two lines per file, so the
// list may not run past the pane.
func TestSidebarStopsAtTheBottom(t *testing.T) {
	m := hugeModel(160, 300, 10)

	if got, want := len(m.sidebarLines()), m.paneHeight(); got > want {
		t.Fatalf("the sidebar drew %d lines for a %d-line pane", got, want)
	}
}

// TestSidebarStillReachesTheSelectedFile guards what the cut must not
// break: followSidebar scrolls fileTop so the selected file is on screen,
// and the cut has to leave that file in the list it returns.
func TestSidebarStillReachesTheSelectedFile(t *testing.T) {
	m := hugeModel(160, 300, 10)
	for range 40 {
		m = m.moveFile(1)
	}
	m.sidebar = true

	lines := m.sidebarLines()
	want := clip(m.files[m.file].Path, sidebarWidth)

	if !slices.ContainsFunc(lines, func(l string) bool { return strings.Contains(l, want) }) {
		t.Fatalf("the selected file %q is not in the %d lines drawn", want, len(lines))
	}
}
```

import に `slices` と `strings` が要る（`strings` は既にあるかもしれない）。`hugeModel` は Task 1 で `bench_test.go` に置いたもので、同じパッケージなのでそのまま使える。

- [ ] **Step 2: 走らせて失敗を確かめる**

```bash
go test ./internal/app/presentation/tui/diff/ -run 'TestSidebar' -v
```

期待: `TestSidebarStopsAtTheBottom` が「600 行描いた」と報告して FAIL。`TestSidebarStillReachesTheSelectedFile` は今も PASS（全件描いているので必ず入っている）。Step 3 で壊さないための回帰テスト。

- [ ] **Step 3: 打ち切る**

`render.go` の `sidebarLines` のループ条件を変える。ほかは変えない。

```go
func (m Model) sidebarLines() []string {
	if len(m.files) == 0 {
		return nil
	}
	// body only draws paneHeight lines, and a file takes two of them. Going
	// past that built rows nobody sees -- three lipgloss renders and a walk
	// of every review thread, per file, on every frame.
	h := m.paneHeight()
	lines := make([]string, 0, h)
	for i := m.fileTop; i < len(m.files) && len(lines) < h; i++ {
```

ループ本体と `return lines` はそのまま。

- [ ] **Step 4: テストとゴールデンを走らせる**

```bash
go test ./internal/app/presentation/tui/diff/ -run 'TestSidebar' -v
go test ./internal/app/presentation/tui/diff/ -run TestGolden
go test ./internal/app/presentation/tui/root/ -run TestGolden
go test ./internal/app/presentation/tui/diff/ -run TestMouse -v
```

期待: すべて PASS。マウスの当たり判定（`mouse.go` の `m.fileTop + (y-headerHeight)/2`）はサイドバーが返す行数ではなく `fileTop` を見ているので影響しないはずだが、走らせて確かめる。

- [ ] **Step 5: ベンチで目標に届いたことを確かめる**

```bash
go test ./internal/app/presentation/tui/diff/ -bench=View -run=XXX -benchtime=100x
```

期待: `BenchmarkViewHugeDiff` が **5 ms/op 未満**。

**当初の合否条件（`BenchmarkView` と同じオーダー）は誤りだった。** Task 2 のあとに取った
プロファイルで、残り 6.27 ms のうち 4.7 ms が `theme.Highlight`（画面に出る約 36 行ぶん）で、
これは PR の大きさに比例しない固定費だと分かった。サイドバーぶんは 1.5 ms なので、この Task で
届くのは約 4.7 ms である。**総量比例のコストが消えたことの確認は数字の絶対値ではなく、
`hugeModel` のファイル数を 300 から 1000 に増やしても `BenchmarkViewHugeDiff` がほとんど
変わらないことで行う**（ベンチを書き換えるのではなく、手元で一時的に数字を変えて確かめる）。

- [ ] **Step 6: コミット**

```bash
git add internal/app/presentation/tui/diff/render.go internal/app/presentation/tui/diff/diff_test.go
git commit -m "perf(diff): draw the file list only as far as the pane goes"
```

---

## 見つかった 3 番目のコスト（この計画では直さない）

Task 2 のあとのプロファイル（`BenchmarkViewHugeDiff` = 6.27 ms、Apple M5）の内訳:

| 箇所 | ms/op | 形 |
|---|---|---|
| `theme.Highlight`（画面に出る約 36 行） | 4.7 | 総量比例ではない |
| ├ chroma の style → エスケープ列の生成 | 2.8 | 1 回の `Format` ごとに固定 |
| └ レキシング（regexp2） | 1.5 | 行の長さに比例 |
| `sidebarLines` | 1.5 | 総量比例（Task 3 が消す） |

chroma の `indexedTTYFormatter.Format` は、呼ばれるたびにスタイルの全トークン型（Go で 80 前後）
について 256 色テーブルを線形走査して最も近い色を探す。1 行あたり最大 80 × 2 × 256 回の色距離計算を、
**その行の中身と無関係に**払っている。行ごとに `Format` を呼んでいるので、1 フレームで 36 回。

これを外すと約 2.2 ms まで落ちる見込みで、36 行を色付きで描く還元不能な床は約 2.1 ms。
ただし素直な直し方が無い。`styleToEscapeSequence` も `indexedTTYFormatter` も chroma の
非公開シンボルなので、`theme` 側で「エスケープ列を 1 回だけ作るフォーマッタ」を差し込むことは
できない。取りうるのは次のどちらかで、どちらも `theme.Highlight` のシグネチャを
1 行から複数行へ変える話になる。

- 画面に出る行を連結して `Highlight` を 1 回だけ呼ぶ — 行をまたぐレキサ状態（ブロックコメント、
  複数行文字列）が変わるので**見た目が変わる**。ゴールデンが動く
- 行ごとの `Tokenise` はそのままに、トークン列を連結して `Format` を 1 回だけ呼ぶ —
  行ごとのレキシングが保たれるので出力は変わらないはずだが、`theme` と `diff` の
  描画ループの両方を組み替える必要がある

**この計画の範囲外とする。** 形が違う（総量比例ではない）うえ、公開 API の変更を伴うので、
やるなら設計から起こす。

### Task 4: 全体の検査と、実機での確認

- [ ] **Step 1: `make check` を通す**

```bash
make check
```

期待: tidy / lint / fmt / test すべて PASS。

- [ ] **Step 2: 実際に起動して大きな PR を見る**

```bash
go run ./cmd/octoscope --repo kukv/octoscope
```

ファイル数の多い PR を開き、diff ビュー（`d`）で確認すること:

- `j` を押しっぱなしにしてスクロールが引っかからない
- `[` / `]` でファイルを移動したとき、サイドバーが選択中のファイルを追いかける（**打ち切りで選択行が消えていないこと**。ここが Task 3 の一番壊しやすいところ）
- 行番号が 5 桁になるファイルでガターが広がり、桁がずれない
- サイドバーのスレッド件数バッジが以前と同じ位置に出る

- [ ] **Step 3: 日本語と狭い端末でも見る**

```bash
go run ./cmd/octoscope --repo kukv/octoscope --lang ja
```

80 桁まで狭めて、サイドバーが畳まれる（`minWidthForSidebar` = 100）ところと、その手前の 100 桁の両方を見る。

- [ ] **Step 4: 見たものを報告する**

何を開いて何を確認したかを PR の説明に書く。「テストが通った」は TUI の完了条件ではない。
