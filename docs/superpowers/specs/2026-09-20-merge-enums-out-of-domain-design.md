# マージの GitHub enum をドメインから外す設計

as-is モデリングの違和感 **F-04**（https://claude.ai/artifact/7HMQJ6kbp2wVs1LoiNG3JU）に対する設計。

## 1. 問題

`domain.MergeState{Unknown, Clean, Blocked, Behind, Dirty, Unstable, HasHooks}` と
`domain.Mergeable{Unknown, Yes, Conflicting}` は、GitHub の `mergeStateStatus` と
`mergeable` の 1 対 1 の写しである。綴りは翻訳されているが、**値の集合が GitHub のもの**で、
形を借りている。`.claude/rules/architecture.md`「名前は借りてよい、形は借りない」に照らすと、
借りてよいのは名前のほうだけである。

その上で `MergeContext.Block()` がアプリ上の意味（`MergeBlock`）を導出しており、
**ドメインの値は導出結果のほう**だと読める。

## 2. 着手前に確かめた事実（2026-09-20、推測ではない）

### 2.1 プレゼン層はこの 2 つの enum を一度も読んでいない

```
grep -rn "MergeState|Mergeable" internal/app --include=*.go | grep -v _test.go
```

が返すのは `adapter/gateway/gh/merge.go` だけ。プレゼン層が読むのは
`Block()` / `CanAutoMerge()` / `CanMergeAsAdmin()` の 3 メソッドのみである
（`presentation/tui/merge/render.go`、`presentation/tui/merge/merge.go`）。

**F-04 が「確認してから」と書いていた「渡すと失われる情報（UI の文言差）」は存在しない。**
UI の文言は `blockText(m.ctx.Block())` が `MergeBlock` から引いており、
`MergeState` の値によって変わるものは 1 つも無い。

### 2.2 `MergeContext.IsDraft` も `Block()` からしか読まれていない

`domain.Item` と `domain.WorkItem` の `IsDraft` は別物で、そちらは UI が直接読む。
**`MergeContext` の `IsDraft` は `Block()` 専用**である。

### 2.3 9 値のうち 3 値は分岐に現れない

`MergeStateUnstable` / `MergeStateHasHooks` / `MergeableYes` は gateway が詰めるだけで、
`Block()` にも `CanAutoMerge()` にも出てこない。

## 3. 決めたこと: 導出は gateway に移す

**翻訳は ACL の仕事、政策はドメインの仕事**として線を引く。

`Block()` の draft 優先ルールは、コメント自身が理由を書いている——
「**GitHub は draft を BLOCKED として報告する**ので、『draft である』のほうが有用だ」。
これは GitHub の報告の癖を吸収する翻訳であって、アプリの政策ではない。

一方、**どのブロックが管理者権限に道を譲るか**（`CanMergeAsAdmin`）と、
**auto-merge を提示してよいか**（`CanAutoMerge`）はアプリの判断で、ドメインに残る。

> 俯瞰図 §07 は「`Block()` と `CanMergeAsAdmin()` は数少ないドメインのルールの実例」と
> 書いており、F-04 の方向性（導出結果だけをドメインに渡す）と一見矛盾する。
> **矛盾は「`Block()` は 1 つのルールではなく 2 つ」と分けることで解ける。**
> ドメインが失うのは翻訳であって、ルールではない。

## 4. 変更後の形

### 4.1 `domain/merge.go`

`Mergeable` と `MergeState` の型・定数をすべて削除する。`MergeBlock` は残る。

```go
type MergeContext struct {
	PullRequest PullRequestHandle

	// Block is why merging is refused right now, as the gateway read it out
	// of what the service reported. BlockNone means nothing is refusing it.
	Block MergeBlock

	// Clean says there is nothing left to wait for. It is not the same as
	// "not blocked": checks still running refuse nothing, and auto-merge is
	// exactly the thing to offer then.
	Clean bool

	Review ReviewState
	Methods []MergeMethod
	DeleteBranchOnMerge bool
	AutoMergeAllowed bool
	ViewerCanEnableAutoMerge bool
	AutoMergeEnabled bool
	ViewerIsAdmin bool
}

func (c MergeContext) CanAutoMerge() bool {
	return c.AutoMergeAllowed && c.ViewerCanEnableAutoMerge && !c.Clean
}

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

`Block()` メソッドは消える。呼び出し側は `m.ctx.Block()` から `m.ctx.Block` になる
（`merge/render.go` 2 箇所、`merge/merge.go` 1 箇所）。

**`Clean` を `Block == BlockNone` で代用しない。** `BlockNone` は CLEAN と
UNSTABLE / HAS_HOOKS を一緒くたにするが、`CanAutoMerge` はその 2 つを区別する必要がある
（GitHub は clean な PR への auto-merge を拒む）。

### 4.2 `adapter/gateway/gh/merge.go`

`parseMergeable` / `parseMergeState` を削除し、次の 2 つに置き換える。

```go
// toMergeBlock reads what the service reported into the one thing the
// application asks: why is this refused right now. Draft comes first
// because GitHub reports a draft as BLOCKED, and "it is a draft" is the
// more useful of the two.
//
// A spelling neither switch knows is not a failure: GitHub adds values to
// these enums. An unknown mergeable means the answer is not worked out yet
// (BlockComputing); an unknown state refuses nothing (BlockNone).
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

`Clean` は `c.MergeStateStatus == "CLEAN"`。

### 4.3 振る舞いの同値性

新しい `toMergeBlock` は、今の `parseMergeable` + `parseMergeState` + `Block()` の
**合成と 1 対 1 で一致する**。非対称もそのまま保つ。

| 入力 | 今 | これから |
|---|---|---|
| `isDraft` | `BlockDraft`（他より先） | 同じ |
| `mergeable = CONFLICTING` | `BlockConflicting` | 同じ |
| `mergeable` が `MERGEABLE` でも `CONFLICTING` でもない | `MergeableUnknown` → `BlockComputing` | `BlockComputing` |
| `state = BLOCKED` | `BlockProtected` | 同じ |
| `state = BEHIND` | `BlockBehind` | 同じ |
| `state = DIRTY` | `BlockDirty` | 同じ |
| `state = CLEAN` / `UNSTABLE` / `HAS_HOOKS` / 未知 | `BlockNone` | 同じ |
| `state = CLEAN` | `Clean` 相当（`State == MergeStateClean`） | `Clean = true` |

**未知の綴りの扱いを「直さない」。** 未知の mergeable が `BlockComputing` で、
未知の state が `BlockNone` なのは今の振る舞いであり、この設計の対象ではない。

## 5. 範囲外

- **`CheckKind{Run, Status}`。** 規約「名前は借りてよい、形は借りない」に照らすと**適合している**。
  借りているのは名前（StatusContext）だけで、区別そのもの——「ログと再実行のあるジョブ」対
  「外部サービスの報告」——はアプリ自身の概念であり、checks ビューが並び順と `canRerun` に
  そう使っている。GitLab にも同じ分かれ方がある
- `MergeMethod{Squash, Commit, Rebase}` — 俯瞰図が「問題にならない」と確認済み
- `ReviewState`、`CheckState` — F-04 に挙がっていない

## 6. テスト

- `domain/merge_test.go` の `Block()` 真理値表は **gateway に移す**。入力が
  `domain.MergeContext` から GitHub の綴り（`isDraft, mergeable, state`）に変わるだけで、
  **ケースは 1 つも減らさない**。draft 優先のケースも含む
- `CanMergeAsAdmin` / `CanAutoMerge` のテストは domain に残り、`Block` と `Clean` で組み立てる
- `presentation/tui/merge/{merge,golden}_test.go`、`presentation/tui/detail/detail_test.go` の
  `MergeContext` リテラルが書き換わる。コンパイルエラーで全部出る

## 7. 完了条件

- `internal/app/domain` に `MergeState` と `Mergeable` が存在しない
- `MergeContext` に `IsDraft` が無い
- `Block()` の真理値表が gateway にあり、draft 優先を含む全ケースが残っている
- `CanMergeAsAdmin` / `CanAutoMerge` は domain にある
- **golden 354 枚が無変更**。`blockText` の入力が同じ値になる以上バイト同一のはずで、
  動いたら真理値表が違うということなので、そこで止める
- `make check` が通る
- 利用者が実機でマージポップアップを確認する（draft・コンフリクト・保護ブランチ・clean）
