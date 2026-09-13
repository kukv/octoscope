# 再構成 PR 3: ハンドルと port の形 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** port から GitHub の識別子の形（GraphQL node id の綴り、Actions の `int64`）と
「ブラウザを開く」通信を追い出し、pending review を作る順序を gateway に移す。

**Architecture:** domain に不透明ハンドルの名前付き文字列型（`PullRequestHandle`
`ReviewHandle` `JobHandle` `RunHandle`）を置き、`int64` の Actions ID は gateway で
文字列に畳む。`opener` port は消し、tui の各ビューが `internal/browser` を
モデルのシーム越しに直接呼ぶ。レビューの `StartReview` / `SubmitNewReview` は
usecase の port から消え、gateway が pending の有無で呼び分ける。

**Tech Stack:** Go 1.25 / Bubble Tea v2 / golangci-lint (depguard) / golden テスト

**Spec:** `docs/superpowers/specs/2026-09-13-architecture-restructure-design.md`
（§5 名前は借りてよい、形は借りない / §6 レビュー port / §7 の表 / §10 PR 3）

**Branch:** `refactor/pr3-handles`（`origin/main` から作成済み）

## Global Constraints

- 各タスクの末尾で `make check` が緑。緑でない状態でコミットしない
- golden テストの差分はゼロ。画面の見た目は一切変えない
- `internal/app/domain` は stdlib のみ import（depguard と `tags_test.go` が守っている）
- `internal/github/**` は `internal/app/**` を import しない
- 新しい domain のエクスポート struct を足したら `internal/app/domain/tags_test.go` の
  `exported` リストに加える（名前付き文字列型は struct ではないので対象外）
- コメントは外部の事情・正しい理由・doc の 3 つだけ（`.claude/rules/go-style.md`）
- 大量のリネームは `gopls rename` を使う。手で `sed` しない（コメント内の語まで壊す）

## 設計からの差分（実測 2026-09-14）

計画時に数え直して、設計書 §7 / §10 の PR 3 リストから 1 項目を落とす。

- **`SplitRepo` を domain から外す** — **PR 2b で完了済み**。現在 `SplitRepo` /
  `SplitRepoVars` は `internal/github/gql/gql.go:86,101` にあり、`internal/app/domain`
  には無い（`grep -rn SplitRepo internal/app` が 0 件）。§7 / §10 の該当行を
  「PR 2b で完了」に直す（タスク 5）。
- `OpenWeb` の削除範囲は usecase の port だけではない。実測 21 箇所:
  `internal/github/cli/cli.go:146`、`internal/github/api/api.go:110`、
  `gateway/gh/backend.go:82,118`、`usecase/usecase.go:95,123,142,286`、
  tui 4 ビュー（detail / checks / repo / search）の `Source` interface と
  そのフェイク 5 個。`internal/browser` を `internal/github` から切るのが §2.5 の理由なので、
  cli / api の `OpenWeb` ごと消す。
- 参照数: `PullRequestID` 72 / `PendingID` 60 / `JobID` 33 / `RunID` 33 /
  `ReviewTarget` 23 / `StartReview` 17 / `SubmitNewReview` 20。

## ハンドルの名前（この計画で決めたこと）

設計 §5 は「識別子は不透明な `string` ハンドル」としか言っていない。この計画では
**名前付き文字列型**を採る。素の `string` へのリネームだと、job のハンドルを run の
引数に渡しても**コンパイルが通り、テストも緑のまま**になる（`mutation-finds-dead-wiring`）。

| 型 | 対象のフィールド / 引数 | 今 |
|---|---|---|
| `domain.PullRequestHandle` | `ReviewContext.PullRequest`、`MergeContext.PullRequest`、`ReviewTarget.PullRequest` | `PullRequestID string` |
| `domain.ReviewHandle` | `ReviewContext.Pending`、`ReviewTarget.Pending` | `PendingID string` |
| `domain.JobHandle` | `CheckRun.Job`、`JobLog` の引数 | `JobID int64` |
| `domain.RunHandle` | `CheckRun.WorkflowRun`、`RerunWorkflow` の引数 | `RunID int64` |

`CheckRun.Run` ではなく `CheckRun.WorkflowRun` にするのは、既存の `Workflow` /
`RunNumber` と並んだときに何の run か読めるのと、`run.Run` という参照を作らないため。

ゼロ値の意味は変えない。`JobID == 0`（StatusContext は job を持たない）は `Job == ""` になる。

## ファイル構成

| ファイル | 役割 | 変更 |
|---|---|---|
| `internal/app/domain/handle.go` | 4 つのハンドル型の宣言と doc | 新規 |
| `internal/app/domain/review.go` | `ReviewContext` のフィールド、`ReviewTarget` の受け入れ | 変更 |
| `internal/app/domain/merge.go` | `MergeContext.PullRequest` | 変更 |
| `internal/app/domain/domain.go` | `CheckRun.Job` / `.WorkflowRun` | 変更 |
| `internal/app/usecase/usecase.go` | `opener` port 削除、`checksFetcher` / `merger` / `reviewer` の署名 | 変更 |
| `internal/app/usecase/review.go` | `ReviewTarget` 削除、`PostLineComment` / `SubmitReview` の畳み込み | 変更 |
| `internal/app/adapter/gateway/gh/checks.go` | `int64` ↔ ハンドルの変換 | 変更 |
| `internal/app/adapter/gateway/gh/review.go` | pending の作成順序 | 変更 |
| `internal/app/adapter/gateway/gh/backend.go` | `opener` 削除 | 変更 |
| `internal/github/{cli,api}` | `OpenWeb` と `internal/browser` の import 削除 | 変更 |
| `internal/app/presentation/tui/{detail,checks,repo,search}` | `Source` から `OpenWeb` を外し、`open` シームを持つ | 変更 |

---

### Task 1: `opener` port を消し、tui が browser を直接呼ぶ

**Files:**
- Modify: `internal/app/presentation/tui/detail/detail.go`（`Source` から `OpenWeb`、`openWeb` の中身）
- Modify: `internal/app/presentation/tui/checks/checks.go:22,252`
- Modify: `internal/app/presentation/tui/repo/repo.go:32,379`
- Modify: `internal/app/presentation/tui/search/search.go:25,205`
- Modify: `internal/app/usecase/usecase.go:95,123,142,286`
- Modify: `internal/app/adapter/gateway/gh/backend.go:82,118`
- Modify: `internal/github/cli/cli.go:141-150`、`internal/github/api/api.go:108-115`
- Test: 各ビューの `*_test.go` のフェイク（`detail_test.go:80`、`repo_test.go:79`、
  `checks_test.go:34`、`search_test.go:32`、`root/root_test.go:129`、
  `root/scenario_test.go:96`）

**Interfaces:**
- Produces: 各ビューの `Model` が非公開フィールド `open func(url string) error` を持ち、
  `New` の中で `browser.Open` が入る。`New` の署名は変えない。テストは同じパッケージから
  `m.open = func(url string) error { ... }` で差し替える。
- Consumes: なし。

**シームがいる理由:** `browser.Open` にはシームが無く、WSL では `powershell.exe` が
実在するので、テストで `o` を押すと**本物のブラウザが開く**。現在
`detail_test.go:80` と `repo_test.go:79` は URL を捕まえて検証しているので、
その検証を保つ必要もある。`root/scenario_test.go` は `o` を押していない（実測: 押すのは
`a` だけ）ので、root 側のシームは要らない。

- [ ] **Step 1: 失敗するテストを書く**

`internal/app/presentation/tui/detail/detail_test.go` の既存の URL 検証を、
`Source` 経由からモデルのシーム経由に書き換える。

```go
func TestOPressOpensTheItemURL(t *testing.T) {
	t.Parallel()
	m := newTestModel(t) // 既存のヘルパーに合わせる
	var got string
	m.open = func(url string) error { got = url; return nil }
	m, cmd := m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	if cmd != nil {
		cmd()
	}
	if got == "" {
		t.Fatal("o did not open any URL")
	}
}
```

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/app/presentation/tui/detail/ -run TestOPress -v`
Expected: FAIL（`m.open` undefined）

- [ ] **Step 3: 4 ビューにシームを足し、`OpenWeb` を `Source` から外す**

```go
// detail.go
import "github.com/kukv/octoscope/internal/browser"

type Model struct {
	// ...
	// open shows a URL. It is browser.Open outside tests: opening a page is
	// not a GitHub call, so it does not go through the backend.
	open func(url string) error
}

func New(/* 既存の引数 */) Model {
	m := Model{/* ... */}
	m.open = browser.Open
	return m
}

func (m Model) openWeb(ref domain.ItemRef, url string) tea.Cmd {
	open := m.open
	return func() tea.Msg {
		if err := open(url); err != nil {
			return errMsg{ref, err}
		}
		return nil
	}
}
```

`Source` interface からは `OpenWeb(url string) error` の行を消す。checks / repo /
search も同じ形にする（`openWeb` 相当の関数が `src` を取っているなら、メソッドにして
`m.open` を読む）。

- [ ] **Step 4: usecase / gateway / github から `OpenWeb` を消す**

- `usecase/usecase.go`: `opener` interface の宣言、`source` の `opener` 行、
  `Usecase.web` フィールド、`New` の `web: src`、`func (u *Usecase) OpenWeb` を削除
- `gateway/gh/backend.go`: `opener` interface と `backend` の `opener` 行を削除
- `internal/github/cli/cli.go`: `OpenWeb` メソッドと `internal/browser` の import を削除
- `internal/github/api/api.go`: 同上

- [ ] **Step 5: テストのフェイクから `OpenWeb` を消す**

上に挙げた 6 ファイルの `func (f *fakeSource) OpenWeb(...)` を削除する。
`detail_test.go` / `repo_test.go` の URL 検証は Step 3 のシームに移してある。

- [ ] **Step 6: 全部通ることを確かめる**

Run: `make check`
Expected: PASS。`grep -rn OpenWeb internal cmd` が 0 件。

- [ ] **Step 7: コミット**

```bash
git add -A
git commit -m "refactor: open URLs from the views, not through the GitHub backend

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: Actions の `int64` を `JobHandle` / `RunHandle` にする

**Files:**
- Create: `internal/app/domain/handle.go`
- Modify: `internal/app/domain/domain.go:140-143`
- Modify: `internal/app/usecase/usecase.go`（`checksFetcher`）と `usecase` の checks 呼び出し
- Modify: `internal/app/adapter/gateway/gh/checks.go:22,35,120,124`
- Modify: `internal/app/presentation/tui/checks/{checks.go,log.go,rerun.go,render.go}`
- Test: `internal/app/adapter/gateway/gh/checks_test.go`、
  `internal/app/presentation/tui/checks/*_test.go`

**Interfaces:**
- Produces:
  ```go
  type JobHandle string
  type RunHandle string
  ```
  port は `JobLog(ctx context.Context, repo string, job domain.JobHandle, failedOnly bool) ([]domain.LogLine, error)` と
  `RerunWorkflow(ctx context.Context, repo string, run domain.RunHandle, scope domain.RerunScope) error`。
  `CheckRun.Job JobHandle` / `CheckRun.WorkflowRun RunHandle`。空文字は「持たない」。
- Consumes: Task 1 の結果（衝突しない）。

- [ ] **Step 1: gateway の変換テストを書く（失敗する）**

`internal/app/adapter/gateway/gh/checks_test.go` に足す。

```go
func TestJobLogRejectsAHandleThatIsNotAnActionsID(t *testing.T) {
	t.Parallel()
	g := &Gateway{backend: &fakeBackend{}}
	if _, err := g.JobLog(context.Background(), "o/r", domain.JobHandle("nope"), false); err == nil {
		t.Fatal("JobLog accepted a non-numeric handle")
	}
}

func TestPRChecksCarriesTheJobAndRunHandlesApart(t *testing.T) {
	t.Parallel()
	// fakeBackend returns one gql.CheckRun with DatabaseID 11 and
	// WorkflowRun.DatabaseID 22.
	g := &Gateway{backend: &fakeBackend{}}
	c, err := g.PRChecks(context.Background(), "o/r", 1)
	if err != nil {
		t.Fatal(err)
	}
	if c.Runs[0].Job != domain.JobHandle("11") {
		t.Errorf("Job = %q, want %q", c.Runs[0].Job, "11")
	}
	if c.Runs[0].WorkflowRun != domain.RunHandle("22") {
		t.Errorf("WorkflowRun = %q, want %q", c.Runs[0].WorkflowRun, "22")
	}
}
```

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/app/adapter/gateway/gh/ -run 'TestJobLog|TestPRChecksCarries' -v`
Expected: FAIL（`domain.JobHandle` undefined）

- [ ] **Step 3: ハンドル型を足す**

`internal/app/domain/handle.go`:

```go
package domain

// A handle addresses something on the service the application is talking to.
// The application never reads one: it passes back what it was given. The
// service's own shape for an id -- a node id, a database id -- stops at the
// gateway.
type (
	// PullRequestHandle addresses one pull request.
	PullRequestHandle string
	// ReviewHandle addresses one review, submitted or not.
	ReviewHandle string
	// JobHandle addresses one job's log. Empty for a check that is not a
	// job, which has no log to read.
	JobHandle string
	// RunHandle addresses one workflow run. Empty for a check that belongs
	// to no run, which is nothing to rerun.
	RunHandle string
)
```

`domain.go` の `CheckRun`:

```go
	// Job addresses the job's log, WorkflowRun the run to rerun. Both are
	// empty for a StatusContext, which has neither.
	Job         JobHandle
	WorkflowRun RunHandle
```

- [ ] **Step 4: gateway で `int64` を畳む**

```go
// checks.go
run.Job = domain.JobHandle(strconv.FormatInt(n.DatabaseID, 10))
// ...
run.WorkflowRun = domain.RunHandle(strconv.FormatInt(wr.DatabaseID, 10))

// JobLog reads one job's log. The handle is the Actions database id the
// gateway handed out; anything else is a caller bug, not a service failure.
func (g *Gateway) JobLog(ctx context.Context, repo string, job domain.JobHandle, failedOnly bool) ([]domain.LogLine, error) {
	id, err := strconv.ParseInt(string(job), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("job handle %q: %w", job, err)
	}
	lines, err := g.backend.JobLog(ctx, repo, id, failedOnly)
	// ... 既存の変換のまま
}
```

`RerunWorkflow` も同じ形にする。`DatabaseID == 0`（StatusContext）はハンドルを
空のままにする — `FormatInt` を通して `"0"` にしない。

```go
if n.DatabaseID != 0 {
	run.Job = domain.JobHandle(strconv.FormatInt(n.DatabaseID, 10))
}
```

- [ ] **Step 5: usecase の port と tui を追う**

- `usecase.go` の `checksFetcher` を新しい署名に
- `tui/checks/checks.go:76` `rerunRunID int64` → `rerunRun domain.RunHandle`、
  `selectedJobID() int64` → `selectedJob() domain.JobHandle`（`0` の比較は `""` へ）
- `checks.go:328` `r.RunID != 0` → `r.WorkflowRun != ""`
- `log.go:140,143`、`render.go:329`、`rerun.go:89,122,135,145` の `int64` を追う
- メッセージ型の `jobID int64` / `runID int64` フィールドも同じ型に直す

- [ ] **Step 6: 全部通ることを確かめる**

Run: `make check`
Expected: PASS。`grep -rn 'JobID\|RunID' internal/app` が `internal/app` で 0 件
（`internal/github/api/checks.go:33` の `RunID int64` は GitHub の層なので残る）。

- [ ] **Step 7: コミット**

```bash
git add -A
git commit -m "refactor: hand the views job and run handles, not Actions database ids

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: `PullRequestID` / `PendingID` をハンドルにし、`ReviewTarget` を domain へ

**Files:**
- Modify: `internal/app/domain/review.go:69,75-78`、`internal/app/domain/merge.go:53`
- Modify: `internal/app/usecase/review.go`（`ReviewTarget` の宣言を削除）
- Modify: `internal/app/usecase/usecase.go`（`reviewer` / `merger` の署名）
- Modify: `internal/app/adapter/gateway/gh/{review.go,merge.go,backend.go}`
- Modify: `internal/app/presentation/tui/{diff,review,detail,root}` の `ReviewTarget` 参照
- Test: 上記の `*_test.go`（`ReviewTarget` 23 参照 / 11 ファイル）

**Interfaces:**
- Produces:
  ```go
  // domain/review.go
  type ReviewTarget struct {
      PullRequest PullRequestHandle
      Pending     ReviewHandle
  }
  ```
  `ReviewContext.PullRequest PullRequestHandle` / `ReviewContext.Pending ReviewHandle`、
  `MergeContext.PullRequest PullRequestHandle`。
  port: `MergePR(pr domain.PullRequestHandle, method domain.MergeMethod) error` ほか
  `EnableAutoMerge` / `DisableAutoMerge` / `DiscardReview(review domain.ReviewHandle) error`。
- Consumes: Task 2 の `domain.PullRequestHandle` / `ReviewHandle`（Task 2 の Step 3 で宣言済み）。

**`ReviewTarget` が domain に移る理由:** Task 4 で port が両方のハンドルを受け取る。
gateway は usecase を import できない（設計 §3.2）ので、両者が見える場所は domain しかない。

**壊してはいけないもの:** `tui/diff/review.go:57-70` の 3 分岐は
「まだ分からない」と「pending が無い」を `PullRequest == ""` → `Pending == ""` の
順で見分けている。フィールド名が変わるだけで、**順序と分岐は変えない**。
`review.Model.Active()` も同じ前提に乗っている。

- [ ] **Step 1: 失敗するテストを書く**

`internal/app/domain/review_test.go`（無ければ新規）に、ハンドルが取り違えられない
ことを型で示すテストを置く。

```go
func TestReviewTargetKeepsTheTwoHandlesApart(t *testing.T) {
	t.Parallel()
	tgt := domain.ReviewTarget{
		PullRequest: domain.PullRequestHandle("PR_1"),
		Pending:     domain.ReviewHandle("PRR_1"),
	}
	if string(tgt.PullRequest) == string(tgt.Pending) {
		t.Fatal("the two handles are the same value")
	}
}
```

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/app/domain/ -run TestReviewTargetKeeps -v`
Expected: FAIL（`domain.ReviewTarget` undefined）

- [ ] **Step 3: domain にフィールドを足す / 直す**

```go
// review.go
type ReviewTarget struct {
	PullRequest PullRequestHandle
	Pending     ReviewHandle
}

type ReviewContext struct {
	PullRequest PullRequestHandle
	Title       string
	Head        string
	Base        string
	Additions   int
	Deletions   int
	// Pending addresses the unsubmitted review, empty when there is none. A
	// pending review is visible only to its author, so anything that comes
	// back here belongs to the viewer.
	Pending ReviewHandle
	Threads []ReviewThread
}
```

`merge.go:53` は `PullRequest PullRequestHandle`。
`tags_test.go` の `exported` は struct の一覧なので、`ReviewTarget` を足す。

- [ ] **Step 4: `usecase.ReviewTarget` を消し、参照を追う**

`gopls rename` で `usecase.ReviewTarget` → `domain.ReviewTarget` を機械的に置き換える
（`gopls` が使えなければ、11 ファイルを 1 つずつ直して都度ビルドする）。
`usecase/review.go` の宣言とその doc コメントを削除する。

port の署名（`usecase.go`）:

```go
type reviewer interface {
	StartReview(pr domain.PullRequestHandle) (domain.ReviewHandle, error)
	AddReviewThread(review domain.ReviewHandle, c domain.PendingComment) error
	SubmitReview(review domain.ReviewHandle, event domain.ReviewEvent, body string) error
	SubmitNewReview(pr domain.PullRequestHandle, event domain.ReviewEvent, body string) error
	DiscardReview(review domain.ReviewHandle) error
}

type merger interface {
	PRMergeContext(ctx context.Context, repo string, number int) (domain.MergeContext, error)
	MergePR(pr domain.PullRequestHandle, method domain.MergeMethod) error
	EnableAutoMerge(pr domain.PullRequestHandle, method domain.MergeMethod) error
	DisableAutoMerge(pr domain.PullRequestHandle) error
}
```

gateway の対応するメソッドは `string(pr)` / `string(review)` で backend に渡す。
`backend` interface（`internal/github` の型を話す側）は `string` のまま変えない。

- [ ] **Step 5: doc コメントから GitHub の綴りを抜く**

`domain/review.go:75` の "the unsubmitted review's node id" と
`domain/merge.go` の同種の記述から "node id" を落とす（ハンドル型の doc に
「サービスの id の形は gateway で止まる」と書いてある）。

- [ ] **Step 6: 全部通ることを確かめる**

Run: `make check`
Expected: PASS。`grep -rn 'PullRequestID\|PendingID' internal/app` が 0 件
（`internal/github/gql/{review.go,merge.go}` は GitHub の層なので残る）。

- [ ] **Step 7: コミット**

```bash
git add -A
git commit -m "refactor: name the review handles for what they address, not how GitHub spells them

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: pending review を作る順序を gateway に移す

**Files:**
- Modify: `internal/app/usecase/review.go`（`PostLineComment` / `SubmitReview`）
- Modify: `internal/app/usecase/usecase.go`（`reviewer` から `StartReview` /
  `SubmitNewReview` を削除）
- Modify: `internal/app/adapter/gateway/gh/review.go`
- Modify: `.claude/rules/architecture.md:106` の節
- Test: `internal/app/usecase/review_test.go`、
  `internal/app/adapter/gateway/gh/review_test.go`

**Interfaces:**
- Produces: port は 3 メソッドになる。
  ```go
  type reviewer interface {
      AddReviewThread(t domain.ReviewTarget, c domain.PendingComment) (domain.ReviewHandle, error)
      SubmitReview(t domain.ReviewTarget, event domain.ReviewEvent, body string) error
      DiscardReview(review domain.ReviewHandle) error
  }
  ```
  `AddReviewThread` は使った pending のハンドル（既存または新規）を返す。
  `tui/diff` はこれで `m.review.Pending` を更新する（今 `PostLineComment` の
  戻り値でやっていることと同じ）。
- Consumes: Task 3 の `domain.ReviewTarget`。

**回帰させてはいけない振る舞い（今の `usecase/review.go:37-47` の理由）:**
pending が無いときの提出は `StartReview` → `SubmitReview` の 2 回ではなく、
**`SubmitNewReview` の 1 回**でなければならない。分けると、提出が失敗したときに
空の pending review が残る。gateway でも同じ分岐を保つ。ここを素直に「畳む」と
**テストは全部緑のまま回帰する**ので、gateway のテストは
「どのメソッドが呼ばれたか」を記録するフェイクで見る（`tests-that-cannot-fail`）。

- [ ] **Step 1: 失敗するテストを書く**

`internal/app/adapter/gateway/gh/review_test.go`:

```go
// recordingBackend records which review calls were made, in order. Asserting
// "no error" here would pass for either arrangement, and the two differ only
// in what they leave behind on GitHub when the submit fails.
type recordingBackend struct {
	backend
	calls []string
}

func (b *recordingBackend) StartReview(string) (string, error) {
	b.calls = append(b.calls, "StartReview")
	return "PRR_new", nil
}

func (b *recordingBackend) SubmitReview(string, gql.ReviewEvent, string) error {
	b.calls = append(b.calls, "SubmitReview")
	return nil
}

func (b *recordingBackend) SubmitNewReview(string, gql.ReviewEvent, string) error {
	b.calls = append(b.calls, "SubmitNewReview")
	return nil
}

func (b *recordingBackend) AddReviewThread(string, gql.PendingComment) error {
	b.calls = append(b.calls, "AddReviewThread")
	return nil
}

func TestSubmitReviewWithNoPendingCreatesAndSubmitsInOneCall(t *testing.T) {
	t.Parallel()
	b := &recordingBackend{}
	g := &Gateway{backend: b}
	tgt := domain.ReviewTarget{PullRequest: "PR_1"}
	if err := g.SubmitReview(tgt, domain.EventComment, "body"); err != nil {
		t.Fatal(err)
	}
	if want := []string{"SubmitNewReview"}; !slices.Equal(b.calls, want) {
		t.Errorf("calls = %v, want %v", b.calls, want)
	}
}

func TestSubmitReviewWithAPendingSubmitsThatOne(t *testing.T) {
	t.Parallel()
	b := &recordingBackend{}
	g := &Gateway{backend: b}
	tgt := domain.ReviewTarget{PullRequest: "PR_1", Pending: "PRR_1"}
	if err := g.SubmitReview(tgt, domain.EventApprove, "body"); err != nil {
		t.Fatal(err)
	}
	if want := []string{"SubmitReview"}; !slices.Equal(b.calls, want) {
		t.Errorf("calls = %v, want %v", b.calls, want)
	}
}

func TestAddReviewThreadStartsAReviewOnlyWhenThereIsNone(t *testing.T) {
	t.Parallel()
	b := &recordingBackend{}
	g := &Gateway{backend: b}
	id, err := g.AddReviewThread(domain.ReviewTarget{PullRequest: "PR_1"}, domain.PendingComment{})
	if err != nil {
		t.Fatal(err)
	}
	if id != domain.ReviewHandle("PRR_new") {
		t.Errorf("handle = %q, want PRR_new", id)
	}
	if want := []string{"StartReview", "AddReviewThread"}; !slices.Equal(b.calls, want) {
		t.Errorf("calls = %v, want %v", b.calls, want)
	}

	b2 := &recordingBackend{}
	g2 := &Gateway{backend: b2}
	id, err = g2.AddReviewThread(domain.ReviewTarget{PullRequest: "PR_1", Pending: "PRR_1"}, domain.PendingComment{})
	if err != nil {
		t.Fatal(err)
	}
	if id != domain.ReviewHandle("PRR_1") {
		t.Errorf("handle = %q, want PRR_1", id)
	}
	if want := []string{"AddReviewThread"}; !slices.Equal(b2.calls, want) {
		t.Errorf("calls = %v, want %v", b2.calls, want)
	}
}
```

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/app/adapter/gateway/gh/ -run 'TestSubmitReview|TestAddReviewThread' -v`
Expected: FAIL（`SubmitReview` の引数が合わない）

- [ ] **Step 3: gateway に順序を持たせる**

```go
// AddReviewThread attaches one line comment to the target's unsubmitted
// review, starting one first if there is none: on GitHub a line comment has
// to hang off a review. It answers the review the comment went onto.
func (g *Gateway) AddReviewThread(t domain.ReviewTarget, c domain.PendingComment) (domain.ReviewHandle, error) {
	review := t.Pending
	if review == "" {
		id, err := g.backend.StartReview(string(t.PullRequest))
		if err != nil {
			return "", wrap(err)
		}
		review = domain.ReviewHandle(id)
	}
	if err := g.backend.AddReviewThread(string(review), fromPendingComment(c)); err != nil {
		return "", wrap(err)
	}
	return review, nil
}

// SubmitReview sends the review out. With nothing waiting it creates and
// submits in one call: starting a review first would leave an empty pending
// review behind if the submission then failed.
func (g *Gateway) SubmitReview(t domain.ReviewTarget, event domain.ReviewEvent, body string) error {
	if t.Pending == "" {
		return wrap(g.backend.SubmitNewReview(string(t.PullRequest), fromReviewEvent(event), body))
	}
	return wrap(g.backend.SubmitReview(string(t.Pending), fromReviewEvent(event), body))
}
```

`SubmitNewReview` の公開メソッドは Gateway から消す（`backend` には残る）。

- [ ] **Step 4: usecase を薄くする**

`usecase/review.go`:

```go
// PostLineComment attaches one line comment to the pull request's review and
// answers the review it went onto: which requests that takes is the
// gateway's knowledge, not the application's.
func (u *Usecase) PostLineComment(t domain.ReviewTarget, c domain.PendingComment) (domain.ReviewHandle, error) {
	id, err := u.reviews.AddReviewThread(t, c)
	if err != nil {
		return "", fmt.Errorf("add review thread: %w", err)
	}
	return id, nil
}

// SubmitReview sends the review out.
func (u *Usecase) SubmitReview(t domain.ReviewTarget, event domain.ReviewEvent, body string) error {
	if err := u.reviews.SubmitReview(t, event, body); err != nil {
		return fmt.Errorf("submit review: %w", err)
	}
	return nil
}
```

`usecase.go` の `reviewer` から `StartReview` と `SubmitNewReview` を削除する。
`review_test.go` の「pending があるとき / ないとき」を検証していたテストは、
usecase にその分岐が無くなったので **gateway のテスト（Step 1）に移した**。
usecase 側には残骸を残さず消す。

- [ ] **Step 5: 規約を直す**

`.claude/rules/architecture.md:106` の「複数の API 呼び出しは `internal/app/usecase` に置く」を、
設計 §6 のとおり部分改訂する。節の見出しは残し、本文を次の趣旨に書き換える:

- `tea.Cmd` のクロージャに 2 つ以上の API 呼び出しを並べない、は維持
- **サービス固有の順序**（pending review が無ければ先に作る）は
  `internal/app/adapter/gateway` に置く。例示をこの PR の形に差し替える
- **アプリの都合による順序**（種別で呼ぶものが変わる、`domain.ItemRef.Kind` を
  View で switch しない）は `internal/app/usecase` に残る

- [ ] **Step 6: 全部通ることを確かめる**

Run: `make check`
Expected: PASS。`grep -rn 'StartReview\|SubmitNewReview' internal/app/usecase internal/app/presentation` が 0 件。

- [ ] **Step 7: コミット**

```bash
git add -A
git commit -m "refactor: let the gateway decide how many requests a review takes

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: 設計書と規約を新しい形に合わせ、実機で確かめる

**Files:**
- Modify: `docs/superpowers/specs/2026-09-13-architecture-restructure-design.md`（§7 の表、§10 の PR 3）
- Modify: `.claude/rules/architecture.md`（§5 の規則がまだ無ければ追記）

- [ ] **Step 1: 設計書の PR 3 の記述を実測に合わせる**

- §7 の `SplitRepo` の行を「PR 2b で完了（`internal/github/gql`）」に直す
- §10 の PR 3 のリストから `SplitRepo を domain から外す` を消し、
  実際にやった 4 項目（ハンドル 4 つ、レビュー port の畳み込み、`opener` 削除、
  規約の改訂）に直す
- §13 の完了条件のうち満たしたものに触れる（削除ではなく、達成として記述）

- [ ] **Step 2: 規約に §5 の規則を書く**

`.claude/rules/architecture.md` に「名前は借りてよい、形は借りない」の節を置く
（設計 §5 の表をそのまま持ち込まず、この PR で実際に決まった形を書く）:

- 名詞と操作名は GitHub のものでよい（`PR` `Review` `Label` `AutoMerge`）
- 識別子は `internal/app/domain` の不透明なハンドル型。`int64` の Actions ID や
  GraphQL node id をその名前のまま port に出さない
- GitHub が余計に 1 回呼ぶ必要があるから存在する port メソッドを作らない

- [ ] **Step 3: 実機で確かめる**

```bash
go run ./cmd/octoscope --repo <確認用リポジトリ>
go run ./cmd/octoscope --repo <確認用リポジトリ> --lang ja
```

確かめること（設計 §10 の PR 3 の検証）:

1. Checks タブで job のログが開く（`JobHandle` 経路）
2. Checks タブで失敗した run の再実行が通る（`RunHandle` 経路）
3. diff でレビューを提出する — **pending がある場合と無い場合の両方**
4. `o` でブラウザが開く（4 ビュー全部: detail / checks / repo / search）
5. 画面の見た目が再構成前と同じ。`--lang ja` で桁が崩れていない

**実機確認の対象リポジトリと PR は、実行前にユーザーに確認する。** レビューの提出と
workflow の再実行は外向きの操作で、取り消せない。

- [ ] **Step 4: コミットして PR を出す**

```bash
git add -A
git commit -m "docs: record what PR 3 changed about handles and the review port

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
git push -u origin refactor/pr3-handles
```

PR 本文には、§13 の完了条件のうちこの PR で満たしたものと、実機で確かめた 5 項目を書く。

---

## Self-Review

- **設計のカバレッジ:** §10 の PR 3 の 4 項目 → タスク 3+4（ハンドルとレビュー port）、
  タスク 1（opener）、タスク 5（規約と設計書）。`SplitRepo` は実測で PR 2b 完了と確認し、
  タスク 5 で設計書を直す。§5 の規則 → タスク 5 Step 2。§6 → タスク 4。
  §12 の「複数の API 呼び出し」の改訂 → タスク 4 Step 5。
- **型の一貫性:** `domain.JobHandle` `RunHandle` `PullRequestHandle` `ReviewHandle`
  `ReviewTarget{PullRequest, Pending}`、`CheckRun{Job, WorkflowRun}`、
  `ReviewContext{PullRequest, Pending}` をタスク 2〜4 で同じ綴りで使っている。
  ハンドル 4 つの宣言はタスク 2 Step 3 にまとめてあり、タスク 3 はそれを使う。
- **順序:** 1（独立・最小）→ 2（checks だけ）→ 3（機械的な改名）→ 4（唯一の振る舞いの変更）。
  各タスクが単独で緑。
