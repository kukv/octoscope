# octoscope Phase 2: PR レビュー 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** PR の diff を octoscope の中で読み、行にコメントを付け、approve / request changes / comment としてレビューを提出できるようにする。

**Architecture:** diff の本文は `gh pr diff --color=never` を 1 回叩いて unified diff を受け取り、`internal/gh/cli` でパースして `[]gh.FileDiff` にする。**各行は旧側・新側の両方の行番号を持つ**（行コメントの投稿が `line` と `side` を要求するため）。未提出のレビューは octoscope のメモリではなく **GitHub 側の pending review** として持ち、GraphQL の `addPullRequestReview` / `addPullRequestReviewThread` / `submitPullRequestReview` / `deletePullRequestReview` で操作する。UI は新パッケージ `internal/tui/diff`（ファイルサイドバー + diff ペイン + 行コメント入力）と `internal/tui/review`（提出ポップアップ）に分け、ルートモデルの `showingDetail bool` をビューのスタックに置き換える。

**Tech Stack:** Go 1.27 / Bubble Tea v2 (`charm.land/bubbletea/v2`) / lipgloss v2 / `charm.land/bubbles/v2` / `github.com/alecthomas/chroma/v2`（シンタックスハイライト。glamour 経由で既に依存にあり、直接依存へ昇格する） / `nicksnyder/go-i18n/v2` / `github.com/charmbracelet/x/ansi` / golangci-lint v2.13.2

**Spec:** `docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md`（特に §4.4.1 / §4.4.2 / §4.5 / §4.6）

**前フェーズの計画:** `docs/superpowers/plans/2026-09-06-octoscope-phase1.md`

**Phase 1 の積み残し:** `docs/superpowers/2026-09-06-phase1-followups.md`

## Global Constraints

- Go モジュールパス: `github.com/kukv/octoscope`。バイナリ名: `octoscope`
- charmbracelet の TUI v2 は `charm.land/*/v2` が正しい import パス。`github.com/charmbracelet/*/v2` は使わない。`github.com/charmbracelet/x/ansi` と `github.com/alecthomas/chroma/v2` は `github.com/` のままで正しい
- 表示幅の計算は `ansi.StringWidth` / `ansi.Truncate` を使う。`len()` や `utf8.RuneCountInString()` で桁を数えない
- 画面に出す文字列は `internal/i18n` から引く。Go コードに英語も日本語も直書きしない。**文字列を足すときは `internal/i18n/locales/active.en.yaml` と `active.ja.yaml` の両方に足す**
- **翻訳しないもの**: diff の本文、ファイルのパス、コメントの本文と投稿者名、GitHub が返したエラーの原文
- カバレッジ基準は 80%（`.octocov.yml`）。下回ると CI が赤くなる
- **ネットワークも外部プロセスも実際には叩かない。** 差し替えは `cli.Client.run` フィールド（関数型）で行う
- **各タスクの完了時に `make check`（tidy-check / lint / fmt-check / test）が緑であること。** 途中でビルドが壊れるタスク分割にしない
- `//nolint` を新たに足さない。必要になったら `.golangci.yml` の `exclusions` に理由つきで書く
- **パッケージを増やしたら、その場で `.golangci.yml` の `depguard` にも足す**（`.claude/rules/architecture.md`）
- **色は `internal/tui/theme` にだけ書く。** ビューに 16 進を書かない。chroma のスタイル名も theme に置く（Task 4）
- `View()` は副作用を持たず、時計も読まない。時刻は `Update` で受け取って持つ
- **GitHub アクセス層は interface を export しない。** 必要な操作の interface は使う側のファイルで宣言する
- **API 固有の文字列（`APPROVED`、`RIGHT`、`PENDING` など）を TUI で switch しない。** `internal/gh` のドメイン値に変換して渡す
- GitHub Actions の `uses:` は full-length commit SHA でピンする（org ポリシー）。本 Phase でワークフローを触る予定は無い
- コーディング規約は `CLAUDE.md` と `.claude/rules/` にある。実装前に読むこと

### Phase 2 のスコープ外（勝手にやらないこと）

- checks の一覧画面、失敗ジョブのログ、ワークフロー再実行（Phase 3）
- merge / auto-merge、キーバーの `m`（Phase 3）。**キーバーに `m` を出さない**
- Repos タブのサイドバーと追加ダイアログ、Search タブ、設定ファイル（Phase 4）
- `internal/gh/api`（go-github / githubv4 バックエンド）（Phase 4）
- 左右 2 分割の split diff。spec §4.4.1 が unified のみと決めている
- diff の折り返し（word wrap）。長い行は桁で切り詰め、横スクロールは持たない
- スレッドへの返信（`addPullRequestReviewThreadReply`）と解決（`resolveReviewThread`）。
  既存スレッドは**読むだけ**
- コミット単位の diff、`base` の選択。常に PR 全体の diff を見る

### 本計画で置く前提

- **未提出レビューは GitHub 側に持つ。** 利用者の選択（spec §4.4.2）。したがって
  「行コメントを書く」は必ず 1 回のネットワーク往復を伴う
- **pending review が既にあれば黙って引き継ぐ。** 「再開しますか」は訊かない。
  代わりにヘッダに `pending · N` と出して、書きかけの存在を隠さない
- **diff は開いたときに 1 回だけ取る。** 自動更新はしない。`r` で取り直す

---

## PR 分割

Phase 1 と同じく、1 PR がレビューできる大きさで切る。

| PR | Task | 内容 |
|---|---|---|
| PR A | 1, 2, 3 | GitHub アクセス層（diff のパーサ、レビューの取得と変更）。UI は触らない |
| PR B | 4, 5, 6 | theme のハイライト、diff ビューアの骨格、ルートモデルの配線。**この時点で `d` が動いて diff が読める** |
| PR C | 7, 8, 9 | 既存スレッドの表示、行コメント、レビュー提出。**この時点で Phase 2 の機能が揃う** |
| PR D | 10, 11, 12 | 幅の劣化、マウス、日本語の通し確認、spec と rules の更新、人手の検証手順 |

---

## File Structure

### 新規

| ファイル | 責務 |
|---|---|
| `internal/gh/diff.go` | diff のドメイン型（`FileDiff` / `Hunk` / `DiffLine` / `FileStatus` / `DiffLineKind` / `DiffSide`） |
| `internal/gh/review.go` | レビューのドメイン型（`ReviewThread` / `ThreadComment` / `PendingReview` / `ReviewEvent` / `ReviewContext`） |
| `internal/gh/cli/diff.go` | `PRDiff`。unified diff のパーサ |
| `internal/gh/cli/diff_test.go` | パーサのテーブル駆動テスト |
| `internal/gh/cli/review.go` | `PRReviewContext` / `StartReview` / `AddReviewThread` / `SubmitReview` / `DiscardReview` |
| `internal/gh/cli/review.graphql` | pending review と reviewThreads を引くクエリ |
| `internal/gh/cli/mutation.graphql` | 4 つの mutation |
| `internal/gh/cli/review_test.go` | 引数の組み立てとパース結果 |
| `internal/tui/diff/diff.go` | Model / Source / Update。ファイル選択、行カーソル、取得 |
| `internal/tui/diff/render.go` | View。サイドバー、diff ペイン、ヘッダ、キーバー |
| `internal/tui/diff/thread.go` | 行の下に差し込むスレッドの描画と畳み |
| `internal/tui/diff/comment.go` | 行コメントの入力（textarea）と投稿 |
| `internal/tui/diff/mouse.go` | クリックとホイールの当たり判定 |
| `internal/tui/diff/*_test.go` | 上記のテストと golden |
| `internal/tui/diff/testdata/*.golden` | en / ja × 80 / 120 / 160 の録画 |
| `internal/tui/review/review.go` | 提出ポップアップの Model / Update / View |
| `internal/tui/review/review_test.go` | キー操作と提出、golden |

### 変更

| ファイル | 変更 |
|---|---|
| `internal/tui/theme/theme.go` | diff の色（追加行 / 削除行 / ハンク見出し / スレッド）と `Highlight` |
| `internal/tui/app/app.go` | `showingDetail bool` → ビューのスタック。`OpenDiffMsg` の受け口 |
| `internal/tui/app/render.go` | スタックの一番上を描く |
| `internal/tui/work/work.go` | `d` で `OpenDiffMsg` を返す |
| `internal/tui/repo/repo.go` | 同上 |
| `internal/tui/detail/detail.go` | `d` で `OpenDiffMsg`、`v` で `OpenReviewMsg` |
| `internal/i18n/locales/active.{en,ja}.yaml` | `diff.*` / `submit.*` / `footer.diff` / `footer.submit` |
| `.golangci.yml` | `depguard` に `tui-diff` と `tui-review` |
| `go.mod` | chroma を直接依存へ |
| `.claude/rules/tui.md` | 色の節に「シンタックスハイライトは theme が返す」を追記 |
| `docs/superpowers/2026-09-06-phase1-followups.md` | Phase 2 で消化した宿題（`d` / `v` キー）に印 |

---
## Task 1: diff のドメイン型と unified diff のパーサ

**Files:**
- Create: `internal/gh/diff.go`
- Create: `internal/gh/cli/diff.go`
- Create: `internal/gh/cli/diff_test.go`
- Create: `internal/gh/cli/testdata/sample.diff`

**Interfaces:**
- Produces: `gh.FileDiff` / `gh.Hunk` / `gh.DiffLine` / `gh.FileStatus` / `gh.DiffLineKind` / `gh.DiffSide`、および `(*cli.Client).PRDiff(ctx context.Context, repo string, number int) ([]gh.FileDiff, error)`
- Consumes: 既存の `cli.Client.run` / `appendRepo` / `effectiveRepo`

### なぜ両側の行番号を持つのか

行コメントの投稿（Task 3）は `line`（そのファイルの行番号）と `side`（`LEFT` = 旧、`RIGHT` = 新）を要求する。削除された行にコメントを付けるなら旧側の行番号、追加された行なら新側の行番号が要る。描画のためだけなら文字列 1 本で足りるが、それでは `c` を押したときに何を送るか決められない。**だからパースの時点で両方を持たせる。**

- [ ] **Step 1: ドメイン型を書く**

`internal/gh/diff.go`:

```go
package gh

// FileStatus is what happened to one file in a diff.
type FileStatus int

const (
	FileModified FileStatus = iota
	FileAdded
	FileDeleted
	FileRenamed
)

// DiffLineKind separates the three kinds of line a unified diff holds.
type DiffLineKind int

const (
	LineContext DiffLineKind = iota
	LineAdded
	LineRemoved
)

// DiffSide names which version of a file a line or a comment belongs to.
// GitHub spells these LEFT and RIGHT; nothing outside the access layer sees
// those words.
type DiffSide int

const (
	SideRight DiffSide = iota // the new file; the default for a comment
	SideLeft                  // the old file
)

// DiffLine is one line of a hunk. OldLine and NewLine are that line's number
// in each version, and 0 where the version has no such line: a removed line
// has no NewLine, an added one no OldLine. Both are needed because posting a
// comment names a line number *and* the side it is on.
type DiffLine struct {
	Kind    DiffLineKind
	OldLine int
	NewLine int
	Text    string
}

// Hunk is one @@ block. Header is the whole @@ line as git wrote it,
// including the function context git appends after the second @@.
type Hunk struct {
	Header string
	Lines  []DiffLine
}

// FileDiff is one file's worth of a diff. OldPath is set only for a rename.
// A binary file has no hunks: git reports that it differs and nothing more.
type FileDiff struct {
	Path      string
	OldPath   string
	Status    FileStatus
	Additions int
	Deletions int
	Binary    bool
	Hunks     []Hunk
}

// Line returns the number to quote when commenting on a line, and the side
// it is on. A context line is quoted on the right: that is the version the
// comment is about.
func (l DiffLine) Line() (int, DiffSide) {
	if l.Kind == LineRemoved {
		return l.OldLine, SideLeft
	}
	return l.NewLine, SideRight
}
```

- [ ] **Step 2: 固定の diff を testdata に置く**

`internal/gh/cli/testdata/sample.diff`。**6 つの形をすべて含む**こと。

```
diff --git a/graph/walk.go b/graph/walk.go
index 1a2b3c4..5d6e7f8 100644
--- a/graph/walk.go
+++ b/graph/walk.go
@@ -12,7 +12,9 @@ func Walk(ctx context.Context, q string) error {
 	ctx, cancel := context.WithTimeout(ctx, d)
 	defer cancel()
-	if depth == 0 {
+	if depth <= 0 {
+		depth = defaultDepth
+	}
 	return nil
 }
@@ -40,3 +42,4 @@ func helper() {
 	_ = q
+	_ = depth
 }
diff --git a/graph/new.go b/graph/new.go
new file mode 100644
index 0000000..9f8e7d6
--- /dev/null
+++ b/graph/new.go
@@ -0,0 +1,2 @@
+package graph
+
diff --git a/graph/old.go b/graph/old.go
deleted file mode 100644
index 9f8e7d6..0000000
--- a/graph/old.go
+++ /dev/null
@@ -1 +0,0 @@
-package graph
diff --git a/docs/a.md b/docs/b.md
similarity index 92%
rename from docs/a.md
rename to docs/b.md
index aaa1111..bbb2222 100644
--- a/docs/a.md
+++ b/docs/b.md
@@ -1,2 +1,2 @@
-# a
+# b
 body
diff --git a/logo.png b/logo.png
index ccc3333..ddd4444 100644
Binary files a/logo.png and b/logo.png differ
diff --git a/noeol.txt b/noeol.txt
index eee5555..fff6666 100644
--- a/noeol.txt
+++ b/noeol.txt
@@ -1 +1 @@
-old
\ No newline at end of file
+new
\ No newline at end of file
```

**注意:** 1 つ目のファイルの 1 つ目のハンクは、コンテキスト 2 行 → 削除 1 行 → 追加 3 行 → コンテキスト 2 行。Step 4 の期待値はこの並びに一致する。testdata を変えるなら期待値も変えること。

- [ ] **Step 3: 失敗するテストを書く**

`internal/gh/cli/diff_test.go`:

```go
package cli

import (
	"context"
	"os"
	"testing"

	"github.com/kukv/octoscope/internal/gh"
)

func readSample(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/sample.diff")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func sampleFiles(t *testing.T) []gh.FileDiff {
	t.Helper()
	c := New("/w", "kukv/koto")
	c.run = func(context.Context, string, ...string) ([]byte, error) { return readSample(t), nil }
	files, err := c.PRDiff(context.Background(), "", 128)
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestPRDiffBuildsTheCommand(t *testing.T) {
	c := New("/w", "kukv/koto")
	var got []string
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return readSample(t), nil
	}
	if _, err := c.PRDiff(context.Background(), "", 128); err != nil {
		t.Fatal(err)
	}
	want := []string{"pr", "diff", "128", "--color", "never", "--repo", "kukv/koto"}
	if len(got) != len(want) {
		t.Fatalf("args = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("args = %v, want %v", got, want)
		}
	}
}

func TestPRDiffParsesEveryShape(t *testing.T) {
	files := sampleFiles(t)
	if len(files) != 6 {
		t.Fatalf("parsed %d files, want 6", len(files))
	}

	tests := []struct {
		name      string
		file      gh.FileDiff
		path      string
		oldPath   string
		status    gh.FileStatus
		additions int
		deletions int
		binary    bool
		hunks     int
	}{
		{"modified", files[0], "graph/walk.go", "", gh.FileModified, 4, 1, false, 2},
		{"added", files[1], "graph/new.go", "", gh.FileAdded, 2, 0, false, 1},
		{"deleted", files[2], "graph/old.go", "", gh.FileDeleted, 0, 1, false, 1},
		{"renamed", files[3], "docs/b.md", "docs/a.md", gh.FileRenamed, 1, 1, false, 1},
		{"binary", files[4], "logo.png", "", gh.FileModified, 0, 0, true, 0},
		{"no trailing newline", files[5], "noeol.txt", "", gh.FileModified, 1, 1, false, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := tt.file
			if f.Path != tt.path || f.OldPath != tt.oldPath {
				t.Errorf("path = %q (old %q), want %q (old %q)", f.Path, f.OldPath, tt.path, tt.oldPath)
			}
			if f.Status != tt.status {
				t.Errorf("status = %v, want %v", f.Status, tt.status)
			}
			if f.Additions != tt.additions || f.Deletions != tt.deletions {
				t.Errorf("+%d -%d, want +%d -%d", f.Additions, f.Deletions, tt.additions, tt.deletions)
			}
			if f.Binary != tt.binary {
				t.Errorf("binary = %v, want %v", f.Binary, tt.binary)
			}
			if len(f.Hunks) != tt.hunks {
				t.Errorf("%d hunks, want %d", len(f.Hunks), tt.hunks)
			}
		})
	}
}

// TestLineNumbersRunDownBothSides is the test the whole parser exists for: a
// comment posts to a line number on a side, so a wrong number here puts the
// comment on the wrong line of a real pull request.
func TestLineNumbersRunDownBothSides(t *testing.T) {
	hunk := sampleFiles(t)[0].Hunks[0]

	got := make([][3]int, 0, len(hunk.Lines))
	for _, l := range hunk.Lines {
		got = append(got, [3]int{int(l.Kind), l.OldLine, l.NewLine})
	}
	want := [][3]int{
		{int(gh.LineContext), 12, 12},
		{int(gh.LineContext), 13, 13},
		{int(gh.LineRemoved), 14, 0},
		{int(gh.LineAdded), 0, 14},
		{int(gh.LineAdded), 0, 15},
		{int(gh.LineAdded), 0, 16},
		{int(gh.LineContext), 15, 17},
		{int(gh.LineContext), 16, 18},
	}
	if len(got) != len(want) {
		t.Fatalf("%d lines, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestDiffLineNamesTheSideToCommentOn(t *testing.T) {
	tests := []struct {
		name string
		line gh.DiffLine
		num  int
		side gh.DiffSide
	}{
		{"removed lines quote the left", gh.DiffLine{Kind: gh.LineRemoved, OldLine: 14}, 14, gh.SideLeft},
		{"added lines quote the right", gh.DiffLine{Kind: gh.LineAdded, NewLine: 15}, 15, gh.SideRight},
		{"context quotes the right", gh.DiffLine{Kind: gh.LineContext, OldLine: 12, NewLine: 12}, 12, gh.SideRight},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			num, side := tt.line.Line()
			if num != tt.num || side != tt.side {
				t.Errorf("Line() = %d %v, want %d %v", num, side, tt.num, tt.side)
			}
		})
	}
}
```

- [ ] **Step 4: テストが落ちることを確かめる**

Run: `go test ./internal/gh/cli/ -run TestPRDiff -v`
Expected: FAIL（`c.PRDiff undefined`）

- [ ] **Step 5: パーサを実装する**

`internal/gh/cli/diff.go`:

```go
package cli

import (
	"bufio"
	"bytes"
	"context"
	"strconv"
	"strings"

	"github.com/kukv/octoscope/internal/gh"
)

// PRDiff returns the pull request's diff, one entry per file.
//
// --color never is passed explicitly: gh colours its output when it thinks a
// terminal is watching, and escape sequences in the middle of a line would
// break both the parser and every width calculation downstream.
func (c *Client) PRDiff(ctx context.Context, repo string, number int) ([]gh.FileDiff, error) {
	args := appendRepo(
		[]string{"pr", "diff", strconv.Itoa(number), "--color", "never"},
		c.effectiveRepo(repo),
	)
	out, err := c.run(ctx, c.dir, args...)
	if err != nil {
		return nil, err
	}
	return parseDiff(out), nil
}

// parseDiff reads a unified diff. It never fails: a line it does not
// recognise inside a hunk is dropped, and one outside a hunk is a header we
// have no use for. A diff that half-parses shows a file short; a parser that
// returns an error shows nothing at all, which is worse.
func parseDiff(b []byte) []gh.FileDiff {
	p := &diffParser{}
	s := bufio.NewScanner(bytes.NewReader(b))
	// A single diff line can be far longer than bufio's default 64KiB limit
	// (a minified bundle is one line), and a scanner that gives up mid-file
	// would drop every file after it.
	s.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for s.Scan() {
		p.line(s.Text())
	}
	return p.done()
}

// diffParser holds the walk's position: which file and hunk are open, and
// how far down each side of the file the next line falls.
type diffParser struct {
	files        []gh.FileDiff
	file         *gh.FileDiff
	hunk         *gh.Hunk
	oldNo, newNo int
}

func (p *diffParser) closeHunk() {
	if p.file != nil && p.hunk != nil {
		p.file.Hunks = append(p.file.Hunks, *p.hunk)
	}
	p.hunk = nil
}

func (p *diffParser) closeFile() {
	p.closeHunk()
	if p.file != nil {
		p.files = append(p.files, *p.file)
	}
	p.file = nil
}

func (p *diffParser) done() []gh.FileDiff {
	p.closeFile()
	return p.files
}

func (p *diffParser) line(line string) {
	switch {
	case strings.HasPrefix(line, "diff --git "):
		p.closeFile()
		p.file = &gh.FileDiff{Status: gh.FileModified, Path: pathFromGitHeader(line)}
	case p.file == nil:
		// Anything before the first "diff --git" is not ours.
	case strings.HasPrefix(line, "new file mode"):
		p.file.Status = gh.FileAdded
	case strings.HasPrefix(line, "deleted file mode"):
		p.file.Status = gh.FileDeleted
	case strings.HasPrefix(line, "rename from "):
		p.file.Status = gh.FileRenamed
		p.file.OldPath = strings.TrimPrefix(line, "rename from ")
	case strings.HasPrefix(line, "rename to "):
		p.file.Status = gh.FileRenamed
		p.file.Path = strings.TrimPrefix(line, "rename to ")
	case strings.HasPrefix(line, "Binary files "), strings.HasPrefix(line, "GIT binary patch"):
		p.file.Binary = true
	case strings.HasPrefix(line, "@@"):
		p.closeHunk()
		p.oldNo, p.newNo = hunkStarts(line)
		p.hunk = &gh.Hunk{Header: line}
	case p.hunk == nil:
		// --- / +++ / index, and anything else before the first hunk.
	case strings.HasPrefix(line, `\`):
		// "\ No newline at end of file" annotates the line above it; it is
		// not a line of the file.
	case strings.HasPrefix(line, "+"):
		p.add(gh.DiffLine{Kind: gh.LineAdded, NewLine: p.newNo, Text: line[1:]})
		p.newNo++
		p.file.Additions++
	case strings.HasPrefix(line, "-"):
		p.add(gh.DiffLine{Kind: gh.LineRemoved, OldLine: p.oldNo, Text: line[1:]})
		p.oldNo++
		p.file.Deletions++
	default:
		// A context line starts with a space. An empty line in the file can
		// arrive as the empty string when the trailing space was stripped in
		// transit, which is why this is the default rather than a " " case.
		p.add(gh.DiffLine{
			Kind: gh.LineContext, OldLine: p.oldNo, NewLine: p.newNo,
			Text: strings.TrimPrefix(line, " "),
		})
		p.oldNo++
		p.newNo++
	}
}

func (p *diffParser) add(l gh.DiffLine) { p.hunk.Lines = append(p.hunk.Lines, l) }

// pathFromGitHeader reads the new path out of `diff --git a/x b/y`. A path
// containing a space makes the two halves ambiguous, so the b/ half is taken
// from the last " b/" in the line, which is where git puts it.
func pathFromGitHeader(line string) string {
	rest := strings.TrimPrefix(line, "diff --git ")
	i := strings.LastIndex(rest, " b/")
	if i < 0 {
		return strings.TrimPrefix(rest, "a/")
	}
	return rest[i+len(" b/"):]
}

// hunkStarts reads the first line number of each side out of
// `@@ -12,7 +12,9 @@ func Walk(...)`.
func hunkStarts(header string) (oldNo, newNo int) {
	for _, f := range strings.Fields(header) {
		switch {
		case strings.HasPrefix(f, "-"):
			oldNo = firstNumber(f[1:])
		case strings.HasPrefix(f, "+"):
			newNo = firstNumber(f[1:])
		}
	}
	return oldNo, newNo
}

func firstNumber(s string) int {
	if i := strings.IndexByte(s, ','); i >= 0 {
		s = s[:i]
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0 // a header we cannot read still draws; it just numbers from 0.
	}
	return n
}
```

**注意:** ハンク見出しの `@@ -1 +0,0 @@` のように件数を省いた形と、`+++ b/x` のように `+` で始まる**ヘッダ**が、`@@` の case より後ろに来ないよう case の順序を守ること。`+++` は `p.hunk == nil` の case で落ちる。

- [ ] **Step 6: テストが通ることを確かめる**

Run: `go test ./internal/gh/cli/ -run 'TestPRDiff|TestLineNumbers|TestDiffLine' -v`
Expected: PASS

- [ ] **Step 7: アサーションが空振りしていないことを確かめる**

`p.oldNo++`（削除行の case）を一時的に消して `TestLineNumbersRunDownBothSides` が落ちることを目で見る。落ちないならそのテストは行番号を守っていない。確認したら戻す。

- [ ] **Step 8: `make check` を通してコミット**

```
make check
```

コミットメッセージ:

```
feat: read a pull request's diff, both sides numbered

A comment posts to a line number and a side, so a rendered string is not
enough: every line carries its number in each version of the file, and 0
where that version has no such line.
```

---
## Task 2: レビューのドメイン型と、pending review・スレッドの取得

**Files:**
- Create: `internal/gh/review.go`
- Create: `internal/gh/cli/review.go`
- Create: `internal/gh/cli/review.graphql`
- Create: `internal/gh/cli/review_test.go`

**Interfaces:**
- Consumes: Task 1 の `gh.DiffSide`
- Produces: `gh.ReviewThread` / `gh.ThreadComment` / `gh.ReviewContext` / `gh.PendingComment` / `gh.ReviewEvent`、および `(*cli.Client).PRReviewContext(ctx context.Context, repo string, number int) (gh.ReviewContext, error)`

### なぜ 1 つのクエリにまとめるのか

diff ビューが開くときに要るのは 3 つ — PR の node id（レビューを始めるのに要る）、未提出レビューの id（あれば引き継ぐ）、既存スレッド（行の下に出す）。3 回のサブプロセスに分けると、Phase 1 の `RepoName` の二重呼び出しと同じ無駄になる。**1 つのクエリで取る。**

未提出レビューのコメントも `reviewThreads` に現れる。そのコメントが属するレビューの `state` が `PENDING` かどうかで、自分の書きかけか既に世に出ているかを区別できる。**専用の問い合わせは要らない。**

- [ ] **Step 1: ドメイン型を書く**

`internal/gh/review.go`:

```go
package gh

import "time"

// ThreadComment is one comment inside a review thread. Pending marks a
// comment the viewer has written but not submitted: GitHub returns it in the
// same place as everyone else's, and only the state of the review it belongs
// to tells the two apart.
type ThreadComment struct {
	Author    Author
	Body      string
	CreatedAt time.Time
	Pending   bool
}

// ReviewThread is one conversation attached to a line of the diff.
//
// Line is the line it sits on in the version named by Side. GitHub returns no
// line for a thread whose code has since moved, in which case Outdated is set
// and Line is the line it was originally written against.
type ReviewThread struct {
	Path     string
	Line     int
	Side     DiffSide
	Resolved bool
	Outdated bool
	Comments []ThreadComment
}

// Pending reports whether this thread is one the viewer has not submitted.
// Such a thread has exactly one comment, and it is theirs.
func (t ReviewThread) Pending() bool {
	return len(t.Comments) > 0 && t.Comments[0].Pending
}

// Collapsed reports whether the thread is drawn as a count rather than in
// full. Settled conversations must not push the code they were about off the
// screen (spec 4.4.1).
func (t ReviewThread) Collapsed() bool { return t.Resolved || t.Outdated }

// PendingComment is a line comment on its way to GitHub.
type PendingComment struct {
	Path string
	Line int
	Side DiffSide
	Body string
}

// ReviewEvent is what submitting a review says about it.
type ReviewEvent int

const (
	EventComment ReviewEvent = iota
	EventApprove
	EventRequestChanges
)

// ReviewContext is everything the diff view needs before it can draw or
// change a review: the pull request's node id, the unsubmitted review if
// there is one, and the threads already on the diff.
type ReviewContext struct {
	PullRequestID string
	// PendingID is the unsubmitted review's node id, empty when there is
	// none. A pending review is visible only to its author, so anything that
	// comes back here belongs to the viewer.
	PendingID string
	Threads   []ReviewThread
}
```

- [ ] **Step 2: クエリを書く**

`internal/gh/cli/review.graphql`:

```graphql
query ($owner: String!, $name: String!, $number: Int!) {
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      id
      reviews(states: [PENDING], first: 1) {
        nodes {
          id
        }
      }
      reviewThreads(first: 100) {
        nodes {
          isResolved
          isOutdated
          path
          line
          originalLine
          diffSide
          comments(first: 50) {
            nodes {
              body
              createdAt
              author {
                login
              }
              pullRequestReview {
                state
              }
            }
          }
        }
      }
    }
  }
}
```

- [ ] **Step 3: 失敗するテストを書く**

`internal/gh/cli/review_test.go`:

```go
package cli

import (
	"context"
	"slices"
	"testing"

	"github.com/kukv/octoscope/internal/gh"
)

const reviewContextJSON = `{"data":{"repository":{"pullRequest":{
  "id":"PR_kwDO1",
  "reviews":{"nodes":[{"id":"PRR_kwDO9"}]},
  "reviewThreads":{"nodes":[
    {"isResolved":false,"isOutdated":false,"path":"graph/walk.go","line":14,
     "originalLine":14,"diffSide":"RIGHT","comments":{"nodes":[
       {"body":"is 2 not the default here?","createdAt":"2026-09-06T12:00:00Z",
        "author":{"login":"kukv"},"pullRequestReview":{"state":"COMMENTED"}}]}},
    {"isResolved":true,"isOutdated":false,"path":"graph/walk.go","line":12,
     "originalLine":12,"diffSide":"LEFT","comments":{"nodes":[
       {"body":"settled","createdAt":"2026-09-05T12:00:00Z",
        "author":{"login":"someone"},"pullRequestReview":{"state":"APPROVED"}}]}},
    {"isResolved":false,"isOutdated":true,"path":"graph/old.go","line":null,
     "originalLine":3,"diffSide":"RIGHT","comments":{"nodes":[
       {"body":"moved since","createdAt":"2026-09-04T12:00:00Z",
        "author":{"login":"someone"},"pullRequestReview":{"state":"COMMENTED"}}]}},
    {"isResolved":false,"isOutdated":false,"path":"graph/walk.go","line":16,
     "originalLine":16,"diffSide":"RIGHT","comments":{"nodes":[
       {"body":"mine, not sent yet","createdAt":"2026-09-06T13:00:00Z",
        "author":{"login":"kukv"},"pullRequestReview":{"state":"PENDING"}}]}}
  ]}
}}}}`

func TestPRReviewContextBuildsTheQuery(t *testing.T) {
	c := New("/w", "kukv/koto")
	var got []string
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return []byte(reviewContextJSON), nil
	}
	if _, err := c.PRReviewContext(context.Background(), "", 128); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"api", "graphql", "-f", "owner=kukv", "-f", "name=koto", "-F", "number=128"} {
		if !slices.Contains(got, want) {
			t.Errorf("args %v do not carry %q", got, want)
		}
	}
}

func TestPRReviewContextReadsTheAnswer(t *testing.T) {
	c := New("/w", "kukv/koto")
	c.run = func(context.Context, string, ...string) ([]byte, error) {
		return []byte(reviewContextJSON), nil
	}
	rc, err := c.PRReviewContext(context.Background(), "", 128)
	if err != nil {
		t.Fatal(err)
	}
	if rc.PullRequestID != "PR_kwDO1" {
		t.Errorf("pull request id = %q", rc.PullRequestID)
	}
	if rc.PendingID != "PRR_kwDO9" {
		t.Errorf("pending id = %q, want the unsubmitted review's", rc.PendingID)
	}
	if len(rc.Threads) != 4 {
		t.Fatalf("%d threads, want 4", len(rc.Threads))
	}

	tests := []struct {
		name      string
		thread    gh.ReviewThread
		line      int
		side      gh.DiffSide
		collapsed bool
		pending   bool
	}{
		{"open thread", rc.Threads[0], 14, gh.SideRight, false, false},
		{"resolved threads collapse", rc.Threads[1], 12, gh.SideLeft, true, false},
		{"outdated threads keep the line they were written against", rc.Threads[2], 3, gh.SideRight, true, false},
		{"the viewer's unsubmitted comment", rc.Threads[3], 16, gh.SideRight, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			th := tt.thread
			if th.Line != tt.line || th.Side != tt.side {
				t.Errorf("line %d side %v, want %d %v", th.Line, th.Side, tt.line, tt.side)
			}
			if th.Collapsed() != tt.collapsed {
				t.Errorf("Collapsed() = %v, want %v", th.Collapsed(), tt.collapsed)
			}
			if th.Pending() != tt.pending {
				t.Errorf("Pending() = %v, want %v", th.Pending(), tt.pending)
			}
		})
	}
}

func TestPRReviewContextWithNoPendingReview(t *testing.T) {
	c := New("/w", "kukv/koto")
	c.run = func(context.Context, string, ...string) ([]byte, error) {
		return []byte(`{"data":{"repository":{"pullRequest":{"id":"PR_1",
			"reviews":{"nodes":[]},"reviewThreads":{"nodes":[]}}}}}`), nil
	}
	rc, err := c.PRReviewContext(context.Background(), "", 128)
	if err != nil {
		t.Fatal(err)
	}
	if rc.PendingID != "" {
		t.Errorf("pending id = %q, want empty", rc.PendingID)
	}
}
```

- [ ] **Step 4: テストが落ちることを確かめる**

Run: `go test ./internal/gh/cli/ -run TestPRReviewContext -v`
Expected: FAIL（`c.PRReviewContext undefined`）

- [ ] **Step 5: 実装する**

`internal/gh/cli/review.go`:

```go
package cli

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/kukv/octoscope/internal/gh"
)

//go:embed review.graphql
var reviewContextQuery string

// splitRepo cuts "owner/name" in two. GraphQL's repository() takes the halves
// separately, unlike `gh pr` which takes the whole thing after --repo.
func splitRepo(repo string) (owner, name string) {
	owner, name, _ = strings.Cut(repo, "/")
	return owner, name
}

type reviewContextResponse struct {
	Data struct {
		Repository struct {
			PullRequest struct {
				ID      string `json:"id"`
				Reviews struct {
					Nodes []struct {
						ID string `json:"id"`
					} `json:"nodes"`
				} `json:"reviews"`
				ReviewThreads struct {
					Nodes []threadNode `json:"nodes"`
				} `json:"reviewThreads"`
			} `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
}

type threadNode struct {
	IsResolved bool   `json:"isResolved"`
	IsOutdated bool   `json:"isOutdated"`
	Path       string `json:"path"`
	// Line is null once the code a thread was written against has moved, so
	// it has to be a pointer to tell "no line" from "line 0".
	Line         *int   `json:"line"`
	OriginalLine int    `json:"originalLine"`
	DiffSide     string `json:"diffSide"`
	Comments     struct {
		Nodes []threadCommentNode `json:"nodes"`
	} `json:"comments"`
}

type threadCommentNode struct {
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
	Author    struct {
		Login string `json:"login"`
	} `json:"author"`
	PullRequestReview struct {
		State string `json:"state"`
	} `json:"pullRequestReview"`
}

// PRReviewContext fetches, in one request, everything the diff view needs to
// draw and change a review.
func (c *Client) PRReviewContext(ctx context.Context, repo string, number int) (gh.ReviewContext, error) {
	owner, name := splitRepo(c.effectiveRepo(repo))
	out, err := c.run(ctx, c.dir, "api", "graphql",
		"-f", "query="+reviewContextQuery,
		"-f", "owner="+owner,
		"-f", "name="+name,
		"-F", "number="+strconv.Itoa(number),
	)
	if err != nil {
		return gh.ReviewContext{}, err
	}
	var resp reviewContextResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return gh.ReviewContext{}, fmt.Errorf("parse review context: %w", err)
	}
	pr := resp.Data.Repository.PullRequest
	rc := gh.ReviewContext{PullRequestID: pr.ID}
	if len(pr.Reviews.Nodes) > 0 {
		rc.PendingID = pr.Reviews.Nodes[0].ID
	}
	for _, n := range pr.ReviewThreads.Nodes {
		rc.Threads = append(rc.Threads, n.toDomain())
	}
	return rc, nil
}

func (n threadNode) toDomain() gh.ReviewThread {
	t := gh.ReviewThread{
		Path:     n.Path,
		Line:     n.OriginalLine,
		Resolved: n.IsResolved,
		Outdated: n.IsOutdated,
	}
	if n.Line != nil {
		t.Line = *n.Line
	}
	if n.DiffSide == "LEFT" {
		t.Side = gh.SideLeft
	}
	for _, c := range n.Comments.Nodes {
		t.Comments = append(t.Comments, gh.ThreadComment{
			Author:    gh.Author{Login: c.Author.Login},
			Body:      c.Body,
			CreatedAt: c.CreatedAt,
			// PENDING is the only review state that means "written but not
			// sent"; every other one means the comment is already public.
			Pending: c.PullRequestReview.State == "PENDING",
		})
	}
	return t
}
```

- [ ] **Step 6: テストが通ることを確かめる**

Run: `go test ./internal/gh/cli/ -run TestPRReviewContext -v`
Expected: PASS

- [ ] **Step 7: アサーションが空振りしていないことを確かめる**

`toDomain` の `if n.Line != nil { t.Line = *n.Line }` を消して「outdated のケースだけ通り、他の 3 つが落ちる」ことを目で見る。3 つとも落ちるなら、`line` と `originalLine` の使い分けを本当に検証している。確認したら戻す。

- [ ] **Step 8: `make check` を通してコミット**

```
make check
```

コミットメッセージ:

```
feat: fetch a pull request's review threads and its unsubmitted review

One query, not three: the node id, the pending review and the threads all
arrive together, the way the Work board's four columns do.

A pending comment comes back beside everyone else's, and only the state of
the review it belongs to says which is which.
```

---

## Task 3: レビューを始める・行コメントを足す・提出する・破棄する

**Files:**
- Modify: `internal/gh/cli/review.go`
- Create: `internal/gh/cli/start_review.graphql`
- Create: `internal/gh/cli/add_thread.graphql`
- Create: `internal/gh/cli/submit_review.graphql`
- Create: `internal/gh/cli/discard_review.graphql`
- Modify: `internal/gh/cli/review_test.go`

**Interfaces:**
- Consumes: Task 2 の `gh.PendingComment` / `gh.ReviewEvent`
- Produces:
  - `(*cli.Client).StartReview(pullRequestID string) (string, error)`
  - `(*cli.Client).AddReviewThread(reviewID string, c gh.PendingComment) error`
  - `(*cli.Client).SubmitReview(reviewID string, event gh.ReviewEvent, body string) error`
  - `(*cli.Client).DiscardReview(reviewID string) error`

### context を通さない理由

`.claude/rules/go-style.md` は「**取得系**の関数は第一引数に `ctx`」としている。この 4 つは取得ではなく変更であり、既存の `AddPRComment` / `ClosePR` / `EditPRLabels` も `ctx` を取っていない。**途中でやめられる操作ではない**（送ったコメントは送られている）ので、揃える。

### なぜ mutation ごとにファイルを分けるのか

`gh api graphql` に operation 名を選ぶフラグが無い。1 つのファイルに 4 つの named operation を書くと、どれを実行するか指定できない。**1 ファイル 1 操作にする。**

- [ ] **Step 1: 4 つの mutation を書く**

`internal/gh/cli/start_review.graphql`:

```graphql
mutation ($pullRequestId: ID!) {
  addPullRequestReview(input: {pullRequestId: $pullRequestId}) {
    pullRequestReview {
      id
    }
  }
}
```

`internal/gh/cli/add_thread.graphql`:

```graphql
mutation ($reviewId: ID!, $path: String!, $line: Int!, $side: DiffSide!, $body: String!) {
  addPullRequestReviewThread(input: {
    pullRequestReviewId: $reviewId
    path: $path
    line: $line
    side: $side
    body: $body
  }) {
    thread {
      id
    }
  }
}
```

`internal/gh/cli/submit_review.graphql`:

```graphql
mutation ($reviewId: ID!, $event: PullRequestReviewEvent!, $body: String!) {
  submitPullRequestReview(input: {
    pullRequestReviewId: $reviewId
    event: $event
    body: $body
  }) {
    pullRequestReview {
      id
    }
  }
}
```

`internal/gh/cli/discard_review.graphql`:

```graphql
mutation ($reviewId: ID!) {
  deletePullRequestReview(input: {pullRequestReviewId: $reviewId}) {
    pullRequestReview {
      id
    }
  }
}
```

- [ ] **Step 2: 失敗するテストを書く**

`internal/gh/cli/review_test.go` に足す:

```go
func TestStartReviewReturnsTheNewReviewID(t *testing.T) {
	c := New("/w", "kukv/koto")
	var got []string
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return []byte(`{"data":{"addPullRequestReview":{"pullRequestReview":{"id":"PRR_new"}}}}`), nil
	}
	id, err := c.StartReview("PR_kwDO1")
	if err != nil {
		t.Fatal(err)
	}
	if id != "PRR_new" {
		t.Errorf("review id = %q, want PRR_new", id)
	}
	if !slices.Contains(got, "pullRequestId=PR_kwDO1") {
		t.Errorf("args %v do not carry the pull request id", got)
	}
}

func TestAddReviewThreadSendsTheLineAndTheSide(t *testing.T) {
	tests := []struct {
		name    string
		comment gh.PendingComment
		side    string
	}{
		{
			name:    "a comment on the new file",
			comment: gh.PendingComment{Path: "graph/walk.go", Line: 15, Side: gh.SideRight, Body: "why?"},
			side:    "side=RIGHT",
		},
		{
			name:    "a comment on a removed line",
			comment: gh.PendingComment{Path: "graph/walk.go", Line: 14, Side: gh.SideLeft, Body: "why?"},
			side:    "side=LEFT",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New("/w", "kukv/koto")
			var got []string
			c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
				got = args
				return []byte(`{"data":{"addPullRequestReviewThread":{"thread":{"id":"T_1"}}}}`), nil
			}
			if err := c.AddReviewThread("PRR_9", tt.comment); err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{
				"reviewId=PRR_9",
				"path=" + tt.comment.Path,
				"line=" + strconv.Itoa(tt.comment.Line),
				tt.side,
				"body=" + tt.comment.Body,
			} {
				if !slices.Contains(got, want) {
					t.Errorf("args %v do not carry %q", got, want)
				}
			}
		})
	}
}

func TestSubmitReviewNamesTheEvent(t *testing.T) {
	tests := []struct {
		name  string
		event gh.ReviewEvent
		want  string
	}{
		{"approve", gh.EventApprove, "event=APPROVE"},
		{"request changes", gh.EventRequestChanges, "event=REQUEST_CHANGES"},
		{"comment", gh.EventComment, "event=COMMENT"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New("/w", "kukv/koto")
			var got []string
			c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
				got = args
				return []byte(`{"data":{"submitPullRequestReview":{"pullRequestReview":{"id":"PRR_9"}}}}`), nil
			}
			if err := c.SubmitReview("PRR_9", tt.event, "looks good"); err != nil {
				t.Fatal(err)
			}
			if !slices.Contains(got, tt.want) {
				t.Errorf("args %v do not carry %q", got, tt.want)
			}
			if !slices.Contains(got, "body=looks good") {
				t.Errorf("args %v do not carry the body", got)
			}
		})
	}
}

func TestDiscardReviewNamesTheReview(t *testing.T) {
	c := New("/w", "kukv/koto")
	var got []string
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return []byte(`{"data":{"deletePullRequestReview":{"pullRequestReview":{"id":"PRR_9"}}}}`), nil
	}
	if err := c.DiscardReview("PRR_9"); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(got, "reviewId=PRR_9") {
		t.Errorf("args %v do not carry the review id", got)
	}
}
```

`import` に `"strconv"` を足すこと。

- [ ] **Step 3: テストが落ちることを確かめる**

Run: `go test ./internal/gh/cli/ -run 'TestStartReview|TestAddReviewThread|TestSubmitReview|TestDiscardReview' -v`
Expected: FAIL（4 つとも undefined）

- [ ] **Step 4: 実装する**

`internal/gh/cli/review.go` に足す:

```go
//go:embed start_review.graphql
var startReviewMutation string

//go:embed add_thread.graphql
var addThreadMutation string

//go:embed submit_review.graphql
var submitReviewMutation string

//go:embed discard_review.graphql
var discardReviewMutation string

// The four mutations take no context. They are changes, not fetches: a
// comment that has been sent has been sent, so there is nothing to abandon
// half-way. The existing AddPRComment and ClosePR take none for the same
// reason (.claude/rules/go-style.md).

// StartReview opens an unsubmitted review on the pull request and returns its
// node id. A pending review is visible only to its author, so this is the id
// the rest of the session adds comments to.
func (c *Client) StartReview(pullRequestID string) (string, error) {
	out, err := c.run(context.Background(), c.dir, "api", "graphql",
		"-f", "query="+startReviewMutation,
		"-f", "pullRequestId="+pullRequestID,
	)
	if err != nil {
		return "", err
	}
	var resp struct {
		Data struct {
			AddPullRequestReview struct {
				PullRequestReview struct {
					ID string `json:"id"`
				} `json:"pullRequestReview"`
			} `json:"addPullRequestReview"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", fmt.Errorf("parse start review: %w", err)
	}
	return resp.Data.AddPullRequestReview.PullRequestReview.ID, nil
}

// apiSide spells a side the way the GraphQL DiffSide enum does. It is the one
// place that knows those words (.claude/rules/architecture.md).
func apiSide(s gh.DiffSide) string {
	if s == gh.SideLeft {
		return "LEFT"
	}
	return "RIGHT"
}

// apiEvent spells an event the way PullRequestReviewEvent does.
func apiEvent(e gh.ReviewEvent) string {
	switch e {
	case gh.EventApprove:
		return "APPROVE"
	case gh.EventRequestChanges:
		return "REQUEST_CHANGES"
	default:
		return "COMMENT"
	}
}

// AddReviewThread attaches one line comment to an unsubmitted review.
func (c *Client) AddReviewThread(reviewID string, comment gh.PendingComment) error {
	_, err := c.run(context.Background(), c.dir, "api", "graphql",
		"-f", "query="+addThreadMutation,
		"-f", "reviewId="+reviewID,
		"-f", "path="+comment.Path,
		"-F", "line="+strconv.Itoa(comment.Line),
		"-f", "side="+apiSide(comment.Side),
		"-f", "body="+comment.Body,
	)
	return err
}

// SubmitReview sends the unsubmitted review, with every comment on it.
func (c *Client) SubmitReview(reviewID string, event gh.ReviewEvent, body string) error {
	_, err := c.run(context.Background(), c.dir, "api", "graphql",
		"-f", "query="+submitReviewMutation,
		"-f", "reviewId="+reviewID,
		"-f", "event="+apiEvent(event),
		"-f", "body="+body,
	)
	return err
}

// DiscardReview throws the unsubmitted review away, comments and all.
func (c *Client) DiscardReview(reviewID string) error {
	_, err := c.run(context.Background(), c.dir, "api", "graphql",
		"-f", "query="+discardReviewMutation,
		"-f", "reviewId="+reviewID,
	)
	return err
}
```

**`-F` と `-f` の使い分けは間違えると本番のデータを壊す。** 確認済みの事実は次のとおり。

- `-F, --field` は**値を解釈する**。`true` / `false` / `null` と整数は JSON のその型に変換され、
  **`@<path>` で始まる値はファイルの中身に置き換えられる**（`@-` は標準入力）
- `-f, --raw-field` は**常に文字列**として送り、何も解釈しない

したがって:

| 変数 | フラグ | 理由 |
|---|---|---|
| `number` / `line` | `-F` | GraphQL の型が `Int!`。文字列を送ると型エラーになる |
| `owner` / `name` / `path` / `body` / `side` / 各種 id | `-f` | 文字列であり、**利用者の入力を含むものは絶対に解釈させない** |

`@` で始まるコメント本文（`@kukv please look`）は現実にあり、`-F` で送ると
**ローカルのファイルを読んで PR に投稿してしまう。** `-f` はこれを解釈しない。

なお引数はいずれも**フラグの値として渡すのでシェルを経由しない**（`.claude/rules/go-style.md`）。

- [ ] **Step 5: `@` で始まる本文が解釈されないことをテストで縛る**

```go
func TestABodyThatStartsWithAtIsNotReadAsAFile(t *testing.T) {
	c := New("/w", "kukv/koto")
	var got []string
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return []byte(`{"data":{"submitPullRequestReview":{"pullRequestReview":{"id":"PRR_9"}}}}`), nil
	}
	if err := c.SubmitReview("PRR_9", gh.EventComment, "@kukv please look"); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(got, "body=@kukv please look") {
		t.Errorf("args %v do not carry the body verbatim", got)
	}
}
```

このテストが守るのは「本文が `-f` の値としてそのまま渡ること」である。`-f` が
`@` を解釈しないことは `gh api --help` で確認済みだが、**`-F` に書き換えられたら
このテストは通ったままになる。** そこで `SubmitReview` の `-f` を `-F` に変えて
`TestSubmitReviewNamesTheEvent` が落ちないことを確認したうえで、
**`grep -n '"-F"' internal/gh/cli/review.go` の結果が `number` と `line` の 2 行だけである**
ことを目で見る。これは lint では縛れないので、レビューで見る。

- [ ] **Step 6: テストが通ることを確かめる**

Run: `go test ./internal/gh/cli/ -v`
Expected: PASS

- [ ] **Step 7: `make check` を通してコミット**

```
make check
```

コミットメッセージ:

```
feat: start, extend, submit and discard a review

Four mutations, four files: gh api graphql has no flag for choosing an
operation by name, so one file cannot hold them all.

None of them takes a context. They are changes rather than fetches -- a
comment that has been sent has been sent -- which is why the existing
AddPRComment and ClosePR take none either.
```

---
## Task 4: theme に diff の色とシンタックスハイライトを置く

**Files:**
- Modify: `internal/tui/theme/theme.go`
- Modify: `internal/tui/theme/theme_test.go`
- Modify: `go.mod` / `go.sum`（chroma を直接依存へ）
- Modify: `.claude/rules/tui.md`

**Interfaces:**
- Produces:
  - `theme.DiffAdded() lipgloss.Style` / `theme.DiffRemoved() lipgloss.Style` / `theme.DiffContext() lipgloss.Style`
  - `theme.HunkHeader() lipgloss.Style` / `theme.LineNumber() lipgloss.Style`
  - `theme.Thread(pending bool) lipgloss.Style`
  - `theme.Highlight(path, code string) string`

### 規約との折り合い

`.claude/rules/tui.md` は「色は `internal/tui/theme` にだけ書く」。chroma のスタイルは
配色そのものなので、ビューが chroma を呼ぶとこの規約が崩れる。**`theme` が
ハイライト済みの文字列を返し、ビューは chroma を import しない。**
spec §4.5 に既にこの旨が書いてある。**このタスクで rules も同じ内容に更新する。**

### 1 行ずつハイライトすることの限界

diff から得られるのは行の断片であり、ファイル全体ではない。複数行にまたがる
文字列やコメントは、1 行だけ見ると閉じていないので色が付かないことがある。
**それでよい。** ファイル全体を別に取りに行くのは、diff を読むために払う代償として高い。
この判断は `Highlight` の doc コメントに書く。

- [ ] **Step 1: 失敗するテストを書く**

`internal/tui/theme/theme_test.go` に足す:

```go
func TestHighlightColoursCodeItKnows(t *testing.T) {
	got := theme.Highlight("walk.go", "func Walk() {}")
	if !strings.Contains(got, "\x1b[") {
		t.Errorf("Highlight returned no escapes for Go source: %q", got)
	}
	if ansi.Strip(got) != "func Walk() {}" {
		t.Errorf("Highlight changed the text: %q", ansi.Strip(got))
	}
}

func TestHighlightLeavesUnknownFilesAlone(t *testing.T) {
	const line = "some prose"
	if got := theme.Highlight("NOTES", line); got != line {
		t.Errorf("Highlight(%q) = %q, want it untouched", line, got)
	}
}

// TestHighlightKeepsTheWidth is what stops highlighting from breaking every
// column downstream: escapes must not count towards the width, and the text
// must come back rune for rune.
func TestHighlightKeepsTheWidth(t *testing.T) {
	for _, line := range []string{"func Walk() {}", "\t// 日本語のコメント", ""} {
		got := theme.Highlight("walk.go", line)
		if ansi.StringWidth(got) != ansi.StringWidth(line) {
			t.Errorf("Highlight(%q) is %d columns, want %d",
				line, ansi.StringWidth(got), ansi.StringWidth(line))
		}
	}
}
```

- [ ] **Step 2: テストが落ちることを確かめる**

Run: `go test ./internal/tui/theme/ -run TestHighlight -v`
Expected: FAIL（`theme.Highlight undefined`）

- [ ] **Step 3: 実装する**

`internal/tui/theme/theme.go` に足す:

```go
// Diff colours. GitHub's own values for an added and a removed line, so a
// diff reads here the way it reads in the web UI.
func DiffAdded() lipgloss.Style   { return fg("#1a7f37", "#3fb950") }
func DiffRemoved() lipgloss.Style { return fg("#cf222e", "#f85149") }
func DiffContext() lipgloss.Style { return lipgloss.NewStyle() }

// HunkHeader styles the @@ line. It is a signpost rather than code, so it
// takes the muted colour and a weight of its own.
func HunkHeader() lipgloss.Style { return muted().Bold(true) }

// LineNumber styles the two numbers down the left of the diff.
func LineNumber() lipgloss.Style { return muted() }

// Thread styles a review comment drawn under the line it is about. A comment
// the user has not submitted yet is marked apart from one everyone can see:
// the difference decides whether pressing X would throw it away.
func Thread(pending bool) lipgloss.Style {
	if pending {
		return attention()
	}
	return accent()
}

// chromaStyle names the syntax-highlighting palette for each background.
// A chroma style *is* a palette, so it belongs here rather than in a view
// (.claude/rules/tui.md).
func chromaStyle() string {
	mu.RLock()
	defer mu.RUnlock()
	return lightDark("github", "github-dark")
}

// Highlight colours one line of source, chosen by the file's name.
//
// It is one line at a time because a diff is all we have: a string or a
// comment that opens on an earlier line looks unterminated here and simply
// goes uncoloured. Fetching whole files to colour a diff would cost a request
// per file, which is more than the colour is worth.
//
// A file chroma has no lexer for, and any failure inside chroma, comes back
// unchanged: the diff is still readable without colour.
func Highlight(path, code string) string {
	lexer := lexers.Match(path)
	if lexer == nil {
		return code
	}
	style := styles.Get(chromaStyle())
	formatter := formatters.Get("terminal256")
	if style == nil || formatter == nil {
		return code
	}
	iter, err := lexer.Tokenise(nil, code)
	if err != nil {
		return code
	}
	var buf strings.Builder
	if err := formatter.Format(&buf, style, iter); err != nil {
		return code
	}
	return buf.String()
}
```

import に足すもの:

```go
	"strings"

	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
```

- [ ] **Step 4: chroma を直接依存へ昇格し、テストを通す**

```
go get github.com/alecthomas/chroma/v2
go mod tidy
go test ./internal/tui/theme/ -run TestHighlight -v
```

Expected: PASS。`go.mod` の `github.com/alecthomas/chroma/v2` から `// indirect` が消える。

**注意:** `lexers.Match` は末尾に改行を含む出力を返すことがある。`TestHighlightKeepsTheWidth` の空文字列のケースがそれを捕まえる。落ちるなら `strings.TrimRight(buf.String(), "\n")` を挟む。

- [ ] **Step 5: アサーションが空振りしていないことを確かめる**

`Highlight` を `return code` だけの実装に一時的に戻し、`TestHighlightColoursCodeItKnows` が落ちることを目で見る。

- [ ] **Step 6: rules を更新する**

`.claude/rules/tui.md` の「## 色」の節の末尾に足す:

```markdown
**シンタックスハイライトも theme が返す。** chroma のスタイルは配色そのものなので、
ビューが chroma を呼ぶとこの規約が崩れる。ビューは `theme.Highlight(path, code)` を
呼び、chroma を import しない。スタイル名（明背景 / 暗背景）は theme が持つ。
```

- [ ] **Step 7: `make check` を通してコミット**

```
make check
```

コミットメッセージ:

```
feat: let the palette colour source, one line at a time

A chroma style is a palette, so naming one is the theme's business and not a
view's -- the rule that keeps hex out of views would not survive a view that
imports chroma.

One line at a time is all a diff allows: a string that opens on an earlier
line looks unterminated here and goes uncoloured. Fetching whole files to
colour a diff costs a request each, which is more than the colour is worth.
```

---

## Task 5: diff ビューアの骨格 — 取得・ファイルサイドバー・diff ペイン

**Files:**
- Create: `internal/tui/diff/diff.go`
- Create: `internal/tui/diff/render.go`
- Create: `internal/tui/diff/diff_test.go`
- Create: `internal/tui/diff/golden_test.go`
- Create: `internal/tui/diff/testdata/*.golden`
- Modify: `internal/i18n/locales/active.en.yaml` / `active.ja.yaml`
- Modify: `.golangci.yml`（`depguard` に `tui-diff`）

**Interfaces:**
- Consumes: Task 1 の `gh.FileDiff` / `gh.DiffLine`、Task 4 の `theme.*`
- Produces:
  - `diff.Source` interface（`PRDiff(ctx, repo string, number int) ([]gh.FileDiff, error)`）
  - `diff.New(src Source, ref gh.ItemRef, title, head, base string) Model`
  - `diff.Model` の `Init` / `Update` / `View() string`
  - `diff.ClosedMsg` / `diff.ErrorMsg`

### レイアウト（spec §4.4.1）

```
行 1        ヘッダ 1: owner/name #番号 タイトル
行 2        ヘッダ 2: head → base · N files +X −Y
行 3        罫線 + "Files" 見出し
行 4..n-1   サイドバー（左 22 桁）│ diff（残り）
行 n        キーバー
```

- サイドバー幅は **22 桁固定**。1 行目にパス（右から切り詰め）、2 行目に `+X −Y`
- diff の行は `旧 新 記号 本文`。行番号は各 4 桁の右詰め、記号 1 桁、区切りの空白で
  **合計 11 桁**を使う。本文は残りを `ansi.Truncate` で切る
- カーソルのある行は `theme.Selected()` の背景。**盤面と同じく、カーソルのある
  ペインだけがスクロールする**

- [ ] **Step 1: カタログに文字列を足す**

`internal/i18n/locales/active.en.yaml`:

```yaml
diff:
  files:
    other: "Files"
  loading:
    other: "loading the diff"
  no_changes:
    other: "no files changed"
  binary:
    other: "binary file"
  file_count:
    one: "{{.Count}} file"
    other: "{{.Count}} files"
```

`footer:` の下に:

```yaml
  diff:
    other: "j/k:line  [/]:file  {/}:hunk  h/l:pane  r:refresh  esc:back"
```

`active.ja.yaml` の同じ場所に:

```yaml
diff:
  files:
    other: "ファイル"
  loading:
    other: "diff を取得中"
  no_changes:
    other: "変更されたファイルはありません"
  binary:
    other: "バイナリファイル"
  file_count:
    other: "{{.Count}} ファイル"
```

```yaml
  diff:
    other: "j/k:行  [/]:ファイル  {/}:ハンク  h/l:ペイン  r:再取得  esc:戻る"
```

**`c` と `v` はまだ出さない。** Task 7 と Task 9 でキーバーに足す。出ていない機能を
キーバーに書くと、押した利用者に何も起きない。

- [ ] **Step 2: depguard に足す**

`.golangci.yml` の `settings.depguard.rules` に、既存の `tui-work` / `tui-repo` と
同じ形で `tui-diff` を足す。**兄弟ビュー（`work` / `repo` / `detail` / `review`）と
親（`app`）を deny する。** 同時に、既存の `tui-work` / `tui-repo` / `tui-detail` の
deny リストにも `internal/tui/diff` を足す（新しい兄弟が増えたため）。

- [ ] **Step 3: 失敗するテストを書く**

`internal/tui/diff/diff_test.go`:

```go
package diff

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/gh"
)

type fakeSource struct {
	files []gh.FileDiff
	err   error
}

func (f *fakeSource) PRDiff(context.Context, string, int) ([]gh.FileDiff, error) {
	return f.files, f.err
}

// fixture is two files, so that moving between files is testable, with a
// second hunk in the first so that hunk movement is too.
func fixture() []gh.FileDiff {
	return []gh.FileDiff{
		{
			Path: "graph/walk.go", Status: gh.FileModified, Additions: 4, Deletions: 1,
			Hunks: []gh.Hunk{
				{
					Header: "@@ -12,7 +12,9 @@ func Walk(ctx context.Context, q string) error {",
					Lines: []gh.DiffLine{
						{Kind: gh.LineContext, OldLine: 12, NewLine: 12, Text: "\tctx, cancel := context.WithTimeout(ctx, d)"},
						{Kind: gh.LineRemoved, OldLine: 13, Text: "\tif depth == 0 {"},
						{Kind: gh.LineAdded, NewLine: 13, Text: "\tif depth <= 0 {"},
						{Kind: gh.LineAdded, NewLine: 14, Text: "\t\tdepth = defaultDepth"},
					},
				},
				{
					Header: "@@ -40,3 +42,4 @@ func helper() {",
					Lines: []gh.DiffLine{
						{Kind: gh.LineContext, OldLine: 40, NewLine: 42, Text: "\t_ = q"},
						{Kind: gh.LineAdded, NewLine: 43, Text: "\t_ = depth"},
					},
				},
			},
		},
		{
			Path: "logo.png", Status: gh.FileModified, Binary: true,
		},
	}
}

func loaded(t *testing.T, width, height int) Model {
	t.Helper()
	m := New(&fakeSource{files: fixture()},
		gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/koto", Number: 128},
		"feat: add relation graph traversal", "feat/graph", "main")
	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	m, _ = m.Update(diffMsg{ref: m.ref, files: fixture()})
	return m
}

func press(m Model, key string) Model {
	m, _ = m.Update(tea.KeyPressMsg{Code: []rune(key)[0], Text: key})
	return m
}

func TestTheFirstFileIsShown(t *testing.T) {
	out := ansi.Strip(loaded(t, 120, 30).View())
	for _, want := range []string{"graph/walk.go", "logo.png", "func Walk", "if depth <= 0 {"} {
		if !strings.Contains(out, want) {
			t.Errorf("the view does not show %q:\n%s", want, out)
		}
	}
}

func TestBracketsMoveBetweenFiles(t *testing.T) {
	m := press(loaded(t, 120, 30), "]")
	if m.file != 1 {
		t.Fatalf("file = %d, want 1", m.file)
	}
	if !strings.Contains(ansi.Strip(m.View()), "binary file") {
		t.Errorf("the binary file does not say so:\n%s", ansi.Strip(m.View()))
	}
	m = press(m, "[")
	if m.file != 0 {
		t.Errorf("file = %d, want back at 0", m.file)
	}
}

func TestBracesMoveBetweenHunks(t *testing.T) {
	m := loaded(t, 120, 30)
	m = press(m, "}")
	if got := m.rows[m.row].hunk; got != 1 {
		t.Errorf("after } the cursor is in hunk %d, want 1", got)
	}
	m = press(m, "{")
	if got := m.rows[m.row].hunk; got != 0 {
		t.Errorf("after { the cursor is back in hunk %d, want 0", got)
	}
}

func TestTheCursorStaysInsideTheFile(t *testing.T) {
	m := loaded(t, 120, 30)
	for range 100 {
		m = press(m, "j")
	}
	if m.row >= len(m.rows) {
		t.Fatalf("row = %d, past the last of %d rows", m.row, len(m.rows))
	}
	for range 100 {
		m = press(m, "k")
	}
	if m.row < 0 {
		t.Fatalf("row = %d, before the first", m.row)
	}
}

func TestChangingFileResetsTheCursor(t *testing.T) {
	m := loaded(t, 120, 30)
	m = press(m, "j")
	m = press(m, "j")
	m = press(m, "]")
	if m.row != 0 {
		t.Errorf("row = %d after changing file, want 0", m.row)
	}
}

func TestEscAsksTheParentToClose(t *testing.T) {
	m := loaded(t, 120, 30)
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'x', Text: "esc"})
	if cmd == nil {
		t.Fatal("esc produced no command")
	}
	if _, ok := cmd().(ClosedMsg); !ok {
		t.Errorf("esc produced %T, want ClosedMsg", cmd())
	}
}

// TestAnswersForAnotherPullRequestAreDropped mirrors the detail view: the
// request for the item the user just left is still running when the next one
// opens, and its answer must not land here.
func TestAnswersForAnotherPullRequestAreDropped(t *testing.T) {
	m := loaded(t, 120, 30)
	other := gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/koto", Number: 999}
	before := len(m.files)
	m, _ = m.Update(diffMsg{ref: other, files: nil})
	if len(m.files) != before {
		t.Errorf("an answer for %v replaced this view's %d files", other, before)
	}
}
```

**注意:** `tea.KeyPressMsg` の組み立て方は既存の `internal/tui/work/work_test.go` の
`press` ヘルパーに合わせること。上のものはそのまま動かない可能性がある。
**既存のヘルパーをコピーして使う。**

- [ ] **Step 4: テストが落ちることを確かめる**

Run: `go test ./internal/tui/diff/ -v`
Expected: FAIL（パッケージが無い）

- [ ] **Step 5: Model と Update を書く**

`internal/tui/diff/diff.go`:

```go
// Package diff shows one pull request's diff: the files it touches down the
// left, the changes to the selected one on the right, and the review threads
// that hang off its lines.
package diff

import (
	"context"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/kukv/octoscope/internal/gh"
)

// Source is what the diff view needs from the GitHub layer. repo is
// "owner/repo"; the empty string targets the workspace repository.
type Source interface {
	PRDiff(ctx context.Context, repo string, number int) ([]gh.FileDiff, error)
}

// ClosedMsg tells the parent the user left the diff view.
type ClosedMsg struct{}

// ErrorMsg carries a failure the parent shows on its error screen.
type ErrorMsg struct{ Err error }

type diffMsg struct {
	ref   gh.ItemRef
	files []gh.FileDiff
}

type errMsg struct {
	ref gh.ItemRef
	err error
}

// rowKind separates the three things a line of the diff pane can be.
type rowKind int

const (
	rowHunkHeader rowKind = iota
	rowLine
	rowNote // "binary file", "no files changed": text with nothing behind it
)

// row is one drawable line of the diff pane. hunk is the index of the hunk it
// belongs to, which is what { and } move between; a note belongs to none and
// carries -1.
type row struct {
	kind rowKind
	hunk int
	line gh.DiffLine
	text string
}

type Model struct {
	src   Source
	ref   gh.ItemRef
	title string
	head  string
	base  string

	width, height int

	loading bool
	spin    spinner.Model

	files []gh.FileDiff
	file  int

	// rows is the current file, flattened for drawing. It is rebuilt whenever
	// the file changes rather than on every draw, because View may do no work
	// that a state change did not ask for.
	rows []row
	row  int
	top  int // the first row on screen

	// sidebar is where the cursor is: false in the diff pane, true in the
	// file list. h and l move between them.
	sidebar bool

	cancel context.CancelFunc
}

func New(src Source, ref gh.ItemRef, title, head, base string) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return Model{
		src: src, ref: ref, title: title, head: head, base: base,
		loading: true, spin: s,
	}
}

// Init starts the fetch. Unlike the Work board this view is built once per
// pull request, so there is no refresh whose cancel function has to outlive
// an Init with a value receiver.
func (m Model) Init() tea.Cmd { return tea.Batch(m.spin.Tick, m.fetch()) }

func (m Model) fetch() tea.Cmd {
	src, ref := m.src, m.ref
	return func() tea.Msg {
		files, err := src.PRDiff(context.Background(), ref.Repo, ref.Number)
		if err != nil {
			return errMsg{ref: ref, err: err}
		}
		return diffMsg{ref: ref, files: files}
	}
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case diffMsg:
		// The request for the pull request the user just left is still in
		// flight; its answer must not replace this one's.
		if msg.ref != m.ref {
			return m, nil
		}
		m.loading = false
		m.files = msg.files
		m.file, m.row, m.top = 0, 0, 0
		m.rows = m.buildRows()
		return m, nil
	case errMsg:
		if msg.ref != m.ref {
			return m, nil
		}
		m.loading = false
		return m, func() tea.Msg { return ErrorMsg{Err: msg.err} }
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		return m, func() tea.Msg { return ClosedMsg{} }
	case "r":
		m.loading = true
		return m, tea.Batch(m.spin.Tick, m.fetch())
	case "j", "down":
		return m.moveRow(1), nil
	case "k", "up":
		return m.moveRow(-1), nil
	case "]":
		return m.moveFile(1), nil
	case "[":
		return m.moveFile(-1), nil
	case "}":
		return m.moveHunk(1), nil
	case "{":
		return m.moveHunk(-1), nil
	case "h":
		m.sidebar = true
		return m, nil
	case "l":
		m.sidebar = false
		return m, nil
	}
	return m, nil
}

// buildRows flattens the selected file into the lines the diff pane draws.
func (m Model) buildRows() []row {
	if len(m.files) == 0 {
		return []row{{kind: rowNote, hunk: -1, text: i18n.T("diff.no_changes")}}
	}
	f := m.files[m.file]
	if f.Binary {
		return []row{{kind: rowNote, hunk: -1, text: i18n.T("diff.binary")}}
	}
	var rows []row
	for i, h := range f.Hunks {
		rows = append(rows, row{kind: rowHunkHeader, hunk: i, text: h.Header})
		for _, l := range h.Lines {
			rows = append(rows, row{kind: rowLine, hunk: i, line: l})
		}
	}
	return rows
}

func (m Model) moveRow(delta int) Model {
	if m.sidebar {
		return m.moveFile(delta)
	}
	m.row = clamp(m.row+delta, 0, len(m.rows)-1)
	return m.follow()
}

func (m Model) moveFile(delta int) Model {
	if len(m.files) == 0 {
		return m
	}
	m.file = clamp(m.file+delta, 0, len(m.files)-1)
	m.row, m.top = 0, 0
	m.rows = m.buildRows()
	return m
}

// moveHunk puts the cursor on the header of the next or previous hunk. It
// moves between hunks rather than by a fixed number of lines, which is the
// whole point of the key: a hunk is as long as it is.
func (m Model) moveHunk(delta int) Model {
	if len(m.rows) == 0 {
		return m
	}
	want := m.rows[m.row].hunk + delta
	for i, r := range m.rows {
		if r.kind == rowHunkHeader && r.hunk == want {
			m.row = i
			return m.follow()
		}
	}
	return m
}

// follow scrolls the window so the cursor stays on it. Only the pane the
// cursor is in scrolls, the same rule the Work board follows.
func (m Model) follow() Model {
	h := m.paneHeight()
	if m.row < m.top {
		m.top = m.row
	}
	if m.row >= m.top+h {
		m.top = m.row - h + 1
	}
	m.top = max(m.top, 0)
	return m
}

func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	return min(max(v, lo), hi)
}
```

`i18n` の import を足すこと。

- [ ] **Step 6: View を書く**

`internal/tui/diff/render.go` に、上のレイアウトのとおり書く。守ること:

- **桁は `ansi.StringWidth` と `ansi.Truncate` で数える。** `len` を使わない
- `sidebarWidth = 22`、`gutterWidth = 11`（旧 4 + 空白 1 + 新 4 + 記号 1 + 空白 1）を
  **定数にし、当たり判定（Task 11）が同じ定数を読む**
- `paneHeight()` = `m.height - headerHeight - keyBarHeight`。**ヘッダとキーバーは
  固定高。盤面が伸びて押し出してはならない**（Phase 1 の高さ予算と同じ）
- 色は `theme.DiffAdded()` / `DiffRemoved()` / `HunkHeader()` / `LineNumber()`、
  本文は `theme.Highlight(f.Path, l.Text)`。**16 進を書かない**
- `View()` は時計を読まない

- [ ] **Step 7: golden を録る**

`internal/tui/diff/golden_test.go` は `internal/tui/work/golden_test.go` を写して作る。
en / ja × 80 / 120 / 160。**日本語のコメントを含む行を fixture に足す**（桁ずれを捕まえるため）。

```
make golden
cat -v internal/tui/diff/testdata/diff_ja_80.golden
```

**diff を目で見てからコミットする。** 80 桁で本文が何桁残るかを数えること
（22 + 1 + 11 = 34 桁が本文の前に消える。80 桁なら本文は 46 桁）。

- [ ] **Step 8: 画面が端末に収まることをテストで縛る**

```go
func TestTheDiffFitsTheTerminal(t *testing.T) {
	for _, height := range []int{24, 40} {
		m := loaded(t, 120, height)
		out := m.View()
		if got := len(strings.Split(out, "\n")); got > height {
			t.Errorf("the diff drew %d lines into a terminal %d high", got, height)
		}
		if !strings.Contains(ansi.Strip(out), ansi.Strip(m.keyBar())) {
			t.Errorf("the key bar was pushed off the screen:\n%s", ansi.Strip(out))
		}
	}
}
```

- [ ] **Step 9: `make check` を通してコミット**

```
make check
```

コミットメッセージ:

```
feat: read a pull request's diff without leaving the terminal

Files down the left, the selected one's changes on the right, the way the
Repos tab is built. Unified only: at 80 columns a split leaves 40 a side,
which is not enough to read code in.

The pane is given a height budget rather than the whole screen, so a long
file cannot push the key bar off the bottom the way the board once did.
```

---

## Task 6: ルートモデルのナビゲーションと `d` の入口

**Files:**
- Modify: `internal/tui/app/app.go`
- Modify: `internal/tui/app/render.go`
- Modify: `internal/tui/app/app_test.go`
- Modify: `internal/tui/work/work.go` / `work_test.go`
- Modify: `internal/tui/repo/repo.go` / `repo_test.go`
- Modify: `internal/tui/detail/detail.go` / `detail_test.go`
- Modify: `internal/i18n/locales/active.{en,ja}.yaml`（`footer.work` / `footer.list` / `footer.detail_*` に `d:diff`）

**Interfaces:**
- Consumes: Task 5 の `diff.Model` / `diff.ClosedMsg` / `diff.ErrorMsg`
- Produces:
  - `work.OpenDiffMsg{Ref gh.ItemRef}` / `repo.OpenDiffMsg{Ref gh.ItemRef}` / `detail.OpenDiffMsg{Ref gh.ItemRef}`
  - `app.Source` に `diff.Source` を足す

### `showingDetail bool` をやめる理由

`d` は詳細ビューからも押せる。押せば diff が詳細の上に乗り、`esc` で詳細に戻る。
真偽値 1 つでは「詳細の上に diff がある」と「diff だけがある」を区別できない。
**ビューのスタックにする。**

```go
// overlay is a view drawn over the tabs. They stack: d from the detail view
// puts the diff on top of it, and esc takes it back off.
type overlay int

const (
	overlayDetail overlay = iota
	overlayDiff
)
```

`Model` の `showingDetail bool` を `stack []overlay` に置き換える。空なら
タブが見えている。`m.stack[len(m.stack)-1]` が一番上。

- [ ] **Step 1: 失敗するテストを書く**

`internal/tui/app/app_test.go` に足す:

```go
func TestDOpensTheDiffOverTheDetailView(t *testing.T) {
	m := started(t) // 既存のヘルパー。無ければ既存テストの組み立てを写す
	m2, _ := m.Update(work.OpenDetailMsg{Ref: someRef})
	m3, _ := m2.(Model).Update(detail.OpenDiffMsg{Ref: someRef})
	got := m3.(Model)
	if len(got.stack) != 2 {
		t.Fatalf("stack = %v, want the detail view with the diff over it", got.stack)
	}
	if got.stack[1] != overlayDiff {
		t.Errorf("the top of the stack is %v, want the diff", got.stack[1])
	}
}

func TestEscTakesTheDiffOffAndLeavesTheDetailView(t *testing.T) {
	m := started(t)
	m2, _ := m.Update(work.OpenDetailMsg{Ref: someRef})
	m3, _ := m2.(Model).Update(detail.OpenDiffMsg{Ref: someRef})
	m4, _ := m3.(Model).Update(diff.ClosedMsg{})
	got := m4.(Model)
	if len(got.stack) != 1 || got.stack[0] != overlayDetail {
		t.Errorf("stack = %v, want just the detail view", got.stack)
	}
}

func TestDFromTheBoardOpensTheDiffOnItsOwn(t *testing.T) {
	m := started(t)
	m2, _ := m.Update(work.OpenDiffMsg{Ref: someRef})
	got := m2.(Model)
	if len(got.stack) != 1 || got.stack[0] != overlayDiff {
		t.Errorf("stack = %v, want just the diff", got.stack)
	}
}

// TestAClosedDiffsFailureIsNotShown mirrors the rule the detail view already
// follows: a request outlives the view that started it, and its failure must
// not drag a closed view's error onto the screen.
func TestAClosedDiffsFailureIsNotShown(t *testing.T) {
	m := started(t)
	m2, _ := m.Update(diff.ErrorMsg{Err: errors.New("boom")})
	if got := m2.(Model).errText; got != "" {
		t.Errorf("error screen shows %q for a diff that is not open", got)
	}
}
```

`internal/tui/work/work_test.go` に足す:

```go
func TestDAsksForTheDiff(t *testing.T) {
	m := loadedBoard(t) // 既存のヘルパーに合わせる
	_, cmd := m.Update(keyPress("d"))
	if cmd == nil {
		t.Fatal("d produced no command")
	}
	msg, ok := cmd().(OpenDiffMsg)
	if !ok {
		t.Fatalf("d produced %T, want OpenDiffMsg", cmd())
	}
	want, _ := m.SelectedRef()
	if msg.Ref != want {
		t.Errorf("d asked for %v, want the selected card %v", msg.Ref, want)
	}
}

// TestDDoesNothingOnAnIssue is what stops the diff view opening on something
// that has no diff.
func TestDDoesNothingOnAnIssue(t *testing.T) {
	m := boardWithOnlyAnIssue(t)
	_, cmd := m.Update(keyPress("d"))
	if cmd != nil {
		t.Errorf("d on an issue produced %T", cmd())
	}
}
```

`repo` と `detail` にも同じ 2 本を、それぞれの組み立てで書く。

- [ ] **Step 2: テストが落ちることを確かめる**

Run: `go test ./internal/tui/... -run 'TestD|TestEscTakes|TestAClosedDiff' -v`
Expected: FAIL

- [ ] **Step 3: 各ビューに `d` を足す**

3 つのビューとも、選択中が **PR のときだけ** `OpenDiffMsg` を返す。

```go
// OpenDiffMsg asks the parent to show the diff of the selected pull request.
type OpenDiffMsg struct{ Ref gh.ItemRef }
```

```go
	case "d":
		ref, ok := m.SelectedRef()
		// An issue has no diff. Opening an empty diff view would be a worse
		// answer than doing nothing.
		if !ok || ref.Kind != gh.ItemPR {
			return m, nil
		}
		return m, func() tea.Msg { return OpenDiffMsg{Ref: ref} }
```

`detail` では `m.ref` を使う（`detail` は 1 件しか持たない）。

- [ ] **Step 4: ルートモデルをスタックにする**

- `showingDetail bool` → `stack []overlay`
- `broadcast` は「スタックに載っているものすべて」に配る（詳細ビューは diff の下でも
  自分の取得を続けているため）
- `handleKey` は**スタックの一番上にだけ**キーを配る
- `resize` はスタック上のビューに端末の全高を渡す（従来の詳細ビューと同じ）
- `diff.ClosedMsg` でスタックを 1 つ pop、`detail.ClosedMsg` で詳細を pop
- `detail.ErrorMsg` / `diff.ErrorMsg` は、**そのビューがスタックに載っているときだけ**
  エラー画面に出す
- `render.go` はスタックの一番上を描く

`app.Source` に `diff.Source` を足す:

```go
type Source interface {
	work.Source
	repo.Source
	detail.Source
	diff.Source
}
```

- [ ] **Step 5: キーバーに `d` を足す**

`footer.work` / `footer.list` / `footer.detail_suffix` の英日両方に `d:diff` /
`d:diff`（日本語も `d:diff` でよい。`diff` は訳さない語である）を足す。
**`footer.diff` に `c` / `v` はまだ足さない。**

- [ ] **Step 6: テストが通ることを確かめる**

Run: `go test ./internal/tui/... -v`
Expected: PASS

- [ ] **Step 7: golden を録り直す**

キーバーが変わったので、`work` / `repo` / `detail` の golden がすべて動く。

```
make golden
git diff --stat internal/tui
```

**diff を目で見る。** 変わったのがキーバーの行だけであることを確かめる。
それ以外が変わっていたら、スタック化がレイアウトを壊している。

- [ ] **Step 8: `make check` を通してコミット**

```
make check
```

コミットメッセージ:

```
feat: open the diff with d, from wherever the pull request is

The root model's showingDetail flag became a stack. d works from the detail
view too, and a boolean cannot tell "the diff over the detail view" from
"the diff on its own" -- esc has to know which one to go back to.

d does nothing on an issue. An empty diff view is a worse answer than none.
```

---
## Task 7: 既存のレビュースレッドを行の下に出す

**Files:**
- Create: `internal/tui/diff/thread.go`
- Create: `internal/tui/diff/thread_test.go`
- Modify: `internal/tui/diff/diff.go`（`Source` に `PRReviewContext`、`buildRows` にスレッド）
- Modify: `internal/tui/diff/render.go`
- Modify: `internal/i18n/locales/active.{en,ja}.yaml`

**Interfaces:**
- Consumes: Task 2 の `gh.ReviewThread` / `gh.ReviewContext`、Task 5 の `row`
- Produces:
  - `diff.Source` に `PRReviewContext(ctx context.Context, repo string, number int) (gh.ReviewContext, error)` を足す
  - `row` に `rowThread` と `rowCollapsed` を足す
  - `Model.threadsFor(path string, line int, side gh.DiffSide) []gh.ReviewThread`

### スレッドを行に結びつける鍵

スレッドは `(path, line, side)` を持ち、diff の行も `Line()` で同じ 3 つを返す
（Task 1）。**この 3 つが一致した行の下に差し込む。** 一致しないスレッド
（ファイルが diff に含まれない、行が消えた）は**捨てない**: そのファイルの
最後のハンクの後ろにまとめて置く。捨てると、指摘があること自体が見えなくなる。

### 畳む条件

`gh.ReviewThread.Collapsed()`（resolved または outdated）が真なら、
`▸ N resolved comments` の 1 行だけを出す。`enter` でその行を開き、もう一度で閉じる。
**開いた状態はモデルが持つ**（`expanded map[string]bool`、キーはスレッドの
`path:line:side`）。

- [ ] **Step 1: カタログに足す**

`active.en.yaml` の `diff:` に:

```yaml
  collapsed:
    one: "{{.Count}} settled comment - enter to open"
    other: "{{.Count}} settled comments - enter to open"
  orphaned:
    other: "comments on lines this diff does not show"
  resolved:
    other: "resolved"
  outdated:
    other: "outdated"
  unsent:
    other: "not sent yet"
```

`active.ja.yaml` の `diff:` に:

```yaml
  collapsed:
    other: "片付いたコメント {{.Count}} 件 — enter で開く"
  orphaned:
    other: "この diff に無い行へのコメント"
  resolved:
    other: "解決済み"
  outdated:
    other: "古い"
  unsent:
    other: "未送信"
```

`footer.diff` の両言語に `enter:open` / `enter:開く` を足す。

- [ ] **Step 2: 失敗するテストを書く**

`internal/tui/diff/thread_test.go`:

```go
package diff

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/gh"
)

func threadFixture() gh.ReviewContext {
	at := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	return gh.ReviewContext{
		PullRequestID: "PR_1",
		Threads: []gh.ReviewThread{
			{
				Path: "graph/walk.go", Line: 13, Side: gh.SideRight,
				Comments: []gh.ThreadComment{
					{Author: gh.Author{Login: "kukv"}, Body: "is 2 not the default?", CreatedAt: at},
				},
			},
			{
				Path: "graph/walk.go", Line: 13, Side: gh.SideLeft, Resolved: true,
				Comments: []gh.ThreadComment{
					{Author: gh.Author{Login: "someone"}, Body: "settled long ago", CreatedAt: at},
				},
			},
			{
				Path: "graph/walk.go", Line: 900, Side: gh.SideRight,
				Comments: []gh.ThreadComment{
					{Author: gh.Author{Login: "someone"}, Body: "on a line not in this diff", CreatedAt: at},
				},
			},
		},
	}
}

func withThreads(t *testing.T, width, height int) Model {
	t.Helper()
	m := loaded(t, width, height)
	m, _ = m.Update(reviewMsg{ref: m.ref, ctx: threadFixture()})
	return m
}

func TestAnOpenThreadIsShownUnderItsLine(t *testing.T) {
	out := ansi.Strip(withThreads(t, 120, 40).View())
	if !strings.Contains(out, "is 2 not the default?") {
		t.Errorf("the open thread is not on screen:\n%s", out)
	}
	if !strings.Contains(out, "kukv") {
		t.Errorf("the thread does not name its author:\n%s", out)
	}
}

func TestASettledThreadIsACountUntilItIsOpened(t *testing.T) {
	m := withThreads(t, 120, 40)
	out := ansi.Strip(m.View())
	if strings.Contains(out, "settled long ago") {
		t.Errorf("a resolved thread is shown in full before it is opened:\n%s", out)
	}
	if !strings.Contains(out, "settled comment") {
		t.Errorf("the resolved thread is not counted:\n%s", out)
	}

	// Walk to the collapsed row and open it.
	for i, r := range m.rows {
		if r.kind == rowCollapsed {
			m.row = i
			break
		}
	}
	m = press(m, "enter")
	if !strings.Contains(ansi.Strip(m.View()), "settled long ago") {
		t.Errorf("enter did not open the settled thread:\n%s", ansi.Strip(m.View()))
	}
	m = press(m, "enter")
	if strings.Contains(ansi.Strip(m.View()), "settled long ago") {
		t.Errorf("enter did not close the settled thread again")
	}
}

// TestACommentOnALineThisDiffDoesNotShowIsStillVisible: dropping it would
// hide the fact that someone objected at all.
func TestACommentOnALineThisDiffDoesNotShowIsStillVisible(t *testing.T) {
	out := ansi.Strip(withThreads(t, 120, 40).View())
	if !strings.Contains(out, "on a line not in this diff") {
		t.Errorf("an unplaceable comment was dropped:\n%s", out)
	}
}

// TestSidesAreNotMixedUp is the test that stops a comment on the old version
// of a line being drawn under the new one.
func TestSidesAreNotMixedUp(t *testing.T) {
	m := withThreads(t, 120, 40)
	left := m.threadsFor("graph/walk.go", 13, gh.SideLeft)
	right := m.threadsFor("graph/walk.go", 13, gh.SideRight)
	if len(left) != 1 || !left[0].Resolved {
		t.Errorf("the left side of line 13 has %+v, want the resolved thread", left)
	}
	if len(right) != 1 || right[0].Resolved {
		t.Errorf("the right side of line 13 has %+v, want the open thread", right)
	}
}
```

- [ ] **Step 3: テストが落ちることを確かめる**

Run: `go test ./internal/tui/diff/ -run 'Thread|Settled|Sides|Comment' -v`
Expected: FAIL

- [ ] **Step 4: 実装する**

`internal/tui/diff/diff.go`:

- `Source` に `PRReviewContext` を足す
- `reviewMsg struct { ref gh.ItemRef; ctx gh.ReviewContext }` を足し、`fetch()` を
  `tea.Batch` で **diff とレビューを並行に取る** 2 本にする。片方が遅れても
  もう片方は描ける
- `Model` に `review gh.ReviewContext` と `expanded map[string]bool` を足す
- `rowKind` に `rowThread`（1 件のコメント）と `rowCollapsed`（畳まれた件数）を足す
- `row` に `thread gh.ReviewThread` と `key string` を足す

`internal/tui/diff/thread.go`:

```go
package diff

import (
	"strconv"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/i18n"
)

// threadKey names one thread by where it sits. It is the map key for which
// settled threads the user has opened, and it has to survive a refetch, so it
// is built from the position rather than from an id.
func threadKey(path string, line int, side gh.DiffSide) string {
	return path + ":" + strconv.Itoa(line) + ":" + strconv.Itoa(int(side))
}

// threadsFor returns the threads that belong under one line of the diff.
// Path, line and side must all match: a comment on the old version of a line
// is not a comment on the new one.
func (m Model) threadsFor(path string, line int, side gh.DiffSide) []gh.ReviewThread {
	var out []gh.ReviewThread
	for _, t := range m.review.Threads {
		if t.Path == path && t.Line == line && t.Side == side {
			out = append(out, t)
		}
	}
	return out
}

// threadRows turns the threads under one line into drawable rows. Settled
// ones collapse into a single count until the user opens them, so that
// finished arguments do not push the code they were about off the screen.
func (m Model) threadRows(hunk int, path string, line int, side gh.DiffSide) []row {
	threads := m.threadsFor(path, line, side)
	if len(threads) == 0 {
		return nil
	}
	key := threadKey(path, line, side)
	var rows []row
	var settled int
	for _, t := range threads {
		if t.Collapsed() && !m.expanded[key] {
			settled++
			continue
		}
		for _, c := range t.Comments {
			rows = append(rows, row{kind: rowThread, hunk: hunk, key: key, thread: t, comment: c})
		}
	}
	if settled > 0 {
		rows = append(rows, row{
			kind: rowCollapsed, hunk: hunk, key: key,
			text: i18n.Tn("diff.collapsed", settled),
		})
	}
	return rows
}

// orphanRows are the comments whose line this diff does not show: the file is
// not in the diff, or the line has gone. They go at the end of the file they
// name rather than being dropped -- a comment nobody can see is a comment
// nobody answers.
func (m Model) orphanRows(placed map[string]bool) []row {
	var rows []row
	for _, t := range m.review.Threads {
		if t.Path != m.files[m.file].Path {
			continue
		}
		if placed[threadKey(t.Path, t.Line, t.Side)] {
			continue
		}
		rows = append(rows, row{kind: rowNote, hunk: -1, text: i18n.T("diff.orphaned")})
		break
	}
	if len(rows) == 0 {
		return nil
	}
	for _, t := range m.review.Threads {
		if t.Path != m.files[m.file].Path || placed[threadKey(t.Path, t.Line, t.Side)] {
			continue
		}
		for _, c := range t.Comments {
			rows = append(rows, row{kind: rowThread, hunk: -1, thread: t, comment: c})
		}
	}
	return rows
}
```

`buildRows` を、各行の後ろに `threadRows` を挟み、最後に `orphanRows` を足す形へ。
挟んだスレッドの `key` を `placed` に記録して `orphanRows` に渡す。

`enter` の処理:

```go
	case "enter":
		if r := m.currentRow(); r.kind == rowCollapsed {
			if m.expanded == nil {
				m.expanded = map[string]bool{}
			}
			m.expanded[r.key] = !m.expanded[r.key]
			m.rows = m.buildRows()
			m.row = clamp(m.row, 0, len(m.rows)-1)
			return m.follow(), nil
		}
		return m, nil
```

描画は `theme.Thread(t.Pending())` で色を分け、行頭に `▌` を置き、
`著者 · 本文` の形で 1 行に収める（`ansi.Truncate` で切る）。
**未送信のスレッドには `i18n.T("diff.unsent")` を添える。**

- [ ] **Step 5: テストが通ることを確かめる**

Run: `go test ./internal/tui/diff/ -v`
Expected: PASS

- [ ] **Step 6: アサーションが空振りしていないことを確かめる**

`threadsFor` の `t.Side == side` を消して `TestSidesAreNotMixedUp` が落ちることを目で見る。

- [ ] **Step 7: golden を録り直してコミット**

```
make golden
cat -v internal/tui/diff/testdata/diff_ja_80.golden
make check
```

コミットメッセージ:

```
feat: show the review already on the diff

A thread hangs under the line it names, matched on path, line and side --
a comment on the old version of a line is not a comment on the new one.

Settled threads collapse to a count until enter opens them: an argument
that finished must not push the code it was about off the screen. A comment
whose line this diff does not show goes at the end of the file rather than
being dropped, because a comment nobody can see is a comment nobody answers.
```

---

## Task 8: 行コメント（`c`）

**Files:**
- Create: `internal/tui/diff/comment.go`
- Create: `internal/tui/diff/comment_test.go`
- Modify: `internal/tui/diff/diff.go` / `render.go`
- Modify: `internal/i18n/locales/active.{en,ja}.yaml`

**Interfaces:**
- Consumes: Task 3 の `StartReview` / `AddReviewThread`、Task 2 の `gh.PendingComment`
- Produces: `diff.Source` に `StartReview(pullRequestID string) (string, error)` と `AddReviewThread(reviewID string, c gh.PendingComment) error` を足す

### `c` を押してから GitHub に届くまで

1. カーソルが `rowLine` の上にあるときだけ `c` が効く。ハンク見出しやスレッドの
   行にはコメントできない
2. `textarea` が開く。`ctrl+s` で送る、`esc` で捨てる
3. 送るとき、**pending review がまだ無ければ `StartReview` で作り**、返ってきた id に
   `AddReviewThread` する。**2 回のネットワーク往復になるのは最初の 1 件だけ**
4. 送り終わったら `PRReviewContext` を取り直す。自分のコメントが
   `Pending` のスレッドとして返ってきて、そのまま行の下に出る

- [ ] **Step 1: カタログに足す**

`active.en.yaml` の `diff:` に:

```yaml
  comment_placeholder:
    other: "Comment on this line"
  posting:
    other: "sending the comment"
```

`footer:` に:

```yaml
  diff_comment:
    other: "ctrl+s:send  esc:cancel"
```

`active.ja.yaml` の対応する場所に:

```yaml
  comment_placeholder:
    other: "この行へのコメント"
  posting:
    other: "コメントを送信中"
```

```yaml
  diff_comment:
    other: "ctrl+s:送信  esc:取消"
```

`footer.diff` の両言語に `c:comment` / `c:コメント` を足す。

- [ ] **Step 2: 失敗するテストを書く**

`internal/tui/diff/comment_test.go`:

```go
package diff

import (
	"strings"
	"sync"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/gh"
)

// recordingSource remembers what was sent, which is the only thing worth
// asserting: nothing here talks to GitHub.
type recordingSource struct {
	fakeSource
	mu       sync.Mutex
	started  int
	comments []gh.PendingComment
	reviewID string
}

func (s *recordingSource) StartReview(string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.started++
	s.reviewID = "PRR_new"
	return s.reviewID, nil
}

func (s *recordingSource) AddReviewThread(_ string, c gh.PendingComment) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.comments = append(s.comments, c)
	return nil
}

func TestCommentingOnAnAddedLineQuotesTheRightSide(t *testing.T) {
	src := &recordingSource{fakeSource: fakeSource{files: fixture()}}
	m := loadedWith(t, src, 120, 40)

	// Put the cursor on the added line "if depth <= 0 {".
	m = cursorOnLine(t, m, gh.LineAdded, 13)
	m = press(m, "c")
	if !m.composing {
		t.Fatal("c did not open the composer")
	}
	m = typeInto(m, "why not 2?")
	_, cmd := m.Update(keyPress("ctrl+s"))
	runCmd(t, cmd)

	if len(src.comments) != 1 {
		t.Fatalf("%d comments sent, want 1", len(src.comments))
	}
	got := src.comments[0]
	want := gh.PendingComment{Path: "graph/walk.go", Line: 13, Side: gh.SideRight, Body: "why not 2?"}
	if got != want {
		t.Errorf("sent %+v, want %+v", got, want)
	}
}

func TestCommentingOnARemovedLineQuotesTheLeftSide(t *testing.T) {
	src := &recordingSource{fakeSource: fakeSource{files: fixture()}}
	m := loadedWith(t, src, 120, 40)
	m = cursorOnLine(t, m, gh.LineRemoved, 13)
	m = press(m, "c")
	m = typeInto(m, "why?")
	_, cmd := m.Update(keyPress("ctrl+s"))
	runCmd(t, cmd)

	if len(src.comments) != 1 {
		t.Fatalf("%d comments sent, want 1", len(src.comments))
	}
	if got := src.comments[0].Side; got != gh.SideLeft {
		t.Errorf("side = %v, want the left: the line was removed", got)
	}
}

// TestTheSecondCommentReusesTheReview: starting a review per comment would
// leave a pile of separate reviews on the pull request.
func TestTheSecondCommentReusesTheReview(t *testing.T) {
	src := &recordingSource{fakeSource: fakeSource{files: fixture()}}
	m := loadedWith(t, src, 120, 40)
	for _, body := range []string{"first", "second"} {
		m = cursorOnLine(t, m, gh.LineAdded, 13)
		m = press(m, "c")
		m = typeInto(m, body)
		var cmd tea.Cmd
		m, cmd = m.Update(keyPress("ctrl+s"))
		m = runInto(t, m, cmd)
	}
	if src.started != 1 {
		t.Errorf("started %d reviews, want 1", src.started)
	}
	if len(src.comments) != 2 {
		t.Errorf("%d comments sent, want 2", len(src.comments))
	}
}

func TestCDoesNothingOnAHunkHeader(t *testing.T) {
	m := loaded(t, 120, 40)
	m.row = 0 // the first row of the fixture is a hunk header
	m = press(m, "c")
	if m.composing {
		t.Error("c opened the composer on a hunk header")
	}
}

func TestEscThrowsTheDraftAway(t *testing.T) {
	src := &recordingSource{fakeSource: fakeSource{files: fixture()}}
	m := loadedWith(t, src, 120, 40)
	m = cursorOnLine(t, m, gh.LineAdded, 13)
	m = press(m, "c")
	m = typeInto(m, "never mind")
	m, _ = m.Update(keyPress("esc"))
	if m.composing {
		t.Error("esc did not close the composer")
	}
	if len(src.comments) != 0 {
		t.Errorf("esc sent %d comments", len(src.comments))
	}
	if strings.Contains(ansi.Strip(m.View()), "never mind") {
		t.Error("the discarded draft is still on screen")
	}
}
```

ヘルパー（`loadedWith` / `cursorOnLine` / `typeInto` / `keyPress` / `runCmd` /
`runInto`）は同じファイルに書く。`cursorOnLine` は `m.rows` を走査して
「その種類とその行番号を持つ `rowLine`」を探し、見つからなければ `t.Fatal`。
**探して見つからないまま通るテストにしない。**

- [ ] **Step 3: テストが落ちることを確かめる**

Run: `go test ./internal/tui/diff/ -run 'Comment|Esc|TheSecond' -v`
Expected: FAIL

- [ ] **Step 4: 実装する**

`internal/tui/diff/comment.go`:

```go
package diff

import (
	tea "charm.land/bubbletea/v2"

	"github.com/kukv/octoscope/internal/gh"
)

type (
	commentPostedMsg struct{ ref gh.ItemRef }
	commentErrorMsg  struct{ err error }
)

// startComposing opens the composer on the line under the cursor. A hunk
// header, a thread and a note have no line to comment on, so c does nothing
// there rather than posting somewhere arbitrary.
func (m Model) startComposing() Model {
	r := m.currentRow()
	if r.kind != rowLine {
		return m
	}
	m.composing = true
	m.textarea.Reset()
	m.textarea.Focus()
	return m
}

// post sends the composed comment. The review is started here rather than
// when the view opens: a diff the user only reads must not leave an empty
// pending review behind on the pull request.
func (m Model) post() (Model, tea.Cmd) {
	body := m.textarea.Value()
	if body == "" {
		return m, nil
	}
	line, side := m.currentRow().line.Line()
	comment := gh.PendingComment{
		Path: m.files[m.file].Path,
		Line: line,
		Side: side,
		Body: body,
	}

	src, ref, reviewID := m.src, m.ref, m.review.PendingID
	m.composing = false
	m.posting = true
	return m, func() tea.Msg {
		if reviewID == "" {
			id, err := src.StartReview(m.review.PullRequestID)
			if err != nil {
				return commentErrorMsg{err}
			}
			reviewID = id
		}
		if err := src.AddReviewThread(reviewID, comment); err != nil {
			return commentErrorMsg{err}
		}
		return commentPostedMsg{ref: ref}
	}
}
```

**`m.review.PullRequestID` をクロージャの外で取り出すこと。** モデルは値なので
クロージャがモデルごと捕まえても動くが、`.claude/rules/tui.md` の
「サブモデルは自分の状態だけを持つ」に沿って、必要な値だけを渡す。

`commentPostedMsg` を受けたら `m.posting = false` にし、`fetchReview()` を返して
スレッドを取り直す。**取り直した結果の `PendingID` が、次のコメントで再利用される。**

`handleKey` の先頭に、composing 中の分岐を置く（`detail` の composer と同じ形）:

```go
	if m.composing {
		switch msg.String() {
		case "esc":
			m.composing = false
			m.textarea.Reset()
			return m, nil
		case "ctrl+s":
			return m.post()
		}
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		return m, cmd
	}
```

`c` の分岐:

```go
	case "c":
		return m.startComposing(), nil
```

描画: composing 中は diff ペインの下に textarea を 3 行で出し、キーバーを
`footer.diff_comment` に差し替える。**盤面の高さ予算からその分を引く。**

- [ ] **Step 5: テストが通ることを確かめる**

Run: `go test ./internal/tui/diff/ -v`
Expected: PASS

- [ ] **Step 6: アサーションが空振りしていないことを確かめる**

`post()` の `line, side := m.currentRow().line.Line()` を
`line, side := m.currentRow().line.NewLine, gh.SideRight` に一時的に変え、
`TestCommentingOnARemovedLineQuotesTheLeftSide` が落ちることを目で見る。
**落ちないなら、そのテストは行コメントが正しい行に付くことを守っていない。**

- [ ] **Step 7: golden を録り直してコミット**

```
make golden
make check
```

コミットメッセージ:

```
feat: comment on a line of the diff

The review is opened on the first comment, not when the view opens: a diff
somebody only read must not leave an empty pending review on the pull
request. Every comment after that reuses it.

Which side a comment lands on comes from the line itself -- a removed line
is quoted on the left, everything else on the right. Getting this wrong
puts the comment on a different line of a real pull request, so the test
that pins it is the one to keep.
```

---

## Task 9: レビューの提出（`v`）と破棄（`X`）

**Files:**
- Create: `internal/tui/review/review.go`
- Create: `internal/tui/review/review_test.go`
- Create: `internal/tui/review/golden_test.go`
- Create: `internal/tui/review/testdata/*.golden`
- Modify: `internal/tui/diff/diff.go`（`v` と `X`、ポップアップの取り込み）
- Modify: `internal/tui/detail/detail.go`（`v`）
- Modify: `internal/tui/app/app.go`（`overlayReview` は作らない。理由は下）
- Modify: `.golangci.yml`（`depguard` に `tui-review`）
- Modify: `internal/i18n/locales/active.{en,ja}.yaml`

**Interfaces:**
- Consumes: Task 3 の `SubmitReview` / `DiscardReview`
- Produces:
  - `review.Source` interface（`SubmitReview(reviewID string, event gh.ReviewEvent, body string) error`）
  - `diff.Source` に `DiscardReview(reviewID string) error` を足す（`X` を押すのは diff ビューであり、ポップアップではない）
  - `review.New(src Source, reviewID string, pendingComments int) Model`
  - `review.Model` の `Update` / `View() string` / `Active() bool`
  - `review.SubmittedMsg{}` / `review.CancelledMsg{}` / `review.ErrorMsg{Err error}`

### なぜルートモデルのスタックに載せないのか

提出のポップアップは**その場に重ねる小さな窓**であり、タブや詳細のような 1 画面ではない。
Repos の追加ダイアログ（Phase 4）も同じ扱いになる。`diff` と `detail` が
それぞれ `review.Model` を 1 つ持ち、`Active()` が真のときだけ自分の `View` に
重ねて描き、キーを先に渡す。**ルートモデルはポップアップの存在を知らない。**

### 破棄（`X`）

大文字の `X` にする。`x` は `repo` タブで「一覧から削除」に使う予定（Phase 4）であり、
**書きかけのレビューを 1 打鍵で消すのは危ない。** 押すと確認を出し、`y` で消す。

- [ ] **Step 1: カタログに足す**

`active.en.yaml`:

```yaml
submit:
  title:
    other: "Submit review"
  approve:
    other: "Approve"
  request_changes:
    other: "Request changes"
  comment:
    other: "Comment"
  placeholder:
    other: "Leave a note (optional)"
  pending_count:
    one: "{{.Count}} line comment will be sent"
    other: "{{.Count}} line comments will be sent"
  nothing_to_submit:
    other: "there is no unsubmitted review"
  sending:
    other: "submitting the review"
  discard_confirm:
    other: "Discard the unsubmitted review and every comment on it?"
```

`footer:`:

```yaml
  submit:
    other: "tab:event  ctrl+s:submit  esc:cancel"
  discard:
    other: "y:discard  n:keep"
```

`active.ja.yaml`:

```yaml
submit:
  title:
    other: "レビューを提出"
  approve:
    other: "承認"
  request_changes:
    other: "変更を要求"
  comment:
    other: "コメント"
  placeholder:
    other: "ひとこと（任意）"
  pending_count:
    other: "行コメント {{.Count}} 件が一緒に送られます"
  nothing_to_submit:
    other: "未提出のレビューはありません"
  sending:
    other: "レビューを提出中"
  discard_confirm:
    other: "未提出のレビューとそのコメントをすべて破棄しますか?"
```

```yaml
  submit:
    other: "tab:種別  ctrl+s:提出  esc:取消"
  discard:
    other: "y:破棄  n:やめる"
```

`footer.diff` の両言語に `v:review` / `v:レビュー` と `X:discard` / `X:破棄` を足す。
`footer.detail_suffix` にも `v:review` / `v:レビュー` を足す。

- [ ] **Step 2: 失敗するテストを書く**

`internal/tui/review/review_test.go`:

```go
package review

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/gh"
)

type fakeSource struct {
	event gh.ReviewEvent
	body  string
	calls int
	err   error
}

func (f *fakeSource) SubmitReview(_ string, event gh.ReviewEvent, body string) error {
	f.calls++
	f.event, f.body = event, body
	return f.err
}

func open(src Source) Model {
	m := New(src, "PRR_9", 2)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return m
}

func TestTabWalksTheThreeEvents(t *testing.T) {
	m := open(&fakeSource{})
	want := []gh.ReviewEvent{gh.EventComment, gh.EventApprove, gh.EventRequestChanges, gh.EventComment}
	if m.event != want[0] {
		t.Fatalf("the popup opens on %v, want comment", m.event)
	}
	for _, w := range want[1:] {
		m, _ = m.Update(keyPress("tab"))
		if m.event != w {
			t.Errorf("tab reached %v, want %v", m.event, w)
		}
	}
}

func TestSubmitSendsTheChosenEventAndBody(t *testing.T) {
	src := &fakeSource{}
	m := open(src)
	m, _ = m.Update(keyPress("tab")) // approve
	m = typeInto(m, "looks good")
	_, cmd := m.Update(keyPress("ctrl+s"))
	runCmd(t, cmd)

	if src.calls != 1 {
		t.Fatalf("%d submissions, want 1", src.calls)
	}
	if src.event != gh.EventApprove {
		t.Errorf("event = %v, want approve", src.event)
	}
	if src.body != "looks good" {
		t.Errorf("body = %q", src.body)
	}
}

// TestApprovingWithNoNoteIsAllowed: an approval usually has nothing to say,
// and refusing to send one without a note would make the common case the
// awkward one.
func TestApprovingWithNoNoteIsAllowed(t *testing.T) {
	src := &fakeSource{}
	m := open(src)
	m, _ = m.Update(keyPress("tab"))
	_, cmd := m.Update(keyPress("ctrl+s"))
	runCmd(t, cmd)
	if src.calls != 1 {
		t.Errorf("%d submissions, want an approval with no note to go through", src.calls)
	}
}

func TestTheLineCommentCountIsShown(t *testing.T) {
	out := ansi.Strip(open(&fakeSource{}).View())
	if !strings.Contains(out, "2 line comments") {
		t.Errorf("the popup does not say how many comments go with it:\n%s", out)
	}
}

func TestEscCancelsWithoutSending(t *testing.T) {
	src := &fakeSource{}
	m := open(src)
	_, cmd := m.Update(keyPress("esc"))
	if src.calls != 0 {
		t.Errorf("esc submitted the review")
	}
	if _, ok := cmd().(CancelledMsg); !ok {
		t.Errorf("esc produced %T, want CancelledMsg", cmd())
	}
}
```

`internal/tui/diff/comment_test.go` に足す:

```go
func TestVOpensTheSubmitPopupOnlyWhenThereIsSomethingToSubmit(t *testing.T) {
	m := loaded(t, 120, 40) // no review context yet: no pending review
	m = press(m, "v")
	if m.review.PendingID == "" && m.submitting {
		t.Error("the popup opened with nothing to submit")
	}

	m = withThreads(t, 120, 40)
	m.review.PendingID = "PRR_9"
	m = press(m, "v")
	if !m.submitting {
		t.Error("v did not open the popup when a review was waiting")
	}
}

func TestCapitalXAsksBeforeDiscarding(t *testing.T) {
	m := withThreads(t, 120, 40)
	m.review.PendingID = "PRR_9"
	m = press(m, "X")
	if !m.discarding {
		t.Fatal("X did not ask")
	}
	if !strings.Contains(ansi.Strip(m.View()), "Discard") {
		t.Errorf("the question is not on screen:\n%s", ansi.Strip(m.View()))
	}
	m = press(m, "n")
	if m.discarding {
		t.Error("n did not take the question away")
	}
}
```

- [ ] **Step 3: テストが落ちることを確かめる**

Run: `go test ./internal/tui/review/ ./internal/tui/diff/ -v`
Expected: FAIL

- [ ] **Step 4: `review` パッケージを書く**

```go
// Package review is the popup that submits a pull request review: which of
// the three things it says, and the note that goes with it.
//
// It is a popup rather than a view of its own, so it has no place in the root
// model's stack. The diff view and the detail view each hold one and draw it
// over themselves.
package review

// Source is what submitting needs from the GitHub layer.
type Source interface {
	SubmitReview(reviewID string, event gh.ReviewEvent, body string) error
}

// SubmittedMsg tells the holder the review went out; it should refetch.
type SubmittedMsg struct{}

// CancelledMsg tells the holder to take the popup away.
type CancelledMsg struct{}

// ErrorMsg carries a failure the holder shows.
type ErrorMsg struct{ Err error }
```

- `Model` は `src` / `reviewID` / `pending int` / `event gh.ReviewEvent` /
  `textarea` / `sending bool` / `width, height`
- `tab` は `EventComment → EventApprove → EventRequestChanges → EventComment` を回す
- `ctrl+s` は `SubmitReview` を `tea.Cmd` の中で呼び、`SubmittedMsg` か `ErrorMsg` を返す
- `View()` は枠つきの箱を返す。**幅は `min(50, width-4)`。80 桁で溢れないこと**
- 描画は `theme` から引く。3 つの選択肢は選択中だけ `theme.Selected()`

- [ ] **Step 5: `diff` と `detail` に組み込む**

- `Model` に `submit review.Model` / `submitting bool` / `discarding bool` を足す
- `v`: `m.review.PendingID == ""` なら**何もしない**（`detail` では
  `PRReviewContext` を取ってから開く）。あれば `submitting = true`
- `submitting` なら、キーは先に `m.submit.Update` へ。`CancelledMsg` で
  `submitting = false`、`SubmittedMsg` で `fetchReview()` を返す
- `X`: `PendingID` があれば `discarding = true`。`y` で `DiscardReview` を呼び、
  `fetchReview()`。`n` / `esc` で取り消し
- `View()`: `submitting` / `discarding` のときはポップアップを**画面の上に重ねる**。
  `lipgloss.Place` ではなく、既存の `detail` の確認ダイアログと同じ描き方に揃えること

`detail` 側は `v` で `PRReviewContext` を取ってから同じポップアップを開く。
**`detail` は diff を持たないので、行コメントは無い。ポップアップだけ。**

- [ ] **Step 6: depguard に `tui-review` を足す**

`tui-diff` と同じ形で、兄弟（`work` / `repo` / `detail` / `diff`）と親（`app`）を deny。
既存の各ルールの deny にも `internal/tui/review` を足す。

- [ ] **Step 7: テストが通ることを確かめる**

Run: `go test ./internal/tui/... -v`
Expected: PASS

- [ ] **Step 8: golden を録ってコミット**

```
make golden
cat -v internal/tui/review/testdata/review_ja_80.golden
make check
```

**80 桁で箱が溢れていないことを目で見る。**

コミットメッセージ:

```
feat: submit and discard a review

The popup is held by the diff view and the detail view rather than by the
root model: it is a small window drawn over what is already there, not a
screen of its own, and the same will be true of the Repos dialog.

Discarding is on capital X and asks first. Lowercase x is spoken for, and
one keystroke is too little between a written review and no review.
```

---
## Task 10: 幅の劣化 — 100 桁未満でサイドバーを畳む

**Files:**
- Modify: `internal/tui/diff/render.go`
- Modify: `internal/tui/diff/diff_test.go`
- Modify: `internal/tui/diff/testdata/*.golden`

**Interfaces:**
- Produces: `Model.showSidebar() bool`（`render.go` と Task 11 の当たり判定が共有する）

### なぜ 100 桁なのか

サイドバーが 22 桁、区切りが 1 桁、行番号と記号のガターが 11 桁。80 桁だと
本文に 46 桁しか残らない。日本語のコメントなら 23 文字である。Work タブの
カードが 100 桁未満で枠を外すのと同じ理由で、**同じしきい値に揃える。**
畳んだ分はヘッダに現在のファイル名を出して補う。

- [ ] **Step 1: 失敗するテストを書く**

```go
func TestTheSidebarFoldsAtNarrowWidths(t *testing.T) {
	tests := []struct {
		name    string
		width   int
		sidebar bool
	}{
		{"wide enough for both", 120, true},
		{"exactly at the threshold", 100, true},
		{"one column short", 99, false},
		{"the narrowest terminal", 80, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := loaded(t, tt.width, 30)
			if got := m.showSidebar(); got != tt.sidebar {
				t.Errorf("showSidebar() = %v at %d columns, want %v", got, tt.width, tt.sidebar)
			}
			out := ansi.Strip(m.View())
			// The file being read is named either way: in the list when
			// there is one, in the header when there is not.
			if !strings.Contains(out, "graph/walk.go") {
				t.Errorf("the file being read is not named at %d columns:\n%s", tt.width, out)
			}
			if !tt.sidebar && strings.Contains(out, "logo.png") {
				t.Errorf("the folded sidebar still lists other files at %d columns:\n%s", tt.width, out)
			}
		})
	}
}

// TestNoLineIsWiderThanTheTerminal is the one that catches a Japanese comment
// pushing the pane a column over. Every recording is checked, not just this
// fixture, because the widths are where this breaks.
func TestNoLineIsWiderThanTheTerminal(t *testing.T) {
	for _, width := range []int{80, 100, 120, 160} {
		t.Run(fmt.Sprintf("w%d", width), func(t *testing.T) {
			i18n.SetLanguage(language.Japanese)
			t.Cleanup(func() { i18n.SetLanguage(language.English) })
			for i, line := range strings.Split(loaded(t, width, 30).View(), "\n") {
				if got := ansi.StringWidth(line); got > width {
					t.Errorf("line %d is %d columns wide in a terminal %d wide: %q",
						i, got, width, ansi.Strip(line))
				}
			}
		})
	}
}
```

`fixture()` に**日本語のコメントを含む行**を足すこと。足さないとこのテストは
桁ずれを一度も見ない。

```go
{Kind: gh.LineAdded, NewLine: 15, Text: "\t// 深さは既定で 2 とする（--depth で変えられる）"},
```

- [ ] **Step 2: テストが落ちることを確かめる**

Run: `go test ./internal/tui/diff/ -run 'Sidebar|NoLineIsWider' -v`
Expected: FAIL

- [ ] **Step 3: 実装する**

```go
// sidebarWidth is the file list's width, and gutterWidth what the two line
// numbers and the marker take. Both are read by the hit testing as well as by
// the drawing: a hit test that computes the layout a second time drifts.
const (
	sidebarWidth = 22
	gutterWidth  = 11
	// minWidthForSidebar is where the file list stops earning its columns.
	// Below it the body would be 46 columns, which is 23 Japanese characters.
	// It matches the width at which the Work board drops its card borders
	// (spec 4.6).
	minWidthForSidebar = 100
)

func (m Model) showSidebar() bool { return m.width >= minWidthForSidebar }
```

サイドバーを畳むときは、ヘッダ 2 行目の末尾に現在のファイル名を出す。
`h` / `l` は畳まれている間 `m.sidebar` を立てない（動く先が無いため）。

- [ ] **Step 4: テストが通ることを確かめ、golden を録り直す**

```
go test ./internal/tui/diff/ -v
make golden
cat -v internal/tui/diff/testdata/diff_ja_80.golden
```

**80 桁の録画でサイドバーが消え、120 桁で出ていることを目で見る。**

- [ ] **Step 5: `make check` を通してコミット**

```
make check
```

コミットメッセージ:

```
fix: fold the file list where it no longer earns its columns

Below 100 the sidebar, the divider and the line-number gutter leave 46
columns for code, which is 23 Japanese characters. The same threshold the
Work board drops its card borders at. The file being read moves into the
header, so nothing about where you are is lost.
```

---

## Task 11: マウス

**Files:**
- Create: `internal/tui/diff/mouse.go`
- Create: `internal/tui/diff/mouse_test.go`
- Modify: `internal/tui/diff/diff.go`

**Interfaces:**
- Consumes: Task 10 の `sidebarWidth` / `gutterWidth` / `showSidebar`
- Produces: `Model.Update` が `tea.MouseClickMsg` と `tea.MouseWheelMsg` を扱う

### spec §4.0 が課すこと

- **当たり判定は描画と同じ関数を読む。** `sidebarWidth` / `showSidebar()` /
  `paneHeight()` / `m.top` を共有する。座標をもう一度計算しない
- **1 回目のクリックで選択、選択中をもう 1 回で開く。** diff ペインで「開く」に
  当たるのは、畳まれたスレッドを開くこと。行そのものには開く先が無いので、
  行のクリックは選択だけ
- **ホイールはポインタの下のものを動かす。** サイドバーの上ならファイル、
  diff ペインの上なら行
- ルートモデルが既にタブ行の高さを引いている。**diff ビューはタブ行の上に
  重なるので、`app` は diff にはオフセットを引かずに渡す**（詳細ビューと同じ扱い）

- [ ] **Step 1: 失敗するテストを書く**

```go
package diff

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/gh"
)

// rowY finds the screen row a given diff row was actually drawn on, so the
// test clicks where the view put it rather than where the test guessed.
func rowY(t *testing.T, m Model, needle string) int {
	t.Helper()
	for y, line := range strings.Split(m.View(), "\n") {
		if strings.Contains(ansi.Strip(line), needle) {
			return y
		}
	}
	t.Fatalf("%q was never drawn:\n%s", needle, ansi.Strip(m.View()))
	return 0
}

func click(m Model, x, y int) Model {
	m, _ = m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	return m
}

func TestClickingTheDiffSelectsThatLine(t *testing.T) {
	m := loaded(t, 120, 30)
	y := rowY(t, m, "if depth <= 0 {")
	m = click(m, sidebarWidth+gutterWidth+5, y)
	if got := m.currentRow(); got.kind != rowLine || got.line.Text != "\tif depth <= 0 {" {
		t.Errorf("the click selected %+v, want the line it landed on", got)
	}
}

func TestClickingTheFileListSelectsThatFile(t *testing.T) {
	m := loaded(t, 120, 30)
	y := rowY(t, m, "logo.png")
	m = click(m, 2, y)
	if m.file != 1 {
		t.Errorf("file = %d, want the one that was clicked", m.file)
	}
}

// TestClickingASettledThreadTwiceOpensIt: the first click selects, the second
// on an already-selected row opens. No double click -- Bubble Tea does not
// report one, and measuring the gap ourselves would put a clock in Update
// (spec 4.0).
func TestClickingASettledThreadTwiceOpensIt(t *testing.T) {
	m := withThreads(t, 120, 40)
	y := rowY(t, m, "settled comment")
	m = click(m, sidebarWidth+5, y)
	if strings.Contains(ansi.Strip(m.View()), "settled long ago") {
		t.Error("one click opened the thread")
	}
	m = click(m, sidebarWidth+5, y)
	if !strings.Contains(ansi.Strip(m.View()), "settled long ago") {
		t.Errorf("the second click did not open it:\n%s", ansi.Strip(m.View()))
	}
}

func TestTheWheelMovesWhateverIsUnderIt(t *testing.T) {
	m := loaded(t, 120, 30)
	before := m.row
	m, _ = m.Update(tea.MouseWheelMsg{X: sidebarWidth + 5, Y: 6, Button: tea.MouseWheelDown})
	if m.row == before {
		t.Error("the wheel over the diff moved nothing")
	}

	m2 := loaded(t, 120, 30)
	m2, _ = m2.Update(tea.MouseWheelMsg{X: 2, Y: 6, Button: tea.MouseWheelDown})
	if m2.file != 1 {
		t.Errorf("the wheel over the file list moved to file %d, want 1", m2.file)
	}
}

// TestClicksOutsideThePanesDoNothing: the header and the key bar are not
// clickable, and a click on them must not select whatever row the arithmetic
// happens to land on.
func TestClicksOutsideThePanesDoNothing(t *testing.T) {
	m := loaded(t, 120, 30)
	before := m.row
	m = click(m, 5, 0)  // the header
	m = click(m, 5, 29) // the key bar
	if m.row != before {
		t.Errorf("row moved to %d on a click outside the panes", m.row)
	}
}
```

**`tea.MouseClickMsg` / `tea.MouseWheelMsg` の組み立ては、既存の
`internal/tui/work/mouse_test.go` に合わせること。** 上のものはそのまま動かない
可能性がある。既存のヘルパーを写す。

- [ ] **Step 2: テストが落ちることを確かめる**

Run: `go test ./internal/tui/diff/ -run 'Click|Wheel' -v`
Expected: FAIL

- [ ] **Step 3: 実装する**

`internal/tui/diff/mouse.go`。要点:

```go
// paneTop is the first screen row the two panes occupy. It is derived from
// the same header the view draws, so the two cannot drift apart.
func (m Model) paneTop() int { return headerHeight }

// rowAt maps a screen position onto a row of the diff pane, or -1 when the
// position is not on one. m.top is the scroll offset the view is drawing
// with, which is why the hit test reads it rather than assuming zero.
func (m Model) rowAt(y int) int {
	i := m.top + (y - m.paneTop())
	if y < m.paneTop() || y >= m.paneTop()+m.paneHeight() || i >= len(m.rows) {
		return -1
	}
	return i
}
```

- クリックが `x < sidebarWidth` かつ `showSidebar()` ならファイル、そうでなければ行
- **選択中の行をもう一度クリックしたときだけ**、`rowCollapsed` を開く
- ホイールは `x` で行き先を決め、`moveRow` / `moveFile` を 1 段ずつ呼ぶ

- [ ] **Step 4: テストが通ることを確かめる**

Run: `go test ./internal/tui/diff/ -v`
Expected: PASS

- [ ] **Step 5: アサーションが空振りしていないことを確かめる**

`rowAt` の `m.top +` を消し、スクロールした状態でクリックするテストが落ちることを
確かめる。落ちないなら、スクロール後のクリック位置を検証していない。
**落ちないなら、スクロールしてからクリックするケースをテストに足す。**

- [ ] **Step 6: `make check` を通してコミット**

```
make check
```

コミットメッセージ:

```
feat: click and scroll the diff

The hit test reads the same sidebarWidth, paneHeight and scroll offset the
drawing does. A hit test that works the layout out a second time drifts the
first time either half changes.

The second click on an already-selected row is what opens a settled thread.
Bubble Tea does not report a double click, and timing one ourselves would
put a clock in Update.
```

---

## Task 12: 日本語の通し確認、spec と rules の更新、人手への引き継ぎ

**Files:**
- Modify: `docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md`
- Modify: `docs/superpowers/2026-09-06-phase1-followups.md`
- Create: `docs/superpowers/2026-09-06-phase2-handoff.md`
- Modify: `README.md` / `README.ja.md`

- [ ] **Step 1: すべての golden を日本語で読む**

```
make golden
cat -v internal/tui/diff/testdata/diff_ja_80.golden
cat -v internal/tui/diff/testdata/diff_ja_120.golden
cat -v internal/tui/diff/testdata/diff_ja_160.golden
cat -v internal/tui/review/testdata/review_ja_80.golden
```

見るもの:

- **桁が揃っているか。** 行番号のガターが日本語の行でずれていないか
- **キーバーが 80 桁で折り返していないか**
- **提出ポップアップの箱が 80 桁で溢れていないか**
- **色が付いているか。** `\033[` が本文にも現れているか（`Highlight` が効いている）

- [ ] **Step 2: `--icons` の 3 セットで録る**

`work` の `TestGoldenIconSets` と同じものを `diff` にも書く。
diff は `▸` や `▌` を使うので、**`ascii` セットで豆腐にならないこと**を確かめる。
diff で使う記号は `internal/tui/icon` に足す。ビューに直書きしない。

- [ ] **Step 3: spec を実装に合わせる**

§4.4.1 の表と、§4.6 の劣化の段を、**実際に作ったものへ書き直す。**
Phase 1 の 2 巡目でやったのと同じ作業である。特に:

- キーの一覧が実装と一致しているか（`X` は spec には無い。足す）
- サイドバー幅としきい値（22 桁 / 100 桁）を書く
- 実装しなかったもの（スレッドへの返信、解決）を「Phase 2 の範囲外」として書く

- [ ] **Step 4: Phase 1 の宿題に印を付ける**

`docs/superpowers/2026-09-06-phase1-followups.md` の
「2 巡目で範囲外にしたもの」の `d` / `v` / `m` キーの行を、
**`d` と `v` は Phase 2 で入った、`m` は Phase 3** に書き換える。

- [ ] **Step 5: README を更新する**

英日とも、キーの表に `d` / `v` を足し、レビューできることを機能の一覧に書く。
**`m`（merge）はまだ書かない。**

- [ ] **Step 6: 人手の検証手順を書く**

`docs/superpowers/2026-09-06-phase2-handoff.md`:

````markdown
# Phase 2 の受け渡し — 実端末での確認

spec §7 の Phase 2 の検証条件は「実際の PR に対して TUI からレビューを提出できる」。
**この確認には TTY と実在の PR が要り、開発環境（TTY 無し）では代行できない。**
golden で確かめられるのは桁と色までである。

## 用意するもの

自分が所有する、**捨ててよい PR**。他人の PR で試さないこと。
未提出のレビューは GitHub 側に残るので、途中でやめると相手に見えない
書きかけが残り続ける。

## 手順

1. 起動して PR を開き、`d` で diff が出ることを確かめる
2. `[` `]` でファイル、`{` `}` でハンクが動くことを確かめる
3. 追加された行で `c`、本文を書いて `ctrl+s`
4. **GitHub の Web UI で同じ PR を開き、コメントが「Pending」として、
   意図した行に付いていることを確かめる。** ここがこの手順の要点である
5. 削除された行でも同じことをし、**旧側の行に付くこと**を確かめる
6. `v` で提出し、Web UI でレビューが出ていることを確かめる
7. 別の PR で `c` して `X` を押し、`y` で破棄。Web UI から Pending が消えること

## 幅の確認

```bash
go run ./cmd/octoscope --lang ja
```

- **80 桁**: サイドバーが畳まれ、ヘッダにファイル名が出る。桁がずれない
- **100 桁**: サイドバーが出る
- **クリックした位置と選択が一致する**（golden は「描かれた位置」までしか見ない）

## 報告してほしいこと

- コメントが違う行に付いた場合、**その PR の URL と、押した行の見た目**
- 桁がずれた場合、**端末の種類とフォント**、`--icons` の指定
````

- [ ] **Step 7: `make check` を通してコミット**

```
make check
```

コミットメッセージ:

```
docs: say what Phase 2 built, and what a person still has to check

The phase's own acceptance condition -- submitting a review to a real pull
request -- needs a TTY and a pull request, neither of which this environment
has. The handoff note says what to do and, more to the point, what to look
at in the web UI: that a comment landed on the line it was written against.
```

---

## Self-Review

### Spec coverage（spec §7 の Phase 2 の項目）

| spec の要件 | Task |
|---|---|
| `internal/gh` に diff とレビューのドメイン型 | 1, 2 |
| `gh pr diff --color=never` のパース、両側の行番号 | 1 |
| diff ビューア: ファイルサイドバー | 5 |
| diff ビューア: ハンク単位の移動 | 5 |
| diff ビューア: シンタックスハイライト | 4 |
| diff ビューア: 既存レビュースレッドの表示 | 7 |
| 行コメント | 8 |
| レビュー提出（GitHub 側の pending review、4 mutation） | 3, 9 |
| ルートモデルのナビゲーションをスタックへ | 6 |
| §4.6 の劣化（100 桁未満でサイドバーを畳む） | 10 |
| §4.0 のマウス | 11 |
| en / ja × 80 / 120 / 160 の golden | 5, 7, 9, 10 |
| 検証は人手（TTY が要る） | 12 |

### 型と名前の整合

| 名前 | 定義 | 使う側 |
|---|---|---|
| `gh.DiffLine.Line() (int, gh.DiffSide)` | Task 1 | Task 7（スレッドの対応付け）、Task 8（投稿） |
| `gh.ReviewThread.Collapsed()` / `.Pending()` | Task 2 | Task 7（畳み）、Task 9（破棄の可否） |
| `gh.ReviewContext.PendingID` | Task 2 | Task 8（再利用）、Task 9（提出・破棄） |
| `cli.Client.AddReviewThread(reviewID, gh.PendingComment)` | Task 3 | Task 8 |
| `theme.Highlight(path, code)` | Task 4 | Task 5（本文の描画） |
| `sidebarWidth` / `gutterWidth` / `showSidebar()` | Task 5, 10 | Task 11（当たり判定） |
| `Model.rows` / `row.kind` / `row.hunk` / `row.key` | Task 5, 7 | Task 8, 11 |
| `diff.Source` | Task 5 で 1 メソッド、7 で 2 つ目、8 で 4 つ目まで増える | `app.Source` |

**`diff.Source` はタスクを追うごとに増える。** 最終形は 4 メソッド:

```go
type Source interface {
	PRDiff(ctx context.Context, repo string, number int) ([]gh.FileDiff, error)
	PRReviewContext(ctx context.Context, repo string, number int) (gh.ReviewContext, error)
	StartReview(pullRequestID string) (string, error)
	AddReviewThread(reviewID string, c gh.PendingComment) error
}
```

`DiscardReview` は `X` を押す `diff` が呼ぶので、Task 9 で 5 つ目として足す。
**最終形は 5 メソッド。**

```go
	DiscardReview(reviewID string) error
```

`review.Source` は 1 メソッド（`SubmitReview`）だけである。提出しか知らないので、
テスト用のフェイクも 1 メソッドで済む（`.claude/rules/architecture.md`）。

### 確認したこと（実装前に実機で確かめた事実）

- `gh pr review` に**行コメントの口は無い**（`--approve` / `--request-changes` /
  `--comment` と `--body` だけ）。だから GraphQL を使う
- GraphQL に `addPullRequestReview` / `addPullRequestReviewThread` /
  `submitPullRequestReview` / `deletePullRequestReview` がすべて存在する
- `AddPullRequestReviewThreadInput` は `pullRequestReviewId` / `path` / `body` /
  `line` / `side` / `startLine` / `startSide` / `subjectType` を取る
- `SubmitPullRequestReviewInput` は `pullRequestReviewId` / `event` / `body`
- `PullRequestReviewEvent` は `COMMENT` / `APPROVE` / `REQUEST_CHANGES` / `DISMISS`
- `gh api` の `-F` は値を解釈し（`@path` はファイル読み込み、整数は数値化）、
  `-f` は常に文字列。**利用者の入力は `-f`**
- `chroma/v2` は glamour 経由で既に依存グラフにある（`go.mod` の indirect）
