# Phase 3 merge 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 詳細ビューの `m` から、リポジトリが許した方式だけを並べたポップアップを開いて PR をマージできる。auto-merge の有効化と解除もそこから行える。

**Architecture:** `internal/gh` に「マージの可否とその理由」を表すドメイン型を足し、`internal/gh/cli` に 1 PR 分のマージ文脈を引く GraphQL と 3 つの mutation を置く。`internal/tui/merge` は `internal/tui/review` と同じ「詳細ビューが持つポップアップ」だが、**自分の取得を持つ**（`internal/tui/checks` と同じ形）。`r` で取り直せる必要があるからで、バックエンドには `internal/usecase` 越しにだけ触る。

**Tech Stack:** Go 1.27.1 / Bubble Tea v2（`charm.land/*/v2`）/ `internal/golden`（`OCTOSCOPE_UPDATE_GOLDEN=1`）/ `gh` CLI 2.100.0

**Spec:** `docs/superpowers/specs/2026-09-07-phase3-design.md`（§3 バックエンド / §4 境界 / §6 テスト / §8 完了条件 4・5・7・8）と `docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md` §4.4.4（画面はこちらが正）

## Global Constraints

- **`make check` が通らない状態でコミットしない。** 各タスクの最後は必ず `make check`
- **`internal/tui` は `internal/gh/cli` を import しない**（depguard が落とす）。TUI が要る操作はサブモデルが interface で宣言し、`internal/usecase` が実装する
- **テストでネットワークもサブプロセスも叩かない**（`.claude/rules/testing.md`）。`gh` の応答は `testdata` に実物を録って使う
- **テストは状態を直接組み立てず、`Update` にメッセージやキーを渡して到達させる**（`.claude/rules/testing.md`）
- **新しく書いたテストは、検証対象を一時的に壊して落ちることを確認してからコミットする**
- **`//nolint` を新しく足さない**（`.claude/rules/go-style.md`）
- **コードのコメントは英語。** 画面に出す文字列は `internal/i18n` から引き、`active.en.yaml` と `active.ja.yaml` の**両方**に足す
- **コメントは書きすぎない。** 外部の事情・そう書いた理由・doc コメントの 3 つだけ残す（`.claude/rules/go-style.md`）
- **`View` は副作用を持たず、時計も読まない**（`.claude/rules/tui.md`）
- **グリフを直接書かない**（`internal/tui/icon` から引く）。**色を直接書かない**（`internal/tui/theme` の役割名で引く）
- **GraphQL の綴りは `internal/gh/cli` に閉じる。** `MERGEABLE` / `BLOCKED` / `SQUASH` といった語が `internal/tui` に出てはならない
- ページング（各スレッドの `comments`）はこの計画に入れない。`docs/superpowers/plans/2026-09-08-phase3-comment-paging.md` にある

---

## Decisions（実装者はここを勝手に読み替えない。変えたくなったら止めて相談する）

### D1. 「ブランチを削除する」は表示のみ。トグルしない

spec §4.4.4 のモックアップは `[x] ブランチを削除する` を**トグルできる**ものとして描いているが、
**それは実装できない。** 2026-09-08 に `MergePullRequestInput` を introspection で実測した結果、
入力フィールドは `clientMutationId` / `pullRequestId` / `commitHeadline` / `commitBody` /
`expectedHeadOid` / `mergeMethod` / `authorEmail` の 7 つで、**ブランチ削除の入力は無い**。
削除はリポジトリの `deleteBranchOnMerge` に従って GitHub 側が行う。

したがってポップアップは `deleteBranchOnMerge` の値を**読み取り専用の 1 行**として見せる。
チェックボックスの見た目（`[x]` / `[ ]`）は使わず、カーソルも止まらない。押せる形に見せて
押せないほうが、押せないと分かる形より驚きが大きい。

`deleteRef` を足して自前で消す案は採らない（ユーザーの判断、2026-09-08）。
**spec §4.4.4 のこの箇条書きは誤りなので、Task 7 で spec 本文も直す。**

### D2. auto-merge は `mergeStateStatus` が `CLEAN` のとき選べない

GitHub は「今すぐマージできる」PR への `enablePullRequestAutoMerge` を拒否する。
spec §4.4.4 の「選べない項目を並べて押させてから断らない」に従い、`CLEAN` のときは
選択肢を落として理由を 1 行出す（ユーザーの判断、2026-09-08）。

`HAS_HOOKS` は落とさない。実測していないからで、GitHub が断ったらそのメッセージが出る。
**測ってから増やす。**

### D3. `mergeable: UNKNOWN` は `enter` を塞ぐ

spec §4.4.4 は「計算中と出して `r` で取り直す」とだけ書き、`enter` を塞ぐとは書いていない。
**塞ぐ。** マージ可能かどうか分かっていない状態で送るのは、確認 1 段だけという方針
（§4.4.4）と噛み合わない。理由の行は「計算中」で、`CONFLICTING` の「衝突しています」とは
別の文言にする。

### D4. `MergeStateStatus` に `DRAFT` は無い

2026-09-08 に実測した enum は `DIRTY` / `UNKNOWN` / `BLOCKED` / `BEHIND` / `UNSTABLE` /
`HAS_HOOKS` / `CLEAN` の 7 つ。draft の PR は `isDraft` で見分ける。`mergePullRequest` は
draft を断るので、**`isDraft` は `enter` を塞ぐ理由の 1 つとして扱う**（spec は draft に
触れていない）。

`UNSTABLE`（checks が落ちている / 走っている）は塞がない。GitHub 自身がマージを許す状態であり、
落ちた checks を承知でマージするのは普通の操作である。

### D5. ポップアップは自分で取得する。`internal/tui/review` ではなく `internal/tui/checks` に倣う

`internal/tui/review` は取得を持たず、詳細ビューが `fetchReviewContext` で先に引いてから
`review.New` に渡している。merge は `r` で取り直せる必要があり（D3）、取得を持ち主に置くと
「持ち主が取り直してポップアップを作り直す」形になって、選んだ方式が消える。
**取得はポップアップが持つ**（`internal/tui/checks` と同じ）。

### D6. auto-merge が既に有効なら、ポップアップは解除だけを提案する

spec §4.4.4 の「`autoMergeRequest` が既にあるときは、`m` は**解除**を提案する」を素直に読む
（ユーザーの判断、2026-09-08）。

- `AutoMergeEnabled` が真のとき、auto-merge の行はチェックボックスではなく
  「auto-merge は有効です。enter で解除します」の 1 行。`space` は効かない
- `enter` は `DisableAutoMerge` を送る。**`Block()` が塞いでいても送る**——
  マージできない PR でも待ち行列からは降りられるべきで、塞がれているのはマージであって
  解除ではない
- したがって `m.auto`（これから auto-merge にするか）は**常に false で開く**。
  `AutoMergeEnabled` が真のときは使われない

トグル式（`space` で `[x]` を外してから `enter`）は採らない。何も変えずに `enter` を押した
ときだけ無反応になる状態が増え、「無言のキーは壊れたキー」（`detail.go` の `stillLoading`）に
反する。

### D7. `usecase` に `MergeTarget` 型を作らない

`ReviewTarget` は「PR の id と未提出レビューの id」の 2 つが要ったから型になった。
merge が要るのは PR の node id 1 つだけである。**引数 1 つに型を被せない。**

---

## File Structure

| ファイル | 役割 |
|---|---|
| `internal/gh/merge.go`（新規） | `MergeMethod` / `Mergeable` / `MergeState` / `MergeBlock` / `MergeContext` とその判定 |
| `internal/gh/merge_test.go`（新規） | `Block()` と `CanAutoMerge()` の表 |
| `internal/gh/cli/merge.graphql`（新規） | リポジトリのマージ設定と PR のマージ可否を 1 リクエストで引く |
| `internal/gh/cli/merge_pr.graphql`（新規） | `mergePullRequest` |
| `internal/gh/cli/enable_auto_merge.graphql`（新規） | `enablePullRequestAutoMerge` |
| `internal/gh/cli/disable_auto_merge.graphql`（新規） | `disablePullRequestAutoMerge` |
| `internal/gh/cli/merge.go`（新規） | `PRMergeContext` / `MergePR` / `EnableAutoMerge` / `DisableAutoMerge` と enum の翻訳 |
| `internal/gh/cli/merge_test.go`（新規） | 応答の読み取りと mutation の引数 |
| `internal/gh/cli/testdata/merge_context.json`（新規） | `merge.graphql` の実応答 |
| `internal/gh/cli/testdata/schema.json`（録り直し） | `AutoMergeRequest` ほか新しい型 |
| `internal/gh/cli/testdata/README.md`（修正） | `merge_context.json` の録り方と型の追加 |
| `internal/gh/cli/schema_test.go`（修正） | 4 つの新しい document を `docs` に足す |
| `internal/usecase/usecase.go`（修正） | `merger` interface と 4 つの委譲メソッド |
| `internal/tui/merge/merge.go`（新規） | `Source` / `Model` / `Update` / キー処理 |
| `internal/tui/merge/render.go`（新規） | ポップアップの描画 |
| `internal/tui/merge/merge_test.go`（新規） | キーと状態遷移 |
| `internal/tui/merge/golden_test.go`（新規） | en / ja × 80 / 120 / 160 |
| `internal/tui/detail/detail.go` ほか（修正） | `m` キー、`modeMerge`、`merge.Model` の保持と描画 |
| `internal/tui/app/app.go`（修正） | `merge.MergedMsg` で Work と Repos を取り直す |
| `internal/i18n/locales/active.{en,ja}.yaml`（修正） | `merge.*` と `keybar.merge` |
| `docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md`（修正） | D1〜D4 に合わせて §4.4.4 を直す |

---

### Task 1: ドメイン型

**Files:**
- Create: `internal/gh/merge.go`
- Test: `internal/gh/merge_test.go`

**Interfaces:**
- Produces: `gh.MergeMethod`（`MergeSquash` / `MergeCommit` / `MergeRebase`）、`gh.Mergeable`（`MergeableUnknown` / `MergeableYes` / `MergeableConflicting`）、`gh.MergeState`（`MergeStateUnknown` / `MergeStateClean` / `MergeStateBlocked` / `MergeStateBehind` / `MergeStateDirty` / `MergeStateUnstable` / `MergeStateHasHooks`）、`gh.MergeBlock`（`BlockNone` / `BlockDraft` / `BlockConflicting` / `BlockComputing` / `BlockProtected` / `BlockBehind` / `BlockDirty`）、`gh.MergeContext`、`(gh.MergeContext).Block() MergeBlock`、`(gh.MergeContext).CanAutoMerge() bool`

- [ ] **Step 1: 失敗するテストを書く**

`internal/gh/merge_test.go`:

```go
package gh

import "testing"

func TestBlockNamesWhyMergingIsRefused(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ctx  MergeContext
		want MergeBlock
	}{
		{"clean", MergeContext{Mergeable: MergeableYes, State: MergeStateClean}, BlockNone},
		{"draft outranks the state GitHub reports for it",
			MergeContext{IsDraft: true, Mergeable: MergeableYes, State: MergeStateBlocked}, BlockDraft},
		{"conflicting", MergeContext{Mergeable: MergeableConflicting, State: MergeStateDirty}, BlockConflicting},
		{"still computing", MergeContext{Mergeable: MergeableUnknown, State: MergeStateUnknown}, BlockComputing},
		{"protected", MergeContext{Mergeable: MergeableYes, State: MergeStateBlocked}, BlockProtected},
		{"behind", MergeContext{Mergeable: MergeableYes, State: MergeStateBehind}, BlockBehind},
		{"dirty", MergeContext{Mergeable: MergeableYes, State: MergeStateDirty}, BlockDirty},
		{"failing checks do not block: GitHub allows the merge",
			MergeContext{Mergeable: MergeableYes, State: MergeStateUnstable}, BlockNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.ctx.Block(); got != tt.want {
				t.Errorf("Block() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAutoMergeNeedsSomethingToWaitFor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ctx  MergeContext
		want bool
	}{
		{"waiting on checks", MergeContext{
			AutoMergeAllowed: true, ViewerCanEnableAutoMerge: true, State: MergeStateUnstable,
		}, true},
		{"nothing to wait for: GitHub refuses auto-merge on a clean pull request", MergeContext{
			AutoMergeAllowed: true, ViewerCanEnableAutoMerge: true, State: MergeStateClean,
		}, false},
		{"repository has it turned off", MergeContext{
			AutoMergeAllowed: false, ViewerCanEnableAutoMerge: true, State: MergeStateUnstable,
		}, false},
		{"viewer may not", MergeContext{
			AutoMergeAllowed: true, ViewerCanEnableAutoMerge: false, State: MergeStateUnstable,
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.ctx.CanAutoMerge(); got != tt.want {
				t.Errorf("CanAutoMerge() = %v, want %v", got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: テストが落ちることを確かめる**

Run: `go test ./internal/gh/ -run 'TestBlock|TestAutoMerge'`
Expected: FAIL（`undefined: MergeContext`）

- [ ] **Step 3: 型を書く**

`internal/gh/merge.go`:

```go
package gh

// MergeMethod is how the pull request's commits land on the base branch.
type MergeMethod int

const (
	MergeSquash MergeMethod = iota
	MergeCommit
	MergeRebase
)

// Mergeable is GitHub's answer to "can this be merged". Unknown is an
// ordinary state, not a failure: GitHub returns it while it is still
// working the answer out.
type Mergeable int

const (
	MergeableUnknown Mergeable = iota
	MergeableYes
	MergeableConflicting
)

// MergeState is the finer answer, translated out of the GraphQL
// mergeStateStatus enum so the UI never switches on API spelling.
type MergeState int

const (
	MergeStateUnknown MergeState = iota
	MergeStateClean
	MergeStateBlocked
	MergeStateBehind
	MergeStateDirty
	MergeStateUnstable
	MergeStateHasHooks
)

// MergeBlock is why merging is refused right now. BlockNone means it is not.
type MergeBlock int

const (
	BlockNone MergeBlock = iota
	BlockDraft
	BlockConflicting
	BlockComputing
	BlockProtected
	BlockBehind
	BlockDirty
)

// MergeContext is everything the merge popup draws and acts on: what the
// repository allows, and what state this pull request is in.
type MergeContext struct {
	PullRequestID string
	IsDraft       bool
	Mergeable     Mergeable
	State         MergeState
	Review        ReviewState

	// Methods holds only the methods the repository allows, in the order
	// the popup lists them: squash, merge commit, rebase.
	Methods             []MergeMethod
	DeleteBranchOnMerge bool

	AutoMergeAllowed         bool
	ViewerCanEnableAutoMerge bool
	AutoMergeEnabled         bool
}

// Block says why merging is refused. Draft comes first: GitHub reports a
// draft as BLOCKED, and "it is a draft" is the more useful of the two.
func (c MergeContext) Block() MergeBlock {
	switch {
	case c.IsDraft:
		return BlockDraft
	case c.Mergeable == MergeableConflicting:
		return BlockConflicting
	case c.Mergeable == MergeableUnknown:
		return BlockComputing
	case c.State == MergeStateBlocked:
		return BlockProtected
	case c.State == MergeStateBehind:
		return BlockBehind
	case c.State == MergeStateDirty:
		return BlockDirty
	}
	return BlockNone
}

// CanAutoMerge reports whether auto-merge can be turned on. GitHub refuses
// it on a pull request that is already clean: there is nothing left to wait
// for, so it wants an ordinary merge instead.
func (c MergeContext) CanAutoMerge() bool {
	return c.AutoMergeAllowed && c.ViewerCanEnableAutoMerge && c.State != MergeStateClean
}
```

- [ ] **Step 4: テストが通ることを確かめる**

Run: `go test ./internal/gh/`
Expected: PASS

- [ ] **Step 5: テストが空振りでないことを確かめる**

`Block()` の `case c.IsDraft:` を消して `go test ./internal/gh/ -run TestBlock` が落ちること、
`CanAutoMerge()` の `&& c.State != MergeStateClean` を消して `-run TestAutoMerge` が落ちることを
見る。見たら戻す。

- [ ] **Step 6: `make check` とコミット**

```bash
make check
git add internal/gh/merge.go internal/gh/merge_test.go
git commit -m "feat: say whether a pull request can be merged, and why not"
```

---

### Task 2: マージ文脈を引く

**Files:**
- Create: `internal/gh/cli/merge.graphql`, `internal/gh/cli/merge.go`, `internal/gh/cli/merge_test.go`, `internal/gh/cli/testdata/merge_context.json`
- Modify: `internal/gh/cli/schema_test.go`, `internal/gh/cli/testdata/schema.json`, `internal/gh/cli/testdata/README.md`

**Interfaces:**
- Consumes: Task 1 の `gh.MergeContext` ほか
- Produces: `(*cli.Client).PRMergeContext(ctx context.Context, repo string, number int) (gh.MergeContext, error)`

**録り直しの順:** schema.json を先に録る。document のテストは schema.json に無い型を
「存在しないフィールド」ではなく `type X is not in testdata/schema.json` で落とすので、
先に足しておかないと Step 3 の失敗が読めない。

- [ ] **Step 1: schema.json に型を足して録り直す**

`internal/gh/cli/testdata/README.md` の `--argjson types` の配列に
`"AutoMergeRequest"`, `"MergePullRequestPayload"`, `"EnablePullRequestAutoMergePayload"`,
`"DisablePullRequestAutoMergePayload"` を足し、README のコマンドをそのまま実行して
`internal/gh/cli/testdata/schema.json` を録り直す。

- [ ] **Step 2: 失敗するテストを書く**

`internal/gh/cli/merge_test.go`:

```go
package cli

import (
	"testing"

	"github.com/kukv/octoscope/internal/gh"
)

func TestPRMergeContextReadsWhatTheRepositoryAllows(t *testing.T) {
	t.Parallel()

	c := &Client{dir: "/repo", repo: "kukv/octoscope", run: fileRun(t, "testdata/merge_context.json")}
	got, err := c.PRMergeContext(t.Context(), "", 61)
	if err != nil {
		t.Fatalf("PRMergeContext: %v", err)
	}
	if got.PullRequestID == "" {
		t.Error("PullRequestID is empty, want the node id the mutations take")
	}
	// Measured on 2026-09-08: kukv/octoscope has all three merge methods
	// on, deleteBranchOnMerge on, and autoMergeAllowed off. The recording
	// is what says the three flags are read into the order the popup lists
	// them in.
	want := []gh.MergeMethod{gh.MergeSquash, gh.MergeCommit, gh.MergeRebase}
	if len(got.Methods) != len(want) {
		t.Fatalf("Methods = %v, want %v", got.Methods, want)
	}
	for i := range want {
		if got.Methods[i] != want[i] {
			t.Fatalf("Methods = %v, want %v", got.Methods, want)
		}
	}
	if got.AutoMergeAllowed {
		t.Error("AutoMergeAllowed = true, want false (measured on kukv/octoscope, spec §2)")
	}
}

func TestPRMergeContextTranslatesTheEnums(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		mergeable     string
		state         string
		wantMergeable gh.Mergeable
		wantState     gh.MergeState
	}{
		{"clean", "MERGEABLE", "CLEAN", gh.MergeableYes, gh.MergeStateClean},
		{"conflicting", "CONFLICTING", "DIRTY", gh.MergeableConflicting, gh.MergeStateDirty},
		{"still computing", "UNKNOWN", "UNKNOWN", gh.MergeableUnknown, gh.MergeStateUnknown},
		{"failing checks", "MERGEABLE", "UNSTABLE", gh.MergeableYes, gh.MergeStateUnstable},
		{"protected", "MERGEABLE", "BLOCKED", gh.MergeableYes, gh.MergeStateBlocked},
		{"behind", "MERGEABLE", "BEHIND", gh.MergeableYes, gh.MergeStateBehind},
		{"hooks", "MERGEABLE", "HAS_HOOKS", gh.MergeableYes, gh.MergeStateHasHooks},
		{"a word we do not know is not a failure", "WAT", "WAT", gh.MergeableUnknown, gh.MergeStateUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			body := `{"data":{"repository":{"squashMergeAllowed":true,"mergeCommitAllowed":false,` +
				`"rebaseMergeAllowed":false,"deleteBranchOnMerge":true,"autoMergeAllowed":true,` +
				`"pullRequest":{"id":"PR_1","isDraft":false,"mergeable":"` + tt.mergeable + `",` +
				`"mergeStateStatus":"` + tt.state + `","reviewDecision":"APPROVED",` +
				`"viewerCanEnableAutoMerge":true,"autoMergeRequest":null}}}}`
			f := &fakeSeq{outs: []string{body}}
			c := &Client{dir: "/repo", repo: "kukv/octoscope", run: f.run}

			got, err := c.PRMergeContext(t.Context(), "", 61)
			if err != nil {
				t.Fatalf("PRMergeContext: %v", err)
			}
			if got.Mergeable != tt.wantMergeable {
				t.Errorf("Mergeable = %v, want %v", got.Mergeable, tt.wantMergeable)
			}
			if got.State != tt.wantState {
				t.Errorf("State = %v, want %v", got.State, tt.wantState)
			}
			if got.Review != gh.ReviewApproved {
				t.Errorf("Review = %v, want ReviewApproved", got.Review)
			}
			if !got.DeleteBranchOnMerge {
				t.Error("DeleteBranchOnMerge = false, want true")
			}
			if got.AutoMergeEnabled {
				t.Error("AutoMergeEnabled = true, want false (autoMergeRequest is null)")
			}
		})
	}
}

func TestPRMergeContextSeesAutoMergeAlreadyOn(t *testing.T) {
	t.Parallel()

	body := `{"data":{"repository":{"squashMergeAllowed":true,"mergeCommitAllowed":true,` +
		`"rebaseMergeAllowed":true,"deleteBranchOnMerge":false,"autoMergeAllowed":true,` +
		`"pullRequest":{"id":"PR_1","isDraft":false,"mergeable":"MERGEABLE",` +
		`"mergeStateStatus":"UNSTABLE","reviewDecision":"","viewerCanEnableAutoMerge":true,` +
		`"autoMergeRequest":{"enabledAt":"2026-09-08T01:00:00Z"}}}}}`
	f := &fakeSeq{outs: []string{body}}
	c := &Client{dir: "/repo", repo: "kukv/octoscope", run: f.run}

	got, err := c.PRMergeContext(t.Context(), "", 61)
	if err != nil {
		t.Fatalf("PRMergeContext: %v", err)
	}
	if !got.AutoMergeEnabled {
		t.Error("AutoMergeEnabled = false, want true: the popup offers to turn it off instead")
	}
}
```

- [ ] **Step 3: テストが落ちることを確かめる**

Run: `go test ./internal/gh/cli/ -run TestPRMergeContext`
Expected: FAIL（`c.PRMergeContext undefined`）

- [ ] **Step 4: クエリを書く**

`internal/gh/cli/merge.graphql`:

```graphql
query ($owner: String!, $name: String!, $number: Int!) {
  repository(owner: $owner, name: $name) {
    squashMergeAllowed
    mergeCommitAllowed
    rebaseMergeAllowed
    deleteBranchOnMerge
    autoMergeAllowed
    pullRequest(number: $number) {
      id
      isDraft
      mergeable
      mergeStateStatus
      reviewDecision
      viewerCanEnableAutoMerge
      # Only its presence is read: it is the difference between offering to
      # turn auto-merge on and offering to turn it off.
      autoMergeRequest {
        enabledAt
      }
    }
  }
}
```

- [ ] **Step 5: 実装を書く**

`internal/gh/cli/merge.go`:

```go
package cli

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/kukv/octoscope/internal/gh"
)

//go:embed merge.graphql
var mergeContextQuery string

type mergeContextResponse struct {
	Data struct {
		Repository struct {
			SquashMergeAllowed  bool `json:"squashMergeAllowed"`
			MergeCommitAllowed  bool `json:"mergeCommitAllowed"`
			RebaseMergeAllowed  bool `json:"rebaseMergeAllowed"`
			DeleteBranchOnMerge bool `json:"deleteBranchOnMerge"`
			AutoMergeAllowed    bool `json:"autoMergeAllowed"`
			PullRequest         struct {
				ID                       string `json:"id"`
				IsDraft                  bool   `json:"isDraft"`
				Mergeable                string `json:"mergeable"`
				MergeStateStatus         string `json:"mergeStateStatus"`
				ReviewDecision           string `json:"reviewDecision"`
				ViewerCanEnableAutoMerge bool   `json:"viewerCanEnableAutoMerge"`
				AutoMergeRequest         *struct {
					EnabledAt string `json:"enabledAt"`
				} `json:"autoMergeRequest"`
			} `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
}

// PRMergeContext fetches what the merge popup draws: what the repository
// allows and what state this pull request is in.
func (c *Client) PRMergeContext(ctx context.Context, repo string, number int) (gh.MergeContext, error) {
	repoFields, err := repoArgs(c.effectiveRepo(repo))
	if err != nil {
		return gh.MergeContext{}, err
	}
	args := append([]string{"api", "graphql", "-f", "query=" + mergeContextQuery}, repoFields...)
	args = append(args, "-F", "number="+strconv.Itoa(number))
	out, err := c.run(ctx, c.dir, args...)
	if err != nil {
		return gh.MergeContext{}, err
	}
	var resp mergeContextResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return gh.MergeContext{}, fmt.Errorf("parse merge context: %w", err)
	}
	r := resp.Data.Repository
	pr := r.PullRequest
	return gh.MergeContext{
		PullRequestID:            pr.ID,
		IsDraft:                  pr.IsDraft,
		Mergeable:                parseMergeable(pr.Mergeable),
		State:                    parseMergeState(pr.MergeStateStatus),
		Review:                   gh.ParseReviewDecision(pr.ReviewDecision),
		Methods:                  allowedMethods(r.SquashMergeAllowed, r.MergeCommitAllowed, r.RebaseMergeAllowed),
		DeleteBranchOnMerge:      r.DeleteBranchOnMerge,
		AutoMergeAllowed:         r.AutoMergeAllowed,
		ViewerCanEnableAutoMerge: pr.ViewerCanEnableAutoMerge,
		AutoMergeEnabled:         pr.AutoMergeRequest != nil,
	}, nil
}

// allowedMethods lists the methods in the order the popup draws them
// (standalone design §4.4.4): squash, merge commit, rebase.
func allowedMethods(squash, commit, rebase bool) []gh.MergeMethod {
	var methods []gh.MergeMethod
	if squash {
		methods = append(methods, gh.MergeSquash)
	}
	if commit {
		methods = append(methods, gh.MergeCommit)
	}
	if rebase {
		methods = append(methods, gh.MergeRebase)
	}
	return methods
}

// A value neither of these knows is read as "unknown" rather than failing
// the fetch: GitHub adds values to these enums.
func parseMergeable(s string) gh.Mergeable {
	switch s {
	case "MERGEABLE":
		return gh.MergeableYes
	case "CONFLICTING":
		return gh.MergeableConflicting
	}
	return gh.MergeableUnknown
}

func parseMergeState(s string) gh.MergeState {
	switch s {
	case "CLEAN":
		return gh.MergeStateClean
	case "BLOCKED":
		return gh.MergeStateBlocked
	case "BEHIND":
		return gh.MergeStateBehind
	case "DIRTY":
		return gh.MergeStateDirty
	case "UNSTABLE":
		return gh.MergeStateUnstable
	case "HAS_HOOKS":
		return gh.MergeStateHasHooks
	}
	return gh.MergeStateUnknown
}
```

- [ ] **Step 6: 実応答を録る**

```bash
D=internal/gh/cli/testdata
gh api graphql -F query=@internal/gh/cli/merge.graphql \
  -f owner=kukv -f name=octoscope -F number=61 | jq . > $D/merge_context.json
```

`$D/README.md` に `## merge_context.json` の節を足す（録った日・対象 PR・上のコマンド、
そして `autoMergeAllowed: false` がそのリポジトリの設定であること）。
61 が使えなければ開いている PR 番号に変え、README にその番号を書く。

- [ ] **Step 7: document を schema のテストに足す**

`internal/gh/cli/schema_test.go` の `docs` map に `"merge.graphql": mergeContextQuery,` を足す。

- [ ] **Step 8: テストが通ることを確かめる**

Run: `go test ./internal/gh/cli/`
Expected: PASS

- [ ] **Step 9: テストが空振りでないことを確かめる**

`parseMergeState` の `case "UNSTABLE":` を消して `-run TestPRMergeContext` が落ちること、
`merge.graphql` の `mergeStateStatus` を `mergeStateStatuss` に打ち間違えて
`-run TestEveryField` が落ちることを見る。見たら戻す。

- [ ] **Step 10: `make check` とコミット**

```bash
make check
git add internal/gh/cli/merge.graphql internal/gh/cli/merge.go internal/gh/cli/merge_test.go internal/gh/cli/schema_test.go internal/gh/cli/testdata
git commit -m "feat: fetch what a pull request needs to be merged"
```

---

### Task 3: 3 つの mutation

**Files:**
- Create: `internal/gh/cli/merge_pr.graphql`, `internal/gh/cli/enable_auto_merge.graphql`, `internal/gh/cli/disable_auto_merge.graphql`
- Modify: `internal/gh/cli/merge.go`, `internal/gh/cli/merge_test.go`, `internal/gh/cli/schema_test.go`

**Interfaces:**
- Produces: `(*cli.Client).MergePR(pullRequestID string, method gh.MergeMethod) error`、`(*cli.Client).EnableAutoMerge(pullRequestID string, method gh.MergeMethod) error`、`(*cli.Client).DisableAutoMerge(pullRequestID string) error`

3 つとも `context.Context` を取らない。**変更であって取得ではない**（`internal/gh/cli/review.go`
の 5 つの mutation と同じ理由。`.claude/rules/go-style.md`）。

- [ ] **Step 1: 失敗するテストを書く**

`internal/gh/cli/merge_test.go` に足す（import に `"slices"` と `"strings"` を足す）:

```go
func TestMergePRSendsTheMethodTheUserChose(t *testing.T) {
	t.Parallel()

	tests := []struct {
		method gh.MergeMethod
		want   string
	}{
		{gh.MergeSquash, "mergeMethod=SQUASH"},
		{gh.MergeCommit, "mergeMethod=MERGE"},
		{gh.MergeRebase, "mergeMethod=REBASE"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()

			f := &fakeSeq{outs: []string{`{"data":{"mergePullRequest":{"pullRequest":{"merged":true}}}}`}}
			c := &Client{dir: "/repo", repo: "kukv/octoscope", run: f.run}
			if err := c.MergePR("PR_1", tt.method); err != nil {
				t.Fatalf("MergePR: %v", err)
			}
			if !slices.Contains(f.calls[0], tt.want) {
				t.Errorf("call = %v, want it to carry %s", f.calls[0], tt.want)
			}
			if !slices.Contains(f.calls[0], "pullRequestId=PR_1") {
				t.Errorf("call = %v, want it to carry pullRequestId=PR_1", f.calls[0])
			}
		})
	}
}

func TestAutoMergeIsTurnedOnWithAMethodAndOffWithout(t *testing.T) {
	t.Parallel()

	on := &fakeSeq{outs: []string{`{"data":{"enablePullRequestAutoMerge":{"clientMutationId":null}}}`}}
	c := &Client{dir: "/repo", repo: "kukv/octoscope", run: on.run}
	if err := c.EnableAutoMerge("PR_1", gh.MergeRebase); err != nil {
		t.Fatalf("EnableAutoMerge: %v", err)
	}
	if !slices.Contains(on.calls[0], "mergeMethod=REBASE") {
		t.Errorf("call = %v, want it to carry mergeMethod=REBASE", on.calls[0])
	}

	off := &fakeSeq{outs: []string{`{"data":{"disablePullRequestAutoMerge":{"clientMutationId":null}}}`}}
	c = &Client{dir: "/repo", repo: "kukv/octoscope", run: off.run}
	if err := c.DisableAutoMerge("PR_1"); err != nil {
		t.Fatalf("DisableAutoMerge: %v", err)
	}
	// Turning it off takes the pull request and nothing else: the method
	// belongs to the request being cancelled, not to the cancellation.
	for _, arg := range off.calls[0] {
		if strings.HasPrefix(arg, "mergeMethod=") {
			t.Errorf("call = %v, want no mergeMethod", off.calls[0])
		}
	}
}
```

- [ ] **Step 2: テストが落ちることを確かめる**

Run: `go test ./internal/gh/cli/ -run 'TestMergePR|TestAutoMergeIs'`
Expected: FAIL（`c.MergePR undefined`）

- [ ] **Step 3: mutation を書く**

`internal/gh/cli/merge_pr.graphql`:

```graphql
mutation ($pullRequestId: ID!, $mergeMethod: PullRequestMergeMethod!) {
  mergePullRequest(input: {pullRequestId: $pullRequestId, mergeMethod: $mergeMethod}) {
    pullRequest {
      merged
    }
  }
}
```

`internal/gh/cli/enable_auto_merge.graphql`:

```graphql
mutation ($pullRequestId: ID!, $mergeMethod: PullRequestMergeMethod!) {
  enablePullRequestAutoMerge(input: {pullRequestId: $pullRequestId, mergeMethod: $mergeMethod}) {
    clientMutationId
  }
}
```

`internal/gh/cli/disable_auto_merge.graphql`:

```graphql
mutation ($pullRequestId: ID!) {
  disablePullRequestAutoMerge(input: {pullRequestId: $pullRequestId}) {
    clientMutationId
  }
}
```

- [ ] **Step 4: 実装を書く**

`internal/gh/cli/merge.go` に足す:

```go
//go:embed merge_pr.graphql
var mergePRMutation string

//go:embed enable_auto_merge.graphql
var enableAutoMergeMutation string

//go:embed disable_auto_merge.graphql
var disableAutoMergeMutation string

// apiMergeMethod spells a method the way the GraphQL PullRequestMergeMethod
// enum does. It is the one place that knows those words
// (.claude/rules/architecture.md).
func apiMergeMethod(m gh.MergeMethod) string {
	switch m {
	case gh.MergeCommit:
		return "MERGE"
	case gh.MergeRebase:
		return "REBASE"
	default:
		return "SQUASH"
	}
}

// The three mutations take no context, for the same reason review.go's do:
// a merge that has happened has happened.

// MergePR merges the pull request now.
func (c *Client) MergePR(pullRequestID string, method gh.MergeMethod) error {
	_, err := c.run(context.Background(), c.dir, "api", "graphql",
		"-f", "query="+mergePRMutation,
		"-f", "pullRequestId="+pullRequestID,
		"-f", "mergeMethod="+apiMergeMethod(method),
	)
	return err
}

// EnableAutoMerge asks GitHub to merge the pull request once what it is
// waiting on is in.
func (c *Client) EnableAutoMerge(pullRequestID string, method gh.MergeMethod) error {
	_, err := c.run(context.Background(), c.dir, "api", "graphql",
		"-f", "query="+enableAutoMergeMutation,
		"-f", "pullRequestId="+pullRequestID,
		"-f", "mergeMethod="+apiMergeMethod(method),
	)
	return err
}

// DisableAutoMerge cancels a queued auto-merge.
func (c *Client) DisableAutoMerge(pullRequestID string) error {
	_, err := c.run(context.Background(), c.dir, "api", "graphql",
		"-f", "query="+disableAutoMergeMutation,
		"-f", "pullRequestId="+pullRequestID,
	)
	return err
}
```

- [ ] **Step 5: document を schema のテストに足す**

`schema_test.go` の `docs` map に足す:

```go
"merge_pr.graphql":           mergePRMutation,
"enable_auto_merge.graphql":  enableAutoMergeMutation,
"disable_auto_merge.graphql": disableAutoMergeMutation,
```

- [ ] **Step 6: テストが通ることを確かめる**

Run: `go test ./internal/gh/cli/`
Expected: PASS

- [ ] **Step 7: テストが空振りでないことを確かめる**

`apiMergeMethod` の `case gh.MergeRebase:` を消して `-run 'TestMergePR|TestAutoMergeIs'` が
落ちることを見る。見たら戻す。

- [ ] **Step 8: `make check` とコミット**

```bash
make check
git add internal/gh/cli
git commit -m "feat: merge a pull request, and queue one behind its checks"
```

---

### Task 4: usecase の口

**Files:**
- Modify: `internal/usecase/usecase.go`
- Test: `internal/usecase/usecase_test.go`

**Interfaces:**
- Produces: `(*usecase.Usecase).PRMergeContext(ctx context.Context, repo string, number int) (gh.MergeContext, error)`、`(*usecase.Usecase).MergePR(pullRequestID string, method gh.MergeMethod) error`、`(*usecase.Usecase).EnableAutoMerge(pullRequestID string, method gh.MergeMethod) error`、`(*usecase.Usecase).DisableAutoMerge(pullRequestID string) error`

- [ ] **Step 1: interface と委譲を書く**

`internal/usecase/usecase.go` の `checksFetcher` の隣に:

```go
type merger interface {
	PRMergeContext(ctx context.Context, repo string, number int) (gh.MergeContext, error)
	MergePR(pullRequestID string, method gh.MergeMethod) error
	EnableAutoMerge(pullRequestID string, method gh.MergeMethod) error
	DisableAutoMerge(pullRequestID string) error
}
```

`source` に `merger` を、`Usecase` に `merges merger` を、`New` に `merges: src,` を足し、
委譲を 4 つ書く:

```go
func (u *Usecase) PRMergeContext(ctx context.Context, repo string, number int) (gh.MergeContext, error) {
	return u.merges.PRMergeContext(ctx, repo, number)
}

func (u *Usecase) MergePR(pullRequestID string, method gh.MergeMethod) error {
	return u.merges.MergePR(pullRequestID, method)
}

func (u *Usecase) EnableAutoMerge(pullRequestID string, method gh.MergeMethod) error {
	return u.merges.EnableAutoMerge(pullRequestID, method)
}

func (u *Usecase) DisableAutoMerge(pullRequestID string) error {
	return u.merges.DisableAutoMerge(pullRequestID)
}
```

- [ ] **Step 2: 既存のテスト用 fake を直す**

`internal/usecase/usecase_test.go` の fake が `source` を満たさなくなってコンパイルが落ちる。
4 つのメソッドを足し、**受け取った引数を記録する**（呼ばれたことだけを見る形にしない）。
記録するフィールドは `mergedID string` / `mergedMethod gh.MergeMethod`。

- [ ] **Step 3: 委譲のテストを書く**

```go
func TestMergePRPassesTheMethodThrough(t *testing.T) {
	t.Parallel()

	f := &fakeSource{} // 既存のテストが使っている fake 型名に合わせる
	u := New(f)
	if err := u.MergePR("PR_1", gh.MergeRebase); err != nil {
		t.Fatalf("MergePR: %v", err)
	}
	if f.mergedID != "PR_1" || f.mergedMethod != gh.MergeRebase {
		t.Errorf("merged (%q, %v), want (%q, %v)", f.mergedID, f.mergedMethod, "PR_1", gh.MergeRebase)
	}
}
```

- [ ] **Step 4: テストが通ることを確かめる**

Run: `go test ./internal/usecase/`
Expected: PASS

- [ ] **Step 5: テストが空振りでないことを確かめる**

委譲を `return u.merges.MergePR(pullRequestID, gh.MergeSquash)` に変えてテストが落ちることを
見る。見たら戻す。

- [ ] **Step 6: `make check` とコミット**

```bash
make check
git add internal/usecase
git commit -m "feat: open the merge calls to the views"
```

---

### Task 5: ポップアップの状態とキー

**Files:**
- Create: `internal/tui/merge/merge.go`, `internal/tui/merge/merge_test.go`

**Interfaces:**
- Consumes: `gh.MergeContext` / `gh.MergeMethod` / `gh.MergeBlock`
- Produces: `merge.Source`、`merge.Model`、`merge.New(src Source, ref gh.ItemRef) Model`、`(Model).Init() tea.Cmd`、`(Model).Update(tea.Msg) (Model, tea.Cmd)`、`(Model).View() string`、`(Model).Active() bool`、`merge.MergedMsg{}`、`merge.CancelledMsg{}`、`merge.ErrorMsg{Err error}`

**キー割り当て**（spec §4.4.4 の「確認はこのポップアップ 1 段だけ」に従う）:

| キー | すること |
|---|---|
| `j` / `down`, `k` / `up` | 方式を選ぶ（`ctx.Methods` の中だけを動く） |
| `space` | auto-merge のトグル（`CanAutoMerge()` のときだけ。既に有効なら効かない。D6） |
| `enter` | 実行（塞がれているときは何もしない） |
| `r` | 取り直す |
| `esc` | 閉じる（`CancelledMsg`） |

**`enter` が何を送るか**（`m.auto` は常に false で開く。D6）:

| 今の auto-merge | `m.auto` | 送るもの |
|---|---|---|
| 有効 | — | `DisableAutoMerge`。**`Block()` に関わらず送る**（マージできない PR でも待ち行列は降りられる） |
| 無効 | true | `EnableAutoMerge`（`Block() == BlockNone` のときだけ） |
| 無効 | false | `MergePR`（`Block() == BlockNone` のときだけ） |

- [ ] **Step 1: 失敗するテストを書く**

`internal/tui/merge/merge_test.go`:

```go
package merge

import (
	"context"
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kukv/octoscope/internal/gh"
)

type fakeSource struct {
	ctx     gh.MergeContext
	err     error
	merged  []gh.MergeMethod
	enabled []gh.MergeMethod
	offCall int
}

func (f *fakeSource) PRMergeContext(context.Context, string, int) (gh.MergeContext, error) {
	return f.ctx, f.err
}

func (f *fakeSource) MergePR(_ string, m gh.MergeMethod) error {
	f.merged = append(f.merged, m)
	return nil
}

func (f *fakeSource) EnableAutoMerge(_ string, m gh.MergeMethod) error {
	f.enabled = append(f.enabled, m)
	return nil
}

func (f *fakeSource) DisableAutoMerge(string) error {
	f.offCall++
	return nil
}

func ref() gh.ItemRef {
	return gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/octoscope", Number: 61}
}

// loaded runs New's fetch and hands the answer back, the way Bubble Tea
// would: the test never builds the loaded Model by hand.
func loaded(t *testing.T, f *fakeSource) Model {
	t.Helper()

	m := New(f, ref())
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init returned no command: nothing fetches the merge context")
	}
	m, _ = m.Update(cmd())
	return m
}

func press(m Model, key string) (Model, tea.Cmd) {
	return m.Update(tea.KeyPressMsg{Code: []rune(key)[0], Text: key})
}

func enter(m Model) (Model, tea.Cmd) {
	return m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
}

func mergeable() gh.MergeContext {
	return gh.MergeContext{
		PullRequestID: "PR_1",
		Mergeable:     gh.MergeableYes,
		State:         gh.MergeStateUnstable,
		Methods:       []gh.MergeMethod{gh.MergeSquash, gh.MergeCommit, gh.MergeRebase},
	}
}

func TestEnterMergesWithTheMethodOnTheCursor(t *testing.T) {
	t.Parallel()

	f := &fakeSource{ctx: mergeable()}
	m := loaded(t, f)
	m, _ = press(m, "j")
	_, cmd := enter(m)
	if cmd == nil {
		t.Fatal("enter returned no command: nothing was sent")
	}
	if msg := cmd(); msg != (MergedMsg{}) {
		t.Fatalf("cmd() = %#v, want MergedMsg{}", msg)
	}
	if len(f.merged) != 1 || f.merged[0] != gh.MergeCommit {
		t.Errorf("merged = %v, want one MergeCommit (j moved off squash)", f.merged)
	}
}

func TestTheCursorStaysInsideWhatTheRepositoryAllows(t *testing.T) {
	t.Parallel()

	only := mergeable()
	only.Methods = []gh.MergeMethod{gh.MergeRebase}
	f := &fakeSource{ctx: only}
	m := loaded(t, f)
	m, _ = press(m, "j")
	m, _ = press(m, "j")
	_, cmd := enter(m)
	_ = cmd()
	if len(f.merged) != 1 || f.merged[0] != gh.MergeRebase {
		t.Errorf("merged = %v, want one MergeRebase: j must not walk past the only method", f.merged)
	}
}

func TestEnterIsRefusedWhileGitHubIsStillWorkingItOut(t *testing.T) {
	t.Parallel()

	computing := mergeable()
	computing.Mergeable = gh.MergeableUnknown
	computing.State = gh.MergeStateUnknown
	f := &fakeSource{ctx: computing}
	m := loaded(t, f)
	_, cmd := enter(m)
	if cmd != nil {
		t.Fatal("enter sent something while mergeable was UNKNOWN")
	}
	if len(f.merged) != 0 {
		t.Errorf("merged = %v, want nothing", f.merged)
	}
}

func TestSpaceQueuesTheMergeBehindTheChecks(t *testing.T) {
	t.Parallel()

	auto := mergeable()
	auto.AutoMergeAllowed = true
	auto.ViewerCanEnableAutoMerge = true
	f := &fakeSource{ctx: auto}
	m := loaded(t, f)
	m, _ = press(m, " ")
	_, cmd := enter(m)
	if msg := cmd(); msg != (MergedMsg{}) {
		t.Fatalf("cmd() = %#v, want MergedMsg{}", msg)
	}
	if len(f.enabled) != 1 {
		t.Fatalf("enabled = %v, want one call: space chose auto-merge", f.enabled)
	}
	if len(f.merged) != 0 {
		t.Errorf("merged = %v, want nothing: auto-merge does not merge now", f.merged)
	}
}

func TestAutoMergeCannotBeChosenOnAPullRequestWithNothingToWaitFor(t *testing.T) {
	t.Parallel()

	clean := mergeable()
	clean.State = gh.MergeStateClean
	clean.AutoMergeAllowed = true
	clean.ViewerCanEnableAutoMerge = true
	f := &fakeSource{ctx: clean}
	m := loaded(t, f)
	m, _ = press(m, " ")
	_, cmd := enter(m)
	_ = cmd()
	if len(f.enabled) != 0 {
		t.Errorf("enabled = %v, want nothing: GitHub refuses auto-merge on a clean pull request", f.enabled)
	}
	if len(f.merged) != 1 {
		t.Errorf("merged = %v, want one: enter still merges", f.merged)
	}
}

func TestEnterCancelsAnAutoMergeThatIsAlreadyOn(t *testing.T) {
	t.Parallel()

	on := mergeable()
	on.AutoMergeAllowed = true
	on.ViewerCanEnableAutoMerge = true
	on.AutoMergeEnabled = true
	f := &fakeSource{ctx: on}
	m := loaded(t, f)
	_, cmd := enter(m)
	if msg := cmd(); msg != (MergedMsg{}) {
		t.Fatalf("cmd() = %#v, want MergedMsg{}", msg)
	}
	if f.offCall != 1 {
		t.Errorf("DisableAutoMerge calls = %d, want 1", f.offCall)
	}
	if len(f.merged) != 0 {
		t.Errorf("merged = %v, want nothing: the popup offers the cancellation, not the merge", f.merged)
	}
}

func TestAQueuedMergeCanBeCancelledEvenWhenMergingIsBlocked(t *testing.T) {
	t.Parallel()

	stuck := mergeable()
	stuck.Mergeable = gh.MergeableConflicting
	stuck.AutoMergeAllowed = true
	stuck.ViewerCanEnableAutoMerge = true
	stuck.AutoMergeEnabled = true
	f := &fakeSource{ctx: stuck}
	m := loaded(t, f)
	_, cmd := enter(m)
	if cmd == nil {
		t.Fatal("enter sent nothing: leaving the queue is not the merge that is blocked")
	}
	_ = cmd()
	if f.offCall != 1 {
		t.Errorf("DisableAutoMerge calls = %d, want 1", f.offCall)
	}
}

func TestSpaceDoesNothingWhenAutoMergeIsAlreadyOn(t *testing.T) {
	t.Parallel()

	on := mergeable()
	on.AutoMergeAllowed = true
	on.ViewerCanEnableAutoMerge = true
	on.AutoMergeEnabled = true
	f := &fakeSource{ctx: on}
	m := loaded(t, f)
	m, _ = press(m, " ")
	_, cmd := enter(m)
	_ = cmd()
	if f.offCall != 1 {
		t.Errorf("DisableAutoMerge calls = %d, want 1: space must not turn enter into something else", f.offCall)
	}
}

func TestRFetchesAgain(t *testing.T) {
	t.Parallel()

	f := &fakeSource{ctx: mergeable()}
	m := loaded(t, f)
	_, cmd := press(m, "r")
	if cmd == nil {
		t.Fatal("r returned no command: nothing refetched")
	}
	if _, ok := cmd().(contextMsg); !ok {
		t.Errorf("cmd() = %T, want contextMsg", cmd())
	}
}

func TestAFetchThatFailsIsHandedToTheHolder(t *testing.T) {
	t.Parallel()

	f := &fakeSource{err: errors.New("gh: not found")}
	m := New(f, ref())
	msg := m.Init()()
	e, ok := msg.(ErrorMsg)
	if !ok || e.Err == nil {
		t.Fatalf("Init()() = %#v, want an ErrorMsg carrying the failure", msg)
	}
}
```

- [ ] **Step 2: テストが落ちることを確かめる**

Run: `go test ./internal/tui/merge/`
Expected: FAIL（パッケージが無い）

- [ ] **Step 3: モデルを書く**

`internal/tui/merge/merge.go`:

```go
// Package merge is the popup that merges a pull request: which method the
// commits land by, whether to wait for the checks, and the one enter that
// sends it.
//
// It is a popup rather than a view of its own, so it has no place in the
// root model's stack. The detail view holds one and draws it over itself.
// Unlike internal/tui/review it fetches for itself: r has to be able to ask
// GitHub again while the popup stays open (standalone design §4.4.4).
package merge

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/kukv/octoscope/internal/gh"
)

// Source is what merging needs.
type Source interface {
	PRMergeContext(ctx context.Context, repo string, number int) (gh.MergeContext, error)
	MergePR(pullRequestID string, method gh.MergeMethod) error
	EnableAutoMerge(pullRequestID string, method gh.MergeMethod) error
	DisableAutoMerge(pullRequestID string) error
}

// MergedMsg tells the holder the pull request was merged, queued, or taken
// out of the queue: whichever it was, what is on screen is now stale.
type MergedMsg struct{}

// CancelledMsg tells the holder to take the popup away.
type CancelledMsg struct{}

// ErrorMsg carries a failure the holder shows at footer level, the same way
// internal/tui/review hands its failures up (.claude/rules/errors.md).
type ErrorMsg struct{ Err error }

// contextMsg carries the fetch's answer for the pull request it was sent
// for; an answer for another one is dropped, the way the checks view drops
// a log that arrives after the user has moved on.
type contextMsg struct {
	ref gh.ItemRef
	ctx gh.MergeContext
}

// boxWidth is the popup's own cap, the same one internal/tui/review uses so
// the two line up when they open over the same detail view.
const boxWidth = 50

type Model struct {
	src Source
	ref gh.ItemRef

	ctx     gh.MergeContext
	loading bool
	row     int
	auto    bool
	sending bool

	width, height int
}

// New builds the popup. Init is what starts the fetch.
func New(src Source, ref gh.ItemRef) Model {
	return Model{src: src, ref: ref, loading: true}
}

// Active reports whether the popup has anything to show. A zero Model,
// before New has built it, reports false.
func (m Model) Active() bool { return m.ref.Number != 0 }

func (m Model) Init() tea.Cmd { return fetch(m.src, m.ref) }

func fetch(src Source, ref gh.ItemRef) tea.Cmd {
	return func() tea.Msg {
		c, err := src.PRMergeContext(context.Background(), ref.Repo, ref.Number)
		if err != nil {
			return ErrorMsg{Err: err}
		}
		return contextMsg{ref: ref, ctx: c}
	}
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case contextMsg:
		if msg.ref != m.ref {
			return m, nil
		}
		m.ctx = msg.ctx
		m.loading = false
		m.row = 0
		// auto is what enter would turn on. On a pull request that already
		// has an auto-merge, enter cancels it instead and auto is unused
		// (standalone design §4.4.4).
		m.auto = false
		return m, nil
	case ErrorMsg:
		m.loading = false
		m.sending = false
		return m, nil
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if m.sending {
		return m, nil // ignore every other key while the merge is in flight
	}
	switch msg.String() {
	case "esc":
		return m, func() tea.Msg { return CancelledMsg{} }
	case "r":
		m.loading = true
		return m, fetch(m.src, m.ref)
	}
	if m.loading {
		return m, nil
	}
	switch msg.String() {
	case "j", "down":
		if m.row < len(m.ctx.Methods)-1 {
			m.row++
		}
		return m, nil
	case "k", "up":
		if m.row > 0 {
			m.row--
		}
		return m, nil
	case " ", "space":
		if m.ctx.CanAutoMerge() && !m.ctx.AutoMergeEnabled {
			m.auto = !m.auto
		}
		return m, nil
	case "enter":
		return m.send()
	}
	return m, nil
}

// send does the one thing the popup is for. Leaving the auto-merge queue is
// the one action offered while merging itself is blocked: a pull request
// that cannot be merged is no reason to be stuck with a queued merge.
func (m Model) send() (Model, tea.Cmd) {
	switch {
	case m.ctx.AutoMergeEnabled:
		return m.sendCmd(func() error { return m.src.DisableAutoMerge(m.ctx.PullRequestID) })
	case m.ctx.Block() != gh.BlockNone:
		return m, nil
	case m.auto:
		method := m.method()
		return m.sendCmd(func() error { return m.src.EnableAutoMerge(m.ctx.PullRequestID, method) })
	default:
		method := m.method()
		return m.sendCmd(func() error { return m.src.MergePR(m.ctx.PullRequestID, method) })
	}
}

// method is the one on the cursor. A repository with no method allowed at
// all cannot happen -- GitHub refuses to turn the last one off -- so the
// fallback is only there to keep the index safe.
func (m Model) method() gh.MergeMethod {
	if m.row < len(m.ctx.Methods) {
		return m.ctx.Methods[m.row]
	}
	return gh.MergeSquash
}

func (m Model) sendCmd(do func() error) (Model, tea.Cmd) {
	m.sending = true
	return m, func() tea.Msg {
		if err := do(); err != nil {
			return ErrorMsg{Err: err}
		}
		return MergedMsg{}
	}
}
```

**`space` は `CanAutoMerge()` が偽のとき `m.auto` を動かさないので、`m.auto` が真になるのは
auto-merge を選べるときだけである。** `send()` の `case m.auto:` が `CanAutoMerge()` を
見直さないのはそのためで、`TestAutoMergeCannotBeChosenOnAPullRequestWithNothingToWaitFor` が
それを見張っている。

- [ ] **Step 4: テストが通ることを確かめる**

Run: `go test ./internal/tui/merge/`
Expected: PASS

- [ ] **Step 5: テストが空振りでないことを確かめる**

4 つ壊して、それぞれ落ちることを見る。見たら戻す。

1. `j` の境界を `m.row < 99` に緩める → `TestTheCursorStaysInside...` が落ちる
2. `send()` の `case m.ctx.Block() != gh.BlockNone:` を消す → `TestEnterIsRefusedWhile...` が落ちる
3. `space` の `if m.ctx.CanAutoMerge() && !m.ctx.AutoMergeEnabled` を外す →
   `TestAutoMergeCannotBeChosen...` が落ちる
4. `send()` の `case m.ctx.AutoMergeEnabled:` を `case m.ctx.AutoMergeEnabled && m.auto:` に
   狭める → `TestEnterCancelsAnAutoMergeThatIsAlreadyOn` が落ちる

- [ ] **Step 6: `make check` とコミット**

```bash
make check
git add internal/tui/merge
git commit -m "feat: choose how a pull request lands"
```

---

### Task 6: ポップアップの描画と i18n と golden

**Files:**
- Create: `internal/tui/merge/render.go`, `internal/tui/merge/golden_test.go`
- Modify: `internal/i18n/locales/active.en.yaml`, `internal/i18n/locales/active.ja.yaml`

**画面**（standalone design §4.4.4。D1 により削除の行は読み取り専用）:

```
┌─ Merge #61 ──────────────────────────────────┐
│ ● squash してマージ                          │
│ ○ マージコミットを作る                       │
│ ○ rebase してマージ                          │
│                                              │
│ [ ] auto-merge（checks の通過後に）          │
│ ブランチはマージ後に削除されます             │
│                                              │
│ ⚠ レビューが承認されていません               │
│ enter:マージ space:auto r:更新 esc:中止      │
└──────────────────────────────────────────────┘
```

**足す i18n キー**（`merge:` の下、en / ja 両方。`keybar.merge` だけ既存の `keybar:` の下）:

| キー | en | ja |
|---|---|---|
| `merge.title` | `Merge #{{.Number}}` | `#{{.Number}} をマージ` |
| `merge.loading` | `Asking GitHub` | `GitHub に問い合わせ中` |
| `merge.method_squash` | `Squash and merge` | `squash してマージ` |
| `merge.method_commit` | `Create a merge commit` | `マージコミットを作る` |
| `merge.method_rebase` | `Rebase and merge` | `rebase してマージ` |
| `merge.auto` | `auto-merge (after the checks pass)` | `auto-merge（checks の通過後に）` |
| `merge.auto_on` | `auto-merge is on; enter cancels it` | `auto-merge は有効です。enter で解除します` |
| `merge.auto_unavailable_repo` | `auto-merge is off for this repository` | `このリポジトリでは auto-merge が無効です` |
| `merge.auto_unavailable_clean` | `auto-merge has nothing left to wait for` | `auto-merge が待つものはありません` |
| `merge.delete_branch_on` | `the branch is deleted after the merge` | `ブランチはマージ後に削除されます` |
| `merge.delete_branch_off` | `the branch is kept` | `ブランチは残ります` |
| `merge.block_draft` | `this pull request is a draft` | `この PR はまだ draft です` |
| `merge.block_conflicting` | `it conflicts with the base branch` | `ベースブランチと衝突しています` |
| `merge.block_computing` | `GitHub is still working out whether it can be merged` | `GitHub がマージ可能かを計算中です` |
| `merge.block_protected` | `a branch rule is holding it` | `保護ルールに止められています` |
| `merge.block_behind` | `it is behind the base branch` | `ベースブランチに追いついていません` |
| `merge.block_dirty` | `GitHub cannot merge it as it stands` | `このままではマージできません` |
| `merge.review_required` | `it has not been approved` | `レビューが承認されていません` |
| `merge.review_changes` | `changes have been requested` | `変更が要求されています` |
| `merge.key_merge` | `enter:merge` | `enter:マージ` |
| `merge.key_auto` | `space:auto` | `space:auto` |
| `merge.key_refresh` | `r:refresh` | `r:更新` |
| `merge.key_cancel` | `esc:cancel` | `esc:中止` |
| `keybar.merge` | `m:merge` | `m:マージ` |

- [ ] **Step 1: 描画を書く**

`internal/tui/merge/render.go` に `func (m Model) View() string` を置く。枠の組み立ては
`internal/tui/review/render.go` の描き方をそのまま踏襲する。**枠の文字は `internal/tui/icon`、
色は `internal/tui/theme` の役割名から引き、直書きしない。**

- 方式の行は `m.ctx.Methods` を順に描き、カーソルの行だけ選択済みの丸にする
- auto-merge の行は 3 通り（D6）:
  - `AutoMergeEnabled` が真 → `merge.auto_on` の 1 行。チェックボックスにしない
  - `CanAutoMerge()` が真 → チェックボックス（`m.auto` で `[x]` / `[ ]`）と `merge.auto`
  - どちらでもない → 理由を 1 行。`AutoMergeAllowed` が偽なら
    `merge.auto_unavailable_repo`、そうでなければ `merge.auto_unavailable_clean`
- 削除の行は `DeleteBranchOnMerge` に応じて `merge.delete_branch_on` / `_off`。
  **印は付けず、カーソルも止まらない**（D1）
- `Block() != gh.BlockNone` なら理由を `⚠` 付きで 1 行。`BlockNone` のときは
  `Review` が `ReviewRequired` / `ReviewChangesRequested` なら `merge.review_*` を出す
  （spec §4.4.4 のモックアップの `⚠` の行）
- `m.loading` なら本文の代わりに `merge.loading` の 1 行

- [ ] **Step 2: golden を書いて録る**

`internal/tui/merge/golden_test.go` は `internal/tui/checks/golden_test.go` の形をそのまま使う
（`goldenWidths` = 160 / 120 / 80、`goldenLanguages` = en / ja）。録る場面は 4 つ:

1. `merge_clean` — 3 方式・`MergeStateClean`・`ReviewRequired`（auto-merge は選べない）
2. `merge_auto` — `MergeStateUnstable` で auto-merge 可、`space` を押して `[x]` になった状態
3. `merge_auto_on` — `AutoMergeEnabled` が真（`merge.auto_on` の行が出る。D6）
4. `merge_blocked` — `MergeableConflicting`
5. `merge_loading` — `Init` の答えが来る前

**モデルは `loaded()` と `press()` を通して作る**（Task 5 のヘルパーを golden からも使う。
状態を直接組み立てない。`.claude/rules/testing.md`）。

```bash
OCTOSCOPE_UPDATE_GOLDEN=1 go test ./internal/tui/merge/
```

- [ ] **Step 3: 録った golden を目で見る**

```bash
cat internal/tui/merge/testdata/merge_clean_ja_80.golden
cat internal/tui/merge/testdata/merge_auto_ja_80.golden
```

**ja の 80 桁でキーバーが折り返していないこと**（spec §6）、枠が崩れていないことを見る。
折り返していたら文言を詰める。**通ったから完了、にしない。**

- [ ] **Step 4: テストが通ることを確かめる**

Run: `go test ./internal/tui/merge/ ./internal/i18n/`
Expected: PASS（`internal/i18n` が en / ja の食い違いを見る）

- [ ] **Step 5: `make check` とコミット**

```bash
make check
git add internal/tui/merge internal/i18n/locales
git commit -m "feat: draw the merge popup"
```

---

### Task 7: 詳細ビューと root への配線、spec の訂正

**Files:**
- Modify: `internal/tui/detail/detail.go`, `internal/tui/detail/render.go`, `internal/tui/app/app.go`, `docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md`
- Test: `internal/tui/detail/detail_test.go`, `internal/tui/app/app_test.go`

**Interfaces:**
- Consumes: Task 5・6 の `merge.Model` ほか

- [ ] **Step 1: 失敗するテストを書く**

`internal/tui/detail/detail_test.go`（ヘルパー名は既存のテストが使っているものに合わせる）:

```go
func TestMOpensTheMergePopupOnAPullRequest(t *testing.T) {
	t.Parallel()

	m := loadedPR(t)
	m, cmd := press(m, "m")
	if cmd == nil {
		t.Fatal("m returned no command: the popup never fetches")
	}
	if !strings.Contains(m.View(), i18n.T("merge.loading")) {
		t.Error("the merge popup is not on screen")
	}
}

func TestMDoesNothingOnAnIssue(t *testing.T) {
	t.Parallel()

	m := loadedIssue(t)
	before := m.View()
	m, cmd := press(m, "m")
	if cmd != nil {
		t.Error("m sent something on an issue, which has no merge")
	}
	if m.View() != before {
		t.Error("m changed the screen on an issue")
	}
}

func TestAMergeClosesTheDetailView(t *testing.T) {
	t.Parallel()

	m := loadedPR(t)
	m, _ = press(m, "m")
	_, cmd := m.Update(merge.MergedMsg{})
	if cmd == nil {
		t.Fatal("MergedMsg produced no command: the view stays open on a merged pull request")
	}
	if msg := cmd(); msg != (ClosedMsg{}) {
		t.Errorf("cmd() = %#v, want ClosedMsg{}: the root is what refetches", msg)
	}
}
```

`internal/tui/app/app_test.go` に、`TestASubmittedReviewRefreshesTheBoardAndTheReposList`
（`app_test.go:440`）をそのまま写した形で 1 本足す:

```go
// TestAMergeRefreshesTheBoardAndTheReposList guards spec §4.4.4: a merged
// pull request must leave the Work board, not only the view it was merged
// from.
func TestAMergeRefreshesTheBoardAndTheReposList(t *testing.T) {
	f := &fakeSource{}
	m := newTestModelWith(f, Options{HasRepo: true})

	_, cmd := m.Update(merge.MergedMsg{})
	if cmd == nil {
		t.Fatal("merge.MergedMsg produced no command")
	}
	resolve(t, m, cmd)

	if f.workCalls == 0 {
		t.Error("the board was not refreshed after a merge")
	}
	if f.prCalls == 0 {
		t.Error("the Repos list was not refreshed after a merge")
	}
}
```

- [ ] **Step 2: テストが落ちることを確かめる**

Run: `go test ./internal/tui/detail/ -run TestM`
Expected: FAIL

- [ ] **Step 3: 詳細ビューに繋ぐ**

- `mode` に `modeMerge` を足し、`String()` に `"merge"` を返す枝を足す
- `Source` に `merge.Source` を足す（`review.Source` の隣）
- `Model` に `merge merge.Model` を足す
- `handleKey` の `case "s":` の隣に:

```go
	case "m":
		// An issue has nothing to merge.
		if m.ref.Kind != gh.ItemPR {
			return m, nil
		}
		if m.phase == phaseLoading {
			return m.stillLoading(), nil
		}
		m.mode, m.phase = modeMerge, phaseIdle
		m.errText = ""
		m.merge = merge.New(m.src, m.ref)
		m.merge, _ = m.merge.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		return m, m.merge.Init()
```

- `handleKey` の mode 振り分けに `case modeMerge:` を足し、`handleSubmitKey` と同じ形で
  `m.merge` に渡す `handleMergeKey` を書く
- `Update` に 3 つの受け口を足す（`review.*` の隣）:
  - `merge.CancelledMsg` → `m.mode, m.phase = modeView, phaseIdle`、`m.errText = ""`、
    **`m.merge = merge.Model{}`**（`Active()` がポップアップの消えたあとも真のままにならないように）
  - `merge.MergedMsg` → `m.merge = merge.Model{}` にして
    `func() tea.Msg { return ClosedMsg{} }` を返す
    （spec §4.4.4「成功したら詳細ビューを閉じて Work に戻り、再取得する」）
  - `merge.ErrorMsg` → `m.errText = msg.Err.Error()` にして `m.merge` にも渡す
    （`submitFailed` と同じ形）
- `Update` の `resize` にある「ポップアップにも配る」枝（`m.submit.Active()` の箇所、
  `internal/tui/detail/detail.go:378`）に `m.merge.Active()` を足す。**そこは
  `tea.WindowSizeMsg` だけを配る枝で、キーは通らない**——キーは `handleKey` の
  mode 振り分けからだけポップアップに届く
- `render.go` で `m.mode == modeMerge` のとき `m.merge.View()` を重ねる

**詳細ビューは `MergedMsg` を出し直さない。** ポップアップの cmd が返した `MergedMsg` は
Bubble Tea が先に root に届け、root が `broadcast` で詳細ビューに配る
（`review.SubmittedMsg` がそう流れている。`app.go:273` → `detail.go:358`）。
詳細ビューが `tea.Batch` で `MergedMsg` を返すと root がもう一度受け取って
もう一度配ることになる。返すのは `ClosedMsg` だけ。

- [ ] **Step 4: root に繋ぐ**

`internal/tui/app/app.go`:

- `Source` に `merge.Source` を足す
- `Update` に `case merge.MergedMsg:` を足す
- `reviewSubmitted` の本体（Work と Repos の取り直し）を `refreshLists()` として括り出し、
  `reviewSubmitted` と新しい `merged` の両方から呼ぶ。**同じ処理に 2 つの実装を置かない**

- [ ] **Step 5: キーバーに `m` を出す**

詳細ビューのキーバー（`internal/tui/detail/render.go`）に `keybar.merge` を足す。
**PR のときだけ出す**（issue にマージは無い）。

- [ ] **Step 6: golden を録り直して目で見る**

```bash
OCTOSCOPE_UPDATE_GOLDEN=1 go test ./internal/tui/detail/ ./internal/tui/app/
git diff --stat internal/tui/detail/testdata internal/tui/app/testdata
cat internal/tui/detail/testdata/<PR の 80 桁 ja の golden>
```

差分が「キーバーに `m` が増えた」だけであることを確かめる。他が動いていたら止めて調べる。
**ja の 80 桁でキーバーが収まっていること**を目で見る。入りきらないキーが落ちる仕組み
（`internal/tui/layout`）に任せる場合も、落ちた結果を見て納得してから進む。

- [ ] **Step 7: spec を D1〜D4 に合わせて直す**

`docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md` §4.4.4:

- モックアップの `[x] ブランチを削除する` を、印の無い 1 行に直す
- 「ブランチ削除の既定はリポジトリの `deleteBranchOnMerge`…その場でトグルもできる」の箇条書きを、
  「`mergePullRequest` に削除の入力が無い（2026-09-08 実測）ので、リポジトリの
  `deleteBranchOnMerge` を**読み取り専用で見せる**」に直す
- auto-merge の箇条書きに「`mergeStateStatus` が `CLEAN` のときも選べない（GitHub が断る）」を足す
- `mergeable: UNKNOWN` の箇条書きに「`enter` は塞ぐ」を明記する（D3）
- draft も `enter` を塞ぐことを足す（D4）

- [ ] **Step 8: 全部通ることを確かめる**

Run: `make check`
Expected: PASS

- [ ] **Step 9: 実際に起動して見る**

```bash
go run ./cmd/octoscope --repo kukv/octoscope
go run ./cmd/octoscope --repo kukv/octoscope --lang ja
```

開いている PR の詳細を出して `m` を押し、ポップアップを**目で見る**。`j` / `k` で方式が動くこと、
`r` で取り直せること、`esc` で閉じること、枠が崩れていないこと。
**`enter` は押さない**（実物がマージされる）。`kukv/octoscope` は `autoMergeAllowed: false`
なので auto-merge の行は `merge.auto_unavailable_repo` になるはずで、それも見る。

- [ ] **Step 10: コミット**

```bash
make check
git add internal docs
git commit -m "feat: merge a pull request from the detail view"
```

---

### Task 8: 受け渡しの文書

**Files:**
- Create: `docs/superpowers/2026-09-08-phase3-merge-handoff.md`
- Modify: `docs/superpowers/2026-09-08-phase3-checks-followups.md`

spec §8 の 9 番目——**実在の PR を TUI からマージできること**——は TTY と実在の PR が要るので
この環境では代行できない。`docs/superpowers/2026-09-07-phase3-checks-handoff.md` と同じ形で書く。

- [ ] **Step 1: handoff を書く**

含めるもの:

- 起動の仕方（`--lang ja` を含む）と、`m` から何が見えるか
- **実マージは戻せない。** 試すなら捨ててよい PR を先に作る、と明記する
- **auto-merge は `kukv/octoscope` では確かめられない**（`autoMergeAllowed: false`、spec §2 の実測）。
  確かめるにはリポジトリ設定で "Allow auto-merge" を入れるか、
  それが入っている別のリポジトリを `--repo` で指す
- 確かめてほしいこと: 方式の一覧がリポジトリの設定と合っているか、`⚠` の理由が実状と合っているか、
  マージ後に Work 板からカードが消えるか、`UNKNOWN` が `r` で解けるか
- 見つかったが直さないと決めたことは followups に足す

- [ ] **Step 2: followups を更新する**

`docs/superpowers/2026-09-08-phase3-checks-followups.md` の「Phase 3 の残り」から merge を消し、
残りがページングだけであることを書く。

- [ ] **Step 3: コミットして PR を出す**

```bash
make check
git add docs
git commit -m "docs: hand the merge popup over for a run on a real pull request"
```

---

## Self-Review

**spec の網羅**

| spec の要求 | どのタスク |
|---|---|
| §4.4.4 方式はリポジトリが許したものだけ | Task 2（`allowedMethods`）/ Task 6（描画） |
| §4.4.4 ブランチ削除 | Task 6（D1 により読み取り専用）/ Task 7 Step 7（spec の訂正） |
| §4.4.4 auto-merge の可否と理由 | Task 1（`CanAutoMerge`）/ Task 6 |
| §4.4.4 既に auto-merge があるなら解除を提案 | Task 5（`send()` の `AutoMergeEnabled` の枝。D6）/ Task 6 |
| §4.4.4 `UNKNOWN` は計算中、`r` で取り直す | Task 1（`BlockComputing`）/ Task 5（`r`） |
| §4.4.4 `CONFLICTING` / `BLOCKED` / `BEHIND` / `DIRTY` は `enter` を塞ぐ | Task 1（`Block`）/ Task 5 |
| §4.4.4 確認はこのポップアップ 1 段だけ | Task 5（確認画面を挟まない） |
| §4.4.4 成功したら詳細を閉じて Work を取り直す | Task 7 Step 3・4 |
| §4.4.4 失敗は GitHub のメッセージをそのまま | Task 5（`ErrorMsg`）/ Task 7（`errText`） |
| §3 3 つの mutation、`gh pr merge` は使わない | Task 3 |
| §4 `internal/tui` は `internal/gh/cli` を見ない | Task 4・5（`Source` interface） |
| §6 fixture / `Update` 越しの状態 / 壊して落ちる確認 / golden en・ja × 3 幅 | 各タスクの Step |
| §8-4・5・7・8 | Task 5・6・7 |
| §8-9（人手） | Task 8 |
