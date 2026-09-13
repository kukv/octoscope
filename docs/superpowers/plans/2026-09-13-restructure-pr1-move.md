# アーキテクチャ再構成 PR 1（機械的な移動とリネーム）実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** パッケージを設計書 §3.1 のツリーへ移し、import パスと 2 つのパッケージ名を書き換える。振る舞いは一切変えない。

**Architecture:** `git mv` でディレクトリを動かし、import パス文字列と修飾子を `sed` で一括置換する。パッケージ名が変わるのは `gh` → `domain` と `tui/app` の `app` → `root` の 2 つだけで、`cli` / `api` / `gql` / `usecase` / `config` はパスだけが変わる。gateway も datasource もこの PR では作らない。`internal/github` は引き続き domain を import する。

**Tech Stack:** Go 1.x、golangci-lint（depguard / gofumpt / goimports）、gotestsum

**Spec:** `docs/superpowers/specs/2026-09-13-architecture-restructure-design.md`

## Global Constraints

- **振る舞いを変えない。** この PR の diff に、import・パッケージ名・パス以外の変更を入れない。ロジックの「ついでの改善」もコメントの整形も禁止
- **golden ファイルと testdata を 1 バイトも変えない。** 各タスクの検証で `git diff --stat -- '*testdata*'` が空であることを確認する
- **`internal/github` はこの PR ではまだ `internal/app/domain` を import する。** domain 非依存化は PR 2
- 設計書 §4 の 3 つの機械検査（reflect テスト、`encoding/json` 禁止、`internal/app` 禁止）はこの PR では**入れない**。PR 2 で入れる
- モジュールパスは `github.com/kukv/octoscope`
- `sed` の修飾子置換は **`*.go` に限定する。** `testdata/` 配下には `.json` / `.txt` の中に Go のソース断片が入っており（`internal/gh/cli/testdata/pr_files.json`、`internal/gh/api/testdata/pr_diff.txt`）、これらを書き換えるとテストが壊れる。`.go` ファイルは testdata 配下に 1 つも無いことを確認済み
- コミットメッセージの末尾に `Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>` を付ける

## 実測値（2026-09-13、着手時点）

置換の規模。タスクの検証で使う。

| 対象 | 値 |
|---|---|
| `.go` ファイル総数 | 173 |
| `internal/gh"`（ドメイン）を import するファイル | 127 |
| `internal/gh/{cli,api,gql}` を import するファイル | 15 |
| `internal/tui` を import するファイル | 46 |
| `internal/usecase` を import するファイル | 23 |
| `internal/config` を import するファイル | 7 |
| `.go` 内の `gh.` + 大文字の出現 | 1,962 |
| `.go` 内の `gh.` + 小文字の出現 | **0**（＝ `\bgh\.[A-Z]` は安全な置換パターン） |
| `internal/tui/app` の外から使われている `app.` 修飾子 | `cmd/octoscope/main.go:80` の 2 箇所のみ |

## ファイル構成（移動後）

```
internal/
  app/
    config/                 ← internal/config（パッケージ名そのまま）
    usecase/                ← internal/usecase（パッケージ名そのまま）
    domain/                 ← internal/gh のルート（package gh → package domain）
    presentation/tui/       ← internal/tui
      root/                 ← internal/tui/app（package app → package root）
      work/ repo/ detail/ diff/ checks/ merge/ review/ search/
      dialog/ layout/ theme/ icon/
  github/
    cli/ api/ gql/          ← internal/gh/{cli,api,gql}（パッケージ名そのまま）
  browser/  i18n/  golden/  ← 変更なし
cmd/octoscope/              ← import パスのみ変更
```

この PR で `internal/app/adapter/` は作らない（空ディレクトリを置かない）。

## 検証の考え方

これはリファクタリングであり、新しい振る舞いを足さない。したがって**既存のテストスイートと golden ファイルがテストそのもの**である。各タスクの検証は次の 3 点で、すべて満たすまで次へ進まない。

1. `make check` が通る（tidy / lint / fmt / race 付きテスト）
2. `git status --porcelain` に testdata / golden の変更が 1 件も無い
3. そのタスクで消したはずの古い import パスが 1 件も残っていない

---

### Task 1: `internal/config` → `internal/app/config`

最小の移動で手順を確立する。パッケージ名は変わらない（`config` のまま）。

**Files:**
- Move: `internal/config/` → `internal/app/config/`
- Modify: `internal/config` を import する 7 ファイル
- Test: 既存の `internal/app/config/config_test.go`（移動に伴って移る）

**Interfaces:**
- Consumes: なし（最初のタスク）
- Produces: import パス `github.com/kukv/octoscope/internal/app/config`。パッケージ名は `config`、`config_test` のまま。公開 API（`Config` `SavedQuery` `Load` `Path` `NewStore` `Store` `DefaultTabName` `SaveRepositories` `SaveQueries`）は一切変えない

- [ ] **Step 1: 移動前の基準を取る**

```bash
make check
git diff --stat   # 空であること
grep -rl 'kukv/octoscope/internal/config' --include='*.go' . | wc -l   # 7 を期待
```

期待: `make check` が通り、作業ツリーがきれい。

- [ ] **Step 2: ディレクトリを移す**

```bash
mkdir -p internal/app
git mv internal/config internal/app/config
```

- [ ] **Step 3: import パスを書き換える**

```bash
grep -rl 'kukv/octoscope/internal/config' --include='*.go' . \
  | xargs sed -i 's#github.com/kukv/octoscope/internal/config#github.com/kukv/octoscope/internal/app/config#g'
```

- [ ] **Step 4: 古いパスが残っていないことを確認する**

```bash
grep -rn 'kukv/octoscope/internal/config"' --include='*.go' .
```

期待: 出力なし（終了コード 1）。

- [ ] **Step 5: 整形して検査する**

```bash
make fmt
make check
```

期待: すべて PASS。

- [ ] **Step 6: golden と testdata が無傷であることを確認する**

```bash
git status --porcelain | grep -i 'testdata' || echo "testdata は無変更"
```

期待: `testdata は無変更` と出る。

- [ ] **Step 7: コミット**

```bash
git add -A
git commit -m "$(cat <<'EOF'
refactor: move config under internal/app

The settings reader belongs to the application, not beside it. No
behaviour changes: paths and imports only.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: `internal/usecase` → `internal/app/usecase`

**Files:**
- Move: `internal/usecase/` → `internal/app/usecase/`
- Modify: `internal/usecase` を import する 23 ファイル

**Interfaces:**
- Consumes: Task 1 の `github.com/kukv/octoscope/internal/app/config`（`usecase.go` が `config.SavedQuery` を参照している）
- Produces: import パス `github.com/kukv/octoscope/internal/app/usecase`。パッケージ名は `usecase` のまま。公開 API（`Usecase` `New` `Item` `ReviewTarget` `SavedQuery` および `Usecase` の全メソッド）は変えない

- [ ] **Step 1: 移動前の基準を取る**

```bash
git diff --stat   # 空であること
grep -rl 'kukv/octoscope/internal/usecase' --include='*.go' . | wc -l   # 23 を期待
```

- [ ] **Step 2: ディレクトリを移す**

```bash
git mv internal/usecase internal/app/usecase
```

- [ ] **Step 3: import パスを書き換える**

```bash
grep -rl 'kukv/octoscope/internal/usecase' --include='*.go' . \
  | xargs sed -i 's#github.com/kukv/octoscope/internal/usecase#github.com/kukv/octoscope/internal/app/usecase#g'
```

- [ ] **Step 4: 古いパスが残っていないことを確認する**

```bash
grep -rn 'kukv/octoscope/internal/usecase"' --include='*.go' .
```

期待: 出力なし。

- [ ] **Step 5: 整形して検査する**

```bash
make fmt
make check
```

期待: すべて PASS。

- [ ] **Step 6: golden と testdata が無傷であることを確認する**

```bash
git status --porcelain | grep -i 'testdata' || echo "testdata は無変更"
```

- [ ] **Step 7: コミット**

```bash
git add -A
git commit -m "$(cat <<'EOF'
refactor: move usecase under internal/app

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: `internal/gh/{cli,api,gql}` → `internal/github/{cli,api,gql}`

通信層をアプリの外へ出す。この時点ではまだ `internal/gh`（ドメイン）を import している。

**Files:**
- Move: `internal/gh/cli/` → `internal/github/cli/`、`internal/gh/api/` → `internal/github/api/`、`internal/gh/gql/` → `internal/github/gql/`（`testdata/` を含む）
- Modify: これらを import する 15 ファイル（`cmd/octoscope/backend.go` を含む）

**Interfaces:**
- Consumes: なし
- Produces: import パス `github.com/kukv/octoscope/internal/github/cli`、`.../api`、`.../gql`。パッケージ名は `cli` / `api` / `gql` のまま。公開 API（`cli.New` `cli.Client`、`api.New` `api.Client` `api.Token`、`gql.Client` `gql.Transport` `gql.Var` `gql.VarKind` `gql.S` `gql.N` `gql.Placeholder` `gql.SplitRepoVars`）は変えない

- [ ] **Step 1: 移動前の基準を取る**

```bash
git diff --stat   # 空であること
grep -rl 'kukv/octoscope/internal/gh/' --include='*.go' . | wc -l   # 15 を期待
```

- [ ] **Step 2: 3 つのディレクトリを移す**

```bash
mkdir -p internal/github
git mv internal/gh/cli internal/github/cli
git mv internal/gh/api internal/github/api
git mv internal/gh/gql internal/github/gql
```

- [ ] **Step 3: import パスを書き換える**

`internal/gh/` の末尾スラッシュが効くので、ドメイン（`internal/gh"`）には当たらない。

```bash
grep -rl 'kukv/octoscope/internal/gh/' --include='*.go' . \
  | xargs sed -i 's#github.com/kukv/octoscope/internal/gh/#github.com/kukv/octoscope/internal/github/#g'
```

- [ ] **Step 4: 古いパスが残っていないことを確認する**

```bash
grep -rn 'kukv/octoscope/internal/gh/' --include='*.go' .
```

期待: 出力なし。

- [ ] **Step 5: `internal/gh` にドメインのファイルだけが残ったことを確認する**

```bash
ls internal/gh
```

期待: `gh.go` `checks.go` `diff.go` `diff_parse.go` `merge.go` `review.go` とそれぞれの `_test.go`、そして `testdata/` だけ。`cli` / `api` / `gql` が無いこと。

- [ ] **Step 6: 整形して検査する**

```bash
make fmt
make check
```

期待: すべて PASS。

- [ ] **Step 7: golden と testdata が無傷であることを確認する**

移動した testdata は `git mv` によって rename として記録される。中身が変わっていないことを確認する。

```bash
git diff --cached --stat -M -- '*testdata*' | tail -3
git status --porcelain | grep -E '^\s*M.*testdata' || echo "testdata の中身は無変更"
```

期待: `testdata の中身は無変更` と出る（rename は出てよい、変更は出てはいけない）。

- [ ] **Step 8: コミット**

```bash
git add -A
git commit -m "$(cat <<'EOF'
refactor: move the GitHub transports to internal/github

They talk to a service, so they sit beside the application rather than
inside it. They still import the domain types; PR 2 cuts that.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: `internal/gh` → `internal/app/domain`（パッケージ名 `gh` → `domain`）

この PR で唯一、修飾子を大量に書き換えるタスク。1,962 箇所。

**Files:**
- Move: `internal/gh/` → `internal/app/domain/`
- Modify: `internal/app/domain/gh.go:1`（パッケージ doc）、`internal/app/domain/*.go` のパッケージ節、`internal/gh"` を import する 127 ファイル
- Rename: `internal/app/domain/gh.go` → `internal/app/domain/domain.go`

**Interfaces:**
- Consumes: なし
- Produces: import パス `github.com/kukv/octoscope/internal/app/domain`、パッケージ名 `domain` / `domain_test`、修飾子 `domain.`。型と関数の名前は**一切変えない**（`domain.PR` `domain.Issue` `domain.ItemRef` `domain.WorkItem` `domain.Checks` `domain.CheckRun` `domain.ReviewContext` `domain.MergeContext` `domain.FileDiff` `domain.SplitRepo` `domain.IsFatal` `domain.Classify` `domain.ErrGhNotFound` `domain.ErrTransient` `domain.ErrUnauthenticated` `domain.ParseItemState` `domain.ParseReviewDecision` `domain.ParseDiff` `domain.ParseFilesAPI` ほか）

- [ ] **Step 1: 移動前の基準を取る**

```bash
git diff --stat   # 空であること
grep -rl 'kukv/octoscope/internal/gh"' --include='*.go' . | wc -l    # 127 を期待
grep -rho '\bgh\.[A-Z][A-Za-z]*' --include='*.go' . | wc -l          # 1962 を期待
grep -rn '\bgh\.[a-z]' --include='*.go' . | wc -l                    # 0 を期待
```

最後の 0 が、`\bgh\.[A-Z]` を機械置換してよい根拠である。0 でなければ止めて報告すること。

- [ ] **Step 2: ディレクトリを移し、ファイル名を直す**

```bash
git mv internal/gh internal/app/domain
git mv internal/app/domain/gh.go internal/app/domain/domain.go
git mv internal/app/domain/gh_test.go internal/app/domain/domain_test.go
```

- [ ] **Step 3: パッケージ節を書き換える**

```bash
sed -i 's/^package gh$/package domain/; s/^package gh_test$/package domain_test/' internal/app/domain/*.go
grep -h '^package ' internal/app/domain/*.go | sort -u
```

期待: `package domain` と `package domain_test` の 2 行だけ。

- [ ] **Step 4: パッケージ doc を書き換える**

`internal/app/domain/domain.go` の 1〜3 行目を、次のとおりに置き換える。

```go
// Package domain holds the types the application is written in terms of.
// It has no behaviour beyond the rules those types carry, and it depends on
// nothing: the clients that fetch this data live under internal/github, and
// they translate their own service's spelling into these values.
package domain
```

- [ ] **Step 5: import パスを書き換える**

```bash
grep -rl 'kukv/octoscope/internal/gh"' --include='*.go' . \
  | xargs sed -i 's#github.com/kukv/octoscope/internal/gh"#github.com/kukv/octoscope/internal/app/domain"#g'
```

- [ ] **Step 6: 修飾子を書き換える**

`[A-Z]` に限定するのが要点。`.go` 以外は触らない。

```bash
grep -rl '\bgh\.[A-Z]' --include='*.go' . \
  | xargs sed -i 's/\bgh\.\([A-Z]\)/domain.\1/g'
```

- [ ] **Step 7: 残りが無いことを確認する**

```bash
grep -rn 'kukv/octoscope/internal/gh"' --include='*.go' .
grep -rn '\bgh\.[A-Z]' --include='*.go' .
```

期待: どちらも出力なし。

- [ ] **Step 8: `gh` という語が正当な場所にだけ残っていることを目で確認する**

gh CLI そのものを指す記述（`gh auth login`、`gh api graphql`、`exec.Command("gh", ...)`、`ErrGhNotFound`）は残ってよい。それ以外に `gh` が残っていないか見る。

```bash
grep -rn '\bgh\b' --include='*.go' internal/app | head -20
```

期待: `internal/app` の下に gh CLI への言及がほぼ無いこと。`ErrGhNotFound` は `internal/app/domain/domain.go` に残る（PR 3 で移す）。

- [ ] **Step 9: 整形して検査する**

```bash
make fmt
make check
```

期待: すべて PASS。`make check` のテスト本数が移動前と同じであること。

- [ ] **Step 10: golden と testdata が無傷であることを確認する**

```bash
git status --porcelain | grep -E '^\s*M.*testdata' || echo "testdata の中身は無変更"
```

期待: `testdata の中身は無変更`。

- [ ] **Step 11: 画面が変わっていないことを実際に見る**

```bash
go run ./cmd/octoscope --repo kukv/octoscope
go run ./cmd/octoscope --repo kukv/octoscope --lang ja
```

期待: 再構成前と同じ画面が出て、キー操作が同じように動く。`q` で終了する。

- [ ] **Step 12: コミット**

```bash
git add -A
git commit -m "$(cat <<'EOF'
refactor: rename the gh package to domain

internal/gh named both the types the application is written in and the
clients that fetch them. The clients moved out last commit; this renames
what is left for what it is. Identifiers are untouched -- only the package
name, its path, and the qualifier on 1,962 references.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: `internal/tui` → `internal/app/presentation/tui`、`tui/app` → `tui/root`

**Files:**
- Move: `internal/tui/` → `internal/app/presentation/tui/`
- Move: `internal/app/presentation/tui/app/` → `internal/app/presentation/tui/root/`
- Rename: `.../tui/root/app.go` → `.../tui/root/root.go`
- Modify: `internal/tui` を import する 46 ファイル、`cmd/octoscope/main.go:80`
- Modify: `.../tui/root/root.go:1`（パッケージ doc）

**Interfaces:**
- Consumes: Task 2 の `.../internal/app/usecase`、Task 4 の `.../internal/app/domain`
- Produces: import パス `github.com/kukv/octoscope/internal/app/presentation/tui/<view>`。root のパッケージ名は `root`、修飾子は `root.`。公開 API（`root.New(src Source, opts Options) Model`、`root.Source`、`root.Options`）は名前だけ変わり形は同じ。他のビューのパッケージ名（`work` `repo` `detail` `diff` `checks` `merge` `review` `search` `dialog` `layout` `theme` `icon`）は変えない

- [ ] **Step 1: 移動前の基準を取る**

```bash
git diff --stat   # 空であること
grep -rl 'kukv/octoscope/internal/tui' --include='*.go' . | wc -l   # 46 を期待
grep -rn '\bapp\.[A-Z]' --include='*.go' . | grep -v '^./internal/tui/app/'
```

最後のコマンドの期待: `cmd/octoscope/main.go:80` の 1 行（`app.New` と `app.Options`）だけがコードとして出る。他はコメント内の言及。

- [ ] **Step 2: ディレクトリを移す**

```bash
mkdir -p internal/app/presentation
git mv internal/tui internal/app/presentation/tui
git mv internal/app/presentation/tui/app internal/app/presentation/tui/root
git mv internal/app/presentation/tui/root/app.go internal/app/presentation/tui/root/root.go
git mv internal/app/presentation/tui/root/app_test.go internal/app/presentation/tui/root/root_test.go
```

- [ ] **Step 3: import パスを書き換える**

`tui/app` の方を先にやる。後から `internal/tui` を置換すると `internal/app/presentation/tui/app` が生まれてしまうため。

```bash
grep -rl 'kukv/octoscope/internal/tui/app' --include='*.go' . \
  | xargs sed -i 's#github.com/kukv/octoscope/internal/tui/app#github.com/kukv/octoscope/internal/app/presentation/tui/root#g'

grep -rl 'kukv/octoscope/internal/tui' --include='*.go' . \
  | xargs sed -i 's#github.com/kukv/octoscope/internal/tui#github.com/kukv/octoscope/internal/app/presentation/tui#g'
```

- [ ] **Step 4: パッケージ節と修飾子を書き換える**

```bash
sed -i 's/^package app$/package root/' internal/app/presentation/tui/root/*.go
sed -i 's/\bapp\.New(/root.New(/g; s/\bapp\.Options{/root.Options{/g' cmd/octoscope/main.go
grep -h '^package ' internal/app/presentation/tui/root/*.go | sort -u
```

期待: `package root` の 1 行だけ。

- [ ] **Step 5: パッケージ doc を書き換える**

`internal/app/presentation/tui/root/root.go` の 1〜2 行目を次に置き換える。

```go
// Package root is the terminal UI's root model: it owns the tabs, hands each
// child its size, and shows the error screen.
package root
```

- [ ] **Step 6: 残りが無いことを確認する**

```bash
grep -rn 'kukv/octoscope/internal/tui' --include='*.go' .
grep -rn '\bapp\.[A-Z]' --include='*.go' cmd/
```

期待: どちらも出力なし。

- [ ] **Step 7: コメント内の `internal/tui/app` への言及を直す**

```bash
grep -rn 'internal/tui\|app\.New\|app\.Options' --include='*.go' internal cmd
```

出たコメント（`internal/app/presentation/tui/search/saved.go:22` の「the way app.New」、`internal/app/usecase/usecase_test.go:478` の「hand app.Options」）を、`root.New` / `root.Options` に直す。コメント以外は変えない。

- [ ] **Step 8: 整形して検査する**

```bash
make fmt
make check
```

期待: すべて PASS。

- [ ] **Step 9: golden と testdata が無傷であることを確認する**

```bash
git status --porcelain | grep -E '^\s*M.*testdata' || echo "testdata の中身は無変更"
```

- [ ] **Step 10: 画面を実際に見る**

```bash
go run ./cmd/octoscope --repo kukv/octoscope
go run ./cmd/octoscope --repo kukv/octoscope --lang ja
```

期待: タブの移動、詳細ビュー、diff、checks が再構成前と同じに動く。

- [ ] **Step 11: コミット**

```bash
git add -A
git commit -m "$(cat <<'EOF'
refactor: move the TUI under app/presentation and rename its root

The terminal UI is how this application is displayed, not what owns it.
internal/tui/app was the Bubble Tea root model, and the name claimed more
than that; it is now tui/root, which frees "app" for the application.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: depguard を新しい構成で書き直す

**Files:**
- Modify: `.golangci.yml` の `depguard` 設定（現在 6 ルール、すべて `internal/gh` などの旧パスを名指ししている）

**Interfaces:**
- Consumes: Task 1〜5 の新しいパス
- Produces: 新しいルール名 `domain-layer` `github-layer` `usecase-layer` `presentation-layer` `config-layer` `i18n-layer` `browser-layer`

- [ ] **Step 1: 現在のルールを読む**

```bash
sed -n '/depguard:/,/^    [a-z]/p' .golangci.yml
```

- [ ] **Step 2: 新しいルールに書き換える**

設計書 §3.2 の向きをそのまま写す。`domain-layer` の `encoding/json` 禁止と `github-layer` の `internal/app` 禁止は **PR 2 で入れるのでここには書かない**（この時点では `internal/github` が domain を import しており、落ちてしまう）。

```yaml
      depguard:
        rules:
          # ドメインは何にも依存しない。encoding/json の禁止は PR 2 で足す。
          domain-layer:
            files:
              - "**/internal/app/domain/**"
            deny:
              - pkg: github.com/kukv/octoscope/internal/app/usecase
                desc: ドメインは上の層を知らない
              - pkg: github.com/kukv/octoscope/internal/app/presentation
                desc: ドメインは UI を知らない
              - pkg: github.com/kukv/octoscope/internal/app/config
                desc: ドメインは設定ファイルを知らない
              - pkg: github.com/kukv/octoscope/internal/github
                desc: ドメインは特定サービスのクライアントを知らない
              - pkg: github.com/kukv/octoscope/internal/i18n
                desc: ドメインは翻訳しない
              - pkg: github.com/kukv/octoscope/internal/browser
                desc: ドメインはブラウザを開かない
          # GitHub クライアントはアプリの上位層を知らない。
          # internal/app 全体の禁止は PR 2（domain 非依存化）で足す。
          github-layer:
            files:
              - "**/internal/github/**"
            deny:
              - pkg: github.com/kukv/octoscope/internal/app/usecase
                desc: クライアントは上の層を知らない
              - pkg: github.com/kukv/octoscope/internal/app/presentation
                desc: クライアントは UI を知らない
              - pkg: github.com/kukv/octoscope/internal/app/config
                desc: クライアントは設定ファイルを知らない
          # usecase は画面も翻訳も知らない。
          usecase-layer:
            files:
              - "**/internal/app/usecase/**"
            deny:
              - pkg: github.com/kukv/octoscope/internal/app/presentation
                desc: usecase は UI に依存しない
              - pkg: github.com/kukv/octoscope/internal/i18n
                desc: usecase は翻訳しない。訳すのは UI
          # ビューは usecase を通す。どのクライアントが動いているかを
          # 知っているのは cmd/octoscope だけである。
          presentation-layer:
            files:
              - "**/internal/app/presentation/**"
            deny:
              - pkg: github.com/kukv/octoscope/internal/github
                desc: ビューは usecase を通す。クライアントを名指しするのは cmd/octoscope だけ
              - pkg: github.com/kukv/octoscope/internal/app/config
                desc: 設定は Options で届く。読むのは cmd/octoscope
          # 設定ファイルの読み取りは葉である。
          config-layer:
            files:
              - "**/internal/app/config/**"
            deny:
              - pkg: github.com/kukv/octoscope/internal/app/domain
                desc: config は他のパッケージに依存しない
              - pkg: github.com/kukv/octoscope/internal/app/usecase
                desc: config は他のパッケージに依存しない
              - pkg: github.com/kukv/octoscope/internal/app/presentation
                desc: config は他のパッケージに依存しない
              - pkg: github.com/kukv/octoscope/internal/github
                desc: config は他のパッケージに依存しない
              - pkg: github.com/kukv/octoscope/internal/i18n
                desc: config は他のパッケージに依存しない
              - pkg: github.com/kukv/octoscope/internal/browser
                desc: config は他のパッケージに依存しない
          # メッセージは葉である。
          i18n-layer:
            files:
              - "**/internal/i18n/**"
            deny:
              - pkg: github.com/kukv/octoscope/internal/app
                desc: i18n は他のパッケージに依存しない
              - pkg: github.com/kukv/octoscope/internal/github
                desc: i18n は他のパッケージに依存しない
              - pkg: github.com/kukv/octoscope/internal/browser
                desc: i18n は他のパッケージに依存しない
          # URL を開くことは葉である。
          browser-layer:
            files:
              - "**/internal/browser/**"
            deny:
              - pkg: github.com/kukv/octoscope/internal/app
                desc: browser は他のパッケージに依存しない
              - pkg: github.com/kukv/octoscope/internal/github
                desc: browser は他のパッケージに依存しない
              - pkg: github.com/kukv/octoscope/internal/i18n
                desc: browser は他のパッケージに依存しない
```

- [ ] **Step 3: ルールが実際に効くことを確かめる**

規約は「書いてあるだけ」では守れない。1 つ違反を入れて落ちることを確認し、戻す。

使い捨てのファイルを 1 つ足して落ちることを見る。既存ファイルは触らない。

```bash
cat > internal/app/domain/depguard_probe.go <<'EOF'
package domain

import _ "github.com/kukv/octoscope/internal/i18n"
EOF
make lint
```

期待: depguard が `domain-layer` のルールで落ち、`ドメインは翻訳しない` が出る。

```bash
rm internal/app/domain/depguard_probe.go
make lint
```

期待: 今度は通る。同じことを `github-layer` でも行う。

```bash
cat > internal/github/gql/depguard_probe.go <<'EOF'
package gql

import _ "github.com/kukv/octoscope/internal/app/usecase"
EOF
make lint
rm internal/github/gql/depguard_probe.go
```

期待: 1 回目は `クライアントは上の層を知らない` で落ち、削除後は通る。`git status --porcelain` が空であることを確認してから次へ進む。

- [ ] **Step 4: 検査する**

```bash
make check
```

期待: すべて PASS。

- [ ] **Step 5: コミット**

```bash
git add .golangci.yml
git commit -m "$(cat <<'EOF'
build: point depguard at the new package tree

The two rules that make the domain vendor-blind (no encoding/json, no
internal/app inside internal/github) land in PR 2, where the code can
actually satisfy them.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 7: 規約とドキュメントを新しい構成に合わせる

**Files:**
- Modify: `.claude/rules/architecture.md`（依存の図、各節のパス、新しい規則の追加）
- Modify: `.claude/rules/tui.md`（frontmatter の `paths`、109 行目の `internal/tui/theme`）
- Modify: `.claude/rules/testing.md`（62 行目の `internal/tui/app`）
- Modify: `docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md` §3.1 / §3.2、`docs/superpowers/specs/2026-09-08-phase4-design.md` §7（置き換え済みであることの追記）

**Interfaces:**
- Consumes: Task 1〜6 の構成
- Produces: なし（ドキュメントのみ）

- [ ] **Step 1: `.claude/rules/tui.md` の paths と本文を直す**

frontmatter:

```yaml
paths:
  - "internal/app/presentation/tui/**"
  - "internal/i18n/**"
```

109 行目付近の `internal/tui/theme` を `internal/app/presentation/tui/theme` にする。それ以外は触らない。

- [ ] **Step 2: `.claude/rules/testing.md` の 62 行目を直す**

`internal/tui/app` を `internal/app/presentation/tui/root` にする。frontmatter の `paths`（`internal/**/*.go` と `**/testdata/**`）はそのままで新しいツリーにも当たるので変えない。

- [ ] **Step 3: `.claude/rules/errors.md` と `go-style.md` を確認する**

```bash
grep -n 'internal/' .claude/rules/errors.md .claude/rules/go-style.md
```

期待: 出力なし。出たら直す。frontmatter は両方 `**/*.go` なので変更不要。

- [ ] **Step 4: `.claude/rules/architecture.md` の「依存の向き」を差し替える**

設計書 §3.2 の図をそのまま入れ、次の 3 点を本文に書く。

- gateway は usecase を import せずに port を満たす（Go の暗黙 interface）。結線は `cmd/octoscope`
- `internal/github` がこの時点ではまだ `internal/app/domain` を import していること、それを断つのは PR 2 であること
- depguard に新しいパッケージを足すのを忘れないこと（既存の文を新しいパス名で残す）

- [ ] **Step 5: `architecture.md` の各節のパス参照を直す**

`internal/tui` → `internal/app/presentation/tui`、`internal/gh` → 文脈に応じて `internal/app/domain` か `internal/github`、`internal/usecase` → `internal/app/usecase`。

次の節は**今回は本文を変えない**（PR 2 / PR 3 で改訂する）。パス名だけ直す。

- 「複数の API 呼び出しは `internal/usecase` に置く」 → PR 3 で §6 のとおり改訂
- 「GitHub API 固有の値はパッケージの外に出さない」 → PR 2 で機械検査に強化
- 「層を足す前に」 → PR 2 で今回の判断と costs を追記

- [ ] **Step 6: `architecture.md` に「名前は借りてよい、形は借りない」を追加する**

設計書 §5 の規則を節として写す。適用の表も入れる。

- [ ] **Step 7: 旧設計書に置き換えの注記を入れる**

`docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md` の §3.1 と §3.2 の冒頭、
`docs/superpowers/specs/2026-09-08-phase4-design.md` の §7 の冒頭に、それぞれ 1 行:

```markdown
> **この節は `2026-09-13-architecture-restructure-design.md` に置き換えられた（2026-09-13）。**
```

本文は消さない。当時の判断の記録として残す。

- [ ] **Step 8: 規約が新しいツリーで読み込まれることを確認する**

`internal/app/presentation/tui/` 配下のファイルを 1 つ開き、`/context` で `tui.md` が読み込まれていることを確認する。読み込まれなければ frontmatter の `paths` が合っていない。

- [ ] **Step 9: 検査する**

```bash
make check
```

期待: すべて PASS（ドキュメントのみの変更なので当然通る。壊れていないことの確認）。

- [ ] **Step 10: コミット**

```bash
git add .claude/rules docs/superpowers/specs
git commit -m "$(cat <<'EOF'
docs: point the rules at the new package tree

Rules and depguard have to move with the code: a convention nobody
updated is one the next session will not find.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## PR 1 の完了条件

1. `internal/` が計画冒頭の「ファイル構成（移動後）」のとおりになっている
2. `make check` が通る
3. `make release-check` が通る（3 OS のクロスコンパイル）
4. `git log` のどのコミットでも `make check` が通る
5. golden ファイルと testdata の**中身**が 1 バイトも変わっていない（rename は可）

```bash
git diff main...HEAD --stat -M -- '*testdata*'
```

期待: rename の行だけが出て、`+`/`-` の行数がすべて 0。

6. `go run ./cmd/octoscope` と `--lang ja` で画面が再構成前と同じ
7. `.claude/rules/` と `.golangci.yml` が新しいパスを指している
8. 振る舞いの変更が diff に 1 行も入っていない

```bash
git diff main...HEAD -- '*.go' | grep '^[+-]' | grep -v '^[+-][+-]' \
  | grep -vE 'kukv/octoscope/internal|^[+-]package |domain\.|root\.|gh\.|app\.'
```

期待: パッケージ doc の書き換え（Task 4 Step 4、Task 5 Step 5）とコメントの修正（Task 5 Step 7）以外は出ないこと。他が出たら、それは振る舞いの変更かもしれないので確認する。
