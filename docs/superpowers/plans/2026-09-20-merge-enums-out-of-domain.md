# マージの GitHub enum をドメインから外す実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `domain.MergeState`（7 値）と `domain.Mergeable`（3 値）を削除し、
`MergeContext` が `Block MergeBlock` と `Clean bool` だけを持つ形にする。
GitHub の綴りから `MergeBlock` への導出は gateway に移す。

**Architecture:** 翻訳は ACL（gateway）、政策はドメイン。`Block()` メソッドが
フィールドになり、`CanAutoMerge` / `CanMergeAsAdmin` はドメインに残る。
**振る舞いは変わらない**——新しい真理値表は、今の
`parseMergeable` + `parseMergeState` + `Block()` の合成と 1 対 1 で一致する。

**Tech Stack:** Go、golangci-lint、gotestsum、`internal/golden`

**Spec:** `docs/superpowers/specs/2026-09-20-merge-enums-out-of-domain-design.md`

## Global Constraints

- **golden 354 枚と `testdata/` 全体を 1 バイトも変えない。** 動いたら真理値表が違う
- **未知の綴りの扱いを「直さない」。** 未知の mergeable は `BlockComputing`、
  未知の state は `BlockNone`。今の振る舞いのままにする
- **`CheckKind` を触らない**（設計 §5）。`MergeMethod` `ReviewState` `CheckState` も触らない
- **`Clean` を `Block == BlockNone` で代用しない。** `BlockNone` は CLEAN と
  UNSTABLE / HAS_HOOKS を一緒くたにするが、`CanAutoMerge` はその 2 つを区別する
- `make check` が通ること
- コミットメッセージの末尾に `Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>` を付ける

## 着手前に調べた事実（2026-09-20）

### A. 触るファイルは 8 つ（テスト込み）

| ファイル | 何をするか |
|---|---|
| `internal/app/domain/merge.go` | `Mergeable` / `MergeState` を削除、`MergeContext` を作り替え、`Block()` を消す |
| `internal/app/domain/merge_test.go` | `Block()` の真理値表を削除（gateway へ移動）、残り 2 本を新フィールドで組み直す |
| `internal/app/adapter/gateway/gh/merge.go` | `parseMergeable` / `parseMergeState` → `toMergeBlock`、`Clean` を詰める |
| `internal/app/adapter/gateway/gh/merge_test.go` | 旧 parse テスト 2 本 → 真理値表テスト、期待値のリテラル |
| `internal/app/presentation/tui/merge/render.go` | `m.ctx.Block()` → `m.ctx.Block`（2 箇所: L111, L170） |
| `internal/app/presentation/tui/merge/merge.go` | 同じ（1 箇所: L195） |
| `internal/app/presentation/tui/merge/merge_test.go` | フィクスチャ（L94-95, 138-139, 176, 217, 446, 488-496） |
| `internal/app/presentation/tui/merge/golden_test.go` | フィクスチャ（L44, 76-77, 91-92） |
| `internal/app/presentation/tui/detail/detail_test.go` | フィクスチャ（L1065-1066, 1130-1131） |

### B. 消える gateway テスト 2 本の中身は捨てない

`TestParseMergeableCoversEveryValue` と `TestParseMergeStateCoversEveryValue` は
`parseMergeable` / `parseMergeState` と一緒に消えるが、**その主張——
「知らない綴りは失敗ではない」——は新しい真理値表テストのケースとして残す。**

### C. `MergeContext.IsDraft` は `Block()` 専用

`domain.Item` と `domain.WorkItem` の `IsDraft` は別物で、UI が直接読む。**それらは触らない。**

---

### Task 1: 真理値表を gateway へ移し、ドメインの enum を消す

コンパイル境界が層をまたぐため、**1 コミットで行う**。
途中でビルドが通らない状態を経由するのは避けられない。

**Files:**
- Modify: `internal/app/adapter/gateway/gh/merge.go`
- Modify: `internal/app/adapter/gateway/gh/merge_test.go`
- Modify: `internal/app/domain/merge.go`
- Modify: `internal/app/domain/merge_test.go`
- Modify: `internal/app/presentation/tui/merge/render.go`
- Modify: `internal/app/presentation/tui/merge/merge.go`
- Modify: `internal/app/presentation/tui/merge/merge_test.go`
- Modify: `internal/app/presentation/tui/merge/golden_test.go`
- Modify: `internal/app/presentation/tui/detail/detail_test.go`

**Interfaces:**
- Produces: `toMergeBlock(isDraft bool, mergeable, state string) domain.MergeBlock`
  （`adapter/gateway/gh` の非公開関数）、`domain.MergeContext.Block domain.MergeBlock`、
  `domain.MergeContext.Clean bool`
- Removes: `domain.Mergeable`（型と 3 定数）、`domain.MergeState`（型と 7 定数）、
  `domain.MergeContext.IsDraft`、`domain.MergeContext.Block()` メソッド、
  `parseMergeable`、`parseMergeState`

- [ ] **Step 1: gateway に真理値表テストを書く（先に失敗させる）**

`internal/app/adapter/gateway/gh/merge_test.go` の
`TestParseMergeableCoversEveryValue` と `TestParseMergeStateCoversEveryValue`
（L72-127 あたり、2 本まるごと）を、次の 2 本に**置き換える**。

```go
// TestToMergeBlockReadsWhatTheServiceReported is the table that used to live
// on domain.MergeContext.Block. It moved here with the translation it does:
// draft-first exists because GitHub reports a draft as BLOCKED, which is a
// fact about GitHub and not a rule of this application.
func TestToMergeBlockReadsWhatTheServiceReported(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		isDraft   bool
		mergeable string
		state     string
		want      domain.MergeBlock
	}{
		{"clean", false, "MERGEABLE", "CLEAN", domain.BlockNone},
		{"draft outranks the state GitHub reports for it", true, "MERGEABLE", "BLOCKED", domain.BlockDraft},
		{"conflicting", false, "CONFLICTING", "DIRTY", domain.BlockConflicting},
		{"still computing", false, "UNKNOWN", "UNKNOWN", domain.BlockComputing},
		{"protected", false, "MERGEABLE", "BLOCKED", domain.BlockProtected},
		{"behind", false, "MERGEABLE", "BEHIND", domain.BlockBehind},
		{"dirty", false, "MERGEABLE", "DIRTY", domain.BlockDirty},
		{"failing checks do not block: GitHub allows the merge", false, "MERGEABLE", "UNSTABLE", domain.BlockNone},
		{"hooks do not block either", false, "MERGEABLE", "HAS_HOOKS", domain.BlockNone},
		{"a mergeable word we do not know means the answer is not in yet", false, "NEW_WORD", "CLEAN", domain.BlockComputing},
		{"a state word we do not know refuses nothing", false, "MERGEABLE", "NEW_WORD", domain.BlockNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := toMergeBlock(tt.isDraft, tt.mergeable, tt.state)
			if got != tt.want {
				t.Errorf("toMergeBlock(%v, %q, %q) = %v, want %v",
					tt.isDraft, tt.mergeable, tt.state, got, tt.want)
			}
		})
	}
}

// Clean is not the same question as Block: UNSTABLE refuses nothing, but
// there is still something to wait for, and that is exactly when auto-merge
// is worth offering.
func TestCleanIsOnlyGitHubsCleanState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		state string
		want  bool
	}{
		{"CLEAN", true},
		{"UNSTABLE", false},
		{"HAS_HOOKS", false},
		{"BLOCKED", false},
		{"UNKNOWN", false},
		{"NEW_WORD", false},
	}
	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			t.Parallel()

			got := toMergeContext(gql.MergeContext{
				Mergeable: "MERGEABLE", MergeStateStatus: tt.state,
			}).Clean
			if got != tt.want {
				t.Errorf("Clean for %q = %v, want %v", tt.state, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: 落ちることを確認する**

```bash
go test ./internal/app/adapter/gateway/gh/ 2>&1 | head -20
```

期待: `undefined: toMergeBlock` でコンパイルが通らない。
**通ってしまったら、その名前が既にある**ので、なぜあるのかを調べてから続ける。

- [ ] **Step 3: `domain/merge.go` を書き換える**

`Mergeable` と `MergeState` の型宣言・const ブロックを**削除**する。
`MergeMethod` と `MergeBlock` は残す。`MergeContext` を次の形にする。

```go
// MergeContext is everything the merge popup draws and acts on: what the
// repository allows, and what state this pull request is in.
type MergeContext struct {
	PullRequest PullRequestHandle

	// Block is why merging is refused right now, as the gateway read it out
	// of what the service reported. BlockNone means nothing refuses it.
	Block MergeBlock

	// Clean says there is nothing left to wait for. That is not the same as
	// "not blocked": checks still running refuse nothing, and auto-merge is
	// exactly the thing to offer then.
	Clean bool

	Review ReviewState

	// Methods holds only the methods the repository allows, in the order
	// the popup lists them: squash, merge commit, rebase.
	Methods             []MergeMethod
	DeleteBranchOnMerge bool

	AutoMergeAllowed         bool
	ViewerCanEnableAutoMerge bool
	AutoMergeEnabled         bool

	// ViewerIsAdmin is what keeps the popup from offering a key that fails.
	// The mutation that merges takes no admin input -- gh pr merge --admin
	// sends the same one -- so nothing in the answer to the merge itself
	// says whether this viewer may push past a rule. GitHub's own
	// viewerCanMergeAsAdmin reads classic branch protection only and answers
	// false under a ruleset, so the gateway fills this from the repository
	// permission instead.
	ViewerIsAdmin bool
}
```

`Block()` メソッドを**削除**し、残る 2 つを次にする。

```go
// CanAutoMerge reports whether auto-merge can be turned on. GitHub refuses
// it on a pull request that is already clean: there is nothing left to wait
// for, so it wants an ordinary merge instead.
func (c MergeContext) CanAutoMerge() bool {
	return c.AutoMergeAllowed && c.ViewerCanEnableAutoMerge && !c.Clean
}

// CanMergeAsAdmin reports whether the viewer can push the merge through what
// is holding it. Only two blocks give way, the same two gh's --admin silences.
// A draft or a conflict is not a rule to bypass: it is work that is not
// finished, and no permission finishes it.
func (c MergeContext) CanMergeAsAdmin() bool {
	if !c.ViewerIsAdmin {
		return false
	}
	switch c.Block {
	case BlockProtected, BlockBehind:
		return true
	}
	return false
}
```

`MergeBlock` の doc コメントを 1 行足す。

```go
// MergeBlock is why merging is refused right now. BlockNone means it is not.
// The gateway decides which one applies: reading a service's own answer into
// this is translation, not a rule of this application.
type MergeBlock int
```

- [ ] **Step 4: `domain/merge_test.go` を直す**

`TestBlockNamesWhyMergingIsRefused` を**削除**する（Step 1 で gateway に移した）。
残る 2 本のテーブルを、新しいフィールドで組み直す。**ケース名と件数は変えない。**

`TestOnlyTwoBlocksGiveWayToAnAdmin`:

```go
		{"protected: this is what the key is for", admin(MergeContext{
			Block: BlockProtected,
		}), true},
		{"behind: gh's --admin bypasses this one too", admin(MergeContext{
			Block: BlockBehind,
		}), true},
		{"clean: nothing to push past", admin(MergeContext{
			Block: BlockNone, Clean: true,
		}), false},
		{"unstable: GitHub allows the merge already", admin(MergeContext{
			Block: BlockNone,
		}), false},
		{"draft: not a rule, unfinished work", admin(MergeContext{
			Block: BlockDraft,
		}), false},
		{"conflicting: no permission resolves a conflict", admin(MergeContext{
			Block: BlockConflicting,
		}), false},
		{"computing: the answer is not in yet", admin(MergeContext{
			Block: BlockComputing,
		}), false},
		{"dirty: GitHub will not merge it at all", admin(MergeContext{
			Block: BlockDirty,
		}), false},
		{"blocked, but the viewer is no admin", MergeContext{
			Block: BlockProtected,
		}, false},
```

`TestAutoMergeNeedsSomethingToWaitFor`:

```go
		{"waiting on checks", MergeContext{
			AutoMergeAllowed: true, ViewerCanEnableAutoMerge: true, Clean: false,
		}, true},
		{"nothing to wait for: GitHub refuses auto-merge on a clean pull request", MergeContext{
			AutoMergeAllowed: true, ViewerCanEnableAutoMerge: true, Clean: true,
		}, false},
		{"repository has it turned off", MergeContext{
			AutoMergeAllowed: false, ViewerCanEnableAutoMerge: true, Clean: false,
		}, false},
		{"viewer may not", MergeContext{
			AutoMergeAllowed: true, ViewerCanEnableAutoMerge: false, Clean: false,
		}, false},
```

```bash
go test ./internal/app/domain/
```

期待: 緑。

- [ ] **Step 5: `gateway/gh/merge.go` を書き換える**

`parseMergeable` と `parseMergeState` を**削除**し、`toMergeBlock` を足す。

```go
// toMergeBlock reads what the service reported into the one thing the
// application asks: why is this refused right now. Draft comes first
// because GitHub reports a draft as BLOCKED, and "it is a draft" is the
// more useful of the two.
//
// A spelling neither switch knows is not a failure -- GitHub adds values to
// these enums. An unknown mergeable means the answer is not worked out yet;
// an unknown state refuses nothing.
func toMergeBlock(isDraft bool, mergeable, state string) domain.MergeBlock {
	switch {
	case isDraft:
		return domain.BlockDraft
	case mergeable == "CONFLICTING":
		return domain.BlockConflicting
	case mergeable != "MERGEABLE":
		return domain.BlockComputing
	case state == "BLOCKED":
		return domain.BlockProtected
	case state == "BEHIND":
		return domain.BlockBehind
	case state == "DIRTY":
		return domain.BlockDirty
	}
	return domain.BlockNone
}
```

`toMergeContext` の 3 行を差し替える。

```go
		PullRequest:              domain.PullRequestHandle(c.PullRequestID),
		Block:                    toMergeBlock(c.IsDraft, c.Mergeable, c.MergeStateStatus),
		Clean:                    c.MergeStateStatus == "CLEAN",
		Review:                   parseReviewDecision(c.ReviewDecision),
```

（`IsDraft:` `Mergeable:` `State:` の 3 行は消える。）

- [ ] **Step 6: 呼び出し側 3 箇所を直す**

```bash
grep -rn "ctx.Block()" internal/app/presentation/
```

`merge/render.go:111`、`merge/render.go:170`、`merge/merge.go:195` の
`m.ctx.Block()` を `m.ctx.Block` にする。**他は変えない。**

- [ ] **Step 7: テストのフィクスチャを直す**

コンパイルエラーが全部出るので、それを潰す。写像は次のとおり。

| 今 | これから |
|---|---|
| `Mergeable: MergeableYes, State: MergeStateClean` | `Block: BlockNone, Clean: true` |
| `Mergeable: MergeableYes, State: MergeStateUnstable` | `Block: BlockNone`（`Clean` は false のまま） |
| `Mergeable: MergeableYes, State: MergeStateBlocked` | `Block: BlockProtected` |
| `Mergeable: MergeableConflicting, State: MergeStateDirty` | `Block: BlockConflicting` |
| `Mergeable: MergeableUnknown, State: MergeStateUnknown` | `Block: BlockComputing` |
| `IsDraft: true`（`MergeContext` の） | `Block: BlockDraft` |

対象は `merge/merge_test.go`、`merge/golden_test.go`、`detail/detail_test.go`、
`gateway/gh/merge_test.go` の期待値（L57-59、L195）。

**共通ヘルパー `mergeable()`（`merge_test.go:91`）が起点である。** 今は
`Mergeable: MergeableYes, State: MergeStateUnstable` を詰めており、これは
新しい形では **`Block: BlockNone`（ゼロ値）、`Clean: false`（ゼロ値）** なので、
**2 行をただ削除する**のが正しい写し方になる。

```go
func mergeable() domain.MergeContext {
	return domain.MergeContext{
		PullRequest: "PR_1",
		Methods:     []domain.MergeMethod{domain.MergeSquash, domain.MergeCommit, domain.MergeRebase},
	}
}
```

そこからの差分は 1 行ずつになる。

- `merge_test.go:488` `draft.IsDraft = true` → `draft.Block = domain.BlockDraft`
- `merge_test.go:176` `clean.State = domain.MergeStateClean` → `clean.Clean = true`
- `merge_test.go:446` `c.State = domain.MergeStateBlocked` → `c.Block = domain.BlockProtected`
- `golden_test.go:44` `c.State = domain.MergeStateClean` → `c.Clean = true`（`Block` は既に `BlockNone`）
- `Mergeable` と `State` を 2 行で上書きしている箇所（`merge_test.go:138-139, 217, 491-492, 495-496`、
  `golden_test.go:76-77, 91-92`）は **`Block` 1 行**になる

- [ ] **Step 8: 検査**

```bash
make fmt
make check
git status --porcelain | grep -c testdata    # 0 であること
```

**golden が 1 枚でも動いたら止める。** 真理値表が今の振る舞いと違うということなので、
どのケースがずれたのかを突き止めてから続ける（`git diff` で golden を見る）。

消え残りが無いことも確かめる。

```bash
grep -rn "MergeState\|Mergeable" internal/ | grep -v "MergeStateStatus"   # 0 件
```

`MergeStateStatus` は `gql` のワイヤ型のフィールド名なので残る。

- [ ] **Step 9: コミット**

```bash
git add internal
git commit -m "refactor: let the gateway say why a merge is refused" \
  -m "domain.MergeState mirrored GitHub's mergeStateStatus value for value and
domain.Mergeable its mergeable, while the popup read neither: it reads
Block(), CanAutoMerge() and CanMergeAsAdmin(). Block() was translation --
draft comes first because GitHub reports a draft as BLOCKED -- so it moved to
the gateway with the two enums it read. What stays in the domain is policy:
which blocks give way to an admin, and when auto-merge is worth offering." \
  -m "Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: 全体検査と実機確認の依頼

**Files:** なし（検査のみ）

- [ ] **Step 1: リリース構成まで通す**

```bash
make check
make release-check
```

- [ ] **Step 2: 設計書の完了条件を 1 つずつ確かめる**

```bash
grep -rn "type MergeState\|type Mergeable" internal/app/domain/     # 0 件
grep -rn "IsDraft" internal/app/domain/merge.go                     # 0 件
grep -rn "func (c MergeContext)" internal/app/domain/merge.go       # CanAutoMerge と CanMergeAsAdmin の 2 本
git diff --stat origin/main -- internal/app/presentation/tui/merge/testdata/   # 空
```

- [ ] **Step 3: 実機確認を利用者に依頼する**

マージポップアップを、**4 つの状態で**見てもらう。golden はフェイクの上の描画しか見ていないので、
GitHub の実際の綴りが真理値表に正しく入ることは実機でしか分からない。

- draft の PR → 「下書き」の理由が出る
- コンフリクトしている PR → 「コンフリクト」
- 保護ブランチで止まっている PR → 「保護」＋ 管理者なら admin マージの案内
- clean な PR → 理由が出ず、auto-merge が**提示されない**

`--lang ja` でも 1 回見てもらう。

---

## この PR の完了条件

- `internal/app/domain` に `MergeState` と `Mergeable` が存在しない
- `MergeContext` に `IsDraft` が無く、`Block` と `Clean` がある
- 真理値表が gateway にあり、draft 優先と未知の綴り 2 種を含む全ケースが残っている
- `CanMergeAsAdmin` / `CanAutoMerge` は domain にある
- golden 354 枚と `testdata/` 全体が無変更
- `make check` と `make release-check` が通る
- **利用者が実機でマージポップアップの 4 状態を確認した**
