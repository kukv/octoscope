# unified diff のパーサーを gateway に移す実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development
> (recommended) or superpowers:executing-plans to implement this plan task-by-task.
> Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `domain.ParseDiff` / `domain.ParseBarePatch` を `internal/app/adapter/gateway/gh` に
移し、非公開の `parseDiff` / `parseBarePatch` にする。差分の**構造**（`FileDiff` / `Hunk` /
`DiffLine`）は domain に残す。ドメインが持つのは差分の形であって、テキストの読み取りではない。

**Architecture:** `diff_parse.go` とそのテスト、`testdata/` を丸ごと `gateway/gh` に移す。
呼び出しは同パッケージの `review.go:24`（`PRDiff` の raw 経路）と `review.go:126`
（`toFileDiff` の patch 経路）の 2 箇所だけなので、公開する必要が無くなる。
**パーサーの挙動は 1 行も変えない**——移動したテストがそのまま通ることが、その証拠になる。
画面も変わらない。

**Tech Stack:** Go、golangci-lint、gotestsum、`internal/golden`

**Spec:** as-is モデリングの違和感 F-08
（https://claude.ai/artifact/7HMQJ6kbp2wVs1LoiNG3JU）。
設計書は書かない——bounded な変更として利用者の承認を得ている（案 A: gateway/gh の中へ）。

---

## 着手前に調べた事実（2026-09-21）

### A. 呼び出しは 2 箇所、どちらも同じパッケージ

| 場所 | 中身 |
|---|---|
| `adapter/gateway/gh/review.go:24` | `return domain.ParseDiff(d.Raw), nil`（`PRDiff` の raw 経路） |
| `adapter/gateway/gh/review.go:126` | `fd.Hunks = domain.ParseBarePatch(*f.Patch)`（`toFileDiff` の files API 経路） |

`internal/app` の他のどこからも**製品コードでは**呼ばれていない。**だから非公開にできる。**
テストからの呼び出しが 1 本あり、それは事実 E で扱う。

### B. これは元いた場所への差し戻しである

パーサーは元々 `internal/gh/cli/diff.go` の非公開 `parseDiff` だった
（`docs/superpowers/plans/2026-09-06-octoscope-phase2.md:462`）。
2026-09-13 の再編（`2026-09-13-restructure-pr1-move.md`）で domain に運ばれた。
今回はアクセス層へ戻す。

### C. テストは domain のパッケージ内テストである

`domain/diff_parse_test.go`（159 行、`package domain`）は `FileDiff` / `FileStatus` /
`LineAdded` を**修飾なし**で書いている。`package gh` に移すと全部 `domain.` が要る。
**アサーションの中身は変えない。型修飾と関数名（`ParseDiff` → `parseDiff`）だけを直す。**

テスト関数は 4 本：

| 行 | 名前 |
|---|---|
| 24 | `TestPRDiffParsesEveryShape` |
| 74 | `TestLineNumbersRunDownBothSides` |
| 106 | `TestHunkHeaderFunctionContextDoesNotShiftLineNumbers` |
| 138 | `TestBarePatchParserMatchesTheFullDiffParser` |

ヘルパーは `readSample` / `sampleFiles` の 2 つで、domain 内の利用者はこのファイルだけ。
**`gh` パッケージに同名は無い**（`grep` 済み。`parseDiff` / `scanBuf` /
`hunkHeaderFields` も衝突しない）。

### D. `domain/testdata/` は `diff_parse_test.go` 専用

`sample.diff` を読むのは `diff_parse_test.go:12` だけ。**ディレクトリごと移す。**

`testdata/README.md` は「`internal/github/cli/diff_test.go` もこのファイルを読む
（`../testdata/sample.diff`）」と書いているが、**これは既に事実ではない**
（`grep -rn sample.diff --include=*.go` の結果は `diff_parse_test.go` の 3 行だけ）。
移す文書なので、この段落は落とす。

### E. `domain.ParseDiff` を呼ぶテストが 1 本あり、移動より先に片付ける必要がある

`gh/review_test.go:159` の `want := domain.ParseDiff(raw)` は**コンパイルされる呼び出し**である。
先に移すと `undefined: domain.ParseDiff` でテストパッケージが壊れるので、
**書き下しへの置き換えを Task 1 に置き、移動を Task 2 にする。**

このテスト（`TestPRDiffFromRawUsesParseDiff`）は `got` を `domain.ParseDiff(raw)` と
比べているだけなので、パーサーが同じパッケージに来れば `parseDiff(raw)` 同士の比較になり、
**何も検査しなくなる**。期待値を書き下す（利用者が承認）。隣の
`TestPRDiffFromFilesTranslatesTheWireShapeIntoTheDomain` が既に全フィールド書き下しの
形をとっているので、パッケージの作法にも揃う。

同テストのコメントにある「Binary is guarded instead by `domain.ParseDiff`'s own tests」
（`review_test.go:171`）は名前が変わるので、移動と同じ Task 2 で直す。

### F. 対象外

- `domain/diff.go`（`FileDiff` / `Hunk` / `DiffLine` / `DiffSide` / `FileStatus` と
  `DiffLine.Line()`）。**差分の構造はドメインの概念**として残す
- サービス中立な共有パッケージ（`adapter/gateway/diffparse` など）を作ること。
  unified diff は git の形式なので 2 つ目の gateway なら再利用できるが、横断（F-11〜F-13）は
  保留中であり、**利用者が 1 つしかない抽象を先に作らない**
- `internal/github` 側の diff 取得（`github.Diff` のワイヤ型）

### G. golden への影響は無いはず

パーサーの挙動が変わらないので画面も変わらない。**golden 354 枚が 1 枚も変わらないことが
合格条件。** golden は `internal/app/presentation/**/testdata/` にしか無い
（`domain/testdata/` の移動と混ざらないよう、検査はブランチの差分で見る）。

## Global Constraints

- **golden 354 枚（`internal/app/presentation/**/testdata/*.golden`）を 1 バイトも変えない。**
  `OCTOSCOPE_UPDATE_GOLDEN` を使わない
- **パーサーの挙動を変えない。** 移したテストの**アサーションを書き換えない**
  （型修飾と関数名の変更だけが許される差分）
- **i18n のカタログを触らない**
- 各タスクの終わりに `make check` が通ること

---

### Task 1: 自己同言になるテストを期待値の書き下しにする

移動より**先**に行う（事実 E）。この時点では `domain.ParseDiff` はまだ存在するので、
このタスク単独でビルドもテストも通る。

**Files:**
- Modify: `internal/app/adapter/gateway/gh/review_test.go:142-164`

**Interfaces:**
- 変えない。テストだけの変更である

- [ ] **Step 1: 期待値を書き下したテストに置き換える**

`TestPRDiffFromRawUsesParseDiff` を丸ごと次に差し替える。入力の raw は変えない。

```go
// TestPRDiffFromRawParsesTheDiffText covers the common path: gh pr diff or
// the REST diff media type answered, so the backend hands back raw unified
// diff text and the gateway reads it. The expectation is written out rather
// than run through the parser: a test that parses on both sides compares
// the parser against itself and would hold whatever it did.
func TestPRDiffFromRawParsesTheDiffText(t *testing.T) {
	t.Parallel()

	raw := []byte("diff --git a/x.go b/x.go\n" +
		"@@ -1,1 +1,1 @@\n-old\n+new\n")

	g := New(fakeBackend{prDiff: func(context.Context, string, int) (github.Diff, error) {
		return github.Diff{Raw: raw}, nil
	}})

	got, err := g.PRDiff(context.Background(), "kukv/octoscope", 1)
	if err != nil {
		t.Fatalf("PRDiff: %v", err)
	}

	want := []domain.FileDiff{{
		Path:      "x.go",
		Status:    domain.FileModified,
		Additions: 1,
		Deletions: 1,
		Hunks: []domain.Hunk{{
			Header: "@@ -1,1 +1,1 @@",
			Lines: []domain.DiffLine{
				{Kind: domain.LineRemoved, OldLine: 1, Text: "old"},
				{Kind: domain.LineAdded, NewLine: 1, Text: "new"},
			},
		}},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("PRDiff() = %+v, want %+v", got, want)
	}
}
```

- [ ] **Step 2: 通ることを確かめる**

Run: `go test ./internal/app/adapter/gateway/gh/ -run TestPRDiffFromRawParsesTheDiffText -v`
Expected: PASS

- [ ] **Step 3: 書き下した期待値が本当に検査していることを確かめる**

`want` の `Deletions: 1` を一時的に `Deletions: 2` に書き換えて実行する。

Run: `go test ./internal/app/adapter/gateway/gh/ -run TestPRDiffFromRawParsesTheDiffText`
Expected: **FAIL**。落ちなければ期待値が本物を見ていないということなので、そこで止める。
確認できたら `Deletions: 1` に戻す

- [ ] **Step 4: `domain.ParseDiff` の製品外からの呼び出しが消えたことを確かめる**

Run: `grep -rn "domain.ParseDiff\|domain.ParseBarePatch" --include="*.go" .`
Expected: `review.go:24` と `review.go:126` の**製品コード 2 行だけ**
（`review_test.go:171` のコメント 1 行も残る。Task 2 で直す）

- [ ] **Step 5: コミット**

```bash
git add internal/app/adapter/gateway/gh/review_test.go
git commit -m "$(cat <<'EOF'
test: check the parsed diff against a written-out expectation

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: パーサーとそのテストを gateway/gh へ移す

移動・非公開化・呼び出し側の修正を**一度に行う**。パッケージをまたぐ移動なので、
途中でコンパイルが通る切り方が無い。

**Files:**
- Create: `internal/app/adapter/gateway/gh/diff_parse.go`（`internal/app/domain/diff_parse.go` の移動）
- Create: `internal/app/adapter/gateway/gh/diff_parse_test.go`（`internal/app/domain/diff_parse_test.go` の移動）
- Create: `internal/app/adapter/gateway/gh/testdata/sample.diff`、`.../testdata/README.md`（`internal/app/domain/testdata/` の移動）
- Delete: `internal/app/domain/diff_parse.go`、`internal/app/domain/diff_parse_test.go`、`internal/app/domain/testdata/`
- Modify: `internal/app/adapter/gateway/gh/review.go:24,126`
- Modify: `internal/app/adapter/gateway/gh/review_test.go:171`（コメントの名前）

**Interfaces:**
- Produces: `func parseDiff(b []byte) []domain.FileDiff`、`func parseBarePatch(patch string) []domain.Hunk`
  （どちらも `package gh` の非公開関数）
- 消えるもの: `domain.ParseDiff`、`domain.ParseBarePatch`

- [ ] **Step 1: ファイルを git 履歴つきで移す**

```bash
git mv internal/app/domain/diff_parse.go      internal/app/adapter/gateway/gh/diff_parse.go
git mv internal/app/domain/diff_parse_test.go internal/app/adapter/gateway/gh/diff_parse_test.go
git mv internal/app/domain/testdata           internal/app/adapter/gateway/gh/testdata
```

- [ ] **Step 2: 落ちることを確かめる**

Run: `go build ./... 2>&1 | head`
Expected: `internal/app/adapter/gateway/gh` がコンパイルエラー
（`package domain` と宣言されたファイルが `gh` ディレクトリにある）

- [ ] **Step 3: `diff_parse.go` をパッケージ `gh` に合わせる**

先頭を書き換える。

```go
package gh

import (
	"bufio"
	"bytes"
	"strconv"
	"strings"

	"github.com/kukv/octoscope/internal/app/domain"
)
```

公開 2 関数を非公開にし、doc コメントの先頭語も合わせる。
**本文（`p.line` / `hunkStarts` / `pathFromGitHeader` / `firstNumber` と定数 3 つ）は
アルゴリズムを一切変えない。** 変えるのは型修飾だけである。

```go
// parseDiff reads a unified diff. It never fails: a line it does not
// recognise inside a hunk is dropped, and one outside a hunk is a header we
// have no use for. A diff that half-parses shows a file short; a parser that
// returns an error shows nothing at all, which is worse.
//
// It lives here rather than in the domain because reading a text format is
// translation: the domain owns the shape of a diff (domain.FileDiff), not
// the spelling git writes it in.
func parseDiff(b []byte) []domain.FileDiff {
```

```go
// parseBarePatch reads the hunks out of a files-API patch: unified diff
// hunks with no "diff --git" header and no ---/+++ lines. It walks the same
// diffParser used for a full gh pr diff, entering it already "inside" a
// file, so the hunk-header parsing (hunkStarts, with its function-context
// fix) is shared rather than duplicated.
func parseBarePatch(patch string) []domain.Hunk {
	p := &diffParser{file: &domain.FileDiff{}}
```

ファイル内で修飾が要るのは次の識別子である（`gofmt` ではなく目で数えること）：
`FileDiff` / `Hunk` / `DiffLine` / `FileModified` / `FileAdded` / `FileDeleted` /
`FileRenamed` / `LineAdded` / `LineRemoved` / `LineContext`。
`diffParser` 構造体のフィールド型（`files []domain.FileDiff`、`file *domain.FileDiff`、
`hunk *domain.Hunk`）と `add(l domain.DiffLine)` も同様。

- [ ] **Step 4: 呼び出し 2 箇所を非公開名に直す**

`internal/app/adapter/gateway/gh/review.go:24`:

```go
	return parseDiff(d.Raw), nil
```

`internal/app/adapter/gateway/gh/review.go:126`:

```go
	fd.Hunks = parseBarePatch(*f.Patch)
```

- [ ] **Step 5: テストをパッケージ `gh` に合わせる**

`internal/app/adapter/gateway/gh/diff_parse_test.go` の先頭。

```go
package gh

import (
	"bytes"
	"os"
	"reflect"
	"testing"

	"github.com/kukv/octoscope/internal/app/domain"
)
```

ヘルパー 2 つ。

```go
func sampleFiles(t *testing.T) []domain.FileDiff {
	t.Helper()
	return parseDiff(readSample(t))
}
```

**4 本のテストのアサーションは 1 つも書き換えない。** `FileDiff` → `domain.FileDiff`、
`FileModified` → `domain.FileModified`、`LineAdded` → `domain.LineAdded`、
`SideLeft` → `domain.SideLeft` のような修飾と、`ParseDiff` → `parseDiff` /
`ParseBarePatch` → `parseBarePatch` だけを直す。
`readSample` の `os.ReadFile("testdata/sample.diff")` はパスも変えない
（`testdata/` ごと隣に移っているため）。

- [ ] **Step 6: testdata の README から事実でない段落を落とす**

`internal/app/adapter/gateway/gh/testdata/README.md` を次の内容にする。

```markdown
# testdata

## `sample.diff`

`git diff` 形式のパース用。unified diff の hunk ヘッダ、追加、削除、文脈行を含む。

パースそのものを見るテストは `diff_parse_test.go` にある。
```

- [ ] **Step 7: 隣のテストのコメントの名前を直す**

`review_test.go:171` 付近、`TestPRDiffFromFilesTranslatesTheWireShapeIntoTheDomain` の
doc コメント末尾。

変更前:

```go
// by this test, and Binary is guarded instead by domain.ParseDiff's own
// tests.
```

変更後:

```go
// by this test, and Binary is guarded instead by parseDiff's own tests in
// diff_parse_test.go.
```

- [ ] **Step 8: 通ることを確かめる**

Run: `go test ./internal/app/adapter/gateway/gh/ ./internal/app/domain/`
Expected: 両方 PASS。`gh` 側で `TestPRDiffParsesEveryShape` /
`TestLineNumbersRunDownBothSides` /
`TestHunkHeaderFunctionContextDoesNotShiftLineNumbers` /
`TestBarePatchParserMatchesTheFullDiffParser` の 4 本が走っている

Run: `grep -rn "ParseDiff\|ParseBarePatch" --include="*.go" .`
Expected: **0 件**

- [ ] **Step 9: コミット**

```bash
git add -A internal/app
git commit -m "$(cat <<'EOF'
refactor: read a unified diff where the service is translated

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: 検査と記録

**Files:**
- Modify: `.claude/rules/architecture.md`（「GitHub API 固有の値はパッケージの外に出さない」の節末、105 行目の直後）

- [ ] **Step 1: CI と同じ検査を全部通す**

Run: `make check`
Expected: tidy / lint / fmt / test すべて成功

- [ ] **Step 2: golden が 1 枚も変わっていないことを確かめる**

ブランチ全体の差分で見る（`git status` はコミット済みの変更を映さない）。

Run: `git diff --name-only origin/main...HEAD | grep '\.golden$'`
Expected: **出力が空**（grep の終了コードは 1 になる）

Run: `find . -name "*.golden" | wc -l`
Expected: `354`

1 枚でも動いていたら、事実 G の前提が誤っていたということなので止めて報告する。

- [ ] **Step 3: 規約に今回の判断を書く**

`.claude/rules/architecture.md` の「GitHub API 固有の値はパッケージの外に出さない」節、
depguard の 3 点を挙げた箇条書きの直後（105 行目のあと、
「## 複数の API 呼び出しは…」の見出しの手前）に足す。

```markdown
**テキスト形式の解釈も同じ側にある。** unified diff を読む `parseDiff` /
`parseBarePatch` は `internal/app/adapter/gateway/gh` にあり、domain には無い
（2026-09-21 に移した。それ以前は `domain.ParseDiff` だった）。
**domain が持つのは差分の形**——`FileDiff` / `Hunk` / `DiffLine` と
「削除された行には新側の行番号が無い」といった型が負うルール——**であって、
git がそれをどう綴るかではない。** #124 の線（翻訳は ACL、政策はドメイン）では、
テキスト形式の読み取りは翻訳にあたる。

**共有パッケージを作らなかった。** unified diff は git の形式であって GitHub の
形式ではないので、2 つ目の gateway があれば再利用できる。それでも
`gateway/gh` の中に非公開で置いたのは、横断（F-11〜F-13）が保留中で
**利用者が 1 つしかない抽象を先に作らない**ためである。2 つ目が現れた時点で
中立なパッケージに切り出す——そのとき動かすのは 1 ファイルとそのテストだけで済む。
```

- [ ] **Step 4: 実機で見る**

**Claude 側からは実行できない**（pty の起動がガードに阻まれる）。利用者に依頼する。
パーサーの挙動は変えていないので、見るのは差分ビューが今までどおり描けることだけ。

```bash
go run ./cmd/octoscope           # PR を開き d で差分を見る（行番号・追加/削除の色）
go run ./cmd/octoscope --lang ja # 同じ画面を日本語で
```

- [ ] **Step 5: コミットして PR**

```bash
git add .claude/rules/architecture.md
git commit -m "$(cat <<'EOF'
docs: record that reading a diff's text is translation

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
)"
```

PR 本文には、実機確認を利用者が行ったか未実施かを明記する。

## 完了条件

- [ ] `grep -rn "ParseDiff\|ParseBarePatch" --include="*.go" .` が 0 件
- [ ] `internal/app/domain` に `diff_parse.go` / `diff_parse_test.go` / `testdata/` が無い
- [ ] `internal/app/adapter/gateway/gh` に `parseDiff` / `parseBarePatch` が非公開で 1 つずつある
- [ ] パーサーのテスト 4 本が `gh` パッケージで走り、アサーションが移動前と同じである
- [ ] `TestPRDiffFromRawParsesTheDiffText` が書き下した期待値を見ており、値を壊すと落ちる
- [ ] golden 354 枚が 1 バイトも変わっていない
- [ ] `make check` が通る
- [ ] `.claude/rules/architecture.md` に判断と、共有パッケージを作らなかった理由が書かれている

## マージ後にやること（計画の対象外だが落とさない）

- Artifact（https://claude.ai/artifact/7HMQJ6kbp2wVs1LoiNG3JU）の F-08 カードに完了の status を足す
- メモリ `octoscope-ddd-refactor-goal.md` の「次にやること」を更新する（残りは F-05 / F-09 / F-10）
