# アーキテクチャ再構成 PR 2a（datasource の導入と `SavedQuery` の domain 化）実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** アプリのデータ（リポジトリ一覧・保存クエリ）の永続化を `internal/app/adapter/datasource` に移し、`SavedQuery` を domain の型にする。`internal/app/config` は設定ファイルの**形**だけを持つ。

**Architecture:** 設定ファイルは 1 つのままで、読み書きの責務を 2 つに割る。`config` が YAML の構造体とファイルの読み取りを持ち、`datasource` がそれを使って domain 型で出し入れする。`usecase` の port は `domain.SavedQuery` を受け取るようになり、そのために置かれていた `usecase.SavedQuery` の re-export が消える。gateway はこの PR では作らない。

**Tech Stack:** Go 1.x、`go.yaml.in/yaml/v3`、golangci-lint（depguard / gofumpt / goimports）、gotestsum

**Spec:** `docs/superpowers/specs/2026-09-13-architecture-restructure-design.md`（§10 の「PR 2a」）

## Global Constraints

- **画面の見た目を変えない。** golden ファイルの中身が変わったら、それは設計の失敗であって再録の理由ではない。`make golden` を実行しない
- **設定ファイルの形式（YAML のキー）を変えない。** 既存のユーザーの `config.yaml` がそのまま読めること。キーは `language` / `icons` / `default_tab` / `repositories` / `saved_queries`、`saved_queries` の要素は `name` と `query`
- **`domain` は何も import しない。** `domain.SavedQuery` に yaml タグを付けない（PR 2b で「domain に struct タグが 1 つでもあれば落ちる」テストが入る。ここで付けると、そのテストが最初から余計な仕事を抱える）
- `internal/github` はこの PR では一切触らない
- gateway（`internal/app/adapter/gateway`）はこの PR では作らない
- モジュールパスは `github.com/kukv/octoscope`
- コミットメッセージの末尾に `Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>` を付ける

## 実測値（2026-09-13、着手時点、main = a5941e6）

| 対象 | 値 |
|---|---|
| `SavedQuery` を名指しするファイル（config 以外） | 12 |
| `internal/app/config/config.go` | 149 行 |
| `internal/app/config/config_test.go` | 298 行 |
| `usecase` の port 宣言のうち config を見るもの | `queryStore.SaveQueries(queries []config.SavedQuery) error` の 1 つ |

## いまの形

`internal/app/config` に 2 つの役割が同居している。

| 内容 | 性質 | 行き先 |
|---|---|---|
| `Config.Language` `Icons` `DefaultTab`、`DefaultTabName()`、`Path()`、`Load()` | 起動時に 1 回読むだけの設定とファイルの形 | `config` に残す |
| `Config.Repositories` `Config.SavedQueries`、`Store`、`SaveRepositories`、`SaveQueries`、`save` | アプリが実行中に書き戻すデータ | `datasource` へ |

`SavedQuery` は今 `config` が定義し、`usecase` が同名の型で re-export し、`cmd/octoscope` が
`usecase.SavedQueriesFrom(cfg.SavedQueries)` で変換して `root.Options` に渡している。
この 3 段は「tui が config を見られない」ためだけに存在する。`domain.SavedQuery` ができれば
1 段で済む。

## 移動後の形

```
internal/app/
  domain/
    domain.go          ← SavedQuery を足す（yaml タグなし）
  config/
    config.go          ← Config（YAML の形）/ Path / Load / DefaultTabName
                          SavedQueries の要素型は config 自身の yaml タグ付き型
  adapter/datasource/
    datasource.go      ← Store: 設定ファイルを介した読み書き。domain 型で出し入れする
  usecase/
    usecase.go         ← queryStore が domain.SavedQuery を受ける
    search.go          ← SavedQuery と SavedQueriesFrom を削除
  presentation/tui/    ← domain.SavedQuery を使う
cmd/octoscope/         ← datasource.NewStore を組む
```

**`datasource` は `config` と `domain` を import する。** `config` は今までどおり何も
import しない。向きは下から上へ流れない。

## 検証の考え方

設定ファイルの読み書きは、いまも `config_test.go` が 298 行で覆っている。移動によって
その担保が薄くならないことが第一の条件である。加えて、この PR は**型の付け替えだけでなく
責務の移動**なので、次の 2 つは新しくテストを書く。

1. `datasource` が保存したものを `config` が読み戻せること（同じファイルを 2 つの
   パッケージが触るので、ここが割れると設定が静かに壊れる）
2. 片方だけを保存しても、もう片方と起動時設定が消えないこと（既存の `Store` の
   「他の設定を潰さない」という性質を、移動先でも持っていること）

---

### Task 1: `domain.SavedQuery` を足す

**Files:**
- Modify: `internal/app/domain/domain.go`
- Test: `internal/app/domain/domain_test.go`

**Interfaces:**
- Consumes: なし
- Produces: `domain.SavedQuery{Name, Query string}`。**yaml タグも json タグも付けない**

- [ ] **Step 1: 失敗するテストを書く**

`internal/app/domain/domain_test.go` に足す。この型に振る舞いは無いので、テストが
確かめるのは「タグを持たない」という制約そのものである。PR 2b で入る全型向けの
reflect テストの、この型に限った先取りになる。

```go
func TestSavedQueryCarriesNoSerialisationTags(t *testing.T) {
	// The settings file's shape belongs to internal/app/config; this type
	// is what the application is written in terms of. A tag here would mean
	// the two had been merged back together.
	typ := reflect.TypeOf(domain.SavedQuery{})
	for i := range typ.NumField() {
		if tag := typ.Field(i).Tag; tag != "" {
			t.Errorf("SavedQuery.%s carries a struct tag %q", typ.Field(i).Name, tag)
		}
	}
}
```

`reflect` の import を足すこと。テストファイルのパッケージは `domain_test` である。

- [ ] **Step 2: 落ちることを確かめる**

```bash
go test ./internal/app/domain/ -run TestSavedQueryCarriesNoSerialisationTags -v
```

期待: コンパイルに失敗する（`undefined: domain.SavedQuery`）。

- [ ] **Step 3: 型を足す**

`internal/app/domain/domain.go` の、`RepoCandidate` の定義のあとに置く。

```go
// SavedQuery is one of the Search tab's saved queries: what the user called
// it, and the search it stands for. The query is the service's own search
// syntax, which is why it is a string and not a parsed structure.
type SavedQuery struct {
	Name  string
	Query string
}
```

- [ ] **Step 4: 通ることを確かめる**

```bash
go test ./internal/app/domain/ -run TestSavedQueryCarriesNoSerialisationTags -v
```

期待: PASS。

- [ ] **Step 5: コミット**

```bash
make fmt
make check
git add internal/app/domain/
git commit -m "$(cat <<'EOF'
feat: give the domain a saved query

The Search tab's saved queries are the application's data, not a detail
of the file they happen to be written to.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: `datasource` パッケージを作る

`config` からは何も消さない。まず新しい置き場所を、テストごと立てる。

**Files:**
- Create: `internal/app/adapter/datasource/datasource.go`
- Create: `internal/app/adapter/datasource/datasource_test.go`
- Modify: `.golangci.yml`（depguard に `datasource-layer` を足す）

**Interfaces:**
- Consumes: Task 1 の `domain.SavedQuery`、既存の `config.Load` / `config.Config`
- Produces:
  - `datasource.Store`、`datasource.NewStore(path string) *Store`
  - `(*Store) Repositories() ([]string, error)`
  - `(*Store) SavedQueries() ([]domain.SavedQuery, error)`
  - `(*Store) SaveRepositories(repos []string) error`
  - `(*Store) SaveQueries(queries []domain.SavedQuery) error`

- [ ] **Step 1: 失敗するテストを書く**

`internal/app/adapter/datasource/datasource_test.go`。パッケージは `datasource_test`。

```go
package datasource_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kukv/octoscope/internal/app/adapter/datasource"
	"github.com/kukv/octoscope/internal/app/config"
	"github.com/kukv/octoscope/internal/app/domain"
)

// write puts raw into a settings file inside a temporary directory and
// returns its path.
func write(t *testing.T, raw string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSavedQueriesComeBackAsDomainValues(t *testing.T) {
	path := write(t, "saved_queries:\n  - name: mine\n    query: is:open author:@me\n")
	got, err := datasource.NewStore(path).SavedQueries()
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.SavedQuery{{Name: "mine", Query: "is:open author:@me"}}
	if len(got) != len(want) || got[0] != want[0] {
		t.Errorf("SavedQueries() = %v, want %v", got, want)
	}
}

func TestRepositoriesComeBackInFileOrder(t *testing.T) {
	path := write(t, "repositories:\n  - kukv/octoscope\n  - kukv/koto\n")
	got, err := datasource.NewStore(path).Repositories()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "kukv/octoscope" || got[1] != "kukv/koto" {
		t.Errorf("Repositories() = %v, want [kukv/octoscope kukv/koto]", got)
	}
}

// A settings file that is not there is not a failure: a first run has none.
func TestAMissingFileReadsAsNothingSaved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	s := datasource.NewStore(path)
	repos, err := s.Repositories()
	if err != nil {
		t.Fatalf("Repositories() on a missing file: %v", err)
	}
	if len(repos) != 0 {
		t.Errorf("Repositories() = %v, want none", repos)
	}
	queries, err := s.SavedQueries()
	if err != nil {
		t.Fatalf("SavedQueries() on a missing file: %v", err)
	}
	if len(queries) != 0 {
		t.Errorf("SavedQueries() = %v, want none", queries)
	}
}

// This is the test the split exists for: two packages now touch one file,
// and what one writes the other has to be able to read.
func TestSavingQueriesLeavesTheStartupSettingsAlone(t *testing.T) {
	path := write(t, "language: ja\nicons: nerdfont\ndefault_tab: search\nrepositories:\n  - kukv/octoscope\n")
	if err := datasource.NewStore(path).SaveQueries([]domain.SavedQuery{{Name: "mine", Query: "is:open"}}); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Language != "ja" {
		t.Errorf("language = %q, want ja", cfg.Language)
	}
	if cfg.Icons != "nerdfont" {
		t.Errorf("icons = %q, want nerdfont", cfg.Icons)
	}
	if cfg.DefaultTabName() != "search" {
		t.Errorf("default tab = %q, want search", cfg.DefaultTabName())
	}
	repos, err := datasource.NewStore(path).Repositories()
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 || repos[0] != "kukv/octoscope" {
		t.Errorf("repositories = %v, want [kukv/octoscope]", repos)
	}
}

func TestSavingRepositoriesLeavesTheQueriesAlone(t *testing.T) {
	path := write(t, "saved_queries:\n  - name: mine\n    query: is:open\n")
	if err := datasource.NewStore(path).SaveRepositories([]string{"kukv/koto"}); err != nil {
		t.Fatal(err)
	}
	queries, err := datasource.NewStore(path).SavedQueries()
	if err != nil {
		t.Fatal(err)
	}
	if len(queries) != 1 || queries[0].Name != "mine" {
		t.Errorf("saved queries = %v, want the one that was there", queries)
	}
}

// main builds a store even when the config directory could not be located,
// so that the failure surfaces on the first save rather than as a save that
// silently does nothing.
func TestAStoreWithNoPathReportsItRatherThanSavingNothing(t *testing.T) {
	if err := datasource.NewStore("").SaveRepositories([]string{"kukv/octoscope"}); err == nil {
		t.Error("SaveRepositories() with no path returned no error")
	}
	if err := datasource.NewStore("").SaveQueries(nil); err == nil {
		t.Error("SaveQueries() with no path returned no error")
	}
}
```

- [ ] **Step 2: 落ちることを確かめる**

```bash
go test ./internal/app/adapter/datasource/ -v
```

期待: ビルドに失敗する（パッケージが無い）。

- [ ] **Step 3: `datasource` を書く**

`internal/app/adapter/datasource/datasource.go`。**`config.Store` の中身をコピーせず、
`config` の `Load` と、この PR ではまだ `config` に残っている `Store.save` の考え方を
引き継いだ実装を書く。** 書き込みは一時ファイル経由（途中で中断されても設定ファイルが
半端な状態で残らないため）。

```go
// Package datasource reads and writes the application's own data -- the
// repositories the Repos tab lists and the Search tab's saved queries --
// through whatever store it is pointed at. Today that store is the settings
// file, whose shape internal/app/config owns; this package translates
// between that shape and the domain's.
package datasource

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	yaml "go.yaml.in/yaml/v3"

	"github.com/kukv/octoscope/internal/app/config"
	"github.com/kukv/octoscope/internal/app/domain"
)

// Store reads and writes one settings file.
type Store struct{ path string }

// NewStore returns a store for the settings file at path. An empty path is
// allowed: main builds a store even when it could not locate the config
// directory, so that the failure surfaces on the first save rather than as a
// save that silently does nothing.
func NewStore(path string) *Store { return &Store{path: path} }

// Repositories is the list the Repos tab shows, in the order it shows them.
func (s *Store) Repositories() ([]string, error) {
	c, err := config.Load(s.path)
	if err != nil {
		return nil, err
	}
	return c.Repositories, nil
}

// SavedQueries is the Search tab's saved queries, in the order the user
// saved them.
func (s *Store) SavedQueries() ([]domain.SavedQuery, error) {
	c, err := config.Load(s.path)
	if err != nil {
		return nil, err
	}
	out := make([]domain.SavedQuery, len(c.SavedQueries))
	for i, q := range c.SavedQueries {
		out[i] = domain.SavedQuery{Name: q.Name, Query: q.Query}
	}
	return out, nil
}

// SaveRepositories replaces the repository list and leaves every other
// setting as it was. A file that cannot be parsed is not written at all: a
// list is not worth flattening the rest of someone's settings for.
func (s *Store) SaveRepositories(repos []string) error {
	c, err := s.load()
	if err != nil {
		return err
	}
	c.Repositories = repos
	return s.save(c)
}

// SaveQueries replaces the saved queries and leaves every other setting as
// it was, for the same reason SaveRepositories does.
func (s *Store) SaveQueries(queries []domain.SavedQuery) error {
	c, err := s.load()
	if err != nil {
		return err
	}
	entries := make([]config.SavedQueryEntry, len(queries))
	for i, q := range queries {
		entries[i] = config.SavedQueryEntry{Name: q.Name, Query: q.Query}
	}
	c.SavedQueries = entries
	return s.save(c)
}

// load is the read every write starts from, so that a write replaces one
// setting rather than the file.
func (s *Store) load() (config.Config, error) {
	if s.path == "" {
		return config.Config{}, errors.New("no settings file to write: the config directory could not be located")
	}
	return config.Load(s.path)
}

// save writes through a temporary file in the same directory so that an
// interrupted write cannot leave a half-written settings file behind.
func (s *Store) save(c config.Config) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	raw, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("encode %s: %w", s.path, err)
	}
	tmp, err := os.CreateTemp(dir, "config-*.yaml")
	if err != nil {
		return fmt.Errorf("create a temporary file in %s: %w", dir, err)
	}
	name := tmp.Name()
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		os.Remove(name)
		return fmt.Errorf("write %s: %w", name, err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return fmt.Errorf("close %s: %w", name, err)
	}
	if err := os.Rename(name, s.path); err != nil {
		os.Remove(name)
		return fmt.Errorf("replace %s: %w", s.path, err)
	}
	return nil
}
```

**注意:** `config.SavedQueryEntry` は次の Task 3 で導入する名前である。このタスクの
時点では `config.SavedQuery` なので、まずそちらで書いてテストを通し、Task 3 で
名前を変える。あるいは Task 3 を先に済ませてもよい — **その場合はこの計画の
Task 2 と Task 3 を入れ替えたことを報告に書くこと。**

実際に `config.Store.save` を読んで、一時ファイルの扱いを写し違えていないか確かめる
（`internal/app/config/config.go` の `save`）。

- [ ] **Step 4: 通ることを確かめる**

```bash
go test ./internal/app/adapter/datasource/ -v
```

期待: 6 本すべて PASS。

- [ ] **Step 5: depguard に `datasource-layer` を足す**

`.golangci.yml` の `depguard.rules` に足す。置く位置は `config-layer` の直後。

```yaml
          # データの出し入れは adapter である。UI も GitHub クライアントも知らない。
          datasource-layer:
            files:
              - "**/internal/app/adapter/datasource/**"
            deny:
              - pkg: github.com/kukv/octoscope/internal/app/usecase
                desc: datasource は上の層を知らない
              - pkg: github.com/kukv/octoscope/internal/app/presentation
                desc: datasource は UI を知らない
              - pkg: github.com/kukv/octoscope/internal/github
                desc: datasource は GitHub クライアントを知らない
              - pkg: github.com/kukv/octoscope/internal/i18n
                desc: datasource は翻訳しない
```

あわせて `presentation-layer` の deny に 1 行足す。ビューが store を直接触らないため。

```yaml
              - pkg: github.com/kukv/octoscope/internal/app/adapter
                desc: 保存は usecase を通す。store を組むのは cmd/octoscope だけ
```

- [ ] **Step 6: ルールが効くことを確かめる**

書いただけでは信じない。使い捨てのファイルを置いて、実際に落ちるのを見る。

```bash
cat > internal/app/adapter/datasource/depguard_probe.go <<'EOF'
package datasource

import "github.com/kukv/octoscope/internal/i18n"

var _ = i18n.T
EOF
make lint
```

期待: `datasource は翻訳しない` で落ちる。

```bash
rm internal/app/adapter/datasource/depguard_probe.go
make lint
```

期待: 通る。`git status --porcelain` が空であることを確認する。

- [ ] **Step 7: コミット**

```bash
make fmt
make check
git add internal/app/adapter/datasource/ .golangci.yml
git commit -m "$(cat <<'EOF'
feat: put the application's own data behind a datasource

The repository list and the saved queries are the application's data;
the settings file is only where they happen to live. This package is the
seam between the two, and it hands out domain values.

config.Store still exists and is still what main uses; the next commit
moves the callers over.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: `config` から書き込みを外し、`SavedQuery` を `SavedQueryEntry` に改名する

**Files:**
- Modify: `internal/app/config/config.go`（`Store` `NewStore` `SaveRepositories` `SaveQueries` `save` を削除、`SavedQuery` を `SavedQueryEntry` に改名）
- Modify: `internal/app/config/config_test.go`（削除した API のテストを外す）
- Modify: `internal/app/adapter/datasource/datasource.go`（Task 2 のメモのとおり `config.SavedQueryEntry` に合わせる）
- Modify: `.golangci.yml`（gosec の `G304` 除外が `config.go` を指している。`Load` は残るので位置は変わらないはずだが、確認する）

**Interfaces:**
- Consumes: Task 2 の `datasource.Store`
- Produces: `config.Config`（`Language` `Icons` `DefaultTab` `Repositories` `SavedQueries []SavedQueryEntry`）、`config.SavedQueryEntry{Name, Query string}`（yaml タグ付き）、`config.Path`、`config.Load`、`Config.DefaultTabName`。**`config.Store` はもう無い**

- [ ] **Step 1: いま何が config を使っているかを数える**

```bash
grep -rn 'config\.Store\|config\.NewStore\|config\.SavedQuery' --include='*.go' internal cmd
```

出た場所を控える。Task 4 / 5 で全部付け替える。

- [ ] **Step 2: `config.SavedQuery` を `config.SavedQueryEntry` に改名する**

`internal/app/config/config.go`:

```go
// SavedQueryEntry is one entry of saved_queries as the file spells it. The
// application's own type is domain.SavedQuery; this one exists to carry the
// yaml tags, which the domain does not have.
type SavedQueryEntry struct {
	Name  string `yaml:"name"`
	Query string `yaml:"query"`
}
```

`Config.SavedQueries` の型も `[]SavedQueryEntry` にする。

- [ ] **Step 3: `Store` とその 3 メソッドを削除する**

`internal/app/config/config.go` から `Store`、`NewStore`、`SaveRepositories`、
`SaveQueries`、`save` を消す。`save` だけが使っていた import（`path/filepath`、
`yaml` の Marshal 側など）が余るので、`make fmt` の前に自分で確認して外す。

パッケージ doc も直す。書き込みをしなくなったので、現在の 1〜3 行目
（「the settings file that persists ...」）を、読むだけであることが分かる文にする。

```go
// Package config is the shape of the settings file and how to read it. What
// the application does with the data inside is not this package's business:
// internal/app/adapter/datasource owns that, and writes the file back.
package config
```

- [ ] **Step 4: `config_test.go` から、消した API のテストを外す**

**移すのではなく外す。** 同等の担保は Task 2 で `datasource_test.go` に書いてある。
`Load` / `Path` / `DefaultTabName` のテストはすべて残す。

外したテストの名前を控えて、報告に「これは datasource 側のどのテストが引き継いだか」を
1 行ずつ書くこと。引き継ぎ先の無いものがあれば、それは担保が消えているので `datasource_test.go`
に足す。

- [ ] **Step 5: `datasource` を新しい名前に合わせる**

Task 2 で `config.SavedQuery` と書いた箇所を `config.SavedQueryEntry` にする。

- [ ] **Step 6: 検査する**

```bash
make fmt
go build ./... 2>&1 | head -20
```

期待: `cmd/octoscope` と `internal/app/usecase` が `config.Store` / `config.SavedQuery` を
見つけられずに落ちる。**これは想定どおり** — Task 4 / 5 で直す。ここでコミットしない。

- [ ] **Step 7: Task 4 へ進む**

このタスクは単独ではビルドが通らない。Task 3〜5 で 1 コミットにする。

---

### Task 4: `usecase` の port を `domain.SavedQuery` にし、re-export を消す

**Files:**
- Modify: `internal/app/usecase/usecase.go`（`queryStore` の署名、`config` の import を外す）
- Modify: `internal/app/usecase/search.go`（`SavedQuery` と `SavedQueriesFrom` を削除、`SaveQueries` の署名）
- Modify: `internal/app/usecase/usecase_test.go`

**Interfaces:**
- Consumes: Task 1 の `domain.SavedQuery`
- Produces: `(*Usecase) SaveQueries(queries []domain.SavedQuery) error`。**`usecase.SavedQuery` と `usecase.SavedQueriesFrom` はもう無い**

- [ ] **Step 1: `queryStore` の署名を変える**

`internal/app/usecase/usecase.go`:

```go
// queryStore is where the Search tab's saved queries survive a restart.
type queryStore interface {
	SaveQueries(queries []domain.SavedQuery) error
}
```

`config` の import が不要になるので外す。`settingsStore` はそのまま
（`repoStore` と `queryStore` を embed している）。

- [ ] **Step 2: `search.go` から re-export を消す**

`SavedQuery` 型と `SavedQueriesFrom` 関数を削除し、`SaveQueries` を書き直す。

```go
// SaveQueries writes the Search tab's saved queries to the store.
func (u *Usecase) SaveQueries(queries []domain.SavedQuery) error {
	return u.queryStore.SaveQueries(queries)
}
```

`config` の import が余るので外す。

- [ ] **Step 3: `usecase_test.go` を直す**

```bash
grep -n 'SavedQuery\|config\.' internal/app/usecase/usecase_test.go
```

出た箇所の `usecase.SavedQuery` / `config.SavedQuery` を `domain.SavedQuery` に変える。
フェイクの `SaveQueries` の署名も変わる。

`internal/app/usecase/usecase_test.go:461` 付近に「`internal/app/config` is not visible
from `internal/app/presentation/tui`, so the ...」という趣旨のコメントがある
（re-export が存在した理由の説明）。**その理由がこの PR で消えるので、コメントも消す。**

- [ ] **Step 4: 検査する**

```bash
go build ./internal/app/... 2>&1 | head
```

期待: `internal/app/presentation/tui` と `cmd/octoscope` がまだ落ちる。Task 5 で直す。

---

### Task 5: tui と `cmd/octoscope` を付け替える

**Files:**
- Modify: `internal/app/presentation/tui/root/root.go`（`Options.SavedQueries`）
- Modify: `internal/app/presentation/tui/search/{search.go, saved.go, saved_render.go}`
- Modify: それぞれのテスト（`root_test.go` `scenario_test.go` `search_test.go` `saved_test.go` `render_test.go`）
- Modify: `cmd/octoscope/main.go`

**Interfaces:**
- Consumes: Task 1 の `domain.SavedQuery`、Task 2 の `datasource.NewStore`、Task 4 の `usecase.SaveQueries`
- Produces: `root.Options.SavedQueries []domain.SavedQuery`

- [ ] **Step 1: tui 側の型を付け替える**

```bash
grep -rn 'usecase\.SavedQuery' --include='*.go' internal/app/presentation
```

出たものを `domain.SavedQuery` にする。`domain` の import を足し、その結果
`usecase` の import が要らなくなったファイルからは外す（`make fmt` は未使用 import を
消さないので自分で確認する）。

- [ ] **Step 2: `cmd/octoscope/main.go` を組み替える**

いまの形（62〜85 行目あたり）:

```go
store := config.NewStore(path)
...
p := tea.NewProgram(root.New(uc, root.Options{
	Repo:         *repoFlag,
	Repositories: cfg.Repositories,
	SavedQueries: usecase.SavedQueriesFrom(cfg.SavedQueries),
	...
```

新しい形:

```go
	// The store is built even when config.Path failed: it reports that
	// failure when something is saved, rather than saving nothing in silence.
	store := datasource.NewStore(path)
	var uc *usecase.Usecase
	if ghClient != nil {
		uc = usecase.New(ghClient, store)
	} else {
		uc = usecase.New(apiClient, store)
	}

	// The lists come from the store rather than out of cfg: the file's shape
	// is config's business, and what the application keeps in it is the
	// store's. A read failure here is the same one config.Load already put in
	// configErr, so it is not reported a second time; the run carries on with
	// nothing saved, which is what a first run does anyway.
	repos, _ := store.Repositories()
	queries, _ := store.SavedQueries()

	p := tea.NewProgram(root.New(uc, root.Options{
		Repo:         *repoFlag,
		Repositories: repos,
		SavedQueries: queries,
		DefaultTab:   cfg.DefaultTabName(),
		ConfigError:  configErr,
	}))
```

**なぜエラーを捨てているのか**を確かめること。`main.go` はすでに 41〜48 行目で
`config.Load(path)` を 1 回呼び、その失敗を `configErr` に入れている。`store` の 2 つの
読み取りは同じファイルを読むので、失敗するなら同じ失敗である。ここで返すと、
タブ行と終了時の stderr に同じ文言が 3 回出る。**捨てるのはそのためで、
「エラー処理が面倒だから」ではない。** コメントにその理由を書くこと。

`cfg` は `Language` / `Icons` / `DefaultTabName()` のためにまだ要る。
`config` の import も `Path` / `Load` のために残る。

- [ ] **Step 3: テストを直す**

```bash
go vet ./... 2>&1 | head -20
```

出たものを順に直す。型の付け替えだけで、テストが確かめている内容は変えない。

- [ ] **Step 4: 全部通ることを確かめる**

```bash
make fmt
make check
```

期待: すべて PASS。テスト本数が着手前と同じか、Task 1 / 2 で足した 7 本ぶん増えていること。

- [ ] **Step 5: golden が無傷であることを確かめる**

```bash
git status --porcelain | grep -E '\.golden|testdata' || echo "golden と testdata は無変更"
```

期待: `golden と testdata は無変更`。

- [ ] **Step 6: 実際に動かして設定ファイルを確かめる**

**ここは省略できない。** 型が通ることと、設定が正しく読み書きされることは別である。

設定ファイルは Linux では `~/.config/octoscope/config.yaml` にある。**先に退避する。**
このタスクは書き込み経路を触っているので、壊れたときに戻せる状態で試すこと。

```bash
cp ~/.config/octoscope/config.yaml ~/octoscope-config-backup.yaml 2>/dev/null || echo "設定ファイルはまだ無い"
cat ~/.config/octoscope/config.yaml 2>/dev/null
go run ./cmd/octoscope --repo kukv/octoscope
```

確かめること:

1. Repos タブに、設定ファイルにあるリポジトリが並ぶ
2. Search タブに、保存済みクエリが並ぶ
3. リポジトリを 1 つ足して終了し、設定ファイルを開いて `repositories` に増えていること、
   **`language` / `icons` / `default_tab` / `saved_queries` が消えていないこと**
4. クエリを 1 つ保存して終了し、同じく他のキーが残っていること
5. `--lang ja` でも同じ

3 と 4 が、この PR で一番壊れやすいところである（2 つのパッケージが 1 つのファイルを
触るようになったため）。

- [ ] **Step 7: コミット**

Task 3〜5 をまとめて 1 コミットにする。

```bash
git add -A
git commit -m "$(cat <<'EOF'
refactor: move the settings file's write side to the datasource

config now only says what the file looks like and how to read it. What
the application keeps in it -- the repository list and the saved queries
-- goes through the store, in domain values.

usecase.SavedQuery went with it. That type existed only because the views
could not see internal/app/config; domain.SavedQuery is visible to both,
so the three-step handover from the file to the screen is now one.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: 規約を新しい形に合わせる

**Files:**
- Modify: `.claude/rules/architecture.md`

**Interfaces:**
- Consumes: Task 1〜5
- Produces: なし

- [ ] **Step 1: 依存の図の注記を直す**

PR 1 で「`adapter/gateway/gh` と `adapter/datasource` はまだ無い（PR 2 / PR 3 で作る）」と
書いた 1 文がある。**`datasource` はこの PR で実在するようになった**ので、その注記を
`gateway` だけに掛かるように直す。

- [ ] **Step 2: 「層を足す前に」に datasource の記録を足す**

この節は「層を足したくなったら、無いと何が壊れるか / 足すと何が減るか / 何が増えるかを
書けるか確かめる」と求めている。`datasource` について実際に書く。

- **無いと何が壊れるか（実測）:** `SavedQuery` が `config`（ファイル形式）→ `usecase`
  （再定義）→ `root.Options` の 3 段を経由していた。tui が `config` を見られないためだけの
  中継で、変換関数 `SavedQueriesFrom` が `cmd/octoscope` の起動処理に置かれていた
- **足すと何が減るか:** その 3 段が 1 段になり、`usecase.SavedQuery` と
  `SavedQueriesFrom` が消えた。設定の保存先を変えるときに触るのは `datasource` だけになる
- **足すと何が増えるか:** 設定ファイルを触るパッケージが 1 つから 2 つになった。
  `config` が形を持ち、`datasource` が書き戻す。両者が食い違うと設定が静かに壊れるので、
  `datasource_test.go` に「片方を保存しても、もう片方と起動時設定が消えない」テストを置いた

- [ ] **Step 3: 検査してコミット**

```bash
make check
git add .claude/rules/architecture.md
git commit -m "$(cat <<'EOF'
docs: record why the datasource earned its place

The rules ask for three things before a layer goes in. This writes them
down for this one, with what it cost.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## PR 2a の完了条件

1. `internal/app/adapter/datasource` が存在し、`domain.SavedQuery` で出し入れする
2. `internal/app/config` に書き込み API が無い（`Store` `NewStore` `SaveRepositories` `SaveQueries` が消えている）
3. `usecase.SavedQuery` と `usecase.SavedQueriesFrom` が消えている
4. `internal/app/usecase` が `internal/app/config` を import していない

```bash
grep -rn 'app/config' --include='*.go' internal/app/usecase internal/app/presentation
```

期待: 出力なし。

5. depguard に `datasource-layer` があり、実際に落ちることを確認済み
6. `make check` と `make release-check` が通る
7. golden ファイルと testdata の中身が変わっていない
8. **既存の `config.yaml` がそのまま読め、片方の保存でもう片方が消えない**（Task 5 Step 6 を実施済み）
9. `internal/github` に一切変更が無い

```bash
git diff main...HEAD --stat -- internal/github
```

期待: 出力なし。
