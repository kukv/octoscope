# アーキテクチャ再構成 PR 2b（gateway の導入と `internal/github` の domain 非依存化）実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `internal/github` が GitHub 自身の型だけを返すようにし、domain への変換を `internal/app/adapter/gateway/gh` に集める。ドメインがベンダー非依存であることを、規約文ではなく CI で守る。

**Architecture:** `Gateway` は `backend` interface を**匿名で embed** する 1 つの型である。まだ変換していないメソッドは promotion でそのまま素通しし、変換したメソッドは同名・別シグネチャの明示メソッドで shadow する。これにより port グループ単位で 1 コミットずつ緑を保ったまま進められる。全部変換し終わった最後に、depguard と reflect テストで壁を立てる。

**Tech Stack:** Go 1.27.1、golangci-lint（depguard / gofumpt / goimports）、gotestsum

**Spec:** `docs/superpowers/specs/2026-09-13-architecture-restructure-design.md`（§10 の「PR 2b」、§4、§5、§7）

## Global Constraints

- **画面の見た目を変えない。** golden ファイルの中身が変わったら設計の失敗であって再録の理由ではない。`make golden` を実行しない
- **各コミットで `make check` が通る。** 唯一の例外は Task 12 で、そこは red → green を 1 コミットの中で閉じる
- **識別子の名前は変えない。** `domain.PR` は `domain.PR` のまま。PR 3 でやる改名（不透明ハンドル化など）をここに混ぜない
- **エクスポートするワイヤ型の命名規則:** GitHub 自身の語彙を略さずに使う。`prNode` → `gql.PullRequest`、`issueNode` → `gql.Issue`、`searchNode` → `gql.SearchItem`、`checkDetailNode` → `gql.CheckRun`、`threadNode` → `gql.ReviewThread`、`threadCommentNode` → `gql.ThreadComment`、`commentNode` → `gql.Comment`、`repoJSON` → `api.Repository`、`labelJSON` → `api.Label`。**`Node` / `JSON` / `Resp` のような接尾辞を残さない**（設計 §5「名前は借りてよい」）
- `internal/app/adapter/gateway` は `internal/app/usecase` を import しない。port は暗黙に満たす
- **`gateway` が `gql` を import するのは型のためだけ**である。メソッド呼び出しは必ず embed した `backend` を通す
- **`backend` には `*gql.Client` を並べて embed しない。** 同名メソッドが 2 つの embed から昇格すると、Go はそれをメソッドセットから黙って落とす（呼ぶまでエラーにならない）。embed は 1 つだけ
- モジュールパスは `github.com/kukv/octoscope`
- コミットメッセージの末尾に `Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>` を付ける
- **`.github/workflows/` に falco-actions を足さない**（試験導入後に削除済み）

## 実測値（2026-09-13、着手時点、main = d4126d2）

| 対象 | 値 |
|---|---|
| `internal/github` から `domain` への参照 | 430 箇所 / 81 シンボル |
| `internal/github` の public メソッド | cli 20 / api 20 / gql 21 |
| `internal/github` のテストで domain を使うファイル | 18 |
| domain を**引数に**取る port | 5（`WorkSection` `PendingComment` `ReviewEvent` `RerunScope` `MergeMethod`） |
| エラー sentinel の参照 | `ErrTransient` 20 / `Classify` 11 / `IsFatal` 6 / `ErrUnauthenticated` 6 / `ErrGhNotFound` 4 |
| `domain.IsFatal` を呼ぶビュー | 3（work / repo / search） |

## 変換が要らない port（そのまま promotion で素通しし続ける）

次の 5 グループ・11 メソッドは domain 型に触らないので、**最後まで override を書かない**。

- `commenter`（2）`stateChanger`（4）`labelEditor`（2）`assigneeEditor`（2）`opener`（1）

`opener` は PR 3 で消える（tui が `internal/browser` を直接呼ぶ）。ここでは触らない。

## 変換タスクの手順

Task 2〜9 は同じ手順を踏む。**この順序でなければ途中でビルドが壊れる。**

1. **`backend` の該当メソッドのシグネチャを github の型に変える。**
   この瞬間 `*cli.Client` / `*api.Client` は `backend` を満たさなくなる
2. **`gateway` に override を書く。** domain のシグネチャを持ち、中で `g.backend.X(...)` を呼ぶ
3. **`internal/github` の実装を github の型に変える。** ワイヤ型を export し、`toX()` を gateway へ移す
4. `internal/github` 側のテストを github の型で書き直す。`toX()` の単体テストがあれば gateway へ移す
5. `make check`

1〜3 は**同じコミット**に入れる。途中でビルドは通らない。

各タスクの **Files** に `internal/app/adapter/gateway/gh/backend.go` が必ず入るのはこのためである。

## ファイル構成（完了後）

```
internal/app/adapter/gateway/gh/
  gateway.go     Gateway 型、New、backend の embed
  backend.go     backend interface（6 メソッド以下の小さい interface を embed して組む）
  items.go       PR / Issue の変換
  lists.go       Label / Author / 一覧の変換
  work.go        WorkSection → 検索文字列、WorkItem / RepoCount の変換
  repos.go       RepoCandidate の変換
  review.go      FileDiff / ReviewContext / PendingComment / ReviewEvent の変換
  checks.go      Checks / CheckRun / LogLine / RerunScope の変換
  merge.go       MergeContext / MergeMethod の変換
  errors.go      github の sentinel → domain の sentinel
internal/github/{cli,api,gql}/    domain を import しない
internal/app/domain/              stdlib のみ。struct タグなし
```

## 検証の考え方

この PR は**意味のある変更**を含む。PR 1 の「振る舞い不変」とは違い、型が移り、変換の置き場所が変わる。したがって golden テストが通ることは必要条件でしかない。各タスクで次を要求する。

1. `make check` が通り、golden の中身が変わらない
2. **移した変換のテストが、変換を壊すと落ちる**こと（破壊確認を実際に行い、出力を報告に貼る）
3. `internal/github` の該当パッケージが、そのタスクの範囲で domain を参照しなくなっていること

---

### Task 1: `gateway/gh` の骨組みと `backend` interface

override を 1 つも書かない。全メソッドが promotion で素通しし、`make check` が通る状態を作る。

**Files:**
- Create: `internal/app/adapter/gateway/gh/gateway.go`
- Create: `internal/app/adapter/gateway/gh/backend.go`
- Create: `internal/app/adapter/gateway/gh/gateway_test.go`
- Modify: `cmd/octoscope/main.go`
- Modify: `.golangci.yml`（depguard に `gateway-layer`）

**Interfaces:**
- Consumes: `cli.Client`、`api.Client`（既存）
- Produces: `gh.Gateway`、`gh.New(b backend) *Gateway`。`Gateway` は `usecase.New` の第 1 引数として通る

- [ ] **Step 1: 現状を記録する**

```bash
make check
git diff --stat   # 空であること
grep -rc 'domain\.' --include='*.go' internal/github | grep -v ':0' | wc -l   # 22 を期待
```

- [ ] **Step 2: `backend.go` を書く**

`usecase` の port グループに対応する小さい interface を並べ、`backend` がそれらを embed する。
**1 つの宣言に直接並べるメソッドは 6 個まで**（`.claude/rules/architecture.md`）。

この時点のシグネチャは**今の `internal/github` のまま**、すなわち domain 型を返す。

```go
// Package gh turns what a GitHub client answers into the domain's values,
// and is what the usecase layer's ports are satisfied by.
//
// Gateway embeds backend anonymously. A method this package has not
// converted yet is promoted from the embedded value unchanged; a converted
// one is shadowed by an explicit method whose signature speaks the domain.
// That is what lets the conversion land one port group at a time.
package gh

import (
	"context"

	"github.com/kukv/octoscope/internal/app/domain"
)

type itemFetcher interface {
	GetPR(ctx context.Context, repo string, number int) (domain.PR, error)
	GetIssue(ctx context.Context, repo string, number int) (domain.Issue, error)
}

// ... commenter, stateChanger, labelEditor, assigneeEditor, lister,
// crossRepoLister, repoFinder, reviewFetcher, reviewer, opener,
// checksFetcher, merger を同様に宣言する。
// internal/app/usecase/usecase.go:13-109 の port 宣言をそのまま写せばよい。
// settingsStore / repoStore / queryStore は store のものなので写さない。

// backend is what a GitHub client answers. It is declared here, on the
// consumer's side, rather than exported by the client packages.
type backend interface {
	itemFetcher
	commenter
	stateChanger
	labelEditor
	assigneeEditor
	lister
	crossRepoLister
	repoFinder
	reviewFetcher
	reviewer
	opener
	checksFetcher
	merger
}
```

- [ ] **Step 3: `gateway.go` を書く**

```go
package gh

// Gateway answers the usecase layer's ports by way of one GitHub client.
type Gateway struct{ backend }

// New wires a gateway to a client.
func New(b backend) *Gateway { return &Gateway{backend: b} }
```

`backend` は非公開だが、`New` が受け取れる。`cmd/octoscope` は `*cli.Client` / `*api.Client` を
渡すだけでよい（Go は暗黙に満たす）。

- [ ] **Step 4: `main.go` を組み替える**

```go
	var uc *usecase.Usecase
	if ghClient != nil {
		uc = usecase.New(gh.New(ghClient), store)
	} else {
		uc = usecase.New(gh.New(apiClient), store)
	}
```

`internal/app/adapter/gateway/gh` の import を足す。

- [ ] **Step 5: 両方のクライアントが `backend` を満たすことをコンパイル時に固定する**

`internal/app/adapter/gateway/gh/gateway_test.go`。パッケージは `gh`（非公開の
`backend` を名指しするため）。

```go
package gh

import (
	"github.com/kukv/octoscope/internal/github/api"
	"github.com/kukv/octoscope/internal/github/cli"
)

// Both clients have to answer everything the gateway promotes or overrides.
// A conversion that changes backend's signature without changing the client
// breaks here, at compile time, rather than at the call site.
var (
	_ backend = (*cli.Client)(nil)
	_ backend = (*api.Client)(nil)
)
```

- [ ] **Step 6: depguard に `gateway-layer` を足す**

`.golangci.yml` の `depguard.rules` に、`datasource-layer` の隣に置く。

```yaml
          # gateway はサービスの型を domain に訳す。上の層は知らない。
          gateway-layer:
            files:
              - "**/internal/app/adapter/gateway/**"
            deny:
              - pkg: github.com/kukv/octoscope/internal/app/usecase
                desc: gateway は port を暗黙に満たす。usecase を import しない
              - pkg: github.com/kukv/octoscope/internal/app/presentation
                desc: gateway は UI を知らない
              - pkg: github.com/kukv/octoscope/internal/app/config
                desc: gateway は設定ファイルを知らない
              - pkg: github.com/kukv/octoscope/internal/i18n
                desc: gateway は翻訳しない
```

- [ ] **Step 7: ルールが効くことを確かめる**

```bash
cat > internal/app/adapter/gateway/gh/depguard_probe.go <<'EOF'
package gh

import "github.com/kukv/octoscope/internal/app/usecase"

var _ = usecase.New
EOF
make lint
```

期待: `gateway は port を暗黙に満たす。usecase を import しない` で落ちる。

```bash
rm internal/app/adapter/gateway/gh/depguard_probe.go
make lint
```

期待: 通る。`git status --porcelain` が空であることを確認する。

- [ ] **Step 8: 検査してコミット**

```bash
make fmt
make check
```

期待: すべて PASS。テスト本数が着手前と同じであること（override を 1 つも書いていないので、
振る舞いは 1 バイトも変わっていない）。

```bash
git add -A
git commit -m "$(cat <<'EOF'
feat: put a gateway between the clients and the usecase layer

It converts nothing yet. Gateway embeds the backend interface, so every
method is promoted exactly as the client wrote it, and the run behaves
identically. What this buys is the seam: the next commits shadow one port
group at a time, and each one still builds.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: `itemFetcher` と `lister` — PR / Issue / Label / Author

タグ付きの葉の型（`domain.Author` `domain.Label` `domain.Comment`）をここで domain から出す。
最後の depguard を通すために最初に片付けるべきものである。

**Files:**
- Modify: `internal/app/adapter/gateway/gh/backend.go`
- Create: `internal/app/adapter/gateway/gh/items.go`、`items_test.go`
- Create: `internal/app/adapter/gateway/gh/lists.go`、`lists_test.go`
- Modify: `internal/github/gql/items.go`、`comments.go`、`items_test.go`、`comments_test.go`
- Modify: `internal/github/cli/cli.go`、`cli_test.go`
- Modify: `internal/github/api/lists.go`、`lists_test.go`
- Modify: `internal/app/domain/domain.go`（`Author` `Label` `Comment` の json タグを外す）

**対象メソッド:** `GetPR` `GetIssue` `ListPRs` `ListIssues` `ListLabels` `ListAssignees` `RepoName`

**Interfaces:**
- Consumes: Task 1 の `Gateway` と `backend`
- Produces:
  - `gql.PullRequest`（旧 `prNode`）、`gql.Issue`（旧 `issueNode`）、`gql.Comment`（旧 `commentNode`）、`gql.Label`、`gql.Author`
  - `api.Label`（旧 `labelJSON`）、`api.Author`
  - gateway: `toPR(gql.PullRequest) domain.PR`、`toIssue(gql.Issue) domain.Issue`、`toLabels([]gql.Label) []domain.Label`、`toAuthors`、`toComments`

- [ ] **Step 1: `domain.Author` / `Label` / `Comment` が今どこでデコードされているか数える**

```bash
grep -rn 'domain\.Label\|domain\.Author\|domain\.Comment' --include='*.go' internal/github | grep -v _test
```

出た 8 箇所が、この Task で github 側の型に置き換わる対象である。数を報告に書く。

- [ ] **Step 2: github 側にワイヤ型を作る**

`internal/github/gql/items.go` の `prNode` を `PullRequest` に export し、`Labels.Nodes` と
`Assignees.Nodes` の要素型を `domain.Label` / `domain.Author` から、この package 自身の
`Label` / `Author` に変える。

```go
// Label is a label as GitHub's documents select it.
type Label struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

// Author is the login GitHub attributes something to.
type Author struct {
	Login string `json:"login"`
}
```

`comments.go` の `commentNode` も `Comment` に export し、`Author` をこの型にする。
`api/lists.go` の `labelJSON` は `api.Label` に、`domain.Author` のデコードは `api.Author` に。
`cli/cli.go` の `var labels []domain.Label` / `var users []domain.Author` も同様。

- [ ] **Step 3: `toX()` を gateway へ移す**

`gql/items.go` の `toPR()` `toIssue()`、`gql/comments.go` の `toComments()` を
`internal/app/adapter/gateway/gh/items.go` に移す。**中身の写し間違いに注意** —
`domain.ParseItemState` / `domain.ParseReviewDecision` の呼び出しはこの時点では
domain のまま残す（Task 10 で `internal/github` へ移す）。

- [ ] **Step 4: `backend` と gateway の override を書く**

「変換タスクの手順」の 1〜3 のとおり、同じコミットの中で順に行う。

- [ ] **Step 5: `domain` から json タグを外す**

`internal/app/domain/domain.go` の `Author` `Label` `Comment` の `json:"..."` を消す。

- [ ] **Step 6: 破壊確認**

移した変換が本当に守られているかを確かめる。`toPR` の中の 1 行（たとえば
`Review: domain.ParseReviewDecision(n.ReviewDecision)`）をゼロ値に置き換えて
`go test ./internal/app/adapter/gateway/...` を走らせ、**落ちること**を確認してから戻す。
両方の出力を報告に貼る。

落ちないなら、そのテストは変換を守っていない（`.claude/rules/testing.md`）。

- [ ] **Step 7: 検査してコミット**

```bash
make fmt
make check
git status --porcelain | grep -E '\.golden|testdata' || echo "golden と testdata は無変更"
```

```bash
git add -A
git commit -m "$(cat <<'EOF'
refactor: move pull request and issue mapping into the gateway

Author, Label and Comment carried json tags in the domain and were
decoded straight into: the wire shape and the application's shape were
the same type. They are two types now, and the gateway is where one
becomes the other.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: `crossRepoLister` — WorkSection / WorkItem / RepoCount

`gql/search.go` が `domain.WorkSection` から GitHub の検索文字列を組んでいる。
**アプリの知識が transport に入っている**ので、これを gateway に出す。

**Files:**
- Modify: `internal/app/adapter/gateway/gh/backend.go`
- Create: `internal/app/adapter/gateway/gh/work.go`、`work_test.go`
- Modify: `internal/github/gql/search.go`、`search_test.go`、`repo_counts.go`、`repo_counts_test.go`

**対象メソッド:** `ListWorkSection` `SearchItems` `RepoCounts`

**Interfaces:**
- Produces:
  - `gql.SearchItem`（旧 `searchNode`）、`gql.RepoCount`
  - `gql.ListWorkSection` は**消える**。代わりに `gql.SearchItems(ctx, query string)` だけが残る
  - gateway: `workQuery(domain.WorkSection) string`、`toWorkItem(gql.SearchItem) domain.WorkItem`、`toRepoCount`

- [ ] **Step 1: いまの検索文字列の組み立てを読む**

```bash
grep -n 'SectionReviewRequested\|SectionYourPRs\|SectionAssigned\|SectionMentioned' -B3 -A8 internal/github/gql/search.go
```

4 カラムそれぞれがどんなクエリになるかを控える。**この文字列を 1 文字も変えない。**
変えると Work ボードの中身が変わる。

- [ ] **Step 2: `workQuery` を gateway に書き、テストで固定する**

`work_test.go` に、4 カラムそれぞれの期待クエリ文字列を**べた書き**する。
実装が組み立てたものを期待値に書かない（`.claude/rules/testing.md`、
実装の鏡は仕様ではない）。Step 1 で控えた文字列をそのまま書く。

- [ ] **Step 3: 「変換タスクの手順」1〜3 を実行する**

`gql.ListWorkSection` を消し、gateway の `ListWorkSection` が `workQuery` で文字列を作って
`g.backend.SearchItems(ctx, query)` を呼ぶ形にする。

- [ ] **Step 4: 破壊確認**

`workQuery` の 1 カラムの文字列を変えて `go test ./internal/app/adapter/gateway/...` が
落ちることを確認し、戻す。出力を報告に貼る。

- [ ] **Step 5: Work ボードの golden が変わっていないことを確かめる**

```bash
make check
git status --porcelain | grep -E '\.golden' || echo "golden は無変更"
```

golden が変わったら、それは検索クエリを変えてしまったということである。**再録しない。**
Step 1 に戻る。

- [ ] **Step 6: コミット**

```bash
git add -A
git commit -m "$(cat <<'EOF'
refactor: build the Work board's queries in the gateway

Which four columns the board has is the application's idea. The GraphQL
package was turning that idea into search strings, which is one layer too
low; it now takes a query and asks it.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: `repoFinder` — RepoCandidate

**Files:**
- Modify: `internal/app/adapter/gateway/gh/backend.go`
- Create: `internal/app/adapter/gateway/gh/repos.go`、`repos_test.go`
- Modify: `internal/github/cli/search.go`、`own_repos.go` とそのテスト
- Modify: `internal/github/api/repos.go`、`repos_test.go`

**対象メソッド:** `SearchRepos` `ListOwnRepos` `ListOrgs`（`ListOrgs` は `[]string` なので変換不要、
`backend` にそのまま残す）

**Interfaces:**
- Produces: `api.Repository`（旧 `repoJSON`）、`cli.Repository`、
  gateway: `toRepoCandidate` を両方の型について 1 つずつ

**注意:** cli と api で型が別なので、gateway の override も 2 つの入力を扱う。
`backend` の `SearchRepos` は 1 つのシグネチャしか持てないので、**両方のクライアントが
同じ github 側の型を返すようにそろえる**こと。`gql` のような共有の置き場所が無いなら、
`internal/github` のルートに小さな共有型を置いてよい（`internal/github/repository.go`）。
その判断をしたら報告に書く。

- [ ] **Step 1〜3:** 「変換タスクの手順」1〜3
- [ ] **Step 4: 破壊確認**（`toRepoCandidate` の `Stars` をゼロ固定にして落ちることを見る）
- [ ] **Step 5:** `make check` と golden 無変更の確認
- [ ] **Step 6: コミット**

```
refactor: move repository suggestions into the gateway
```

---

### Task 5: `reviewFetcher` — FileDiff / ReviewContext

`domain.ParseFilesAPI` と `prFileJSON` はここで `internal/github` へ移す
（設計 §7、PR 3 から前倒し済み）。`domain.ParseDiff`（git の unified diff）は **domain に残す**。

**Files:**
- Modify: `internal/app/adapter/gateway/gh/backend.go`
- Create: `internal/app/adapter/gateway/gh/review.go`、`review_test.go`
- Modify: `internal/github/gql/review.go`、`review_test.go`
- Modify: `internal/github/cli/diff.go`、`api/diff.go` とそのテスト
- Modify: `internal/app/domain/diff_parse.go`（`ParseFilesAPI` と `prFileJSON` を外す）
- Create: `internal/github/files.go`（`ParseFilesAPI` の新しい置き場所。両バックエンドが使う）

**対象メソッド:** `PRDiff` `PRReviewContext`

**Interfaces:**
- Produces: `gql.ReviewThread`（旧 `threadNode`）、`gql.ThreadComment`、`github.PRFile`（旧 `prFileJSON`）、
  `github.ParseFilesAPI(out []byte) ([]github.PRFile, error)`、
  gateway: `toFileDiff`、`toReviewContext`、`toReviewThread`

- [ ] **Step 1: `domain/diff_parse.go` から `encoding/json` が消えるか確かめる**

```bash
grep -n 'encoding/json' internal/app/domain/*.go
```

`ParseFilesAPI` と `prFileJSON` を外したあと、`domain` に `encoding/json` の import が
1 つも残らないこと。**残るなら Task 12 の depguard が通らない。** 残る場合は何が使っているかを
報告して止まる。

- [ ] **Step 2〜4:** 「変換タスクの手順」1〜3 と、`diff_parse_test.go` のうち
  `ParseFilesAPI` を見るテスト（`TestFileStatusFromAPIMapsEveryValue`
  `TestPRFileWithNoPatchIsMarkedOmitted` `TestPRFileRenameCarriesBothPaths`）を
  `internal/github` へ移す
- [ ] **Step 5: 破壊確認**（`toFileDiff` の `OldPath` を落として、rename のテストが落ちることを見る）
- [ ] **Step 6:** `make check` と golden 無変更の確認
- [ ] **Step 7: コミット**

```
refactor: move the diff and review mapping into the gateway
```

---

### Task 6: `reviewer` — PendingComment / ReviewEvent（引数の変換）

ここから**引数方向の変換**が入る。gateway の override が domain の値を受け取り、
github の値に直してから `g.backend` を呼ぶ。

**設計 §6 の port の畳み込み（`StartReview` / `SubmitNewReview` を消す）は PR 3 で行う。**
このタスクは型だけを移す。

**Files:**
- Modify: `internal/app/adapter/gateway/gh/backend.go`、`review.go`、`review_test.go`
- Modify: `internal/github/gql/review.go`、`review_test.go`

**対象メソッド:** `StartReview` `AddReviewThread` `SubmitReview` `SubmitNewReview` `DiscardReview`

**Interfaces:**
- Produces: `gql.PendingComment`、`gql.ReviewEvent`（`APPROVE` / `REQUEST_CHANGES` / `COMMENT` を
  そのまま持つ）、gateway: `fromReviewEvent(domain.ReviewEvent) gql.ReviewEvent`、
  `fromPendingComment(domain.PendingComment) gql.PendingComment`

- [ ] **Step 1: いまの enum の対応を読む**

```bash
grep -n 'EventApprove\|EventRequestChanges\|EventComment' -B2 -A6 internal/github/gql/review.go
```

GraphQL に送っている文字列を控える。**1 文字も変えない。**

- [ ] **Step 2: 逆方向の変換を書き、全値を覆うテストを書く**

`domain.ReviewEvent` の定義（`internal/app/domain/review.go`）を読み、**すべての値**に
ついてテストする。ひとつでも漏れると、その操作が黙って別の意味になる。

- [ ] **Step 3〜4:** 「変換タスクの手順」1〜3
- [ ] **Step 5: 破壊確認**（`fromReviewEvent` の approve と comment を入れ替えて落ちることを見る）
- [ ] **Step 6:** `make check`
- [ ] **Step 7: コミット**

```
refactor: convert review events at the gateway
```

---

### Task 7: `checksFetcher` — Checks / CheckRun / LogLine / RerunScope

`domain.NewLogLine` が Actions のタイムスタンプ接頭辞を解釈している（設計 §7）。
**解釈を `internal/github` へ移し、domain には素の構築だけ残す。**

**Files:**
- Modify: `internal/app/adapter/gateway/gh/backend.go`
- Create: `internal/app/adapter/gateway/gh/checks.go`、`checks_test.go`
- Modify: `internal/github/gql/checks.go`、`checks_test.go`
- Modify: `internal/github/cli/checks.go`、`api/checks.go`、`api/joblog.go` とそのテスト
- Modify: `internal/app/domain/checks.go`、`checks_test.go`

**対象メソッド:** `PRChecks` `JobLog` `RerunWorkflow`

**Interfaces:**
- Produces: `gql.CheckRun`（旧 `checkDetailNode`）、`github.LogLine`、
  `github.ParseLogLine(step, raw string) github.LogLine`（タイムスタンプの解釈はここ）、
  `gql.RerunScope`、gateway: `toChecks`、`toCheckRun`、`toLogLine`、`fromRerunScope`

- [ ] **Step 1: `NewLogLine` が何をしているか読む**

```bash
grep -n 'NewLogLine' -A 20 internal/app/domain/checks.go
```

タイムスタンプの切り出しが Actions のログ形式に固有かどうかを確かめる。
**固有なら github へ、そうでないなら domain に残す。** 判断と理由を報告に書く。

domain 側には `LogLine{Step, Text, ...}` を素直に組む形だけ残す。
`domain/checks_test.go` の `TestALogLineKeepsTheStampOutOfTheText`
`TestAnUnstampedLineKeepsItsWholeText` `TestAFirstWordThatIsNotATimestampStays` の 3 本は
解釈と一緒に `internal/github` へ移す。

- [ ] **Step 2〜4:** 「変換タスクの手順」1〜3
- [ ] **Step 5: 破壊確認**（`toCheckRun` の `State` をゼロ固定にして、checks ビューの
  golden テストが落ちることを見る。**落ちなければ、checks の golden は状態を描いていない**ので
  そのことを報告する）
- [ ] **Step 6:** `make check` と golden 無変更の確認
- [ ] **Step 7: コミット**

```
refactor: move check and log mapping into the gateway
```

---

### Task 8: `merger` — MergeContext / MergeMethod

**Files:**
- Modify: `internal/app/adapter/gateway/gh/backend.go`
- Create: `internal/app/adapter/gateway/gh/merge.go`、`merge_test.go`
- Modify: `internal/github/gql/merge.go`、`merge_test.go`

**対象メソッド:** `PRMergeContext` `MergePR` `EnableAutoMerge` `DisableAutoMerge`

**Interfaces:**
- Produces: `gql.MergeContext`、`gql.MergeMethod`、
  gateway: `toMergeContext`、`fromMergeMethod(domain.MergeMethod) gql.MergeMethod`

**注意:** `domain.MergeContext` には `Block()` と `CanAutoMerge()` というドメインのルールが
乗っている（`internal/app/domain/merge.go:71,92`）。**これは domain に残す。**
gateway が作るのは `MergeContext` の値だけで、判断は domain のメソッドが持つ。

- [ ] **Step 1: `Mergeable` と `MergeState` の対応を読む**

```bash
grep -n 'MergeableYes\|MergeStateClean\|MergeStateBlocked' -B3 -A8 internal/github/gql/merge.go
```

GraphQL の enum 綴りと domain の値の対応を控える。**全値を覆うテストを書く。**

- [ ] **Step 2〜4:** 「変換タスクの手順」1〜3
- [ ] **Step 5: 破壊確認**（`fromMergeMethod` の squash と rebase を入れ替えて落ちることを見る）
- [ ] **Step 6:** `make check` と golden 無変更の確認
- [ ] **Step 7: コミット**

```
refactor: move merge mapping into the gateway
```

---

### Task 9: 残りの参照を掃除する

Task 2〜8 で拾い切れなかった `domain.` 参照を潰す。**エラー sentinel は Task 10 で扱うので
ここでは触らない。**

**Files:** Step 1 の結果しだい

- [ ] **Step 1: 何が残っているかを数える**

```bash
grep -rhno 'domain\.[A-Z][A-Za-z]*' --include='*.go' internal/github | sed 's/.*://' | sort | uniq -c | sort -rn
```

期待: `ErrTransient` `ErrUnauthenticated` `ErrGhNotFound` `Classify` `IsFatal`
`ParseItemState` `ParseReviewDecision` `SplitRepo` だけが残る。
**それ以外が残っていたら、それは Task 2〜8 の取りこぼしである。** ここで潰す。

- [ ] **Step 2: `ParseItemState` と `ParseReviewDecision` を `internal/github` へ移す**

GitHub の enum 綴りを読む処理なので domain には居られない（設計 §7）。
`internal/github/parse.go` に移し、gateway は github 側のそれを呼ぶ。
`domain/domain_test.go` の `TestParseItemState` `TestParseReviewDecision` も一緒に移す。

- [ ] **Step 3: `SplitRepo` を domain から外す**

`owner/name` 固定の分解は GitHub の形である（設計 §7）。`internal/github` へ移す。
tui 側の 2 箇所（`search/render.go:366`、`repo/add.go:83`）は **presentation が
`internal/github` を import できない**ので、別の解決が要る:

- `search/render.go` は「`owner/name` から name だけを取る」ための呼び出し。
  表示用の整形なので、その場で `strings.Cut` すれば足りる
- `repo/add.go` は入力の検証。ダイアログが受け付ける形の検証なので、
  presentation 側にその判定を置いてよい

**どちらもふるまいを変えないこと。** `domain.SplitRepo` の
`strings.TrimSpace` の扱い（前後に空白があれば不正とする）を落とさない。
`domain/domain_test.go` の `TestSplitRepo` は移し先へ持っていく。

- [ ] **Step 4: 検査してコミット**

```bash
make check
```

```
refactor: move GitHub's spellings out of the domain
```

---

### Task 10: エラー sentinel（**この順序でなければ壊れる**）

**なぜ最後なのか。** `domain.IsFatal` は 3 つのビュー（work / repo / search）が
「ユーザーが手を打たないと何も動かないか」を判断するのに使っている。
sentinel を早い段階で `internal/github` に移すと、**まだ override していない
素通しのメソッドが github の sentinel を返し、`domain.IsFatal` がそれを認識しなくなる。**
認証失敗でエラー画面が出なくなる、という形で壊れる。

Task 9 までで全メソッドが override 済みになっているので、ここで安全に移せる。

**Files:**
- Create: `internal/github/errors.go`
- Create: `internal/app/adapter/gateway/gh/errors.go`、`errors_test.go`
- Modify: `internal/app/domain/domain.go`
- Modify: `internal/github/cli/cli.go`、`api/transport.go` とそのテスト
- Modify: `internal/app/adapter/gateway/gh/*.go`（全 override がエラーを包む）

**Interfaces:**
- Produces:
  - `github.ErrNotInstalled`（旧 `domain.ErrGhNotFound`）、`github.ErrTransient`、
    `github.ErrUnauthenticated`、`github.Classify`
  - domain: `ErrBackendUnavailable`（中立名）、`ErrTransient`、`ErrUnauthenticated` を残し、
    `IsFatal` は**中立な sentinel だけ**を見る。`Classify` は domain に残す
  - gateway: `wrap(err error) error` — github の sentinel を domain の sentinel に訳す

- [ ] **Step 1: いま何がどこで分類されているかを数える**

```bash
grep -rn 'domain\.Err\|domain\.Classify\|domain\.IsFatal' --include='*.go' internal/github | grep -v _test
```

47 箇所前後を期待する。実数を報告に書く。

- [ ] **Step 2: `wrap` を書き、全 sentinel を覆うテストを書く**

```go
// wrap turns a client's sentinel into the domain's. The client names what
// went wrong in its own service's terms -- "the gh binary is not on PATH" --
// and the application only needs to know which of its own kinds that is.
func wrap(err error) error { ... }
```

テストは **github の 4 つの sentinel すべて**について、`errors.Is` で domain 側の
対応する sentinel になることを確かめる。加えて、**訳されなかったエラーがそのまま
通ること**（未知のエラーを握り潰さない）も確かめる。

エラーの文言（`Error()` が返す文字列）は**変えない**。`.claude/rules/errors.md` は
「GitHub が言ったことは訳さずそのまま出す」としている。

- [ ] **Step 3: 全 override でエラーを包む**

`gateway/gh` の各ファイルの override が返すエラーを `wrap` に通す。
**包み忘れが 1 つでもあると、そのメソッドの失敗だけエラー画面が出なくなる。**

- [ ] **Step 4: sentinel を移す**

`internal/github/errors.go` に `ErrNotInstalled` / `ErrTransient` / `ErrUnauthenticated` /
`Classify` を置き、`cli` と `api` をそちらに向ける。
`domain.ErrGhNotFound` は `domain.ErrBackendUnavailable` に改名する
（gh CLI という特定実装の名前をドメインから消す）。tui の 8 参照を追随させる。

- [ ] **Step 5: 破壊確認 — これがこのタスクの要である**

1. `wrap` から `ErrUnauthenticated` の分岐を 1 つ消す
2. `go test ./internal/app/... ` を走らせ、**落ちること**を確認する
3. 戻す

落ちないなら、認証失敗でエラー画面が出なくなる変更を誰も検出できないということである。
その場合は `internal/app/presentation/tui/work/work_test.go` などに、
**fatal なエラーでエラー画面に遷移する**ことを見るテストが要る。あるか確かめて、
無ければ書く。両方の出力を報告に貼る。

- [ ] **Step 6:** `make check` と golden 無変更の確認
- [ ] **Step 7: コミット**

```
refactor: give the domain its own names for what went wrong
```

---

### Task 11: `internal/github` から domain が消えたことを確かめる

**Files:** なし（確認のみ。取りこぼしがあれば潰す）

- [ ] **Step 1: 数える**

```bash
grep -rn 'app/domain' --include='*.go' internal/github
```

期待: **出力なし**。テストファイルも含めてゼロ。

残っていたら、それがどのメソッドのものかを特定して潰す。潰せない理由があるなら、
**それは設計の見落とし**なので報告して止まる。

- [ ] **Step 2: domain に struct タグが残っていないか数える**

```bash
grep -rn 'json:"\|yaml:"' internal/app/domain/*.go
```

期待: 出力なし。

- [ ] **Step 3: domain が何を import しているか数える**

```bash
grep -rhn '"' internal/app/domain/*.go | grep -A100 'import' | grep '\s"' | sort -u
```

期待: stdlib だけ（`errors` `strings` `time` `bufio` `bytes` `strconv` `fmt`）。
`encoding/json` が無いこと。

- [ ] **Step 4:** 3 つとも満たしていればコミットなし。潰した場合のみコミットする

---

### Task 12: 壁を立てる — depguard と reflect テスト

**このタスクだけ、コミットの途中で `make check` が落ちてよい。** 最後には緑にする。

**Files:**
- Create: `internal/app/domain/tags_test.go`
- Modify: `.golangci.yml`

**Interfaces:**
- Produces: なし（検査のみ）

- [ ] **Step 1: reflect テストを書く**

`internal/app/domain/tags_test.go`。パッケージは `domain_test`。

エクスポート型の一覧は**手で持つ**。自動で歩くと、新しい型が足されたときに黙って
対象から漏れる。そのため**一覧の長さが、ソース中の `^type [A-Z].* struct` の数と
一致すること**を検査する 2 本目を置く。

```go
package domain_test

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/kukv/octoscope/internal/app/domain"
)

// exported is every struct this package publishes. The list is written by
// hand so that a new type has to be added here deliberately; the test below
// it fails when someone forgets.
var exported = []any{
	domain.Author{}, domain.Label{}, domain.Comment{}, domain.PR{},
	domain.Issue{}, domain.ItemRef{}, domain.CheckRun{}, domain.Checks{},
	domain.WorkItem{}, domain.RepoCount{}, domain.RepoCandidate{},
	domain.SavedQuery{}, domain.LogLine{}, domain.DiffLine{}, domain.Hunk{},
	domain.FileDiff{}, domain.MergeContext{}, domain.ThreadComment{},
	domain.ReviewThread{}, domain.PendingComment{}, domain.ReviewContext{},
}

// TestNoDomainTypeCarriesASerialisationTag is the wall this package's
// neutrality stands on. A tag here means some wire format reached in and
// made the application's shape its own -- which is how Author, Label and
// Comment ended up being decoded straight into before this was written.
func TestNoDomainTypeCarriesASerialisationTag(t *testing.T) {
	t.Parallel()
	for _, v := range exported {
		typ := reflect.TypeOf(v)
		for i := range typ.NumField() {
			f := typ.Field(i)
			if f.Tag != "" {
				t.Errorf("%s.%s carries a struct tag %q", typ.Name(), f.Name, f.Tag)
			}
		}
	}
}

// TestTheTagListCoversEveryExportedStruct keeps the list above honest: a
// type added to the package without being added here would go unchecked.
func TestTheTagListCoversEveryExportedStruct(t *testing.T) {
	t.Parallel()
	decl := regexp.MustCompile(`(?m)^type ([A-Z]\w*) struct`)
	seen := map[string]bool{}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range decl.FindAllStringSubmatch(string(raw), -1) {
			seen[m[1]] = true
		}
	}
	listed := map[string]bool{}
	for _, v := range exported {
		listed[reflect.TypeOf(v).Name()] = true
	}
	for name := range seen {
		if !listed[name] {
			t.Errorf("domain.%s is not in the list the tag test walks", name)
		}
	}
	for name := range listed {
		if !seen[name] {
			t.Errorf("the list names domain.%s, which this package does not declare", name)
		}
	}
}
```

- [ ] **Step 2: このテストが本当に噛むことを確かめる**

```bash
# 1. タグを 1 つ戻す
sed -i 's/^\tLogin string$/\tLogin string `json:"login"`/' internal/app/domain/domain.go
go test ./internal/app/domain/ -run TestNoDomainTypeCarriesASerialisationTag -v
```

期待: **FAIL**（`Author.Login carries a struct tag`）。

```bash
git checkout -- internal/app/domain/domain.go
go test ./internal/app/domain/ -run TestNoDomainTypeCarriesASerialisationTag -v
```

期待: PASS。

一覧のほうも噛むことを確かめる。`exported` から 1 行消して
`TestTheTagListCoversEveryExportedStruct` が落ちること、戻して通ることを見る。
**4 つの出力すべてを報告に貼る。**

- [ ] **Step 3: depguard に 2 つの deny を足す**

```yaml
          domain-layer:
            files:
              - "**/internal/app/domain/**"
            deny:
              # （既存の deny はそのまま）
              - pkg: encoding/json
                desc: ドメインはワイヤ形式を知らない。デコードは internal/github が行う
```

```yaml
          github-layer:
            files:
              - "**/internal/github/**"
            deny:
              # （既存の deny はそのまま。次の 1 行に置き換えられるものは整理する）
              - pkg: github.com/kukv/octoscope/internal/app
                desc: クライアントはアプリケーションを知らない。変換は gateway が行う
```

`github-layer` の既存の 3 つの deny（`usecase` / `presentation` / `config`）は
`internal/app` の prefix に含まれるので、1 行にまとめてよい。まとめたら報告に書く。

- [ ] **Step 4: 2 つの deny が効くことを確かめる**

```bash
cat > internal/app/domain/depguard_probe.go <<'EOF'
package domain

import _ "encoding/json"
EOF
make lint
rm internal/app/domain/depguard_probe.go
```

期待: 1 回目は `ドメインはワイヤ形式を知らない` で落ち、削除後は通る。

```bash
cat > internal/github/gql/depguard_probe.go <<'EOF'
package gql

import _ "github.com/kukv/octoscope/internal/app/domain"
EOF
make lint
rm internal/github/gql/depguard_probe.go
```

期待: 1 回目は `クライアントはアプリケーションを知らない` で落ち、削除後は通る。
**両方の lint 出力を報告に貼る。**

- [ ] **Step 5: 検査してコミット**

```bash
make fmt
make check
git status --porcelain   # probe が残っていないこと
```

```bash
git add -A
git commit -m "$(cat <<'EOF'
test: make the domain's neutrality something CI can fail on

The boundary was made of discipline before, and it came apart in seven
places without anyone noticing. Two depguard rules and a reflect walk now
say what a review had to catch by eye.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 13: 規約と設計書を新しい形に合わせる

**Files:**
- Modify: `.claude/rules/architecture.md`
- Modify: `docs/superpowers/specs/2026-09-13-architecture-restructure-design.md`

- [ ] **Step 1: 依存図の注記を直す**

「`adapter/gateway` はまだ存在しない（PR 2b で作る）」という趣旨の 1 文がある。
gateway は実在するようになったので消す。
`internal/github → internal/app/domain` の辺も消える。

- [ ] **Step 2: 「GitHub API 固有の値はパッケージの外に出さない」を機械検査に強化する**

この節は今「TUI 側で `switch` しない」と書いている。Task 12 で入れた 3 つの検査
（`encoding/json` の禁止、`internal/app` の禁止、struct タグの禁止）を明記し、
**「規約文ではなく CI が守る」**ことを書く。

- [ ] **Step 3: 「層を足す前に」に gateway の記録を足す**

`datasource` について書いたのと同じ 3 つを、`gateway` について実測で書く。

- **無いと何が壊れるか:** `internal/github` から domain への参照が 430 箇所 / 81 シンボル
  あり、`Author` `Label` `Comment` は json タグ付きのまま両者で共有されていた。
  `gql/search.go` が Work ボードの 4 カラムという**アプリの概念**を検索文字列に
  変換していた
- **足すと何が減るか:** domain がワイヤ形式を一切知らなくなり、それを CI が守る。
  別サービスを足すときに書くのは `internal/<service>` と `gateway/<service>` だけで、
  domain も usecase も tui も動かない
- **足すと何が増えるか:** GitHub への新しい操作を足すときに触るファイルが
  `internal/github` + `usecase` + ビューの 3 つから、`internal/github` + `gateway` +
  `usecase` + ビューの 4 つになる。ワイヤ型が public API になった（245 フィールド）

- [ ] **Step 4: 設計書の §10 に PR 2b 完了の印を付ける**

- [ ] **Step 5: 検査してコミット**

```bash
make check
```

```
docs: record what the gateway cost and what it now guarantees
```

---

## PR 2b の完了条件

1. `grep -rn 'app/domain' --include='*.go' internal/github` が空
2. `grep -rn 'json:"\|yaml:"' internal/app/domain/*.go` が空
3. `internal/app/domain` が stdlib しか import していない（`encoding/json` を含まない）
4. Task 12 の 3 つの検査が CI で動き、**それぞれ実際に落ちることを確認済み**
5. `make check` と `make release-check` が通る
6. golden ファイルと testdata の中身が変わっていない

```bash
git diff main...HEAD --stat -- '*.golden' '*testdata*'
```

期待: 出力なし。

7. `go run ./cmd/octoscope` と `--lang ja` を実際に起動し、次が再構成前と同じに動く:
   Work ボードの 4 カラム、Repos タブ、Search タブ、詳細、diff、checks、merge、
   **そして認証を外した状態（`PATH` から gh を外し `GH_TOKEN` を空にする）でエラー画面が出ること**
8. `.claude/rules/architecture.md` と設計書が新しい構成を指している
