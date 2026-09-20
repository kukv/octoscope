# 遷移メッセージの共通化と、保存クエリの不透明性の明文化

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development
> (recommended) or superpowers:executing-plans to implement this plan task-by-task.
> Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 違和感 **F-09** と **F-10** を片付ける。
F-09: work / repo / search が個別に定義している `OpenDetailMsg` / `OpenDiffMsg` /
`OpenChecksMsg`（8 型）を新パッケージ `presentation/tui/nav` の 3 型に畳む。
F-10: `domain.SavedQuery.Query` が不透明な値であることを doc に明文化する（コード変更なし）。

**Architecture:** `nav` は `domain` だけを import する葉パッケージで、責務は
「タブが別の画面を求める信号」の 1 つ。**`detail` の 2 型は畳まない**——root が
`openDiffOverDetail` / `openChecksOverDetail` に回しており、「詳細の上に重ねる」は
別の意味だからである。root の `switch` は 10 本から 5 本になる。
**画面の振る舞いは変わらない。**

**Tech Stack:** Go、Bubble Tea v2、golangci-lint、gotestsum、`internal/golden`

**Spec:** as-is モデリングの違和感 F-09 / F-10
（https://claude.ai/artifact/7HMQJ6kbp2wVs1LoiNG3JU）。
設計書は書かない——bounded な変更として利用者の承認を得ている。

---

## 着手前に調べた事実（2026-09-21）

### A. root は送信元で挙動を変えている。ただし detail だけ

`internal/app/presentation/tui/root/root.go:275-293` の 10 本。

| 送信元の型 | root の呼び先 |
|---|---|
| `work.OpenDetailMsg` / `repo.OpenDetailMsg` / `search.OpenDetailMsg` | `m.openDetail(msg.Ref)` |
| `work.OpenDiffMsg` / `repo.OpenDiffMsg` / `search.OpenDiffMsg` | `m.openDiff(msg.Ref)` |
| **`detail.OpenDiffMsg`** | **`m.openDiffOverDetail(msg.Ref)`** |
| `work.OpenChecksMsg` / `repo.OpenChecksMsg` | `m.openChecks(msg.Ref)` |
| **`detail.OpenChecksMsg`** | **`m.openChecksOverDetail(msg.Ref)`** |

**タブ 3 つの 8 型は完全に同じ扱い。** Artifact の F-09 が挙げた代償
（共通化すると発信元が消える）が実際に効くのは detail の 2 型だけである。

### B. 中身は 10 型すべて `struct{ Ref domain.ItemRef }`

doc コメントの文言だけが少しずつ違う（「the selected card」「one item」
「the shown pull request」）。

### C. 触るファイルは 15、参照は 99

| ファイル | 参照数 | 役割 |
|---|---|---|
| `root/root_test.go` | 29 | 最多。`work.OpenDetailMsg` などで遷移を起こす |
| `root/root.go` | 10 | `switch` 10 本 |
| `repo/repo_test.go` | 10 | |
| `work/work.go` | 9 | 定義 3 + 送信 3 |
| `repo/repo.go` | 9 | 定義 3 + 送信 3 |
| `work/work_test.go` | 6 | |
| `search/search.go` | 6 | 定義 2 + 送信 2 |
| `detail/detail.go` | 4 | **定義 2。触らない** |
| `detail/detail_test.go` | 4 | **触らない** |
| `search/search_test.go` | 4 | |
| `detail/keys.go` | 2 | **触らない** |
| `work/mouse_test.go` / `repo/mouse_test.go` | 各 2 | |
| `work/mouse.go` / `repo/mouse.go` | 各 1 | `OpenDetailMsg{ref}`（**位置指定**の複合リテラル） |

送信側は 10 箇所。`work/work.go:211`、`work/mouse.go:29`、`repo/repo.go:535`、
`repo/mouse.go:57` の 4 つは `OpenDetailMsg{ref}` という**フィールド名なしの**リテラルで、
残りは `OpenDiffMsg{Ref: ref}` 形式。

### D. `search` には `OpenChecksMsg` が無い

Search タブは c キーを CI に割り当てていない。**共通化してもキーバインドは変えない**
（投げないだけ）。型の側からは非対称が消えるが、これは振る舞いの変更ではない。

### E. F-10: 保存クエリは実際に不透明である

- 保存: `search/search.go:568` が `domain.SavedQuery{Name: name, Query: m.query()}` を作る
- 再生: `search/search.go:373` が `m.raw = m.saved[m.pick].Query`、
  `m.query()` がそれをそのまま返し、`runSearch` に渡る
- **フィルタに戻す経路は無い。** octoscope はこの文字列を書いて流すだけで、読まない

`domain/query.go` の現コメントは「サービス自身の検索構文だから string である」までは
言っているが、**「解釈しない」とは言っていない**。足りないのはそこだけ。

### F. 対象外

- `detail.OpenDiffMsg` / `detail.OpenChecksMsg`（事実 A）
- `FatalMsg` / `ErrorMsg` / `ClosedMsg` の重複。**これらは中身も意味も違う**
  （`work.FatalMsg` と `search.FatalMsg` の doc は別のことを言っている）
- F-14（検索クエリの組み立てを gateway へ）と `domain.SearchQuery` 型の導入。保留中
- キーバインドの変更

### G. golden への影響は無いはず

送るメッセージの**型名**が変わるだけで、root が何を開くかは変わらない。
**golden 354 枚が 1 枚も変わらないことが合格条件。**

## Global Constraints

- **golden 354 枚（`internal/app/presentation/**/testdata/*.golden`）を 1 バイトも変えない。**
  `OCTOSCOPE_UPDATE_GOLDEN` を使わない
- **キーバインドと画面の振る舞いを変えない**
- **i18n のカタログを触らない**
- `detail` パッケージの 2 型には触らない
- 各タスクの終わりに `make check` が通ること

---

### Task 1: `nav` パッケージを作り、3 タブを載せ替える

一度に行う。型を消すと 3 タブと root が同時にコンパイルを失うので、
途中でビルドが通る切り方が無い。

**Files:**
- Create: `internal/app/presentation/tui/nav/nav.go`
- Modify: `internal/app/presentation/tui/work/work.go`（定義 3 を削除、送信 3 を `nav.` に）
- Modify: `internal/app/presentation/tui/work/mouse.go:29`
- Modify: `internal/app/presentation/tui/repo/repo.go`（定義 3 を削除、送信 3 を `nav.` に）
- Modify: `internal/app/presentation/tui/repo/mouse.go:57`
- Modify: `internal/app/presentation/tui/search/search.go`（定義 2 を削除、送信 2 を `nav.` に）
- Modify: `internal/app/presentation/tui/root/root.go:275-293`（`switch` 10 本 → 5 本）
- Test: `work/work_test.go`、`work/mouse_test.go`、`repo/repo_test.go`、
  `repo/mouse_test.go`、`search/search_test.go`、`root/root_test.go`

**Interfaces:**
- Produces: `nav.OpenDetailMsg`、`nav.OpenDiffMsg`、`nav.OpenChecksMsg`
  （いずれも `struct{ Ref domain.ItemRef }`）
- 消えるもの: `work.OpenDetailMsg` / `work.OpenDiffMsg` / `work.OpenChecksMsg`、
  `repo.OpenDetailMsg` / `repo.OpenDiffMsg` / `repo.OpenChecksMsg`、
  `search.OpenDetailMsg` / `search.OpenDiffMsg`
- 残るもの: `detail.OpenDiffMsg`、`detail.OpenChecksMsg`

- [ ] **Step 1: 失敗するテストを書く（root が nav のメッセージを受けること）**

`internal/app/presentation/tui/root/root_test.go:531` の
`TestOpenDetailMsgShowsTheDetailView` の `work.OpenDetailMsg{` を
`nav.OpenDetailMsg{` に変える。**このテストは既にその経路を通っている**ので、
`nav` がまだ無い時点でコンパイルに失敗する。

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/app/presentation/tui/root/`
Expected: コンパイルエラー（`nav` パッケージが存在しない）

- [ ] **Step 3: `nav` パッケージを作る**

`internal/app/presentation/tui/nav/nav.go` を新規作成する。

```go
// Package nav holds the messages a tab sends when the user asks for another
// screen. A tab knows what the user picked, not what opens: root decides
// that, and these three types are the whole vocabulary between them.
//
// The detail view has its own OpenDiffMsg and OpenChecksMsg rather than
// using these. That is not duplication: root answers them with
// openDiffOverDetail and openChecksOverDetail, so the sending package is
// what says whether the new screen stacks on top of a detail view.
package nav

import "github.com/kukv/octoscope/internal/app/domain"

// OpenDetailMsg asks the parent to show the detail view for one item.
type OpenDetailMsg struct{ Ref domain.ItemRef }

// OpenDiffMsg asks the parent to show the diff of the selected pull request.
type OpenDiffMsg struct{ Ref domain.ItemRef }

// OpenChecksMsg asks the parent to show the checks of the selected pull
// request.
type OpenChecksMsg struct{ Ref domain.ItemRef }
```

- [ ] **Step 4: work タブを載せ替える**

`internal/app/presentation/tui/work/work.go`:

- 定義 3 つ（`OpenDetailMsg` / `OpenDiffMsg` / `OpenChecksMsg` と各 doc コメント、
  33-40 行付近）を削除する。**`FatalMsg` は残す**
- import に `"github.com/kukv/octoscope/internal/app/presentation/tui/nav"` を足す
- `work.go:211` `return m, func() tea.Msg { return OpenDetailMsg{ref} }`
  → `return m, func() tea.Msg { return nav.OpenDetailMsg{Ref: ref} }`
  （**フィールド名を補う**。位置指定のままでもコンパイルは通るが、`go vet` の
  composites を避け、同ファイル内の他の送信と形を揃える）
- `work.go:220` → `return nav.OpenDiffMsg{Ref: ref}`
- `work.go:227` → `return nav.OpenChecksMsg{Ref: ref}`

`internal/app/presentation/tui/work/mouse.go:29` → `return nav.OpenDetailMsg{Ref: ref}`、
import を足す。

- [ ] **Step 5: repo タブを載せ替える**

`internal/app/presentation/tui/repo/repo.go`:

- 定義 3 つ（52-59 行付近）を削除。**`FatalMsg` は残す**
- import に `nav` を足す
- `repo.go:535` → `return nav.OpenDetailMsg{Ref: ref}`
- `repo.go:550` → `return nav.OpenDiffMsg{Ref: ref}`
- `repo.go:557` → `return nav.OpenChecksMsg{Ref: ref}`

`internal/app/presentation/tui/repo/mouse.go:57` → `return nav.OpenDetailMsg{Ref: ref}`、
import を足す。

- [ ] **Step 6: search タブを載せ替える**

`internal/app/presentation/tui/search/search.go`:

- 定義 2 つ（40-43 行付近）を削除。**`FatalMsg` は残す**
- import に `nav` を足す
- `search.go:453` → `return nav.OpenDetailMsg{Ref: ref}`
- `search.go:461` → `return nav.OpenDiffMsg{Ref: ref}`

**`nav.OpenChecksMsg` を search から投げない。** Search タブは CI を開くキーを
持っていない（事実 D）。

- [ ] **Step 7: root の `switch` を畳む**

`internal/app/presentation/tui/root/root.go:275-293` を次に置き換える。

```go
	case nav.OpenDetailMsg:
		return m.openDetail(msg.Ref)
	case nav.OpenDiffMsg:
		return m.openDiff(msg.Ref)
	case detail.OpenDiffMsg:
		return m.openDiffOverDetail(msg.Ref)
	case nav.OpenChecksMsg:
		return m.openChecks(msg.Ref)
	case detail.OpenChecksMsg:
		return m.openChecksOverDetail(msg.Ref)
```

import を整える。`work` / `repo` / `search` の import は**消さない**——
それぞれのモデル型（`work.New` など）を root はまだ使っている。`nav` を足す。

- [ ] **Step 8: テストの参照を載せ替える**

型名の変更だけで、**アサーションは 1 つも書き換えない**。

| ファイル | 置換 |
|---|---|
| `root/root_test.go` | `work.OpenDetailMsg` / `repo.OpenDetailMsg` / `work.OpenDiffMsg` / `work.OpenChecksMsg` → `nav.*`。**`detail.OpenDiffMsg` / `detail.OpenChecksMsg` はそのまま** |
| `work/work_test.go`、`work/mouse_test.go` | `OpenDetailMsg` などの裸の参照 → `nav.*` |
| `repo/repo_test.go`、`repo/mouse_test.go` | 同上 |
| `search/search_test.go` | 同上 |
| `detail/detail_test.go` | **触らない** |

`root/root_test.go:546` の `TestRepoOpenDetailMsgShowsTheDetailView` は
`repo.OpenDetailMsg` を `nav.OpenDetailMsg` にすると
`TestOpenDetailMsgShowsTheDetailView`（531 行）の**部分集合になる**——
両者は同じ `newTestModel(Options{Repo: "kukv/demo"})` から始まり、531 行のほうは
開いたあと `detail.ClosedMsg` で閉じることまで見ている（546 行は開くことだけ）。

**531 行を残し、546 行を丸ごと削除する。** 残す側の doc コメントに 1 行足す。

```go
// TestOpenDetailMsgShowsTheDetailView: any tab sends the same nav message,
// so root's answer does not depend on which one the user was looking at.
func TestOpenDetailMsgShowsTheDetailView(t *testing.T) {
```

- [ ] **Step 9: 通ることを確かめる**

Run: `go test ./internal/app/presentation/...`
Expected: すべて PASS

Run: `grep -rn "work.OpenDetailMsg\|repo.OpenDetailMsg\|search.OpenDetailMsg\|work.OpenDiffMsg\|repo.OpenDiffMsg\|search.OpenDiffMsg\|work.OpenChecksMsg\|repo.OpenChecksMsg" --include="*.go" .`
Expected: **0 件**

Run: `grep -rn "detail.OpenDiffMsg\|detail.OpenChecksMsg" --include="*.go" internal | wc -l`
Expected: **0 でない**（detail の 2 型は残っている）

- [ ] **Step 10: golden が動いていないことを確かめる**

Run: `git status --porcelain -- 'internal/app/presentation'`
Expected: 出力に `.golden` を含む行が**無い**（変更したのは `.go` だけ）

- [ ] **Step 11: コミット**

```bash
git add -A internal/app/presentation
git commit -m "$(cat <<'EOF'
refactor: give the tabs one vocabulary for asking to open a screen

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: 保存クエリが不透明であることを doc に書く（F-10）

**Files:**
- Modify: `internal/app/domain/query.go`

**Interfaces:**
- 変えない。コメントだけの変更である

- [ ] **Step 1: コメントを書き換える**

`internal/app/domain/query.go` を次にする。

```go
package domain

// SavedQuery is one of the Search tab's saved queries: what the user called
// it, and the search it stands for.
//
// Query is opaque to this application. It is the service's own search
// syntax, and octoscope never reads into it: the Search tab writes one from
// its filters (or takes what the user typed), stores it, and hands it back
// unchanged when the user picks that row. Nothing parses one into filters
// again. That is why it is a string rather than a parsed structure, and why
// a query saved against one service does not carry to another.
type SavedQuery struct {
	Name  string
	Query string
}
```

- [ ] **Step 2: 通ることを確かめる**

Run: `go test ./internal/app/domain/`
Expected: PASS（コメントだけの変更なので落ちる理由が無い）

- [ ] **Step 3: コミット**

```bash
git add internal/app/domain/query.go
git commit -m "$(cat <<'EOF'
docs: say that a saved query is one the application never reads

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: 検査と記録

**Files:**
- Modify: `.claude/rules/tui.md`（「サブモデル」節の末尾）

- [ ] **Step 1: CI と同じ検査を全部通す**

Run: `make check`
Expected: tidy / lint / fmt / test すべて成功

- [ ] **Step 2: golden が 1 枚も変わっていないことを確かめる**

Run: `git diff --name-only origin/main...HEAD | grep '\.golden$'`
Expected: **出力が空**（grep の終了コードは 1）

Run: `find . -name "*.golden" | wc -l`
Expected: `354`

- [ ] **Step 3: 規約に書く**

`.claude/rules/tui.md` の「## サブモデル」節の末尾（「サブモデルが必要とする
データ取得の interface は…」の段落のあと、「## UI の状態は enum」の見出しの手前）に
足す。

```markdown
**画面遷移の信号は `presentation/tui/nav` にある。** タブ（work / repo / search）が
「これを開いてほしい」と言うための型は `nav.OpenDetailMsg` / `nav.OpenDiffMsg` /
`nav.OpenChecksMsg` の 3 つで、タブごとに定義しない（2026-09-21 に畳んだ。
それ以前は 3 パッケージが同じ型を別々に持ち、root が 10 本の case で受けていた）。

**ただし `detail` は自分の `OpenDiffMsg` / `OpenChecksMsg` を持つ。これは重複ではない。**
root はタブからのものを `openDiff` に、detail からのものを `openDiffOverDetail` に回す——
**どのパッケージが送ったかが「詳細の上に重ねるか」を意味している。** 畳むと、
コンパイラが保証しているこの区別が実行時の bool になる。同じ形の struct が 2 つ
並んでいるのを見て 1 つにしたくなったら、root の `switch` の答えが同じかを先に見ること。
```

- [ ] **Step 4: 実機で見る**

**Claude 側からは実行できない**（pty の起動がガードに阻まれる）。利用者に依頼する。
遷移そのものを触ったので、**3 つのタブすべてから開く**。

```bash
go run ./cmd/octoscope
# Work: Enter で詳細 / d で差分 / s で CI
# Repos: 同じ 3 つ
# Search: Enter で詳細 / d で差分
# 詳細を開いた状態から d と s（詳細の上に重なることを見る）
go run ./cmd/octoscope --lang ja
```

- [ ] **Step 5: コミット**

```bash
git add .claude/rules/tui.md
git commit -m "$(cat <<'EOF'
docs: record where a tab's request to open a screen lives

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
)"
```

PR 本文には、実機確認を利用者が行ったか未実施かを明記する。

## 完了条件

- [ ] `work` / `repo` / `search` に `Open*Msg` の定義が 1 つも無い
- [ ] `nav` パッケージに 3 型があり、`domain` 以外を import していない
- [ ] `detail.OpenDiffMsg` / `detail.OpenChecksMsg` は残っている
- [ ] `root.go` の遷移の `case` が 5 本（`nav` 3 + `detail` 2）
- [ ] `domain/query.go` のコメントが「octoscope はこの文字列を読まない」と言っている
- [ ] golden 354 枚が 1 バイトも変わっていない
- [ ] `make check` が通る
- [ ] `.claude/rules/tui.md` に、畳んだことと **detail を畳まなかった理由**が書かれている

## マージ後にやること（計画の対象外だが落とさない）

- Artifact の F-09 / F-10 カードに完了の status を足す（F-09 には「root は detail だけ
  扱いを変えていた」という調査結果を書く）
- メモリ `octoscope-ddd-refactor-goal.md` を更新（残るのは F-05 と、depguard の宿題）
