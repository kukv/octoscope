# Phase 4 スライス 2-1: Repos サイドバーのバックエンド 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** サイドバーが要る 3 つのものを、画面を変えずに用意する — 設定ファイルの `repositories`、リポジトリを名指しできる一覧取得、複数リポジトリの件数を 1 リクエストで引く口。

**Architecture:** 既存の形をなぞる。設定は `internal/config` に読むだけを足し、`internal/gh/cli` に GraphQL 文書 1 本と `Client` のメソッド 1 つを足し、`internal/usecase` がそれを通す。**このスライスで画面は変わらない**（`internal/tui` の変更は、シグネチャが変わった呼び出しの追従だけ）。

**Tech Stack:** Go 1.25、`go.yaml.in/yaml/v3`、`gh api graphql`

**Spec:** `docs/superpowers/specs/2026-09-08-phase4-design.md`（§2 の実測値と §4「`internal/gh` 側の変更」）。画面は `docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md` §4.2 が正。

**スライス 2 は 3 本に割った**（2-1 バックエンド / 2-2 サイドバー本体 / 2-3 追加ダイアログ・削除・初回投入）。設計 §10 は 2 を 1 本と書いているが、数えたら 1 本に収まらなかった。Phase 2 を 1 本に詰め込んで立て直しに 2 本を要した反省による。

## Global Constraints

- **依存の向き**（`.claude/rules/architecture.md`）: `internal/tui` は `internal/gh/cli` も `internal/config` も import しない。`internal/gh` は interface を export しない。**利用側が自分の使う分だけ interface を宣言する**（1 宣言に直接並べるメソッドは 6 個まで）
- **GitHub API 固有の文字列を層の外に出さない。** `internal/gh` のドメイン型に直して返す
- **外部プロセスに渡す引数は内部で組み立てた値だけ。** 利用者由来の文字列（`owner/name`）はクエリ本文に埋め込まず、**GraphQL の変数として渡す**
- **テスト**（`.claude/rules/testing.md`）: ネットワークもサブプロセスも叩かない。パースのテストの入力は**実際に録ったレスポンス**を使い、録り方と録った日を `internal/gh/cli/testdata/README.md` に書く。**実装が組み立てた引数をコピーした期待値を書かない。** 書いた直後に検証対象を壊して落ちることを確かめる
- **コメント**（`.claude/rules/go-style.md`）: 基本は書かない。書くのは外部の事情・一見おかしいコードが正しい理由・エクスポートした識別子の doc の 3 つだけ。**実装計画や設計書への参照をコードに書かない**。コメントは英語
- **`make check` が緑でないコミットを作らない**

## File Structure

| ファイル | 責務 |
|---|---|
| `internal/config/config.go`（変更） | `Config.Repositories` を足す |
| `internal/config/config_test.go`（変更） | `Config` が slice を持つと `==` で比較できない。比較を直す |
| `internal/gh/gh.go`（変更） | `RepoCount` ドメイン型 |
| `internal/gh/cli/cli.go`（変更） | `ListPRs` / `ListIssues` が `repo` を取る。`runGh` が終了コード非 0 でも stdout を返す |
| `internal/gh/cli/repo_counts.go`（新規） | 件数クエリの組み立て・実行・パース |
| `internal/gh/cli/repo_counts.graphql`（新規） | 1 リポジトリ分の選択（組み立ての材料） |
| `internal/gh/cli/repo_counts_test.go`（新規） | 組み立てた文書・変数・パース・部分的な失敗 |
| `internal/gh/cli/schema_test.go`（変更） | 組み立てた文書もスキーマに当てる |
| `internal/gh/cli/testdata/repo_counts.json` ほか（新規） | 実際に録った応答 |
| `internal/usecase/usecase.go`（変更） | `ListPRs` / `ListIssues` の引数、`RepoCounts` を通す |
| `internal/tui/repo/repo.go`（変更） | 宣言している interface と呼び出しの追従 |

---

### Task 1: 設定に `repositories` を足す

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

**Interfaces:**
- Consumes: なし
- Produces: `config.Config.Repositories []string`（YAML の `repositories`）

- [ ] **Step 1: 失敗するテストを書く**

`internal/config/config_test.go` に足す:

```go
// The Repos tab's list is the reason the settings file exists; it must
// survive a round trip through the parser in the order the user wrote it.
func TestLoadReadsTheRepositoryList(t *testing.T) {
	t.Parallel()

	path := write(t, "repositories:\n  - kukv/octoscope\n  - cli/cli\n")

	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []string{"kukv/octoscope", "cli/cli"}
	if !slices.Equal(got.Repositories, want) {
		t.Errorf("Repositories = %v, want %v", got.Repositories, want)
	}
}

// A settings file with no list at all is the common case on a first run.
func TestLoadLeavesTheRepositoryListEmptyWhenTheFileHasNone(t *testing.T) {
	t.Parallel()

	path := write(t, "language: ja\n")

	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Repositories) != 0 {
		t.Errorf("Repositories = %v, want none", got.Repositories)
	}
}
```

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/config/`
Expected: FAIL（`Repositories` が無い、コンパイルエラー）

- [ ] **Step 3: 実装する**

`internal/config/config.go` の `Config` に足す:

```go
	// Repositories is the list the Repos tab shows, in the order it shows
	// them.
	Repositories []string `yaml:"repositories"`
```

**`Config` が slice を持つと `==` で比較できなくなる。** 既存のテストが
`got != (config.Config{})` の形で書かれているので、そこを直す。ゼロ値かどうかを
見たいだけなので、確かめたいフィールドを個別に見るか `reflect.DeepEqual` に
変える。**どちらにするかは既存のテストの読みやすさで決めてよい**が、
「何を確かめているか」がテスト名と食い違わないようにすること。

- [ ] **Step 4: 通ることを確かめる**

Run: `go test ./internal/config/`
Expected: PASS

- [ ] **Step 5: 空振りしていないことを確かめる**

`yaml:"repositories"` のタグを一時的に `yaml:"-"` にして
`TestLoadReadsTheRepositoryList` が落ちることを見る。戻す。

- [ ] **Step 6: 検査してコミット**

```bash
make check
git add internal/config
git commit -m "feat: read the repository list from the settings file"
```

---

### Task 2: 一覧取得がリポジトリを名指しできるようにする

今の `ListPRs` / `ListIssues` はクライアントが握っている 1 つのリポジトリしか見ない
（`c.repo` を直接読む）。サイドバーは選択されたリポジトリを引くので、`GetPR` と
同じく第 2 引数に `repo` を取る形に変える。**空文字はこれまでどおりクライアントの
リポジトリに落ちる**（`effectiveRepo`）ので、既存の呼び出しの意味は変わらない。

**Files:**
- Modify: `internal/gh/cli/cli.go`（`ListPRs`、`ListIssues`）
- Modify: `internal/gh/cli/cli_test.go`
- Modify: `internal/usecase/usecase.go`（`lister` interface と 2 つのメソッド）
- Modify: `internal/tui/repo/repo.go`（`prSource` / `issueSource` の宣言と `fetchList`）
- Modify: `internal/tui/repo/repo_test.go`（fake の追従）

**Interfaces:**
- Consumes: なし
- Produces:
  - `func (c *Client) ListPRs(ctx context.Context, repo string) ([]gh.PR, error)`
  - `func (c *Client) ListIssues(ctx context.Context, repo string) ([]gh.Issue, error)`
  - `func (u *Usecase) ListPRs(ctx context.Context, repo string) ([]gh.PR, error)`（`ListIssues` も同型）

- [ ] **Step 1: 失敗するテストを書く**

`internal/gh/cli/cli_test.go` に足す。**実装が組み立てた引数をコピーしない**:
確かめたいのは「名指ししたリポジトリが `--repo` として渡ること」と
「空文字ならクライアントのリポジトリに落ちること」の 2 つ。

```go
// The sidebar lists a repository the client was not built for, so the call
// has to be able to name one.
func TestListPRsAsksForTheRepositoryItWasGiven(t *testing.T) {
	t.Parallel()

	c := New("/tmp", "kukv/octoscope")
	var got []string
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return []byte("[]"), nil
	}

	if _, err := c.ListPRs(context.Background(), "cli/cli"); err != nil {
		t.Fatalf("ListPRs: %v", err)
	}
	if repoArg(got) != "cli/cli" {
		t.Errorf("--repo = %q, want %q", repoArg(got), "cli/cli")
	}
}

// An empty name still means "the repository this client was built for":
// every existing caller passes nothing.
func TestListPRsFallsBackToTheClientsRepository(t *testing.T) {
	t.Parallel()

	c := New("/tmp", "kukv/octoscope")
	var got []string
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return []byte("[]"), nil
	}

	if _, err := c.ListPRs(context.Background(), ""); err != nil {
		t.Fatalf("ListPRs: %v", err)
	}
	if repoArg(got) != "kukv/octoscope" {
		t.Errorf("--repo = %q, want %q", repoArg(got), "kukv/octoscope")
	}
}

// repoArg is the value gh was given for --repo, or "" if it was not given.
func repoArg(args []string) string {
	for i, a := range args {
		if a == "--repo" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}
```

`ListIssues` にも同じ 2 本を書く（テーブルにまとめてよい。まとめるなら
ケース名に PR / Issue が出るようにする）。

**`repoArg` と同じ役割のヘルパーが `cli_test.go` に既にあるなら、それを使う。**
無いことを確かめてから足すこと。

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/gh/cli/ -run TestList`
Expected: FAIL（引数の数が合わない、コンパイルエラー）

- [ ] **Step 3: 実装する**

```go
func (c *Client) ListPRs(ctx context.Context, repo string) ([]gh.PR, error) {
	args := appendRepo([]string{"pr", "list", "--json", prListFields, "--limit", listLimit}, c.effectiveRepo(repo))
	...
}
```

`ListIssues` も同じ形。**それ以外は変えない。**

- [ ] **Step 4: 呼び出し側を追従させる**

1. `internal/usecase/usecase.go` の `lister` interface と `ListPRs` / `ListIssues`
   に `repo string` を通す
2. `internal/tui/repo/repo.go` の `prSource` / `issueSource` の宣言と `fetchList`
   を追従させる。**この段階では常に空文字を渡す**（今の Repos ビューはクライアントの
   リポジトリを見る画面のままで、選ぶのはサイドバーが入る次の計画）
3. テストの fake を追従させる

- [ ] **Step 5: 通ることを確かめる**

Run: `make test`
Expected: PASS（既存のテストが 1 本も落ちないこと。落ちたら、それは意味を変えた印）

- [ ] **Step 6: 空振りしていないことを確かめる**

`effectiveRepo(repo)` を `c.repo` に一時的に戻して
`TestListPRsAsksForTheRepositoryItWasGiven` が落ちることを見る。戻す。

- [ ] **Step 7: 検査してコミット**

```bash
make check
git add internal/gh internal/usecase internal/tui/repo
git commit -m "feat: let a list call name its repository"
```

---

### Task 3: GraphQL の部分的な応答を捨てない

`gh api graphql` は、応答に `errors` があると**終了コード 1** を返す。`runGh` は
そのとき stdout を捨てるので、**消えたリポジトリが 1 つ混じるだけで他のリポジトリの
件数も全部失われる**（設計 §2 の実測）。

実測した応答（2026-09-08）:

```json
{"data":{"r0":{"nameWithOwner":"kukv/octoscope","pullRequests":{"totalCount":0},
"issues":{"totalCount":2}},"r1":null},
"errors":[{"type":"NOT_FOUND","path":["r1"],
"message":"Could not resolve to a Repository with the name 'kukv/no-such-repository-xyz'."}]}
```

**`runGh` 自体にはテストを書かない。** `gh` を実際に起動する関数であり、
`.claude/rules/testing.md` の「ネットワークも外部プロセスも実際には叩かない。例外なし」に
当たる。既存のテストも例外なく `run` フィールドを差し替えて `runGh` を迂回している。
**この振る舞いが固定されるのは Task 4 の
`TestRepoCountsKeepsTheAnswersItGotWhenOneRepositoryIsGone`** で、そこでは fake の
`run` が本文とエラーの両方を返し、`RepoCounts` がその本文を読むことを確かめる。
この Task が変えるのは、**その約束を `runGh` 側でも実際に満たすようにする**ことである。

**Files:**
- Modify: `internal/gh/cli/cli.go`（`runGh` と `ListWork` のコメント）

**Interfaces:**
- Consumes: なし
- Produces: `runGh` が失敗時にも stdout を返す（`runFunc` の型は変わらない）

- [ ] **Step 1: 実装する**

`runGh` の失敗の戻り値を、捨てずに返す形に変える:

```go
	if err := cmd.Run(); err != nil {
		// gh api graphql exits non-zero when the body carries a top-level
		// "errors" array, and that body still holds the data GitHub could
		// answer for. Hand both back and let the caller decide.
		if msg := bytes.TrimSpace(stderr.Bytes()); len(msg) > 0 {
			return stdout.Bytes(), fmt.Errorf("gh %s: %s", args[0], msg)
		}
		return stdout.Bytes(), fmt.Errorf("gh %s: %w", args[0], err)
	}
```

**既存の呼び出しは全部 `err != nil` を先に見て戻るので、意味は変わらない。**

- [ ] **Step 2: 古くなるコメントを直す**

`ListWork` の「gh api graphql exits non-zero when the response body carries a
top-level "errors" array, so a query GitHub rejects arrives here as an error from
c.run rather than as a body we'd otherwise parse into empty columns」は、この変更で
**半分だけ正しくなくなる**（本文も返るようになる）。`ListWork` がその本文を読まずに
エラーで戻るのは今も正しい選択なので、**その理由が読める文に直す。**

- [ ] **Step 3: 既存のテストが落ちないことを確かめる**

Run: `make test`
Expected: PASS。**1 本でも落ちたら「意味は変わらない」という前提が外れた印。**
落ちたテストを読み、どの呼び出しが stdout を先に見ていたかを報告に書く。

- [ ] **Step 4: 検査してコミット**

```bash
make check
git add internal/gh/cli
git commit -m "fix: keep the body gh printed when it exits non-zero"
```

---

### Task 4: 複数リポジトリの件数を 1 リクエストで引く

設計 §2 の実測: `repository` を alias で並べ、各々 `pullRequests(states: OPEN) { totalCount }` と
`issues(states: OPEN) { totalCount }` を選ぶ。3 リポジトリで 1 リクエスト。
`search(type: ISSUE, query: "repo:a/b repo:c/d")` の 1 発は `issueCount` が
**クエリ全体の合計**にしかならないので採らない。

**リポジトリ名をクエリ本文に埋め込まない。** alias（`r0`、`r1`、…）だけを Go で
組み立て、`owner` と `name` は GraphQL の変数として渡す。これで利用者が設定ファイルに
書いた文字列がクエリ本文に一切入らない。

組み立てた文書（2 リポジトリの例）:

```graphql
query ($o0: String!, $n0: String!, $o1: String!, $n1: String!) {
  r0: repository(owner: $o0, name: $n0) { nameWithOwner pullRequests(states: OPEN) { totalCount } issues(states: OPEN) { totalCount } }
  r1: repository(owner: $o1, name: $n1) { nameWithOwner pullRequests(states: OPEN) { totalCount } issues(states: OPEN) { totalCount } }
}
```

**Files:**
- Create: `internal/gh/cli/repo_counts.go`、`internal/gh/cli/repo_counts.graphql`、`internal/gh/cli/repo_counts_test.go`
- Create: `internal/gh/cli/testdata/repo_counts.json`、`internal/gh/cli/testdata/repo_counts_partial.json`
- Modify: `internal/gh/gh.go`（`RepoCount`）
- Modify: `internal/gh/cli/schema_test.go`（組み立てた文書を検証に足す）
- Modify: `internal/gh/cli/testdata/README.md`（録り方と録った日）
- Modify: `internal/usecase/usecase.go`（`lister` に 1 つ足す）

**Interfaces:**
- Consumes: Task 3 の「エラーと一緒に stdout が返る」
- Produces:
  - `type gh.RepoCount struct { Repo string; PRs, Issues int; Unavailable bool }`
  - `func (c *Client) RepoCounts(ctx context.Context, repos []string) ([]gh.RepoCount, error)`
  - `func (u *Usecase) RepoCounts(ctx context.Context, repos []string) ([]gh.RepoCount, error)`

- [ ] **Step 1: 応答を実際に録る**

```bash
D=internal/gh/cli/testdata
cat > /tmp/counts.graphql <<'EOF'
query ($o0: String!, $n0: String!, $o1: String!, $n1: String!) {
  r0: repository(owner: $o0, name: $n0) { nameWithOwner pullRequests(states: OPEN) { totalCount } issues(states: OPEN) { totalCount } }
  r1: repository(owner: $o1, name: $n1) { nameWithOwner pullRequests(states: OPEN) { totalCount } issues(states: OPEN) { totalCount } }
}
EOF
gh api graphql -F query=@/tmp/counts.graphql \
  -f o0=kukv -f n0=octoscope -f o1=cli -f n1=cli | jq . > $D/repo_counts.json

gh api graphql -F query=@/tmp/counts.graphql \
  -f o0=kukv -f n0=octoscope -f o1=kukv -f n1=no-such-repository-xyz \
  | jq . > $D/repo_counts_partial.json   # 終了コード 1。JSON は出る
```

2 本目は終了コード 1 で終わるので、`jq` に渡す前に落ちないよう気をつける
（`gh ... > /tmp/out.json; jq . /tmp/out.json > $D/...` と分ければよい）。
`testdata/README.md` に、この 2 本の録り方・録った日・対象リポジトリを書く。

- [ ] **Step 2: 失敗するテストを書く**

`internal/gh/cli/repo_counts_test.go`:

```go
// The document must not carry a repository name: the names come from the
// settings file, and only GraphQL variables keep them out of the query text.
func TestRepoCountsPassesTheNamesAsVariables(t *testing.T) {
	t.Parallel()

	c := New("/tmp", "")
	var got []string
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return os.ReadFile("testdata/repo_counts.json")
	}

	if _, err := c.RepoCounts(context.Background(), []string{"kukv/octoscope", "cli/cli"}); err != nil {
		t.Fatalf("RepoCounts: %v", err)
	}
	query := flagValue(got, "query")
	for _, name := range []string{"kukv", "octoscope", "cli"} {
		if strings.Contains(query, name) {
			t.Errorf("the query text carries %q; it must travel as a variable", name)
		}
	}
	if flagValue(got, "o0") != "kukv" || flagValue(got, "n0") != "octoscope" {
		t.Errorf("first repository was not passed as variables: %v", got)
	}
}

// The badge is per repository, so the counts have to come back split by
// repository rather than summed.
func TestRepoCountsReadsEachRepositorysOwnCounts(t *testing.T) {
	t.Parallel()

	c := New("/tmp", "")
	c.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return os.ReadFile("testdata/repo_counts.json")
	}

	got, err := c.RepoCounts(context.Background(), []string{"kukv/octoscope", "cli/cli"})
	if err != nil {
		t.Fatalf("RepoCounts: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d counts, want 2", len(got))
	}
	if got[0].Repo != "kukv/octoscope" || got[1].Repo != "cli/cli" {
		t.Errorf("counts came back in another order: %+v", got)
	}
	if got[0].PRs == got[1].PRs && got[0].Issues == got[1].Issues {
		t.Errorf("both repositories got the same counts: %+v", got)
	}
}

// One repository the user renamed or lost access to must not cost the badges
// of every other row.
func TestRepoCountsKeepsTheAnswersItGotWhenOneRepositoryIsGone(t *testing.T) {
	t.Parallel()

	c := New("/tmp", "")
	c.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		raw, err := os.ReadFile("testdata/repo_counts_partial.json")
		if err != nil {
			return nil, err
		}
		return raw, errors.New("gh api: Could not resolve to a Repository")
	}

	got, err := c.RepoCounts(context.Background(), []string{"kukv/octoscope", "kukv/no-such-repository-xyz"})
	if err != nil {
		t.Fatalf("RepoCounts: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d counts, want 2", len(got))
	}
	if got[0].Unavailable {
		t.Errorf("the repository that answered was marked unavailable: %+v", got[0])
	}
	if !got[1].Unavailable {
		t.Errorf("the repository that could not be resolved was not marked: %+v", got[1])
	}
}

// A line the user typed by hand can be anything; it must not become a
// request.
func TestRepoCountsDoesNotAskAboutAMalformedName(t *testing.T) {
	t.Parallel()

	c := New("/tmp", "")
	called := false
	c.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		called = true
		return []byte(`{"data":{}}`), nil
	}

	got, err := c.RepoCounts(context.Background(), []string{"not-a-repository"})
	if err != nil {
		t.Fatalf("RepoCounts: %v", err)
	}
	if called {
		t.Error("a malformed name was sent to GitHub")
	}
	if len(got) != 1 || !got[0].Unavailable {
		t.Errorf("got %+v, want one unavailable row", got)
	}
}
```

`flagValue(args []string, name string) string` は `-f name=value` / `-F name=value`
の値を取り出すヘルパー。`repo_counts_test.go` に書く。

**すべてのリポジトリ名が不正で、送るものが 1 つも無いときにリクエストを出さないこと**
も同じ形で確かめられる（上の 4 本目がその 1 リポジトリ版）。

- [ ] **Step 3: 落ちることを確かめる**

Run: `go test ./internal/gh/cli/ -run TestRepoCounts`
Expected: FAIL（`RepoCounts` が無い）

- [ ] **Step 4: 実装する**

`internal/gh/gh.go`:

```go
// RepoCount is how much is open in one repository: the badge the Repos
// sidebar puts beside its name. Unavailable says GitHub did not answer for
// this one -- it was renamed, deleted, or is no longer visible -- which is
// not the same as a repository with nothing open in it.
type RepoCount struct {
	Repo        string
	PRs, Issues int
	Unavailable bool
}
```

`internal/gh/cli/repo_counts.graphql`（**1 リポジトリ分の選択だけ**を持つ。
alias と変数名は Go が付ける）:

```graphql
repository(owner: $OWNER, name: $NAME) {
  nameWithOwner
  pullRequests(states: OPEN) {
    totalCount
  }
  issues(states: OPEN) {
    totalCount
  }
}
```

`internal/gh/cli/repo_counts.go`:

```go
package cli

import (
	_ "embed"
	"strings"
)

//go:embed repo_counts.graphql
var repoCountSelection string

// buildRepoCountsQuery writes one aliased repository selection per name.
// The names themselves never enter the document: they travel as the
// variables this declares, so a name from the settings file cannot become
// part of the query.
func buildRepoCountsQuery(n int) string {
	var decls, body strings.Builder
	for i := range n {
		if i > 0 {
			decls.WriteString(", ")
		}
		fmt.Fprintf(&decls, "$o%d: String!, $n%d: String!", i, i)
		selection := strings.NewReplacer(
			"$OWNER", fmt.Sprintf("$o%d", i),
			"$NAME", fmt.Sprintf("$n%d", i),
		).Replace(repoCountSelection)
		fmt.Fprintf(&body, "  r%d: %s", i, strings.TrimSpace(selection)+"\n")
	}
	return "query (" + decls.String() + ") {\n" + body.String() + "}\n"
}
```

`$OWNER` / `$NAME` は `repo_counts.graphql` の中の差し替え点であり、**リポジトリ名では
ない**（名前は `-f o0=…` として `gh` に渡る）。この置換で入るのは `$o0` のような
変数名だけなので、利用者の文字列は文書に一切入らない。

`RepoCounts` の流れ:

1. 名前を `owner` と `name` に割る。割れないものは `Unavailable` にして送らない
2. 送るものが 0 件なら、リクエストを出さずに戻る
3. `buildRepoCountsQuery` で文書を組み、`-f query=…` と `-f o<i>=…` `-f n<i>=…` を渡す
4. 応答を `{ "data": {alias: {...}}, "errors": [...] }` として読む。
   **エラーがあっても `data` を読む**（Task 3）。alias が `null`、あるいは
   `data` に無い場合は `Unavailable`
5. **戻す順は渡された順**。呼び出し側（サイドバー）は行の順で描く

**`gh` が変数をどう受け取るかは実測で確かめること。** `-f` は文字列、`-F` は
型付き（数値・真偽値・`@file`）。Step 1 で録るときに使った形が正しい。

- [ ] **Step 5: スキーマ検証に足す**

`internal/gh/cli/schema_test.go` の `docs` マップに、**組み立てた文書**を足す:

```go
		"repo_counts (built for two)": buildRepoCountsQuery(2),
```

これで、`repository` の下のフィールド名を間違えたときにテストが落ちる。

- [ ] **Step 6: usecase を通す**

`lister` interface に `RepoCounts(ctx context.Context, repos []string) ([]gh.RepoCount, error)`
を足し、`Usecase` に同名のメソッドを足す。**`lister` は今 6 メソッドあり、これで 7 に
なる。** `.claude/rules/architecture.md` の「1 つの interface 宣言に直接並べる
メソッドは 6 個まで」に触れるので、**`lister` を意味のある単位に割ること**
（例: リポジトリ 1 つの中身を引くものと、リポジトリをまたぐものを分ける）。
割り方は自分で決めてよいが、**なぜその線で割ったかを報告に書く。**

- [ ] **Step 7: 通ることを確かめる**

Run: `go test ./internal/gh/... ./internal/usecase/`
Expected: PASS

- [ ] **Step 8: 空振りしていないことを確かめる**

次の 2 つを別々に確かめる。

1. `buildRepoCountsQuery` を、変数ではなく名前を直接埋め込む形に一時的に変えて
   `TestRepoCountsPassesTheNamesAsVariables` が落ちることを見る
2. 部分的な失敗の分岐（エラーがあっても `data` を読む）を一時的に「エラーなら即戻る」に
   変えて `TestRepoCountsKeepsTheAnswersItGotWhenOneRepositoryIsGone` が落ちることを見る

どちらも確かめたら戻す。

- [ ] **Step 9: 検査してコミット**

```bash
make check
git add internal/gh internal/usecase
git commit -m "feat: count what is open in several repositories at once"
```

---

### Task 5: 積み残しを書き残す

スライス 1 のレビューで見つかったが直さなかったものが 1 つある。**このスライスでも
直さない**（タブ行の桁の話で、サイドバーのバックエンドとは別の作業）が、
PR の本文にしか無い状態を解消する。

**Files:**
- Create: `docs/superpowers/2026-09-08-phase4-config-followups.md`

- [ ] **Step 1: 書く**

Phase 3 の積み残し文書（`docs/superpowers/2026-09-08-phase3-checks-followups.md`）に
倣い、**直さないと決めた理由も書く**。内容:

- **ja の 80 桁で、リポジトリ判定のタイムアウト警告と設定の警告が同時に立つと
  タブ行が 89 桁になり折り返す。** 文言を縮めた後も残る。稀な組み合わせ
  （設定が壊れている、かつリポジトリ判定が 20 秒で返らない）であり、描画側で
  切ると「どちらの警告か分からない」という別の問題を招く。golden も録っていない
- **設定ファイルの実端末での確認は 2026-09-08 に利用者が実施済み**（日本語表示・
  ASCII グリフ・Repos タブ起動・壊れた設定の警告）。TTY の無い環境では代行できない
  ため、以後のスライスでも同じ受け渡しが要る

- [ ] **Step 2: コミット**

```bash
git add docs/superpowers
git commit -m "docs: record what the settings file slice left behind"
```

---

## 完了条件

1. `config.yaml` の `repositories` が読める
2. `ListPRs` / `ListIssues` がリポジトリを名指しでき、空文字ならこれまでどおり
   クライアントのリポジトリに落ちる
3. `gh` が終了コード 1 で終わっても、その応答本文が呼び出し側に届く
4. `RepoCounts` が複数リポジトリの件数を 1 リクエストで引き、**1 つが消えていても
   残りの件数を返す**
5. リポジトリ名がクエリ本文に入らない（変数として渡る）
6. 組み立てた文書が `schema_test.go` の検証を通る
7. `internal/tui` が `internal/gh/cli` も `internal/config` も import していない
8. 画面は変わっていない（golden が 1 枚も変わらない）
9. `make check` が緑
