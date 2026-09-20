# Item 統合 PR 1（ファイル分割）実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `internal/app/domain` と `internal/app/usecase` を設計書 §4.4 の概念単位のファイルに割る。宣言の中身は 1 文字も変えない。

**Architecture:** 純粋な移動である。型も関数もテストも、**別のファイルに切り出すだけ**で、名前も署名も本文もコメントも変えない。Go では同じパッケージ内でファイルをまたいでも参照が変わらないので、import の調整以外に書き換えは発生しない。`domain.go` と、肥大化した `usecase_test.go` が無くなる。

**Tech Stack:** Go、golangci-lint（depguard / gofumpt / goimports）、gotestsum、`internal/golden`

**Spec:** `docs/superpowers/specs/2026-09-20-item-unification-design.md`（§4.4 ファイル構成、§8 PR 1）

## Global Constraints

- **振る舞いを一切変えない。** この PR の diff は、行の移動と import 節の増減だけで説明できなければならない。宣言の名前・署名・本文・コメントを変えない。「ついでの整形」も「ついでのコメント改善」も禁止
- **`domain.Item` も `domain.Change` もこの PR では作らない。** `domain.PR` と `domain.Issue` は現状のまま `item.go` に入る。統合は PR 2 以降
- **ポートの形も変えない。** `AddPRComment` / `AddIssueComment` の対はそのまま。`ctx` の追加もしない
- **golden ファイルと testdata を 1 バイトも変えない。** 各タスクの検証で `git status --porcelain -- '*testdata*'` が空であることを確認する
- **`internal/app/domain/tags_test.go` の `exported` リストは変えない。** 型は増減しないので、22 個のまま
- 各タスクの最後に `make check` が通ること。通らない状態でコミットしない
- コミットメッセージの末尾に `Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>` を付ける

## 実測値（2026-09-20、着手時点）

| 対象 | 値 |
|---|---|
| `internal/app/domain/domain.go` | 281 行、8 概念、公開宣言 20 |
| `internal/app/domain/domain_test.go` | 105 行、テスト 5 本 |
| `internal/app/usecase/usecase.go` | **315 行**（規約の 300 行を超過）、interface 14 / メソッド 23 |
| `internal/app/usecase/usecase_test.go` | **526 行**、テスト 16 本 + 共有フェイク `fakeSource` |
| golden ファイル | 386 枚 / 13 ディレクトリ |
| domain の公開 struct（`tags_test.go` の `exported`） | 22 |

## ファイル構成（分割後）

```
internal/app/domain/
  doc.go          パッケージ doc のみ
  item.go         Author, Label, Comment, ItemState, PR, Issue, ItemKind, ItemRef
  work.go         WorkItem, WorkSection, WorkSections(), Work
  checks.go       CheckState, CheckRun, Duration(), Checks ＋ 既存の CheckKind, LogLine, RerunScope
  review.go       ReviewState ＋ 既存すべて
  repo.go         RepoCount, RepoCandidate
  query.go        SavedQuery
  errors.go       ErrBackendUnavailable, ErrTransient, ErrUnauthenticated,
                  classified, Classify(), IsFatal()
  merge.go / diff.go / diff_parse.go / handle.go   変更なし

  errors_test.go  TestIsFatalOnlyForWhatTheUserMustActOn
  work_test.go    TestWorkSectionsCoversEveryColumn, TestWorkIndexesBySection,
                  TestEverySectionConstantIsASlotInWork
  query_test.go   TestSavedQueryCarriesNoSerialisationTags
  tags_test.go / checks_test.go / merge_test.go / diff_test.go /
  diff_parse_test.go                                変更なし

internal/app/usecase/
  usecase.go      source, settingsStore, Usecase, New
  item.go         itemFetcher, commenter, stateChanger, labelEditor, assigneeEditor,
                  Item, GetItem, AddComment, SetState, EditLabels, EditAssignees
  lists.go        lister, viewerFetcher, ListPRs, ListIssues, RepoName, Viewer,
                  ListLabels, ListAssignees
  work.go         crossRepoLister, ListWorkSection, RepoCounts
  review.go       reviewFetcher, reviewer, PRDiff, PRReviewContext, DiscardReview
                  ＋ 既存の PostLineComment, SubmitReview
  checks.go       checksFetcher, PRChecks, JobLog, RerunWorkflow
  merge.go        merger, PRMergeContext, MergePR, EnableAutoMerge, DisableAutoMerge
  repos.go        repoFinder, repoStore ＋ 既存すべて
  search.go       queryStore ＋ 既存すべて

  fake_test.go    fakeSource（共有フェイク）
  item_test.go    GetItem / AddComment / SetState / EditLabels / EditAssignees のテスト
  lists_test.go   Viewer のテスト
  checks_test.go  PRChecks / JobLog / RerunWorkflow のテスト
  merge_test.go   MergePR のテスト
  search_test.go  SearchItems / SaveQueries のテスト
  repos_test.go / review_test.go                    変更なし
```

`domain.go` / `domain_test.go` / `usecase.go` の 23 メソッド / `usecase_test.go` は無くなる
（`usecase.go` というファイル名自体は残り、中身が `Usecase` と `New` だけになる）。

**`crossRepoLister` は `work.go` に置く。** この interface は `SearchItems` も宣言しており、
それを使う `Usecase.SearchItems` は `search.go` にある。Go では同じパッケージ内なので
参照できる。**interface を分割しない** — それは移動ではなく変更になるため。

**`ListPRs` / `ListIssues` は `lists.go` に置く。** 設計書 §4.4 では `ListItems` が
`item.go` に入るが、それは PR 3 で `lister` から抜き出すときに移す。この PR では
`lister` を丸ごと `lists.go` に置く。

## 検証の考え方

新しい振る舞いを足さない。したがって**既存のテストスイートと golden ファイルがテストそのもの**である。
各タスクの検証は次の 4 点で、すべて満たすまで次へ進まない。

1. `make check` が通る（tidy / lint / fmt / race 付きテスト）
2. `git status --porcelain -- '*testdata*'` が空
3. `git diff -M --stat` で、そのタスクの差分が移動として説明できる
4. **そのファイルの宣言の集合が、分割前と後で完全に一致する**（Step で `grep` を使って機械的に確認する）

---

### Task 1: `domain` の型を概念単位のファイルに割る

**Files:**
- Create: `internal/app/domain/doc.go`, `item.go`, `work.go`, `repo.go`, `query.go`, `errors.go`
- Modify: `internal/app/domain/checks.go`, `internal/app/domain/review.go`
- Delete: `internal/app/domain/domain.go`

**Interfaces:**
- Consumes: なし（このタスクが最初）
- Produces: パッケージの公開 API は一切変わらない。`domain.PR`、`domain.Checks`、`domain.IsFatal` などの
  参照はすべてそのまま動く。後続タスクはこの前提に立つ

- [ ] **Step 1: 分割前の基準を取る**

分割後に照合するための、宣言の一覧と行数を記録する。

```bash
cd internal/app/domain
grep -h '^type \|^func \|^var \|^const ' *.go | grep -v '_test.go' | sort > /tmp/domain-decls-before.txt
wc -l /tmp/domain-decls-before.txt
go build ./... && echo BUILD-OK
```

期待: 宣言が一覧に出ること。この時点のビルドが通ること。

- [ ] **Step 2: `doc.go` を作る**

`domain.go` の先頭 4 行（パッケージ doc）と `package domain` だけを移す。

```go
// Package domain holds the types the application is written in terms of.
// It has no behaviour beyond the rules those types carry, and it depends on
// nothing: internal/app/adapter/gateway/gh translates a service's own
// spelling into these values before anything reaches this package.
package domain
```

`domain.go` からはこの doc コメントを消す（`package domain` の行は残す。次の Step で全部消える）。

- [ ] **Step 3: `errors.go` を作る**

`domain.go` から次を**そのまま**移す。コメントも一緒に運ぶ。

- `ErrBackendUnavailable`（doc コメント込み、L12-15）
- `ErrTransient`（L17-20）
- `ErrUnauthenticated`（L22-23）
- `classified` struct とその `Error()` / `Unwrap()`（L255-265）
- `Classify()`（L267-269）
- `IsFatal()`（L271-281）

import は `"errors"` のみ。

- [ ] **Step 4: `item.go` を作る**

`domain.go` から次をそのまま移す。

- `Author`（L25-27）
- `Label`（L29-32）
- `Comment`（L34-38）
- `ItemState` と const ブロック（L40-51）
- `PR`（L50-78。`// PR and Issue carry no JSON tags:` で始まるコメント L50-52 を含む）
- `Issue`（L80-94）
- `ItemKind` と const ブロック（L96-104）
- `ItemRef`（L106-112）

import は `"time"` のみ。

- [ ] **Step 5: `work.go` を作る**

`domain.go` から次をそのまま移す。

- `WorkItem`（L178-201）
- `WorkSection` と const ブロック（L203-215）
- `WorkSections()`（L217-224）
- `Work`（L226-230）

import は `"time"` のみ。

- [ ] **Step 6: `repo.go` と `query.go` を作る**

`repo.go` に `RepoCount`（L228-237）と `RepoCandidate`（L239-243）を、
`query.go` に `SavedQuery`（L245-251）を、それぞれコメント込みでそのまま移す。
どちらも import は不要。

- [ ] **Step 7: `checks.go` と `review.go` に追記する**

既存ファイルの**末尾に**追記する（既存の宣言は動かさない）。

`checks.go` へ:
- `CheckState` と const ブロック（`domain.go` L123-137）
- `CheckRun`（L134-157）
- `func (c CheckRun) Duration()`（L159-165）
- `Checks`（L167-177）

`review.go` へ:
- `ReviewState` と const ブロック（`domain.go` L114-122）

`checks.go` は既に `"time"` を import している。`review.go` も同様。

- [ ] **Step 8: `domain.go` を消す**

```bash
git rm internal/app/domain/domain.go
```

- [ ] **Step 9: 宣言の集合が一致することを確認する**

```bash
cd internal/app/domain
grep -h '^type \|^func \|^var \|^const ' *.go | grep -v '_test.go' | sort > /tmp/domain-decls-after.txt
diff /tmp/domain-decls-before.txt /tmp/domain-decls-after.txt && echo DECLS-IDENTICAL
```

期待: `DECLS-IDENTICAL`。差分が出たら、宣言を落としたか増やしたかしている。直す。

- [ ] **Step 10: 整形して検査する**

```bash
make fmt
make check
```

期待: 通る。`goimports` が余分な import を落とすので、Step 3-7 で import を厳密に書けていなくてもここで揃う。

- [ ] **Step 11: golden と testdata が無傷であることを確認する**

```bash
git status --porcelain -- '*testdata*'
```

期待: **出力が空**。1 行でも出たら、テストが画面出力を変えている。原因を調べる。

- [ ] **Step 12: 行数を確認する**

```bash
wc -l internal/app/domain/*.go | grep -v _test | sort -rn
```

期待: `domain.go` が無く、`diff_parse.go`（171 行）以外はすべて 120 行以下。

- [ ] **Step 13: コミット**

```bash
git add internal/app/domain
git commit -m "refactor: split the domain package by concept" \
  -m "domain.go held eight unrelated concepts in 281 lines, with Checks and ReviewState sitting apart from the checks.go and review.go files that already existed. Move each declaration to the file its neighbours are in. Nothing is renamed and no body changes." \
  -m "Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: `domain` のテストを割る

**Files:**
- Create: `internal/app/domain/errors_test.go`, `work_test.go`, `query_test.go`
- Delete: `internal/app/domain/domain_test.go`

**Interfaces:**
- Consumes: Task 1 の後のファイル構成
- Produces: テスト関数の集合は変わらない。後続タスクは `make test` が同じ本数を通すことを前提にする

- [ ] **Step 1: 分割前のテスト本数を数える**

```bash
cd internal/app/domain
grep -h '^func Test' *_test.go | sort > /tmp/domain-tests-before.txt
wc -l /tmp/domain-tests-before.txt
```

期待: 18 本（`checks_test.go` 3 / `diff_parse_test.go` 4 / `diff_test.go` 1 /
`domain_test.go` 5 / `merge_test.go` 3 / `tags_test.go` 2）。

- [ ] **Step 2: `errors_test.go` を作る**

`domain_test.go` から `TestIsFatalOnlyForWhatTheUserMustActOn`（L15-33）をそのまま移す。
`package domain_test`、import は `"errors"` `"testing"` と domain。

- [ ] **Step 3: `work_test.go` を作る**

`domain_test.go` から次の 3 本をそのまま移す。

- `TestWorkSectionsCoversEveryColumn`（L35-53）
- `TestWorkIndexesBySection`（L55-69）
- `TestEverySectionConstantIsASlotInWork`（L71-96）

- [ ] **Step 4: `query_test.go` を作る**

`domain_test.go` から `TestSavedQueryCarriesNoSerialisationTags`（L98-105）をそのまま移す。
import に `"reflect"` が要る。

- [ ] **Step 5: `domain_test.go` を消す**

```bash
git rm internal/app/domain/domain_test.go
```

- [ ] **Step 6: テストの集合が一致することを確認する**

```bash
cd internal/app/domain
grep -h '^func Test' *_test.go | sort > /tmp/domain-tests-after.txt
diff /tmp/domain-tests-before.txt /tmp/domain-tests-after.txt && echo TESTS-IDENTICAL
```

期待: `TESTS-IDENTICAL`。

- [ ] **Step 7: 整形して検査する**

```bash
make fmt
make check
```

期待: 通る。

- [ ] **Step 8: golden と testdata が無傷であることを確認する**

```bash
git status --porcelain -- '*testdata*'
```

期待: 出力が空。

- [ ] **Step 9: コミット**

```bash
git add internal/app/domain
git commit -m "refactor: split the domain tests alongside the types" \
  -m "domain_test.go covered three unrelated concepts. Put each test next to the file that now holds what it tests." \
  -m "Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: `usecase` のポートとメソッドを概念単位のファイルに割る

**Files:**
- Create: `internal/app/usecase/item.go`, `lists.go`, `work.go`, `checks.go`, `merge.go`
- Modify: `internal/app/usecase/usecase.go`（`source` / `settingsStore` / `Usecase` / `New` だけを残す）、
  `review.go`、`repos.go`、`search.go`（それぞれポートの interface を受け取る）

**Interfaces:**
- Consumes: Task 1・2 の後の `domain` パッケージ
- Produces: `usecase` パッケージの公開 API は一切変わらない。`*usecase.Usecase` の 30 メソッドも
  `usecase.Item` もそのまま。PR 2 以降はこの構成の上で `item.go` を書き換える

- [ ] **Step 1: 分割前の基準を取る**

```bash
cd internal/app/usecase
grep -h '^type \|^func ' *.go | grep -v '_test.go' | sort > /tmp/usecase-decls-before.txt
wc -l /tmp/usecase-decls-before.txt
go build ./... && echo BUILD-OK
```

- [ ] **Step 2: `item.go` を作る**

`usecase.go` から次をそのまま移す（コメント込み）。

- `itemFetcher`（L13-16）
- `commenter`（L18-21）
- `stateChanger`（L23-28）
- `labelEditor`（L30-33）
- `assigneeEditor`（L35-38）
- `Item`（L169-186。`// Item is where a pull request and an issue meet:` のコメントを含む）
- `GetItem`（L188-211）
- `AddComment`（L213-218）
- `SetState`（L220-232）
- `EditLabels`（L234-239）
- `EditAssignees`（L241-246）

import は `"context"` `"fmt"` `"time"` と domain。

- [ ] **Step 3: `lists.go` を作る**

`usecase.go` から次をそのまま移す。

- `lister`（L40-46）
- `viewerFetcher`（L48-52。`// viewerFetcher names the signed-in user.` のコメントを含む）
- `ListPRs`（L256-258）
- `ListIssues`（L260-262）
- `RepoName`（L264）
- `Viewer`（L266-267。コメント込み）
- `ListLabels`（L269-271）
- `ListAssignees`（L273-275）

- [ ] **Step 4: `work.go` を作る**

`usecase.go` から次をそのまま移す。

- `crossRepoLister`（L54-61。コメント込み）
- `ListWorkSection`（L248-250）
- `RepoCounts`（L252-254）

`crossRepoLister` が宣言する `SearchItems` を使う `Usecase.SearchItems` は `search.go` に残るが、
同じパッケージなので参照できる。**interface は分割しない。**

- [ ] **Step 5: `checks.go` と `merge.go` を作る**

`checks.go` へ:
- `checksFetcher`（L99-103）
- `PRChecks`（L289-291）
- `JobLog`（L293-295）
- `RerunWorkflow`（L297-299）

`merge.go` へ:
- `merger`（L105-110）
- `PRMergeContext`（L301-303）
- `MergePR`（L305-307）
- `EnableAutoMerge`（L309-311）
- `DisableAutoMerge`（L313-315）

- [ ] **Step 6: `review.go` / `repos.go` / `search.go` に interface を移す**

既存ファイルの**先頭（import の直後）に**追記する。既存の関数は動かさない。

`review.go` へ:
- `reviewFetcher`（`usecase.go` L88-91）
- `reviewer`（L93-97）
- `PRDiff`（L277-279）
- `PRReviewContext`（L281-283）
- `DiscardReview`（L285-287）

`repos.go` へ:
- `repoFinder`（L63-69。コメント込み）
- `repoStore`（L71-74。コメント込み）

`search.go` へ:
- `queryStore`（L76-79。コメント込み）

- [ ] **Step 7: `usecase.go` に残るものを確認する**

`usecase.go` に残るのは次だけになる。

- パッケージ doc（L1-2）と import
- `settingsStore`（L81-86）
- `source`（L112-126）
- `Usecase`（L128-145）
- `New`（L147-167）

`settingsStore` は `repoStore` と `queryStore` を embed するが、それらは `repos.go` と `search.go` に移った。
同じパッケージなので参照できる。

- [ ] **Step 8: 宣言の集合が一致することを確認する**

```bash
cd internal/app/usecase
grep -h '^type \|^func ' *.go | grep -v '_test.go' | sort > /tmp/usecase-decls-after.txt
diff /tmp/usecase-decls-before.txt /tmp/usecase-decls-after.txt && echo DECLS-IDENTICAL
```

期待: `DECLS-IDENTICAL`。

- [ ] **Step 9: 整形して検査する**

```bash
make fmt
make check
```

期待: 通る。

- [ ] **Step 10: golden と testdata が無傷であることを確認する**

```bash
git status --porcelain -- '*testdata*'
```

期待: 出力が空。

- [ ] **Step 11: 行数を確認する**

```bash
wc -l internal/app/usecase/*.go | grep -v _test | sort -rn
```

期待: 300 行を超えるファイルが 1 つも無い。`usecase.go` は 90 行前後になる。

- [ ] **Step 12: コミット**

```bash
git add internal/app/usecase
git commit -m "refactor: split the usecase package by concept" \
  -m "usecase.go was 315 lines, past the 300-line mark the rules set, with fourteen port interfaces and twenty-three methods in one file. Put each port next to the methods that use it, so what one operation needs reads in one file. Nothing is renamed and no body changes." \
  -m "Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: `usecase` のテストを割る

**Files:**
- Create: `internal/app/usecase/fake_test.go`, `item_test.go`, `lists_test.go`, `checks_test.go`,
  `merge_test.go`, `search_test.go`
- Delete: `internal/app/usecase/usecase_test.go`

**Interfaces:**
- Consumes: Task 3 の後のファイル構成。`fakeSource` は `package usecase`（内部テストパッケージ）にある
- Produces: テスト関数 16 本はそのまま。`fakeSource` が `fake_test.go` に移り、
  PR 3 以降で新ポートのフェイクを足すときの置き場所が決まる

- [ ] **Step 1: 分割前のテスト本数を数える**

```bash
cd internal/app/usecase
grep -h '^func Test' *_test.go | sort > /tmp/usecase-tests-before.txt
wc -l /tmp/usecase-tests-before.txt
```

期待: 26 本（`usecase_test.go` 16 / `repos_test.go` 6 / `review_test.go` 4）。

**`usecase_test.go` にはフェイクが 4 つある。** 置き場所は使われ方で決まる。

| フェイク | 位置 | 使うテスト | 行き先 |
|---|---|---|---|
| `fakeSource` | L13-88 | item / lists / checks / merge の各テスト | `fake_test.go`（共有） |
| `fakeWriter` | L232-286 | AddComment / SetState / EditLabels / EditAssignees | `item_test.go` |
| `fakeCrossRepo` | L423-440 | SearchItems | `search_test.go` |
| `fakeQueryStore` | L454-462 | SaveQueries | `search_test.go` |

**1 ファイルからしか使われないフェイクは、そのファイルに置く。** `fake_test.go` に集めない——
集めると「このテストが何を必要としているか」がまた読めなくなり、分割した意味が消える。

- [ ] **Step 2: `fake_test.go` を作る**

`usecase_test.go` の L1-88 をそのまま移す。`package usecase`、`fakeSource` の struct 定義と
そのメソッド 10 本（`GetPR` `GetIssue` `Viewer` `PRChecks` `JobLog` `RerunWorkflow`
`PRMergeContext` `MergePR` `EnableAutoMerge` `DisableAutoMerge`）。

`fakeSource` だけが複数のテストファイルから使われるので、これだけを独立したファイルにする。

- [ ] **Step 3: `item_test.go` を作る**

`usecase_test.go` から次をそのまま移す。**`fakeWriter` も一緒に移す。**

- `TestGetItemFetchesAPullRequestForAPRRef`（L89-119）
- `TestGetItemFetchesAnIssueForAnIssueRef`（L120-143）
- `TestGetItemPassesTheFetchFailureThrough`（L144-156）
- `TestGetItemCopiesEveryFieldAGhIssueHas`（L157-231）
- **`fakeWriter` の struct とメソッド 10 本**（L232-286）
- `TestAddCommentPicksTheCallByKind`（L287-308）
- `TestSetStatePicksTheCallByKindAndDirection`（L309-334）
- `TestEditLabelsPicksTheCallByKind`（L335-356）
- `TestEditAssigneesPicksTheCallByKind`（L506-526）

- [ ] **Step 4: `merge_test.go` / `checks_test.go` / `search_test.go` / `lists_test.go` を作る**

`merge_test.go` へ:
- `TestMergePRPassesTheMethodThrough`（L357-369）

`checks_test.go` へ:
- `TestPRChecksReachesTheBackend`（L370-387）
- `TestJobLogReachesTheBackend`（L388-406）
- `TestRerunWorkflowReachesTheBackend`（L407-422）

`search_test.go` へ（**`fakeCrossRepo` と `fakeQueryStore` も一緒に移す**）:
- `fakeCrossRepo` の struct とメソッド 3 本（L423-440）
- `TestSearchItemsReachesTheGitHubLayer`（L441-453）
- `fakeQueryStore` の struct とメソッド 1 本（L454-462）
- `TestSaveQueriesReachesTheStore`（L463-480）

`lists_test.go` へ:
- `TestViewerPassesTheLoginThrough`（L481-494）
- `TestViewerPassesTheFailureThrough`（L495-505）

- [ ] **Step 5: `usecase_test.go` を消す**

```bash
git rm internal/app/usecase/usecase_test.go
```

- [ ] **Step 6: テストの集合が一致することを確認する**

```bash
cd internal/app/usecase
grep -h '^func Test' *_test.go | sort > /tmp/usecase-tests-after.txt
diff /tmp/usecase-tests-before.txt /tmp/usecase-tests-after.txt && echo TESTS-IDENTICAL
grep -h '^type fake' *_test.go | sort
```

期待: `TESTS-IDENTICAL`。そしてフェイクが 7 つ、それぞれ 1 回だけ出ること
（`fakeSource` `fakeWriter` `fakeCrossRepo` `fakeQueryStore` `fakeReviewer`
`fakeRepoFinder` `fakeRepoStore`）。重複していたら、移したつもりで元にも残している。

- [ ] **Step 7: 整形して検査する**

```bash
make fmt
make check
```

期待: 通る。フェイクの定義が重複したり、どのファイルからも使われないヘルパーが残ったりすると
`golangci-lint` が落とすので、ここで気づける。

- [ ] **Step 8: golden と testdata が無傷であることを確認する**

```bash
git status --porcelain -- '*testdata*'
```

期待: 出力が空。

- [ ] **Step 9: コミット**

```bash
git add internal/app/usecase
git commit -m "refactor: split the usecase tests alongside the ports" \
  -m "usecase_test.go was 526 lines covering six concepts around one shared fake. Put each test next to the file that holds what it exercises, and give the fake a file of its own." \
  -m "Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: PR 全体の確認

**Files:** なし（検査のみ）

**Interfaces:**
- Consumes: Task 1-4 のすべて
- Produces: PR 2 が乗る土台。設計書 §9 の完了条件のうち、ファイル構成に関する 2 項目が満たされる

- [ ] **Step 1: 差分が移動として読めることを確認する**

```bash
git diff -M --stat main...HEAD
```

期待: 追加行と削除行がおおむね釣り合っていること。
**新規に書かれたロジックの行が無いこと**を目で確認する。

- [ ] **Step 2: 設計書 §9 の該当項目を確認する**

```bash
test ! -f internal/app/domain/domain.go && echo "domain.go: GONE"
wc -l internal/app/domain/*.go internal/app/usecase/*.go | grep -v total | awk '$1 > 300 {print "OVER 300:", $2, $1}'
```

期待: `domain.go: GONE` が出て、300 行超のファイルが 1 つも報告されないこと。
（`diff_parse.go` は 171 行、`usecase_test.go` は消えている）

- [ ] **Step 3: 公開 API が変わっていないことを確認する**

```bash
go build ./... && echo BUILD-OK
git diff main...HEAD -- internal/app/presentation internal/app/adapter cmd | head
```

期待: `BUILD-OK`。そして **presentation / adapter / cmd に差分が 1 行も無いこと**。
1 行でも出たら、パッケージの公開 API を変えてしまっている。

- [ ] **Step 4: 全体検査**

```bash
make check
make release-check
```

期待: どちらも通る。

- [ ] **Step 5: TUI を起動して目視で確認する**

```bash
go run ./cmd/octoscope
go run ./cmd/octoscope --lang ja
```

期待: どちらも起動し、Work ボードが描かれる。
CLAUDE.md の「TUI の変更は、テストが通っただけで完了とみなさない」に従う。
このタスクは振る舞いを変えていないので、**分割前と見た目が同じであること**が確認したいことである。

- [ ] **Step 6: 設計書の PR 1 にチェックを入れる**

`docs/superpowers/specs/2026-09-20-item-unification-design.md` §8 の PR 1 節に、
完了した旨と日付を 1 行足してコミットする。

```bash
git add docs/superpowers/specs/2026-09-20-item-unification-design.md
git commit -m "docs: mark PR 1 of the Item unification as done" \
  -m "Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## この PR の完了条件

- `internal/app/domain/domain.go` と `internal/app/domain/domain_test.go` が存在しない
- `internal/app/usecase/usecase_test.go` が存在しない
- `internal/app/domain` と `internal/app/usecase` に 300 行を超える `.go` が無い
- `domain` の宣言の集合と、`usecase` の宣言の集合が、分割前と完全に一致する
- テスト関数の集合が分割前と完全に一致する（domain 18 本、usecase 26 本）
- `presentation` / `adapter` / `cmd` に差分が 1 行も無い
- golden 386 枚と testdata が無変更
- `make check` と `make release-check` が通る
- `go run ./cmd/octoscope` と `--lang ja` が分割前と同じ画面を描く

## 次の PR

PR 2（`domain.Item` と `Change` の追加、gateway に新ポートを実装）は、
**この PR がマージされてから**計画を書く。この PR が作る `domain/item.go` と
`usecase/item.go` が PR 2 の作業対象であり、その中身が確定してからでないと
正確な行番号を書けないためである。
