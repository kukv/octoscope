# 選択行の塗りを最後まで届かせる 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 一覧・Diff・Work のカーソル位置を、行の色を捨てずに、行の端から端まで塗って示す。

**Architecture:** `theme.Selected()`（`lipgloss.Style`）を `theme.SelectedLine(string) string` に
置き換える。この関数は、行の中に現れる 2 形のリセット（lipgloss の `ansi.ResetStyle` と
chroma の `"\x1b[0m"`）の直後に選択色の背景を置き直す。呼び出し 12 箇所をすべて移し、
Diff が色を捨てて回避していた経路と Work カードの塗り漏れも同じ関数で直す。
データ取得・domain・usecase は触らない。

**Tech Stack:** Go 1.25 / Bubble Tea v2 / lipgloss v2 / `github.com/charmbracelet/x/ansi` / golden テスト

**Spec:** `docs/superpowers/specs/2026-09-16-selection-highlight-design.md`

**Branch:** `fix/selection-highlight`（作成済み。設計書のコミットが載っている）

## Global Constraints

- 各タスクの末尾で `make check` が緑。緑でない状態でコミットしない
- 新しい表示文字列は足さない。カタログ（`en` / `ja`）は変わらない
- 色は `theme` にだけ書く（`.claude/rules/tui.md`）。ビューに 16 進を書かない
- 桁数は `ansi.StringWidth` で数える。`len` も `utf8.RuneCountInString` も使わない
- コメントは「外部の事情・正しい理由・doc」の 3 つだけ（`.claude/rules/go-style.md`）
- golden は**実装を直したあとに**再生成する。テストを通すために先に録り直さない
- 見た目が変わらないはずの箇所（Task 6・Task 7）で golden が動いたら、
  それは想定外である。録り直す前に理由を突き止める

## ファイル構成

| ファイル | 役割 |
|---|---|
| `theme/theme.go` | `SelectedLine` を持つ。選択色とリセットの扱いはここだけが知る |
| `theme/theme_test.go` | 「リセットのあとに背景が戻る」ことを主張する |
| `layout/columns.go` | `Fill`（clip して幅まで空白で埋める）を持つ |
| 各ビューの `render.go` | `SelectedLine` を呼ぶだけ。ANSI を組み立てない |

---

### Task 1: `theme.SelectedLine` を足す

**Files:**
- Modify: `internal/app/presentation/tui/theme/theme.go`
- Test: `internal/app/presentation/tui/theme/theme_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`internal/app/presentation/tui/theme/theme_test.go` の末尾に足す。

```go
// selectionBackground is the SGR sequence SelectedLine opens the line with.
// The test reads it out of the output rather than naming a colour: which
// colour the selection uses is theme's business and may change, but whatever
// it is must come back after every reset.
func selectionBackground(t *testing.T, line string) string {
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

// TestSelectedLineCarriesTheBackgroundPastALipglossReset is the whole point of
// the function: a reset clears the background along with the foreground, so a
// line with any colour in it would lose its fill from the first reset on.
func TestSelectedLineCarriesTheBackgroundPastALipglossReset(t *testing.T) {
	dark(t)

	line := theme.SelectedLine(theme.Dim().Render("x") + "y")
	bg := selectionBackground(t, line)

	if n := strings.Count(line, ansi.ResetStyle+bg); n != 1 {
		t.Errorf("the background is restored %d times after a lipgloss reset, want 1: %q", n, line)
	}
	if !strings.HasSuffix(line, ansi.ResetStyle) {
		t.Errorf("the line does not close with a reset: %q", line)
	}
}

// TestSelectedLineCarriesTheBackgroundPastAChromaReset covers the other reset
// the drawing can produce. chroma does not use the ansi package and writes
// "\x1b[0m" after every token it colours, so a highlighted diff line is full
// of them (.../specs/2026-09-16-selection-highlight-design.md §1.4).
func TestSelectedLineCarriesTheBackgroundPastAChromaReset(t *testing.T) {
	dark(t)

	line := theme.SelectedLine("a\x1b[0mb")
	bg := selectionBackground(t, line)

	if !strings.Contains(line, "\x1b[0m"+bg) {
		t.Errorf("the background is not restored after a chroma reset: %q", line)
	}
}

// TestSelectedLineKeepsASpanWithItsOwnBackground guards the one case where the
// row's fill must *not* win: a GitHub label is drawn in the colour GitHub gave
// it, and the row's background belongs on either side of the chip, not over it.
func TestSelectedLineKeepsASpanWithItsOwnBackground(t *testing.T) {
	dark(t)

	chip := theme.Badge("d73a4a").Render(" bug ")
	line := theme.SelectedLine("title " + chip)
	bg := selectionBackground(t, line)

	if !strings.Contains(line, "48;2;215;58;74") {
		t.Errorf("the label lost the colour GitHub gave it: %q", line)
	}
	if !strings.Contains(line, ansi.ResetStyle+bg) {
		t.Errorf("the row's background does not come back after the label: %q", line)
	}
}
```

- [ ] **Step 2: 落ちることを確かめる**

```bash
go test ./internal/app/presentation/tui/theme/ -run TestSelectedLine
```

期待: コンパイルエラー `undefined: theme.SelectedLine`。

- [ ] **Step 3: 最小の実装を書く**

`internal/app/presentation/tui/theme/theme.go` の `Selected()` の**すぐ下**に足す。
`Selected()` はまだ消さない（Task 9 で消す）。

import に `"github.com/charmbracelet/x/ansi"` を足す。

```go
// chromaReset is the reset chroma's terminal formatter writes after each
// token it colours. chroma does not go through the ansi package, so this is
// the one reset in the drawing that is not ansi.ResetStyle.
const chromaReset = "\x1b[0m"

// SelectedLine draws s as the selected row: the selection's background,
// carried the whole way across the line.
//
// A background cannot simply be wrapped around a line that already has colour
// in it. Every coloured span ends in a reset, and a reset clears the
// background along with the foreground, so the fill would stop at the first
// one and leave the cursor invisible from there on. Putting the background
// back after each reset is what makes a selected row both fully filled and
// still readable as itself.
//
// The two resets below are the only ones the drawing produces: lipgloss
// renders through ansi.Style.Styled, which always closes a span with
// ansi.ResetStyle, and chroma writes chromaReset.
//
// s must already be padded to the width it should fill: a background lands
// only on columns that have a character in them.
func SelectedLine(s string) string {
	bg := ansi.Style{}.BackgroundColor(pick("#e8eef5", "#1d2735")).String()
	s = strings.ReplaceAll(s, ansi.ResetStyle, ansi.ResetStyle+bg)
	s = strings.ReplaceAll(s, chromaReset, chromaReset+bg)
	return bg + s + ansi.ResetStyle
}
```

- [ ] **Step 4: 通ることを確かめる**

```bash
go test ./internal/app/presentation/tui/theme/ -run TestSelectedLine -v
```

期待: 3 つとも PASS。

- [ ] **Step 5: `make check`**

```bash
make check
```

期待: 緑。

- [ ] **Step 6: コミット**

```bash
git add internal/app/presentation/tui/theme/
git commit -m "feat(theme): carry the selection background past a reset

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: `layout.Fill` を足す

塗る前に行を幅まで埋める必要がある。同じ「clip して空白で埋める」処理は
`diff` と `checks` がそれぞれ私有の `fit` として持っており、Search にも要る。
3 つ目の写しを作らないため、`layout` に 1 つ置いて既存の 2 つを畳む。

**Files:**
- Modify: `internal/app/presentation/tui/layout/columns.go`
- Test: `internal/app/presentation/tui/layout/columns_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`internal/app/presentation/tui/layout/columns_test.go` の末尾に足す。

```go
// TestFillPadsToExactlyTheWidth is what a selection background needs: it
// lands only on columns that have a character in them, so a short line must
// be carried out to the full width before it is wrapped.
func TestFillPadsToExactlyTheWidth(t *testing.T) {
	if got := ansi.StringWidth(layout.Fill("ab", 5)); got != 5 {
		t.Errorf("Fill(%q, 5) is %d columns wide, want 5", "ab", got)
	}
}

// TestFillCountsJapaneseAsTwoColumns: a full-width character takes two
// columns, so padding by rune count would overshoot by the number of them.
func TestFillCountsJapaneseAsTwoColumns(t *testing.T) {
	if got := ansi.StringWidth(layout.Fill("設定", 6)); got != 6 {
		t.Errorf("Fill(%q, 6) is %d columns wide, want 6", "設定", got)
	}
}

// TestFillDoesNotOverrunTheWidth: unlike Pad, Fill is given the width the
// line itself occupies, so a long line is cut to it rather than one short.
func TestFillDoesNotOverrunTheWidth(t *testing.T) {
	if got := ansi.StringWidth(layout.Fill("abcdefgh", 4)); got != 4 {
		t.Errorf("Fill(%q, 4) is %d columns wide, want 4", "abcdefgh", got)
	}
}
```

`columns_test.go` の import に `"github.com/charmbracelet/x/ansi"` が無ければ足す。

- [ ] **Step 2: 落ちることを確かめる**

```bash
go test ./internal/app/presentation/tui/layout/ -run TestFill
```

期待: コンパイルエラー `undefined: layout.Fill`。

- [ ] **Step 3: 実装する**

`internal/app/presentation/tui/layout/columns.go` の `Pad` の下に足す。

```go
// Fill clips s to w display columns and pads it out to exactly w. Unlike Pad
// it does not keep a column back: it is for a line that occupies the whole
// width rather than for a field that sits next to another one.
func Fill(s string, w int) string {
	s = Clip(s, w)
	return s + strings.Repeat(" ", max(w-ansi.StringWidth(s), 0))
}
```

- [ ] **Step 4: 通ることを確かめる**

```bash
go test ./internal/app/presentation/tui/layout/ -run TestFill -v
```

期待: 3 つとも PASS。

- [ ] **Step 5: `diff` と `checks` の私有 `fit` を畳む**

`internal/app/presentation/tui/diff/render.go` の

```go
// fit clips s and pads it out to exactly w display columns.
func fit(s string, w int) string {
	s = clip(s, w)
	return s + strings.Repeat(" ", max(w-ansi.StringWidth(s), 0))
}
```

を削除し、同ファイル内の `fit(` 6 箇所を `layout.Fill(` に置き換える。
`internal/app/presentation/tui/checks/render.go` も同じく定義を削除し、
`fit(` 4 箇所を `layout.Fill(` に置き換える。

```bash
grep -rn "fit(" internal/app/presentation/tui/diff/ internal/app/presentation/tui/checks/
```

期待: 何も出ない。出たら置き換え漏れである。

どちらのファイルも既に `layout` を import しているか確かめ、無ければ足す。
`strings` や `ansi` が使われなくなったら、その import も落とす
（自分の変更で使われなくなったものは自分で片づける。`.claude/rules` の外の
デッドコードには触らない）。

- [ ] **Step 6: `make check`**

```bash
make check
```

期待: 緑。**golden は 1 つも動かない。** `fit` と `Fill` は同じ処理であり、
描かれる文字列は変わらない。動いたら置き換えを間違えている。

- [ ] **Step 7: コミット**

```bash
git add internal/app/presentation/tui/
git commit -m "refactor(layout): move the clip-and-pad helper out of two views

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: Repo タブを移す

**Files:**
- Modify: `internal/app/presentation/tui/repo/render.go:223`
- Modify: `internal/app/presentation/tui/repo/sidebar.go:98`
- Golden: `internal/app/presentation/tui/repo/testdata/*.golden`

- [ ] **Step 1: 呼び出しを移す**

`repo/render.go` の `row` の末尾:

```go
	if i == m.cursors[m.tab] {
		return theme.SelectedLine(layout.Clip(line, m.bodyWidth()))
	}
	return layout.Clip(line, m.bodyWidth())
```

`repo/sidebar.go` の `sidebar`:

```go
		switch {
		case i == m.selected && m.focus == paneSidebar:
			line = theme.SelectedLine(line)
		case i == m.selected:
			line = theme.Accent().Render(line)
		}
```

どちらも行は既に幅ちょうどに組まれている（`row` は `Pad` / `Right` の合計が
`bodyWidth`、サイドバーは `Pad` + `Right` が `sidebarWidth`）ので、
`Fill` は要らない。

- [ ] **Step 2: golden が落ちることを確かめる**

```bash
go test ./internal/app/presentation/tui/repo/
```

期待: FAIL。`repo_prs_*` などの golden が合わない。

- [ ] **Step 3: golden を録り直す**

```bash
OCTOSCOPE_UPDATE_GOLDEN=1 go test ./internal/app/presentation/tui/repo/
```

- [ ] **Step 4: 録り直したものを目で確かめる**

```bash
cat -v internal/app/presentation/tui/repo/testdata/repo_prs_en_80.golden | sed -n '5p'
```

期待: カーソル行の中に `48;2;29;39;53` が**繰り返し**現れる。
直前までは行頭の 1 回だけだった。1 回しか出てこないなら直っていない。

```bash
git diff --stat internal/app/presentation/tui/repo/testdata/
```

期待: PR タブ・Issue タブを持つ golden だけが動いている。

- [ ] **Step 5: `make check`**

```bash
make check
```

期待: 緑。`root` の golden も動くが、これは Repo タブを含んで描いているためで、
`make check` の中で同じ更新が要る。落ちたら Step 3 を `./...` で回す。

- [ ] **Step 6: コミット**

```bash
git add internal/app/presentation/tui/
git commit -m "fix(repo): fill the cursor row from end to end

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: Search タブを移す

Search は 3 箇所あり、うち 2 箇所は幅が埋まっていないので `Fill` を通す。

**Files:**
- Modify: `internal/app/presentation/tui/search/render.go:292`（フィルタ行）
- Modify: `internal/app/presentation/tui/search/render.go:378`（結果行）
- Modify: `internal/app/presentation/tui/search/saved_render.go:77`（保存済みクエリ）
- Golden: `internal/app/presentation/tui/search/testdata/*.golden`

- [ ] **Step 1: 結果行を移す**

`search/render.go` の `resultRow` の末尾。行は列幅の合計で埋まっているので
そのまま移す。

```go
	if i == m.sel {
		return theme.SelectedLine(layout.Clip(line, width))
	}
	return layout.Clip(line, width)
```

- [ ] **Step 2: フィルタ行を移す**

`search/render.go` の `filterRow`。`label + value` はペインの幅まで
埋まっていないので `Fill` を通す。

```go
	if m.pane == paneFilters && id == m.cursor {
		return theme.SelectedLine(layout.Fill(line, filterPaneWidth))
	}
	return line
```

選択していない行は今までどおり埋めない。埋めても見た目は同じで、
golden に意味のない空白が増えるだけである。

- [ ] **Step 3: 保存済みクエリの行を移す**

`search/saved_render.go` の `savedRow`。`layout.Clip` を `layout.Fill` に変える。

```go
func (m Model) savedRow(q domain.SavedQuery, i int, width int) string {
	line := q.Name
	if rest := width - ansi.StringWidth(line) - 1; rest > 0 {
		line += " " + theme.Dim().Render(layout.Clip(q.Query, rest))
	}
	if i == m.pick {
		return theme.SelectedLine(layout.Fill(line, width))
	}
	return layout.Clip(line, width)
}
```

- [ ] **Step 4: golden が落ちることを確かめる**

```bash
go test ./internal/app/presentation/tui/search/
```

期待: FAIL。

- [ ] **Step 5: golden を録り直して目で確かめる**

```bash
OCTOSCOPE_UPDATE_GOLDEN=1 go test ./internal/app/presentation/tui/search/
cat -v internal/app/presentation/tui/search/testdata/search_en_80.golden | sed -n '4p'
```

期待: カーソル行の中に `48;2;29;39;53` が繰り返し現れる。

```bash
cat -v internal/app/presentation/tui/search/testdata/search_picker_en_120.golden | head -8
```

期待: 選んでいる保存済みクエリの行が、名前の終わりではなく**枠の幅いっぱい**まで
塗られている。

- [ ] **Step 6: `make check`**

```bash
make check
```

期待: 緑。

- [ ] **Step 7: コミット**

```bash
git add internal/app/presentation/tui/
git commit -m "fix(search): fill the cursor row and the picked filter

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: リポジトリ追加ダイアログを移す

**Files:**
- Modify: `internal/app/presentation/tui/dialog/render.go:50`
- Golden: `internal/app/presentation/tui/repo/testdata/repo_add_dialog_*.golden`
  （ダイアログ自身に testdata は無く、Repo タブの golden に写る）

- [ ] **Step 1: 呼び出しを移す**

```go
		line := layout.Pad(c.Name, nameWidth) + layout.Right(theme.Dim().Render(stars(c.Stars)), starColumn)
		if i == m.cursor {
			line = theme.SelectedLine(line)
		}
```

行は `Pad` + `Right` で `contentWidth` ちょうどなので `Fill` は要らない。

- [ ] **Step 2: golden を録り直して目で確かめる**

```bash
go test ./internal/app/presentation/tui/repo/
```

期待: FAIL。

```bash
OCTOSCOPE_UPDATE_GOLDEN=1 go test ./internal/app/presentation/tui/repo/
git diff --stat internal/app/presentation/tui/repo/testdata/
```

期待: 動くのは `repo_add_dialog_*` の 6 本だけ。他が動いたら Task 3 が漏れている。

- [ ] **Step 3: `make check` してコミット**

```bash
make check
git add internal/app/presentation/tui/
git commit -m "fix(dialog): fill the picked candidate row

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: Checks と Review を移す

この 3 箇所は今たまたま色を持たない文字列を渡しているので、**見た目は変わらない。**
移すのは、`lipgloss.Style` を返す入口を残さないためである。

**Files:**
- Modify: `internal/app/presentation/tui/checks/render.go:154`（rerun の選択肢）
- Modify: `internal/app/presentation/tui/checks/render.go:284`（check 行）
- Modify: `internal/app/presentation/tui/review/render.go:43`（event の選択）

- [ ] **Step 1: 3 箇所を移す**

`checks/render.go` の `rerunOption`:

```go
func (m Model) rerunOption(scope domain.RerunScope, text string) string {
	if scope == m.rerunScope {
		return theme.SelectedLine(text)
	}
	return text
}
```

`checks/render.go` の `checkLine`:

```go
	if cursor && m.pane == paneList {
		return theme.SelectedLine(layout.Fill(text, listWidth))
	}
```

`review/render.go` の `eventLine`:

```go
		if o.event == m.event {
			parts[i] = theme.SelectedLine(o.text)
			continue
		}
```

`rerunOption` と `eventLine` は幅まで埋めない。**並んだ選択肢のうち 1 語を
指すもので、行全体ではない。** 全幅に広げるとどれを選んでいるか読めなくなる。

- [ ] **Step 2: golden が動かないことを確かめる**

```bash
go test ./internal/app/presentation/tui/checks/ ./internal/app/presentation/tui/review/
```

期待: **PASS。** 色のない文字列を包むので、出てくる ANSI 列は前と同じである。

落ちたら止まって理由を調べる。`SelectedLine` は `lipgloss` と違う経路で
背景を出すので、同じ色でも列の形が違う可能性がある。その場合は差分を読み、
色が同じであることを確かめたうえで録り直す。**理由を確かめずに録り直さない。**

- [ ] **Step 3: `make check` してコミット**

```bash
make check
git add internal/app/presentation/tui/
git commit -m "refactor(checks,review): draw the selection through one function

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 7: Diff が色を捨てる経路をやめる

**Files:**
- Modify: `internal/app/presentation/tui/diff/render.go:365-380`（サイドバー）
- Modify: `internal/app/presentation/tui/diff/render.go:417-462`（カーソル行、`plainText`）
- Golden: `internal/app/presentation/tui/diff/testdata/*.golden`

- [ ] **Step 1: カーソル行を色付きのまま塗る**

`diffLine` を丸ごと次に置き換える（コメントは逆の理由に差し替わる）。

```go
// diffLine draws one row of the diff pane. The selected row is drawn exactly
// as an unselected one and then filled by theme.SelectedLine, which carries
// the fill past the resets that lipgloss and chroma leave behind. The cursor
// row keeps its syntax highlighting and its +/- colours.
func (m Model) diffLine(r row, selected bool, width int) string {
	line := m.styledLine(r, width)
	if selected {
		return theme.SelectedLine(layout.Fill(line, width))
	}
	return line
}

// styledLine is one row in its own colours, whether or not the cursor is on
// it.
func (m Model) styledLine(r row, width int) string {
	switch r.kind {
	case rowHunkHeader:
		return theme.HunkHeader().Render(clip(r.text, width))
	case rowNote:
		return theme.Dim().Render(clip(r.text, width))
	case rowThread:
		return m.threadLine(r, width)
	case rowCollapsed:
		return theme.Dim().Render(clip(icon.Collapsed()+" "+r.text, width))
	default:
		return m.diffTextLine(r.line, width)
	}
}
```

- [ ] **Step 2: `plainText` を消す**

```bash
grep -rn "plainText" internal/app/presentation/tui/diff/
```

期待: Step 1 のあとは定義しか出てこない。出てきたら、その呼び手も移す。

`plainText` のメソッド定義を削除する。`threadText` は `threadLine` が
まだ使っているので残す。

- [ ] **Step 3: サイドバーの選択ファイルを色付きのまま塗る**

`sidebarLines` から `plainRow` の分岐を落とし、色付きで組んだ `size` を通す。
`plainSize` は幅の判定にだけ残す。

```go
		path := clip(f.Path, sidebarWidth)
		plainSize := fmt.Sprintf("+%d −%d", f.Additions, f.Deletions)
		count, pending := m.threadCount(f.Path)
		badge := ""
		if count > 0 && ansi.StringWidth(plainSize)+1+ansi.StringWidth(fmt.Sprintf("%s%d", icon.ThreadBadge(), count)) <= sidebarWidth {
			badge = fmt.Sprintf("%s%d", icon.ThreadBadge(), count)
		}
		size := theme.Added().Render("+"+strconv.Itoa(f.Additions)) +
			" " + theme.Removed().Render("−"+strconv.Itoa(f.Deletions))
		if badge != "" {
			size += " " + theme.Count(pending).Render(badge)
		}
		if i == m.file && m.sidebar {
			lines = append(lines,
				theme.SelectedLine(layout.Fill(path, sidebarWidth)),
				theme.SelectedLine(layout.Fill(size, sidebarWidth)))
			continue
		}
		lines = append(lines, path, size)
```

バッジを出すかどうかの判定は色の付いていない `plainSize` と、色を付ける前の
バッジの文字列で行う。`ansi.StringWidth` は色を無視するので色付きで数えても
同じ答えになるが、**判定と描画で別の文字列を見ないほうが読める。**

- [ ] **Step 4: golden が落ちることを確かめる**

```bash
go test ./internal/app/presentation/tui/diff/
```

期待: FAIL。

- [ ] **Step 5: golden を録り直して目で確かめる**

```bash
OCTOSCOPE_UPDATE_GOLDEN=1 go test ./internal/app/presentation/tui/diff/
grep -o $'\033\[[0-9;]*m' internal/app/presentation/tui/diff/testdata/diff_en_120.golden | grep -c '48;2;29;39;53'
```

期待: **1 より大きい。** 直前は 1 だった（カーソル行の頭の 1 回だけ）。

- [ ] **Step 6: `make check` してコミット**

```bash
make check
git add internal/app/presentation/tui/
git commit -m "fix(diff): keep the cursor row's colour under the fill

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 8: Work のカーソルカードの中身を塗る

**Files:**
- Modify: `internal/app/presentation/tui/work/render.go`（`card`）
- Golden: `internal/app/presentation/tui/work/testdata/*.golden`

- [ ] **Step 1: 箱に渡す前に各行を塗る**

`card` を次のようにする。

```go
// card draws one card in a box of its own: where it lives on the first line,
// its title on the next two, how it is doing on the last. The selection is
// the box's colour and background.
//
// The body's lines are filled one at a time before the box is drawn around
// them. lipgloss fills its own padding, but the background it is given ends
// at the first reset inside the text, and every line here has a coloured
// marker or a dimmed repository in it.
func (m Model) card(it domain.WorkItem, at time.Time, w int, selected bool) []string {
	// The box's own border and padding come out of the width lipgloss is
	// given, so the text is clipped to what is left before it is handed over.
	inner := w - 4
	body := append([]string{cardHead(it, inner)}, cardTitle(it, inner, selected)...)
	body = append(body, m.cardMeta(it, at, inner))
	if selected {
		for i, line := range body {
			body[i] = theme.SelectedLine(line)
		}
	}
	return strings.Split(theme.Card(selected).Width(w).Render(strings.Join(body, "\n")), "\n")
}
```

行を `Fill` で埋めないのは、**`theme.Card` の `Background` が残っていて
lipgloss が行末までの padding を自分で塗るから**である。文字の終わりから
行末までは lipgloss の領分で、そこは今までどおり塗れている。

- [ ] **Step 2: golden が落ちることを確かめる**

```bash
go test ./internal/app/presentation/tui/work/
```

期待: FAIL。

- [ ] **Step 3: golden を録り直して目で確かめる**

```bash
OCTOSCOPE_UPDATE_GOLDEN=1 go test ./internal/app/presentation/tui/work/
cat -v internal/app/presentation/tui/work/testdata/work_en_120.golden | sed -n '3p'
```

期待: カーソルカードの 1 行目で、`kukv/octoscope` と `#12` の**まわりにも**
`48;2;29;39;53` が乗っている。直前は左右の padding にしか乗っていなかった。

- [ ] **Step 4: `make check` してコミット**

```bash
make check
git add internal/app/presentation/tui/
git commit -m "fix(work): fill the cursor card's body, not just its padding

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 9: `theme.Selected()` を消す

ここまでで呼び手はいなくなっている。関数を消して、同じ罠に戻れなくする。

**Files:**
- Modify: `internal/app/presentation/tui/theme/theme.go`

- [ ] **Step 1: 呼び手がいないことを確かめる**

```bash
grep -rn "theme.Selected()\|func Selected()" internal/
```

期待: `theme/theme.go` の定義 1 行だけ。他が出たら、そのビューをまだ移していない。

- [ ] **Step 2: 消す**

`theme/theme.go` から `Selected()` とその doc コメントを削除する。

- [ ] **Step 3: ビルドが通ることを確かめる**

```bash
go build ./... && make check
```

期待: 緑。

- [ ] **Step 4: コミット**

```bash
git add internal/app/presentation/tui/theme/theme.go
git commit -m "refactor(theme): remove the style that could not fill a line

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 10: 実機で見る

**テストが通っただけでは完了にしない**（`.claude/rules/tui.md`）。

- [ ] **Step 1: 英語で 4 画面を見る**

```bash
go run ./cmd/octoscope
```

確かめること。

- Repo タブで `j` / `k`。カーソル行が**端から端まで**塗られている
- カーソル行のチェックのバー（緑 / 赤）とラベルのチップが**色を保っている**
- `h` / `l` でサイドバーへ移り、リポジトリ行のカーソルが同じく塗られている
- `tab` で Issue タブ。同じこと
- `d` で Diff。カーソル行にシンタックスハイライトと `+` / `−` の色が**残っている**
- Diff のサイドバーで選択ファイルの `+12 −3` が色を保っている
- Work 板でカーソルカードの**中身**が塗られている（枠の内側の左右だけではない）
- Search タブで結果行・フィルタ行・保存済みクエリの選択

- [ ] **Step 2: 日本語で見る**

```bash
go run ./cmd/octoscope --lang ja
```

全角が 2 桁を使うので、塗りが行の端で 1 桁余ったり溢れたりしていないか見る。

- [ ] **Step 3: 80 桁で見る**

端末を 80 桁にして両方をもう一度見る。狭い幅では列が詰まるので、
塗りの継ぎ目が出やすい。

- [ ] **Step 4: 明るい背景で見る**

端末の背景を明るい色にして起動し、`#e8eef5` の側でも選択が読めることを見る。
`theme.SetDark` は起動時の問い合わせで決まるので、端末の設定を変えて確かめる。

- [ ] **Step 5: 見つかった崩れがあれば、直してから次へ**

崩れが無ければこのタスクにコミットは無い。

---

## 完了の条件

- `make check` が緑
- `theme.Selected()` が存在しない（`grep` で 0 件）
- Repo・Search・Diff・Work の golden で、カーソル行に `48;2;29;39;53` が
  **複数回**現れる
- 実機で英語・日本語・80 桁・明るい背景の 4 通りを見た
