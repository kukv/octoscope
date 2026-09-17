# 色付けした行を憶える 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 同じ行を何度も色付けし直すのをやめる。スクロール中と、画面が変わらない再描画での 1 フレームのコストを落とす。見た目は変えない。

**Architecture:** `theme.Highlight(path, code)` の出力は入力から決まるので、`(背景, パス, 行)` → 色付き文字列を憶えれば足りる。公開 API は変えず、呼び出し元（`diff/render.go` の 1 箇所）も動かさない。同じ形のキャッシュを #103 で 2 つ入れているので新しい概念ではない。

上限 8192 エントリ、超えたら丸ごと捨てて作り直す。LRU は入れない — 効かせたいのは「直前のフレームで見た 36 行」で、それは上限のどこにいても残る。

**Tech Stack:** Go / chroma v2.27.0 / `internal/golden` によるゴールデンテスト / `go test -bench`

**設計:** `docs/superpowers/specs/2026-09-18-highlight-cost-design.md`（案 C。案 A・B を採らなかった理由もそこにある）

**前提:** このブランチは `perf/diff-per-row-work`（PR #104）の上に積む。#104 と #103 が main に入るまでマージしない。

---

## 実測した現状（2026-09-18, Apple M5, darwin/arm64）

`BenchmarkViewHugeDiff` = 4.93 ms/frame。内訳:

| 箇所 | ms/frame |
|---|---|
| `theme.Highlight`（画面に出る約 36 行） | 4.7 |
| ├ chroma が style → エスケープ列の表を組み直す | 2.8 |
| └ レキシング（regexp2） | 1.5 |
| その他 | 0.2 |

1 行あたり約 130 マイクロ秒。

## ファイル構成

| ファイル | 役割 | 変更 |
|---|---|---|
| `internal/app/presentation/tui/theme/theme.go` | 配色と syntax highlight | キャッシュと `Highlight` の差し替え |
| `internal/app/presentation/tui/theme/theme_test.go` | theme のテスト | 退行テストを 3 本追加 |
| `internal/app/presentation/tui/diff/bench_test.go` | diff のベンチ | `hugeDiff` の行を 1 行ずつ違うテキストにする、スクロールのベンチを 1 本追加 |

`Highlight` のシグネチャ、`diff` パッケージの production コード、ゴールデンファイルは**変更しない**。

---

### Task 1: 測れるベンチにする

今のベンチは 2 つの意味で、このタスクの効果を測れない。

1. `hugeDiff` は**全行が同じテキスト**なので、キャッシュを入れると初回フレームから
   ほぼ全ヒットになる。実態より良い数字が出る
2. `BenchmarkViewHugeDiff` は**同じフレームを繰り返し描く**。キャッシュ後は 2 回目以降が
   全ヒットで、「キャッシュが速い」以上のことを言わない

**Files:**
- Modify: `internal/app/presentation/tui/diff/bench_test.go`

- [x] **Step 1: 行を 1 行ずつ違うテキストにする**

`hugeDiff` の `Text` を差し替える。ほかは変えない。

```go
			lines = append(lines, domain.DiffLine{
				Kind:    domain.LineContext,
				OldLine: i + 1,
				NewLine: i + 1,
				Text:    fmt.Sprintf("\tif err := walk(ctx, node%d, depth+1); err != nil {", i),
			})
```

`hugeDiff` の doc コメントに、なぜ 1 行ずつ違えるのかを 1 文足す:
色付けの結果を憶える実装にとって、全行が同じテキストの fixture は最良の場合しか測らない。

- [x] **Step 2: スクロールのベンチを書く**

`bench_test.go` の末尾に追記する。

```go
// BenchmarkScrollHugeDiff is what holding j down costs: one frame in which
// the cursor has moved a line, so all but one of the rows on screen were
// on screen a frame ago. It is the case the view is judged on -- a frame
// that redraws with nothing changed is cheaper, and a first paint is rarer.
func BenchmarkScrollHugeDiff(b *testing.B) {
	m := hugeModel(5000)
	if len(m.rows) < 5000 {
		b.Fatalf("the diff did not land: %d rows", len(m.rows))
	}
	for b.Loop() {
		// The cursor runs out of file long before the benchmark runs out
		// of iterations; starting over costs one uncached frame, which is
		// lost in the average.
		if m.row >= len(m.rows)-1 {
			m.row, m.top = 0, 0
		}
		m = m.moveRow(1)
		benchSink = m.View()
	}
}
```

- [x] **Step 3: 走らせて現状値を記録する**

```bash
go test ./internal/app/presentation/tui/diff/ -bench='HugeDiff' -run=XXX -benchtime=100x
```

期待: `BenchmarkViewHugeDiff` と `BenchmarkScrollHugeDiff` がどちらも 5 ms/op 前後。
Step 1 で行が 1 行ずつ違うようになったので、`BenchmarkViewHugeDiff` の数字は
以前の 4.93 ms から動きうる。**両方の数値を控える。**

- [x] **Step 4: コミット**

```bash
git add internal/app/presentation/tui/diff/bench_test.go
git commit -m "test(diff): measure a frame that scrolled, on rows that differ"
```

---

### Task 2: 色付けした行を憶える

**Files:**
- Modify: `internal/app/presentation/tui/theme/theme.go`
- Modify: `internal/app/presentation/tui/theme/theme_test.go`

- [x] **Step 1: 失敗するテストを書く**

`theme_test.go` に追記する（`package theme_test` なので `theme.` を付ける）。
既に `TestHighlightFollowsTheBackground` が「背景を変えると色が変わる」を固定していて、
これはキャッシュがキーに背景を含め忘れたら落ちる。足すのは残り 3 つ。

```go
// TestHighlightIsStableForTheSameLine is the whole premise of remembering a
// coloured line: the same input must always give the same output, or the
// cache would be showing something the uncached call would not have.
func TestHighlightIsStableForTheSameLine(t *testing.T) {
	const code = "func Walk(ctx context.Context) error {"

	first := theme.Highlight("walk.go", code)
	second := theme.Highlight("walk.go", code)

	if first != second {
		t.Fatalf("the same line coloured two ways:\n%q\n%q", first, second)
	}
}

// TestHighlightTellsPathsApart guards the cache key: two files with the
// same text but different lexers must not answer for each other. A key
// missing the path would colour a Makefile as Go.
func TestHighlightTellsPathsApart(t *testing.T) {
	const code = "install: build"

	asMake := theme.Highlight("Makefile", code)
	asGo := theme.Highlight("walk.go", code)

	if asMake == asGo {
		t.Fatalf("a Makefile and a Go file coloured the same: %q", asMake)
	}
}

// TestHighlightSurvivesTheCacheFilling is the one thing the size limit can
// break. The cache is dropped whole when it fills, so a line asked for
// across that boundary must still come back the same -- the answer comes
// from chroma either way, and only the speed changes.
func TestHighlightSurvivesTheCacheFilling(t *testing.T) {
	const code = "func Walk(ctx context.Context) error {"
	want := theme.Highlight("walk.go", code)

	// One more than the limit: enough to cross the boundary once.
	for i := range highlightCacheMax + 1 {
		theme.Highlight("walk.go", fmt.Sprintf("x%d := %d", i, i))
	}

	if n := len(highlightLines); n > highlightCacheMax {
		t.Fatalf("the cache holds %d entries, want at most %d", n, highlightCacheMax)
	}
	if got := Highlight("walk.go", code); got != want {
		t.Fatalf("the answer changed once the cache had been dropped:\n%q\n%q", got, want)
	}
}

**訂正（実施後）:** 当初この計画は上限の検査を書いておらず、「境界をまたいでも答えが同じ」
だけを見ていた。それは上限が有ろうが無かろうが真なので、**上限を丸ごと削ってもテストが
通ってしまった**。`len(highlightLines)` を直接見る行が要る。これで内部テストパッケージを
置く理由も、定数 1 つを読むためではなく map を読むためになる。
```

`highlightCacheMax` は theme パッケージの非公開の定数なので、外部テストパッケージからは
見えない。**この 1 本だけ内部テストパッケージに置く**（`.claude/rules/testing.md` の
「非公開フィールドを読むテストは `package foo`」）。`theme_internal_test.go` を新規作成し、
`package theme` として、その中では `Highlight(...)` を修飾なしで呼ぶ。`fmt` を import する。

- [x] **Step 2: 走らせて、最初の 2 本が通り 3 本目が落ちることを確かめる**

```bash
go test ./internal/app/presentation/tui/theme/ -run 'TestHighlight' -v
```

期待: `TestHighlightIsStableForTheSameLine` と `TestHighlightTellsPathsApart` は PASS
（キャッシュが無くても成り立つ回帰テスト）。`TestHighlightSurvivesTheCacheFilling` は
`highlightCacheMax` が未定義でコンパイルできず失敗する。

- [x] **Step 3: キャッシュを入れる**

`theme.go` の `Highlight` の手前に足す。

```go
// highlightCacheMax is how many coloured lines are remembered before the
// lot is dropped. A line and its colours run to about a kilobyte and a half
// together, so this is about nine megabytes -- and 8192 is two hundred
// screenfuls, far more than scrolling or moving between files needs.
const highlightCacheMax = 8192

// highlightKey is what decides a coloured line: the palette the background
// chose, the lexer the path chose, and the text itself. The caller clips
// the text to the width it has, so a resize asks with a different key
// rather than getting a line that no longer fits.
type highlightKey struct {
	dark bool
	path string
	code string
}

// highlightMu and highlightLines remember coloured lines. Colouring one
// costs about 130 microseconds, of which chroma spends two thirds
// rebuilding its style-to-escape-sequence table -- work that does not
// depend on the line at all. View runs on every message, and scrolling a
// line leaves all but one row of the screen the same as the frame before,
// so the same lines are coloured again and again.
//
// When it fills it is dropped whole rather than evicted one at a time.
// What has to survive is the screenful just drawn, and that is the most
// recently added however the limit is reached; one frame after the drop
// pays full price, and the rest are cheap again.
var (
	highlightMu    sync.RWMutex
	highlightLines = map[highlightKey]string{}
)

// cachedHighlight answers from the cache, colouring and remembering the
// line on a miss.
func cachedHighlight(key highlightKey, colour func() string) string {
	highlightMu.RLock()
	line, ok := highlightLines[key]
	highlightMu.RUnlock()
	if ok {
		return line
	}

	line = colour()

	highlightMu.Lock()
	defer highlightMu.Unlock()
	if len(highlightLines) >= highlightCacheMax {
		highlightLines = map[highlightKey]string{}
	}
	highlightLines[key] = line
	return line
}
```

`Highlight` の本体を、今の中身を包む形に変える。`isDark` は `mu` が守っているが、
`Highlight` の中で 1 回だけ読んで `dark` に持つ。`chromaStyle` と `highlight` は
`isDark` を自分で読まず、渡された `dark` を使う — 鍵を作ってから色を付けるまでの間に
別のゴルーチンで `SetDark` が走ると、2 回読んだのでは片方の背景で色を付けて
もう片方の背景の鍵の下にしまうことになる。

```go
func Highlight(path, code string) string {
	if lexerFor(path) == nil {
		return code
	}

	mu.RLock()
	dark := isDark
	mu.RUnlock()

	return cachedHighlight(highlightKey{dark: dark, path: path, code: code}, func() string {
		return highlight(path, code, dark)
	})
}

// highlight is Highlight without the cache in front of it: everything below
// here is what colouring one line actually costs.
//
// dark is read once by Highlight and passed down rather than read again
// here, for the reason above.
func highlight(path, code string, dark bool) string {
	lexer := lexerFor(path)
	...
	style := styles.Get(chromaStyle(dark))
	...
}
```

`lexerFor(path) == nil` の早期リターンは、レキサの無いファイルがキャッシュに触れずに
今までどおりの速さで返るためにある — キャッシュはそこでは当たらず、本物のエントリを
押し出すだけになる。

`highlight` の中身は今の `Highlight` の本体をそのまま移す。**変えるのは
`chromaStyle()` を `chromaStyle(dark)` にする 1 行だけ。**
`Highlight` の doc コメントは `Highlight` に残し、`highlight` には上の 2 行を付ける。

- [x] **Step 4: テストを走らせる**

```bash
go test ./internal/app/presentation/tui/theme/ -run 'TestHighlight' -v
go test ./internal/app/presentation/tui/theme/ -race -run 'TestHighlight'
go test ./internal/app/presentation/tui/diff/ -run TestGolden
go test ./internal/app/presentation/tui/root/ -run TestGolden
```

期待: すべて PASS。ゴールデンは `-update` なしで通ること。
`TestHighlightFollowsTheBackground` が落ちたら、キーに背景が入っていない。

- [x] **Step 5: ベンチで効いたことを確かめる**

```bash
go test ./internal/app/presentation/tui/diff/ -bench='HugeDiff' -run=XXX -benchtime=100x
go test ./internal/app/presentation/tui/theme/ -bench=Highlight -run=XXX -benchtime=300x
```

期待:

- `BenchmarkScrollHugeDiff` が **0.5 ms/op 未満**（Task 1 の値から 1 桁落ちる）
- `BenchmarkViewHugeDiff` も 0.5 ms/op 未満
- `BenchmarkHighlightScreen` は**同じ 1 行を 50 回**色付けするので、ほぼ 0 になる。
  これは期待どおりで、このベンチはもう「1 画面ぶんのハイライト費用」を測っていない。
  **数字が落ちたことを報告するだけでよく、ベンチを書き換えない**（何を測っているかが
  変わったことは Task 3 で記録する）

- [x] **Step 6: コミット**

```bash
git add internal/app/presentation/tui/theme/theme.go internal/app/presentation/tui/theme/theme_test.go internal/app/presentation/tui/theme/theme_internal_test.go
git commit -m "perf(theme): remember a coloured line instead of colouring it again"
```

---

### Task 3: 記録と、実機での確認

- [x] **Step 1: `BenchmarkHighlightScreen` が何を測っているか書き直す**

Task 2 のあと、このベンチは同じ行を 50 回引くだけになる。コメントがそう言っていないと、
次に読む人が「1 画面ぶんのハイライトが 0 ミリ秒」と誤読する。

`theme_test.go` のコメントを、キャッシュが入った後に何を測っているのかに直す
（キャッシュのヒットの値段であって、色付けの値段ではない）。ベンチ自体は残す。

```bash
git add internal/app/presentation/tui/theme/theme_test.go
git commit -m "docs(theme): say what the screen benchmark measures now"
```

- [x] **Step 2: `make check` を通す**

```bash
make check
```

期待: tidy / lint / fmt / test すべて PASS。

- [x] **Step 3: 実際に起動して確かめる**

```bash
go run ./cmd/octoscope --repo kukv/octoscope
```

diff ビュー（`d`）で確認すること:

- `j` を押しっぱなしにしてスクロールする。**色が付いたままで、行がちらつかない**
- `[` / `]` でファイルを行き来する。**別のファイルに移っても前のファイルの色が出ない**
  （キーにパスが入っているかを目で見る）
- 端末の幅を変える。**桁がずれない**（キーはクリップ済みのテキストなので、幅を変えると
  別のキーになる）

- [x] **Step 4: 日本語でも見る**

```bash
go run ./cmd/octoscope --repo kukv/octoscope --lang ja
```

全角の桁がずれないことを見る。

- [x] **Step 5: 見たものを報告する**

何を開いて何を確認したかを PR の説明に書く。
