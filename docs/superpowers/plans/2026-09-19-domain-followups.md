# domain の積み残し 3 件 — 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** domain の分割で「振る舞いを変えないため別件にする」と残した 3 件を片付ける。テストを足し、`Kind ==` の比較を型のメソッドに寄せ、読み手のいないフィールドを消す。

**Architecture:** 5 タスク。Task 1 は実装に触れずテストだけを足す。Task 2 は新しいメソッドを TDD で足す。Task 3 はその置き換え。Task 4 はフィールドの削除。Task 5 で仕上げる。どのタスクも振る舞いを変えないので、golden ファイルは 1 つも変わらない。

**Tech Stack:** Go、`make check`（tidy / lint / fmt / test）、`gofumpt`、`golangci-lint`

**設計:** `docs/superpowers/specs/2026-09-19-domain-followups-design.md`

---

## 前提

作業前に必ず読む。

- 設計: `docs/superpowers/specs/2026-09-19-domain-followups-design.md`
- 規約: `.claude/rules/architecture.md` と `.claude/rules/testing.md`（該当ファイルを触ると自動で読み込まれる）

**全タスク共通:**

- テストファイルは `package domain_test` で、先頭に `t.Parallel()` を書く
  （同ディレクトリの `item_ref_test.go` に合わせる）
- `make check` が通らない状態でコミットしない
- **`testdata/*.golden` を 1 つも変更しない。** 変わったら実装が間違っている
- `git` コマンドは単純な形で 1 コマンドずつ実行する
- 複数行のコミットメッセージは `git commit -F <ファイル>` を使う。一時ファイルは
  `/private/tmp/claude-501/-Users-nonaka-koki-dev-ghq-github-com-kukv-octoscope/0ee7bb1b-9217-409d-89f1-d907be9e8165/scratchpad/`
  に、`taskN-commit-msg.txt` のように自分専用の名前で置く

---

## Task 1: レビュー 3 メソッドのテスト

**実装は 1 行も変えない。** 既存の振る舞いを固定するだけ。

**Files:**
- Create: `internal/app/domain/review_context_test.go`
- Create: `internal/app/domain/review_thread_test.go`

対象の実装（変更しない。テストを書く前に読むこと）:

```go
// review_context.go
func (c ReviewContext) PendingCount() int {
	n := 0
	for _, t := range c.Threads {
		for _, comment := range t.Comments {
			if comment.Pending {
				n++
			}
		}
	}
	return n
}

// review_thread.go
func (t ReviewThread) Pending() bool {
	return slices.ContainsFunc(t.Comments, func(c ThreadComment) bool { return c.Pending })
}

func (t ReviewThread) Collapsed() bool { return t.Resolved || t.Outdated }
```

- [ ] **Step 1: `review_context_test.go` を書く**

```go
package domain_test

import (
	"testing"

	"github.com/kukv/octoscope/internal/app/domain"
)

// PendingCount walks two loops -- threads, then the comments inside each --
// so the case that matters is unsubmitted comments spread across more than
// one thread. A single thread would only exercise the inner loop.
func TestPendingCountAddsUpAcrossThreads(t *testing.T) {
	t.Parallel()

	c := domain.ReviewContext{Threads: []domain.ReviewThread{
		{Comments: []domain.ThreadComment{
			{Body: "someone else", Pending: false},
			{Body: "mine, not sent", Pending: true},
		}},
		{Comments: []domain.ThreadComment{
			{Body: "mine, not sent", Pending: true},
		}},
		{Comments: []domain.ThreadComment{
			{Body: "all public", Pending: false},
		}},
	}}

	if got := c.PendingCount(); got != 2 {
		t.Errorf("PendingCount() = %d, want 2", got)
	}
}

func TestPendingCountIsZeroWhenNothingIsUnsent(t *testing.T) {
	t.Parallel()

	c := domain.ReviewContext{Threads: []domain.ReviewThread{
		{Comments: []domain.ThreadComment{{Body: "public"}}},
		{Comments: []domain.ThreadComment{{Body: "public"}}},
	}}

	if got := c.PendingCount(); got != 0 {
		t.Errorf("PendingCount() = %d, want 0", got)
	}
}

// A review context arrives before its threads do, and a pull request with no
// conversation on it never gets any.
func TestPendingCountIsZeroWithNoThreads(t *testing.T) {
	t.Parallel()

	var c domain.ReviewContext
	if got := c.PendingCount(); got != 0 {
		t.Errorf("PendingCount() = %d, want 0", got)
	}
}
```

- [ ] **Step 2: `review_thread_test.go` を書く**

```go
package domain_test

import (
	"testing"

	"github.com/kukv/octoscope/internal/app/domain"
)

// GitHub returns an unsubmitted comment in the same place as everyone
// else's, so a thread that is public apart from one unsent reply is still
// pending.
func TestAThreadWithOneUnsentReplyIsPending(t *testing.T) {
	t.Parallel()

	thread := domain.ReviewThread{Comments: []domain.ThreadComment{
		{Body: "someone else", Pending: false},
		{Body: "mine, not sent", Pending: true},
	}}

	if !thread.Pending() {
		t.Error("a thread holding an unsent reply says it is not pending")
	}
}

func TestAThreadWithNothingUnsentIsNotPending(t *testing.T) {
	t.Parallel()

	thread := domain.ReviewThread{Comments: []domain.ThreadComment{
		{Body: "someone else"},
		{Body: "and a reply"},
	}}

	if thread.Pending() {
		t.Error("a thread with no unsent comment says it is pending")
	}

	var empty domain.ReviewThread
	if empty.Pending() {
		t.Error("a thread with no comments at all says it is pending")
	}
}

// A settled conversation is drawn as a count so that it does not push the
// code it was about off the screen. Either reason is enough on its own.
func TestEitherReasonCollapsesAThread(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		thread   domain.ReviewThread
		collapse bool
	}{
		{"still open", domain.ReviewThread{}, false},
		{"resolved", domain.ReviewThread{Resolved: true}, true},
		{"outdated", domain.ReviewThread{Outdated: true}, true},
		{"both", domain.ReviewThread{Resolved: true, Outdated: true}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.thread.Collapsed(); got != tt.collapse {
				t.Errorf("Collapsed() = %v, want %v", got, tt.collapse)
			}
		})
	}
}
```

- [ ] **Step 3: 通ることを確かめる**

```bash
go test ./internal/app/domain/ -run 'TestPendingCount|TestAThread|TestEitherReason' -v
```

期待: 6 本すべて PASS。

**落ちたら止めて報告すること。** このタスクは既存の振る舞いを写しているだけなので、落ちるのは
テストの書き方が間違っているか、実装に本当にバグがあるかのどちらか。**勝手に実装を直さない。**

- [ ] **Step 4: テストが空振りしていないことを確かめる**

`PendingCount()` の `if comment.Pending` を一時的に `if !comment.Pending` に書き換えて、

```bash
go test ./internal/app/domain/ -run TestPendingCount
```

期待: FAIL。**確かめたら必ず元に戻すこと**（`git diff` で戻ったことを確認）。

- [ ] **Step 5: 検査**

```bash
make check
```

期待: 終了コード 0。

- [ ] **Step 6: コミット**

```bash
git add internal/app/domain/review_context_test.go internal/app/domain/review_thread_test.go
```

メッセージ:

```
test(domain): pin what a pending review counts and what collapses a thread

PendingCount はスレッドをまたいで数えるので、複数スレッドのケースを置く。
1 スレッドだけでは内側のループしか守れない。

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
```

（`Co-Authored-By` は、自分のセッションの attribution 指示が別の文面を指定しているならそちらに従う。）

---

## Task 2: `ItemRef.IsIssue()`

新規メソッド。TDD で書く。

**Files:**
- Modify: `internal/app/domain/item_ref.go`
- Modify: `internal/app/domain/item_ref_test.go`

- [ ] **Step 1: 落ちるテストを書く**

`internal/app/domain/item_ref_test.go` に足す:

```go
func TestIsIssueAnswersFromTheKindAlone(t *testing.T) {
	t.Parallel()

	issue := domain.ItemRef{Kind: domain.ItemIssue, Repo: "kukv/octoscope", Number: 1}
	if !issue.IsIssue() {
		t.Error("a ref whose kind is ItemIssue does not say it is an issue")
	}

	pr := domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/octoscope", Number: 1}
	if pr.IsIssue() {
		t.Error("a ref whose kind is ItemPR says it is an issue")
	}
}
```

- [ ] **Step 2: 落ちることを確かめる**

```bash
go test ./internal/app/domain/ -run TestIsIssueAnswersFromTheKindAlone
```

期待: FAIL。`issue.IsIssue undefined (type domain.ItemRef has no field or method IsIssue)`

**目で確かめてから次へ。**

- [ ] **Step 3: 実装する**

`internal/app/domain/item_ref.go` の `IsPR()` の下に足す:

```go
// IsIssue reports whether this reference names an issue. It is not merely
// the negation of IsPR at the call site: the two places that ask are written
// around what an issue does not have, and reading them as "if not a pull
// request" would put them at odds with the comments above them.
func (r ItemRef) IsIssue() bool { return r.Kind == ItemIssue }
```

- [ ] **Step 4: 通ることを確かめる**

```bash
go test ./internal/app/domain/ -run TestIsIssueAnswersFromTheKindAlone
```

期待: PASS

- [ ] **Step 5: 検査**

```bash
make check
```

期待: 終了コード 0。

- [ ] **Step 6: コミット**

```bash
git add internal/app/domain/item_ref.go internal/app/domain/item_ref_test.go
```

メッセージ:

```
feat(domain): let a reference say whether it names an issue

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
```

---

## Task 3: `Kind ==` 14 箇所を置き換える

**振る舞いは変えない。** 比較を型のメソッドに寄せるだけ。

**Files:**
- Modify: `internal/app/usecase/usecase.go`（6 箇所）
- Modify: `internal/app/presentation/tui/search/render.go`（1 箇所）
- Modify: `internal/app/presentation/tui/detail/render.go`（3 箇所）
- Modify: `internal/app/presentation/tui/detail/detail.go`（1 箇所）
- Modify: `internal/app/presentation/tui/detail/update.go`（1 箇所）
- Modify: `internal/app/presentation/tui/work/render.go`（1 箇所）
- Modify: `internal/app/presentation/tui/work/drawer.go`（1 箇所）

- [ ] **Step 1: 現在地を確かめる**

```bash
grep -rn 'Kind == domain.Item' --include='*.go' internal | grep -v _test
```

期待: 14 行。行番号は下の表と違っているかもしれないので、**grep の結果に従う**。

- [ ] **Step 2: `ItemRef.Kind == domain.ItemPR` の 11 箇所を差し替える**

| ファイル:行 | 今 | 差し替え後 |
|---|---|---|
| `usecase/usecase.go:190` | `if ref.Kind == domain.ItemPR {` | `if ref.IsPR() {` |
| `usecase/usecase.go:214` | `if ref.Kind == domain.ItemPR {` | `if ref.IsPR() {` |
| `usecase/usecase.go:223` | `case ref.Kind == domain.ItemPR && closing:` | `case ref.IsPR() && closing:` |
| `usecase/usecase.go:225` | `case ref.Kind == domain.ItemPR:` | `case ref.IsPR():` |
| `usecase/usecase.go:235` | `if ref.Kind == domain.ItemPR {` | `if ref.IsPR() {` |
| `usecase/usecase.go:242` | `if ref.Kind == domain.ItemPR {` | `if ref.IsPR() {` |
| `search/render.go:363` | `if item.Ref.Kind == domain.ItemPR {` | `if item.Ref.IsPR() {` |
| `detail/render.go:149` | `if m.ref.Kind == domain.ItemPR {` | `if m.ref.IsPR() {` |
| `detail/render.go:228` | `case m.ref.Kind == domain.ItemPR && closing:` | `case m.ref.IsPR() && closing:` |
| `detail/render.go:230` | `case m.ref.Kind == domain.ItemPR:` | `case m.ref.IsPR():` |
| `detail/detail.go:302` | `return m.ref.Kind == domain.ItemPR && ok && closing` | `return m.ref.IsPR() && ok && closing` |

**否定・`&&`・`case` の形をそのまま保つこと。**

- [ ] **Step 3: `detail/update.go:108` を差し替える**

ここだけ扱いが違う。今は読み込んだ `usecase.Item` の `Kind` を見ている。

```go
	if it.Kind == domain.ItemPR {
		m.title = i18n.Tf("detail.pr_title", map[string]any{"Number": it.Number, "Title": it.Title})
	} else {
		m.title = i18n.Tf("detail.issue_title", map[string]any{"Number": it.Number, "Title": it.Title})
	}
```

`it` は `fetch(m.src, m.ref)` で取りに行った当のアイテムなので、`m.ref` と同じものを指す。
detail の他の 4 箇所はすでに `m.ref` を見ているので、ここも揃える。

```go
	if m.ref.IsPR() {
		m.title = i18n.Tf("detail.pr_title", map[string]any{"Number": it.Number, "Title": it.Title})
	} else {
		m.title = i18n.Tf("detail.issue_title", map[string]any{"Number": it.Number, "Title": it.Title})
	}
```

**`it.Number` と `it.Title` はそのまま。** 変えるのは条件だけ。

**この差し替えのあと、`usecase.Item.Kind` を読む非テストコードがゼロになる。
それでもフィールドは消さないこと。** `usecase.Item` は PR と Issue の合流点で、
`Kind` はその合流点が「これはどちらだったか」を運ぶ契約である（設計 §5 ②）。
読み手が今いないのは、たまたま detail が `m.ref` からも同じことを知れるからにすぎない。

- [ ] **Step 4: `ItemIssue` との比較 2 箇所を差し替える**

`work/render.go:376`:

```go
func stateMarker(it domain.WorkItem) string {
	if it.Ref.IsIssue() {
		return theme.Issue().Render(icon.Issue())
	}
```

`work/drawer.go:119`:

```go
	// Issues have no checks at all, so they get no pane.
	if it.Ref.IsIssue() {
		return nil
	}
```

**直上のコメントは変えない。**

- [ ] **Step 5: 取り残しが無いことを確かめる**

```bash
grep -rn 'Kind == domain.Item' --include='*.go' internal | grep -v _test
```

期待: 出力が空。

使われなくなった import があれば落とす（`domain` を他でも使っているファイルが多いので、
たいていは残る。`go build` が教えてくれる）。

- [ ] **Step 6: 検査**

```bash
make check
```

期待: 終了コード 0。**detail と work の golden テストも通ること。**
落ちたら振る舞いを変えてしまっている。

```bash
git status --short
```

期待: `.golden` が 1 つも出ないこと。

- [ ] **Step 7: コミット**

```bash
git add internal/app/usecase/
```
```bash
git add internal/app/presentation/tui/
```

メッセージ:

```
refactor: ask the reference instead of comparing its kind

detail/update.go は読み込んだ Item ではなく m.ref を見るようにした。
同じアイテムを指しており、detail の他の 4 箇所はすでにそうしている。

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
```

---

## Task 4: `WorkItem.Author` を消す

読み手が 1 箇所も無いフィールドを落とす。

**Files:**
- Modify: `internal/app/domain/work_item.go`
- Modify: `internal/app/adapter/gateway/gh/work.go`
- Modify: `internal/app/adapter/gateway/gh/work_test.go`

- [ ] **Step 1: 読み手が本当にいないことを確かめる**

```bash
grep -rn 'Author' --include='*.go' internal | grep -i workitem
```

期待: 出力が空（`WorkItem` と `Author` が同じ行に出ない）。

```bash
grep -rn '\.Author' --include='*.go' internal/app/presentation/tui/work internal/app/presentation/tui/search
```

期待: 出力が空。

**どちらかに出力があれば止めて報告すること。** 読み手がいるなら消してはいけない。

- [ ] **Step 2: フィールドを消す**

`internal/app/domain/work_item.go` の

```go
	Title   string
	Body    string
	Author  string
	IsDraft bool
```

から `Author  string` の行を消す。

- [ ] **Step 3: 代入を消す**

`internal/app/adapter/gateway/gh/work.go` の

```go
		Title:     n.Title,
		State:     parseItemState(n.State),
		Body:      n.BodyText,
		Author:    n.Author.Login,
		Labels:    toLabels(n.Labels.Nodes),
```

から `Author:    n.Author.Login,` の行を消す。

- [ ] **Step 4: テストの期待値を消す**

`internal/app/adapter/gateway/gh/work_test.go` の 2 箇所（`Author:  "kukv",` と
`Author: "kukv",`）を消す。行番号は 164 と 217 のあたりだが、`grep` で探すこと:

```bash
grep -n 'Author' internal/app/adapter/gateway/gh/work_test.go
```

**`gql.Author{Login: "kukv"}` の行は消さない。** それは GitHub から来る側の入力で、
`domain.WorkItem` の期待値ではない。

- [ ] **Step 5: ビルドが通ることを確かめる**

```bash
go build ./...
```

期待: 終了コード 0。取り残しがあればここで止まる。

- [ ] **Step 6: 検査**

```bash
make check
```

期待: 終了コード 0。golden テストも通ること（誰も描いていなかったので変わらない）。

- [ ] **Step 7: コミット**

```bash
git add internal/app/domain/work_item.go
```
```bash
git add internal/app/adapter/gateway/gh/
```

メッセージ:

```
refactor(domain): drop the board item's author, which nothing reads

Work board も Search もカードに作成者を描いていない。型が PR.Author と
食い違っていたのは事実だが、揃えても読み手がいない。

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
```

---

## Task 5: 仕上げの検査

**Files:** なし（確認のみ）

- [ ] **Step 1: 4 つの機械的な確認**

```bash
make check
```

期待: 終了コード 0。

```bash
grep -rn 'Kind == domain.Item' --include='*.go' internal | grep -v _test
```

期待: 出力が空。

```bash
grep -rn 'Author' --include='*.go' internal | grep -i workitem
```

期待: 出力が空。

```bash
git diff --stat 5c69d47..HEAD -- '*.golden'
```

期待: 出力が空（golden ファイルの変更が 0 件）。

- [ ] **Step 2: 波及範囲を確かめる**

```bash
git diff --stat 5c69d47..HEAD -- . ':(exclude)docs'
```

期待: `internal/app/domain` / `internal/app/adapter/gateway/gh` /
`internal/app/usecase` / `internal/app/presentation/tui/detail` /
`internal/app/presentation/tui/work` / `internal/app/presentation/tui/search` だけ。
他が出たら計画外の変更。

- [ ] **Step 3: 実際に起動して見る**

```bash
go run ./cmd/octoscope
```

見るもの:

- **Work board** — 4 列が出るか。カードの左に PR / Issue のアイコンが正しく出るか
  （Task 3 の `IsIssue()` がここを決めている）。PR のカードにチェックバーが出て、
  Issue のカードには出ないか（Task 3 の `drawer.go`）
- **詳細画面** — PR を開いてタイトルが `#123 タイトル` の PR 用の形で出るか。
  Issue を開いて Issue 用の形で出るか（Task 3 の `update.go`）

```bash
go run ./cmd/octoscope --lang ja
```

同じ確認を日本語でする。全角で桁を 2 つ使うので、桁ずれが無いか見る。

**テストが通っただけでは完了としない**（`CLAUDE.md`）。Task 3 は詳細画面のタイトルと
Work board のアイコンを決めている箇所を触っている。

---

## 完了条件

- [ ] `make check` が通る
- [ ] `Kind == domain.Item` の非テスト参照が 0 件
- [ ] `WorkItem` と `Author` が同じ行に出る箇所が 0 件
- [ ] `.golden` の変更が 0 件
- [ ] 波及が 6 パッケージに収まっている
- [ ] `go run ./cmd/octoscope` と `--lang ja` で、Work board のアイコンと
      詳細画面のタイトルが従来どおり
