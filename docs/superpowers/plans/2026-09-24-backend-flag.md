# バックエンドを選べるようにし、トークンでの使い方を README に書く 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `gh` が PATH にあっても、`GH_TOKEN` / `GITHUB_TOKEN` を使う API バックエンドを
選べるようにする。選び方は `--icons` と同じ 3 段（フラグ → 環境変数 → 設定ファイル）。
あわせて、トークンだけで動くことを README（英・日）に書く。

**Architecture:** バックエンドの選択は今、`cmd/octoscope/backend.go` の `chooseBackend` が
「`gh` があれば cli、無ければトークンで api」と決めている（Phase 4 設計「認証」）。
利用者はこの選択に手を出せないので、`gh` が入っている機械では API バックエンドを使えない。

選択肢を 3 つにする。

| 値 | 挙動 | 条件を満たさないとき |
|---|---|---|
| `auto`（既定・未指定） | 今と同じ。`gh` → トークンの順 | `error.no_backend`（今と同じ） |
| `gh` | `gh` だけを使う | `gh` が無ければ `error.gh_not_found` |
| `api` | トークンだけを使う。`gh` は見ない | トークンが無ければ `error.no_token`（新規） |
| それ以外 | 起動しない | `error.unknown_backend`（新規）、終了コード 2 |

指定する場所は 3 つで、先にあるものが勝つ。空（未指定）なら次を見る。

1. `--backend`
2. `OCTOSCOPE_BACKEND`
3. 設定ファイルの `backend`

- **知らない値は、どの段にあっても黙って `auto` に落とさない。** `--icons` や
  `default_tab` は知らない値を既定に落とすが、あちらは落ちたことが画面で分かる。
  バックエンドは画面から区別がつかないので、打ち間違いに気づけない。
  メッセージにはどの段の値かを出す（`--backend` / `OCTOSCOPE_BACKEND` / `config.yaml`）。
  終了コードは `flag` パッケージの使い方の誤りと同じ 2。
  **設定ファイルの打ち間違いでも起動しない**のは `default_tab` の扱い
  （「設定の打ち間違いは起動を拒む理由にならない」）と逆になる。上の理由で、
  バックエンドだけは例外とする。
- **`gh` が認証していない場合は、`gh` を選んでも `auto` と同じ扱い**（起動後の
  認証エラー画面）。起動時に確かめるのは `gh` の有無だけ、というのは今と同じ。
- 解決（3 段のどれを採るか）とモードの解釈は `cmd/octoscope/backend.go` の
  純粋な関数 `resolveBackend(flag, env, configured string)` にし、`os.Getenv` は `main` が
  呼んで渡す（`resolveVersion` と同じく、テストから直接呼べるように）。
- 失敗の文言の選び分け（どのモードで何が欠けたか）は、モードからメッセージ ID を引く
  純粋な関数 `backendFailure` にしてテストする。センチネルは増やさない
  （`.claude/rules/errors.md`「分岐に使わないエラーをセンチネルにしない」）。
  `gh` が無いことは既存の `domain.ErrBackendUnavailable` で包む。
  知らない値は、段と値を持つ小さなエラー型 `unknownBackendError` で返し、`main` が
  `errors.As` で取り出してメッセージに埋める。

**Tech Stack:** Go / `flag` / `internal/i18n` / `internal/app/config`

**設計:** 設計文書は新しく書かない。Phase 4 設計（`docs/superpowers/specs/2026-09-08-phase4-design.md`）
の「認証」の段落に、上書きできることを 1 段落足す（Task 4）。
パッケージ境界・データの流れ・画面は動かない。

---

## ファイル構成

| ファイル | 変更 |
|---|---|
| `internal/app/config/config.go` / `config_test.go` | `Backend` フィールド（`yaml:"backend,omitempty"`）。値の解釈はしない（`Icons` と同じ） |
| `internal/app/adapter/datasource/datasource_test.go` | 保存しても `backend` が残ることを既存テストに足す |
| `cmd/octoscope/backend.go` | `backendMode`、`backendEnvVar`、`resolveBackend`、`unknownBackendError`、`chooseBackend` がモードを受け取る、`backendFailure` |
| `cmd/octoscope/backend_test.go` | 既存テストをモード付きに直し、解決・`gh` / `api` / 不正値のテストを足す |
| `cmd/octoscope/main.go` | `--backend` フラグ、不正値で終了コード 2、失敗時の文言をモードで引く |
| `internal/i18n/locales/active.en.yaml` / `active.ja.yaml` | `error.no_token`、`error.unknown_backend` |
| `README.md` / `README.ja.md` | 必要なもの（gh またはトークン）、認証の節、フラグ表と設定表に `backend` |
| `docs/superpowers/specs/2026-09-08-phase4-design.md` | 「認証」に 1 段落 |

---

### Task 1: 設定ファイルの `backend`

**Files:** `internal/app/config/config.go`、`config_test.go`、`internal/app/adapter/datasource/datasource_test.go`

- [ ] **Step 1: テストを直す** — `TestLoadReadsEveryField` の入力に `backend: api` を足し、
  `want` に `Backend: "api"`。`TestSaveRepositoriesKeepsTheOtherSettings` と
  `TestSavingQueriesLeavesTheStartupSettingsAlone` の入力にも `backend: api` を足し、残ることを確かめる
  （`Store.save` は `Config` を丸ごと書き戻すので、構造体に無いキーは保存で消える。それを防ぐテスト）。
- [ ] **Step 2: 失敗を確かめる** — `go test ./internal/app/config/ ./internal/app/adapter/datasource/`
- [ ] **Step 3: `Config` に `Backend string \`yaml:"backend,omitempty"\`` を足す**（`Icons` の下）
- [ ] **Step 4: テストを通す**

### Task 2: モードの解決と `chooseBackend`

**Files:** `cmd/octoscope/backend.go`、`cmd/octoscope/backend_test.go`

- [ ] **Step 1: 失敗するテストを書く**

既存の 4 テストは `chooseBackend(dir, repo, backendAuto, lookPath, token)` に直すだけで意味は変えない。足すもの:

- `TestResolveBackend`（表駆動）:
  - 全部空 → `backendAuto`
  - フラグ `api`、環境 `gh`、設定 `gh` → `backendAPI`（フラグが勝つ）
  - フラグ空、環境 `gh`、設定 `api` → `backendGh`
  - 設定だけ `api` → `backendAPI`
  - ` GH ` → `backendGh`（大文字・前後空白を受ける。`icon.Resolve` と同じ）
  - フラグ `cli` → `unknownBackendError{Source: "--backend", Value: "cli"}`
  - 環境 `cli`、設定 `api` → `Source: "OCTOSCOPE_BACKEND"`（次の段へ落ちない）
  - 設定 `cli` → `Source: "config.yaml"`
- `TestBackendGhIgnoresAToken`: `gh` モードで `gh` が無くトークンがある → cli も api も返さず、`domain.ErrBackendUnavailable`
- `TestBackendGhUsesGh`: `gh` がある → cli
- `TestBackendAPISkipsGh`: `gh` があってもトークンで api を返す
- `TestBackendAPIWithoutATokenIsUnauthenticated`: `domain.ErrUnauthenticated`、トークン側のエラーも包まれている
- `TestBackendFailure`（表駆動）: `auto` → `error.no_backend`、`gh` → `error.gh_not_found`、`api` → `error.no_token`

- [ ] **Step 2: 走らせて失敗を確かめる** — `go test ./cmd/octoscope/`（未定義でコンパイルエラー）

- [ ] **Step 3: 実装する**

```go
type backendMode string

const (
	backendAuto backendMode = "auto"
	backendGh   backendMode = "gh"
	backendAPI  backendMode = "api"
)

const backendEnvVar = "OCTOSCOPE_BACKEND"

type unknownBackendError struct {
	Source string // --backend, OCTOSCOPE_BACKEND, or config.yaml
	Value  string
}

func (e *unknownBackendError) Error() string { ... }

// resolveBackend picks the backend from the flag, the environment and the
// settings file, in that order; an empty one defers to the next. An unknown
// value is an error rather than a fall back to auto: nothing on screen tells
// the two backends apart, so a typo would otherwise go unnoticed.
func resolveBackend(flag, env, configured string) (backendMode, error) {
	for _, c := range []struct{ source, value string }{
		{"--backend", flag}, {backendEnvVar, env}, {"backend", configured},
	} {
		v := strings.ToLower(strings.TrimSpace(c.value))
		switch backendMode(v) {
		case "":
			continue
		case backendAuto, backendGh, backendAPI:
			return backendMode(v), nil
		}
		return "", &unknownBackendError{Source: c.source, Value: c.value}
	}
	return backendAuto, nil
}

func chooseBackend(dir, repo string, mode backendMode,
	lookPath func(string) (string, error), token func() (string, error),
) (*cli.Client, *api.Client, error) {
	if mode != backendAPI {
		_, err := lookPath("gh")
		if err == nil {
			return cli.New(dir, repo), nil, nil
		}
		if mode == backendGh {
			return nil, nil, fmt.Errorf("%w: %w", domain.ErrBackendUnavailable, err)
		}
	}
	t, err := token()
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", domain.ErrUnauthenticated, err)
	}
	return nil, api.New(dir, repo, t), nil
}

// backendFailure is the message ID for chooseBackend failing under mode:
// what was missing depends on what the user asked for.
func backendFailure(mode backendMode) string { ... }
```

`chooseBackend` の doc comment に、モードで何が変わるかを書き足す。

- [ ] **Step 4: テストを走らせる** — `go test ./cmd/octoscope/ -v`

### Task 3: フラグとメッセージ

**Files:** `cmd/octoscope/main.go`、`internal/i18n/locales/active.{en,ja}.yaml`

- [ ] **Step 1: カタログに足す**

```yaml
# en
  no_token:
    other: "The api backend needs a personal access token in GH_TOKEN or GITHUB_TOKEN."
  unknown_backend:
    other: "Unknown backend {{.Value}} ({{.Source}}); use auto, gh, or api."
# ja
  no_token:
    other: "api バックエンドには、GH_TOKEN か GITHUB_TOKEN に個人アクセストークンが必要です。"
  unknown_backend:
    other: "バックエンド {{.Value}}（{{.Source}}）は使えません。auto、gh、api のどれかを指定してください。"
```

- [ ] **Step 2: `main.go` に配線する**

```go
	backendFlag := flag.String("backend", "",
		"how to reach GitHub: auto (gh if it is on PATH, else a token; the default), gh, "+
			"or api (GH_TOKEN or GITHUB_TOKEN, without gh); "+
			"OCTOSCOPE_BACKEND or the settings file can set it permanently")
```

フラグの既定は `""`（`auto` にすると環境変数と設定が効かなくなる）。
設定の読み込みと言語の決定の**後**に `resolveBackend(*backendFlag, os.Getenv(backendEnvVar), cfg.Backend)`
を呼ぶ（メッセージを利用者の言語で出すため）。`unknownBackendError` は
`i18n.Tf("error.unknown_backend", …)` を stderr に出して `os.Exit(2)`。
`chooseBackend` の失敗は `i18n.T(backendFailure(mode))`。

- [ ] **Step 3: `make check`**（i18n のカタログ整合テストも含む）

- [ ] **Step 4: 実際に動かす**

```bash
go build -o /tmp/octoscope ./cmd/octoscope
/tmp/octoscope --backend nope; echo $?                              # 2 と unknown_backend（--backend）
OCTOSCOPE_BACKEND=nope /tmp/octoscope; echo $?                      # 2 と unknown_backend（OCTOSCOPE_BACKEND）
/tmp/octoscope --lang ja --backend nope                             # 日本語の文言
env -u GH_TOKEN -u GITHUB_TOKEN /tmp/octoscope --backend api        # no_token
env PATH=/nonexistent /tmp/octoscope --backend gh                   # gh_not_found
GH_TOKEN=$(gh auth token) /tmp/octoscope --backend api --repo kukv/octoscope   # gh があっても api で起動する
```

設定ファイルの `backend: nope` も `XDG_CONFIG_HOME` を一時ディレクトリに向けて確かめる。
最後の 1 つは `gh auth` が通る環境でだけ試せる。通らなければ利用者への受け渡しに回す。

### Task 4: README と設計

**Files:** `README.md`、`README.ja.md`、`docs/superpowers/specs/2026-09-08-phase4-design.md`

- [ ] **Step 1: README（英・日で同じ内容）**
  - 「Requirements」を「`gh`（`gh auth login` 済み）**または** `GH_TOKEN` / `GITHUB_TOKEN` の個人アクセストークン」に直す
  - 「Authentication」節を足す: 選ばれる順序、`gh` がある場合もトークンは `gh` 経由で効くこと、
    `--backend` / `OCTOSCOPE_BACKEND` / `backend` で選べること、必要な権限
    （API バックエンドが叩くエンドポイントから確かめて書く）、github.com 専用であること
  - フラグ表に `--backend auto|gh|api`、設定表に `backend`

- [ ] **Step 2: Phase 4 設計の「認証」に 1 段落**（上書きできる。既定は変わらない）

- [ ] **Step 3: `make check` を通してコミット、push**
