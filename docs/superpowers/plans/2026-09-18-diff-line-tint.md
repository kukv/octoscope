# Diff 行の背景色 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** diff ビューの追加行/削除行を、行全体の薄い背景色で見分けられるようにする。

**Architecture:** `theme.SelectedLine` が持つ「リセットの直後に背景を張り直す」処理を
非公開の `fillLine(s, bg)` に切り出し、選択背景と diff 背景の 2 つの公開関数が
それを共有する。描画側は `diff/render.go` の `diffLine` を 3 分岐にするだけで、
`styledLine` から下は一切変えない。設計は
`docs/superpowers/specs/2026-09-18-diff-line-tint-design.md`。

**Tech Stack:** Go, charm.land/lipgloss/v2, github.com/charmbracelet/x/ansi, chroma（既存）

---

## ファイル構成

- 変更: `internal/app/presentation/tui/theme/theme.go` — `fillLine` の切り出し、
  diff 背景 2 色、`DiffLine`
- 変更: `internal/app/presentation/tui/theme/theme_test.go` — `DiffLine` のテスト
- 変更: `internal/app/presentation/tui/diff/render.go:diffLine` — 3 分岐
- 変更: `internal/app/presentation/tui/diff/diff_test.go` — 着色の単体テスト
- 再生成: `internal/app/presentation/tui/diff/testdata/*.golden`

---

### Task 1: theme に diff 行の背景を足す

**Files:**
- Modify: `internal/app/presentation/tui/theme/theme.go`
- Test: `internal/app/presentation/tui/theme/theme_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`theme_test.go` の末尾に足す。`domain` はこのファイルで既に import 済み
（`Review` / `Check` のテストが使っている）。未 import なら
`"github.com/kukv/octoscope/internal/app/domain"` を足す。

```go
// TestDiffLineCarriesTheBackgroundPastAChromaReset is the same guarantee
// SelectedLine has, for the second colour that now uses the same mechanism:
// a diff line is full of chroma's resets, and each one would end the tint.
func TestDiffLineCarriesTheBackgroundPastAChromaReset(t *testing.T) {
	dark(t)

	line := theme.DiffLine(domain.LineAdded, "a\x1b[0mb")
	bg := selectionBackground(t, line)

	if !strings.Contains(line, "\x1b[0m"+bg) {
		t.Errorf("the background is not restored after a chroma reset: %q", line)
	}
	if !strings.HasSuffix(line, ansi.ResetStyle) {
		t.Errorf("the line does not close with a reset: %q", line)
	}
}

// TestDiffLineCarriesTheBackgroundPastALipglossReset covers the other reset
// the drawing produces: a line chroma has no lexer for still goes through
// lipgloss styles around it.
func TestDiffLineCarriesTheBackgroundPastALipglossReset(t *testing.T) {
	dark(t)

	line := theme.DiffLine(domain.LineRemoved, theme.Dim().Render("x")+"y")
	bg := selectionBackground(t, line)

	if n := strings.Count(line, ansi.ResetStyle+bg); n != 1 {
		t.Errorf("the background is restored %d times after a lipgloss reset, want 1: %q", n, line)
	}
}

// TestDiffLineLeavesAContextLineAlone guards the third kind: an unchanged
// line is the majority of a diff and must stay as cheap and as plain as it
// is today.
func TestDiffLineLeavesAContextLineAlone(t *testing.T) {
	dark(t)

	if got := theme.DiffLine(domain.LineContext, "x"); got != "x" {
		t.Errorf("a context line was styled: %q", got)
	}
}

// TestDiffLineSeparatesAddedFromRemoved is what the whole change is for:
// the two states must not land on the same colour.
func TestDiffLineSeparatesAddedFromRemoved(t *testing.T) {
	dark(t)

	added := theme.DiffLine(domain.LineAdded, "x")
	removed := theme.DiffLine(domain.LineRemoved, "x")
	if selectionBackground(t, added) == selectionBackground(t, removed) {
		t.Errorf("added and removed lines share a background: %q", added)
	}
}

// TestDiffLineFollowsTheBackground guards the light variants the way
// TestSelectedLineFollowsTheBackground guards the selection's: a mistyped
// light hex would pass every test above.
func TestDiffLineFollowsTheBackground(t *testing.T) {
	dark(t)

	theme.SetDark(true)
	onDark := theme.DiffLine(domain.LineAdded, "x")
	theme.SetDark(false)
	onLight := theme.DiffLine(domain.LineAdded, "x")

	if onDark == onLight {
		t.Errorf("the same colour is used on both backgrounds: %q", onDark)
	}
}
```

- [ ] **Step 2: 失敗することを確かめる**

Run: `go test ./internal/app/presentation/tui/theme/ -run TestDiffLine`
Expected: FAIL — `undefined: theme.DiffLine`

- [ ] **Step 3: 通す最小の実装**

`theme.go` の `selection()` の下に背景色を 2 つ足す。

```go
// diffAddedBg and diffRemovedBg are the backgrounds an added and a removed
// line are filled with. GitHub's own values are alpha tints over the page's
// canvas -- addition rgba(46,160,67,.15) and deletion rgba(248,81,73,.10) on
// dark -- and a terminal has no alpha, so the dark pair below is those tints
// flattened onto #0d1117. The light pair is Primer's own opaque values.
func diffAddedBg() color.Color   { return pick("#e6ffec", "#12261e") }
func diffRemovedBg() color.Color { return pick("#ffebe9", "#25171c") }
```

`SelectedLine` の本体を `fillLine` に移す。既存の doc コメントは `fillLine`
に付け替え、`SelectedLine` には 1 行だけ残す。

```go
// fillLine draws s filled with bg the whole way across.
//
// （ここに現行 SelectedLine の doc コメント本文をそのまま移す。背景は文字の
// ある桁にしか乗らないこと、リセットは 2 形しかないことの説明。）
func fillLine(s string, bg color.Color) string {
	seq := ansi.Style{}.BackgroundColor(bg).String()
	s = strings.ReplaceAll(s, ansi.ResetStyle, ansi.ResetStyle+seq)
	s = strings.ReplaceAll(s, chromaReset, chromaReset+seq)
	return seq + s + ansi.ResetStyle
}

// SelectedLine draws s as the selected row.
func SelectedLine(s string) string { return fillLine(s, selection()) }

// DiffLine fills an added or a removed line with the colour that says which
// it is, so the two are told apart by the whole row rather than by the one
// marker column. A context line is the majority of a diff and is returned
// untouched.
func DiffLine(k domain.DiffLineKind, s string) string {
	switch k {
	case domain.LineAdded:
		return fillLine(s, diffAddedBg())
	case domain.LineRemoved:
		return fillLine(s, diffRemovedBg())
	default:
		return s
	}
}
```

- [ ] **Step 4: 通ることを確かめる**

Run: `go test ./internal/app/presentation/tui/theme/`
Expected: PASS（`TestSelectedLine*` も含めて全部）

- [ ] **Step 5: コミット**

theme.go と theme_test.go の 2 ファイルだけを add してコミットする。
メッセージ: `feat(theme): give an added and a removed line a background of their own`

---

### Task 2: diff の描画を 3 分岐にする

**Files:**
- Modify: `internal/app/presentation/tui/diff/render.go`（`diffLine`）
- Test: `internal/app/presentation/tui/diff/diff_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`diff_test.go` の末尾に足す。`strings` と `domain` は既に import 済み。

```go
// openingStyle is the SGR sequence a filled row opens with. The test reads
// it out of the output rather than naming a colour: which colour is theme's
// business (.claude/rules/tui.md).
func openingStyle(t *testing.T, line string) string {
	t.Helper()
	if !strings.HasPrefix(line, "\x1b[") {
		t.Fatalf("the line does not open with a style: %q", line)
	}
	end := strings.IndexByte(line, 'm')
	if end < 0 {
		t.Fatalf("the opening style is unterminated: %q", line)
	}
	return line[:end+1]
}

// rowOfLineKind is the first row of the current file holding a line of the
// kind asked for. It fails rather than returning nothing, so a fixture that
// stops covering a kind is a failure and not a silently empty test.
func rowOfLineKind(t *testing.T, m Model, k domain.DiffLineKind) row {
	t.Helper()
	for _, r := range m.rows {
		if r.kind == rowLine && r.line.Kind == k {
			return r
		}
	}
	t.Fatalf("the fixture has no line of kind %v", k)
	return row{}
}

// TestAddedAndRemovedLinesAreFilled is the point of the change: the two
// states are told apart by the whole row, not by the marker column alone.
func TestAddedAndRemovedLinesAreFilled(t *testing.T) {
	m := goldenModel(120)

	added := m.diffLine(rowOfLineKind(t, m, domain.LineAdded), false, 80)
	removed := m.diffLine(rowOfLineKind(t, m, domain.LineRemoved), false, 80)

	if openingStyle(t, added) == openingStyle(t, removed) {
		t.Errorf("an added and a removed line are filled the same: %q", added)
	}
}

// TestAContextLineIsNotFilled guards the majority of a diff: an unchanged
// line keeps no background of its own, or the fill would say nothing.
func TestAContextLineIsNotFilled(t *testing.T) {
	m := goldenModel(120)

	line := m.diffLine(rowOfLineKind(t, m, domain.LineContext), false, 80)
	if strings.Contains(line, "\x1b[48;") {
		t.Errorf("a context line carries a background: %q", line)
	}
}

// TestTheCursorWinsOverTheFill is the one interaction the two fills have.
// Tinting a row and then wrapping it in SelectedLine would leave the tint's
// own background in the line, re-asserting itself after every reset, and the
// cursor row would come out striped in two colours.
func TestTheCursorWinsOverTheFill(t *testing.T) {
	m := goldenModel(120)
	added := rowOfLineKind(t, m, domain.LineAdded)

	tint := openingStyle(t, m.diffLine(added, false, 80))
	selected := m.diffLine(added, true, 80)

	if strings.Contains(selected, tint) {
		t.Errorf("the cursor row still carries the added line's fill: %q", selected)
	}
	if openingStyle(t, selected) == tint {
		t.Errorf("the cursor row opens with the added line's fill: %q", selected)
	}
}
```

- [ ] **Step 2: 失敗することを確かめる**

Run: `go test ./internal/app/presentation/tui/diff/ -run 'TestAddedAndRemoved|TestAContextLine|TestTheCursorWins'`
Expected: FAIL — `TestAddedAndRemovedLinesAreFilled` が
"the line does not open with a style"（まだ着色していない）

- [ ] **Step 3: 通す最小の実装**

`render.go` の `diffLine` を置き換える。doc コメントも直す。

```go
// diffLine draws one row of the diff pane. The cursor row is drawn exactly
// as an unselected one and then filled by theme.SelectedLine, which carries
// the fill past the resets that lipgloss and chroma leave behind; an added
// or removed row elsewhere is filled the same way in its own colour. The
// two never stack: a row carrying the added fill and then wrapped in the
// selection's would re-assert its own background after every reset and come
// out striped, so the cursor wins outright.
func (m Model) diffLine(r row, selected bool, width int) string {
	line := m.styledLine(r, width)
	switch {
	case selected:
		return theme.SelectedLine(layout.Fill(line, width))
	case r.kind == rowLine && r.line.Kind != domain.LineContext:
		// layout.Fill is needed here for the same reason it is above: a
		// background lands only on columns that have a character in them.
		return theme.DiffLine(r.line.Kind, layout.Fill(line, width))
	default:
		return line
	}
}
```

- [ ] **Step 4: 通ることを確かめる**

Run: `go test ./internal/app/presentation/tui/diff/ -run 'TestAddedAndRemoved|TestAContextLine|TestTheCursorWins'`
Expected: PASS

- [ ] **Step 5: コミット**

golden はこの時点でまだ失敗する（Task 3 で更新する）ので、render.go と
diff_test.go の 2 ファイルだけを add してコミットする。
メッセージ: `feat(diff): fill an added or removed row with a colour of its own`

---

### Task 3: golden を更新し、差分を読む

**Files:**
- Modify: `internal/app/presentation/tui/diff/testdata/*.golden`

- [ ] **Step 1: 何が落ちるかを見る**

Run: `go test ./internal/app/presentation/tui/diff/ -run TestGolden`
Expected: FAIL — `diff_*` の各録画が背景色の分だけ食い違う

- [ ] **Step 2: 再生成する**

Run: `OCTOSCOPE_UPDATE_GOLDEN=1 go test ./internal/app/presentation/tui/diff/ -run TestGolden`

- [ ] **Step 3: 差分を目視で確かめる**

**受け入れる前に読む。** 差分を見て、追加行に背景が入り、行の中身（文字）は
変わっていないことを確かめる。そのうえで数える。

```bash
# 追加行の背景（dark の #12261e = 48;2;18;38;30）が入っていること
grep -c '48;2;18;38;30' internal/app/presentation/tui/diff/testdata/diff_en_120.golden
# 削除行の背景（#25171c = 48;2;37;23;28）
grep -c '48;2;37;23;28' internal/app/presentation/tui/diff/testdata/diff_en_120.golden
# カーソルが追加行に乗った録画に、選択背景が残っていること
grep -c '48;2;29;39;53' internal/app/presentation/tui/diff/testdata/diff_added_line_en_120.golden
```

Expected: いずれも 1 以上。カーソル行そのものに追加背景が無いことは Task 2 の
`TestTheCursorWinsOverTheFill` が保証しているので、ここでは録画に選択背景が
残っていることだけ確かめる。

- [ ] **Step 4: 全部通ることを確かめる**

Run: `make check`
Expected: tidy / lint / fmt / test すべて通る

- [ ] **Step 5: コミット**

`internal/app/presentation/tui/diff/testdata` を add してコミットする。
メッセージ: `test(diff): record the filled added and removed rows`

---

### Task 4: 実際に起動して見る

**Files:** なし（CLAUDE.md の「TUI の変更は起動して見る」）

- [ ] **Step 1: 英語で見る**

Run: `go run ./cmd/octoscope --repo kukv/koto`

PR を開き、diff に入る。確かめること:
- 追加行と削除行が行の右端まで色付いている
- シンタックスハイライトの文字が背景の上で潰れていない
- カーソルを `j` で追加行に乗せると、選択色 1 色になる（縞にならない）

- [ ] **Step 2: 日本語で見る**

Run: `go run ./cmd/octoscope --repo kukv/koto --lang ja`

全角の行でも背景が右端で切れたり溢れたりしないことを見る。

- [ ] **Step 3: 明るい背景の端末で見る**

端末の配色を light にして Step 1 を繰り返す。light 側の 2 色
（`#e6ffec` / `#ffebe9`）の上で本文が読めることを確かめる。

- [ ] **Step 4: 直すところがあれば theme.go の色だけを調整する**

色の値以外を直す必要が出たら、設計に戻る。
