# domain をモデル単位のファイルに割り直す — 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `internal/app/domain` の 7 ファイル 792 行を、モデル単位の 29 ファイルに割り直す。振る舞いは変えない。

**Architecture:** 宣言を移動するだけのタスクと、新しいコードを足すタスクを分ける。移動タスクは `make check` が通れば正しい（コンパイラが全部確かめる）。新規タスクだけ TDD で書く。まとまり（Item / Checks / Review / Diff / Merge / Work）ごとにコミットするので、途中で止めてもビルドは通る。

**Tech Stack:** Go 1.x、`make check`（tidy / lint / fmt / test）、`gofumpt`、`golangci-lint`

**設計:** `docs/superpowers/specs/2026-09-19-domain-per-model-files-design.md`

---

## 前提

作業前に必ず読む。

- 設計: `docs/superpowers/specs/2026-09-19-domain-per-model-files-design.md`
- 規約: `.claude/rules/architecture.md`（`internal/app/domain` を触ると自動で読み込まれる）

**移動タスクの原則:**

- 宣言は**一字一句そのまま**移す。コメントも含む。書き直さない
- ファイルの先頭は `package domain`。import はそのファイルが実際に使うものだけ
- 移動後に `make check` が通れば、移動は正しい

**各タスクの最後で必ず:**

```bash
make check
```

期待: 終了コード 0。1 つでも落ちたら次のタスクへ進まない。

---

## ファイル構成（最終形）

```
internal/app/domain/
  author.go           comment.go          label.go
  item_ref.go         issue.go            pull_request.go
  item_state.go       review_state.go
  checks.go           check_run.go        log_line.go        rerun_request.go
  review_context.go   review_thread.go    thread_comment.go
  pending_comment.go  review_target.go
  file_diff.go        hunk.go             diff_line.go       diff_parser.go
  merge_context.go
  work.go             work_item.go
  repo_count.go       repo_candidate.go   saved_query.go
  handle.go           error.go
```

消えるファイル: `domain.go` / `review.go` / `merge.go` / `diff.go` / `diff_parse.go`
残るファイル: `handle.go`（中身も変えない）

---

## Task 1: 失敗を `error.go` へ

`domain.go` から失敗まわりだけを抜く。最初のタスクとして一番小さいものを選んでいる。

**Files:**
- Create: `internal/app/domain/error.go`
- Create: `internal/app/domain/error_test.go`
- Modify: `internal/app/domain/domain.go`（該当宣言を削除）
- Modify: `internal/app/domain/domain_test.go`（該当テストを削除）

- [ ] **Step 1: `error.go` を作り、`domain.go` から 6 つの宣言を移す**

移す宣言（`domain.go` から、コメントごと一字一句そのまま）:

| 宣言 | 種類 |
|---|---|
| `ErrBackendUnavailable` | `var` |
| `ErrTransient` | `var` |
| `ErrUnauthenticated` | `var` |
| `classified` | `type`（非公開 struct） |
| `Classify` | `func` |
| `IsFatal` | `func` |

`classified` のメソッド `Error()` と `Unwrap()` も一緒に移す。

ファイルの先頭:

```go
package domain

import "errors"
```

- [ ] **Step 2: `domain.go` から同じ 6 つを削除する**

`domain.go` の `import` から `errors` を落とす（残るのは `time` だけ）。

- [ ] **Step 3: `error_test.go` を作り、テストを移す**

`domain_test.go` の `TestIsFatalOnlyForWhatTheUserMustActOn` をそのまま移す。
`domain_test.go` から削除する。

- [ ] **Step 4: 検査**

```bash
make check
```

期待: 終了コード 0。

- [ ] **Step 5: コミット**

```bash
git add internal/app/domain/error.go internal/app/domain/error_test.go internal/app/domain/domain.go internal/app/domain/domain_test.go
git commit -m "refactor(domain): give the failure sentinels their own file

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: Item まわりを 8 ファイルへ

`domain.go` の残りから、PR と Issue に関わるものを全部出す。

**Files:**
- Create: `internal/app/domain/author.go`
- Create: `internal/app/domain/label.go`
- Create: `internal/app/domain/comment.go`
- Create: `internal/app/domain/item_state.go`
- Create: `internal/app/domain/review_state.go`
- Create: `internal/app/domain/item_ref.go`
- Create: `internal/app/domain/pull_request.go`
- Create: `internal/app/domain/issue.go`
- Modify: `internal/app/domain/domain.go`

- [ ] **Step 1: 8 ファイルを作り、宣言を移す**

| 新ファイル | 移す宣言 | import |
|---|---|---|
| `author.go` | `Author` | なし |
| `label.go` | `Label` | なし |
| `comment.go` | `Comment` | `time` |
| `item_state.go` | `ItemState` と 3 つの定数（`StateOpen` / `StateClosed` / `StateMerged`） | なし |
| `review_state.go` | `ReviewState` と 4 つの定数（`ReviewNone` / `ReviewRequired` / `ReviewApproved` / `ReviewChangesRequested`） | なし |
| `item_ref.go` | `ItemKind` と 2 つの定数（`ItemPR` / `ItemIssue`）、`ItemRef` | なし |
| `pull_request.go` | `PR` | `time` |
| `issue.go` | `Issue` | `time` |

`PR` の上にある「PR and Issue carry no JSON tags: ...」というコメントは
`pull_request.go` へ移す。

- [ ] **Step 2: `domain.go` から同じ 8 群を削除する**

この時点で `domain.go` に残るのは Checks / Work board / Repos / SavedQuery まわり。

- [ ] **Step 3: 検査**

```bash
make check
```

期待: 終了コード 0。

- [ ] **Step 4: コミット**

```bash
git add internal/app/domain/
git commit -m "refactor(domain): split the pull request and issue types by model

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: `ItemRef.IsPR()`

新規コード。TDD で書く。

**Files:**
- Modify: `internal/app/domain/item_ref.go`
- Create: `internal/app/domain/item_ref_test.go`

- [ ] **Step 1: 落ちるテストを書く**

`internal/app/domain/item_ref_test.go`:

```go
package domain_test

import (
	"testing"

	"github.com/kukv/octoscope/internal/app/domain"
)

func TestIsPRAnswersFromTheKindAlone(t *testing.T) {
	pr := domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/octoscope", Number: 1}
	if !pr.IsPR() {
		t.Error("a ref whose kind is ItemPR does not say it is a pull request")
	}

	issue := domain.ItemRef{Kind: domain.ItemIssue, Repo: "kukv/octoscope", Number: 1}
	if issue.IsPR() {
		t.Error("a ref whose kind is ItemIssue says it is a pull request")
	}
}
```

- [ ] **Step 2: 落ちることを確かめる**

```bash
go test ./internal/app/domain/ -run TestIsPRAnswersFromTheKindAlone
```

期待: FAIL。`pr.IsPR undefined (type domain.ItemRef has no field or method IsPR)`

- [ ] **Step 3: 実装する**

`internal/app/domain/item_ref.go` の `ItemRef` の下に足す:

```go
// IsPR reports whether this reference names a pull request. Views ask the
// reference rather than comparing Kind themselves: the comparison is the
// closest thing in this package to the API's own spelling, and it had spread
// to fourteen places by 2026-09-19.
func (r ItemRef) IsPR() bool { return r.Kind == ItemPR }
```

- [ ] **Step 4: 通ることを確かめる**

```bash
go test ./internal/app/domain/ -run TestIsPRAnswersFromTheKindAlone
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
git commit -m "feat(domain): let a reference say whether it names a pull request

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: Checks まわりを 3 ファイルへ

今の `checks.go` には `Checks` が**無い**（`domain.go` にある）。中身を名前に合わせる。

**Files:**
- Create: `internal/app/domain/check_run.go`
- Create: `internal/app/domain/check_run_test.go`
- Create: `internal/app/domain/log_line.go`
- Modify: `internal/app/domain/checks.go`（中身を入れ替える）
- Modify: `internal/app/domain/domain.go`
- Delete: `internal/app/domain/checks_test.go`（`check_run_test.go` へ移す）

- [ ] **Step 1: `check_run.go` を作る**

移す宣言:

- `domain.go` から `CheckRun` と、そのメソッド `Duration()`
- `checks.go` から `CheckKind` と 2 つの定数（`CheckKindRun` / `CheckKindStatus`）

import: `time`

- [ ] **Step 2: `log_line.go` を作る**

`checks.go` から `LogLine` を移す。import: `time`

- [ ] **Step 3: `checks.go` の中身を入れ替える**

今 `checks.go` にあるもの（`CheckKind` / `LogLine` / `RerunScope`）を全部消し、
`domain.go` から `CheckState`（と 5 つの定数）、`Checks` を移してくる。

`RerunScope` は Task 6 で `rerun_request.go` へ行く。**それまでの置き場所として、
この Step では `checks.go` に残す**（消してしまうとビルドが通らない）。

import: なし（`Checks` も `CheckState` も `time` を使わない）。`RerunScope` も使わない。

- [ ] **Step 4: `domain.go` から `CheckState` / `Checks` / `CheckRun` / `Duration()` を削除する**

- [ ] **Step 5: `check_run_test.go` を作り、テストを移す**

`checks_test.go` の 3 本をそのまま移す:

- `TestDurationIsTheTimeBetweenStartAndFinish`
- `TestARunningCheckHasNoDuration`
- `TestACheckWithNoStartTimeHasNoDuration`

`checks_test.go` を削除する。

- [ ] **Step 6: 検査**

```bash
make check
```

期待: 終了コード 0。

- [ ] **Step 7: コミット**

```bash
git add internal/app/domain/
git commit -m "refactor(domain): put the roll-up, the run and the log line each in their own file

checks.go did not hold Checks. It does now.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: `CheckRun.HasWorkflow()`

`internal/app/presentation/tui/checks` の非公開関数 `hasWorkflow()` を型に返す。
判断の対象が `CheckRun` なので、`CheckRun` が答えるべき。

**Files:**
- Modify: `internal/app/domain/check_run.go`
- Modify: `internal/app/domain/check_run_test.go`
- Modify: `internal/app/presentation/tui/checks/checks.go:299,316,317,319,332-338`
- Modify: `internal/app/presentation/tui/checks/render.go:239,241,261`
- Modify: `internal/app/presentation/tui/checks/log.go:134`
- Modify: `internal/app/presentation/tui/checks/rerun.go:80`

現行の `hasWorkflow`（`checks.go:332-338`）はこう書かれている。
**2 つの条件の AND** であることに注意する。

```go
// hasWorkflow reports whether a check has a workflow run behind it to be
// grouped under. A StatusContext never does, and neither does a check run an
// App created: GitHub reports those with a null checkSuite.workflowRun,
// leaving WorkflowRun empty and the workflow's name empty.
func hasWorkflow(r domain.CheckRun) bool {
	return r.Kind == domain.CheckKindRun && r.WorkflowRun != ""
}
```

- [ ] **Step 1: 落ちるテストを書く**

`internal/app/domain/check_run_test.go` に足す:

```go
func TestOnlyACheckRunWithARunBehindItHasAWorkflow(t *testing.T) {
	run := domain.CheckRun{
		Name:        "build",
		Kind:        domain.CheckKindRun,
		WorkflowRun: domain.RunHandle("R_1"),
	}
	if !run.HasWorkflow() {
		t.Error("a check run that names a workflow run says it has none")
	}

	// A StatusContext is an external service reporting a state and a link.
	status := domain.CheckRun{Name: "codecov", Kind: domain.CheckKindStatus}
	if status.HasWorkflow() {
		t.Error("a StatusContext says it has a workflow")
	}

	// A check run an App created has a null checkSuite.workflowRun behind it,
	// which leaves WorkflowRun empty.
	appRun := domain.CheckRun{Name: "dependabot", Kind: domain.CheckKindRun}
	if appRun.HasWorkflow() {
		t.Error("a check run with no run id says it has a workflow")
	}
}
```

- [ ] **Step 2: 落ちることを確かめる**

```bash
go test ./internal/app/domain/ -run TestOnlyACheckRunWithARunBehindItHasAWorkflow
```

期待: FAIL。`run.HasWorkflow undefined`

- [ ] **Step 3: 実装する**

`internal/app/domain/check_run.go` の `Duration()` の下に足す。
**コメントは現行の `hasWorkflow` のものをそのまま持ってくる**（言っている
ことは変わらないため）:

```go
// HasWorkflow reports whether this check has a workflow run behind it to be
// grouped under. A StatusContext never does, and neither does a check run an
// App created: GitHub reports those with a null checkSuite.workflowRun,
// leaving WorkflowRun empty and the workflow's name empty.
func (c CheckRun) HasWorkflow() bool {
	return c.Kind == CheckKindRun && c.WorkflowRun != ""
}
```

- [ ] **Step 4: 通ることを確かめる**

```bash
go test ./internal/app/domain/ -run TestOnlyACheckRunWithARunBehindItHasAWorkflow
```

期待: PASS

- [ ] **Step 5: 呼び出し側 9 箇所を差し替える**

```bash
grep -rn 'hasWorkflow' internal/app/presentation/tui/checks/
```

| ファイル:行 | 今 | 差し替え後 |
|---|---|---|
| `checks.go:299` | `if !hasWorkflow(r) {` | `if !r.HasWorkflow() {` |
| `checks.go:316` | `if hasWorkflow(a) != hasWorkflow(b) {` | `if a.HasWorkflow() != b.HasWorkflow() {` |
| `checks.go:317` | `return hasWorkflow(a)` | `return a.HasWorkflow()` |
| `checks.go:319` | `if !hasWorkflow(a) \|\| a.Workflow == b.Workflow {` | `if !a.HasWorkflow() \|\| a.Workflow == b.Workflow {` |
| `render.go:239` | `if !hasWorkflow(r) {` | `if !r.HasWorkflow() {` |
| `render.go:241` | `return hasWorkflow(prev)` | `return prev.HasWorkflow()` |
| `render.go:261` | `if !hasWorkflow(r) {` | `if !r.HasWorkflow() {` |
| `log.go:134` | `if !hasWorkflow(r) {` | `if !r.HasWorkflow() {` |
| `rerun.go:80` | `if !hasWorkflow(r) {` | `if !r.HasWorkflow() {` |

- [ ] **Step 6: `hasWorkflow` を削除する**

`internal/app/presentation/tui/checks/checks.go:332-338` から関数ごと消す。
消したことで使われなくなった import があれば落とす。

- [ ] **Step 7: 検査**

```bash
make check
```

期待: 終了コード 0。`checks` パッケージのテストも通ること。

- [ ] **Step 8: コミット**

```bash
git add internal/app/domain/ internal/app/presentation/tui/checks/
git commit -m "refactor(domain): let a check say whether it has a run to start again

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 6: `RerunRequest` と `rerun_request.go`

`RerunScope` はモデルの属性ではなく操作の引数なので、単独では置き場所が無い。
宛先と組にして型にする。

**Files:**
- Create: `internal/app/domain/rerun_request.go`
- Modify: `internal/app/domain/checks.go`（`RerunScope` を出す）
- Modify: `internal/app/domain/tags_test.go`（一覧に `RerunRequest` を足す）

- [ ] **Step 1: `rerun_request.go` を作る**

```go
package domain

// RerunRequest is what a rerun request names: the run to start again, and how
// of it. The two travel together -- a scope with no run names nothing -- so
// they are one type rather than two arguments.
type RerunRequest struct {
	Run   RunHandle
	Scope RerunScope
}
```

続けて、`checks.go` から `RerunScope` と 2 つの定数（`RerunFailed` /
`RerunAll`）をコメントごと移す。

- [ ] **Step 2: `checks.go` から `RerunScope` を削除する**

この時点で `checks.go` に残るのは `CheckState` と `Checks` だけ。

- [ ] **Step 3: `tags_test.go` の一覧に `RerunRequest` を足す**

`tags_test.go` の公開 struct の一覧（`domain.PR{}` などが並んでいる箇所、
23 行目あたり）に `domain.RerunRequest{},` を足す。

- [ ] **Step 4: 検査**

```bash
make check
```

期待: 終了コード 0。`TestTheTagListCoversEveryExportedStruct` が
`RerunRequest` の追加を認めること。

- [ ] **Step 5: コミット**

```bash
git add internal/app/domain/
git commit -m "feat(domain): name what a rerun request targets

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

**注:** `RerunRequest` はまだ誰も使っていない。`checks.Model` が
`rerunRun` / `rerunScope` として持っている 2 フィールドを差し替えるのは
TUI 側の別の spec の仕事で、この計画の範囲外。

---

## Task 7: Review まわりを 5 ファイルへ

`review.go`（103 行）を割る。

**Files:**
- Create: `internal/app/domain/thread_comment.go`
- Create: `internal/app/domain/review_thread.go`
- Create: `internal/app/domain/pending_comment.go`
- Create: `internal/app/domain/review_target.go`
- Create: `internal/app/domain/review_context.go`
- Delete: `internal/app/domain/review.go`

- [ ] **Step 1: 5 ファイルを作り、宣言を移す**

| 新ファイル | 移す宣言 | import |
|---|---|---|
| `thread_comment.go` | `ThreadComment` | `time` |
| `review_thread.go` | `ReviewThread`、`Pending()`、`Collapsed()` | `slices` |
| `pending_comment.go` | `PendingComment` | なし |
| `review_target.go` | `ReviewTarget`、`ReviewEvent` と 3 つの定数（`EventComment` / `EventApprove` / `EventRequestChanges`） | なし |
| `review_context.go` | `ReviewContext`、`PendingCount()` | なし |

`ReviewEvent` を `review_target.go` に置くのは、提出の宛先と一緒に渡る
ものだから（設計 §5 ④）。

- [ ] **Step 2: `review.go` を削除する**

- [ ] **Step 3: 検査**

```bash
make check
```

期待: 終了コード 0。

- [ ] **Step 4: コミット**

```bash
git add internal/app/domain/
git commit -m "refactor(domain): split the review types by model

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 8: Diff まわりを 4 ファイルへ

**Files:**
- Create: `internal/app/domain/file_diff.go`
- Create: `internal/app/domain/hunk.go`
- Create: `internal/app/domain/diff_line.go`
- Create: `internal/app/domain/diff_line_test.go`
- Rename: `internal/app/domain/diff_parse.go` → `diff_parser.go`
- Rename: `internal/app/domain/diff_parse_test.go` → `diff_parser_test.go`
- Delete: `internal/app/domain/diff.go`
- Delete: `internal/app/domain/diff_test.go`

- [ ] **Step 1: 3 ファイルを作り、`diff.go` から宣言を移す**

| 新ファイル | 移す宣言 |
|---|---|
| `file_diff.go` | `FileDiff`、`FileStatus` と 7 つの定数（`FileModified` / `FileAdded` / `FileDeleted` / `FileRenamed` / `FileCopied` / `FileChanged` / `FileUnchanged`） |
| `hunk.go` | `Hunk` |
| `diff_line.go` | `DiffLine`、`Line()`、`DiffLineKind` と 3 つの定数（`LineContext` / `LineAdded` / `LineRemoved`）、`DiffSide` と 2 つの定数（`SideRight` / `SideLeft`） |

import はどれも不要。

`DiffSide` を `diff_line.go` に置く理由（設計 §5 ③）を、型の上のコメントに
残す。既存のコメントに無いので、次の一文を足す:

```go
// DiffSide is which version of a file a line or a comment belongs to.
// It lives beside DiffLine because Line below is the only place that decides
// a side; ReviewThread and PendingComment carry that answer, they do not make
// it.
```

- [ ] **Step 2: `diff.go` を削除する**

- [ ] **Step 3: `diff_parse.go` を `diff_parser.go` にリネームする**

```bash
git mv internal/app/domain/diff_parse.go internal/app/domain/diff_parser.go
git mv internal/app/domain/diff_parse_test.go internal/app/domain/diff_parser_test.go
```

中身は変えない。

- [ ] **Step 4: `diff_line_test.go` を作り、テストを移す**

`diff_test.go` の `TestDiffLineNamesTheSideToCommentOn` をそのまま移し、
`diff_test.go` を削除する。

- [ ] **Step 5: 検査**

```bash
make check
```

期待: 終了コード 0。

- [ ] **Step 6: コミット**

```bash
git add internal/app/domain/
git commit -m "refactor(domain): split the diff types by model

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 9: `merge.go` を `merge_context.go` へ

中身は 1 モデルなので、リネームだけ。

**Files:**
- Rename: `internal/app/domain/merge.go` → `merge_context.go`
- Rename: `internal/app/domain/merge_test.go` → `merge_context_test.go`

- [ ] **Step 1: リネームする**

```bash
git mv internal/app/domain/merge.go internal/app/domain/merge_context.go
git mv internal/app/domain/merge_test.go internal/app/domain/merge_context_test.go
```

中身は変えない。`MergeMethod` / `Mergeable` / `MergeState` / `MergeBlock`
の 4 つの enum は、どれも `MergeContext` の入力か出力で、単独では意味を
持たないため同じファイルに残す（設計 §4）。

- [ ] **Step 2: 検査**

```bash
make check
```

期待: 終了コード 0。

- [ ] **Step 3: コミット**

```bash
git add internal/app/domain/
git commit -m "refactor(domain): name the merge file after the model it holds

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 10: Work board と Repos を 5 ファイルへ

`domain.go` の残りを全部出し、`domain.go` を消す。`Work` はまだ配列のまま
にしておく（struct 化は Task 11）。

**Files:**
- Create: `internal/app/domain/work.go`
- Create: `internal/app/domain/work_item.go`
- Create: `internal/app/domain/repo_count.go`
- Create: `internal/app/domain/repo_candidate.go`
- Create: `internal/app/domain/saved_query.go`
- Create: `internal/app/domain/work_test.go`
- Create: `internal/app/domain/saved_query_test.go`
- Delete: `internal/app/domain/domain.go`
- Delete: `internal/app/domain/domain_test.go`

- [ ] **Step 1: 5 ファイルを作り、宣言を移す**

| 新ファイル | 移す宣言 | import |
|---|---|---|
| `work.go` | `WorkSection` と 4 つの定数 + `WorkSectionCount`、`WorkSections()`、`Work` | なし |
| `work_item.go` | `WorkItem` | `time` |
| `repo_count.go` | `RepoCount` | なし |
| `repo_candidate.go` | `RepoCandidate` | なし |
| `saved_query.go` | `SavedQuery` | なし |

`WorkSectionCount` が `iota` ブロックの最後にある理由のコメントも一緒に移す。

- [ ] **Step 2: パッケージのドキュメントコメントを移す**

`domain.go` の先頭にある `// Package domain holds the types ...` のコメントを
`work.go` ではなく、新しく作る `doc.go`... **ではなく**、`item_ref.go` の
先頭へ移す。`ItemRef` がこのパッケージのハブであり、パッケージの説明を
読む人が最初に見る型として妥当なため。

（`go doc` はどのファイルに置いてもパッケージコメントとして拾うが、
1 ファイルにしか置けない。2 つ以上あると `go vet` が警告する。）

- [ ] **Step 3: `domain.go` を削除する**

```bash
rm internal/app/domain/domain.go
```

削除後、`domain.go` が空になっていることを確認してから消すこと。
残っている宣言があれば、それはこの計画が取りこぼしたもの。その場合は
どのまとまりのものか判断して、対応するファイルへ移す。

- [ ] **Step 4: テストを移す**

`domain_test.go` から:

| テスト | 移す先 |
|---|---|
| `TestWorkSectionsCoversEveryColumn` | `work_test.go` |
| `TestWorkIndexesBySection` | `work_test.go` |
| `TestEverySectionConstantIsASlotInWork` | `work_test.go` |
| `TestSavedQueryCarriesNoSerialisationTags` | `saved_query_test.go` |

`domain_test.go` を削除する（`TestIsFatal...` は Task 1 で移済み）。

- [ ] **Step 5: 検査**

```bash
make check
wc -l internal/app/domain/*.go
```

期待: `make check` が終了コード 0。`domain.go` が一覧に無いこと。

- [ ] **Step 6: コミット**

```bash
git add internal/app/domain/
git commit -m "refactor(domain): split the board and the repository types by model

domain.go is gone. Every file is now named after the model it holds.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 11: `Work` を struct にする

配列型をやめ、添字を非公開にする。**この計画で唯一、`domain` の外の
呼び出しが壊れるタスク。**

**Files:**
- Modify: `internal/app/domain/work.go`
- Modify: `internal/app/domain/work_test.go`
- Modify: `internal/app/presentation/tui/work/work.go:170,200,251,272,273,315`
- Modify: `internal/app/presentation/tui/work/render.go:198`
- Modify: `internal/app/presentation/tui/work/mouse.go:51,96`
- Modify: `internal/app/presentation/tui/work/drawer.go:34`
- Modify: `internal/app/presentation/tui/work/mouse_test.go:223-225`
- Modify: `internal/app/presentation/tui/work/golden_test.go:94,105`

- [ ] **Step 1: 落ちるテストを書く**

`internal/app/domain/work_test.go` に足す:

```go
func TestSectionGivesBackWhatSetSectionPutIn(t *testing.T) {
	var w domain.Work
	w.SetSection(domain.SectionAssigned, []domain.WorkItem{{Title: "assigned"}})
	w.SetSection(domain.SectionMentioned, []domain.WorkItem{{Title: "mentioned"}})

	if got := w.Section(domain.SectionAssigned); len(got) != 1 || got[0].Title != "assigned" {
		t.Errorf("the assigned column holds %v", got)
	}
	if got := w.Section(domain.SectionMentioned); len(got) != 1 || got[0].Title != "mentioned" {
		t.Errorf("the mentioned column holds %v", got)
	}
	if got := w.Section(domain.SectionReviewRequested); got != nil {
		t.Errorf("a column nothing was put in holds %v", got)
	}
}
```

- [ ] **Step 2: 落ちることを確かめる**

```bash
go test ./internal/app/domain/ -run TestSectionGivesBackWhatSetSectionPutIn
```

期待: FAIL。`w.SetSection undefined`

- [ ] **Step 3: `Work` を struct にする**

`internal/app/domain/work.go` の

```go
// Work holds the items of each column, indexed by WorkSection.
type Work [WorkSectionCount][]WorkItem
```

を次に差し替える:

```go
// Work holds the items of each column. The columns are unexported so that
// the range of a section index is this type's business rather than every
// caller's: a WorkSection is the only way in, and there is no index to get
// wrong.
type Work struct {
	sections [WorkSectionCount][]WorkItem
}

// Section is the items of one column, in the order they arrived.
func (w Work) Section(s WorkSection) []WorkItem { return w.sections[s] }

// SetSection replaces one column. A fetch answers for one column at a time,
// and what the others hold is not its business.
func (w *Work) SetSection(s WorkSection, items []WorkItem) { w.sections[s] = items }
```

- [ ] **Step 4: 通ることを確かめる**

```bash
go test ./internal/app/domain/ -run TestSectionGivesBackWhatSetSectionPutIn
```

期待: PASS

- [ ] **Step 5: `domain` 内の既存テストを直す**

`work_test.go` の `TestWorkIndexesBySection` と
`TestEverySectionConstantIsASlotInWork` が添字を使っている。
`w[s]` を `w.Section(s)` に、`w[s] = items` を `w.SetSection(s, items)` に
差し替える。

**テストの意図は変えない。** `TestEverySectionConstantIsASlotInWork` は
「すべての section 定数が `Work` の枠に対応する」ことを確かめるテストで、
struct 化してもその意図は変わらない。

- [ ] **Step 6: 呼び出し側 9 箇所を差し替える**

```bash
grep -rn 'm\.work\[' internal/app/presentation/tui/work/
```

| 今 | 差し替え後 |
|---|---|
| `m.work[msg.section] = msg.items` (work.go:170) | `m.work.SetSection(msg.section, msg.items)` |
| `len(m.work[m.section()])` (work.go:200, 251) | `len(m.work.Section(m.section()))` |
| `len(m.work[domain.SectionReviewRequested])` (work.go:272) | `len(m.work.Section(domain.SectionReviewRequested))` |
| `items := m.work[m.section()]` (work.go:315) | `items := m.work.Section(m.section())` |
| `items := m.work[s]` (render.go:198) | `items := m.work.Section(s)` |
| `len(m.work[m.section()])` (mouse.go:51) | `len(m.work.Section(m.section()))` |
| `len(m.work[section])` (mouse.go:96) | `len(m.work.Section(section))` |
| `m.work[m.section()][m.row]` (drawer.go:34) | `m.work.Section(m.section())[m.row]` |

`work.go:273` の

```go
	for _, items := range m.work {
```

は配列を直接まわしている。次に差し替える:

```go
	for _, s := range domain.WorkSections() {
		items := m.work.Section(s)
```

（ループ本体のインデントを合わせること。`items` を使っている行はそのまま。）

- [ ] **Step 7: テストの呼び出しも差し替える**

`mouse_test.go:223-225`:

```go
	var w domain.Work
	for ... {
		w[domain.SectionReviewRequested] = append(w[domain.SectionReviewRequested], domain.WorkItem{
```

を次に差し替える:

```go
	var w domain.Work
	var requested []domain.WorkItem
	for ... {
		requested = append(requested, domain.WorkItem{
```

でループを回し、ループの後で

```go
	w.SetSection(domain.SectionReviewRequested, requested)
```

とする。

`golden_test.go:94` の `len(m.work[domain.SectionReviewRequested])` を
`len(m.work.Section(domain.SectionReviewRequested))` に、
`golden_test.go:105` の `m.work[m.section()][m.row].Title` を
`m.work.Section(m.section())[m.row].Title` にする。

- [ ] **Step 8: `tags_test.go` に `Work` を載せるか判断する**

`Work` は公開 struct になったので、`TestTheTagListCoversEveryExportedStruct`
が一覧に無いと落ちる可能性がある。まず走らせて確かめる:

```bash
go test ./internal/app/domain/ -run TestTheTagListCoversEveryExportedStruct
```

- **落ちたら:** `tags_test.go` の一覧に `domain.Work{},` を足す
- **通ったら:** 足さない。`Work` のフィールドは非公開で、中の `WorkItem` は
  既に一覧にあり `walkFields` が歩いている

- [ ] **Step 9: 検査**

```bash
make check
```

期待: 終了コード 0。golden テストも通ること（描画は変わらないため）。

- [ ] **Step 10: コミット**

```bash
git add internal/app/domain/ internal/app/presentation/tui/work/
git commit -m "refactor(domain): let the board keep its own column index

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## Task 12: 仕上げの検査

**Files:** なし（確認のみ）

- [ ] **Step 1: ファイルが 29 個、どれも 150 行以下であることを確かめる**

```bash
ls internal/app/domain/*.go | grep -v _test | wc -l
wc -l internal/app/domain/*.go | grep -v _test | sort -rn | head -5
```

期待: 29。最大は `merge_context.go` の約 118 行。

150 行を超えるファイルがあれば、責務が 1 つか確かめる。割らないなら
理由を spec に書き足す。

- [ ] **Step 2: `domain` の外への波及が 2 つに収まっていることを確かめる**

```bash
git diff --stat 1356952..HEAD -- . ':(exclude)internal/app/domain' ':(exclude)docs'
```

期待: `internal/app/presentation/tui/checks/`（Task 5）と
`internal/app/presentation/tui/work/`（Task 11）だけ。他が出たら、
それは計画外の変更。

- [ ] **Step 3: 実際に起動して見る**

```bash
go run ./cmd/octoscope
```

Work board・Repos・詳細・diff・checks・merge を開いて、従来どおり
描画されることを見る。**特に Work board**（Task 11 が触っているため）:
4 列が出るか、カードが正しい列に入っているか、カーソル移動とマウスが
効くか。

```bash
go run ./cmd/octoscope --lang ja
```

日本語でも同じ確認をする。全角で桁を 2 つ使うので、桁ずれが無いか見る。

**テストが通っただけでは完了としない**（`CLAUDE.md`）。

- [ ] **Step 4: 残った積み残しを記録する**

設計 §3 に挙げた 2 件は、このリファクタリングでは直していない。
`docs/superpowers/specs/2026-09-19-domain-per-model-files-design.md` の
§3 に書いてあるので、新しく書くことは無い。確認だけする。

- `WorkItem.Author` が `string`、`PR.Author` / `Issue.Author` が
  `domain.Author`
- View に `ref.Kind == domain.ItemPR` の分岐が 8 箇所（`IsPR()` に
  置き換えていない。§7 の判断）

---

## 完了条件

- [ ] `make check` が通る
- [ ] `internal/app/domain` が 29 ファイル（非テスト）で、`domain.go` が無い
- [ ] どのファイルも 150 行以下
- [ ] `domain` の外の変更が `tui/checks` と `tui/work` の 2 つだけ
- [ ] `go run ./cmd/octoscope` と `--lang ja` で Work board が従来どおり動く
