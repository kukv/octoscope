# 共通 GraphQL 層 実装計画（Phase 4 スライス 4-1）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `.graphql` 文書とその応答のデコードを `internal/gh/cli` から `internal/gh/gql` に出し、
**transport（実際に GitHub と話す部分）だけを差し替えられる形**にする。このスライス単体では
振る舞いを 1 つも変えない。`gh` はこれまでどおり同じ引数で起動し、既存のテストは全部緑のまま。

**Architecture:** `internal/gh/gql` に「GraphQL 文書・変数・応答のデコード・ドメイン型への変換」を置き、
`gql.Client` が `Transport` 関数 1 つを通して外と話す。`internal/gh/cli` はその `Transport` を
`gh api graphql` の引数に組み立てて実行するだけになり、`gql.Client` を embed して
GraphQL 系メソッドを利用側（`internal/usecase` の `source`）に出す。
スライス 4-2 で `internal/gh/api` が同じ `gql.Client` を別の `Transport`（HTTPS への POST）で組む。

**Tech Stack:** Go / `gh` CLI（`gh api graphql`）/ 標準ライブラリのみ（このスライスでは依存を増やさない）

**Spec:**
- `docs/superpowers/specs/2026-09-08-phase4-design.md`（§6 API フォールバック、§7 境界、§8 テスト）
- `docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md`（§3.2 バックエンド抽象、§3.3 データ取得）
- 前スライスの積み残し: `docs/superpowers/2026-09-12-phase4-search-tab-followups.md`

---

## Global Constraints

- **振る舞いを変えない。** このスライスは移設であって機能追加ではない。`gh` に渡る引数は
  1 バイトも変えない（既存の `cli_test.go` / `graphql_test.go` / `review_test.go` /
  `merge_test.go` / `checks_test.go` / `repo_counts_test.go` の引数アサーションがその保証）
- ネットワークも外部プロセスも実際には叩かない。応答は既存の `testdata/*.json` を使う
- 先に失敗するテストを書く。書いた直後に検証対象を一時的に壊し、**落ちることを目で見てから**コミットする
- コメントは英語。書くのは「外部の事情」「一見おかしいコードが正しい理由」「エクスポートした識別子の doc」の 3 つだけ。
  **実装計画や設計書への参照（`Task 4`、`spec §6`）をコードに書かない**（`.claude/rules/*.md` への参照は可）
- エラーは `.claude/rules/errors.md` に従う。文脈は小文字で始め、`%w` で包む。
  **エラー文字列で分岐しない**（`gh.ErrTransient` / `gh.ErrUnauthenticated` は `errors.Is`）
- `internal/gh/gql` は `internal/tui` も `internal/usecase` も import しない。
  **パッケージを増やしたら depguard に足す**（`.golangci.yml`、`gh-layer` は `**/internal/gh/**` で
  効くので既存ルールで覆われる。Task 5 で実際に確認する）
- カバレッジ基準は 80%（`.octocov.yml`）。移設でカバレッジが落ちないよう、テストも一緒に動かす
- 各タスクの終わりに `make check` が緑であること

## このスライスに入れないもの

- **`internal/gh/api` そのもの**（4-2）。このスライスは `gql` を作るところまで
- **`go-github` の追加**（4-3）。`go.mod` は触らない
- **`pr list` / `issue list` / `repo view` の GraphQL 化**（4-2）。下の前提 3 を見る
- **`run view --log` の代替**（4-4）
- **スライス 3-3（保存クエリ）**。Search の未完部分はこのスライスと独立で、順序の依存もない

---

## この計画が判断した前提（着手前に承認を取ること）

### 1. 新しいパッケージ名は `internal/gh/gql`

`internal/gh` はドメイン型だけを持つ約束（`gh.go` の package doc）なので、GraphQL 文書は
その下の新パッケージに置く。責務は 1 つに言える —
**「GitHub の GraphQL 文書と、その応答をドメイン型に直すこと」**。

### 2. `gql.Client` は `Transport` 関数 1 つだけを外に持つ

`internal/gh/cli` の `run`（`runFunc`）を差し替え可能なフィールドにした形の延長で、
spec §3.2 がそう書いている。interface ではなく関数型にするのは
`.claude/rules/testing.md`（「interface を増やす前に、関数型のフィールドで足りるか先に考える」）。

```go
// Transport sends one GraphQL document and returns the response body.
//
// It must return the body even when err is non-nil: GitHub answers a
// partially resolvable query with HTTP 200 plus a top-level "errors" array,
// and gh api graphql exits non-zero for that same body. RepoCounts salvages
// what did resolve.
type Transport func(ctx context.Context, doc string, vars []Var) ([]byte, error)
```

### 3. `repoArgs` の `{owner}` / `{repo}` プレースホルダは `Var` の第 3 の種類として残す

`internal/gh/cli/review.go:47` の `repoArgs` は、リポジトリが指定されていないとき
`-F owner={owner}` を渡して **`gh api` にカレントディレクトリの remote から埋めさせている**。
`api` バックエンドにこの代行者はいないので、4-2 では自分で remote を読んで owner/name に
解決する。**この差を `Var` の種類として表に出す**（`VarPlaceholder`）ことで、4-2 の実装者が
「ここは自分で解決しないといけない」と気づける形にする。黙って文字列にすると、
api が `{owner}` という名前のリポジトリを探しに行く。

### 4. `rollup` / `checkNode` は `gql` に移して export する

`internal/gh/cli/item.go` の `prJSON`（`gh pr list --json` の応答）も `rollup` を呼んでいる。
`gh pr list --json statusCheckRollup` が返すのは GraphQL とまったく同じ形なので、
`gql` に置いて `cli` から呼ぶのが正しい。`gql.CheckContext` / `gql.RollupContexts` として export する。

### 5. 読みの再試行（`ErrTransient` で 1 回だけ引き直す）は `gql.Client` に置く

`internal/gh/cli/cli.go:102` の `read` が持っている判断（「502 は答えが返らなかっただけなので
もう一度聞く。書き込みは二重適用になりうるので再試行しない」）は、transport が何であっても
同じである。`gql.Client` に `Read`（再試行あり）と `Write`（再試行なし）の 2 つを置く。
**`gh` のサブコマンド側の `read` は `cli` に残す**（`pr list` などが使う）。

### 6. 設計 §6 の表は古い。この計画では直さず、指摘だけ残す

設計 §6 は「`gh` のサブコマンドに依存している面」を
`pr list` / `issue list` / `repo view` / `label list` / コメント・close/reopen・ラベルと担当者の編集 /
`run rerun` / `run view --log` と書いているが、**2026-09-12 に実測した結果、次の 2 点で実態と合わない**。

- **`pr list` / `issue list` / `repo view` を REST で置けない。** `gh api 'repos/kukv/octoscope/pulls?per_page=1'`
  の返すキーに `additions` / `deletions` / `reviewDecision` / `statusCheckRollup` / `comments` が無い
  （実測。単体の `pulls/{n}` には `additions` / `deletions` はあるが、`reviewDecision` と
  `statusCheckRollup` はどちらにも無い）。`gh.PR` はその全部を持つので、REST で組むと
  PR 1 件ごとに追加のリクエストが要り、それでも review decision は埋まらない。
  **これらは新しい `.graphql` 文書として `gql` に足すのが正しい**（4-2 でやる）
- **表に載っていないサブコマンド依存が 5 つある。** `pr diff`（+ files API のフォールバック）、
  `search repos`、`repo list`、`api user/orgs`、`api repos/.../assignees`。
  設計を書いた 2026-09-08 以降に足されたものを含む

**パリティの正本は設計 §6 の表ではなく `internal/usecase/usecase.go` の `source` interface である。**
4-2 以降はそちらを数える。設計 §6 の書き換えは利用者の承認事項なので、この計画では行わない。

### 7. スライス 4 は 5 本の PR に割る

このスライスは 1 本目。残りは着手前にそれぞれ計画を書く。

| PR | 内容 |
|---|---|
| **4-1（この計画）** | `internal/gh/gql` の新設。文書とデコードの移設、transport の切り出し。振る舞いは不変 |
| 4-2 | `internal/gh/api`: トークン検出・HTTP transport・カレントリポジトリの解決・GraphQL 系の全メソッド。`pr list` / `issue list` / `repo view` の新 `.graphql` 文書。`main.go` のバックエンド選択と認証エラー画面 |
| 4-3 | REST 系（`go-github`）: label list / assignees / コメント / close・reopen / ラベルと担当者の編集 / `pulls/{n}/files` / `search/repositories` / `user/repos`・`orgs/{o}/repos` / `user/orgs` |
| 4-4 | Actions: `RerunWorkflow` と `JobLog`（`run view --log` の代替） |
| 4-5 | 引き継ぎ文書と、`gh` を PATH から外した手動確認の手順（完了条件 10） |

---

## ファイル構成

| ファイル | 責務 |
|---|---|
| `internal/gh/gql/gql.go`（新規） | `Client` / `Transport` / `Var` と、読み（再試行あり）・書き（再試行なし） |
| `internal/gh/gql/search.go`（新規） | `work.graphql`、`ListWorkSection` / `SearchItems`、`searchNode` の変換 |
| `internal/gh/gql/checks.go`（新規） | `checks.graphql`、`PRChecks`、`CheckContext` / `RollupContexts` |
| `internal/gh/gql/review.go`（新規） | review 系 7 文書、`PRReviewContext` と 5 つの mutation |
| `internal/gh/gql/merge.go`（新規） | merge 系 4 文書、`PRMergeContext` と 3 つの mutation |
| `internal/gh/gql/repo_counts.go`（新規） | `repo_counts.graphql`、`buildRepoCountsQuery`、`RepoCounts` |
| `internal/gh/gql/schema_test.go`（移設） | 全文書を録画したスキーマに突き合わせる |
| `internal/gh/gql/testdata/`（移設） | `schema.json` と GraphQL 応答の fixture、`README.md` の該当部分 |
| `internal/gh/cli/cli.go`（変更） | `gql.Client` の embed と、`gh api graphql` を組む transport |
| `internal/gh/cli/graphql.go`（削除） | 中身は `gql/search.go` へ |
| `internal/gh/cli/checks.go`（変更） | GraphQL 部を落とし、`JobLog` / `RerunWorkflow` だけ残す |
| `internal/gh/cli/review.go`（削除） | 中身は `gql/review.go` へ |
| `internal/gh/cli/merge.go`（削除） | 中身は `gql/merge.go` へ |
| `internal/gh/cli/repo_counts.go`（削除） | 中身は `gql/repo_counts.go` へ |
| `internal/gh/cli/item.go`（変更） | `rollup` の呼び先を `gql.RollupContexts` に |

---

### Task 1: `gql` の土台と、`gh` 側 transport の契約

文書を 1 つも動かす前に、**transport の形だけを先に決めて固定する**。ここが正しければ、
以降のタスクは中身を運ぶだけになる。

**Files:**
- Create: `internal/gh/gql/gql.go`
- Create: `internal/gh/gql/gql_test.go`
- Modify: `internal/gh/cli/cli.go`
- Modify: `internal/gh/cli/cli_test.go`

**Interfaces:**
- Produces:
  - `type Transport func(ctx context.Context, doc string, vars []Var) ([]byte, error)`
  - `type VarKind int` と `VarString` / `VarInt` / `VarPlaceholder`
  - `type Var struct { Name string; Kind VarKind; Str string; Int int }`
  - `func S(name, value string) Var` / `func N(name string, value int) Var` /
    `func Placeholder(name, value string) Var`
  - `type Client struct { Do Transport; RepoVars func(repo string) ([]Var, error) }`
  - `func (c *Client) Read(ctx context.Context, doc string, vars ...Var) ([]byte, error)` — `gh.ErrTransient` なら 1 回だけ引き直す
  - `func (c *Client) Write(ctx context.Context, doc string, vars ...Var) ([]byte, error)` — 再試行しない
- Consumes: `internal/gh` の `ErrTransient`

- [ ] **Step 1: 失敗するテストを書く**

`internal/gh/gql/gql_test.go` を新規に作る。内部テストパッケージ（`package gql`）。

```go
package gql

import (
	"context"
	"errors"
	"testing"

	"github.com/kukv/octoscope/internal/gh"
)

// A 502 means no answer came back, not that nothing arrived: the request is
// well-formed, so asking again is the right response.
func TestAReadAsksAgainWhenGitHubDidNotAnswer(t *testing.T) {
	t.Parallel()

	calls := 0
	c := &Client{Do: func(context.Context, string, []Var) ([]byte, error) {
		calls++
		if calls == 1 {
			return nil, gh.Classify(gh.ErrTransient, "HTTP 502")
		}
		return []byte(`{"data":{}}`), nil
	}}
	if _, err := c.Read(context.Background(), "query {}"); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if calls != 2 {
		t.Errorf("transport called %d times, want 2", calls)
	}
}

// A write could apply twice. 502 says the answer is missing, not the effect.
func TestAWriteIsNeverSentTwice(t *testing.T) {
	t.Parallel()

	calls := 0
	c := &Client{Do: func(context.Context, string, []Var) ([]byte, error) {
		calls++
		return nil, gh.Classify(gh.ErrTransient, "HTTP 502")
	}}
	if _, err := c.Write(context.Background(), "mutation {}"); !errors.Is(err, gh.ErrTransient) {
		t.Fatalf("Write err = %v, want ErrTransient", err)
	}
	if calls != 1 {
		t.Errorf("transport called %d times, want 1", calls)
	}
}

// A cancelled context must not turn into a second request.
func TestACancelledReadStops(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	c := &Client{Do: func(context.Context, string, []Var) ([]byte, error) {
		calls++
		return nil, gh.Classify(gh.ErrTransient, "HTTP 502")
	}}
	if _, err := c.Read(ctx, "query {}"); err == nil {
		t.Fatal("Read succeeded, want an error")
	}
	if calls != 1 {
		t.Errorf("transport called %d times, want 1", calls)
	}
}

// The transport must see the body even when the call failed: a partially
// resolvable query answers with data and errors at once.
func TestTheBodyOfAFailedReadIsReturned(t *testing.T) {
	t.Parallel()

	c := &Client{Do: func(context.Context, string, []Var) ([]byte, error) {
		return []byte(`{"data":{"r0":null}}`), errors.New("gh api: exit 1")
	}}
	out, err := c.Read(context.Background(), "query {}")
	if err == nil {
		t.Fatal("Read succeeded, want an error")
	}
	if string(out) != `{"data":{"r0":null}}` {
		t.Errorf("body = %q, want the partial body", out)
	}
}
```

- [ ] **Step 2: テストが落ちることを確認する**

Run: `go test ./internal/gh/gql/`
Expected: FAIL（`internal/gh/gql` がまだ無い、もしくは `Client` が未定義）

- [ ] **Step 3: `internal/gh/gql/gql.go` を書く**

```go
// Package gql holds the GraphQL documents octoscope sends to GitHub and the
// decoding of their answers. What actually carries a document to GitHub is
// the caller's business: internal/gh/cli runs gh api graphql, internal/gh/api
// posts to the endpoint itself.
package gql

import (
	"context"
	"errors"
	"fmt"

	"github.com/kukv/octoscope/internal/gh"
)

// VarKind is how a transport has to spell one variable.
type VarKind int

const (
	// VarString is an ordinary string variable.
	VarString VarKind = iota
	// VarInt is a number: GraphQL rejects "3" where it wants 3.
	VarInt
	// VarPlaceholder is a value only gh substitutes -- "{owner}" and
	// "{repo}", which it fills from the working directory's remote. A
	// transport that is not gh has to resolve the repository itself before
	// it builds the variables, and never receives one of these.
	VarPlaceholder
)

// Var is one GraphQL variable.
type Var struct {
	Name string
	Kind VarKind
	Str  string
	Int  int
}

// S names a string variable.
func S(name, value string) Var { return Var{Name: name, Kind: VarString, Str: value} }

// N names a number variable.
func N(name string, value int) Var { return Var{Name: name, Kind: VarInt, Int: value} }

// Placeholder names a value gh fills in from the working directory.
func Placeholder(name, value string) Var {
	return Var{Name: name, Kind: VarPlaceholder, Str: value}
}

// Transport sends one document with its variables and returns the response
// body.
//
// It must return the body even when err is non-nil: GitHub answers a
// partially resolvable query with a top-level "errors" array beside the data
// it could resolve, and gh api graphql exits non-zero for that same body.
// RepoCounts is the caller that salvages it; everyone else looks at err.
type Transport func(ctx context.Context, doc string, vars []Var) ([]byte, error)

// Client sends the documents in this package through one transport.
// RepoVars turns "owner/name" (empty for "wherever we are") into the
// variables repository() takes, which the two backends answer differently.
type Client struct {
	Do       Transport
	RepoVars func(repo string) ([]Var, error)
}

// Read sends a query, asking again once when GitHub's front end did not
// answer. Only reads take this path: a 502 says no answer came back, not
// that nothing arrived, so a repeated write could apply twice.
func (c *Client) Read(ctx context.Context, doc string, vars ...Var) ([]byte, error) {
	out, err := c.Do(ctx, doc, vars)
	if err == nil || ctx.Err() != nil || !errors.Is(err, gh.ErrTransient) {
		return out, err
	}
	return c.Do(ctx, doc, vars)
}

// Write sends a mutation. It is never retried.
func (c *Client) Write(ctx context.Context, doc string, vars ...Var) ([]byte, error) {
	return c.Do(ctx, doc, vars)
}

// repoVars is what a caller uses to name the repository of a call.
func (c *Client) repoVars(repo string) ([]Var, error) {
	vars, err := c.RepoVars(repo)
	if err != nil {
		return nil, fmt.Errorf("name repository: %w", err)
	}
	return vars, nil
}
```

`gh.Classify` が既にあるかを確かめる（`internal/gh/gh.go`）。無ければテスト側を
`fmt.Errorf("HTTP 502: %w", gh.ErrTransient)` に読み替える。

- [ ] **Step 4: テストが通ることを確認する**

Run: `go test ./internal/gh/gql/`
Expected: PASS

- [ ] **Step 5: 空振りしないことを確かめる**

`Read` の再試行の行を一時的に `return out, err` に変えて
`go test ./internal/gh/gql/ -run TestAReadAsksAgain` が FAIL することを目で見る。戻す。

- [ ] **Step 6: `cli` 側に transport を足す（まだ誰も使わない）**

`internal/gh/cli/cli.go` に、`Var` を `gh api graphql` の引数に組む関数を足す。
**引数の並びは今と同じ**（`query` が先頭、その後に変数が渡された順）。

```go
// ghArgs spells one document and its variables the way gh api graphql takes
// them. -F is for values gh has to parse rather than pass through: numbers,
// and the {owner}/{repo} placeholders it fills from the working directory.
func ghArgs(doc string, vars []gql.Var) []string {
	args := []string{"api", "graphql", "-f", "query=" + doc}
	for _, v := range vars {
		switch v.Kind {
		case gql.VarInt:
			args = append(args, "-F", v.Name+"="+strconv.Itoa(v.Int))
		case gql.VarPlaceholder:
			args = append(args, "-F", v.Name+"="+v.Str)
		default:
			args = append(args, "-f", v.Name+"="+v.Str)
		}
	}
	return args
}
```

- [ ] **Step 7: transport が今までと同じ引数を組むことをテストする**

`internal/gh/cli/cli_test.go` に足す。**これが「振る舞いを変えない」の要**なので、
期待値は実装のコピーではなく「なぜその綴りなのか」が名前から分かる形にする。

```go
// GraphQL rejects "3" where it wants 3, and gh substitutes {owner}/{repo}
// only in -F values. Everything the user typed stays in -f, where gh passes
// it through untouched.
func TestNumbersAndPlaceholdersAreTheOnlyTypedArguments(t *testing.T) {
	t.Parallel()

	got := ghArgs("query {}", []gql.Var{
		gql.Placeholder("owner", "{owner}"),
		gql.N("number", 3),
		gql.S("body", "-F not a flag"),
	})
	want := []string{
		"api", "graphql", "-f", "query=query {}",
		"-F", "owner={owner}",
		"-F", "number=3",
		"-f", "body=-F not a flag",
	}
	if !slices.Equal(got, want) {
		t.Errorf("args =\n%q\nwant\n%q", got, want)
	}
}
```

- [ ] **Step 8: `make check` が緑であることを確認してコミットする**

```bash
make check
git add internal/gh/gql internal/gh/cli
git commit -m "refactor: give the GraphQL documents a transport to travel through"
```

---

### Task 2: Work と Search の移設

一番小さい文書から運ぶ。ここで「embed を動かす」「デコードを動かす」「`cli` は
1 行の委譲だけになる」という型を作り、以降のタスクはそれを繰り返す。

**Files:**
- Create: `internal/gh/gql/search.go`（`internal/gh/cli/graphql.go` の中身）
- Create: `internal/gh/gql/search_test.go`（`internal/gh/cli/graphql_test.go` のデコード部）
- Move: `internal/gh/cli/work.graphql` → `internal/gh/gql/work.graphql`
- Move: `internal/gh/cli/testdata/work_section.json`、`search_items.json` → `internal/gh/gql/testdata/`
- Delete: `internal/gh/cli/graphql.go`
- Modify: `internal/gh/cli/cli.go`（`gql.Client` を embed）、`internal/gh/cli/item.go`（`gql.RollupContexts`）
- Modify: `internal/gh/cli/graphql_test.go`（引数のアサーションだけ残す）

**Interfaces:**
- Produces:
  - `func (c *Client) ListWorkSection(ctx context.Context, s gh.WorkSection) ([]gh.WorkItem, error)`（`gql.Client` のメソッド）
  - `func (c *Client) SearchItems(ctx context.Context, query string) ([]gh.WorkItem, error)`
  - `type CheckContext struct { ... }` — `checkNode` を export した形
  - `func RollupContexts(nodes []CheckContext) gh.Checks`
- Consumes: Task 1 の `Client` / `Read` / `Var`

- [ ] **Step 1: 移設先のデコードテストを書いて落とす**

`internal/gh/cli/graphql_test.go` のうち、**録った JSON を食わせて `gh.WorkItem` を
確かめている部分**を `internal/gh/gql/search_test.go` に写す。`c.run` の差し替えは
`Client{Do: ...}` の差し替えに読み替える。

```go
package gql

import (
	"context"
	"os"
	"testing"

	"github.com/kukv/octoscope/internal/gh"
)

func TestASearchResultBecomesWorkItems(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("testdata/work_section.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	c := &Client{Do: func(context.Context, string, []Var) ([]byte, error) { return raw, nil }}
	items, err := c.ListWorkSection(context.Background(), gh.SectionReviewRequested)
	if err != nil {
		t.Fatalf("ListWorkSection: %v", err)
	}
	// ... 既存のアサーションをそのまま持ってくる
}
```

Run: `go test ./internal/gh/gql/`
Expected: FAIL（`ListWorkSection` が未定義、fixture が無い）

- [ ] **Step 2: ファイルを動かす**

```bash
git mv internal/gh/cli/work.graphql internal/gh/gql/work.graphql
git mv internal/gh/cli/graphql.go internal/gh/gql/search.go
git mv internal/gh/cli/testdata/work_section.json internal/gh/gql/testdata/
git mv internal/gh/cli/testdata/search_items.json internal/gh/gql/testdata/
```

`internal/gh/gql/search.go` を直す。

- `package cli` → `package gql`
- `func (c *Client) searchItems` の中の
  `c.read(ctx, c.dir, "api", "graphql", "-f", "query="+workQuery, "-f", "search="+search)` を
  `c.Read(ctx, workQuery, S("search", search))` に
- `checkNode` → `CheckContext`、`rollup` → `RollupContexts`、`pageInfo` → `PageInfo`（Task 3 でも使う）
  に改名して export する。doc コメントを付ける:

```go
// CheckContext is one entry of a commit's status check rollup. gh pr list
// --json statusCheckRollup returns the same shape in a flat array, which is
// why the roll-up below is a free function rather than a method on the
// commit around it.
type CheckContext struct { ... }

// RollupContexts counts every check-run context once: each context
// increments Total and exactly one of Passed, Failed, or Running, so
// Passed+Failed+Running always equals Total.
func RollupContexts(nodes []CheckContext) gh.Checks { ... }
```

- [ ] **Step 3: `cli` から委譲する**

`internal/gh/cli/cli.go` の `Client` に `gql.Client` を embed し、`New` で組む。

```go
type Client struct {
	*gql.Client
	dir  string
	repo string
	run  runFunc
}

func New(dir, repo string) *Client {
	c := &Client{dir: dir, repo: repo, run: runGh}
	c.Client = &gql.Client{
		// The closure reads c.run at call time: tests replace it after New.
		Do: func(ctx context.Context, doc string, vars []gql.Var) ([]byte, error) {
			return c.run(ctx, c.dir, ghArgs(doc, vars)...)
		},
		RepoVars: func(repo string) ([]gql.Var, error) { return repoVars(c.effectiveRepo(repo)) },
	}
	return c
}
```

`repoVars` は `internal/gh/cli/review.go` の `repoArgs` を `[]gql.Var` を返す形に直したもの。
Task 4 で `review.go` が消えるので、この関数は `cli.go` に置く。

```go
// repoVars names the repository for a GraphQL call. GraphQL's repository()
// takes owner and name separately, unlike `gh pr` which takes the whole
// "owner/name" after --repo. When no repository was named -- the ordinary
// case of running octoscope inside a checkout -- there is nothing to split,
// and gh fills the placeholders from the working directory's remote.
func repoVars(repo string) ([]gql.Var, error) {
	if repo == "" {
		return []gql.Var{gql.Placeholder("owner", "{owner}"), gql.Placeholder("name", "{repo}")}, nil
	}
	owner, name, ok := strings.Cut(repo, "/")
	if !ok {
		return nil, fmt.Errorf("repo %q has no owner/name separator", repo)
	}
	return []gql.Var{gql.S("owner", owner), gql.S("name", name)}, nil
}
```

`internal/gh/cli/item.go` の `rollup(...)` を `gql.RollupContexts(...)` に、
`[]checkNode` を `[]gql.CheckContext` に直す。

- [ ] **Step 4: `cli` 側には引数のアサーションだけ残す**

`internal/gh/cli/graphql_test.go` から、デコードを見ている関数を消す（`gql` に写した）。
**`gh` に渡る引数を見ている関数は残す。** これが「振る舞いを変えていない」の証拠になる。
残す関数が 1 つも無ければ、次を足す。

```go
// The document travels as gh's own "query" parameter, so the search string
// has to go under a different name or it would overwrite the document.
func TestTheSearchStringDoesNotTravelAsQuery(t *testing.T) {
	t.Parallel()

	c := New("", "")
	var got []string
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return []byte(`{"data":{"results":{"nodes":[]}}}`), nil
	}
	if _, err := c.SearchItems(context.Background(), "is:open"); err != nil {
		t.Fatalf("SearchItems: %v", err)
	}
	if !slices.Contains(got, "search=is:open") {
		t.Errorf("args %q carry no search variable", got)
	}
	for _, a := range got {
		if a == "query=is:open" {
			t.Error("the search string was sent as the document")
		}
	}
}
```

- [ ] **Step 5: 全テストが通ることを確認する**

Run: `go test ./internal/gh/...`
Expected: PASS

- [ ] **Step 6: 空振りしないことを確かめる**

`gql/search.go` の `S("search", search)` を `S("search", "")` に変えて
`go test ./internal/gh/...` が FAIL することを目で見る。戻す。

- [ ] **Step 7: `make check` を通してコミットする**

```bash
make check
git add -A internal/gh
git commit -m "refactor: move the work and search document into the shared layer"
```

---

### Task 3: checks の GraphQL 部の移設

`checks.go` は 1 ファイルに GraphQL（`PRChecks`）とサブコマンド（`JobLog` / `RerunWorkflow`）が
同居している唯一のファイルなので、**分けて運ぶ**。

**Files:**
- Create: `internal/gh/gql/checks.go`
- Create: `internal/gh/gql/checks_test.go`
- Move: `internal/gh/cli/checks.graphql` → `internal/gh/gql/checks.graphql`
- Move: `internal/gh/cli/testdata/pr_checks.json` → `internal/gh/gql/testdata/`
- Modify: `internal/gh/cli/checks.go`（`JobLog` / `RerunWorkflow` と `parseJobLog` だけにする）
- Modify: `internal/gh/cli/checks_test.go`（ログ側のテストだけ残す）

**Interfaces:**
- Produces: `func (c *Client) PRChecks(ctx context.Context, repo string, number int) (gh.Checks, error)`
- Consumes: Task 2 の `CheckContext` / `RollupContexts` / `PageInfo`

- [ ] **Step 1: 移設先のテストを書いて落とす**

`internal/gh/cli/checks_test.go` のうち `pr_checks.json` を食わせている関数を
`internal/gh/gql/checks_test.go` に写す。**ページングのテストを必ず含める**
（`hasNextPage` が true の応答を 1 回目に返し、2 回目で false にして、両ページの
コンテキストが 1 つの `gh.Checks` に合流することを見る）。

Run: `go test ./internal/gh/gql/`
Expected: FAIL（`PRChecks` が未定義）

- [ ] **Step 2: ファイルを動かす**

```bash
git mv internal/gh/cli/checks.graphql internal/gh/gql/checks.graphql
git mv internal/gh/cli/testdata/pr_checks.json internal/gh/gql/testdata/
```

`internal/gh/cli/checks.go` の先頭から `detailRollup` までを `internal/gh/gql/checks.go` に切り出す
（`checksResponse` / `contexts` / `commitNode` / `statusCheckRollup` / `contextPage` /
`checkDetailNode` / `checkSuiteNode` / `workflowRunNode` / `PRChecks` / `detailRollup` / `toRun`）。
`cli/checks.go` に残るのは `JobLog` / `parseJobLog` / `RerunWorkflow` と、それが使う import だけ。

`PRChecks` のループの中は次に変わる。

```go
	repoFields, err := c.repoVars(repo)
	if err != nil {
		return gh.Checks{}, err
	}
	var nodes []checkDetailNode
	cursor := ""
	for {
		vars := append(slices.Clone(repoFields), N("number", number))
		if cursor != "" {
			vars = append(vars, S("after", cursor))
		}
		out, err := c.Read(ctx, checksQuery, vars...)
		...
	}
```

`c.effectiveRepo(repo)` の呼び出しは消える — `RepoVars` が `cli` 側でそれをやっている。

- [ ] **Step 3: 全テストが通ることを確認する**

Run: `go test ./internal/gh/...`
Expected: PASS

- [ ] **Step 4: 空振りしないことを確かめる**

`PRChecks` のページングの `cursor = contexts.PageInfo.EndCursor` を消して
`go test ./internal/gh/gql/` が FAIL することを目で見る。戻す。

- [ ] **Step 5: `make check` を通してコミットする**

```bash
make check
git add -A internal/gh
git commit -m "refactor: move the checks document into the shared layer"
```

---

### Task 4: review と merge の移設

一番大きい 2 つ。文書が 11 個（review 7 + merge 4）あり、mutation を含む。
**mutation は `Write` を通す**（再試行しない）。

**Files:**
- Move: `internal/gh/cli/review.go` → `internal/gh/gql/review.go`
- Move: `internal/gh/cli/merge.go` → `internal/gh/gql/merge.go`
- Move: `internal/gh/cli/review_test.go` → `internal/gh/gql/review_test.go`
- Move: `internal/gh/cli/merge_test.go` → `internal/gh/gql/merge_test.go`
- Move: `*.graphql` 11 個と `testdata/review_context.json`、`testdata/merge_context.json`
- Modify: `internal/gh/cli/cli_test.go`（`repoVars` のテストをここに引き取る）

**Interfaces:**
- Produces（すべて `gql.Client` のメソッド）:
  - `PRReviewContext(ctx, repo string, number int) (gh.ReviewContext, error)`
  - `StartReview(pullRequestID string) (string, error)`
  - `AddReviewThread(reviewID string, c gh.PendingComment) error`
  - `SubmitReview(reviewID string, event gh.ReviewEvent, body string) error`
  - `SubmitNewReview(pullRequestID string, event gh.ReviewEvent, body string) error`
  - `DiscardReview(reviewID string) error`
  - `PRMergeContext(ctx, repo string, number int) (gh.MergeContext, error)`
  - `MergePR(pullRequestID string, method gh.MergeMethod) error`
  - `EnableAutoMerge(pullRequestID string, method gh.MergeMethod) error`
  - `DisableAutoMerge(pullRequestID string) error`

- [ ] **Step 1: mutation が再試行されないことを見るテストを書いて落とす**

移設したテストに加えて、`internal/gh/gql/review_test.go` に足す。
**これは今どこにも無い検証で、移設によって初めて 1 か所で書ける。**

```go
// A submitted review that was sent twice would post twice. 502 says the
// answer is missing, not that the mutation did not run.
func TestASubmittedReviewIsNeverSentTwice(t *testing.T) {
	t.Parallel()

	calls := 0
	c := &Client{Do: func(context.Context, string, []Var) ([]byte, error) {
		calls++
		return nil, gh.Classify(gh.ErrTransient, "HTTP 502")
	}}
	if err := c.SubmitReview("R_1", gh.EventApprove, ""); err == nil {
		t.Fatal("SubmitReview succeeded, want an error")
	}
	if calls != 1 {
		t.Errorf("transport called %d times, want 1", calls)
	}
}
```

Run: `go test ./internal/gh/gql/`
Expected: FAIL（`SubmitReview` が未定義）

- [ ] **Step 2: ファイルを動かす**

```bash
git mv internal/gh/cli/review.go internal/gh/gql/review.go
git mv internal/gh/cli/merge.go internal/gh/gql/merge.go
git mv internal/gh/cli/review_test.go internal/gh/gql/review_test.go
git mv internal/gh/cli/merge_test.go internal/gh/gql/merge_test.go
git mv internal/gh/cli/review.graphql internal/gh/gql/
git mv internal/gh/cli/start_review.graphql internal/gh/gql/
git mv internal/gh/cli/add_thread.graphql internal/gh/gql/
git mv internal/gh/cli/submit_review.graphql internal/gh/gql/
git mv internal/gh/cli/review_at_once.graphql internal/gh/gql/
git mv internal/gh/cli/discard_review.graphql internal/gh/gql/
git mv internal/gh/cli/thread_comments.graphql internal/gh/gql/
git mv internal/gh/cli/merge.graphql internal/gh/gql/
git mv internal/gh/cli/merge_pr.graphql internal/gh/gql/
git mv internal/gh/cli/enable_auto_merge.graphql internal/gh/gql/
git mv internal/gh/cli/disable_auto_merge.graphql internal/gh/gql/
git mv internal/gh/cli/testdata/review_context.json internal/gh/gql/testdata/
git mv internal/gh/cli/testdata/merge_context.json internal/gh/gql/testdata/
```

置換は機械的に 3 つ。

- `package cli` → `package gql`
- 読み: `c.read(ctx, c.dir, "api", "graphql", "-f", "query="+X, ...)` → `c.Read(ctx, X, ...)`
- 書き: `c.run(context.Background(), c.dir, "api", "graphql", "-f", "query="+X, ...)` →
  `c.Write(context.Background(), X, ...)`

変数は `-f name=v` → `S("name", v)`、`-F name=<数>` → `N("name", <数>)`。
`repoArgs` は `cli` の `repoVars`（Task 2）に置き換わっているので、この 2 ファイルからは消す。
`repoArgs` のテストが `review_test.go` にあれば `cli_test.go` に引き取って `repoVars` を見る形に直す。

- [ ] **Step 3: 全テストが通ることを確認する**

Run: `go test ./internal/gh/...`
Expected: PASS

- [ ] **Step 4: 空振りしないことを確かめる**

`AddReviewThread` の `N("line", comment.Line)` を `N("line", 0)` に変えて
`go test ./internal/gh/gql/` が FAIL することを目で見る。戻す。

- [ ] **Step 5: `make check` を通してコミットする**

```bash
make check
git add -A internal/gh
git commit -m "refactor: move the review and merge documents into the shared layer"
```

---

### Task 5: repo_counts と schema_test の移設、depguard の確認

最後に、**全文書を検証している `schema_test.go` を文書と同じ場所に置く**。
設計の完了条件 8（「`.graphql` 文書が 1 組のまま両バックエンドから使われ、`schema_test.go` が
その 1 組を検証している」）がここで満たされる。

**Files:**
- Move: `internal/gh/cli/repo_counts.go` → `internal/gh/gql/repo_counts.go`
- Move: `internal/gh/cli/repo_counts_test.go` → `internal/gh/gql/repo_counts_test.go`
- Move: `internal/gh/cli/repo_counts.graphql` → `internal/gh/gql/repo_counts.graphql`
- Move: `internal/gh/cli/schema_test.go` → `internal/gh/gql/schema_test.go`
- Move: `internal/gh/cli/testdata/schema.json`、`repo_counts.json`、`repo_counts_partial.json` → `internal/gh/gql/testdata/`
- Modify: `internal/gh/cli/testdata/README.md` と `internal/gh/gql/testdata/README.md`（録り方の記述を移す）
- Modify: `.golangci.yml`（必要なら）

**Interfaces:**
- Produces: `func (c *Client) RepoCounts(ctx context.Context, repos []string) ([]gh.RepoCount, error)`

- [ ] **Step 1: 部分的な応答を拾えることを見るテストを移して落とす**

`repo_counts_test.go` の「消えたリポジトリが 1 つあっても他の件数が出る」テスト
（`repo_counts_partial.json` を使い、transport がエラーと本文の**両方**を返す形）を
`internal/gh/gql/repo_counts_test.go` に写す。**これが Task 1 の「本文も返す」契約の
唯一の実利用者**なので、必ず動かす。

Run: `go test ./internal/gh/gql/`
Expected: FAIL（`RepoCounts` が未定義）

- [ ] **Step 2: ファイルを動かす**

```bash
git mv internal/gh/cli/repo_counts.go internal/gh/gql/repo_counts.go
git mv internal/gh/cli/repo_counts_test.go internal/gh/gql/repo_counts_test.go
git mv internal/gh/cli/repo_counts.graphql internal/gh/gql/repo_counts.graphql
git mv internal/gh/cli/schema_test.go internal/gh/gql/schema_test.go
git mv internal/gh/cli/testdata/schema.json internal/gh/gql/testdata/
git mv internal/gh/cli/testdata/repo_counts.json internal/gh/gql/testdata/
git mv internal/gh/cli/testdata/repo_counts_partial.json internal/gh/gql/testdata/
```

`RepoCounts` の引数組み立ては `[]gql.Var` になる。

```go
	var vars []Var
	for i, repo := range repos {
		...
		n := len(indices)
		vars = append(vars, S(fmt.Sprintf("o%d", n), owner), S(fmt.Sprintf("n%d", n), name))
		indices = append(indices, i)
	}
	...
	out, runErr := c.Read(ctx, buildRepoCountsQuery(len(indices)), vars...)
```

`schema_test.go` の `docs` マップはそのまま動く（変数名が同じパッケージに揃うため）。

- [ ] **Step 3: `cli` に GraphQL 文書が 1 つも残っていないことを確認する**

```bash
ls internal/gh/cli/*.graphql          # 何も無いこと
grep -rn 'api", "graphql"' internal/gh/cli/   # 何も無いこと
```

残っていたら、それは移し忘れである。

- [ ] **Step 4: depguard を確認する**

```bash
grep -n 'internal/gh/\*\*' .golangci.yml
```

`gh-layer` の `files` が `**/internal/gh/**` なら `internal/gh/gql` は既存ルールで覆われている。
覆われていなければ `gh-layer` に足す。あわせて `tui-layer` の deny に
`github.com/kukv/octoscope/internal/gh/gql` を足す — **`internal/tui` が
バックエンドを名指しできないのと同じ理由で、GraphQL 層も名指しできてはいけない**。

```yaml
            - pkg: github.com/kukv/octoscope/internal/gh/gql
              desc: views go through internal/usecase; only cmd/octoscope names a backend
```

- [ ] **Step 5: `testdata/README.md` を両方直す**

移した fixture の録り方（録った日・対象リポジトリ・`jq` の絞り込み）を
`internal/gh/gql/testdata/README.md` に移す。`cli` 側には残った fixture
（`pr_list.json` / `pr_view.json` / `issue_list.json` / `issue_view.json` / `pr_files.json` /
`own_repos.json` / `search_repos.json` / `sample.diff` / `job_log*.txt`）の分だけ残す。

- [ ] **Step 6: `make check` を通してコミットする**

```bash
make check
git add -A internal/gh .golangci.yml
git commit -m "refactor: keep the schema check next to the documents it validates"
```

---

### Task 6: 引き継ぎ文書

**Files:**
- Create: `docs/superpowers/2026-09-12-phase4-shared-graphql-followups.md`

- [ ] **Step 1: 積み残しを書く**

過去のスライスと同じ形で書く。**直さなかったものは理由つきで。** 最低限:

- 設計 §6 の表が実態と合わない 2 点（この計画の前提 6）。書き換えは利用者の承認事項
- `gh.PR` / `gh.Issue` を埋める `.graphql` 文書はまだ無い（4-2）
- `cli` に残ったサブコマンド依存の一覧（4-3 / 4-4 が引き取るもの）
- `gql.Client` の `RepoVars` が `nil` のときの挙動（今は panic）。
  4-2 で `api` を組むときに必ず入れる側なので、守りを足していない

- [ ] **Step 2: コミットする**

```bash
git add docs/superpowers/2026-09-12-phase4-shared-graphql-followups.md
git commit -m "docs: record what the shared GraphQL layer left behind"
```

---

## 完了条件（このスライス）

1. `internal/gh/cli` に `.graphql` ファイルが 1 つも無く、`"api", "graphql"` の文字列も無い
2. `internal/gh/gql/schema_test.go` が全 14 文書を録画したスキーマに突き合わせている
   — 完結した文書 13 ファイルと、`repo_counts.graphql`（断片）から組んだ
   `buildRepoCountsQuery(2)`。`.graphql` ファイルは全部で 14 個
3. `gh` に渡る引数が移設前と同じ（`cli` 側に残した引数アサーションが緑）
4. `make check` が緑で、カバレッジが 80% を下回っていない
5. `internal/usecase` と `internal/tui` に 1 行の変更も無い（`git diff --stat` で確認できる）
