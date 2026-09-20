# Item 統合 PR 5b（フィクスチャの移行、`domain.PR` / `domain.Issue` の削除）実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** テストフィクスチャ 163 箇所を `domain.Item` に移し、`domain.PR` / `domain.Issue` を削除する。
Item 統合の最後の PR。これで**同じものを指す型が 1 つになる**（違和感 F-01 の解消）。

**Architecture:** テストだけの変更 + 型 2 つの削除。本体コードは 1 行も変わらない。
`domain.PR` / `domain.Issue` は PR 5a の時点で**テストの中にしか残っていない。**

**Tech Stack:** Go、golangci-lint、gotestsum、`internal/golden`

**Spec:** `docs/superpowers/specs/2026-09-20-item-unification-design.md`（§8 PR 5b、§9 完了条件）

**Base:** PR 5a（ブランチ `feat/item-unification-pr5`）の上に積む。

---

## Global Constraints

- **golden 354 枚と `testdata/` 全体を 1 バイトも変えない。** golden の**入力**が変わるので、
  ここが最強の検査になる。1 枚でも変わったらフィクスチャの写し違いである
- **本体コード（非テスト）を 1 行も変えない。** `git diff` に `_test.go` 以外が出るのは
  `domain/item.go`（型の削除）と `domain/tags_test.go` だけ
- **不変条件を守る。** `Ref.Kind == ItemPR` のフィクスチャは `Change` を非 nil に、
  `ItemIssue` は nil にする。破ると repo ビューの `row()` が panic する
- **リテラルを機械的に増やさない。** 同じ形が繰り返される箇所はパッケージ内のビルダーにまとめる
  （利用者判断）。**ただし `domain.PR` と同じフィールドを持つテスト専用の型は作らない**——
  それは F-01 をテスト側に復活させることになる
- 各タスクの最後に `make check` が通ること
- コミットメッセージの末尾に `Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>` を付ける

## 着手前に調べた事実（2026-09-20）

### A. 参照は 163 箇所 / 16 ファイル。すべてテスト

| パッケージ | ファイル | 箇所 |
|---|---|---|
| `detail` | `detail_test.go` 61 / `picker_test.go` 13 / `render_test.go` 5 / `meta_test.go` 4 / `golden_test.go` 3 / `review_test.go` 3 / `mouse_test.go` 2 | 91 |
| `repo` | `repo_test.go` 25 / `table_test.go` 19 / `golden_test.go` 4 / `mouse_test.go` 3 | 51 |
| `root` | `root_test.go` 11 / `scenario_test.go` 3 / `mouse_test.go` 1 | 15 |
| `gateway/gh` | `items_test.go` | 4 |
| `domain` | `tags_test.go` | 2 |

### B. 既にある変換ヘルパー（これが消える）

| パッケージ | ヘルパー | 消し方 |
|---|---|---|
| `detail` | `prItem(domain.PR) domain.Item` / `issueItem(domain.Issue) domain.Item` | フィクスチャが最初から `domain.Item` になるので不要 |
| `repo` | `itemFromPR` / `itemFromIssue` / `itemsFromPRs` / `itemsFromIssues` | 同上 |
| `root` | `itemFromPR` / `itemsFromPRs` | 同上 |

**これらが恒等関数になったら消す。** 残すと「変換しているように見えて何もしていない」ものになる。

### C. 繰り返しが激しいリテラル 2 つ

- `detail`: `domain.PR{Number: 1, Title: "first pr", State: domain.StateOpen}` 系が **30 回超**。
  ラベル付き（`Labels: []domain.Label{{Name: "bug"}}`）が 10 回
- `repo`: `domain.PR{Number: i + 1, Title: "a pull request"}` がループ内に 4 箇所

**ここだけビルダーにまとめる。** それ以外の一点物のリテラルは直接書き換える。

### D. `gateway/gh/items_test.go` の 4 箇所は doc コメントと `want` 変数

`TestGetPR...` / `TestGetIssue...` は PR 5a で消えたので、残っているのは
**`toItemFromPR` / `toItemFromIssue` の検査**の中のコメント文言だけの可能性がある。
**着手時に確認すること**——`want := domain.PR{...}` が残っていれば、それは
`toItemFromPR` の検査が `domain.PR` を期待している箇所なので `domain.Item` に直す。

### E. `tags_test.go` の `exported`

`domain.PR{}` と `domain.Issue{}` を消して 24 → 22。
このテストは「`domain` の公開 struct に struct タグが無いこと」を reflect で確かめており、
一覧の網羅性を `go/ast` で照合する別のテストが対になっている。**数を合わせないと落ちる。**

### F. `domain.Item` に写らないフィールドは無い

`domain.PR` の 18 フィールドは `domain.Item` の 12 + `domain.Change` の 7 に対応する
（`Number` は `Ref.Number` に、`Kind` 相当は `Ref.Kind` に）。**写せないものは無い。**

---

## 対応表（書き換えの唯一の規則）

| `domain.PR` / `domain.Issue` | `domain.Item` |
|---|---|
| `Number: n` | `Ref: domain.ItemRef{Kind: domain.ItemPR, Number: n}` |
| `Title` `Author` `State` `URL` `Body` `BodyText` `Comments` `Labels` `Assignees` `UpdatedAt` | 同名のまま |
| `IsDraft` `Review` `Head` `Base` `Additions` `Deletions` `Checks` | `Change: &domain.Change{...}` の中へ |
| （`domain.Issue`） | `Ref.Kind` は `domain.ItemIssue`、`Change` は nil |

**`Ref.Repo` は空のままにする。** 今のフィクスチャは持っておらず、埋めると detail の
メタ欄にリポジトリ行が生えて golden が変わる（detail の `metaRows` は `ref` 引数から描くので
実際には変わらないが、`Ref` を読む箇所が将来増えたときのために揃えておく）。

---

### Task 1: `detail` パッケージのフィクスチャ（91 箇所）

**Files:**
- `internal/app/presentation/tui/detail/detail_test.go`（変更）
- `internal/app/presentation/tui/detail/picker_test.go`（変更）
- `internal/app/presentation/tui/detail/render_test.go`（変更）
- `internal/app/presentation/tui/detail/meta_test.go`（変更）
- `internal/app/presentation/tui/detail/golden_test.go`（変更）
- `internal/app/presentation/tui/detail/review_test.go`（変更）
- `internal/app/presentation/tui/detail/mouse_test.go`（変更）

- [ ] **Step 1: ビルダーを置く**

`detail_test.go` に、繰り返しの激しい 2 つをまとめる:

```go
// firstPR is the fixture most of these tests only need to be an open pull
// request: what is being checked is the view, not the item.
func firstPR() domain.Item {
	return domain.Item{
		Ref:    domain.ItemRef{Kind: domain.ItemPR, Number: 1},
		Title:  "first pr",
		State:  domain.StateOpen,
		Change: &domain.Change{},
	}
}

// labelledPR is firstPR with one label on it, which the picker tests start from.
func labelledPR() domain.Item {
	it := firstPR()
	it.Labels = []domain.Label{{Name: "bug"}}
	return it
}
```

**`Change: &domain.Change{}` を忘れない**（不変条件）。

- [ ] **Step 2: `fakeSource` のフィールドと `GetItem`**

`pr domain.PR` / `issue domain.Issue` を `pr domain.Item` / `issue domain.Item` にする。
`GetItem` は変換せずそのまま返す:

```go
func (f *fakeSource) GetItem(_ context.Context, ref domain.ItemRef) (domain.Item, error) {
	if ref.Kind == domain.ItemPR {
		return f.pr, f.err
	}
	return f.issue, f.err
}
```

**`prItem` / `issueItem` を消す。**

- [ ] **Step 3: リテラルを書き換える**

対応表のとおり。`firstPR()` / `labelledPR()` で置ける箇所は置き換える。
**フィールドの値は 1 つも変えない。** `golden_test.go` の `goldenPR()` /
`goldenMentionPR()` は golden の入力なので、**写し漏れが即 golden の差分になる**。
事実 F の対応表を 1 行ずつ確かめること。

- [ ] **Step 4: 検査**

```bash
make check
git status --porcelain | grep -c testdata    # 0 であること
```

**golden が 1 枚でも変わったら、そこで止めて写し違いを探す。**
`OCTOSCOPE_UPDATE_GOLDEN` を使わない。

- [ ] **Step 5: コミット**

```bash
git add internal/app/presentation/tui/detail
git commit -m "test: build the detail fixtures as domain.Item" \
  -m "Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: `repo` と `root` のフィクスチャ（66 箇所）

**Files:**
- `internal/app/presentation/tui/repo/repo_test.go`（変更）
- `internal/app/presentation/tui/repo/table_test.go`（変更）
- `internal/app/presentation/tui/repo/golden_test.go`（変更）
- `internal/app/presentation/tui/repo/mouse_test.go`（変更）
- `internal/app/presentation/tui/root/root_test.go`（変更）
- `internal/app/presentation/tui/root/scenario_test.go`（変更）
- `internal/app/presentation/tui/root/mouse_test.go`（変更）

- [ ] **Step 1: `repo` のビルダーとフェイク**

`somePRs(n)` / `table_test.go` のループが作る `domain.PR{Number: i + 1, Title: "a pull request"}` を
`domain.Item` で作るようにする（**`Change: &domain.Change{}` を忘れない**）。
`fakeSource.prs` / `issues` を `[]domain.Item` にし、
`itemsFromPRs` / `itemsFromIssues` / `itemFromPR` / `itemFromIssue` を消す。
`ListItems` は変換せずそのまま返す。

- [ ] **Step 2: `root` も同じ**

`fakeSource.prs` と `scenarioSource.pr` を `domain.Item` に。`itemFromPR` / `itemsFromPRs` を消す。

- [ ] **Step 3: リテラルを書き換える**

`table_test.go` の `row()` の検査（draft / approved / changes requested / review required）は
**`Change.IsDraft` と `Change.Review` を読む検査**になる。値は変えない。

- [ ] **Step 4: 検査**

```bash
make check
git status --porcelain | grep -c testdata    # 0 であること
```

- [ ] **Step 5: コミット**

```bash
git add internal/app/presentation/tui/repo internal/app/presentation/tui/root
git commit -m "test: build the repo and root fixtures as domain.Item" \
  -m "Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: `domain.PR` / `domain.Issue` を削除する

**Files:**
- `internal/app/domain/item.go`（変更）
- `internal/app/domain/tags_test.go`（変更）
- `internal/app/adapter/gateway/gh/items_test.go`（変更）

- [ ] **Step 1: gateway のテストに残った参照を直す**

事実 D。`grep -n "domain\.PR\b\|domain\.Issue\b" internal/app/adapter/gateway/gh/items_test.go` で
4 箇所を見て、`want := domain.PR{...}` なら `domain.Item` に、コメントだけなら文言を直す。

- [ ] **Step 2: 型を消す**

`domain/item.go` から `type PR struct` と `type Issue struct` を消す。
**`// PR and Issue carry no JSON tags:` で始まるコメントは、`Item` と `Change` に向けて書き直す**——
そこに書かれている性質（ワイヤ形式を知らない、gateway が翻訳する）は今も真である。

- [ ] **Step 3: `tags_test.go` の `exported` を 24 → 22**

`domain.PR{}` と `domain.Issue{}` の 2 行を消す。事実 E のとおり、
網羅性を照合する対のテストがあるので**数が合わないと落ちる。落ちたら数え直す。**

- [ ] **Step 4: 検査**

```bash
make fmt
make check
make release-check
git status --porcelain | grep -c testdata    # 0 であること
```

- [ ] **Step 5: 消えたことを確認する**

```bash
grep -rn "domain\.PR\b\|domain\.Issue\b" --include="*.go" .   # 0 件
grep -rn "type PR struct\|type Issue struct" internal/app/domain   # 0 件
```

- [ ] **Step 6: コミット**

```bash
git add internal/app
git commit -m "refactor: delete domain.PR and domain.Issue" \
  -m "One pull request or issue is one type now. The four shapes the
modelling found -- domain.PR, domain.Issue, usecase.Item and the views'
copies -- are down to domain.Item and the read model the Work board needs." \
  -m "Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: Item 統合の完了確認

**Files:**
- `docs/superpowers/specs/2026-09-20-item-unification-design.md`（変更）

- [ ] **Step 1: §9 の完了条件を 1 つずつ確かめる**

```bash
test ! -f internal/app/domain/domain.go && echo "domain.go: gone"
find internal/app/domain internal/app/usecase -name "*.go" ! -name "*_test.go" -exec wc -l {} + | sort -rn | head -3
grep -rn "domain\.PR\b\|domain\.Issue\b" --include="*.go" . | wc -l    # 0
grep -rn "usecase\.Item\b" --include="*.go" . | wc -l                  # 0
grep -rn "ItemPR\|ItemIssue" internal/app/usecase --include="*.go" | grep -v _test | wc -l   # 0
grep -rn "ctx context.Context" internal/app/usecase/item.go | wc -l    # 新ポートが ctx を取る
git status --porcelain | grep -c testdata                              # 0
make check && make release-check
```

`domain` と `usecase` に 300 行超の非テストファイルが無いことも見る。

- [ ] **Step 2: 実機確認を利用者に依頼する**

Claude のセッションからは pty を起動できない。**返事を待ってから PR を作る。**

```bash
go run ./cmd/octoscope --repo kukv/octoscope
go run ./cmd/octoscope --repo kukv/octoscope --lang ja
```

Work / Repos / Search の 3 タブ、detail（PR と Issue）、diff、checks、merge。
**Item 統合の最後なので、ここは広めに見てもらう。**

- [ ] **Step 3: 設計書に完了を書く**

§8 PR 5b に `**完了: 2026-09-20。**` を足し、§9 の完了条件のうち
**PR 5a で訂正済みの 1 行以外がすべて満たされたこと**を確認する。
満たされていない条件があれば、消さずに「なぜ満たさなかったか」を書く。

- [ ] **Step 4: コミット**

```bash
git add docs/superpowers/specs/2026-09-20-item-unification-design.md
git commit -m "docs: mark the Item unification done" \
  -m "Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## この PR の完了条件

- **`domain.PR` と `domain.Issue` がリポジトリのどこにも無い**
- `tags_test.go` の `exported` が 22 個で、網羅性の照合テストが通る
- 変換ヘルパー（`prItem` `issueItem` `itemFromPR` `itemFromIssue` `itemsFromPRs`
  `itemsFromIssues`）がすべて消えている
- `Ref.Kind == ItemPR` のフィクスチャが `Change` を必ず持つ
- **本体コードの差分が `domain/item.go` の型削除だけ**
- **golden 354 枚と `testdata/` 全体が無変更**
- `make check` と `make release-check` が通る
- 利用者が実機で確認した

## Item 統合の後に残るもの

- **独立 PR**: `internal/github` の書き込みメソッドに `ctx` を通し、
  `gateway/gh/writes.go` の `_ = ctx` を消す（`go-style.md` への期限付き逸脱の解消）
- 違和感 F-04 以降は未着手。横断（F-11 / F-12）は利用者判断で保留中
