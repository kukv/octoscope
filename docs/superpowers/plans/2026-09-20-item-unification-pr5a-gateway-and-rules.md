# Item 統合 PR 5a（gateway の旧オーバーライド削除、規約更新）実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 誰も呼ばなくなった gateway のオーバーライド 4 本と変換関数 2 本を消し、
`.claude/rules/architecture.md` の 3 節を設計書 §7 のとおり更新する。
**`domain.PR` / `domain.Issue` 自体は残す**——削除はテストフィクスチャを移す PR 5b。

**Architecture:** 削除と文書更新だけ。振る舞いは 1 バイトも変わらない。

**Tech Stack:** Go、golangci-lint（depguard / gofumpt / goimports）、gotestsum、`internal/golden`

**Spec:** `docs/superpowers/specs/2026-09-20-item-unification-design.md`（§7 規約変更提案、§8 PR 5）

---

## 設計書 §8 の「旧ポート 14 本を削除」は実際には 4 本（2026-09-20 に実測、利用者承認済み）

`backend.go` を読んだ結果、**書き込み側 10 本**（`AddPRComment` `AddIssueComment`
`ClosePR` `ReopenPR` `CloseIssue` `ReopenIssue` `EditPRLabels` `EditIssueLabels`
`EditPRAssignees` `EditIssueAssignees`）は **`backend` インターフェースのメソッド**である。
`internal/github` のクライアントが答えるもので、`writes.go` が `g.backend.AddPRComment(...)`
として呼んでいる。**消せない。** PR と Issue を別エンドポイントで呼び分けるのは GitHub の現実で、
gateway はまさにそれを隠すために在る。

**PR 5a で消えるのは gateway の未使用オーバーライド 4 本**（`GetPR` `GetIssue`
`ListPRs` `ListIssues`）**と変換関数 2 本**（`toPR` `toIssue`）。

§9 の「ポート対 14 本が 6 本になっている」は **PR 4 で既に達成済み**である（usecase 側のポートの話）。

## Global Constraints

- **`domain.PR` / `domain.Issue` を消さない。** 削除は PR 5b。テストフィクスチャがまだ使っている
- **`backend` インターフェースを触らない。** 書き込み 10 本も `itemFetcher` / `lister` も残る
- **`internal/github` を触らない**
- **golden 354 枚と `testdata/` 全体を 1 バイトも変えない。** 削除だけなので変わりようがないが、
  変わったら削除しすぎている
- **`Gateway` の埋め込みを外さない。** `Gateway.AddPRComment` はメソッド昇格で今も存在するが、
  やめるには構造変更が要り §8 の範囲外（§7.1 の「変える」はポートの形の話、という解釈。利用者承認済み）
- 各タスクの最後に `make check` が通ること
- コミットメッセージの末尾に `Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>` を付ける

## 着手前に調べた事実（2026-09-20）

### A. 消す 6 つと、その呼び出し元

| 消すもの | 位置 | 呼び出し元 |
|---|---|---|
| `Gateway.GetPR` | `items.go:12` | `items_test.go` と `errors_test.go` だけ |
| `Gateway.GetIssue` | `items.go:21` | 同上 |
| `Gateway.ListPRs` | `lists.go:11` | `lists_test.go` と `errors_test.go` だけ |
| `Gateway.ListIssues` | `lists.go:24` | 同上 |
| `toPR` | `items.go:55` | 上の 2 つ（`items.go:17`、`lists.go:18`）だけ |
| `toIssue` | `items.go:78` | 同上（`items.go:26`、`lists.go:31`） |

**本体コードからの呼び出しは 0。** PR 3 と PR 4 で全部 `GetItem` / `ListItems` に移った。

### B. `toItemFromPR` / `toItemFromIssue` は `toPR` / `toIssue` を呼んでいない

どちらも `gql` のノードから直接 `domain.Item` を組み立てる（`items.go:95〜`）。
**共有している下請けは `parseItemState` `parseReviewDecision` `toComments` `toLabels`
`toAuthors` `toChecksFromContexts` で、これらは残る。**

### C. 消えるテストと、移すテスト

| テスト | 扱い |
|---|---|
| `items_test.go` `TestGetPRTranslatesTheWireShapeIntoTheDomain` | 消す。`TestGetItemTranslatesTheWireShapeIntoTheDomain` が同じ検査をしている |
| `items_test.go` `TestGetIssueTranslatesTheWireShapeIntoTheDomain` | 同上 |
| `lists_test.go` `TestListPRsTranslatesEveryNode` | 消す。`TestListItemsPicksTheQueryByKind` がある |
| `lists_test.go` `TestListIssuesTranslatesEveryNode` | 同上 |
| **`lists_test.go` `TestAListedItemCarriesItsBodyAsText`** | **消さない。`ListItems` に書き換える** |
| `errors_test.go` のオーバーライド表 4 行 | 消す |

`TestAListedItemCarriesItsBodyAsText` は「一覧で来たアイテムは `Body` ではなく `BodyText` を持つ」
という、**他のどのテストも見ていない性質**を守っている（drawer のプレビューが読む先）。
消すと誰も守らなくなる。

### D. depguard に変更は要らない

`.golangci.yml` の 4 ルールはパッケージ単位で書かれており、**パッケージは増減しない。**
確認済み（設計書 §8 PR 5 の条件）。

### E. 規約 3 節の現状の位置

`.claude/rules/architecture.md` の

- 「名前は借りてよい、形は借りない」の表（§7.1）
- 「`usecase.Item` を画面の写しにしない」節（§7.2）
- 「複数の API 呼び出しは `internal/app/usecase` に置く」節（§7.3）

---

### Task 1: gateway の未使用オーバーライドと変換関数を消す

**Files:**
- `internal/app/adapter/gateway/gh/items.go`（変更）
- `internal/app/adapter/gateway/gh/lists.go`（変更）
- `internal/app/adapter/gateway/gh/items_test.go`（変更）
- `internal/app/adapter/gateway/gh/lists_test.go`（変更）
- `internal/app/adapter/gateway/gh/errors_test.go`（変更）

- [ ] **Step 1: `TestAListedItemCarriesItsBodyAsText` を `ListItems` に書き換える（先にやる）**

消す側より先に、残す検査を新しい呼び口に移す。`g.ListPRs(...)` を
`g.ListItems(ctx, "kukv/octoscope", domain.ItemPR)` に、`g.ListIssues(...)` を
`domain.ItemIssue` に。検証している性質（`Body` が空で `BodyText` が入っていること）は変えない。

```bash
make check    # ここで通ること。通らないなら移し方を間違えている
```

- [ ] **Step 2: オーバーライド 4 本を消す**

`items.go` から `GetPR` `GetIssue`、`lists.go` から `ListPRs` `ListIssues` を消す。
**`GetItem` `getPRItem` `getIssueItem` `ListItems` `listPRItems` `listIssueItems` は残す。**

- [ ] **Step 3: `toPR` / `toIssue` を消す**

`items.go` から 2 関数を消す。事実 B のとおり下請け 6 つは `toItemFromPR` /
`toItemFromIssue` が使い続けるので**消さない**。

- [ ] **Step 4: 用済みになったテストを消す**

事実 C の表のとおり 4 つのテストと、`errors_test.go` のオーバーライド表から 4 行。
`lists_test.go` の `fakeLister` は `backend` を満たすために `ListPRs` / `ListIssues` を
**持ち続ける**（`backend` インターフェースは変えない）。

- [ ] **Step 5: 検査**

```bash
make fmt
make check
git status --porcelain | grep -c testdata    # 0 であること
```

- [ ] **Step 6: 消し残しと消しすぎを確認する**

```bash
grep -rn "toPR\|toIssue\b" internal/app/adapter/gateway/gh          # 0 件
grep -rn "func (g \*Gateway) GetPR\|func (g \*Gateway) ListPRs" internal   # 0 件
grep -rn "toItemFromPR\|toItemFromIssue" internal/app/adapter/gateway/gh | wc -l   # 残っていること
grep -rn "domain\.PR\b" internal/app/adapter/gateway/gh --include="*.go" | grep -v _test   # 0 件
```

最後の 1 つが 0 になることが、この PR の実質である——**gateway の本体コードが
`domain.PR` / `domain.Issue` を作らなくなる。**

- [ ] **Step 7: コミット**

```bash
git add internal/app/adapter
git commit -m "refactor: drop the gateway overrides nothing calls any more" \
  -m "GetPR, GetIssue, ListPRs and ListIssues lost their callers in PR 3 and
PR 4, and toPR/toIssue with them. The backend keeps its own PR- and
issue-specific methods: splitting the two is GitHub's doing, which is what
this layer exists to hide." \
  -m "Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: `.claude/rules/architecture.md` の 3 節を更新する

**Files:**
- `.claude/rules/architecture.md`（変更）

設計書 §7 のとおり。**規則の本体を変えるのは §7.1 の表だけ**で、他の 2 節は
主語と例の差し替えである。

- [ ] **Step 1: 「名前は借りてよい、形は借りない」の表（§7.1）**

「変えない」欄の `AddPRComment` `ClosePR` `EditPRLabels` `ListPRs` を「変える」欄へ移す。
理由を本文に足す——**PR と Issue を別エンドポイントで呼び分けるのは GitHub の都合**であり、
規約が禁じている「GitHub が余計に 1 回呼ぶ必要があるから存在する port メソッド」に当たる。

**守れなくなるものも書く**（CLAUDE.md の規約変更手順）: port を読んでも GitHub への
呼び出しが何回になるか分からなくなる。`StartReview` / `SubmitNewReview` を畳んだときと
同じ性質の反転であることを添える。

**`backend` 側は別である**ことを 1 行で書く。`internal/github` のクライアントは今も
`AddPRComment` と `AddIssueComment` を別々に持ち、`Gateway` はそれを埋め込みで昇格している。
規約が言う「形」は usecase に向いたポートの形であって、ACL の内側ではない。

- [ ] **Step 2: 「`usecase.Item` を画面の写しにしない」節（§7.2）**

節の見出しと主語を `domain.Item` に、`Item.PR`（`*domain.PR`）を `Item.Change`
（`*domain.Change`）に差し替える。**規則の本体は変えない**——共通フィールドを足してよいのは
両方にサービス側の対応物があるときだけ、という条件も、「画面に出したいものが無いと思ったら
まずドメイン型を疑え」という指示も残す。

「`domain.Issue` の公開フィールドは 10 個」という 2026-09-07 の実測は、
**`domain.Item` の数に置き換える**（数え直すこと）。

- [ ] **Step 3: 「複数の API 呼び出しは `internal/app/usecase` に置く」節（§7.3）**

「アプリの都合による順序」の例が種別の振り分けだけだったので、差し替える。

- 振り分けは gateway に移った（`GetItem` / `ListItems` / `writes.go`）
- **この層が今は薄いこと**と、それでも残す理由（`SeedCandidates` のアプリ判断、
  ポートの束ね方、横断を入れるときに同じ層を再導入することになる。設計書 §6）
- 「View で `Kind` を `switch` しない」という指示は**残す**。移る先が usecase から gateway に変わっただけ

**実コストを隠さず書く**（設計書 §7.3）: 振り分けのテストが usecase から gateway に降り、
フェイクがドメイン型から `gql` のワイヤ型になった。テストの準備が重くなった。

- [ ] **Step 4: 規約と実装が食い違っていないか読み返す**

```bash
grep -rn "usecase\.Item\|Item\.PR\b" .claude/rules/     # 0 件
grep -rn "ListPRs\|AddPRComment" .claude/rules/architecture.md   # 「変える」欄と backend の説明だけ
```

- [ ] **Step 5: 検査**

```bash
make check
```

- [ ] **Step 6: コミット**

```bash
git add .claude/rules/architecture.md
git commit -m "docs: update the three rules the Item unification replaced" \
  -m "Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: 設計書の訂正と PR の確認

**Files:**
- `docs/superpowers/specs/2026-09-20-item-unification-design.md`（変更）

- [ ] **Step 1: §8 PR 5 を 5a / 5b に割り、「14 本」を訂正する**

- 「gateway の旧ポート 14 本を削除」を「gateway の未使用オーバーライド 4 本と
  `toPR` / `toIssue` を削除」に直し、**書き込み 10 本が `backend` のメソッドであって
  消せない理由**を 2 行で書く
- PR 5 を **5a（gateway + 規約）** と **5b（フィクスチャ + 型削除）** に割る。
  割った理由は、フィクスチャの `domain.PR` / `domain.Issue` 参照が
  **163 箇所 / 16 ファイル**あり、1 回のレビューに乗る量ではないため（§11 の「一括 1 PR」の却下理由と同じ）
- §9 の「ポート対 14 本が 6 本になっている」に **PR 4 で達成済み**と注記する
- 5a に `**完了: 2026-09-20。**` を足す

- [ ] **Step 2: 全体検査**

```bash
make check
make release-check
```

- [ ] **Step 3: 範囲を確認する**

```bash
git diff --stat origin/main -- internal/app/usecase internal/app/presentation internal/github   # 空
git status --porcelain | grep -c testdata    # 0
```

- [ ] **Step 4: コミット**

```bash
git add docs/superpowers/specs/2026-09-20-item-unification-design.md
git commit -m "docs: correct the port count and split PR 5 in two" \
  -m "Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## この PR の完了条件

- `Gateway` に `GetPR` / `GetIssue` / `ListPRs` / `ListIssues` のオーバーライドが無い
- `toPR` / `toIssue` が無く、下請け 6 つ（`parseItemState` `parseReviewDecision`
  `toComments` `toLabels` `toAuthors` `toChecksFromContexts`）は残っている
- **gateway の本体コードに `domain.PR` / `domain.Issue` が 0 件**
- `TestAListedItemCarriesItsBodyAsText` が `ListItems` の上で生きている
- `backend` インターフェースと `internal/github` が無変更
- `domain.PR` / `domain.Issue` はまだ存在する（PR 5b で消す）
- `.claude/rules/architecture.md` の 3 節が設計書 §7 のとおり更新済み
- golden 354 枚と `testdata/` 全体が無変更
- `make check` と `make release-check` が通る

## 次の PR

- **PR 5b**: テストフィクスチャ 163 箇所を `domain.Item` に、`domain.PR` / `domain.Issue` を削除、
  `tags_test.go` の `exported` を 24 → 22
- **独立 PR**: `internal/github` の書き込みメソッドに `ctx` を通し、`writes.go` の `_ = ctx` を消す
