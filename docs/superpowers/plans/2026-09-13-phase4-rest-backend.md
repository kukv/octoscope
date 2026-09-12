# REST 系 実装計画（Phase 4 スライス 4-3）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `gh` のサブコマンドでしか取れていなかった 16 メソッド（コメント・close/reopen・
ラベルと担当者の編集・`label list` / assignees・`pr diff` / `pulls/{n}/files`・
`search repos` / `repo list` / `user/orgs`）を `internal/gh/api` に REST で実装し、
`api.Client` が Actions 2 本を除く `source` を満たす状態にする。

**Architecture:** 4-2 で作った `internal/gh/api` の transport を GraphQL 専用から
ベース URL 方式に広げ、`send` / `read` / `write` の 3 つの口を足す。認証ヘッダ・
User-Agent・`classify` によるエラー分類は GraphQL と 1 本を共有する。`pr diff` は
`gh pr diff` の正体（`GET pulls/{n}` + diff メディアタイプ）と同じ二段構えにし、
そのために `parseDiff` と files API のデコードを `internal/gh` に出して `cli` と
`api` の両方から使う。

**Tech Stack:** Go 1.27.1、標準ライブラリのみ（`net/http` / `net/url` /
`encoding/json`）。**`go-github` は足さない**（下の「着手前に確かめたこと §1」。
設計 §6 の表からの逸脱で、2026-09-13 に利用者の承認を得た。§6 の訂正は Task 6）。
`githubv4` は Phase 4 全体で足さない。

**Spec:** `docs/superpowers/specs/2026-09-08-phase4-design.md`（特に §6・§7・§8）。
前スライスの積み残しは `docs/superpowers/2026-09-12-phase4-api-backend-followups.md`。

---

## Global Constraints

設計と `.claude/rules/` から、このスライスの全タスクに掛かるもの。

- **ネットワークも外部プロセスも実際には叩かない**（`.claude/rules/testing.md`）。
  HTTP は `httptest.Server` に向ける。既存の継ぎ目は `Client.endpoint`（Task 1 で
  `baseURL` に改名する）
- **パーステストの入力は実際に録ったレスポンス**。`internal/gh/api/testdata/` に置き、
  録り方・録った日・対象リポジトリを同ディレクトリの `README.md` に書く。
  自分の公開リポジトリか `cli/cli` を使い、私有情報を入れない
- **新しく書いたテストは、検証対象を一時的に壊して落ちることを確認してからコミットする**
  （設計 §8）
- **実装が組み立てた引数をコピーした期待値を書かない**（`.claude/rules/testing.md`）。
  テスト名は「なぜその引数が要るのか」が読める形にする
- **`internal/gh` は `internal/tui` も `internal/usecase` も import しない**
  （`.golangci.yml` の `gh-layer`）。`api` は `cli` も import しない
  （2 バックエンドが互いに依存しないこと。`parity_test.go` が `package api_test`
  なのはこのため）
- **GitHub API 固有の文字列はこの層で止める。** `"closed"` / `"completed"` を外に出さない
- **エラーは 3 種類に分ける**（`.claude/rules/errors.md`）。センチネルは
  `errors.Is` で分岐する箇所を実際に書くものだけ。この層に `internal/i18n` を入れない
- **コメントは英語。** 外部の事情・一見おかしいコードが正しい理由・doc コメントの
  3 つだけ。**実装計画や設計書への参照をコードに書かない**（`.claude/rules/go-style.md`）
- **`context.Context` は取得系の第一引数。** 書き込み系は `source` interface に
  `ctx` が無いので `cli` と同じく `context.Background()` を使う
- 各タスクの最後に `make check` が緑であること。カバレッジの基準は 80%

---

## このスライスでやらないこと（理由つき）

- **Actions（`RerunWorkflow` / `JobLog`）は 4-4。** `gh` の
  `pkg/cmd/run/view/view.go` を読んでからでないと実装方針を決めない
- **`cmd/octoscope` のバックエンド選択と認証エラー画面も 4-4。** このスライスの
  完了時点でも `source` は Actions の 2 本が足りず、`usecase.New(api.New(...))` は
  まだコンパイルできない
- **取得のタイムアウト（`http.DefaultClient` にタイムアウトが無い件）は 4-4。**
  4-2 の積み残しに記録済みで、GraphQL と REST の両方に同時に掛けるべきものなので
  分けて決める
- **GitHub Enterprise。** `github.com` のみ。`GH_HOST` / `GH_ENTERPRISE_TOKEN` は見ない
- **`cli` 側の振る舞いの変更。** Task 4 は `cli` の `parseDiff` と files API の
  デコードを `internal/gh` へ**移すだけ**で、`gh pr diff` を呼ぶ経路も引数も変えない
- **assignees とラベルのページング。** `cli` が `per_page=100` の 1 ページしか
  読んでいないので、`api` も揃える（100 人を超えるリポジトリは `cli` でも
  選べない。差を作らないことが優先）

---

## 着手前に読んで確かめたこと（推測ではない）

実装者はここを疑ってよいが、疑うなら同じ手段で確かめ直すこと。

### 1. `go-github` を足さない（設計 §6 からの逸脱。承認済み）

設計 §6 の表は REST 側の手段として `go-github` を名指ししている。2026-09-13 に
次の 3 点を示して利用者の判断を仰ぎ、**自前実装**が選ばれた。

- `internal/gh/api/transport.go` の `classify(status, body)` は既に REST の
  `{"message": ...}` 形を読み、`gh.ErrTransient` / `gh.ErrUnauthenticated` に
  振り分けている。`go-github` を入れると `*github.ErrorResponse` から同じ
  マッピングをもう一度書くことになる
- テストの継ぎ目が 1 つ（`Client.endpoint`）から 2 つ（`+ github.Client.BaseURL`）に増える
- `go-github` が持っていて自前に無いのはページングと型付きボディだが、**ページングが
  実際に要るのは `pulls/{n}/files` だけ**（下の §5）で、Link ヘッダの解析は 15 行程度

設計 §6 の表の訂正は Task 6 で同じ PR に入れる。

### 2. `gh pr diff` は `GET /repos/{o}/{r}/pulls/{n}` に diff メディアタイプを付けている

`GH_DEBUG=api gh pr diff 82 --color never` の出力（2026-09-13 実測）:

```
* Request to https://api.github.com/graphql          （番号 → id の解決だけ）
* Request to https://api.github.com/repos/kukv/octoscope/pulls/82
> GET /repos/kukv/octoscope/pulls/82 HTTP/1.1
> Accept: application/vnd.github.v3.diff
```

**`api` は `cli` と同じ二段構え（diff メディアタイプ → 失敗したら files API）が
取れる。** GraphQL の 1 発目は `gh` が PR の id を要るからで、番号をそのまま
REST に渡せる `api` には不要。

### 3. `gh label list` の並び順は REST の `id` 昇順と一致する

`gh label list` は GraphQL に `orderBy: {field: CREATED_AT, direction: ASC}` を
渡している（`GH_DEBUG=api gh label list --repo cli/cli --limit 5` で確認）。
REST の `repos/{o}/{r}/labels` は**名前の昇順**で返すので、そのままでは並びが変わる。

**`id` の昇順に並べ替えると一致する。** `cli/cli` の 83 件で突き合わせた（2026-09-13）:

```bash
gh label list --repo cli/cli --json name --limit 100 --jq '.[].name' > /tmp/a.txt
gh api 'repos/cli/cli/labels?per_page=100' \
  --jq '[.[]|{n:.name,id:.id}]|sort_by(.id)|.[].n' > /tmp/b.txt
diff /tmp/a.txt /tmp/b.txt   # 差分なし（83 行）
```

**並び順は飾りではない。** ラベルの選択ポップアップ（`internal/tui/detail`）にも
Search タブの候補チップ（`internal/tui/search`）にも並べ替えは無く、
`ListLabels` が返した順がそのまま画面の順である。

### 4. `gh repo list` は `ownerAffiliations: OWNER` の `PUSHED_AT` 降順である

`GH_DEBUG=api gh repo list --limit 3` の GraphQL（2026-09-13 実測）:

```
repositories(first: $perPage, after: $endCursor, privacy: $privacy, isFork: $fork,
             ownerAffiliations: OWNER, orderBy: { field: PUSHED_AT, direction: DESC })
```

REST では `user/repos?affiliation=owner&sort=pushed&direction=desc` が同じ条件を指す。

### 5. `ListOwnRepos` の `owner` は空文字か Org のログインしか来ない

`internal/usecase/repos.go` の呼び出しは 2 か所だけで、片方は `""`、もう片方は
`ListOrgs` が返したログインを渡している（`repos.go:32` と `repos.go:53`）。
**`owner != ""` のとき `orgs/{owner}/repos` に投げてよい**のはこのためで、
interface の signature 自身はそれを言っていない。**この前提は `api` 側の
doc コメントに書く**（利用側が変わったら壊れる）。

### 6. ページングが実際に要るのは `pulls/{n}/files` だけ

| 呼び出し | `cli` が読む量 | `api` の指定 | ページング |
|---|---|---|---|
| `ListLabels` | `--limit 100` | `per_page=100` | 要らない（REST の上限も 100） |
| `ListAssignees` | `per_page=100` の 1 ページ | 同じ | 要らない（`cli` に揃える） |
| `SearchRepos` | `--limit N`（画面の `searchLimit`） | `per_page=N` | 要らない |
| `ListOwnRepos` | `--limit 100`（`seedLimit`） | `per_page=100` | 要らない |
| `ListOrgs` | 1 ページ | 既定 | 要らない |
| `prFiles` | `--paginate` | Link ヘッダを辿る | **要る**（既定 30 件・実例は 418 ファイル） |

`seedLimit = 100`（`internal/usecase/repos.go:13`）で REST の `per_page` の上限
ちょうどなので、**`per_page` は 100 で頭打ちにする**（101 を送ると GitHub は
422 を返す）。

### 7. `classify` の「200 に errors 配列」の分岐は GraphQL 専用である

`internal/gh/api/transport.go:classify` の最後の `case msg != ""` は、GraphQL が
HTTP 200 で部分的な失敗を返す形のためにある。REST にこの形は無いので、Task 1 で
ステータスだけを見る `statusError` を切り出し、`classify` はそれに GraphQL 用の
分岐を足したものにする。**REST から `classify` をそのまま呼ばない。**

---

## File Structure

| ファイル | 責務 |
|---|---|
| `internal/gh/api/api.go` | `Client`（`baseURL` への改名）・トークン検出・`OpenWeb` |
| `internal/gh/api/transport.go` | GraphQL の POST と `classify` / `statusError` |
| `internal/gh/api/rest.go` | **新設。** REST の `send` / `read` / `write` / `restURL` / `repoPath` / `nextLink` |
| `internal/gh/api/items.go` | **新設。** コメント・close/reopen・ラベルと担当者の編集 |
| `internal/gh/api/lists.go` | **新設。** `ListLabels` / `ListAssignees` |
| `internal/gh/api/diff.go` | **新設。** `PRDiff` と files API のフォールバック |
| `internal/gh/api/repos.go` | **新設。** `SearchRepos` / `ListOwnRepos` / `ListOrgs` |
| `internal/gh/diff_parse.go` | **新設。** `ParseDiff` と `ParseFilesAPI`（`cli` から移す） |
| `internal/gh/cli/diff.go` | `gh pr diff` の呼び出しと `gh api` のパスだけに縮む |

`internal/gh/api/repo.go`（カレントリポジトリの解決）は Task 1 が `repoPath` から
使うだけで、中身は変えない。

**行数の目安**（`.claude/rules/architecture.md` の「300 行を超えたら責務が増えて
いないか疑う」）: どのファイルも 200 行に届かない見込み。Task 5 の終わりに
`wc -l internal/gh/api/*.go` を数え、300 行を超えたファイルがあればその場で
理由を積み残しに書くこと。

---

## Task 1: REST の transport

**Files:**
- Modify: `internal/gh/api/api.go`（`endpoint` → `baseURL`、`endpointURL` の組み立て）
- Modify: `internal/gh/api/transport.go`（`statusError` の切り出し）
- Modify: `internal/gh/api/transport_test.go` / `repo_test.go`（`c.endpoint` の改名追従）
- Create: `internal/gh/api/rest.go`
- Create: `internal/gh/api/rest_test.go`

**Interfaces:**
- Produces:
  - `func (c *Client) restURL(path string) string`
  - `func (c *Client) send(ctx context.Context, method, url string, body any, accept string) ([]byte, http.Header, error)`
  - `func (c *Client) read(ctx context.Context, path, accept string) ([]byte, error)` — GET、`gh.ErrTransient` のとき 1 回だけ再送
  - `func (c *Client) write(ctx context.Context, method, path string, body any) ([]byte, error)` — 再送しない
  - `func (c *Client) repoPath(repo string) (string, error)` — `"owner/name"` を返す
  - `func nextLink(h http.Header) string`
  - `func statusError(status int, body []byte) error`
- Consumes: `internal/gh/api/repo.go` の `currentRepo()`、`gh.SplitRepo`

- [ ] **Step 1: 失敗するテストを書く（ヘッダとパス）**

`internal/gh/api/rest_test.go`:

```go
package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kukv/octoscope/internal/gh"
)

// serveREST stands in for GitHub's REST API. It records every request, since
// a call that pages sends more than one.
func serveREST(t *testing.T, handler http.HandlerFunc) (*Client, *[]*http.Request) {
	t.Helper()

	var got []*http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		clone := r.Clone(r.Context())
		clone.Body = io.NopCloser(strings.NewReader(string(body)))
		got = append(got, clone)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)

	c := New("", "kukv/octoscope", "secret-token")
	c.baseURL = srv.URL
	return c, &got
}

// GitHub refuses an unnamed client and serves a different shape to a client
// that does not pin the API version, so every REST call carries both, plus
// the token this backend exists for.
func TestARestCallNamesItselfAndTheApiVersionAndTheToken(t *testing.T) {
	t.Parallel()

	c, got := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `[]`)
	})
	if _, err := c.read(context.Background(), "repos/kukv/octoscope/labels", ""); err != nil {
		t.Fatalf("read: %v", err)
	}

	req := (*got)[0]
	if req.URL.Path != "/repos/kukv/octoscope/labels" {
		t.Errorf("path = %q, want /repos/kukv/octoscope/labels", req.URL.Path)
	}
	if h := req.Header.Get("Authorization"); h != "bearer secret-token" {
		t.Errorf("Authorization = %q", h)
	}
	if req.Header.Get("User-Agent") == "" {
		t.Error("User-Agent is empty; GitHub may refuse an unnamed client")
	}
	if v := req.Header.Get("X-GitHub-Api-Version"); v == "" {
		t.Error("X-GitHub-Api-Version is empty")
	}
	if a := req.Header.Get("Accept"); a != "application/vnd.github+json" {
		t.Errorf("Accept = %q, want the JSON media type by default", a)
	}
}

// A read is safe to repeat: a 502 says no answer came back, not that nothing
// arrived. gql.Client.Read does the same for GraphQL.
func TestAReadAsksAgainOnceWhenGitHubsFrontEndDidNotAnswer(t *testing.T) {
	t.Parallel()

	calls := 0
	c, _ := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = io.WriteString(w, `[]`)
	})
	if _, err := c.read(context.Background(), "repos/kukv/octoscope/labels", ""); err != nil {
		t.Fatalf("read: %v", err)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
}

// A write is not safe to repeat: a comment posted twice is two comments.
func TestAWriteIsNotRepeated(t *testing.T) {
	t.Parallel()

	calls := 0
	c, _ := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadGateway)
	})
	err := func() error {
		_, err := c.write(context.Background(), http.MethodPost, "repos/kukv/octoscope/issues/1/comments", map[string]string{"body": "hi"})
		return err
	}()
	if !errors.Is(err, gh.ErrTransient) {
		t.Fatalf("err = %v, want ErrTransient", err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

// The files API's default page is 30 entries and one pull request under
// review had 418, so a caller that stops at the first page loses most of the
// diff. The link to the next page is the only thing that says there is more.
func TestNextLinkFindsTheNextPageAmongTheOtherRelations(t *testing.T) {
	t.Parallel()

	h := http.Header{}
	h.Set("Link", `<https://api.github.com/x?page=2>; rel="next", <https://api.github.com/x?page=5>; rel="last"`)
	if got := nextLink(h); got != "https://api.github.com/x?page=2" {
		t.Errorf("nextLink = %q", got)
	}
	if got := nextLink(http.Header{}); got != "" {
		t.Errorf("nextLink with no header = %q, want empty", got)
	}
	last := http.Header{}
	last.Set("Link", `<https://api.github.com/x?page=1>; rel="prev"`)
	if got := nextLink(last); got != "" {
		t.Errorf("nextLink on the last page = %q, want empty", got)
	}
}

// Every REST path needs an owner and a name. An explicit repository wins, then
// the client's own, then the working directory's remote -- the same order
// repoVars uses for GraphQL, so the two halves of this backend cannot disagree
// about which repository a screen is showing.
func TestRepoPathPrefersTheExplicitRepositoryOverTheClients(t *testing.T) {
	t.Parallel()

	c := New("", "kukv/octoscope", "t")
	got, err := c.repoPath("cli/cli")
	if err != nil {
		t.Fatalf("repoPath: %v", err)
	}
	if got != "cli/cli" {
		t.Errorf("repoPath = %q, want cli/cli", got)
	}
}

// A repository with no owner/name separator would build a path GitHub reads as
// a different endpoint entirely, so it is refused before the request is sent.
func TestRepoPathRefusesAThingThatIsNotOwnerSlashName(t *testing.T) {
	t.Parallel()

	c := New("", "octoscope", "t")
	if _, err := c.repoPath(""); err == nil {
		t.Fatal("repoPath accepted a repository with no owner")
	}
}
```

- [ ] **Step 2: 落ちることを確認する**

Run: `go test ./internal/gh/api/ -run 'Rest|NextLink|RepoPath|ReadAsks|WriteIsNot' -v`
Expected: FAIL（`c.baseURL` / `c.read` / `nextLink` / `repoPath` が未定義）

- [ ] **Step 3: `api.go` の継ぎ目をベース URL に広げる**

`internal/gh/api/api.go` の `endpoint` フィールドと `endpointURL` を差し替える:

```go
	// baseURL is the seam the tests use: a local server in place of
	// github.com. Both the GraphQL endpoint and every REST path hang off it.
	baseURL string
```

```go
// base is where every request goes. Tests point it at a local server.
func (c *Client) base() string {
	if c.baseURL != "" {
		return c.baseURL
	}
	return defaultBase
}

// endpointURL is where the GraphQL documents go.
func (c *Client) endpointURL() string {
	return c.base() + "/graphql"
}
```

`internal/gh/api/transport.go` の定数を差し替える:

```go
// defaultBase is github.com's API root. GitHub Enterprise is out of scope for
// this phase, so GH_HOST is not read.
const defaultBase = "https://api.github.com"
```

`transport_test.go:40` と `repo_test.go:243` の `c.endpoint = srv.URL` を
`c.baseURL = srv.URL` に直す。

- [ ] **Step 4: `classify` から `statusError` を切り出す**

`internal/gh/api/transport.go`:

```go
// statusError names the failures a caller acts on differently: one worth
// asking again for, and one only the user can fix. Everything else keeps what
// GitHub said and no type at all.
func statusError(status int, body []byte) error {
	var b errorsBody
	// A body that is not JSON leaves b zero, which reads as "no words from
	// GitHub" -- the status alone then describes the failure.
	_ = json.Unmarshal(body, &b)
	msg := b.text()

	switch {
	case status == http.StatusBadGateway,
		status == http.StatusServiceUnavailable,
		status == http.StatusGatewayTimeout:
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", status)
		}
		return gh.Classify(gh.ErrTransient, msg)
	case status == http.StatusUnauthorized:
		if msg == "" {
			msg = "HTTP 401"
		}
		return gh.Classify(gh.ErrUnauthenticated, msg)
	case status >= 400:
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", status)
		}
		return gh.Classify(nil, msg)
	}
	return nil
}

// classify is statusError plus the one shape only GraphQL has: HTTP 200 with
// a top-level errors array, which is how a partially resolvable query answers.
func classify(status int, body []byte) error {
	if err := statusError(status, body); err != nil {
		return err
	}
	var b errorsBody
	_ = json.Unmarshal(body, &b)
	if msg := b.text(); msg != "" {
		return gh.Classify(nil, msg)
	}
	return nil
}
```

- [ ] **Step 5: `rest.go` を書く**

```go
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/kukv/octoscope/internal/gh"
)

// jsonMediaType is what GitHub asks REST clients to send. A call that wants
// something else -- a diff, say -- names its own.
const jsonMediaType = "application/vnd.github+json"

// apiVersion pins the REST shape. Without it GitHub is free to serve a newer
// one, and the decoding here is written against this version.
const apiVersion = "2022-11-28"

// restURL turns a path into an absolute URL against this client's base.
func (c *Client) restURL(path string) string {
	return c.base() + "/" + strings.TrimPrefix(path, "/")
}

// send makes one REST request. url is absolute: a paging caller gets the next
// one from the Link header rather than building it.
//
// It returns the body even when err is non-nil, the way post does: a caller
// that wants GitHub's own words has them, and a partial answer is not thrown
// away before anyone has looked at it.
func (c *Client) send(ctx context.Context, method, url string, body any, accept string) ([]byte, http.Header, error) {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, nil, fmt.Errorf("build request body: %w", err)
		}
		payload = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, payload)
	if err != nil {
		return nil, nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "bearer "+c.token)
	req.Header.Set("User-Agent", "octoscope")
	req.Header.Set("X-GitHub-Api-Version", apiVersion)
	if accept == "" {
		accept = jsonMediaType
	}
	req.Header.Set("Accept", accept)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("reach GitHub: %w", err)
	}
	// Closing a body that was only read has nothing to report.
	defer func() { _ = resp.Body.Close() }()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.Header, fmt.Errorf("read answer: %w", err)
	}
	return out, resp.Header, statusError(resp.StatusCode, out)
}

// read fetches one path, asking again once when GitHub's front end did not
// answer. Only reads take this path: a 502 says no answer came back, not that
// nothing arrived, so a repeated write could apply twice.
func (c *Client) read(ctx context.Context, path, accept string) ([]byte, error) {
	out, _, err := c.send(ctx, http.MethodGet, c.restURL(path), nil, accept)
	if err == nil || ctx.Err() != nil || !errors.Is(err, gh.ErrTransient) {
		return out, err
	}
	out, _, err = c.send(ctx, http.MethodGet, c.restURL(path), nil, accept)
	return out, err
}

// write sends one change. It is never repeated.
func (c *Client) write(ctx context.Context, method, path string, body any) ([]byte, error) {
	out, _, err := c.send(ctx, method, c.restURL(path), body, "")
	return out, err
}

// repoPath is the "owner/name" every REST path is built from. An explicit
// repository wins, then the client's own, then the working directory's
// remote: the same order repoVars uses for GraphQL.
func (c *Client) repoPath(repo string) (string, error) {
	if repo == "" {
		repo = c.repo
	}
	if repo == "" {
		var err error
		if repo, err = c.currentRepo(); err != nil {
			return "", err
		}
	}
	if _, _, ok := gh.SplitRepo(repo); !ok {
		return "", fmt.Errorf("repo %q has no owner/name separator", repo)
	}
	return repo, nil
}

// nextLink is the URL of the page after this one, or empty on the last page.
// GitHub puts every relation in one Link header, so the rel has to be read
// rather than the position.
func nextLink(h http.Header) string {
	for _, part := range strings.Split(h.Get("Link"), ",") {
		fields := strings.Split(part, ";")
		if len(fields) < 2 {
			continue
		}
		url := strings.TrimSpace(fields[0])
		if !strings.HasPrefix(url, "<") || !strings.HasSuffix(url, ">") {
			continue
		}
		for _, f := range fields[1:] {
			if strings.TrimSpace(f) == `rel="next"` {
				return url[1 : len(url)-1]
			}
		}
	}
	return ""
}
```

- [ ] **Step 6: 通ることを確認する**

Run: `go test ./internal/gh/api/ -v`
Expected: PASS（改名した既存テストも含めて全部）

- [ ] **Step 7: 落ちることを確かめる（空振り確認）**

`read` の再送を消して `TestAReadAsksAgainOnce...` が RED になること、
`nextLink` の `rel="next"` 判定を先頭要素の決め打ちに変えて
`TestNextLinkFindsTheNextPage...` が RED になることを、それぞれ手で確かめてから戻す。

- [ ] **Step 8: コミット**

```bash
git add internal/gh/api/
git commit -m "feat: give the api backend a REST transport beside its GraphQL one"
```

---

## Task 2: コメント・close / reopen・ラベルと担当者の編集

**Files:**
- Create: `internal/gh/api/items.go`
- Create: `internal/gh/api/items_test.go`

**Interfaces:**
- Consumes: Task 1 の `write` / `repoPath`
- Produces: `AddPRComment` / `AddIssueComment` / `ClosePR` / `ReopenPR` /
  `CloseIssue` / `ReopenIssue` / `EditPRLabels` / `EditIssueLabels` /
  `EditPRAssignees` / `EditIssueAssignees`（すべて `cli` と同じ signature。
  `ctx` を取らず `error` だけを返す）

- [ ] **Step 1: 失敗するテストを書く**

`internal/gh/api/items_test.go`:

```go
package api

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// GitHub posts a comment on a pull request through the issues endpoint: a
// pull request is an issue with a branch, and /pulls/{n}/comments is the
// review threads instead. Sending a conversation comment there would put it
// on a diff line.
func TestAPullRequestCommentGoesToTheIssuesEndpoint(t *testing.T) {
	t.Parallel()

	c, got := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{}`)
	})
	if err := c.AddPRComment("cli/cli", 61, "looks good"); err != nil {
		t.Fatalf("AddPRComment: %v", err)
	}

	req := (*got)[0]
	if req.Method != http.MethodPost {
		t.Errorf("method = %s, want POST", req.Method)
	}
	if req.URL.Path != "/repos/cli/cli/issues/61/comments" {
		t.Errorf("path = %q", req.URL.Path)
	}
	var sent struct {
		Body string `json:"body"`
	}
	body, _ := io.ReadAll(req.Body)
	if err := json.Unmarshal(body, &sent); err != nil {
		t.Fatalf("parse request: %v", err)
	}
	if sent.Body != "looks good" {
		t.Errorf("body = %q", sent.Body)
	}
}

// Closing a pull request is a different endpoint from closing an issue: the
// issues endpoint would answer for a pull request too, but the pulls one is
// what gh pr close uses and the only one that carries a pull request's own
// fields back.
func TestClosingAPullRequestPatchesThePullsEndpoint(t *testing.T) {
	t.Parallel()

	c, got := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{}`)
	})
	if err := c.ClosePR("cli/cli", 61); err != nil {
		t.Fatalf("ClosePR: %v", err)
	}

	req := (*got)[0]
	if req.Method != http.MethodPatch {
		t.Errorf("method = %s, want PATCH", req.Method)
	}
	if req.URL.Path != "/repos/cli/cli/pulls/61" {
		t.Errorf("path = %q", req.URL.Path)
	}
	var sent map[string]string
	body, _ := io.ReadAll(req.Body)
	_ = json.Unmarshal(body, &sent)
	if sent["state"] != "closed" {
		t.Errorf("state = %q, want closed", sent["state"])
	}
}

func TestReopeningAnIssuePatchesTheIssuesEndpointBackToOpen(t *testing.T) {
	t.Parallel()

	c, got := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{}`)
	})
	if err := c.ReopenIssue("cli/cli", 50); err != nil {
		t.Fatalf("ReopenIssue: %v", err)
	}

	req := (*got)[0]
	if req.URL.Path != "/repos/cli/cli/issues/50" {
		t.Errorf("path = %q", req.URL.Path)
	}
	var sent map[string]string
	body, _ := io.ReadAll(req.Body)
	_ = json.Unmarshal(body, &sent)
	if sent["state"] != "open" {
		t.Errorf("state = %q, want open", sent["state"])
	}
}

// A label's name is a path segment, and GitHub's own labels have spaces in
// them ("help wanted", "good first issue"). An unescaped name would address a
// different label, or none.
func TestRemovingALabelEscapesItsNameIntoThePath(t *testing.T) {
	t.Parallel()

	c, got := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `[]`)
	})
	if err := c.EditIssueLabels("cli/cli", 50, nil, []string{"help wanted"}); err != nil {
		t.Fatalf("EditIssueLabels: %v", err)
	}

	req := (*got)[0]
	if req.Method != http.MethodDelete {
		t.Errorf("method = %s, want DELETE", req.Method)
	}
	if req.URL.EscapedPath() != "/repos/cli/cli/issues/50/labels/help%20wanted" {
		t.Errorf("escaped path = %q", req.URL.EscapedPath())
	}
}

// Additions travel as one request and each removal as its own, so a caller
// asking for both must not lose either half.
func TestEditingLabelsSendsTheAdditionsTogetherAndEachRemovalOnItsOwn(t *testing.T) {
	t.Parallel()

	c, got := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `[]`)
	})
	if err := c.EditPRLabels("cli/cli", 61, []string{"bug", "docs"}, []string{"stale", "keep"}); err != nil {
		t.Fatalf("EditPRLabels: %v", err)
	}
	if len(*got) != 3 {
		t.Fatalf("requests = %d, want 3 (one add, two removes)", len(*got))
	}
	var added struct {
		Labels []string `json:"labels"`
	}
	body, _ := io.ReadAll((*got)[0].Body)
	_ = json.Unmarshal(body, &added)
	if len(added.Labels) != 2 {
		t.Errorf("added labels = %v, want both in one request", added.Labels)
	}
}

// Removing assignees carries them in the body, not the path: GitHub takes a
// list there, and the endpoint is the same one additions go to.
func TestRemovingAssigneesCarriesThemInTheBody(t *testing.T) {
	t.Parallel()

	c, got := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{}`)
	})
	if err := c.EditIssueAssignees("cli/cli", 50, nil, []string{"octocat"}); err != nil {
		t.Fatalf("EditIssueAssignees: %v", err)
	}

	req := (*got)[0]
	if req.Method != http.MethodDelete {
		t.Errorf("method = %s, want DELETE", req.Method)
	}
	if req.URL.Path != "/repos/cli/cli/issues/50/assignees" {
		t.Errorf("path = %q", req.URL.Path)
	}
	var sent struct {
		Assignees []string `json:"assignees"`
	}
	body, _ := io.ReadAll(req.Body)
	_ = json.Unmarshal(body, &sent)
	if len(sent.Assignees) != 1 || sent.Assignees[0] != "octocat" {
		t.Errorf("assignees = %v", sent.Assignees)
	}
}

// An edit with nothing on one side must not send an empty request: POSTing an
// empty label list is a change GitHub accepts and applies.
func TestAnEditWithNothingToAddSendsNoAddRequest(t *testing.T) {
	t.Parallel()

	c, got := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `[]`)
	})
	if err := c.EditPRLabels("cli/cli", 61, nil, nil); err != nil {
		t.Fatalf("EditPRLabels: %v", err)
	}
	if len(*got) != 0 {
		t.Errorf("requests = %d, want none", len(*got))
	}
}
```

- [ ] **Step 2: 落ちることを確認する**

Run: `go test ./internal/gh/api/ -run 'Comment|Closing|Reopening|Label|Assignee|Edit' -v`
Expected: FAIL（メソッドが未定義）

- [ ] **Step 3: `items.go` を書く**

```go
package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// itemPath is where one pull request or issue lives. The two have separate
// endpoints for their own fields, and share the issues one for everything
// GitHub gives both: comments, labels, assignees.
func (c *Client) itemPath(kind, repo string, number int) (string, error) {
	r, err := c.repoPath(repo)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("repos/%s/%s/%d", r, kind, number), nil
}

// comment posts one conversation comment. A pull request takes this path too:
// /pulls/{n}/comments is the review threads instead, and a comment sent there
// would land on a diff line.
func (c *Client) comment(repo string, number int, body string) error {
	path, err := c.itemPath("issues", repo, number)
	if err != nil {
		return err
	}
	_, err = c.write(context.Background(), http.MethodPost, path+"/comments",
		map[string]string{"body": body})
	return err
}

func (c *Client) AddPRComment(repo string, number int, body string) error {
	return c.comment(repo, number, body)
}

func (c *Client) AddIssueComment(repo string, number int, body string) error {
	return c.comment(repo, number, body)
}

// setState opens or closes one item. kind picks the endpoint: a pull request
// is closed through /pulls, the way gh pr close does it.
func (c *Client) setState(kind, repo string, number int, state string) error {
	path, err := c.itemPath(kind, repo, number)
	if err != nil {
		return err
	}
	_, err = c.write(context.Background(), http.MethodPatch, path,
		map[string]string{"state": state})
	return err
}

func (c *Client) ClosePR(repo string, number int) error {
	return c.setState("pulls", repo, number, "closed")
}

func (c *Client) ReopenPR(repo string, number int) error {
	return c.setState("pulls", repo, number, "open")
}

func (c *Client) CloseIssue(repo string, number int) error {
	return c.setState("issues", repo, number, "closed")
}

func (c *Client) ReopenIssue(repo string, number int) error {
	return c.setState("issues", repo, number, "open")
}

// editLabels adds and removes labels on one item. Additions travel together;
// a removal names its label in the path, one request each, which is the only
// shape GitHub offers.
func (c *Client) editLabels(repo string, number int, add, remove []string) error {
	path, err := c.itemPath("issues", repo, number)
	if err != nil {
		return err
	}
	if len(add) > 0 {
		if _, err := c.write(context.Background(), http.MethodPost, path+"/labels",
			map[string][]string{"labels": add}); err != nil {
			return err
		}
	}
	for _, name := range remove {
		// A label name is a path segment and GitHub's own have spaces in
		// them ("help wanted"), which would otherwise address another label.
		if _, err := c.write(context.Background(), http.MethodDelete,
			path+"/labels/"+url.PathEscape(name), nil); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) EditPRLabels(repo string, number int, add, remove []string) error {
	return c.editLabels(repo, number, add, remove)
}

func (c *Client) EditIssueLabels(repo string, number int, add, remove []string) error {
	return c.editLabels(repo, number, add, remove)
}

// editAssignees adds and removes assignees on one item. Both sides take a
// list in the body at the same path; the method is what separates them.
func (c *Client) editAssignees(repo string, number int, add, remove []string) error {
	path, err := c.itemPath("issues", repo, number)
	if err != nil {
		return err
	}
	if len(add) > 0 {
		if _, err := c.write(context.Background(), http.MethodPost, path+"/assignees",
			map[string][]string{"assignees": add}); err != nil {
			return err
		}
	}
	if len(remove) > 0 {
		if _, err := c.write(context.Background(), http.MethodDelete, path+"/assignees",
			map[string][]string{"assignees": remove}); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) EditPRAssignees(repo string, number int, add, remove []string) error {
	return c.editAssignees(repo, number, add, remove)
}

func (c *Client) EditIssueAssignees(repo string, number int, add, remove []string) error {
	return c.editAssignees(repo, number, add, remove)
}
```

- [ ] **Step 4: 通ることを確認する**

Run: `go test ./internal/gh/api/ -v`
Expected: PASS

- [ ] **Step 5: 落ちることを確かめる（空振り確認）**

`url.PathEscape` を外して `TestRemovingALabelEscapes...` が RED になること、
`editLabels` の `len(add) > 0` を外して `TestAnEditWithNothingToAdd...` が
RED になることを確かめてから戻す。

- [ ] **Step 6: コミット**

```bash
git add internal/gh/api/items.go internal/gh/api/items_test.go
git commit -m "feat: comment, close, reopen and edit items through REST"
```

---

## Task 3: `ListLabels` と `ListAssignees`

**Files:**
- Create: `internal/gh/api/lists.go`
- Create: `internal/gh/api/lists_test.go`
- Create: `internal/gh/api/testdata/labels.json`
- Create: `internal/gh/api/testdata/assignees.json`
- Create: `internal/gh/api/testdata/README.md`

**Interfaces:**
- Consumes: Task 1 の `read` / `repoPath`
- Produces: `ListLabels(ctx, repo) ([]gh.Label, error)` / `ListAssignees(ctx, repo) ([]string, error)`

- [ ] **Step 1: fixture を録る**

```bash
gh api 'repos/cli/cli/labels?per_page=100' > internal/gh/api/testdata/labels.json
gh api 'repos/cli/cli/assignees?per_page=100' \
  --jq '[.[] | {login, id}]' > internal/gh/api/testdata/assignees.json
```

assignees は `jq` で `login` と `id` に絞る（録りものに他人のプロフィール URL を
入れない）。**録ったあと中身を読み、私有情報が無いことを確かめる。**

`internal/gh/api/testdata/README.md`:

```markdown
# testdata

GitHub の REST API から実際に録った応答。手書きしない。
テストを通すために編集しない（落ちたら実装を疑う）。

| ファイル | 録り方 | 録った日 | 対象 |
|---|---|---|---|
| `labels.json` | `gh api 'repos/cli/cli/labels?per_page=100'` | 2026-09-13 | `cli/cli`（83 件） |
| `assignees.json` | `gh api 'repos/cli/cli/assignees?per_page=100' --jq '[.[] | {login, id}]'` | 2026-09-13 | `cli/cli` |

`assignees.json` は `jq` で `login` と `id` に絞ってある。生の応答には
プロフィール URL とアバター URL が並び、録りものに残す理由が無い。
```

- [ ] **Step 2: 失敗するテストを書く**

`internal/gh/api/lists_test.go`:

```go
package api

import (
	"context"
	"io"
	"net/http"
	"os"
	"testing"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return string(b)
}

// gh label list asks GraphQL for CREATED_AT ascending; REST answers by name.
// Nothing between here and the label picker reorders the list, so leaving
// REST's order alone would silently reshuffle the picker for anyone on this
// backend. Sorting by id restores gh's order -- verified against all 83 of
// cli/cli's labels on 2026-09-13.
func TestLabelsComeBackInTheOrderTheWereCreatedNotAlphabetically(t *testing.T) {
	t.Parallel()

	c, _ := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, fixture(t, "labels.json"))
	})
	labels, err := c.ListLabels(context.Background(), "cli/cli")
	if err != nil {
		t.Fatalf("ListLabels: %v", err)
	}
	if len(labels) != 83 {
		t.Fatalf("labels = %d, want 83", len(labels))
	}
	if labels[0].Name != "bug" {
		t.Errorf("first label = %q, want bug (the oldest); alphabetical would be accessibility", labels[0].Name)
	}
	if labels[1].Name != "blocked" {
		t.Errorf("second label = %q, want blocked", labels[1].Name)
	}
	if labels[0].Color == "" {
		t.Error("label carries no colour; the picker draws one")
	}
}

// The picker shows 100 assignable users at most, the same ceiling the cli
// backend reads. Asking for the default 30 would hide most of a large team.
func TestAssigneesAskForAFullPageNotTheDefaultThirty(t *testing.T) {
	t.Parallel()

	c, got := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, fixture(t, "assignees.json"))
	})
	logins, err := c.ListAssignees(context.Background(), "cli/cli")
	if err != nil {
		t.Fatalf("ListAssignees: %v", err)
	}
	if len(logins) == 0 {
		t.Fatal("no logins parsed")
	}
	if logins[0] == "" {
		t.Error("first login is empty")
	}
	if per := (*got)[0].URL.Query().Get("per_page"); per != "100" {
		t.Errorf("per_page = %q, want 100", per)
	}
}
```

- [ ] **Step 3: 落ちることを確認する**

Run: `go test ./internal/gh/api/ -run 'Labels|Assignees' -v`
Expected: FAIL（`ListLabels` / `ListAssignees` が未定義）

- [ ] **Step 4: `lists.go` を書く**

```go
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/kukv/octoscope/internal/gh"
)

// pageSize is REST's maximum for one page, and the ceiling the cli backend
// reads too. Asking for more is a 422.
const pageSize = 100

// labelJSON is one entry of the labels endpoint. The id is what puts the list
// back in the order gh shows: gh asks GraphQL for CREATED_AT ascending, while
// REST answers by name, and a label's id rises with its creation.
type labelJSON struct {
	Name  string `json:"name"`
	Color string `json:"color"`
	ID    int64  `json:"id"`
}

// ListLabels names the repository's labels, oldest first. Nothing between
// here and the picker reorders them.
func (c *Client) ListLabels(ctx context.Context, repo string) ([]gh.Label, error) {
	r, err := c.repoPath(repo)
	if err != nil {
		return nil, err
	}
	out, err := c.read(ctx, fmt.Sprintf("repos/%s/labels?per_page=%d", r, pageSize), "")
	if err != nil {
		return nil, err
	}
	var found []labelJSON
	if err := json.Unmarshal(out, &found); err != nil {
		return nil, fmt.Errorf("parse labels: %w", err)
	}
	slices.SortFunc(found, func(a, b labelJSON) int {
		switch {
		case a.ID < b.ID:
			return -1
		case a.ID > b.ID:
			return 1
		default:
			return 0
		}
	})
	labels := make([]gh.Label, len(found))
	for i, f := range found {
		labels[i] = gh.Label{Name: f.Name, Color: f.Color}
	}
	return labels, nil
}

// ListAssignees returns the logins of users assignable on the repository.
// The request is not paged: a repository with more than a page of assignable
// users would give the picker a list nobody could pick from anyway, and the
// cli backend reads the same one page.
func (c *Client) ListAssignees(ctx context.Context, repo string) ([]string, error) {
	r, err := c.repoPath(repo)
	if err != nil {
		return nil, err
	}
	out, err := c.read(ctx, fmt.Sprintf("repos/%s/assignees?per_page=%d", r, pageSize), "")
	if err != nil {
		return nil, err
	}
	var users []gh.Author
	if err := json.Unmarshal(out, &users); err != nil {
		return nil, fmt.Errorf("parse assignees: %w", err)
	}
	logins := make([]string, len(users))
	for i, u := range users {
		logins[i] = u.Login
	}
	return logins, nil
}
```

- [ ] **Step 5: 通ることを確認する**

Run: `go test ./internal/gh/api/ -v`
Expected: PASS

- [ ] **Step 6: 落ちることを確かめる（空振り確認）**

`slices.SortFunc` の行を消して `TestLabelsComeBackInTheOrder...` が RED に
なることを確かめてから戻す。**この 1 つが並び順のパリティを守る唯一の見張りである。**

- [ ] **Step 7: コミット**

```bash
git add internal/gh/api/lists.go internal/gh/api/lists_test.go internal/gh/api/testdata/
git commit -m "feat: list labels and assignees through REST, in gh's order"
```

---

## Task 4: `PRDiff` と、`parseDiff` を `internal/gh` へ移す

**Files:**
- Create: `internal/gh/diff_parse.go`（`cli/diff.go` から移す）
- Create: `internal/gh/diff_parse_test.go`（`cli/diff_test.go` から移す）
- Modify: `internal/gh/cli/diff.go`（`gh` の呼び出しとパス組み立てだけに縮む）
- Modify: `internal/gh/cli/diff_test.go`（移した分を削り、`gh` の引数の検証を残す）
- Create: `internal/gh/api/diff.go`
- Create: `internal/gh/api/diff_test.go`
- Create: `internal/gh/api/testdata/pr_diff.txt`
- Create: `internal/gh/api/testdata/pr_files.json`

**Interfaces:**
- Produces（`internal/gh`）:
  - `func ParseDiff(out []byte) []FileDiff`
  - `func ParseFilesAPI(out []byte) ([]FileDiff, error)`
- Produces（`internal/gh/api`）: `PRDiff(ctx, repo string, number int) ([]gh.FileDiff, error)`
- Consumes: Task 1 の `send` / `read` / `repoPath` / `nextLink`

- [ ] **Step 1: `cli` から `internal/gh` へ移す（振る舞いは変えない）**

`internal/gh/cli/diff.go` から次を `internal/gh/diff_parse.go` へ移す:

- 定数 `scanBufInit` / `scanBufMax` / `hunkHeaderFields`
- `parseDiff` → `ParseDiff` に改名（export）
- `prFileJSON` と `toDomain`（非公開のまま）
- `prFileJSON` を使うデコードを `ParseFilesAPI` にまとめる:

```go
// ParseFilesAPI decodes the files API's answer, which is what a diff falls
// back to. Both backends read the same shape: the cli one through gh api, the
// api one through the endpoint itself.
func ParseFilesAPI(out []byte) ([]FileDiff, error) {
	var entries []prFileJSON
	if err := json.Unmarshal(out, &entries); err != nil {
		return nil, fmt.Errorf("parse pr files: %w", err)
	}
	files := make([]FileDiff, len(entries))
	for i, e := range entries {
		files[i] = e.toDomain()
	}
	return files, nil
}
```

`internal/gh/cli/diff.go` は `parseDiff(out)` を `gh.ParseDiff(out)` に、
`prFiles` の中の decode を `gh.ParseFilesAPI(out)` に差し替える。**それ以外は
変えない。**

`internal/gh/cli/diff_test.go` のうち、パースそのものを見ているテストを
`internal/gh/diff_parse_test.go` へ移す。`gh pr diff` の引数を見ているテスト
（`--color never` があること、フォールバックが走ること）は `cli` に残す。

**テストのパッケージは `package gh`。** `internal/gh` は両方が混在しているが
（`checks_test.go` / `merge_test.go` は `package gh`、`gh_test.go` は
`package gh_test`）、移すテストは `prFileJSON` と `toDomain` を直接見ており、
これらは移したあとも非公開のままなので内部テストパッケージしか選べない。

- [ ] **Step 2: 移しただけで緑なことを確認する**

Run: `make test`
Expected: PASS（テストの本数は変わらない。移動だけ）

- [ ] **Step 3: コミット**

```bash
git add internal/gh/diff_parse.go internal/gh/diff_parse_test.go internal/gh/cli/diff.go internal/gh/cli/diff_test.go
git commit -m "refactor: move diff parsing where both backends can reach it"
```

- [ ] **Step 4: fixture を録る**

```bash
gh api repos/kukv/octoscope/pulls/<N> \
  -H 'Accept: application/vnd.github.v3.diff' > internal/gh/api/testdata/pr_diff.txt
gh api 'repos/kukv/octoscope/pulls/<N>/files?per_page=100' \
  > internal/gh/api/testdata/pr_files.json
```

**`--jq` で絞らない。** `patch` は GitHub が binary や大きすぎるファイルに対して
**フィールドごと省く**ものであり、`{... patch}` の射影はそれを明示的な `null` に
変えてしまう。`toDomain` が `*string` で受けているのはまさにその区別のためで、
形を変えた録りものはその分岐を守らない（`.claude/rules/testing.md`「実物を録る」）。
自分の公開リポジトリなので伏せるものも無い。

`<N>` は**小さめのマージ済み PR**（5 ファイル程度）を選ぶ。テストが見るのは
`len(files) > 0` と Accept ヘッダだけで、レビューする人がパース結果と突き合わせ
やすいほうがよい。`testdata/README.md` に 2 行足し、**録ったあと中身を読んで
確かめる。**

- [ ] **Step 5: 失敗するテストを書く**

下のテストは `<N>` を 82 として書いてある。**Step 4 で別の番号を選んだら、
テスト中の `82` とパスの期待値をその番号に揃えること。**

`internal/gh/api/diff_test.go`:

```go
package api

import (
	"context"
	"io"
	"net/http"
	"testing"
)

// gh pr diff is a GET on the pull request with the diff media type -- verified
// with GH_DEBUG=api on 2026-09-13. Asking for JSON there answers with the pull
// request's fields instead, and the parser would find no files at all.
func TestADiffAsksForTheDiffMediaType(t *testing.T) {
	t.Parallel()

	c, got := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, fixture(t, "pr_diff.txt"))
	})
	files, err := c.PRDiff(context.Background(), "kukv/octoscope", 82)
	if err != nil {
		t.Fatalf("PRDiff: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no files parsed")
	}
	req := (*got)[0]
	if a := req.Header.Get("Accept"); a != "application/vnd.github.v3.diff" {
		t.Errorf("Accept = %q", a)
	}
	if req.URL.Path != "/repos/kukv/octoscope/pulls/82" {
		t.Errorf("path = %q", req.URL.Path)
	}
}

// GitHub refuses a diff past a few hundred files; the files API has no such
// limit. The fallback is what makes a large pull request readable at all, so
// a failure on the first call must not end the call.
func TestADiffGitHubRefusesFallsBackToTheFilesApi(t *testing.T) {
	t.Parallel()

	calls := 0
	c, got := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusNotAcceptable)
			_, _ = io.WriteString(w, `{"message":"too large"}`)
			return
		}
		_, _ = io.WriteString(w, fixture(t, "pr_files.json"))
	})
	files, err := c.PRDiff(context.Background(), "kukv/octoscope", 82)
	if err != nil {
		t.Fatalf("PRDiff: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no files parsed from the fallback")
	}
	if (*got)[1].URL.Path != "/repos/kukv/octoscope/pulls/82/files" {
		t.Errorf("fallback path = %q", (*got)[1].URL.Path)
	}
}

// The files API's default page is 30 and one pull request under review had
// 418 files, so a caller that reads only the first page loses most of them.
func TestTheFilesApiFallbackWalksEveryPage(t *testing.T) {
	t.Parallel()

	calls := 0
	var srvURL string
	c, got := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		switch calls {
		case 1:
			w.WriteHeader(http.StatusNotAcceptable)
			_, _ = io.WriteString(w, `{"message":"too large"}`)
		case 2:
			w.Header().Set("Link", `<`+srvURL+`/repos/kukv/octoscope/pulls/82/files?page=2>; rel="next"`)
			_, _ = io.WriteString(w, `[{"filename":"a.go","status":"modified","additions":1,"deletions":0,"patch":"@@ -1 +1 @@\n-a\n+b"}]`)
		case 3:
			w.Header().Set("Link", `<`+srvURL+`/repos/kukv/octoscope/pulls/82/files?page=3>; rel="next"`)
			_, _ = io.WriteString(w, `[{"filename":"b.go","status":"modified","additions":1,"deletions":0,"patch":"@@ -1 +1 @@\n-a\n+b"}]`)
		default:
			_, _ = io.WriteString(w, `[{"filename":"c.go","status":"modified","additions":1,"deletions":0,"patch":"@@ -1 +1 @@\n-a\n+b"}]`)
		}
	})
	srvURL = c.baseURL

	files, err := c.PRDiff(context.Background(), "kukv/octoscope", 82)
	if err != nil {
		t.Fatalf("PRDiff: %v", err)
	}
	if len(files) != 3 {
		t.Errorf("files = %d, want 3 (one per page)", len(files))
	}
	if len(*got) != 4 {
		t.Errorf("requests = %d, want 4 (the refused diff and three pages)", len(*got))
	}
}
```

- [ ] **Step 6: 落ちることを確認する**

Run: `go test ./internal/gh/api/ -run 'Diff|FilesApi' -v`
Expected: FAIL（`PRDiff` が未定義）

- [ ] **Step 7: `internal/gh/api/diff.go` を書く**

```go
package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/kukv/octoscope/internal/gh"
)

// diffMediaType is what asks the pull request endpoint for a unified diff
// rather than the pull request's own fields.
const diffMediaType = "application/vnd.github.v3.diff"

// PRDiff returns the pull request's diff, one entry per file.
//
// GitHub refuses a diff past a few hundred files; the files API has no such
// limit. Rather than matching GitHub's error text for that (its wording is
// not ours to depend on), any failure here is retried through the files API,
// which is cheap since failures are rare. If that also fails, both errors are
// joined rather than one discarding the other: this one still describes what
// the user actually asked for, but a bug in the fallback itself must not go
// unseen either.
func (c *Client) PRDiff(ctx context.Context, repo string, number int) ([]gh.FileDiff, error) {
	r, err := c.repoPath(repo)
	if err != nil {
		return nil, err
	}
	out, derr := c.read(ctx, fmt.Sprintf("repos/%s/pulls/%d", r, number), diffMediaType)
	if derr != nil {
		files, ferr := c.prFiles(ctx, r, number)
		if ferr == nil {
			return files, nil
		}
		return nil, errors.Join(derr, ferr)
	}
	return gh.ParseDiff(out), nil
}

// prFiles is the files-API fallback for PRDiff. It walks every page: the
// endpoint's default page is 30 files, and the pull request this fallback
// exists for had 418.
func (c *Client) prFiles(ctx context.Context, repo string, number int) ([]gh.FileDiff, error) {
	url := c.restURL(fmt.Sprintf("repos/%s/pulls/%d/files?per_page=%d", repo, number, pageSize))
	var files []gh.FileDiff
	for url != "" {
		out, header, err := c.send(ctx, http.MethodGet, url, nil, "")
		if err != nil {
			return nil, err
		}
		page, err := gh.ParseFilesAPI(out)
		if err != nil {
			return nil, err
		}
		files = append(files, page...)
		url = nextLink(header)
	}
	return files, nil
}
```

- [ ] **Step 8: 通ることを確認する**

Run: `make test`
Expected: PASS

- [ ] **Step 9: 落ちることを確かめる（空振り確認）**

`prFiles` のループを 1 ページで抜ける形に変えて
`TestTheFilesApiFallbackWalksEveryPage` が RED になること、`diffMediaType` を
`jsonMediaType` に変えて `TestADiffAsksForTheDiffMediaType` が RED になることを
確かめてから戻す。

- [ ] **Step 10: コミット**

```bash
git add internal/gh/api/diff.go internal/gh/api/diff_test.go internal/gh/api/testdata/
git commit -m "feat: fetch a pull request's diff through REST, files API and all"
```

---

## Task 5: `SearchRepos` / `ListOwnRepos` / `ListOrgs`

**Files:**
- Create: `internal/gh/api/repos.go`
- Create: `internal/gh/api/repos_test.go`
- Create: `internal/gh/api/testdata/search_repos.json`
- Create: `internal/gh/api/testdata/user_repos.json`
- Create: `internal/gh/api/testdata/user_orgs.json`

**Interfaces:**
- Consumes: Task 1 の `read`
- Produces: `SearchRepos(ctx, query string, limit int) ([]gh.RepoCandidate, error)` /
  `ListOwnRepos(ctx, owner string, limit int) ([]gh.RepoCandidate, error)` /
  `ListOrgs(ctx) ([]string, error)`

- [ ] **Step 1: fixture を録る**

```bash
gh api 'search/repositories?q=octoscope&per_page=5' \
  > internal/gh/api/testdata/search_repos.json
gh api 'user/repos?affiliation=owner&sort=pushed&direction=desc&per_page=5' \
  --jq '[.[] | {full_name, private}]' > internal/gh/api/testdata/user_repos.json
gh api user/orgs --jq '[.[] | {login}]' > internal/gh/api/testdata/user_orgs.json
```

`search_repos.json` は**絞らない**（公開の検索結果で、伏せるものが無い）。
`user_repos.json` と `user_orgs.json` は自分のアカウントの中身なので `jq` で
絞る。**録ったあと読んで、private なリポジトリ名や所属が残っていないか確かめる。
残っていたら公開リポジトリだけに絞り直すか、`cli/cli` で代替する。**
`testdata/README.md` に 3 行足す。

- [ ] **Step 2: 失敗するテストを書く**

`internal/gh/api/repos_test.go`:

```go
package api

import (
	"context"
	"io"
	"net/http"
	"testing"
)

// The dialog's suggestions are typed into, so a word starting with a dash or
// carrying a space has to survive the trip. A query pasted into the path
// unescaped would be a different search, or a 404.
func TestASearchEscapesTheTypedQueryIntoTheParameter(t *testing.T) {
	t.Parallel()

	c, got := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, fixture(t, "search_repos.json"))
	})
	found, err := c.SearchRepos(context.Background(), "go tui/term", 5)
	if err != nil {
		t.Fatalf("SearchRepos: %v", err)
	}
	if len(found) == 0 {
		t.Fatal("no candidates parsed")
	}
	if found[0].Name == "" {
		t.Error("candidate has no name")
	}
	req := (*got)[0]
	if q := req.URL.Query().Get("q"); q != "go tui/term" {
		t.Errorf("q = %q, want the query as typed", q)
	}
	if per := req.URL.Query().Get("per_page"); per != "5" {
		t.Errorf("per_page = %q, want the limit the caller asked for", per)
	}
}

// REST caps a page at 100 and answers 422 above it. The seeding path asks for
// 100, so one more would turn every first run into an error.
func TestASearchNeverAsksForMoreThanAPage(t *testing.T) {
	t.Parallel()

	c, got := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, fixture(t, "search_repos.json"))
	})
	if _, err := c.SearchRepos(context.Background(), "go", 500); err != nil {
		t.Fatalf("SearchRepos: %v", err)
	}
	if per := (*got)[0].URL.Query().Get("per_page"); per != "100" {
		t.Errorf("per_page = %q, want 100", per)
	}
}

// gh repo list shows the repositories the user owns, most recently pushed
// first. Leaving the order to REST's default would put the seeding dialog's
// candidates in a different order than gh's, for the same account.
func TestOwnReposAreTheOnesOwnedMostRecentlyPushedFirst(t *testing.T) {
	t.Parallel()

	c, got := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, fixture(t, "user_repos.json"))
	})
	repos, err := c.ListOwnRepos(context.Background(), "", 100)
	if err != nil {
		t.Fatalf("ListOwnRepos: %v", err)
	}
	if len(repos) == 0 {
		t.Fatal("no repositories parsed")
	}
	q := (*got)[0].URL.Query()
	if (*got)[0].URL.Path != "/user/repos" {
		t.Errorf("path = %q, want /user/repos for the authenticated user", (*got)[0].URL.Path)
	}
	if q.Get("affiliation") != "owner" {
		t.Errorf("affiliation = %q, want owner", q.Get("affiliation"))
	}
	if q.Get("sort") != "pushed" || q.Get("direction") != "desc" {
		t.Errorf("sort = %q %q, want pushed desc", q.Get("sort"), q.Get("direction"))
	}
}

// A named owner is an organisation -- the seeding path only ever passes one
// of ListOrgs' answers. /users/{owner}/repos would answer too, but shows only
// what is public, hiding exactly the repositories a member joined for.
func TestANamedOwnerReadsTheOrganisationsRepositories(t *testing.T) {
	t.Parallel()

	c, got := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, fixture(t, "user_repos.json"))
	})
	if _, err := c.ListOwnRepos(context.Background(), "kukv", 100); err != nil {
		t.Fatalf("ListOwnRepos: %v", err)
	}
	if (*got)[0].URL.Path != "/orgs/kukv/repos" {
		t.Errorf("path = %q, want /orgs/kukv/repos", (*got)[0].URL.Path)
	}
}

func TestOrgsComeBackAsLogins(t *testing.T) {
	t.Parallel()

	c, _ := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, fixture(t, "user_orgs.json"))
	})
	orgs, err := c.ListOrgs(context.Background())
	if err != nil {
		t.Fatalf("ListOrgs: %v", err)
	}
	for i, o := range orgs {
		if o == "" {
			t.Errorf("org %d has an empty login", i)
		}
	}
}
```

`user_orgs.json` が空配列になるアカウントもある。**その場合は
`TestOrgsComeBackAsLogins` が何も検証しなくなるので、fixture は Org に
所属しているアカウントで録るか、録れなければ `cli/cli` の
`gh api users/cli/orgs` で代替して README にそう書く。**

- [ ] **Step 3: 落ちることを確認する**

Run: `go test ./internal/gh/api/ -run 'Search|OwnRepos|NamedOwner|Orgs' -v`
Expected: FAIL

- [ ] **Step 4: `repos.go` を書く**

```go
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/kukv/octoscope/internal/gh"
)

// repoJSON is one repository as REST spells it, in the two shapes that carry
// it: the search results and the listings.
type repoJSON struct {
	FullName string `json:"full_name"`
	Stars    int    `json:"stargazers_count"`
	Private  bool   `json:"private"`
}

func (r repoJSON) toDomain() gh.RepoCandidate {
	return gh.RepoCandidate{Name: r.FullName, Stars: r.Stars, Private: r.Private}
}

// page is what REST will actually answer with. Asking for more than a page is
// a 422, so a caller's larger limit is cut down rather than sent.
func page(limit int) int {
	if limit > pageSize || limit <= 0 {
		return pageSize
	}
	return limit
}

// SearchRepos looks for repositories matching query.
func (c *Client) SearchRepos(ctx context.Context, query string, limit int) ([]gh.RepoCandidate, error) {
	path := fmt.Sprintf("search/repositories?q=%s&per_page=%d",
		url.QueryEscape(query), page(limit))
	out, err := c.read(ctx, path, "")
	if err != nil {
		return nil, err
	}
	var found struct {
		Items []repoJSON `json:"items"`
	}
	if err := json.Unmarshal(out, &found); err != nil {
		return nil, fmt.Errorf("parse repo search: %w", err)
	}
	candidates := make([]gh.RepoCandidate, len(found.Items))
	for i, f := range found.Items {
		candidates[i] = f.toDomain()
	}
	return candidates, nil
}

// ListOwnRepos lists the repositories of owner, or of the authenticated user
// when owner is empty, most recently pushed first -- the order gh repo list
// shows them in.
//
// A named owner is read as an organisation: the only caller passes either an
// empty string or one of ListOrgs' answers. /users/{owner}/repos would answer
// for a person, but shows only public repositories.
func (c *Client) ListOwnRepos(ctx context.Context, owner string, limit int) ([]gh.RepoCandidate, error) {
	path := fmt.Sprintf("user/repos?affiliation=owner&sort=pushed&direction=desc&per_page=%d", page(limit))
	if owner != "" {
		path = fmt.Sprintf("orgs/%s/repos?sort=pushed&direction=desc&per_page=%d",
			url.PathEscape(owner), page(limit))
	}
	out, err := c.read(ctx, path, "")
	if err != nil {
		return nil, err
	}
	var found []repoJSON
	if err := json.Unmarshal(out, &found); err != nil {
		return nil, fmt.Errorf("parse repo list: %w", err)
	}
	repos := make([]gh.RepoCandidate, len(found))
	for i, f := range found {
		// The listings carry a star count too, but the seeding dialog reads
		// these by name; keeping it costs nothing and the field is there.
		repos[i] = f.toDomain()
	}
	return repos, nil
}

// ListOrgs names the organisations the authenticated user belongs to.
func (c *Client) ListOrgs(ctx context.Context) ([]string, error) {
	out, err := c.read(ctx, "user/orgs", "")
	if err != nil {
		return nil, err
	}
	var found []struct {
		Login string `json:"login"`
	}
	if err := json.Unmarshal(out, &found); err != nil {
		return nil, fmt.Errorf("parse orgs: %w", err)
	}
	logins := make([]string, len(found))
	for i, f := range found {
		logins[i] = f.Login
	}
	return logins, nil
}
```

- [ ] **Step 5: 通ることを確認する**

Run: `make test`
Expected: PASS

- [ ] **Step 6: 落ちることを確かめる（空振り確認）**

`page(limit)` の頭打ちを外して `TestASearchNeverAsksForMoreThanAPage` が RED に
なること、`sort=pushed&direction=desc` を落として
`TestOwnReposAreTheOnesOwnedMostRecentlyPushedFirst` が RED になることを
確かめてから戻す。

- [ ] **Step 7: ファイルの行数を数える**

```bash
wc -l internal/gh/api/*.go
```

300 行を超えたファイルがあれば、責務が 1 つか確かめ、割らないなら理由を
Task 6 の積み残しに書く。**数える前に割らない。**

- [ ] **Step 8: コミット**

```bash
git add internal/gh/api/repos.go internal/gh/api/repos_test.go internal/gh/api/testdata/
git commit -m "feat: search, list and seed repositories through REST"
```

---

## Task 6: パリティの見張りと、設計 §6 の訂正・積み残し

**Files:**
- Modify: `internal/gh/api/parity_test.go`
- Modify: `docs/superpowers/specs/2026-09-08-phase4-design.md`（§6 の表）
- Create: `docs/superpowers/2026-09-13-phase4-rest-backend-followups.md`

**Interfaces:** なし（このタスクはコードの振る舞いを変えない）

- [ ] **Step 1: `parity_test.go` に REST 分を足す**

`graphQLSource` はそのまま残し、**別の宣言として**足す。1 つの interface に
直接並べるメソッドは 6 個までなので、3 つに割って embed でまとめる
（`.claude/rules/architecture.md`）。

```go
// restSource is the part of the usecase layer's source interface that REST
// answers. Both backends satisfy it with the same domain types: a method only
// one of them has would leave the other's screens empty.
//
// Actions (RerunWorkflow, JobLog) are the last group and arrive in the next
// slice; this file goes away then, when usecase.New(api.New(...)) compiles
// and the compiler itself becomes the parity check.
type restWriter interface {
	AddPRComment(repo string, number int, body string) error
	AddIssueComment(repo string, number int, body string) error
	ClosePR(repo string, number int) error
	ReopenPR(repo string, number int) error
	CloseIssue(repo string, number int) error
	ReopenIssue(repo string, number int) error
}

type restEditor interface {
	EditPRLabels(repo string, number int, add, remove []string) error
	EditIssueLabels(repo string, number int, add, remove []string) error
	EditPRAssignees(repo string, number int, add, remove []string) error
	EditIssueAssignees(repo string, number int, add, remove []string) error
}

type restReader interface {
	ListLabels(ctx context.Context, repo string) ([]gh.Label, error)
	ListAssignees(ctx context.Context, repo string) ([]string, error)
	PRDiff(ctx context.Context, repo string, number int) ([]gh.FileDiff, error)
	SearchRepos(ctx context.Context, query string, limit int) ([]gh.RepoCandidate, error)
	ListOwnRepos(ctx context.Context, owner string, limit int) ([]gh.RepoCandidate, error)
	ListOrgs(ctx context.Context) ([]string, error)
}

type restSource interface {
	restWriter
	restEditor
	restReader
}

var (
	_ restSource = (*api.Client)(nil)
	_ restSource = (*cli.Client)(nil)
)
```

- [ ] **Step 2: コンパイルが通ることを確認する**

Run: `go test ./internal/gh/api/ -run XXX`
Expected: PASS（`ok`。コンパイルできれば宣言は満たされている）

- [ ] **Step 3: 設計 §6 の表を訂正する**

`docs/superpowers/specs/2026-09-08-phase4-design.md` §6 の表の 2 行目
「`go-github` の REST で `api` 側に実装する」を、実際にしたことに直す。
直下の「依存に増えるのは `go-github` だけで、`githubv4` は増えない。」も
「依存は増えない。`go-github` も `githubv4` も足さない。」に直し、**いつ・
なぜ変えたかを段落で残す**（4-2 が §10 を訂正したときと同じ形）。

- [ ] **Step 4: 積み残しを書く**

`docs/superpowers/2026-09-13-phase4-rest-backend-followups.md` に、
`docs/superpowers/2026-09-12-phase4-api-backend-followups.md` と同じ節立てで:

- **繰り越し**: Actions（4-4）、`main.go` の配線と認証エラー画面（4-4）、
  タイムアウト（4-4。REST も `http.DefaultClient` を使うので対象が増えた）、
  `parseRemote` の明示ポート（4-2 の 4 番。`parseRemote` を触るときに直す）
- **このスライスで決めたこと**: `go-github` を足さなかった理由と実測、
  ラベルの `id` 昇順、`gh pr diff` の正体、`ListOwnRepos` の `owner` が
  Org 限定であるという前提
- **実測**: `gh` のサブコマンドと REST の突き合わせ表（下の Step 5）
- **`internal/gh/cli` から移ったもの**: `parseDiff` と files API のデコード

- [ ] **Step 5: `gh` と突き合わせて実測する**

golden では確かめられないので、実リポジトリに対して数と順序を比べ、
結果を積み残しの表に書く。**中間ファイルはスクラッチパッド**（環境が示す
セッション用ディレクトリ）に置き、プロセス置換（`<(...)`）は使わない
（worktree 隔離のガードに引っかかる）。以下は `$S` をそのディレクトリとする。

```bash
# ラベル: 数と順序
gh label list --repo cli/cli --json name --limit 100 --jq '.[].name' > $S/a.txt
gh api 'repos/cli/cli/labels?per_page=100' --jq '[.[]|{n:.name,id:.id}]|sort_by(.id)|.[].n' > $S/b.txt
diff $S/a.txt $S/b.txt

# assignees: 数
gh api 'repos/cli/cli/assignees?per_page=100' --jq 'length'

# repo list: 数と順序
gh repo list --limit 20 --json nameWithOwner --jq '.[].nameWithOwner' > $S/c.txt
gh api 'user/repos?affiliation=owner&sort=pushed&direction=desc&per_page=20' --jq '.[].full_name' > $S/d.txt
diff $S/c.txt $S/d.txt

# search repos: 数と順序
gh search repos --limit 5 --json fullName --jq '.[].fullName' -- octoscope > $S/e.txt
gh api 'search/repositories?q=octoscope&per_page=5' --jq '.items[].full_name' > $S/f.txt
diff $S/e.txt $S/f.txt

# pr diff: ファイル数（<N> は Task 4 で録った PR と同じ番号）
gh pr diff <N> --color never | grep -c '^diff --git'
gh api 'repos/kukv/octoscope/pulls/<N>/files?per_page=100' --jq 'length'
```

**一致しなかったものは「一致した」と書かない。** 差が出たら、その差が
利用者から見えるものか（並び順・件数）を判断し、見えるなら直し、
見えないなら理由とともに積み残しに書く。

- [ ] **Step 6: 進捗メモリを更新する**

`~/.claude/projects/.../memory/octoscope-phase4-progress.md` の 4-3 の行を
「完了」に直し、**次は 4-4** と書く。このスライスで確定した設計判断 3 つも足す:
`go-github` を足さなかったこと、ラベルは `id` 昇順に並べ替えて `gh` と合わせること、
`gh pr diff` は REST の diff メディアタイプであること。`description` の
frontmatter も現在地に合わせて書き直す。

- [ ] **Step 7: `make check` を通す**

Run: `make check`
Expected: 緑（tidy / lint / fmt / test すべて）

- [ ] **Step 8: コミット**

```bash
git add internal/gh/api/parity_test.go docs/
git commit -m "docs: record what the REST slice decided, and correct the design's table"
```

---

## Self-Review（この計画を書いたあとに確かめたこと）

**1. 設計のカバー:** 設計 §6 の表のうち 4-3 に割り当てられた 9 項目
（`label list` / assignees / コメント / close・reopen / ラベルと担当者の編集 /
`pulls/{n}/files` / `search repos` / `repo list` / `user/orgs`）が、それぞれ
Task 2〜5 のどれかに乗っている。`pr diff` は表では「`pr diff`（失敗時に files API へ
フォールバック）」の 1 項目で、Task 4 が両方を持つ。設計 §8 の「`api` と `cli` が
同じドメイン型を返すことを、両方に同じ検証を当てて確かめる」は Task 6 の
`restSource` と、Task 4 で `ParseDiff` / `ParseFilesAPI` が 1 組になったことで満たす。

**2. プレースホルダ:** 「適切なエラー処理を足す」の類は無い。全ステップに
実際のコードがある。**録りもの（fixture）だけは中身をここに書けない**ので、
録るコマンドと、録ったあと何を確かめるかをステップにした。

**3. 型の一致:** Task 1 が出す `read(ctx, path, accept)` / `write(ctx, method, path, body)` /
`send(ctx, method, url, body, accept)` / `repoPath(repo)` / `restURL(path)` /
`nextLink(h)` / `statusError(status, body)` の名前と引数は、Task 2〜5 の呼び出しと
一致している。`pageSize` は Task 3 が定義し Task 4・5 が使う（Task 3 より先に
Task 4・5 に着手する場合は `pageSize` の定義を持って行くこと）。`fixture(t, name)`
ヘルパーは Task 3 の `lists_test.go` が定義し、Task 4・5 のテストが使う。
`serveREST` は Task 1 の `rest_test.go` が定義する。

**4. 依存の順序:** Task 1 → 2・3 → 4・5 → 6。Task 4 の Step 1〜3（`cli` からの
移動）は他のどのタスクにも依存しないので、先に単独で出しても構わない。

---

## 実行の選択

**1. Subagent-Driven（推奨）** — タスクごとに新しい subagent を出し、タスク間で
レビューする。4-2 と同じ形。

**2. Inline Execution** — このセッションで順に実行し、区切りで確認する。
