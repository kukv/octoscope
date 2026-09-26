# gh があっても API を直接叩けるようにする 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `--api` / `OCTOSCOPE_API` / 設定ファイルの `api: true` のいずれかで、`gh` が入っていてもトークンで GitHub API を直接叩く経路（api バックエンド）を選べるようにする。

**Architecture:** 経路の選択は今も `cmd/octoscope/backend.go` の `chooseBackend` にある。そこに `useAPI bool` を足し、
真なら `gh` を探さずに api 経路を組む。フラグ・環境変数・設定ファイルから `useAPI` を決める純粋関数 `resolveAPI` を
同じファイルに置き、`main` が 3 つの入力を集めて渡す。設定ファイルには `api` キーを 1 つ足す。

**Tech Stack:** Go / 標準 `flag` / `strconv.ParseBool` / `go.yaml.in/yaml/v3` / go-i18n

**Spec:** `docs/superpowers/specs/2026-09-26-api-switch-design.md`

## Global Constraints

- 何も指定しなければ今と同じ動作（`gh` が `PATH` にあれば gh 経路、無ければトークン）。
- 優先順位は `--api`（`--api=false` を含む）→ `OCTOSCOPE_API` → 設定ファイル `api`。
- `OCTOSCOPE_API` が `strconv.ParseBool` で読めなければ起動を止める。黙って既定値に落とさない。
- api 経路が指定されたときは `lookPath("gh")` を呼ばない。
- `auto` / `gh` という値、gh 経路の強制、画面上の経路表示は作らない。
- トークンは今と同じく `GH_TOKEN` → `GITHUB_TOKEN`（`api.Token`）。
- 画面に出す文字列は `internal/i18n` のカタログ（en / ja 両方）から引く。
- コミット前に `make check` が通ること。

---

## ファイル構成

| ファイル | 役割 | 変更 |
|---|---|---|
| `internal/app/config/config.go` | 設定ファイルの形 | `API bool` を足す |
| `internal/app/config/config_test.go` | 〃 のテスト | `api: true` を読むケースを足す |
| `cmd/octoscope/backend.go` | 経路の選択 | `resolveAPI` を足し、`chooseBackend` に `useAPI` を足す |
| `cmd/octoscope/backend_test.go` | 〃 のテスト | `resolveAPI` の表テストと、`useAPI` の 2 テストを足す |
| `cmd/octoscope/main.go` | 起動 | `--api` フラグ、`resolveAPI` 呼び出し、失敗時メッセージの出し分け |
| `internal/i18n/locales/active.{en,ja}.yaml` | 文言 | `error.no_token` / `error.invalid_api` |
| `README.md` / `README.ja.md` | 利用者向け文書 | 必要なもの・フラグ表・設定表・「認証」節 |
| `docs/superpowers/specs/2026-09-08-phase4-design.md` | 既存設計 | 「認証」段落に上書き手段を書き足す |

---

### Task 1: 設定ファイルに `api` を足す

**Files:**
- Modify: `internal/app/config/config.go`（`Config` 構造体）
- Test: `internal/app/config/config_test.go`

**Interfaces:**
- Produces: `config.Config.API bool`（yaml キー `api`）

- [ ] **Step 1: 失敗するテストを書く**

`TestLoadReadsEveryField` の入力と期待値に `api` を足す（「全フィールドを読む」テストなので、フィールドが増えたらここに入れる）。

```go
func TestLoadReadsEveryField(t *testing.T) {
	t.Parallel()

	path := write(t, "language: ja\nicons: nerd\ndefault_tab: repos\napi: true\n")

	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := config.Config{Language: "ja", Icons: "nerd", DefaultTab: "repos", API: true}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Load = %+v, want %+v", got, want)
	}
}
```

- [ ] **Step 2: 失敗を確かめる**

Run: `go test ./internal/app/config/ -run TestLoadReadsEveryField`
Expected: コンパイルエラー `unknown field API in struct literal`

- [ ] **Step 3: 実装する**

`Config` の `DefaultTab` の直後に足す。

```go
	DefaultTab string `yaml:"default_tab,omitempty"`

	// API takes the API backend even when gh is installed. It is the last
	// place asked, after --api and OCTOSCOPE_API, so false and unset need not
	// be told apart.
	API bool `yaml:"api,omitempty"`
```

- [ ] **Step 4: 通ることを確かめる**

Run: `go test ./internal/app/config/`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add internal/app/config/config.go internal/app/config/config_test.go
git commit -m "feat(config): read api from the settings file"
```

---

### Task 2: `resolveAPI` でどこの指定を採るか決める

**Files:**
- Modify: `cmd/octoscope/backend.go`
- Test: `cmd/octoscope/backend_test.go`

**Interfaces:**
- Produces:
  - `const apiEnvVar = "OCTOSCOPE_API"`
  - `func resolveAPI(flagSet, flagValue bool, env string, configured bool) (bool, error)`

- [ ] **Step 1: 失敗するテストを書く**

`backend_test.go` の末尾に足す。

```go
// --api, then OCTOSCOPE_API, then the settings file. A false written in a
// higher place is an answer, not an absence: it is how a user who set api in
// the settings file gets gh back for one run.
func TestResolveAPIOrder(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		flagSet    bool
		flagValue  bool
		env        string
		configured bool
		want       bool
	}{
		{"nothing set keeps today's choice", false, false, "", false, false},
		{"the settings file on its own", false, false, "", true, true},
		{"the environment on its own", false, false, "1", false, true},
		{"the flag on its own", true, true, "", false, true},
		{"--api=false beats the environment and the settings file", true, false, "true", true, false},
		{"OCTOSCOPE_API=0 beats the settings file", false, false, "0", true, false},
		{"the flag beats OCTOSCOPE_API=0", true, true, "0", false, true},
		{"a blank environment variable reads as unset", false, false, "  ", true, true},
		{"case and space are not a typo", false, false, " TRUE ", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolveAPI(tc.flagSet, tc.flagValue, tc.env, tc.configured)
			if err != nil {
				t.Fatalf("resolveAPI: %v", err)
			}
			if got != tc.want {
				t.Errorf("resolveAPI(%v, %v, %q, %v) = %v, want %v",
					tc.flagSet, tc.flagValue, tc.env, tc.configured, got, tc.want)
			}
		})
	}
}

// Nothing on screen says which backend is running, so a typo that quietly fell
// through to the settings file or to gh would never be noticed. It stops the
// start instead.
func TestAnUnreadableOCTOSCOPEAPIIsAnError(t *testing.T) {
	t.Parallel()

	if _, err := resolveAPI(false, false, "yes", true); err == nil {
		t.Error("resolveAPI accepted OCTOSCOPE_API=yes")
	}
}
```

- [ ] **Step 2: 失敗を確かめる**

Run: `go test ./cmd/octoscope/ -run 'TestResolveAPIOrder|TestAnUnreadableOCTOSCOPEAPIIsAnError'`
Expected: コンパイルエラー `undefined: resolveAPI`

- [ ] **Step 3: 実装する**

`backend.go` の import に `"strconv"` と `"strings"` を足し、`chooseBackend` の前に置く。

```go
// apiEnvVar names the environment variable that takes the API backend,
// between --api and the settings file.
const apiEnvVar = "OCTOSCOPE_API"

// resolveAPI decides whether to take the API backend: --api when it was given
// at all, then OCTOSCOPE_API when it is set, then the settings file.
//
// flagSet says whether --api was on the command line, so that --api=false can
// overrule the two below it. env is OCTOSCOPE_API's value, read by the caller
// so a test need not touch the environment.
//
// Only env can be unreadable here: the flag package and the YAML decoder
// check the other two. Unlike --icons, an unreadable value is an error rather
// than a fall back, because nothing on screen tells the two backends apart.
func resolveAPI(flagSet, flagValue bool, env string, configured bool) (bool, error) {
	if flagSet {
		return flagValue, nil
	}
	if v := strings.TrimSpace(env); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return false, fmt.Errorf("%s: %w", apiEnvVar, err)
		}
		return b, nil
	}
	return configured, nil
}
```

- [ ] **Step 4: 通ることを確かめる**

Run: `go test ./cmd/octoscope/`
Expected: PASS（`" TRUE "` は `strconv.ParseBool` が `TRUE` を受け付けるので通る）

- [ ] **Step 5: コミット**

```bash
git add cmd/octoscope/backend.go cmd/octoscope/backend_test.go
git commit -m "feat: decide between --api, OCTOSCOPE_API and the settings file"
```

---

### Task 3: `useAPI` で gh を飛ばし、`main` から配線する

`chooseBackend` の引数が変わると `main.go` がコンパイルできなくなるので、配線と文言まで 1 タスクで行う。

**Files:**
- Modify: `cmd/octoscope/backend.go`（`chooseBackend`）
- Modify: `cmd/octoscope/main.go`
- Modify: `internal/i18n/locales/active.en.yaml` / `active.ja.yaml`（`error:` の下、`no_backend` の直後）
- Test: `cmd/octoscope/backend_test.go`

**Interfaces:**
- Consumes: `resolveAPI`, `apiEnvVar`（Task 2）、`config.Config.API`（Task 1）
- Produces: `func chooseBackend(dir, repo string, useAPI bool, lookPath func(string) (string, error), token func() (string, error)) (*cli.Client, *api.Client, error)`

- [ ] **Step 1: 失敗するテストを書く**

既存の 4 テストの `chooseBackend("/work", "kukv/octoscope",` を `chooseBackend("/work", "kukv/octoscope", false,` に変える（既存の動作が `useAPI=false` で保たれることの確認になる）。そのうえで末尾に足す。

```go
// --api exists for a machine where gh is installed but should not be used, so
// gh is not even looked for.
func TestUseAPISkipsGhEvenWhenItIsThere(t *testing.T) {
	t.Parallel()

	c, a, err := chooseBackend("/work", "kukv/octoscope", true,
		func(string) (string, error) {
			t.Error("looked for gh although the API backend was asked for")
			return "/usr/bin/gh", nil
		},
		func() (string, error) { return "a-token", nil })
	if err != nil {
		t.Fatalf("chooseBackend: %v", err)
	}
	if a == nil || c != nil {
		t.Errorf("did not choose the API backend when asked for it")
	}
}

// Asking for the API backend without a token has no gh to fall back to: it is
// the same authentication failure as having neither.
func TestUseAPIWithoutATokenIsAnAuthenticationFailure(t *testing.T) {
	t.Parallel()

	_, _, err := chooseBackend("/work", "kukv/octoscope", true,
		func(string) (string, error) { return "/usr/bin/gh", nil },
		func() (string, error) { return "", domain.ErrUnauthenticated })
	if !errors.Is(err, domain.ErrUnauthenticated) {
		t.Errorf("err = %v, want domain.ErrUnauthenticated", err)
	}
}
```

- [ ] **Step 2: 失敗を確かめる**

Run: `go test ./cmd/octoscope/`
Expected: コンパイルエラー `too many arguments in call to chooseBackend`

- [ ] **Step 3: `chooseBackend` を変える**

関数全体を次に置き換える。

```go
// chooseBackend picks which client talks to GitHub. gh comes first: it is
// already signed in, and it carries a login the environment variables need not
// have. Without it a token is the whole reason the API backend exists.
//
// useAPI skips gh altogether, for a machine where gh is installed but should
// not be the way to GitHub. Only a token is tried then.
//
// lookPath and token are parameters rather than the functions themselves so a
// test can say what the machine has.
//
// Exactly one of the two clients is non-nil when err is nil. Two return values
// rather than one interface is deliberate: the caller passes whichever it got
// to usecase.New, and the compiler checks both clients answer everything the
// usecase layer asks for.
func chooseBackend(dir, repo string, useAPI bool,
	lookPath func(string) (string, error), token func() (string, error),
) (*cli.Client, *api.Client, error) {
	if !useAPI {
		if _, err := lookPath("gh"); err == nil {
			return cli.New(dir, repo), nil, nil
		}
	}
	t, err := token()
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", domain.ErrUnauthenticated, err)
	}
	return nil, api.New(dir, repo, t), nil
}
```

- [ ] **Step 4: 通ることを確かめる**

Run: `go test ./cmd/octoscope/`
Expected: `main.go` の呼び出しが古いのでまだコンパイルエラー。Step 5 のあとで通る。

- [ ] **Step 5: 文言を足す**

`internal/i18n/locales/active.en.yaml` の `no_backend:` の 2 行の直後:

```yaml
  no_token:
    other: "The GitHub API was asked for (--api, OCTOSCOPE_API or api in the settings file), but neither GH_TOKEN nor GITHUB_TOKEN is set. Set one to a personal access token."
  invalid_api:
    other: "OCTOSCOPE_API is \"{{.Value}}\", which is neither true nor false. Use 1 or true to call the GitHub API directly, 0 or false not to."
```

`internal/i18n/locales/active.ja.yaml` の `no_backend:` の 2 行の直後:

```yaml
  no_token:
    other: "GitHub API への直接接続が指定されています（--api / OCTOSCOPE_API / 設定ファイルの api）が、GH_TOKEN も GITHUB_TOKEN も設定されていません。どちらかに個人アクセストークンを設定してください。"
  invalid_api:
    other: "OCTOSCOPE_API の値「{{.Value}}」は真偽値として読めません。GitHub API に直接接続するなら 1 か true、しないなら 0 か false を指定してください。"
```

- [ ] **Step 6: `main.go` を配線する**

`--icons` の宣言の直後にフラグを足す。

```go
	useAPIFlag := flag.Bool("api", false,
		"call the GitHub API directly with GH_TOKEN or GITHUB_TOKEN, even when gh is installed; "+
			apiEnvVar+" or the settings file can set it permanently")
```

`flag.Parse()` と `--version` の処理はそのまま。`chooseBackend` を呼んでいる箇所（`ghClient, apiClient, err := chooseBackend(...)` から `os.Exit(1)` の `}` まで）を次に置き換える。

```go
	apiSet := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "api" {
			apiSet = true
		}
	})
	apiEnv := os.Getenv(apiEnvVar)
	useAPI, err := resolveAPI(apiSet, *useAPIFlag, apiEnv, cfg.API)
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.Tf("error.invalid_api", map[string]any{"Value": apiEnv}))
		os.Exit(1)
	}

	// Whether the current directory has a repository is settled by the UI,
	// not here: answering it costs a gh subprocess, and waiting for one before
	// the first frame left the terminal blank for as long as it took.
	ghClient, apiClient, err := chooseBackend(dir, *repoFlag, useAPI, exec.LookPath, api.Token)
	if err != nil {
		// Printed before the program starts: once it is in the alt screen,
		// nothing written here survives the screen being cleared. Asked for
		// the API, installing gh would not help, so only the token is named.
		msg := "error.no_backend"
		if useAPI {
			msg = "error.no_token"
		}
		fmt.Fprintln(os.Stderr, i18n.T(msg))
		os.Exit(1)
	}
```

- [ ] **Step 7: 通ることを確かめる**

Run: `go test ./cmd/octoscope/ ./internal/i18n/ ./internal/app/config/`
Expected: PASS

- [ ] **Step 8: 実際に起動して確かめる**

トークンを持たない状態と持つ状態で、`gh` が入っているこのマシンで確かめる。

```bash
env -u GH_TOKEN -u GITHUB_TOKEN go run ./cmd/octoscope --api; echo "exit=$?"
# 期待: error.no_token の英文、exit=1（gh があっても no_backend にならない）

env -u GH_TOKEN -u GITHUB_TOKEN go run ./cmd/octoscope --api --lang ja; echo "exit=$?"
# 期待: error.no_token の日本語文、exit=1

OCTOSCOPE_API=yes go run ./cmd/octoscope; echo "exit=$?"
# 期待: error.invalid_api（"yes" が入る）、exit=1

GH_TOKEN="$(gh auth token)" go run ./cmd/octoscope --api
# 期待: TUI が起動し、Work タブに一覧が出る（api 経路で取れている）。q で抜ける
```

最後の確認は TTY が要る。TTY が無い環境ではユーザーに実行を頼む。

- [ ] **Step 9: コミット**

```bash
git add cmd/octoscope/backend.go cmd/octoscope/backend_test.go cmd/octoscope/main.go \
  internal/i18n/locales/active.en.yaml internal/i18n/locales/active.ja.yaml
git commit -m "feat: --api takes the API backend even with gh installed"
```

---

### Task 4: 文書を直す

**Files:**
- Modify: `README.md` / `README.ja.md`
- Modify: `docs/superpowers/specs/2026-09-08-phase4-design.md`（「**認証**」段落）

トークン権限の一覧は PR #132 から引き継いだ文面で、まだ検証していない。コミット前に、`internal/github/api` が
実際に叩くエンドポイント（GraphQL / REST、`/user/orgs` を含む）と突き合わせ、合わない行は直す。

- [ ] **Step 1: README.md**

「## Requirements」の箇条書き 1 行を置き換える。

```markdown
One of:

- [GitHub CLI](https://cli.github.com/) (`gh`), authenticated via `gh auth login`
- A personal access token in `GH_TOKEN` or `GITHUB_TOKEN`

See [Authentication](#authentication).
```

Flags 表の `--icons` の行の直後に足す。

```markdown
| `--api` | Call the GitHub API directly with a token, even when `gh` is installed. `OCTOSCOPE_API` or the settings file sets it permanently. See [Authentication](#authentication). |
```

Settings file の表の `icons` の行の直後に足す。

```markdown
| `api` | `true` to call the GitHub API directly with a token, even when `gh` is installed |
```

`--icons nerd` の段落（`...so the default is the set that needs none.`）の直後、`### Keys` の前に足す。

```markdown
### Authentication

octoscope reaches GitHub in one of two ways:

- **gh**: runs the `gh` command, which signs in as it always does — `GH_TOKEN`
  or `GITHUB_TOKEN` when set, otherwise the account from `gh auth login`.
- **API**: calls the GitHub API over HTTPS itself, with a personal access token
  read from `GH_TOKEN`, or `GITHUB_TOKEN` when that is unset. `gh` is not run.

By default octoscope uses `gh` when it is on `PATH`, and the API otherwise. To
use the API even with `gh` installed, pass `--api`, set `OCTOSCOPE_API=1`, or
put `api: true` in the settings file; they are read in that order, so
`--api=false` or `OCTOSCOPE_API=0` undoes the one below it for a single run.
A value of `OCTOSCOPE_API` that is neither true nor false stops octoscope from
starting.

The token needs these permissions:

- Classic token: `repo`. Add `read:org` if you want your organizations'
  repositories suggested in the Repos tab.
- Fine-grained token, for each repository you open: Pull requests, Issues,
  Contents and Actions (read and write), and Checks, Commit statuses and
  Metadata (read). Grant only read access if you only want to browse.

The API talks to github.com only; `GH_HOST` and `GH_ENTERPRISE_TOKEN` are not
read, so GitHub Enterprise Server needs `gh`.
```

- [ ] **Step 2: README.ja.md**

「## 必要なもの」の箇条書き 1 行を置き換える。

```markdown
次のどちらか。

- [GitHub CLI](https://cli.github.com/)（`gh`）。`gh auth login` で認証済みであること
- `GH_TOKEN` か `GITHUB_TOKEN` に設定した個人アクセストークン

詳しくは[認証](#認証)を参照。
```

フラグ表の `--icons` の行の直後に足す。

```markdown
| `--api` | `gh` が入っていても、トークンで GitHub API を直接呼ぶ。`OCTOSCOPE_API` または設定ファイルで恒久的に指定できる。[認証](#認証)を参照 |
```

設定表の `icons` の行の直後に足す。

```markdown
| `api` | `true` で、`gh` が入っていてもトークンで GitHub API を直接呼ぶ |
```

`--icons nerd` の段落（`...デフォルトはフォントを要求しない側にしてある。`）の直後、`### キー` の前に足す。

```markdown
### 認証

octoscope が GitHub に接続する経路は 2 つある。

- **gh**: `gh` コマンドを実行する。認証は `gh` 自身が行う。`GH_TOKEN` か
  `GITHUB_TOKEN` があればそれを、無ければ `gh auth login` のアカウントを使う。
- **API**: octoscope 自身が HTTPS で GitHub API を呼ぶ。トークンは `GH_TOKEN`、
  それが無ければ `GITHUB_TOKEN` から読む。`gh` は実行しない。

デフォルトでは、`PATH` に `gh` があれば gh、無ければ API を使う。`gh` が入っていても
API を使うには、`--api` を渡すか、`OCTOSCOPE_API=1` を設定するか、設定ファイルに
`api: true` を書く。この順に読むので、`--api=false` や `OCTOSCOPE_API=0` で、
その下の指定を 1 回だけ打ち消せる。`OCTOSCOPE_API` が真偽値として読めない値なら
起動を止める。

トークンに必要な権限は次のとおり。

- classic トークン: `repo`。Repos タブで所属する組織のリポジトリを候補に出すなら
  `read:org` も付ける。
- fine-grained トークン: 開くリポジトリごとに、Pull requests・Issues・Contents・
  Actions を読み書き、Checks・Commit statuses・Metadata を読み取り。見るだけなら
  すべて読み取りでよい。

API は github.com にしか接続しない。`GH_HOST` と `GH_ENTERPRISE_TOKEN` は
読まないので、GitHub Enterprise Server では `gh` を使う。
```

- [ ] **Step 3: phase4 設計の「認証」段落**

`docs/superpowers/specs/2026-09-08-phase4-design.md` の次の段落:

```markdown
**認証**: `GH_TOKEN` → `GITHUB_TOKEN`。起動時に `exec.LookPath("gh")` を試み、
見つかれば `cli`、見つからなければトークンで `api` を組む。どちらも無ければ、
`gh auth login` かトークンの設定を促すエラー画面を出す（spec §3.2）。
```

の直後に 1 段落足す。

```markdown
`gh` があっても `api` を選ぶ手段として、`--api` / `OCTOSCOPE_API` / 設定ファイルの
`api` を後から足した（`docs/superpowers/specs/2026-09-26-api-switch-design.md`）。
指定されたときは `exec.LookPath("gh")` を試みない。
```

- [ ] **Step 4: コミット**

```bash
git add README.md README.ja.md docs/superpowers/specs/2026-09-08-phase4-design.md
git commit -m "docs: say how to call the GitHub API directly with gh installed"
```

---

### Task 5: 全体の検査

- [ ] **Step 1:** Run: `make check` → Expected: tidy / lint / fmt / test がすべて通る。落ちたら直してコミットする。
- [ ] **Step 2:** `git diff main --stat` を見て、表に無いファイルに変更が入っていないことを確かめる。
- [ ] **Step 3:** PR #132 をクローズし（「#<新 PR> でやり直した」とコメント）、このブランチで PR を作る。これはユーザーの確認を取ってから行う。
