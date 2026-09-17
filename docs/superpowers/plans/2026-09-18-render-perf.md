# 描画コストの削減（レキサのキャッシュとログ行のキャッシュ）実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** diff ビューと Actions ログビューが 1 フレーム描くのにかかる時間を、行数に依存しない範囲まで落とす。見た目は 1 ピクセルも変えない。

**Architecture:** 描画そのものは既に「画面に見える行だけ」に絞られている。重いのは、**見える行ごとに毎回やり直している検索**と、**毎フレーム全行を組み直している文字列生成**の 2 つである。どちらも「答えが変わらないものを 1 回だけ計算する」という同じ形で直す。新しい抽象は導入せず、`order` が「チェックが届いたときに 1 回だけ組む」のと同じ既存の型を踏襲する。

- **レキサ:** `theme.Highlight` は 1 行ごとに `lexers.Match(path)` を呼ぶ。実測 **1.46 ms/回**で、`Highlight` 全体の 97%。パス → レキサの対応は実行中ずっと変わらないので、`theme` パッケージ内の `sync.Map` に憶える。**一致しなかった（nil）場合も憶える** — 未知の拡張子のほうが 2.07 ms と高くつく。配色（`chromaStyle`）はキャッシュしない。背景色は `SetDark` で実行中に変わる。
- **ログ行:** `checks.logRows()` はログの全行を毎フレーム組み直す（文字列連結・`Time.Format`・ステップ見出しの `Render`）。しかも 1 キー入力につき 3 箇所から呼ばれる（`render.go` の描画、`checks.go` のカーソル範囲、`log.go` の横スクロール範囲）。`JobLog` が届いた時点で 1 回だけ組んで Model に持つ。ローディング中と空表示の分岐は状態が毎フレーム変わりうるので、そこは今のまま毎フレーム組む。

**Tech Stack:** Go / Bubble Tea v2 (`charm.land/bubbletea/v2`) / chroma v2.27.0 / `internal/golden` によるゴールデンテスト / `go test -bench`

**設計:** 設計文書は無い。挙動を変えない内部最適化で、画面仕様・データの流れ・パッケージ境界のいずれも動かさないため。判断の根拠となる実測値はこの計画の「実測した現状」に残す。

---

## 実測した現状（2026-09-18, Apple M5, darwin/arm64）

| 処理 | 実測 |
|---|---|
| `lexers.Match("render.go")` | 1,463,797 ns/op |
| `Tokenise` + `Format`（1 行） | 130,603 ns/op |
| `styles.Get("github-dark")` | 14.83 ns/op |
| `formatters.Get("terminal256")` | 4.67 ns/op |
| `theme.Highlight` 1 行 | 1,500,850 ns/op |
| `theme.Highlight` 50 行（1 画面） | 79,960,288 ns/op |
| `theme.Highlight`（未知の拡張子） | 2,067,549 ns/op |

**80 ms/フレーム。** 60 fps が 16.7 ms、人が「もたつく」と感じ始めるのが 100 ms 前後なので、キー入力 1 回ごとにその大半を払っている。レキサをキャッシュすると 50 行あたり 6.5 ms（`Tokenise`+`Format` のみ）まで落ちる見込み。

ログ側は行数に比例する。全行ぶんの文字列生成 × 3 呼び出し / キー入力。

## この計画で**やらないこと**

次の 3 つは同じ調査で見つかったが、範囲外とする。別の計画で扱う。

- `diff.lineNumberWidth()` が全 `m.rows` を走査し、1 行描くたびに 2 回呼ばれる（`render.go:464` と `m.gutter()` 経由の `:465`）
- `diff.sidebarLines()` が `paneHeight` で打ち切らず全ファイルを描き、中の `threadCount` が O(threads)
- 描画済み行文字列のメモ化

## ファイル構成

| ファイル | 役割 | 変更 |
|---|---|---|
| `internal/app/presentation/tui/theme/theme.go` | 配色と syntax highlight | `lexerFor` を追加、`Highlight` がそれを呼ぶ |
| `internal/app/presentation/tui/theme/theme_test.go` | theme のテスト | 挙動テスト 2 本とベンチ 1 本を追加 |
| `internal/app/presentation/tui/checks/checks.go` | checks の Model | `logLines` フィールドを追加 |
| `internal/app/presentation/tui/checks/log.go` | ログ取得と状態 | `logArrived` で行を組む、`clearLog` で捨てる |
| `internal/app/presentation/tui/checks/render.go` | ログ描画 | `logRows` が `m.logLines` を返す、組む処理を `buildLogLines` に出す |
| `internal/app/presentation/tui/checks/bench_test.go` | ログのベンチ | 新規 |
| `internal/app/presentation/tui/checks/log_test.go` | ログのキャッシュのテスト | 新規 |
| `internal/app/presentation/tui/diff/bench_test.go` | diff 全体のベンチ | 新規 |

`Highlight` のシグネチャ、`logRows` のシグネチャと戻り値、`domain.LogLine`、ゴールデンファイルは**変更しない**。

---

### Task 1: 現状を測るベンチマークを置く

先に測る場所を作る。これが Task 2・3 の合否判定になる。

**Files:**
- Create: `internal/app/presentation/tui/diff/bench_test.go`
- Create: `internal/app/presentation/tui/checks/bench_test.go`
- Modify: `internal/app/presentation/tui/theme/theme_test.go`（末尾に追記）

- [x] **Step 1: theme のベンチを書く**

`internal/app/presentation/tui/theme/theme_test.go` の末尾に追記する。このファイルは `package theme_test`（外部テストパッケージ）なので、`theme.` を付けて呼ぶ。

```go
// benchSink keeps the compiler from optimising the benchmarked call away.
var benchSink string

// BenchmarkHighlightScreen is one screenful of diff: Highlight is called
// once per visible row, so this is what a keypress costs before anything
// else the view does.
func BenchmarkHighlightScreen(b *testing.B) {
	const code = "func (m Model) diffTextLine(l domain.DiffLine, width int) string {"
	for b.Loop() {
		for range 50 {
			benchSink = theme.Highlight("internal/app/presentation/tui/diff/render.go", code)
		}
	}
}

// BenchmarkHighlightUnknownExt is a file chroma has no lexer for. It costs
// more than a Go file, not less: lexers.Match only gives up after trying
// every pattern it has.
func BenchmarkHighlightUnknownExt(b *testing.B) {
	const code = "the quick brown fox jumps over the lazy dog"
	for b.Loop() {
		benchSink = theme.Highlight("docs/notes.unknownext", code)
	}
}
```

- [x] **Step 2: diff のベンチを書く**

`internal/app/presentation/tui/diff/bench_test.go` を新規作成する。`goldenModel` と `fixture` は既存のテストヘルパ（`golden_test.go` / `diff_test.go`）にある。

```go
package diff

import "testing"

// benchSink keeps the compiler from optimising View away.
var benchSink string

// BenchmarkView is one frame of the diff pane at the width a screen most
// commonly opens at. Bubble Tea calls View once per message, so this is
// what a single keypress costs.
func BenchmarkView(b *testing.B) {
	m := goldenModel(160)
	for b.Loop() {
		benchSink = m.View()
	}
}
```

- [x] **Step 3: checks のベンチを書く**

`internal/app/presentation/tui/checks/bench_test.go` を新規作成する。`goldenModel` は `golden_test.go` にある。5 万行は、長い Actions のジョブログの現実的な上限。

```go
package checks

import (
	"fmt"
	"testing"
	"time"

	"github.com/kukv/octoscope/internal/app/domain"
)

// benchSink keeps the compiler from optimising View away.
var benchSink string

// hugeLog is a job log of the size a long Actions run actually produces.
// Every tenth line starts a new step, so the step headings are in the
// measurement too.
func hugeLog(n int) []domain.LogLine {
	at := time.Date(2026, 9, 7, 10, 15, 30, 0, time.UTC)
	lines := make([]domain.LogLine, 0, n)
	for i := range n {
		lines = append(lines, domain.LogLine{
			Step: fmt.Sprintf("Run step %d", i/10),
			Time: at.Add(time.Duration(i) * time.Second),
			Text: fmt.Sprintf("go: downloading github.com/example/module/v%d v1.2.3", i),
		})
	}
	return lines
}

// BenchmarkViewHugeLog is one frame with a 50,000-line log open. Only a
// screenful is drawn, so the cost must not follow the log's length.
func BenchmarkViewHugeLog(b *testing.B) {
	m := goldenModel(160)
	m, _ = m.Update(keyPress("enter"))
	m = m.logArrived(logMsg{ref: m.ref, jobID: m.selectedJob(), lines: hugeLog(50000)})
	for b.Loop() {
		benchSink = m.View()
	}
}

// BenchmarkMoveRowHugeLog is one keypress in the log pane: it clamps the
// cursor and re-bounds the horizontal offset, each of which asks for the
// log's rows.
func BenchmarkMoveRowHugeLog(b *testing.B) {
	m := goldenModel(160)
	m, _ = m.Update(keyPress("enter"))
	m = m.logArrived(logMsg{ref: m.ref, jobID: m.selectedJob(), lines: hugeLog(50000)})
	m.pane = paneLog
	for b.Loop() {
		m = m.moveRow(1)
	}
}
```

- [x] **Step 4: 走らせて現状値を記録する**

```bash
go test ./internal/app/presentation/tui/theme/ -bench=Highlight -run=XXX -benchtime=300x
go test ./internal/app/presentation/tui/diff/ -bench=View -run=XXX -benchtime=100x
go test ./internal/app/presentation/tui/checks/ -bench=HugeLog -run=XXX -benchtime=20x
```

期待: すべて PASS し、`BenchmarkHighlightScreen` が 50〜100 ms/op、`BenchmarkViewHugeLog` と `BenchmarkMoveRowHugeLog` が数十 ms/op のオーダーで出る。**この 5 本の数値を控えておく。** Task 2・3 の後で同じコマンドを走らせて比べる。

`logArrived` が `msg.jobID != m.selectedJob()` でログを捨てる作りなので、`keyPress("enter")` を先に送ってカーソル位置のジョブを確定させている。もし `m.log` が空のままベンチが数 μs で終わったら、それはログが捨てられている。`m.selectedJob()` が空でないかを確認すること。

- [x] **Step 5: コミット**

```bash
git add internal/app/presentation/tui/theme/theme_test.go internal/app/presentation/tui/diff/bench_test.go internal/app/presentation/tui/checks/bench_test.go
git commit -m "test(perf): measure what one frame of the diff and the log cost"
```

---

### Task 2: パスからレキサを引いた結果を憶える

**Files:**
- Modify: `internal/app/presentation/tui/theme/theme.go`（`Highlight` とその手前）
- Modify: `internal/app/presentation/tui/theme/theme_test.go`

- [x] **Step 1: 失敗するテストを書く**

`theme_test.go` に追記する。ここも `package theme_test` なので `theme.` を付ける。キャッシュは外から見えないので、**キャッシュしてはいけないものがキャッシュされていないこと**を固定する。これが壊れると画面の配色が背景に追従しなくなる。

**実施時の訂正（406c3f0）:** 下の `TestHighlightFollowsBackground` は、既にあった
`TestHighlightFollowsTheBackground` と同じことを検証していた。計画を書いた時点で既存テストを
見落としていた。追加したうえで削除し、既存のものを残してある。残ったのは
`TestHighlightUnknownExtension` だけで、こちらは既存の `TestHighlightLeavesUnknownFilesAlone`
とは別物である（別のパスを引いた直後に、一致しないパスを繰り返し引く経路を見ている）。

```go
// TestHighlightFollowsBackground locks the boundary of the lexer cache:
// which lexer a path gets never changes, but which palette it is drawn in
// does, every time the terminal reports its background. Caching the whole
// answer by path would freeze the colours at whatever the first draw used.
func TestHighlightFollowsBackground(t *testing.T) {
	t.Cleanup(func() { theme.SetDark(true) })
	const code = "func main() { return }"

	theme.SetDark(true)
	dark := theme.Highlight("main.go", code)
	theme.SetDark(false)
	light := theme.Highlight("main.go", code)

	if dark == light {
		t.Fatalf("the same colours on both backgrounds: %q", dark)
	}
}

// TestHighlightUnknownExtension is a file chroma has no lexer for, asked
// for twice: the second call must come back uncoloured the same way the
// first did. A cache that only remembers hits would colour it as whatever
// was asked for before it.
func TestHighlightUnknownExtension(t *testing.T) {
	const code = "the quick brown fox"
	theme.Highlight("main.go", code)

	for range 2 {
		if got := theme.Highlight("docs/notes.unknownext", code); got != code {
			t.Fatalf("Highlight coloured a file with no lexer: %q", got)
		}
	}
}
```

- [x] **Step 2: 走らせて、現状では通ることを確かめる**

```bash
go test ./internal/app/presentation/tui/theme/ -run 'TestHighlight(FollowsBackground|UnknownExtension)' -v
```

期待: 両方 PASS。**これは回帰テストなので、今 PASS するのが正しい。** Step 3 の変更でここが FAIL したら、キャッシュの範囲を間違えている。

- [x] **Step 3: キャッシュを入れる**

`theme.go` の import に `"github.com/alecthomas/chroma/v2"` を足す。

```go
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
```

`Highlight` の直前に足す。

```go
// lexerCache remembers which lexer a path resolved to. lexers.Match globs a
// path against every lexer chroma has registered, which measures at about
// 1.5ms -- and Highlight is called once per visible row, every frame, so
// that one call is nearly the whole cost of drawing the diff. A path's
// lexer never changes while the program runs.
//
// A path that matched nothing is remembered too, as a nil lexer: a miss
// costs more than a hit, not less, because Match only gives up after trying
// every pattern it has.
var lexerCache sync.Map // path -> chroma.Lexer, nil when none matches

func lexerFor(path string) chroma.Lexer {
	if v, ok := lexerCache.Load(path); ok {
		// A remembered miss is a nil interface, which this assertion
		// reports as not-ok; either way the answer is nil.
		l, _ := v.(chroma.Lexer)
		return l
	}
	l := lexers.Match(path)
	lexerCache.Store(path, l)
	return l
}
```

`Highlight` の 1 行目を差し替える。ほかの行は変えない。

```go
func Highlight(path, code string) string {
	lexer := lexerFor(path)
	if lexer == nil {
		return code
	}
```

`mu` は使わない。`sync.Map` は自前で同期する。

- [x] **Step 4: テストとベンチを走らせる**

```bash
go test ./internal/app/presentation/tui/theme/ -run 'TestHighlight' -v
go test ./internal/app/presentation/tui/theme/ -bench=Highlight -run=XXX -benchtime=300x
go test ./internal/app/presentation/tui/diff/ -bench=View -run=XXX -benchtime=100x
```

期待:
- テストは全 PASS（`TestHighlightFollowsBackground` が落ちたら、配色までキャッシュしている）
- `BenchmarkHighlightScreen` が **16 ms/op 未満**（80 ms → 6〜7 ms のはず）
- `BenchmarkHighlightUnknownExt` が **10 μs 未満**。キャッシュされた miss 自体は約 10 ns だが、
  この `-benchtime=300x` には初回の実 miss（約 2 ms）が 1 回だけ含まれ、300 で割った約 7 μs が
  平均に乗る。実測 8.0 μs。**計画時に書いた「1 μs 未満」は、この初回ぶんを数え忘れていた。**
  キャッシュが効いていることを直接見たいなら `-benchtime=10000x` で約 10 ns/op に収束する
- `BenchmarkViewHugeLog` は変わらない（ログ側は別の原因）

- [x] **Step 5: 見た目が変わっていないことをゴールデンで確かめる**

```bash
go test ./internal/app/presentation/tui/diff/ -run TestGolden
```

期待: PASS。**`-update` を付けない。** ここでゴールデンが落ちるなら色か桁が動いているので、更新ではなく原因を直す。

- [x] **Step 6: コミット**

```bash
git add internal/app/presentation/tui/theme/theme.go internal/app/presentation/tui/theme/theme_test.go
git commit -m "perf(theme): look a path's lexer up once instead of once a row"
```

---

### Task 3: ログの行をジョブが届いたときに 1 回だけ組む

**Files:**
- Modify: `internal/app/presentation/tui/checks/checks.go:83` 付近（Model のフィールド）
- Modify: `internal/app/presentation/tui/checks/log.go`（`logArrived` と `clearLog`）
- Modify: `internal/app/presentation/tui/checks/render.go:323`（`logRows`）

- [x] **Step 1: 失敗するテストを書く**

`internal/app/presentation/tui/checks/log_test.go` を新規作成する（今は無い）。冒頭は `package checks` と `import "testing"` だけでよい。`goldenModel` / `keyPress` / `goldenLog` は既存のテストヘルパにある。

```go
// TestLogLinesClearedWithLog locks what the cached rows must follow: the
// log they were built from. A cache left behind after clearLog would draw
// the previous check's log under the check the cursor moved to.
func TestLogLinesClearedWithLog(t *testing.T) {
	m := goldenModel(160)
	m, _ = m.Update(keyPress("enter"))
	m = m.logArrived(logMsg{ref: m.ref, jobID: m.selectedJob(), lines: goldenLog()})

	if len(m.logRows()) == 0 {
		t.Fatal("no rows after a log arrived")
	}

	m = m.clearLog()

	if got := m.logRows(); got != nil {
		t.Fatalf("rows survived clearLog: %q", got)
	}
}

// TestLogRowsBuiltOnce is why the rows are kept at all: View, the cursor
// clamp and the horizontal bound each ask for them, and a job log runs to
// tens of thousands of lines. Asking twice must not build twice.
func TestLogRowsBuiltOnce(t *testing.T) {
	m := goldenModel(160)
	m, _ = m.Update(keyPress("enter"))
	m = m.logArrived(logMsg{ref: m.ref, jobID: m.selectedJob(), lines: goldenLog()})

	first := m.logRows()
	second := m.logRows()

	if len(first) == 0 {
		t.Fatal("no rows after a log arrived")
	}
	if &first[0] != &second[0] {
		t.Fatal("logRows built the rows a second time")
	}
}
```

- [x] **Step 2: 走らせて失敗を確かめる**

```bash
go test ./internal/app/presentation/tui/checks/ -run 'TestLogLinesClearedWithLog|TestLogRowsBuiltOnce' -v
```

期待: `TestLogRowsBuiltOnce` が `logRows built the rows a second time` で FAIL。`TestLogLinesClearedWithLog` は PASS（`clearLog` が `m.log` を捨てるので今も通る。Step 3 で壊さないための回帰テスト）。

- [x] **Step 3: Model に組んだ行を持たせる**

`checks.go` の `log` フィールドのコメントブロックに続けて足す。

```go
	log        []domain.LogLine
	// logLines is log drawn as rows, built when the log arrives rather than
	// on every draw: View, the cursor clamp and the horizontal bound each
	// ask for them, and a job log runs to tens of thousands of lines. The
	// checks list keeps its own order the same way.
	logLines   []string
	logJob     domain.JobHandle
```

`log.go` の `logArrived` で組む。

```go
	m.log = msg.lines
	m.logLines = buildLogLines(msg.lines)
	m.logPhase = phaseIdle
```

`log.go` の `clearLog` で捨てる。

```go
	m.log = nil
	m.logLines = nil
	m.logJob = ""
```

- [x] **Step 4: `logRows` を組む側と返す側に分ける**

`render.go` の `logRows` を差し替える。ローディング中と空表示は毎フレームのままにする（スピナーのコマは 1 フレームごとに進み、空表示の文言は `failedOnly` と選択中のジョブで変わる）。

```go
// logRows is every line the log pane would draw, ungated by scrolling. The
// log's own lines were built when it arrived (buildLogLines); what is left
// here is the three states that have no log to draw and do change from one
// frame to the next -- the spinner advances, and which "empty" it is
// depends on the cursor.
func (m Model) logRows() []string {
	if m.logPhase == phaseLoading {
		return []string{m.spin.View() + " " + i18n.T("checks.log_loading")}
	}
	if len(m.log) == 0 {
		if m.logJob != "" && m.logJob == m.selectedJob() {
			if m.failedOnly {
				return []string{theme.Dim().Render(i18n.T("checks.log_empty_failed"))}
			}
			return []string{theme.Dim().Render(i18n.T("checks.log_empty_full"))}
		}
		return nil
	}
	return m.logLines
}

// buildLogLines draws a job's log as rows: a heading per step and the
// step's own lines under it. A continuation line has no timestamp of its
// own (LogLine.Time is zero), so only a stamped line gets one drawn in
// front of it.
func buildLogLines(log []domain.LogLine) []string {
	var lines []string
	last := ""
	first := true
	for _, l := range log {
		if first || l.Step != last {
			lines = append(lines, theme.Heading().Render(l.Step))
			last = l.Step
		}
		first = false
		text := l.Text
		if !l.Time.IsZero() {
			text = l.Time.Format("15:04:05") + "  " + text
		}
		lines = append(lines, text)
	}
	return lines
}
```

`render.go` の import から使わなくなったものが出たら消す（`domain` は `buildLogLines` の引数で使い続けるので残る）。

- [x] **Step 5: テストを走らせる**

```bash
go test ./internal/app/presentation/tui/checks/ -run 'TestLog' -v
go test ./internal/app/presentation/tui/checks/ -run TestGolden
```

期待: すべて PASS。ゴールデンは `-update` を付けずに通ること。

- [x] **Step 6: ベンチで効いたことを確かめる**

```bash
go test ./internal/app/presentation/tui/checks/ -bench=HugeLog -run=XXX -benchtime=100x
```

期待: `BenchmarkViewHugeLog` と `BenchmarkMoveRowHugeLog` がどちらも **16 ms/op 未満**。Task 1 で控えた値と比べて桁で落ちていること。落ちていないなら、`logRows` のどこかがまだ全行を触っている。

- [x] **Step 7: コミット**

```bash
git add internal/app/presentation/tui/checks/checks.go internal/app/presentation/tui/checks/log.go internal/app/presentation/tui/checks/render.go internal/app/presentation/tui/checks/log_test.go
git commit -m "perf(checks): build a job log's rows when it arrives, not every frame"
```

---

### Task 4: 全体の検査と、実機での確認

- [x] **Step 1: `make check` を通す**

```bash
make check
```

期待: tidy / lint / fmt / test すべて PASS。`sync.Map` に `//nolint` は要らない。もし lint が出たら `.claude/rules/go-style.md` の順（指摘どおり直す → 設計を変える → 設定を直す）に従う。

- [x] **Step 2: 実際に起動して diff を見る**

`.claude/rules` と CLAUDE.md は、TUI の変更をテストだけで完了としない。

```bash
go run ./cmd/octoscope --repo kukv/octoscope
```

大きめの PR を開き、diff ビュー（`d`）で `j` を押しっぱなしにしてスクロールする。確認すること:

- 色が付いている（キャッシュが効いていても syntax highlight は消えていない）
- スクロールが引っかからない
- 未知の拡張子のファイル（`.md`、`.yaml` でないもの）でも崩れない

- [x] **Step 3: 日本語でも見る**

```bash
go run ./cmd/octoscope --repo kukv/octoscope --lang ja
```

全角は 1 文字で 2 桁使う。日本語コメントを含む diff 行で桁がずれていないことを見る。

- [x] **Step 4: Actions のログを見る**

同じ起動から checks ビュー（`c`）を開き、失敗したジョブで `enter`、続けて `L` を押して全ステップのログを出す。`j` を押しっぱなしにしてスクロールし、引っかからないこと、ステップ見出しが正しい位置にあること、`h` / `l` の横スクロールが効くことを見る。

- [x] **Step 5: 見たものを報告する**

何を開いて何を確認したかを、PR の説明に書く。「テストが通った」は TUI の完了条件ではない。
