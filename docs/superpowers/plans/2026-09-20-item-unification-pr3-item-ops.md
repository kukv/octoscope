# Item 統合 PR 3（アイテム操作 5 本を新ポートへ、detail ビュー移行）実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** アイテム操作 5 本（`GetItem` `AddComment` `SetState` `EditLabels` `EditAssignees`）を
設計書 §5 の新ポートに差し替え、`usecase.Item` を削除し、種別の振り分けを usecase から消す。
detail ビューは同じコミットで `domain.Item` に移る。

**Architecture:** 差し替えであって追加ではない。usecase のポート対 10 本（`GetPR`/`GetIssue`、
`AddPRComment`/`AddIssueComment`、`ClosePR`/`ReopenPR`/`CloseIssue`/`ReopenIssue`、
`EditPRLabels`/`EditIssueLabels`、`EditPRAssignees`/`EditIssueAssignees`）が
1 メソッドずつの新ポート 5 本になり、5 つのメソッドは `Kind` 分岐を失って 1 行の委譲になる。
振り分けは PR 2 で gateway に移してあるので、ここで消えるのは**重複**である。

**Tech Stack:** Go、golangci-lint（depguard / gofumpt / goimports）、gotestsum、`internal/golden`

**Spec:** `docs/superpowers/specs/2026-09-20-item-unification-design.md`
（§5 ポートの形、§4.4 ファイル構成、§8 PR 3）

---

## この PR の分け方（2026-09-20 に設計書 §8 から変更、利用者承認済み）

設計書 §8 は「PR 3 = usecase 層、PR 4 = ビュー層」と**層で**分けていたが、実測で成立しないと分かった。
`detail/detail.go` の `itemSource` がアイテム操作 5 メソッドを現署名で宣言しているため、
`usecase.GetItem` の戻り値を変えた瞬間に detail の 5 ファイルが同じコミットで壊れる。

**機能で割る**形に変える。

- **PR 3（この計画）** = アイテム操作 5 本 + detail ビュー + `usecase.Item` 削除
- **PR 4** = `ListPRs` / `ListIssues` → `ListItems` + repo ビュー
  （repo ビューの依存は一覧 2 本だけで、アイテム操作 5 本と無関係なので切り離せる）
- **PR 5** = 旧型・旧ポートの削除、規約更新（設計書のまま、変更なし）

`ctx` を `internal/github` の書き込みメソッドに通す件（`gateway/gh/writes.go` の `_ = ctx`）は、
cli / api 両クライアントに及び Item 統合と独立なので、**この PR には入れず単独 PR にする**（利用者承認済み）。

## Global Constraints

- **golden 354 枚と `testdata/` 全体を 1 バイトも変えない。** 画面の出力は変わらない。
  この PR は detail ビューが動くので、これが実効性のある唯一かつ最強の検査である。
  `OCTOSCOPE_UPDATE_GOLDEN` を使いたくなったら、それは作業ではなく**画面を変えてしまった証拠**。
  そのステップを止めて原因を調べる
- **`domain.PR` / `domain.Issue` を消さない。** 削除は PR 5。detail のテストフィクスチャも `domain.PR` のまま
- **gateway を触らない。** 新ポート 6 本は PR 2 で実装済み。`internal/github` も触らない
- **repo ビューと `ListPRs` / `ListIssues` を触らない。** PR 4 の範囲
- **`domain` に振る舞いを足さない。** ビューは `item.Change != nil` を読む
- **新ポート 5 本はすべて第 1 引数に `ctx` を取る。** usecase の書き込み 4 メソッドも `ctx` を取るようになる
- 各タスクの最後に `make check` が通ること
- コミットメッセージの末尾に `Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>` を付ける

## 着手前に調べた事実（2026-09-20）

計画の判断はここから来ている。実装者はこれを前提にしてよい。

### A. gateway の新ポート 5 本は既にある（PR 2）

`internal/app/adapter/gateway/gh/items.go:32` に `GetItem(ctx, domain.ItemRef) (domain.Item, error)`、
`writes.go:21,36,56,71` に `AddComment` / `SetState` / `EditLabels` / `EditAssignees` が
すべて `ctx` 付きである。**usecase は宣言を変えるだけで、実装は既に待っている。**

### B. `domain.Item` のフィールド名は `usecase.Item` と 2 箇所だけ違う

| `usecase.Item` | `domain.Item` |
|---|---|
| `Kind` | `Ref.Kind` |
| `Number` | `Ref.Number` |
| `PR *domain.PR` | `Change *domain.Change` |

`Change` が持つのは 7 つ（`IsDraft` `Review` `Head` `Base` `Additions` `Deletions` `Checks`）で、
detail の `meta.go` が `it.PR` から読んでいるものと**完全に一致する**。
`domain.Item` には `BodyText` が増えるが、detail は読まない。

### C. detail が `usecase.Item` に触るのは 5 ファイル

| ファイル | 箇所 |
|---|---|
| `detail.go` | `itemSource` の 5 署名（29-33）、`itemMsg.item`（74）、`Model.item`（215） |
| `commands.go` | `GetItem`（19）、書き込み 4 箇所（51, 60, 91, 93） |
| `meta.go` | `metaRows`（45）、`it.PR`（57）、`itemStateText`（114, 116）、`it.Number`（50） |
| `body.go` | `bodyLines`（57） |
| `update.go` | `it.Kind`（108）、`it.Number`（109, 111） |

**種別で分岐する残り 8 箇所（`render.go` 3、`keys.go` 4、`detail.go:302`）は
すべて `m.ref.Kind` を見ており、`item` を見ていない。触らない。**

### D. 書き込みに渡す `ctx` は既にある形をなぞる

`commands.go` の読み出し 3 箇所（`GetItem` `PRReviewContext` `ListLabels` / `ListAssignees`）は
すでに `context.Background()` を渡している。書き込み 4 箇所も同じ形にする。
tea.Cmd はキャンセル可能な文脈を持たないので、これが現状で唯一取れる形である。

### E. `root` のフェイク 2 つが detail の `Source` を満たしている

`root/root_test.go:125-146` と `root/scenario_test.go:89-131` がアイテム操作 5 メソッドを持つ。
**テストのみの変更**だが、これを直さないとコンパイルが通らない。

### F. `usecase/item_test.go` は 300 行中 200 行が振り分けのテスト

`TestAddCommentPicksTheCallByKind` / `TestSetStatePicksTheCallByKindAndDirection` /
`TestEditLabelsPicksTheCallByKind` / `TestEditAssigneesPicksTheCallByKind` と、
その受け皿の `fakeWriter`（10 メソッド、155-208 行）が消える。
`TestGetItemFetchesAPullRequestForAPRRef` / `...AnIssueRef` / `TestGetItemCopiesEveryFieldAGhIssueHas` も
振り分けと変換の検査なので、gateway 側（`items_test.go`）が代わりになる。
**残すのは `TestGetItemPassesTheFetchFailureThrough` の系統**——委譲が素通しであることの検査。

### G. `crossRepoLister` は `work.go` にあり、`SearchItems` だけ利用側が `search.go`

`work.go:12-16` の 3 メソッドのうち `SearchItems` の唯一の呼び出し元は `search.go:16`。
§4.4 の「ポートの interface は、それを使うメソッドと同じファイルに置く」に唯一背く箇所。

### H. `fake_test.go` の `fakeSource` は 4 ポート 10 メソッド

`GetPR` / `GetIssue` / `Viewer` / `PRChecks` / `JobLog` / `RerunWorkflow` /
`PRMergeContext` / `MergePR` / `EnableAutoMerge` / `DisableAutoMerge`。
使う側は `item_test.go` / `checks_test.go` / `merge_test.go` / `lists_test.go`。
`GetPR` / `GetIssue` が抜けるのが割る機会（PR 1 からの申し送り）。

### I. `GetItem` の失敗メッセージから `get pr: ` が落ちる

現在の `usecase.GetItem` は失敗を `fmt.Errorf("get pr: %w", err)` /
`fmt.Errorf("get issue: %w", err)` で包んでいる。1 行の委譲にすると**この接頭辞が消える**——
gateway の `getPRItem` / `getIssueItem` は `wrap(err)` だけで、文言を足さない（PR 2 の事実 A）。

このエラーは `errMsg` → `ErrorMsg` として root に上がり、**画面に出る**。
golden には含まれない（detail / root のテストフェイクは usecase より上にいて、
この経路を通らない）ので、**golden の不変検査ではこの変化を捕まえられない**。

**接頭辞を種別ごとに保つには `Kind` の分岐が要り、「usecase に分岐 0 箇所」と両立しない。**
利用者の判断で **(a) 接頭辞を落とす**（この計画の前提）とした場合、設計書 §8 に
「PR 3 で失敗メッセージの `get pr: ` / `get issue: ` が落ちる」と記録すること。

---

## ファイル構成（このタスク後）

```
internal/app/usecase/
  item.go        新ポート 5 本 + 1 行委譲 5 本（usecase.Item は無い）
  item_test.go   委譲とエラーの素通しだけ（振り分けのテストは無い）
  search.go      itemSearcher（1 メソッド）+ SearchItems + SaveQueries
  work.go        crossRepoLister（2 メソッド）+ ListWorkSection + RepoCounts
  fake_test.go   ポート単位のフェイク（1 つの巨大な fakeSource は無い）

internal/app/presentation/tui/detail/
  detail.go commands.go meta.go body.go update.go   domain.Item を読む
  detail_test.go                                     prItem / issueItem が domain.Item を返す
```

---

### Task 1: `crossRepoLister` から `SearchItems` を切り出す

**Files:**
- `internal/app/usecase/work.go`（変更）
- `internal/app/usecase/search.go`（変更）
- `internal/app/usecase/usecase.go`（変更）
- `internal/app/usecase/search_test.go`（変更）

PR 1 からの申し送りのうち、他と独立に片付く 1 件。先に済ませて差分を小さくしておく。

- [ ] **Step 1: `search.go` に 1 メソッドのポートを足す**

`search.go` の `queryStore` の上に置く:

```go
// itemSearcher runs a query that names no repository: the Search tab's
// results can come from anywhere the viewer can see.
type itemSearcher interface {
	SearchItems(ctx context.Context, query string) ([]domain.WorkItem, error)
}
```

- [ ] **Step 2: `work.go` から `SearchItems` を外す**

`crossRepoLister` を 2 メソッドにし、doc をそれに合わせる:

```go
// crossRepoLister is what the Work board takes: neither of these is "the
// contents of one named repository".
type crossRepoLister interface {
	ListWorkSection(ctx context.Context, s domain.WorkSection) ([]domain.WorkItem, error)
	RepoCounts(ctx context.Context, repos []string) ([]domain.RepoCount, error)
}
```

- [ ] **Step 3: `usecase.go` を配線する**

`source` に `itemSearcher` を足し、`Usecase` に `search itemSearcher` を足し、`New` で `src` を入れる。
`SearchItems` の呼び出しを `u.crossRepo.SearchItems` から `u.search.SearchItems` に変える。

- [ ] **Step 4: `search_test.go` のフェイクを新ポートに合わせる**

`search_test.go:12` の `fakeCrossRepo` は `crossRepoLister` を満たすために
`ListWorkSection` / `RepoCounts` のスタブ 2 本を持ち、`&Usecase{crossRepo: f}` で配線している。
`fakeSearcher` に改名し、**スタブ 2 本を消し**、`&Usecase{search: f}` に変える。
呼ばないメソッドのスタブが消えるので、`architecture.md` の判定も良くなる。

- [ ] **Step 5: 検査**

```bash
make check
```

- [ ] **Step 6: 範囲と golden を確認する**

```bash
git status --porcelain              # usecase の 3 ファイルだけ
git diff --stat -- internal/app/presentation internal/app/adapter   # 空であること
```

- [ ] **Step 7: コミット**

```bash
git add internal/app/usecase
git commit -m "refactor: give SearchItems its own port in search.go" \
  -m "Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: アイテム操作 5 本を新ポートへ、`usecase.Item` を削除

**Files:**
- `internal/app/usecase/item.go`（変更）
- `internal/app/usecase/usecase.go`（変更）
- `internal/app/usecase/item_test.go`（変更）
- `internal/app/usecase/fake_test.go`（変更）
- `internal/app/presentation/tui/detail/detail.go`（変更）
- `internal/app/presentation/tui/detail/commands.go`（変更）
- `internal/app/presentation/tui/detail/meta.go`（変更）
- `internal/app/presentation/tui/detail/body.go`（変更）
- `internal/app/presentation/tui/detail/update.go`（変更）
- `internal/app/presentation/tui/detail/detail_test.go`（変更）
- `internal/app/presentation/tui/detail/body_test.go`（変更）
- `internal/app/presentation/tui/root/root_test.go`（変更）
- `internal/app/presentation/tui/root/scenario_test.go`（変更）

**このタスクは途中でコンパイルが通らない。** 型の境界が usecase とビューを跨いでいるので、
1 コミットで渡りきる。ステップは「どの順で書けば迷子にならないか」の順序であって、
各ステップの末尾でビルドが通るという意味ではない。**ビルドが通ることを確認するのは Step 9。**

- [ ] **Step 1: `usecase/item.go` を書き換える**

ポート 5 種を 1 メソッドずつに縮め、`usecase.Item` を消し、5 メソッドを 1 行の委譲にする。
ファイル全体を次の形にする:

```go
package usecase

import (
	"context"

	"github.com/kukv/octoscope/internal/app/domain"
)

// itemFetcher is one item, whichever of the two the reference names. Which
// of GitHub's two APIs answers is the gateway's business.
type itemFetcher interface {
	GetItem(ctx context.Context, ref domain.ItemRef) (domain.Item, error)
}

type commenter interface {
	AddComment(ctx context.Context, ref domain.ItemRef, body string) error
}

type stateChanger interface {
	SetState(ctx context.Context, ref domain.ItemRef, closing bool) error
}

type labelEditor interface {
	EditLabels(ctx context.Context, ref domain.ItemRef, add, remove []string) error
}

type assigneeEditor interface {
	EditAssignees(ctx context.Context, ref domain.ItemRef, add, remove []string) error
}

// GetItem fetches whichever of the two the reference names.
func (u *Usecase) GetItem(ctx context.Context, ref domain.ItemRef) (domain.Item, error) {
	return u.items.GetItem(ctx, ref)
}

func (u *Usecase) AddComment(ctx context.Context, ref domain.ItemRef, body string) error {
	return u.comments.AddComment(ctx, ref, body)
}

// SetState closes the item when closing is true and reopens it otherwise.
func (u *Usecase) SetState(ctx context.Context, ref domain.ItemRef, closing bool) error {
	return u.states.SetState(ctx, ref, closing)
}

func (u *Usecase) EditLabels(ctx context.Context, ref domain.ItemRef, add, remove []string) error {
	return u.labels.EditLabels(ctx, ref, add, remove)
}

func (u *Usecase) EditAssignees(ctx context.Context, ref domain.ItemRef, add, remove []string) error {
	return u.assignees.EditAssignees(ctx, ref, add, remove)
}
```

`fmt` と `time` の import が要らなくなる。`usecase.go` は**変えない**——
ポートの名前も `Usecase` のフィールド名も同じままなので、配線に変更は無い。

- [ ] **Step 2: detail の `itemSource` を新署名にする**

`detail.go:28-34`:

```go
type itemSource interface {
	GetItem(ctx context.Context, ref domain.ItemRef) (domain.Item, error)
	AddComment(ctx context.Context, ref domain.ItemRef, body string) error
	SetState(ctx context.Context, ref domain.ItemRef, closing bool) error
	EditLabels(ctx context.Context, ref domain.ItemRef, add, remove []string) error
	EditAssignees(ctx context.Context, ref domain.ItemRef, add, remove []string) error
}
```

`detail.go:74` の `itemMsg.item` と `detail.go:215` の `Model.item` を `domain.Item` にする。
`usecase` の import が不要になるはずなので外す（`goimports` が教えてくれる）。

- [ ] **Step 3: `commands.go` の書き込み 4 箇所に `ctx` を渡す**

```go
src.AddComment(context.Background(), ref, body)
src.SetState(context.Background(), ref, closing)
src.EditLabels(context.Background(), ref, add, remove)
src.EditAssignees(context.Background(), ref, add, remove)
```

読み出しが既にそうしている形（事実 D）。**他は何も変えない。**

- [ ] **Step 4: `meta.go` を `Change` に移す**

- `metaRows(ref domain.ItemRef, it domain.Item)`
- `it.Number` → `it.Ref.Number`（`ref` 引数ではなく**取得した item** を読む。`prItem` が
  `Ref.Number` に `pr.Number` を入れるので、出力は現在と 1 バイトも変わらない）
- `if pr := it.PR; pr != nil` → `if ch := it.Change; ch != nil`。
  中の `pr.Review` / `pr.IsDraft` / `pr.Checks` / `pr.Head` / `pr.Base` / `pr.Additions` / `pr.Deletions`
  を `ch.` にする（フィールド名は同じ。事実 B）
- `itemStateText(it domain.Item)`、`it.PR != nil && it.PR.IsDraft` → `it.Change != nil && it.Change.IsDraft`

`nil` の意味は「Issue には無い」であって種別の分岐ではない。
**doc コメントを書き換えるのはこの意味が変わる箇所だけにする。**

- [ ] **Step 5: `body.go` と `update.go`**

- `body.go:57` — `bodyLines(it domain.Item, w int, viewer string)`。中身は `it.Body` と
  `it.Comments` だけなので他は変わらない
- `update.go:108-112` — `it.Kind` → `it.Ref.Kind`、`it.Number` → `it.Ref.Number`（2 箇所）

- [ ] **Step 6: テストのフェイクとフィクスチャ**

**フィクスチャの `domain.PR` / `domain.Issue` はそのまま。変換関数だけ差し替える**（利用者承認済み）。

`detail/detail_test.go`:

```go
func (f *fakeSource) GetItem(_ context.Context, ref domain.ItemRef) (domain.Item, error) { ... }

func prItem(pr domain.PR) domain.Item {
	return domain.Item{
		Ref:       domain.ItemRef{Kind: domain.ItemPR, Number: pr.Number},
		Title:     pr.Title,
		...
		Change: &domain.Change{
			IsDraft: pr.IsDraft, Review: pr.Review, Head: pr.Head, Base: pr.Base,
			Additions: pr.Additions, Deletions: pr.Deletions, Checks: pr.Checks,
		},
	}
}
```

`Ref` は**元の `usecase.Item` が持っていた情報だけ**を移すこと。`Repo` はフィクスチャが
持っていない（`usecase.Item` に `Repo` は無かった）ので空のままにする。
なお `metaRows` の repo 行は `m.ref` から描かれるので、ここを埋めても golden は変わらない。

`issueItem` も同様（`Change` は `nil`）。書き込み 4 メソッドの署名に `ctx` を足す。
`body_test.go:14` の `withComments() usecase.Item` を `domain.Item` にする。

`root/root_test.go` と `root/scenario_test.go` のフェイク 5 メソッドも同じ署名に揃える（事実 E）。

- [ ] **Step 7: `usecase/item_test.go` から振り分けのテストを消す**

消すもの（事実 F）:
- `fakeWriter`（10 メソッド）
- `TestAddCommentPicksTheCallByKind` / `TestSetStatePicksTheCallByKindAndDirection` /
  `TestEditLabelsPicksTheCallByKind` / `TestEditAssigneesPicksTheCallByKind`
- `TestGetItemFetchesAPullRequestForAPRRef` / `TestGetItemFetchesAnIssueForAnIssueRef` /
  `TestGetItemCopiesEveryFieldAGhIssueHas` と、その助けの `assertItemMatchesCommonFields`

**代わりに置くのは「委譲が素通しである」検査だけ。** 新しいフェイクを `item_test.go` に置く:

```go
type fakeItems struct {
	item   domain.Item
	err    error
	gotRef domain.ItemRef
	calls  []string
}
```

書くテスト:
- `GetItem` が受け取った `ref` をそのまま渡し、返ってきた `domain.Item` をそのまま返すこと
- `GetItem` が失敗を**包まずそのまま**返すこと（事実 I。`errors.Is` ではなく同一であることを見る。
  この検査が「接頭辞を落とした」という判断を明文に残す）
- 書き込み 4 本が、受け取った引数をそのままポートに渡すこと（1 本 1 テストで足りる）

**この時点で `item_test.go` は 150 行前後になるはず。300 行を超えたら書きすぎである。**

- [ ] **Step 8: `fake_test.go` から `GetPR` / `GetIssue` を外す**

`fakeSource` から `GetPR` / `GetIssue` と、それが使う `pr` / `issue` / `called` フィールドを消す
（他のテストが `called` を使っていないことを `grep -rn "\.called" internal/app/usecase` で確かめる）。
**ポート単位への分割は Task 3。** ここでは消すだけにして、差分を読めるままにする。

- [ ] **Step 9: ビルドが通ることを確認する**

```bash
go build ./... && go vet ./...
```

ここで初めて通るはず。通らなければ、通らない箇所が上の 5 ファイル + テスト 4 ファイルの
外に出ていないかを見る。**出ていたら、そこは PR 4 の範囲かもしれない。止めて報告する。**

- [ ] **Step 10: テストと整形**

```bash
make fmt
make check
```

- [ ] **Step 11: golden が無傷であることを確認する（このタスクの本丸）**

```bash
git status --porcelain | grep -c testdata    # 0 であること
```

**1 枚でも変わっていたら画面を変えている。** `OCTOSCOPE_UPDATE_GOLDEN` を使わず、
差分を読んで原因を直す。疑う先は `prItem` / `issueItem` の写し漏れ（事実 B の対応表）。

- [ ] **Step 12: 振り分けが消えたことを確認する**

```bash
grep -rn "ItemPR\|ItemIssue" internal/app/usecase    # 0 件であること
grep -rn "usecase\.Item\b" --include="*.go" .        # 0 件であること
```

- [ ] **Step 13: コミット**

```bash
git add internal/app/usecase internal/app/presentation
git commit -m "refactor: move the item operations onto the unified ports" \
  -m "The five item operations now take one port each, and the detail view reads
a domain.Item. The kind dispatch they held is gone -- the gateway has done it
since PR 2, and usecase.Item is deleted with it." \
  -m "Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: `usecase` のフェイクをポート単位に割る

**Files:**
- `internal/app/usecase/fake_test.go`（変更、または削除して各テストへ移す）
- `internal/app/usecase/checks_test.go`（変更）
- `internal/app/usecase/merge_test.go`（変更）
- `internal/app/usecase/lists_test.go`（変更）

PR 1 からの申し送りの最後の 1 件。**テストのみの変更**なので、本体の差分と混ぜず独立させる。

- [ ] **Step 1: 誰が何を使っているかを数える**

```bash
grep -rn "fakeSource{" internal/app/usecase
```

`fakeSource` の残り 8 メソッドは viewer / checks / merge の 3 ポートに属する。

- [ ] **Step 2: ポート単位に割る**

`checks_test.go` に `fakeChecks`、`merge_test.go` に `fakeMerges`、
`Viewer` は使っている `lists_test.go` に `fakeViewer` を置く。
**`.claude/rules/architecture.md` の判定**（「呼ばないメソッドのスタブを何本書かされたか」）が
通る大きさにすること。割った結果 `fake_test.go` が空になるなら、ファイルごと消す。

- [ ] **Step 3: 検査**

```bash
make check
git status --porcelain | grep -c testdata    # 0 であること
```

- [ ] **Step 4: コミット**

```bash
git add internal/app/usecase
git commit -m "test: split the usecase fake along its ports" \
  -m "Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: PR 全体の確認と設計書の更新

**Files:**
- `docs/superpowers/specs/2026-09-20-item-unification-design.md`（変更）

- [ ] **Step 1: 完了条件を機械的に確かめる**

```bash
grep -rn "usecase\.Item\b" --include="*.go" .                 # 0 件
grep -rn "ItemPR\|ItemIssue" internal/app/usecase             # 0 件
grep -rn "GetPR\|GetIssue" internal/app/usecase               # 0 件
wc -l internal/app/usecase/item_test.go                       # 300 以下
git diff --stat main -- internal/app/adapter internal/github  # 空
git diff --stat main -- internal/app/presentation/tui/repo    # 空
```

最後の 2 つが空でなければ、PR 4 / 別 PR の範囲に踏み込んでいる。

- [ ] **Step 2: 実際に起動して見る**

```bash
go run ./cmd/octoscope
go run ./cmd/octoscope --lang ja
```

detail ビューを PR と Issue の両方で開き、メタ欄（レビュー / チェック / ブランチ / 変更量が
PR にだけ出て、Issue には出ないこと）、コメント投稿、クローズ / 再オープン、
ラベルとアサイニーの編集を触る。**`make check` が通っただけで完了にしない**（CLAUDE.md）。

- [ ] **Step 3: 全体検査**

```bash
make check
make release-check
```

- [ ] **Step 4: 設計書 §8 を書き直す**

1. §8 の冒頭、PR の一覧が層で分かれている前提を、機能で割る形に直す
2. 「### PR 3: usecase を新ポートへ」を「### PR 3: アイテム操作と detail ビュー」にし、
   範囲に detail ビューを含める。**成立しなかった理由を 2 行で残す**
   （`itemSource` が 5 メソッドを宣言しているのでコンパイル境界が層と一致しない）
3. 「### PR 4: ビューの移行」を「### PR 4: 一覧と repo ビュー」にし、
   範囲を `ListPRs` / `ListIssues` → `ListItems` と `repo/repo.go` に絞る
4. PR 3 の申し送りのうち片付いた 3 件（`SearchItems` の切り出し、`fakeSource` の分割、
   `item_test.go` の行数）を消し、**`**完了: 2026-09-20。**` を足す**
5. `ctx` を `backend` に通す件は PR 3 の申し送りから外す。§5 末尾の
   「PR 3 で `backend` の書き込みメソッドに `ctx` を通せば解消する（§8 PR 3 への申し送り）」を
   「**独立した PR で片付ける**（利用者判断、2026-09-20）」に**書き換える**
6. §8 PR 3 に、失敗メッセージから `get pr: ` / `get issue: ` が落ちたことを 1 行記録する（事実 I）

**§9 の完了条件と他の節は変えない。**

- [ ] **Step 5: コミット**

```bash
git add docs/superpowers/specs/2026-09-20-item-unification-design.md
git commit -m "docs: split PR 3 and PR 4 by feature, not by layer" \
  -m "Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## この PR の完了条件

- `usecase.Item` が存在しない
- アイテム操作のポートが 1 メソッドずつの 5 本で、すべて `ctx` を第 1 引数に取る
- `internal/app/usecase` に `ItemKind` の分岐が 0 箇所
- `SearchItems` が `search.go` の独立ポートになっている
- `usecase` のテストフェイクがポート単位に割れている
- `usecase/item_test.go` が 300 行以下
- detail ビュー 5 ファイルが `domain.Item` を読み、PR 固有の値は `Change` 経由である
- **golden 354 枚と `testdata/` 全体が無変更**
- `domain.PR` / `domain.Issue` / 旧ポート / repo ビュー / gateway / `internal/github` が無変更
- `make check` と `make release-check` が通る
- 実際に起動して、PR と Issue の detail を日英両方で確認した

## 次の PR

- **PR 4**: `ListPRs` / `ListIssues` → `ListItems`、repo ビューの移行
- **独立 PR**: `internal/github` の書き込みメソッドに `ctx` を通し、`writes.go` の `_ = ctx` を消す
