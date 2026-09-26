# gh があっても API を直接叩けるようにする設計

PR #132（`--backend auto|gh|api`）をやり直すための設計。

## 1. 問題

octoscope が GitHub に接続する経路は 2 つある。

- **gh 経路**: `gh` を子プロセスとして起動する。認証は `gh` が行う
  （`GH_TOKEN` / `GITHUB_TOKEN` があればそれを使い、無ければ `gh auth login` の保存済みログインを使う）。
- **api 経路**: octoscope 自身がトークンを付けて HTTPS で GitHub API を呼ぶ。`gh` は使わない。

`cmd/octoscope/backend.go` の `chooseBackend` は、`exec.LookPath("gh")` が成功した時点で必ず gh 経路を選ぶ。
そのため `gh` が入っているマシンでは、**`gh` を通らずに API を叩く手段が無い**。
トークンを使うこと自体は今でもできる（`gh` がトークンを読む）が、それでも `gh` のプロセスは必ず経由する。

欲しいのは、この api 経路への逃げ道である。

## 2. PR #132 から変えること

PR #132 は `auto` / `gh` / `api` の 3 値で経路を選ばせていた。
欲しいのは「api 経路を使う」という 1 つのスイッチだけで、`auto` と `gh` の 2 値は過剰だった。

- `gh` を強制するモードは作らない。
- 既定の動作は変えない。

## 3. 利用者から見た形

### 3.1 指定のしかた

上から順に見て、先に指定が見つかったものを使う。

| 優先順 | 場所 | 書き方 |
|---|---|---|
| 1 | フラグ | `--api`。`--api=false` と書けば、下の 2 つの指定を打ち消せる |
| 2 | 環境変数 | `OCTOSCOPE_API`。`strconv.ParseBool` が読める値（`1` / `true` / `0` / `false` など）。`0` / `false` なら設定ファイルの指定を打ち消せる |
| 3 | 設定ファイル | `api: true` |

### 3.2 動作

- **どこにも指定が無い**: 今と同じ。`gh` が `PATH` にあれば gh 経路、無ければトークンで api 経路。
- **api 経路が指定された**: `gh` を探さず、トークン（`GH_TOKEN`、それが無ければ `GITHUB_TOKEN`）で api 経路を組む。
  トークンが無ければ、画面に入る前に stderr へ `error.no_token` を出して終了する。
  今の `error.no_backend`（「gh を入れるかトークンを設定せよ」）は、この場合には案内が的外れなので使わない。
- **`OCTOSCOPE_API` が真偽値として読めない**（`yes` など）: 起動を止め、`error.invalid_api` で値と、それが書かれていた場所を示す。
  `--icons` は打ち間違いを黙って既定値として扱うが、ここではそうしない。
  どちらの経路で動いているかは画面に出ないので、打ち間違いに気づく機会が無いためである。
- **設定ファイルの `api:` が bool として読めない**: YAML のデコードに失敗するので、既存の「設定ファイルが読めない」扱い
  （既定値で起動し、警告を出す）になる。新しい扱いは足さない。

### 3.3 作らないもの

- `auto` / `gh` という値
- gh 経路の強制
- 画面上に経路を表示すること
- GitHub Enterprise Server への対応（api 経路は今と同じく github.com のみ。`GH_HOST` / `GH_ENTERPRISE_TOKEN` は読まない）

## 4. 内部の構造

### 4.1 `internal/app/config`

`Config` に次を足す。

```go
API bool `yaml:"api,omitempty"`
```

設定ファイルは優先順位が最下位なので、「未指定」と `false` を区別する必要が無い。そのため `*bool` にはしない。

### 4.2 `cmd/octoscope/backend.go`

経路の選択は今もここにあるので、同じ場所に置く。

```go
// resolveAPI decides whether to take the API backend.
func resolveAPI(flagSet, flagValue bool, env string, configured bool) (bool, error)

// invalidAPIError is an OCTOSCOPE_API value strconv.ParseBool cannot read.
type invalidAPIError struct {
	Source string
	Value  string
}
```

- `flagSet` が真なら `flagValue` を返す。フラグが明示されたかどうかは、`main` が `flag.Visit` で調べて渡す。
- `env` が（前後の空白を除いて）空でなければ `strconv.ParseBool` で読む。読めなければ `*invalidAPIError` を返す。
- どちらも無ければ `configured` を返す。
- 環境変数は `main` が `os.Getenv` で読んでから渡す。こうするとテストで環境変数を触らずに済む。

`chooseBackend` に `useAPI bool` を足す。

```go
func chooseBackend(dir, repo string, useAPI bool,
	lookPath func(string) (string, error), token func() (string, error),
) (*cli.Client, *api.Client, error)
```

- `useAPI` が偽: 今と同じ処理。
- `useAPI` が真: `lookPath` を呼ばず、トークンで api 経路を組む。
  トークンが無いときのエラーの包み方（`domain.ErrUnauthenticated` で包む）は今と同じ。

### 4.3 `cmd/octoscope/main.go`

- `--api` フラグを足す。
- `resolveAPI` を呼ぶ。失敗したら `error.invalid_api` を stderr に出して終了する。
- `chooseBackend` に `useAPI` を渡す。失敗したときのメッセージは、`useAPI` が真なら `error.no_token`、偽なら今の `error.no_backend`。

### 4.4 i18n（en / ja）

| キー | 内容 |
|---|---|
| `error.no_token` | api 経路が指定されているが `GH_TOKEN` / `GITHUB_TOKEN` が無い |
| `error.invalid_api` | `{{.Source}}` に書かれた `{{.Value}}` は真偽値として読めない |

## 5. テスト

`cmd/octoscope/backend_test.go`:

- `resolveAPI` の優先順位を表で確かめる。次を含める。
  - `--api=false` が環境変数と設定ファイルを打ち消す
  - `OCTOSCOPE_API=0` が設定ファイルを打ち消す
  - どこにも指定が無ければ偽
- 不正な値のエラーが、場所（`OCTOSCOPE_API`）と値を持っていること。
- `chooseBackend`:
  - `useAPI` が真なら、`gh` があっても api 経路になり、`lookPath` が呼ばれないこと
  - `useAPI` が真でトークンが無ければ、`domain.ErrUnauthenticated` になること
  - 既存の 3 テストは `useAPI=false` を足すだけで通ること

`internal/app/config/config_test.go`: `api: true` が読めること。

## 6. 文書

- README（en / ja）: オプション表に `--api`、設定表に `api`、「認証」節（2 つの経路と、その選び方）。
- `docs/superpowers/specs/2026-09-08-phase4-design.md` の「認証」段落: 「`gh` があれば `cli`」に、`--api` / `OCTOSCOPE_API` / `api` による上書きを書き足す。

## 7. 進め方

PR #132 はクローズし、この設計と実装計画を載せたブランチ `feat/api-switch` から 1 本の PR にする。
