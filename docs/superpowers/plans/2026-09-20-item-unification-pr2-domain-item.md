# Item 統合 PR 2（`domain.Item` と新ポートの追加）実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `domain.Item` と `domain.Change` を足し、`gateway/gh` に設計書 §5 の新ポート 6 本を実装する。種別の振り分けは gateway に置く。旧ポート 14 本はまだ残し、誰も新ポートを呼んでいない状態で終わる。

**Architecture:** 追加だけで、既存のものは何も消さない。`domain.PR` と `domain.Issue` はそのまま残り、`toPR` / `toIssue` も残る。新しい `toItem` / `toItemFromPR` / `toItemFromIssue` が gql のワイヤ型から `domain.Item` を作り、`Gateway` の 6 メソッドが `domain.ItemRef.Kind` で呼び分ける。振り分けが usecase から gateway へ移る最初の一歩。

**Tech Stack:** Go、golangci-lint（depguard / gofumpt / goimports）、gotestsum、`internal/golden`

**Spec:** `docs/superpowers/specs/2026-09-20-item-unification-design.md`（§4.1 モデル、§4.2 `ItemRef`、§5 ポートの形、§8 PR 2）

## Global Constraints

- **既存のものを消さない。** `domain.PR` `domain.Issue` `toPR` `toIssue`、旧ポート 14 本はすべて残る。削除は PR 5
- **新ポートを誰も呼ばない状態で終わる。** `usecase` も `presentation` も触らない。差し替えは PR 3・4
- **golden ファイルと testdata を 1 バイトも変えない。** 画面に出るものは何も変わらない
- **`domain` に振る舞いを足さない。** `Item.IsChange()` のようなヘルパーメソッドを作らない。ビューは `item.Change != nil` を読む
- **`Change != nil` ⟺ `Ref.Kind == ItemPR`** が守るべき不変条件。保証するのは gateway で、テストもそこに置く
- 新ポート 6 本はすべて第 1 引数に `ctx` を取る
- 各タスクの最後に `make check` が通ること
- コミットメッセージの末尾に `Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>` を付ける

## 着手前に調べた事実（2026-09-20）

計画の判断はここから来ている。実装者はこれを前提にしてよい。

### A. `wrap()` は文言を変えない

`internal/app/adapter/gateway/gh/errors.go` の `wrap` は 3 つのセンチネル
（`ErrNotInstalled` / `ErrUnauthenticated` / `ErrTransient`）だけを `domain.Classify` で包み、
それ以外は**そのまま通す**。そして `domain.Classify` が作る `classified` は
`Error()` で gh の文言だけを返す（センチネルの英文を前に付けない）。

**したがって `wrap` を通しても、画面に出る文字列は変わらない。**

### B. 書き込みの失敗は `IsFatal` を見ていない

`domain.IsFatal` を参照しているのは `work.go:176` / `search.go:293` / `repo.go:472` の 3 箇所だけで、
**いずれも一覧取得（読み取り）の失敗**である。detail ビューの書き込みの失敗は
`commentErrorMsg` / `stateErrorMsg` / `pickErrorMsg` に入って行内の通知として出るだけで
（`presentation/tui/detail/commands.go:52,61,71,81,96`）、`IsFatal` を通らない。

**A と B から、新ポート 6 本すべてで `wrap` を通してよい。**
今日の観測可能な振る舞いは変わらず、将来ビューが書き込みの失敗に `IsFatal` を使いたくなったとき、
それが正しく動く形になる。現在の書き込みポート（`AddPRComment` など）は `Gateway` が
override せず embed から promote しているだけなので `wrap` を通っていない。
**新ポートはここを揃える。**

### C. gateway のテスト用フェイクは既にあり、拡張は軽い

`internal/app/adapter/gateway/gh/items_test.go:18` の `fakeBackend` は
**`backend` interface を embed** し、テストが設定した関数フィールドだけを答える形になっている。
実装しないメソッドを呼ぶと nil パニックで大きな音が鳴る。

設計書 §7.3 は「振り分けのテストが gateway に降りるとフェイクが重くなる」を
規約変更の代償として挙げていたが、**実測ではこの仕組みが既にあるため、
足すのは関数フィールドとメソッドを数本だけ**である。想定より軽い。

### D. `usecase.Item` に無く `domain.Item` にあるフィールド

`BodyText`。現在 `usecase.Item` はこれを持たず、`presentation` 側で `BodyText` を読んでいるのは
`repo/render.go:97,109`（`issue.BodyText` / `pr.BodyText`）だけで、これは `domain.Issue` /
`domain.PR` から直接読んでいる。PR 4 で `item.BodyText` に変わるが、値は同じものである。

### E. `ctx` は gateway で受け取るが backend には渡せない

`backend` の書き込みメソッド（`AddPRComment` など）は `ctx` を取らない。
新ポートは `ctx` を取るので、gateway は**受け取って使わない**。
これは設計書 §5 が決めた形であり、`backend` 側に `ctx` を通すのは PR 2 の範囲外。
**PR 3 への申し送りとして §8 に記録する**（Task 4）。書き残さないと `ctx` は飾りのまま残る。

## ファイル構成（このタスク後）

```
internal/app/domain/
  item.go        ＋ Item, Change を追加。ItemRef の doc を書き直す
  tags_test.go   exported を 22 → 24 に（domain 側で新しいテストファイルは作らない。下記参照）

internal/app/adapter/gateway/gh/
  items.go       ＋ GetItem, toItemFromPR, toItemFromIssue
  lists.go       ＋ ListItems
  writes.go      （新規）AddComment, SetState, EditLabels, EditAssignees
  items_test.go  fakeBackend に関数フィールドを追加、GetItem と不変条件のテスト
  lists_test.go  ListItems のテスト
  writes_test.go （新規）振り分けのテスト
```

**`domain` に新しいテストファイルは作らない。** `Item` も `Change` もただの struct で、
振る舞いを持たない（Global Constraints のとおりヘルパーも足さない）。
アサーションの無いテストを置くのは、レビューの規準では欠陥である。
不変条件を検査するのは、それを生み出す gateway 側（Task 2）。

`domain` 側で走るテストは `tags_test.go` の既存 2 本で、これは型を足すと**落ちる**。
Task 1 はそれを RED として使う。

---

### Task 1: `domain.Item` と `domain.Change` を足す

**Files:**
- Modify: `internal/app/domain/item.go`
- Modify: `internal/app/domain/tags_test.go`

**Interfaces:**
- Consumes: なし
- Produces: `domain.Item` と `domain.Change`。Task 2・3 はこれを組み立てる

- [ ] **Step 1: 型を足す（まだテストは通らない）**

`internal/app/domain/item.go` の `ItemRef` の**後ろに**次を足す。

```go
// Item is one pull request or issue: the thing this application is about.
// A view that does not care which of the two it has holds an Item; one that
// does asks Ref.Kind, or reads Change.
type Item struct {
	Ref       ItemRef
	Title     string
	Author    Author
	State     ItemState
	URL       string
	// Body is the markdown the author wrote, which the detail view renders.
	// BodyText is the same text with the markdown stripped, which the drawer
	// previews. They come from different queries: a listed item carries only
	// BodyText, and one fetched on its own only Body.
	Body      string
	BodyText  string
	Comments  []Comment
	Labels    []Label
	Assignees []Author
	UpdatedAt time.Time

	// Change is the part only a pull request has: it proposes a change to the
	// repository. It is non-nil if and only if Ref.Kind is ItemPR, which the
	// gateway guarantees when it builds an Item.
	Change *Change
}

// Change is what a pull request adds to an item: the branches it moves
// between, how big it is, and what review and CI have said about it.
type Change struct {
	IsDraft   bool
	Review    ReviewState
	Head      string
	Base      string
	Additions int
	Deletions int
	Checks    Checks
}
```

`Number` は `Ref` の中にあるので `Item` は持たない。

- [ ] **Step 2: `ItemRef` の doc を書き直す**

現在の doc:

```go
// ItemRef names one pull request or issue. Repo is "owner/name": both the
// Work board and the Repos tab can open an item from another repository, so
// the reference carries its own.
```

次に差し替える。

```go
// ItemRef is an Item's identity: which repository it is in, and which number
// it has there. Repo is "owner/name", and the reference carries its own
// because both the Work board and the Repos tab can open an item from
// another repository.
//
// Kind is part of the identity rather than of the Item because a reference
// is held before the Item is fetched -- a card on the Work board sends one
// to open a view, and what it points at has to be known by then.
```

**他の doc は変えない。**

- [ ] **Step 3: テストが落ちることを確認する（RED）**

```bash
go test ./internal/app/domain/ -run TestTheTagListCoversEveryExportedStruct -v 2>&1 | tail -20
```

期待: **FAIL**。`tags_test.go` の `exported` に `Item` と `Change` が無いので、
`go/ast` でパッケージを解析して一覧の網羅を検査するテストが「2 型が漏れている」と言って落ちる。

落ちない場合は、型の追加が効いていないか、テストが期待どおり働いていない。止まって報告すること。

- [ ] **Step 4: `exported` に 2 型を足す（GREEN）**

`internal/app/domain/tags_test.go` の `exported` リストに、`domain.ItemRef{}` の**隣に**足す。

```go
	domain.ItemRef{},
	domain.Item{},
	domain.Change{},
```

リストは 22 → 24 個になる。

- [ ] **Step 5: テストが通ることを確認する（GREEN）**

```bash
go test ./internal/app/domain/ -v 2>&1 | tail -20
```

期待: **PASS**。`TestTheTagListCoversEveryExportedStruct` と
`TestNoDomainTypeCarriesASerialisationTag` の両方が通る。後者は新しい 2 型に
json タグが無いことも検査する（`Change` が `Checks` を持つので、その中まで再帰する）。

- [ ] **Step 6: 整形して全体検査**

```bash
make fmt
make check
```

期待: 通る。

- [ ] **Step 7: golden と testdata が無傷であることを確認する**

```bash
git status --porcelain -- '*testdata*'
```

期待: 出力が空。

- [ ] **Step 8: コミット**

```bash
git add internal/app/domain
git commit -m "feat: add domain.Item and domain.Change" \
  -m "One pull request or issue, with the pull-request-only part behind a pointer that is non-nil exactly when the reference says ItemPR. Nothing builds one yet; the gateway does that next. ItemRef's doc now says it is the Item's identity, and why Kind belongs in it." \
  -m "Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: gateway に読み取りの新ポート 2 本を足す

**Files:**
- Modify: `internal/app/adapter/gateway/gh/items.go`（`GetItem`、`toItemFromPR`、`toItemFromIssue`）
- Modify: `internal/app/adapter/gateway/gh/lists.go`（`ListItems`）
- Modify: `internal/app/adapter/gateway/gh/items_test.go`（`fakeBackend` はそのまま使える）
- Modify: `internal/app/adapter/gateway/gh/lists_test.go`

**Interfaces:**
- Consumes: Task 1 の `domain.Item` / `domain.Change`
- Produces:
  - `func (g *Gateway) GetItem(ctx context.Context, ref domain.ItemRef) (domain.Item, error)`
  - `func (g *Gateway) ListItems(ctx context.Context, repo string, kind domain.ItemKind) ([]domain.Item, error)`
  - `func toItemFromPR(n gql.PullRequest, repo string) domain.Item`
  - `func toItemFromIssue(n gql.Issue, repo string) domain.Item`

  Task 3 は同じ `Gateway` に書き込みを足す。PR 3 の usecase はこの 2 本を呼ぶ

- [ ] **Step 1: 落ちるテストを書く（RED）— 不変条件**

`internal/app/adapter/gateway/gh/items_test.go` の末尾に足す。
既存の `fakeBackend`（`items_test.go:18`、`backend` を embed し関数フィールドで答える）を使う。

```go
// TestGetItemKeepsChangeAndKindInStep is the invariant domain.Item's doc
// states: Change is non-nil exactly when the reference says ItemPR. The
// gateway is what guarantees it, because the gateway is what builds an Item.
func TestGetItemKeepsChangeAndKindInStep(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		ref       domain.ItemRef
		backend   fakeBackend
		wantKind  domain.ItemKind
		wantChange bool
	}{
		{
			name: "a pull request has a change",
			ref:  domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/octoscope", Number: 7},
			backend: fakeBackend{getPR: func(context.Context, string, int) (gql.PullRequest, error) {
				return gql.PullRequest{Number: 7, HeadRefName: "topic", BaseRefName: "main"}, nil
			}},
			wantKind:   domain.ItemPR,
			wantChange: true,
		},
		{
			name: "an issue has none",
			ref:  domain.ItemRef{Kind: domain.ItemIssue, Repo: "kukv/octoscope", Number: 9},
			backend: fakeBackend{getIssue: func(context.Context, string, int) (gql.Issue, error) {
				return gql.Issue{Number: 9}, nil
			}},
			wantKind:   domain.ItemIssue,
			wantChange: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := New(tt.backend).GetItem(context.Background(), tt.ref)
			if err != nil {
				t.Fatalf("GetItem: %v", err)
			}
			if got.Ref.Kind != tt.wantKind {
				t.Errorf("Ref.Kind = %v, want %v", got.Ref.Kind, tt.wantKind)
			}
			if (got.Change != nil) != tt.wantChange {
				t.Errorf("Change != nil = %v, want %v", got.Change != nil, tt.wantChange)
			}
			if got.Ref != tt.ref {
				t.Errorf("Ref = %+v, want %+v", got.Ref, tt.ref)
			}
		})
	}
}
```

- [ ] **Step 2: テストが落ちることを確認する（RED）**

```bash
go test ./internal/app/adapter/gateway/gh/ -run TestGetItemKeepsChangeAndKindInStep 2>&1 | tail -10
```

期待: **コンパイルエラー**で落ちる（`New(...).GetItem` がまだ無い）。
「undefined: GetItem」のような出力になる。これが期待する失敗である。

- [ ] **Step 3: `toItemFromPR` と `toItemFromIssue` を書く**

`internal/app/adapter/gateway/gh/items.go` の `toIssue` の**後ろに**足す。
`toPR` / `toIssue` は**消さない**。

```go
// toItemFromPR builds the domain's Item out of a pull request. Change is
// always set here: that is the half of domain.Item's invariant this function
// owns, and toItemFromIssue owns the other.
func toItemFromPR(n gql.PullRequest, repo string) domain.Item {
	return domain.Item{
		Ref:       domain.ItemRef{Kind: domain.ItemPR, Repo: repo, Number: n.Number},
		Title:     n.Title,
		Author:    domain.Author{Login: n.Author.Login},
		State:     parseItemState(n.State),
		URL:       n.URL,
		Body:      n.Body,
		BodyText:  n.BodyText,
		Comments:  toComments(n.Comments.Nodes),
		Labels:    toLabels(n.Labels.Nodes),
		Assignees: toAuthors(n.Assignees.Nodes),
		UpdatedAt: n.UpdatedAt,
		Change: &domain.Change{
			IsDraft:   n.IsDraft,
			Review:    parseReviewDecision(n.ReviewDecision),
			Head:      n.HeadRefName,
			Base:      n.BaseRefName,
			Additions: n.Additions,
			Deletions: n.Deletions,
			Checks:    toChecksFromContexts(n.StatusCheckContexts()),
		},
	}
}

// toItemFromIssue builds the domain's Item out of an issue. Change stays nil:
// an issue proposes no change.
func toItemFromIssue(n gql.Issue, repo string) domain.Item {
	return domain.Item{
		Ref:       domain.ItemRef{Kind: domain.ItemIssue, Repo: repo, Number: n.Number},
		Title:     n.Title,
		Author:    domain.Author{Login: n.Author.Login},
		State:     parseItemState(n.State),
		URL:       n.URL,
		Body:      n.Body,
		BodyText:  n.BodyText,
		Comments:  toComments(n.Comments.Nodes),
		Labels:    toLabels(n.Labels.Nodes),
		Assignees: toAuthors(n.Assignees.Nodes),
		UpdatedAt: n.UpdatedAt,
	}
}
```

- [ ] **Step 4: `GetItem` を書く**

`items.go` の `GetIssue` の**後ろに**足す。`GetPR` / `GetIssue` は**消さない**。

```go
// GetItem returns whichever of the two the reference names, with its body and
// conversation. Which of GitHub's two queries that takes is this layer's
// knowledge: nothing above it switches on the kind.
func (g *Gateway) GetItem(ctx context.Context, ref domain.ItemRef) (domain.Item, error) {
	if ref.Kind == domain.ItemPR {
		n, err := g.backend.GetPR(ctx, ref.Repo, ref.Number)
		if err != nil {
			return domain.Item{}, wrap(err)
		}
		return toItemFromPR(n, ref.Repo), nil
	}
	n, err := g.backend.GetIssue(ctx, ref.Repo, ref.Number)
	if err != nil {
		return domain.Item{}, wrap(err)
	}
	return toItemFromIssue(n, ref.Repo), nil
}
```

- [ ] **Step 5: テストが通ることを確認する（GREEN）**

```bash
go test ./internal/app/adapter/gateway/gh/ -run TestGetItemKeepsChangeAndKindInStep -v 2>&1 | tail -15
```

期待: **PASS**（サブテスト 2 本とも）。

- [ ] **Step 6: `ListItems` の落ちるテストを書く（RED）**

`internal/app/adapter/gateway/gh/lists_test.go` の末尾に足す。

```go
// TestListItemsPicksTheQueryByKind checks the other half of the dispatch:
// which of GitHub's two listings runs is decided here, not above.
func TestListItemsPicksTheQueryByKind(t *testing.T) {
	t.Parallel()
	t.Run("pull requests", func(t *testing.T) {
		t.Parallel()
		b := fakeBackend{listPRs: func(context.Context, string) ([]gql.PullRequest, error) {
			return []gql.PullRequest{{Number: 1}, {Number: 2}}, nil
		}}
		got, err := New(b).ListItems(context.Background(), "kukv/octoscope", domain.ItemPR)
		if err != nil {
			t.Fatalf("ListItems: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("got %d items, want 2", len(got))
		}
		for _, item := range got {
			if item.Ref.Kind != domain.ItemPR || item.Change == nil {
				t.Errorf("item %+v is not a pull request with a change", item.Ref)
			}
			if item.Ref.Repo != "kukv/octoscope" {
				t.Errorf("Ref.Repo = %q, want the repository asked for", item.Ref.Repo)
			}
		}
	})
	t.Run("issues", func(t *testing.T) {
		t.Parallel()
		b := fakeBackend{listIssues: func(context.Context, string) ([]gql.Issue, error) {
			return []gql.Issue{{Number: 3}}, nil
		}}
		got, err := New(b).ListItems(context.Background(), "kukv/octoscope", domain.ItemIssue)
		if err != nil {
			t.Fatalf("ListItems: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("got %d items, want 1", len(got))
		}
		if got[0].Ref.Kind != domain.ItemIssue || got[0].Change != nil {
			t.Errorf("item %+v is not an issue without a change", got[0].Ref)
		}
	})
}
```

- [ ] **Step 7: テストが落ちることを確認する（RED）**

```bash
go test ./internal/app/adapter/gateway/gh/ -run TestListItemsPicksTheQueryByKind 2>&1 | tail -10
```

期待: **コンパイルエラー**（`ListItems` が無い）。

- [ ] **Step 8: `ListItems` を書く（GREEN）**

`internal/app/adapter/gateway/gh/lists.go` の `ListIssues` の**後ろに**足す。
`ListPRs` / `ListIssues` は**消さない**。

```go
// ListItems returns the open pull requests or the open issues of one
// repository. kind is which listing to run, not a branch in the caller: the
// Repos tab draws the two in separate panes and knows which it is filling.
func (g *Gateway) ListItems(ctx context.Context, repo string, kind domain.ItemKind) ([]domain.Item, error) {
	if kind == domain.ItemPR {
		nodes, err := g.backend.ListPRs(ctx, repo)
		if err != nil {
			return nil, wrap(err)
		}
		items := make([]domain.Item, len(nodes))
		for i, n := range nodes {
			items[i] = toItemFromPR(n, repo)
		}
		return items, nil
	}
	nodes, err := g.backend.ListIssues(ctx, repo)
	if err != nil {
		return nil, wrap(err)
	}
	items := make([]domain.Item, len(nodes))
	for i, n := range nodes {
		items[i] = toItemFromIssue(n, repo)
	}
	return items, nil
}
```

- [ ] **Step 9: テストが通ることを確認する（GREEN）**

```bash
go test ./internal/app/adapter/gateway/gh/ -run 'TestGetItemKeepsChangeAndKindInStep|TestListItemsPicksTheQueryByKind' -v 2>&1 | tail -20
```

期待: **PASS**（4 サブテストすべて）。

- [ ] **Step 10: 整形して全体検査**

```bash
make fmt
make check
```

期待: 通る。旧ポートのテスト（`TestGetPRTranslatesTheWireShapeIntoTheDomain` など）も通ったままであること。

- [ ] **Step 11: golden と testdata が無傷であることを確認する**

```bash
git status --porcelain -- '*testdata*'
git diff --stat -- internal/app/usecase internal/app/presentation cmd
```

期待: どちらも**出力が空**。この PR は usecase もビューも触らない。

- [ ] **Step 12: コミット**

```bash
git add internal/app/adapter/gateway/gh
git commit -m "feat: build domain.Item in the gateway" \
  -m "GetItem and ListItems pick which of GitHub's two queries to run, so nothing above this layer switches on the kind. Change is set exactly when the reference says ItemPR, which is the invariant domain.Item's doc states and this layer owns. The old GetPR, GetIssue, ListPRs and ListIssues stay until PR 5." \
  -m "Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: gateway に書き込みの新ポート 4 本を足す

**Files:**
- Create: `internal/app/adapter/gateway/gh/writes.go`
- Create: `internal/app/adapter/gateway/gh/writes_test.go`
- Modify: `internal/app/adapter/gateway/gh/items_test.go`（`fakeBackend` に関数フィールドを追加）

**Interfaces:**
- Consumes: Task 1 の `domain.ItemRef`
- Produces:
  - `func (g *Gateway) AddComment(ctx context.Context, ref domain.ItemRef, body string) error`
  - `func (g *Gateway) SetState(ctx context.Context, ref domain.ItemRef, closing bool) error`
  - `func (g *Gateway) EditLabels(ctx context.Context, ref domain.ItemRef, add, remove []string) error`
  - `func (g *Gateway) EditAssignees(ctx context.Context, ref domain.ItemRef, add, remove []string) error`

  PR 3 の usecase はこの 4 本を呼び、自分の `Kind` 分岐 5 本を消す

- [ ] **Step 1: `fakeBackend` に関数フィールドを足す**

`internal/app/adapter/gateway/gh/items_test.go:18` の `fakeBackend` に、
既存の並びに合わせて足す（フィールド名は先頭小文字、メソッド名と対応させる）。

```go
	addPRComment       func(repo string, number int, body string) error
	addIssueComment    func(repo string, number int, body string) error
	closePR            func(repo string, number int) error
	reopenPR           func(repo string, number int) error
	closeIssue         func(repo string, number int) error
	reopenIssue        func(repo string, number int) error
	editPRLabels       func(repo string, number int, add, remove []string) error
	editIssueLabels    func(repo string, number int, add, remove []string) error
	editPRAssignees    func(repo string, number int, add, remove []string) error
	editIssueAssignees func(repo string, number int, add, remove []string) error
```

対応するメソッドも、既存の `GetPR` / `GetIssue` の書き方に合わせて 10 本足す。例:

```go
func (f fakeBackend) AddPRComment(repo string, number int, body string) error {
	return f.addPRComment(repo, number, body)
}
```

**`backend` を embed しているので、テストが設定しなかったメソッドを呼べば nil パニックで落ちる。**
これが「呼ぶべきでない方を呼んだ」ことの検査になる。

- [ ] **Step 2: 落ちるテストを書く（RED）**

`internal/app/adapter/gateway/gh/writes_test.go` を作る。
**呼ばれるべき方だけを設定し、呼ばれてはいけない方は設定しない。**
間違った方を呼べばパニックで落ちる。

```go
package gh

import (
	"context"
	"slices"
	"testing"

	"github.com/kukv/octoscope/internal/app/domain"
)

func prRef() domain.ItemRef {
	return domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/octoscope", Number: 7}
}

func issueRef() domain.ItemRef {
	return domain.ItemRef{Kind: domain.ItemIssue, Repo: "kukv/octoscope", Number: 9}
}

// TestAddCommentPicksTheCallByKind is the dispatch that used to live in the
// usecase layer. The fake leaves the other call unset, so reaching for it
// panics rather than passing quietly.
func TestAddCommentPicksTheCallByKind(t *testing.T) {
	t.Parallel()
	t.Run("pull request", func(t *testing.T) {
		t.Parallel()
		var gotRepo, gotBody string
		var gotNumber int
		b := fakeBackend{addPRComment: func(repo string, number int, body string) error {
			gotRepo, gotNumber, gotBody = repo, number, body
			return nil
		}}
		if err := New(b).AddComment(context.Background(), prRef(), "hello"); err != nil {
			t.Fatalf("AddComment: %v", err)
		}
		if gotRepo != "kukv/octoscope" || gotNumber != 7 || gotBody != "hello" {
			t.Errorf("backend saw %q %d %q", gotRepo, gotNumber, gotBody)
		}
	})
	t.Run("issue", func(t *testing.T) {
		t.Parallel()
		var gotRepo, gotBody string
		var gotNumber int
		b := fakeBackend{addIssueComment: func(repo string, number int, body string) error {
			gotRepo, gotNumber, gotBody = repo, number, body
			return nil
		}}
		if err := New(b).AddComment(context.Background(), issueRef(), "hello"); err != nil {
			t.Fatalf("AddComment: %v", err)
		}
		if gotRepo != "kukv/octoscope" || gotNumber != 9 || gotBody != "hello" {
			t.Errorf("backend saw %q %d %q", gotRepo, gotNumber, gotBody)
		}
	})
}

// TestSetStatePicksTheCallByKindAndDirection covers all four combinations:
// two kinds times closing and reopening.
func TestSetStatePicksTheCallByKindAndDirection(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		ref     domain.ItemRef
		closing bool
		set     func(called *string) fakeBackend
		want    string
	}{
		{
			name: "close a pull request", ref: prRef(), closing: true, want: "ClosePR",
			set: func(called *string) fakeBackend {
				return fakeBackend{closePR: func(string, int) error { *called = "ClosePR"; return nil }}
			},
		},
		{
			name: "reopen a pull request", ref: prRef(), closing: false, want: "ReopenPR",
			set: func(called *string) fakeBackend {
				return fakeBackend{reopenPR: func(string, int) error { *called = "ReopenPR"; return nil }}
			},
		},
		{
			name: "close an issue", ref: issueRef(), closing: true, want: "CloseIssue",
			set: func(called *string) fakeBackend {
				return fakeBackend{closeIssue: func(string, int) error { *called = "CloseIssue"; return nil }}
			},
		},
		{
			name: "reopen an issue", ref: issueRef(), closing: false, want: "ReopenIssue",
			set: func(called *string) fakeBackend {
				return fakeBackend{reopenIssue: func(string, int) error { *called = "ReopenIssue"; return nil }}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var called string
			if err := New(tt.set(&called)).SetState(context.Background(), tt.ref, tt.closing); err != nil {
				t.Fatalf("SetState: %v", err)
			}
			if called != tt.want {
				t.Errorf("called %q, want %q", called, tt.want)
			}
		})
	}
}

// TestEditLabelsPicksTheCallByKind and TestEditAssigneesPicksTheCallByKind
// check the two remaining pairs, and that add and remove arrive in order.
func TestEditLabelsPicksTheCallByKind(t *testing.T) {
	t.Parallel()
	t.Run("pull request", func(t *testing.T) {
		t.Parallel()
		var add, remove []string
		b := fakeBackend{editPRLabels: func(_ string, _ int, a, r []string) error {
			add, remove = a, r
			return nil
		}}
		if err := New(b).EditLabels(context.Background(), prRef(), []string{"bug"}, []string{"wip"}); err != nil {
			t.Fatalf("EditLabels: %v", err)
		}
		if !slices.Equal(add, []string{"bug"}) || !slices.Equal(remove, []string{"wip"}) {
			t.Errorf("add = %v, remove = %v", add, remove)
		}
	})
	t.Run("issue", func(t *testing.T) {
		t.Parallel()
		var called bool
		b := fakeBackend{editIssueLabels: func(string, int, []string, []string) error {
			called = true
			return nil
		}}
		if err := New(b).EditLabels(context.Background(), issueRef(), nil, nil); err != nil {
			t.Fatalf("EditLabels: %v", err)
		}
		if !called {
			t.Error("EditIssueLabels was not called")
		}
	})
}

func TestEditAssigneesPicksTheCallByKind(t *testing.T) {
	t.Parallel()
	t.Run("pull request", func(t *testing.T) {
		t.Parallel()
		var add, remove []string
		b := fakeBackend{editPRAssignees: func(_ string, _ int, a, r []string) error {
			add, remove = a, r
			return nil
		}}
		if err := New(b).EditAssignees(context.Background(), prRef(), []string{"kukv"}, nil); err != nil {
			t.Fatalf("EditAssignees: %v", err)
		}
		if !slices.Equal(add, []string{"kukv"}) || remove != nil {
			t.Errorf("add = %v, remove = %v", add, remove)
		}
	})
	t.Run("issue", func(t *testing.T) {
		t.Parallel()
		var called bool
		b := fakeBackend{editIssueAssignees: func(string, int, []string, []string) error {
			called = true
			return nil
		}}
		if err := New(b).EditAssignees(context.Background(), issueRef(), nil, nil); err != nil {
			t.Fatalf("EditAssignees: %v", err)
		}
		if !called {
			t.Error("EditIssueAssignees was not called")
		}
	})
}
```

- [ ] **Step 3: テストが落ちることを確認する（RED）**

```bash
go test ./internal/app/adapter/gateway/gh/ -run 'PicksTheCall' 2>&1 | tail -10
```

期待: **コンパイルエラー**（`AddComment` `SetState` `EditLabels` `EditAssignees` が無い）。

- [ ] **Step 4: `writes.go` を書く（GREEN）**

```go
package gh

import (
	"context"

	"github.com/kukv/octoscope/internal/app/domain"
)

// The four operations below are where the difference between a pull request
// and an issue stops. GitHub gives each of them two endpoints; the
// application has one operation, and this layer is what joins them.
//
// ctx is taken and not used: the client's write methods do not accept one
// yet. The port takes it so that threading it through later changes this
// file and not every caller.

// AddComment posts one comment on the item the reference names.
func (g *Gateway) AddComment(ctx context.Context, ref domain.ItemRef, body string) error {
	_ = ctx
	if ref.Kind == domain.ItemPR {
		return wrap(g.backend.AddPRComment(ref.Repo, ref.Number, body))
	}
	return wrap(g.backend.AddIssueComment(ref.Repo, ref.Number, body))
}

// SetState closes the item when closing is true and reopens it otherwise.
func (g *Gateway) SetState(ctx context.Context, ref domain.ItemRef, closing bool) error {
	_ = ctx
	switch {
	case ref.Kind == domain.ItemPR && closing:
		return wrap(g.backend.ClosePR(ref.Repo, ref.Number))
	case ref.Kind == domain.ItemPR:
		return wrap(g.backend.ReopenPR(ref.Repo, ref.Number))
	case closing:
		return wrap(g.backend.CloseIssue(ref.Repo, ref.Number))
	default:
		return wrap(g.backend.ReopenIssue(ref.Repo, ref.Number))
	}
}

// EditLabels adds and removes labels in one call.
func (g *Gateway) EditLabels(ctx context.Context, ref domain.ItemRef, add, remove []string) error {
	_ = ctx
	if ref.Kind == domain.ItemPR {
		return wrap(g.backend.EditPRLabels(ref.Repo, ref.Number, add, remove))
	}
	return wrap(g.backend.EditIssueLabels(ref.Repo, ref.Number, add, remove))
}

// EditAssignees adds and removes assignees in one call.
func (g *Gateway) EditAssignees(ctx context.Context, ref domain.ItemRef, add, remove []string) error {
	_ = ctx
	if ref.Kind == domain.ItemPR {
		return wrap(g.backend.EditPRAssignees(ref.Repo, ref.Number, add, remove))
	}
	return wrap(g.backend.EditIssueAssignees(ref.Repo, ref.Number, add, remove))
}
```

`wrap` を通すのは、この計画の「着手前に調べた事実」A と B で確かめたとおり、
文言も画面も変わらず、将来ビューが書き込みの失敗に `IsFatal` を使いたくなったときに効くからである。

- [ ] **Step 5: テストが通ることを確認する（GREEN）**

```bash
go test ./internal/app/adapter/gateway/gh/ -run 'PicksTheCall' -v 2>&1 | tail -25
```

期待: **PASS**（サブテスト 10 本すべて）。
**パニックが 1 つも出ないこと**——出たら、呼ぶべきでない方を呼んでいる。

- [ ] **Step 6: `wrap` が文言を変えないことを確認するテストを足す**

`writes_test.go` の末尾に足す。Step 4 の判断が正しいことを、実装の中で固定する。

```go
// TestAddCommentKeepsWhatTheClientSaid checks the claim the wrap in writes.go
// rests on: a failure the domain has a sentinel for gains that sentinel and
// keeps its own text, so what reaches the screen is unchanged.
func TestAddCommentKeepsWhatTheClientSaid(t *testing.T) {
	t.Parallel()
	b := fakeBackend{addPRComment: func(string, int, string) error {
		return fmt.Errorf("gh: %w", github.ErrUnauthenticated)
	}}
	err := New(b).AddComment(context.Background(), prRef(), "hello")
	if err == nil {
		t.Fatal("AddComment returned no error")
	}
	if !errors.Is(err, domain.ErrUnauthenticated) {
		t.Errorf("error does not carry the domain's sentinel: %v", err)
	}
	if got, want := err.Error(), "gh: "+github.ErrUnauthenticated.Error(); got != want {
		t.Errorf("Error() = %q, want the client's own text %q", got, want)
	}
}
```

import に `"errors"` `"fmt"` と `"github.com/kukv/octoscope/internal/github"` を足す。

- [ ] **Step 7: テストが通ることを確認する**

```bash
go test ./internal/app/adapter/gateway/gh/ -run TestAddCommentKeepsWhatTheClientSaid -v 2>&1 | tail -10
```

期待: **PASS**。落ちたら `wrap` の前提が崩れているので、止まって報告すること
（`writes.go` の `wrap` を外して逃げない）。

- [ ] **Step 8: 整形して全体検査**

```bash
make fmt
make check
```

期待: 通る。

- [ ] **Step 9: 範囲と golden を確認する**

```bash
git status --porcelain -- '*testdata*'
git diff --stat -- internal/app/usecase internal/app/presentation cmd internal/github
```

期待: どちらも**出力が空**。この PR は gateway と domain しか触らない。

- [ ] **Step 10: コミット**

```bash
git add internal/app/adapter/gateway/gh
git commit -m "feat: move the PR/issue dispatch into the gateway" \
  -m "AddComment, SetState, EditLabels and EditAssignees each take an ItemRef and pick which of GitHub's two endpoints to call. The fake leaves the other endpoint unset, so a wrong pick panics rather than passing. The old pairs stay until PR 5, and the usecase layer still calls them until PR 3." \
  -m "Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: PR 全体の確認と申し送り

**Files:**
- Modify: `docs/superpowers/specs/2026-09-20-item-unification-design.md`（§8 PR 2 に完了、PR 3 に申し送りを 1 件）

**Interfaces:**
- Consumes: Task 1〜3
- Produces: PR 3 が乗る土台

- [ ] **Step 1: 新ポートが 6 本そろっていることを確認する**

```bash
cd internal/app/adapter/gateway/gh
grep -n 'func (g \*Gateway) \(GetItem\|ListItems\|AddComment\|SetState\|EditLabels\|EditAssignees\)' *.go
```

期待: **6 行**が出る。

- [ ] **Step 2: 旧ポートがまだ全部あることを確認する**

```bash
cd internal/app/adapter/gateway/gh
grep -c 'func (g \*Gateway) \(GetPR\|GetIssue\|ListPRs\|ListIssues\)' *.go | grep -v ':0'
grep -rn 'AddPRComment\|ClosePR\|EditPRLabels\|EditPRAssignees' ../../../usecase/*.go | grep -v _test | head
```

期待: gateway に `GetPR` `GetIssue` `ListPRs` `ListIssues` が残っており、
`usecase` が旧ポートを宣言したまま（PR 3 で差し替える）であること。
**この PR で消えたものが 1 つも無いこと**が要点である。

- [ ] **Step 3: 誰も新ポートを呼んでいないことを確認する**

```bash
grep -rn 'GetItem\|ListItems\|\.AddComment(\|\.SetState(\|\.EditLabels(\|\.EditAssignees(' internal/app/usecase internal/app/presentation cmd --include='*.go' | grep -v _test
```

期待: `usecase` の**自分自身のメソッド**（`Usecase.GetItem` など、旧ポートを呼ぶ既存の実装）
だけが出て、**gateway の新ポートを呼んでいる行が 1 つも無い**こと。
`usecase` の `GetItem` は自分の `itemFetcher`（＝旧 `GetPR`/`GetIssue`）を呼び続けている。

- [ ] **Step 4: 全体検査**

```bash
make check
make release-check
git status --porcelain -- '*testdata*'
git diff --stat main...HEAD -- internal/app/presentation cmd
```

期待: `make check` と `make release-check` が通る。
testdata の出力が空。**presentation と cmd の差分が 0 行**。

- [ ] **Step 5: 設計書を更新する**

`docs/superpowers/specs/2026-09-20-item-unification-design.md` の §8 で 2 箇所を直す。

1. 「### PR 2: `domain.Item` と新ポートの追加（旧は残す）」の成功条件の後ろに
   `**完了: 2026-09-20。**` を足す（PR 1 と同じ書き方）

2. 「### PR 3: usecase を新ポートへ」の申し送りに、次の 1 件を足す

```markdown
- **`ctx` を `backend` の書き込みメソッドに通す。** 新ポート 6 本は `ctx` を取るが、
  gateway の書き込み 4 本は受け取って使っていない（`writes.go` の `_ = ctx`）。
  `internal/github` の `AddPRComment` などが `ctx` を取らないためで、PR 2 の範囲外とした。
  書き残さないと `ctx` は飾りのまま残る
```

**他の節は変えない。**

- [ ] **Step 6: コミット**

```bash
git add docs/superpowers/specs/2026-09-20-item-unification-design.md
git commit -m "docs: mark PR 2 done and carry the ctx question to PR 3" \
  -m "Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## この PR の完了条件

- `domain.Item` と `domain.Change` が存在し、`tags_test.go` の `exported` が 24 個
- `ItemRef` の doc が「Item の同一性」を言っている
- gateway に新ポート 6 本があり、すべて `ctx` を第 1 引数に取る
- **不変条件 `Change != nil` ⟺ `Ref.Kind == ItemPR` にテストがある**
- 振り分け 4 本にテストがあり、間違った方を呼べばフェイクがパニックする形になっている
- `wrap` が文言を変えないことにテストがある
- **旧ポート 14 本と `domain.PR` / `domain.Issue` / `toPR` / `toIssue` がすべて残っている**
- **`usecase` / `presentation` / `cmd` の差分が 0 行**
- golden 386 枚と testdata が無変更
- `make check` と `make release-check` が通る

## 次の PR

PR 3（usecase を新ポートへ差し替え、`usecase.Item` を削除、`Kind` 分岐 5 本を削除）は、
この PR がマージされてから計画を書く。
