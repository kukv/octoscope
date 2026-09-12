# 保存クエリ 実装計画（Phase 4 スライス 3-3）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Search タブで組んだクエリに名前を付けて `s` で保存し、`Ctrl+O` のポップアップから
呼び出す。保存したものは設定ファイルに残り、次の起動でも使える。`x` で消せる。
`default_tab: search` で起動時に Search タブを開ける。

**Architecture:** 保存先は `internal/config`（`saved_queries`）。書き込みは Repos の
`SaveRepositories` と同じ形で、Search のサブモデルが宣言する小さな interface を
`internal/usecase` が満たす。ポップアップは `internal/tui/search` の中に置き、
`internal/tui/dialog` とは**型を共有せず、箱の寸法だけ `internal/tui/layout` に出して共有する**。

**Tech Stack:** Go / Bubble Tea v2（`charm.land/*/v2`）/ `go.yaml.in/yaml/v3`

**Spec:**
- `docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md` §4.3（Search タブ）・§5（設定ファイル）
- `docs/superpowers/specs/2026-09-08-phase4-design.md` §4（ダイアログ）・§5（Search タブ）・§7（境界）・§8（テスト）・§9（幅）
- 前スライスの積み残し: `docs/superpowers/2026-09-12-phase4-search-tab-followups.md`

---

## Global Constraints

- Bubble Tea 系の import は `charm.land/*/v2`。`github.com/charmbracelet/bubbletea/v2` は壊れている
- 画面に出す文字列は `internal/i18n` から引く。新しい ID は `active.en.yaml` と `active.ja.yaml` の
  **両方**に足す。**GitHub の検索構文（`is:open`、`label:`）は翻訳しない**
- 桁は `ansi.StringWidth` で数える。`len` も `utf8.RuneCountInString` も使わない
- 色は `internal/tui/theme` からだけ引く。16 進の色をビューに書かない
- ネットワークも外部プロセスも実際には叩かない。設定ファイルは `t.TempDir()` に読み書きする
- 先に失敗するテストを書く。書いた直後に検証対象を一時的に壊し、**落ちることを目で見てから**コミットする
- コメントは英語。書くのは「外部の事情」「一見おかしいコードが正しい理由」「エクスポートした識別子の doc」の 3 つだけ。
  **実装計画や設計書への参照（`Task 4`、`spec §5`）をコードに書かない**（`.claude/rules/*.md` への参照は可）
- `internal/tui` は `internal/gh/cli` も `internal/gh/gql` も `internal/config` も import しない。
  interface は**利用側**（`internal/tui/search`）で宣言する。1 つの interface 宣言に直接並べるメソッドは 6 個まで
- `View()` は副作用を持たない。時計を読まない
- **各タスクの `git commit` 例には共著者行を省いてある。** 実際のコミットには末尾に
  1 行足す（空行を 1 つ挟んでから）:
  `Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>`
- 各タスクの終わりに `make check` が緑であること
- golden は en / ja × 80 / 120 / 160。`make golden` で録り直し、**diff を目で見てから**コミットする

## このスライスに入れないもの

- **生クエリの「未設定」と「空文字」の区別**（前スライスの積み残し 11 番）。保存するのは
  組み上がったクエリ**文字列**なので、このスライスでは決着が要らない
- **生クエリからフィルタへの逆解析。** 設計が「編集は一方向」と決めている。呼び出した
  保存クエリは生クエリとして入り、フィルタ側は淡く描かれたままになる
- **保存クエリの並べ替え・名前の変更。** 仕様に無い。消して付け直せば済む
- **マウス。** Search タブは前スライスから持たない（積み残し 1 番）

---

## この計画が判断した前提（着手前に承認を取ること）

### 1. 設計 §4 からの逸脱 —— ポップアップは `dialog` と共用しない

設計 `2026-09-08-phase4-design.md:101` は
「`internal/tui/dialog` を新設し、Repos の追加ダイアログと Search の保存クエリ
ポップアップで**共用する**」と書いている。**この計画はそこから逸れる**（利用者が選択済み）。

理由は、実際に両方を並べてみると操作が別物だったこと。

| | Repos の追加ダイアログ | 保存クエリのポップアップ |
|---|---|---|
| 入力欄 | いる（打って GitHub を検索する） | **いらない**（一覧から選ぶだけ） |
| 候補の出どころ | `gh search repos` の非同期取得 | 設定ファイル。取得が無い |
| 候補の型 | `gh.RepoCandidate`（名前・スター・private） | 名前とクエリ文字列 |
| 決定したときの行き先 | 一覧に追加 | 生クエリに入れて検索 |

`dialog.Model` は `gh.RepoCandidate` に固定されており、共用するには型パラメータか
interface で一般化することになる。**その一般化は両方の呼び出し側を歪める**（Repos 側は
使わない「入力欄なし」の分岐を持ち、Search 側は使わない `Searching()` / `SetError` を持つ）。

**共有するのは箱の寸法だけにする** —— `dialog` の `boxWidth` / `contentWidth`
（`internal/tui/dialog/render.go:73,82`）を `internal/tui/layout` に出し、両方から使う。
ここは本当に同じ判断（80 桁でも読め、広い端末で間延びしない）なので、共有する価値がある。

**設計 §4 の該当文を「箱の寸法を共有する」に直す提案を、このスライスの引き継ぎ文書に書く。**
黙って逸れない（`.claude/rules/architecture.md`「規約そのものを変える」と同じ扱い）。

### 2. `Config.WantsRepos()` を `DefaultTabName()` に置き換える

今は `default_tab: repos` かどうかの真偽値 1 つ（`internal/config/config.go:29`）で、
`app.Options.DefaultRepos bool` に載っている。**`search` を表現できない。**
タブが増えるたびに bool が増える形でもある。

`Config.DefaultTabName() string` が正規化した名前（`repos` / `search` / 未設定なら `""`）を
返し、`app.Options.DefaultTab string` に載せて `app` がタブに写す。大文字小文字と前後の
空白は今と同じく許す（`strings.EqualFold` + `TrimSpace`）。**知らない名前は `""` 扱い**
にして Work タブで起動する —— 設定の綴り間違いで起動できなくなるのは割に合わない
（設計 §3「壊れていても起動を止めない」）。

### 3. 保存クエリの境界型は `internal/usecase` に置く

`internal/tui` は `internal/config` を import できない（depguard）。Repos の一覧は
`[]string` で済んだが、保存クエリは**名前とクエリの対**なので型が要る。

**置き場所は `internal/usecase`。** `internal/tui` は `internal/usecase` を既に
import しており（`internal/tui/detail` / `diff` / `review` が `usecase.Item` を扱う）、
`internal/usecase` は `internal/config` を import できる。**両方から見える唯一の場所が
ここである。**

- `usecase.SavedQuery{Name, Query string}` —— 層をまたぐ形。`usecase.Item` と同じ立場
- `config.SavedQuery{Name, Query string}` —— 設定ファイルの形（YAML タグを持つ）
- **写すのは `internal/usecase` だけ**（読み書きの両方向）

**採らなかった案:** `internal/tui/search` に型を置いて `usecase` がそれを受ける形。
`.golangci.yml` の `usecase-layer` が `internal/tui` を deny しているので**通らない**
（`internal/tui/search` もその配下）。`internal/gh` に置く案も採らない —— あの
パッケージの doc は「GitHub アクセス層が返すドメイン型」と書いており、保存クエリは
GitHub が返すものではない。

`usecase.Item` を太らせない規約（`.claude/rules/architecture.md`）はここには当たらない。
足すのは `Item` のフィールドではなく、独立した小さな型である。

### 4. `s` は「名前を打つ」→「保存」の 2 段

仕様は「名前とクエリ文字列」なので名前が要る。`s` で入力欄が開き、`enter` で保存、
`esc` で取消。**同じ名前で保存したら上書きする**（同名が 2 つ並ぶほうが混乱する）。
**名前が空のまま `enter` は保存しない**（一覧に空行が並ぶため）。

### 5. `Capturing()` はポップアップ中も true

`internal/tui/app` は `q` / `1` / `2` / `3` をタブより先に処理する
（`internal/tui/app/app.go`）。ポップアップが開いている間にこれを許すと、
`q` で octoscope が終了し `3` でタブが飛ぶ。**`Capturing()` が
`m.mode != modeBrowse` のままで済むよう、ポップアップも `mode` で表す。**

---

## ファイル構成

| ファイル | 責務 |
|---|---|
| `internal/config/config.go`（変更） | `SavedQuery` 型、`SavedQueries` フィールド、`SaveQueries`、`DefaultTabName` |
| `internal/tui/layout/popup.go`（新規） | `PopupWidth` / `PopupContentWidth`（箱の寸法だけ） |
| `internal/tui/dialog/render.go`（変更） | `boxWidth` / `contentWidth` を `layout` の関数に差し替える |
| `internal/tui/search/saved.go`（新規） | `SavedQuery` 型、ポップアップの状態と `Update`、保存・削除 |
| `internal/tui/search/saved_render.go`（新規） | ポップアップの `View`（一覧・選択・空のとき） |
| `internal/tui/search/search.go`（変更） | `modeName` / `modePicker`、`s` と `ctrl+o`、`Source` に `queryStore` |
| `internal/tui/search/render.go`（変更） | ポップアップの重ね描き、キーバーの分岐 |
| `internal/usecase/search.go`（変更） | `SavedQuery` 型、`SaveQueries` の配線、`config.SavedQuery` との相互変換 |
| `internal/usecase/usecase.go`（変更） | `queryStore` interface を `New` の store に足す |
| `internal/tui/app/app.go`（変更） | `Options.SavedQueries` / `Options.DefaultTab`、起動タブの決定 |
| `cmd/octoscope/main.go`（変更） | `config` の値を `usecase` の変換関数に通して `app.Options` に載せる |
| `internal/i18n/locales/active.{en,ja}.yaml`（変更） | 保存・ポップアップの文言 |

---

### Task 1: 設定ファイルが保存クエリと既定タブを持つ

画面より先に、設定ファイル側を固める。ここが正しければ、あとは画面から呼ぶだけになる。

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `internal/tui/app/app.go`（`Options.DefaultRepos` → `Options.DefaultTab`）
- Modify: `internal/tui/app/app_test.go`
- Modify: `cmd/octoscope/main.go`

**Interfaces:**
- Produces:
  - `type SavedQuery struct { Name string \`yaml:"name"\`; Query string \`yaml:"query"\` }`
  - `Config.SavedQueries []SavedQuery \`yaml:"saved_queries,omitempty"\``
  - `func (c Config) DefaultTabName() string` — `"repos"` / `"search"` / `""`
  - `func (s *Store) SaveQueries(queries []SavedQuery) error`
  - `app.Options.DefaultTab string`
- Removes: `Config.WantsRepos()`、`app.Options.DefaultRepos`

- [ ] **Step 1: 失敗するテストを書く**

`internal/config/config_test.go` に足す。

```go
// The settings file names a tab, not a boolean: a boolean cannot say
// "search", and every tab added later would need another one.
func TestDefaultTabNamesTheTab(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"repos":    "repos",
		"  SEARCH": "search",
		"work":     "",
		"":         "",
		"nonsense": "",
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			t.Parallel()

			if got := config.Config{DefaultTab: in}.DefaultTabName(); got != want {
				t.Errorf("DefaultTabName(%q) = %q, want %q", in, got, want)
			}
		})
	}
}

// A name the settings file does not know must not stop octoscope from
// starting: a typo in a setting is not a reason to lose GitHub.
func TestAnUnknownTabStartsOnTheDefault(t *testing.T) {
	t.Parallel()

	if got := config.Config{DefaultTab: "detail"}.DefaultTabName(); got != "" {
		t.Errorf("DefaultTabName = %q, want the empty default", got)
	}
}

// Saving queries must leave every other setting where it was: the list and
// the language are not this call's business.
func TestSavingQueriesKeepsTheRestOfTheSettings(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("language: ja\nrepositories:\n  - kukv/octoscope\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	s := config.NewStore(path)
	if err := s.SaveQueries([]config.SavedQuery{{Name: "mine", Query: "is:open author:@me"}}); err != nil {
		t.Fatalf("SaveQueries: %v", err)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Language != "ja" || len(got.Repositories) != 1 {
		t.Errorf("SaveQueries flattened the rest: %+v", got)
	}
	if len(got.SavedQueries) != 1 || got.SavedQueries[0].Query != "is:open author:@me" {
		t.Errorf("SavedQueries = %+v", got.SavedQueries)
	}
}

// A settings file that cannot be parsed is not written at all: a query is
// not worth flattening the rest of someone's settings for.
func TestSavingQueriesRefusesAFileItCannotRead(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("language: [unclosed\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := config.NewStore(path).SaveQueries(nil); err == nil {
		t.Fatal("SaveQueries succeeded on a file it could not parse")
	}
}
```

- [ ] **Step 2: テストが落ちることを確認する**

Run: `go test ./internal/config/`
Expected: FAIL（`SavedQuery` / `DefaultTabName` / `SaveQueries` が未定義）

- [ ] **Step 3: `internal/config/config.go` を直す**

```go
// SavedQuery is one entry of saved_queries: what the user called it, and
// the GitHub search it stands for.
type SavedQuery struct {
	Name  string `yaml:"name"`
	Query string `yaml:"query"`
}
```

`Config` に `SavedQueries []SavedQuery \`yaml:"saved_queries,omitempty"\`` を足す。

`WantsRepos` を消して次を置く。

```go
// DefaultTabName is the tab default_tab asks to start on, normalised. A name
// no tab answers to reads as unset: a typo in a setting is not a reason to
// refuse to start.
func (c Config) DefaultTabName() string {
	switch name := strings.ToLower(strings.TrimSpace(c.DefaultTab)); name {
	case "repos", "search":
		return name
	default:
		return ""
	}
}
```

`SaveQueries` は `SaveRepositories` と同じ形にする（`s.path == ""` の案内、`Load`、
1 フィールドだけ差し替え、`s.save(c)`）。**`save` はそのまま使う。**

- [ ] **Step 4: 呼び出し側を直す**

`internal/tui/app/app.go` の `Options.DefaultRepos bool` を `Options.DefaultTab string` にし、
起動タブの決定を 3 分岐にする（`"repos"` → `tabRepos`、`"search"` → `tabSearch`、
それ以外 → 今までどおり）。**`m.wantRepos` が今持っている「Repos で起動したい」の意味を
壊さないこと** —— 今の実装は `repo.Model` の準備ができるまで待つ形になっているので、
`search` を足すときにその待ちを壊していないか、`app_test.go` の既存テストで確かめる。

`cmd/octoscope/main.go` は `DefaultRepos: cfg.WantsRepos()` を
`DefaultTab: cfg.DefaultTabName()` に替える。

- [ ] **Step 5: テストが通ることを確認する**

Run: `go test ./internal/config/ ./internal/tui/app/ ./cmd/...`
Expected: PASS

- [ ] **Step 6: 空振りしないことを確かめる**

`DefaultTabName` の `case "repos", "search"` から `"search"` を外して
`go test ./internal/config/` が FAIL することを目で見る。戻す。

- [ ] **Step 7: `make check` を通してコミットする**

```bash
make check
git add internal/config internal/tui/app cmd/octoscope
git commit -m "feat: let the settings file hold saved queries and name the starting tab"
```

---

### Task 2: `s` でクエリに名前を付けて保存する

**Files:**
- Create: `internal/tui/search/saved.go`
- Modify: `internal/tui/search/search.go`
- Modify: `internal/tui/search/search_test.go`
- Modify: `internal/usecase/usecase.go`、`internal/usecase/search.go`
- Modify: `internal/usecase/usecase_test.go`
- Modify: `internal/tui/app/app.go`（`Options.SavedQueries`）
- Modify: `cmd/octoscope/main.go`
- Modify: `internal/i18n/locales/active.{en,ja}.yaml`

**Interfaces:**
- Produces:
  - `usecase.SavedQuery{ Name, Query string }`（`internal/usecase/search.go`）
  - `func (m Model) SetSavedQueries(qs []usecase.SavedQuery) Model`
  - `search` の `queryStore interface { SaveQueries(queries []usecase.SavedQuery) error }`、`Source` に足す
  - `func (u *Usecase) SaveQueries(queries []SavedQuery) error`
  - `app.Options.SavedQueries []usecase.SavedQuery`
  - `mode` に `modeName`
- Consumes: Task 1 の `config.SavedQuery` / `config.Store.SaveQueries`

- [ ] **Step 1: 失敗するテストを書く**

`internal/tui/search/search_test.go` に足す。**キーを送って到達させる**（`tests-that-cannot-fail`）。

```go
// s names the query before it is saved: saved_queries holds a name and a
// query, and a list of bare query strings is not one a user can pick from.
func TestSavingAsksForANameFirst(t *testing.T) {
	t.Parallel()

	store := &fakeStore{}
	m := newTestModel(t, store)
	m, _ = press(m, "s")
	if !m.Capturing() {
		t.Fatal("s did not open a field")
	}
	if len(store.saved) != 0 {
		t.Fatalf("s saved before a name was typed: %+v", store.saved)
	}
	m = typeInto(m, "mine")
	m, cmd := press(m, "enter")
	if cmd == nil {
		t.Fatal("enter did not start the save")
	}
	resolve(t, m, cmd)
	if len(store.saved) != 1 || store.saved[0].Name != "mine" {
		t.Fatalf("saved = %+v", store.saved)
	}
	if store.saved[0].Query == "" {
		t.Error("the saved entry carries no query")
	}
}

// An empty name would put a blank row in the picker that nothing can
// identify.
func TestAnEmptyNameSavesNothing(t *testing.T) {
	t.Parallel()

	store := &fakeStore{}
	m := newTestModel(t, store)
	m, _ = press(m, "s")
	m, cmd := press(m, "enter")
	if cmd != nil {
		m = resolve(t, m, cmd)
	}
	if len(store.saved) != 0 {
		t.Fatalf("an empty name was saved: %+v", store.saved)
	}
}

// esc must leave the tab as it was: a half-typed name is not a query.
func TestEscapeAbandonsTheName(t *testing.T) {
	t.Parallel()

	store := &fakeStore{}
	m := newTestModel(t, store)
	m, _ = press(m, "s")
	m = typeInto(m, "mine")
	m, _ = press(m, "esc")
	if m.Capturing() {
		t.Error("esc left the field open")
	}
	if len(store.saved) != 0 {
		t.Fatalf("esc saved anyway: %+v", store.saved)
	}
}

// Saving the same name twice replaces it: two rows with one name is a list
// nobody can choose from.
func TestSavingTheSameNameReplacesIt(t *testing.T) {
	t.Parallel()

	store := &fakeStore{}
	m := newTestModel(t, store)
	m = m.SetSavedQueries([]usecase.SavedQuery{{Name: "mine", Query: "is:open"}})
	m, _ = press(m, "s")
	m = typeInto(m, "mine")
	m, cmd := press(m, "enter")
	resolve(t, m, cmd)
	if len(store.saved) != 1 {
		t.Fatalf("saved = %+v, want one entry", store.saved)
	}
}
```

**テストヘルパーの現状を確かめてある**（`internal/tui/search/search_test.go`、
内部テストパッケージ `package search`）。

- **あるもの:** `press(m, key)`（66 行目）、`resolve(t, m, cmd)`（48 行目。
  返ってきた `tea.Cmd` を実行してメッセージを `Update` に戻す）、
  `onFilterCmd`（367 行目）
- **足すもの:** `fakeStore`（`SaveQueries` を記録するフェイク。既存の `fakeSource` に
  相当）、`newTestModel(t, store)`、`typeInto(m, "文字列")`（1 文字ずつ `press` する）

**`press` は `ctrl+o` を送れない。** 既定の枝が `[]rune(key)[0]` を返すので、
`"ctrl+o"` は `c` になる。**このタスクで枝を足すこと。**

```go
	case "ctrl+o":
		return m.Update(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
```

前スライスは `onFilterCmd` で「ヘルパーの戻り値の `tea.Cmd` を捨てない」形に直している
（捨てたせいでキャッシュのテストが空振りしていた）。**同じ轍を踏まないこと。**

- [ ] **Step 2: テストが落ちることを確認する**

Run: `go test ./internal/tui/search/`
Expected: FAIL（`s` が何もしない。`TestSDoesNothingYet` が前スライスにあるので**消す**）

- [ ] **Step 3: `internal/tui/search/saved.go` を書く**

`usecase.SavedQuery` は `internal/usecase/search.go` に置く。

```go
// SavedQuery is one of the Search tab's saved queries: what the user called
// it, and the GitHub search it stands for. It crosses the boundary because
// internal/tui cannot see internal/config, and this package can see both.
type SavedQuery struct {
	Name  string
	Query string
}
```

`m.saved []usecase.SavedQuery` をモデルに足し、`SetSavedQueries` で入れる。
保存は「同名を置き換え、無ければ末尾に足す」の純粋関数にして、単体でも試せる形にする。

```go
// upsert replaces the entry of the same name, or appends a new one. Two rows
// under one name is a list nobody can choose from.
func upsert(qs []usecase.SavedQuery, q usecase.SavedQuery) []usecase.SavedQuery
```

- [ ] **Step 4: `search.go` に `modeName` と `s` を足す**

`mode` に `modeName` を足し、`handleFilterKey` と `handleResultKey` の両方で `s` を
受ける（どちらのペインからでも保存できる。クエリはペインに依らず 1 つ）。
`handleNameKey` は `esc` で `modeBrowse`、`enter` で名前が空でなければ保存 `tea.Cmd` を返す。

**保存は `tea.Cmd` の中で行う**（`.claude/rules/errors.md`：`tea.Cmd` の中のエラーは
メッセージ型に載せて `Update` に返す）。失敗は既存の `notice` 行に出す。

- [ ] **Step 5: 配線する**

`internal/usecase/usecase.go` の `repoStore` の隣に `queryStore` を足し、`New` の
`store` がどちらも満たすようにする（`config.Store` が両方を持つ）。
`internal/usecase/search.go` に変換つきの 1 行を足す。

```go
// SaveQueries writes the Search tab's saved queries to the settings file.
func (u *Usecase) SaveQueries(queries []SavedQuery) error {
	saved := make([]config.SavedQuery, len(queries))
	for i, q := range queries {
		saved[i] = config.SavedQuery{Name: q.Name, Query: q.Query}
	}
	return u.queryStore.SaveQueries(saved)
}
```

`app.Options.SavedQueries []usecase.SavedQuery` を足し、`app` が `search.New` の後に
`SetSavedQueries` で渡す。`cmd/octoscope/main.go` は `config.SavedQuery` を直接は写せない（`app.Options` は `usecase.SavedQuery` を取る）ので、`internal/usecase` に読み込み方向の変換も置く: `func SavedQueriesFrom(qs []config.SavedQuery) []SavedQuery`。

- [ ] **Step 6: 文言を両カタログに足す**

`active.en.yaml` / `active.ja.yaml` に `search.save_prompt`（名前を促す 1 行）と
`footer.search.save`（`s: save`）、保存失敗の `search.save_failed` を足す。
**en のキーバーはコロンの後に空白を置く既存の `footer.search.*` に揃える**
（前スライスの積み残し 9 番。ここだけ直さない）。

- [ ] **Step 7: テストが通ることを確認する**

Run: `go test ./internal/tui/... ./internal/usecase/ ./cmd/...`
Expected: PASS

- [ ] **Step 8: 空振りしないことを確かめる**

`handleNameKey` の「名前が空なら保存しない」を外して
`TestAnEmptyNameSavesNothing` が FAIL することを目で見る。戻す。
`upsert` の置き換えを `append` だけに変えて `TestSavingTheSameNameReplacesIt` が
FAIL することも見る。戻す。

- [ ] **Step 9: `make check` を通してコミットする**

```bash
make check
git add -A internal cmd
git commit -m "feat: name a search and keep it"
```

---

### Task 3: `Ctrl+O` で保存クエリを呼び出す

**Files:**
- Create: `internal/tui/layout/popup.go`
- Create: `internal/tui/layout/popup_test.go`
- Create: `internal/tui/search/saved_render.go`
- Modify: `internal/tui/dialog/render.go`
- Modify: `internal/tui/search/search.go`、`render.go`、`search_test.go`
- Modify: `internal/i18n/locales/active.{en,ja}.yaml`

**Interfaces:**
- Produces:
  - `func layout.PopupWidth(termCols int) int`
  - `func layout.PopupContentWidth(boxCols int) int`
  - `mode` に `modePicker`
  - `func (m Model) pickerView() string`
- Consumes: Task 2 の `SavedQuery` / `m.saved`

- [ ] **Step 1: 箱の寸法を `layout` に出す**

`internal/tui/dialog/render.go:73,82` の `boxWidth` / `contentWidth` を
`internal/tui/layout/popup.go` に移す。**判断（80 桁でも読め、広い端末で間延びしない）が
同じ**なので共有する。`dialog` はそれを呼ぶだけにする。

```go
// PopupWidth keeps a popup readable at eighty columns without letting it run
// the full width of a wide terminal.
func PopupWidth(termCols int) int

// PopupContentWidth is what a line inside the box may occupy. A longer line
// makes lipgloss wrap it, which splits a row across two.
func PopupContentWidth(boxCols int) int
```

`popup_test.go` は移設元 `dialog` のテストを引き取る。**`dialog` の golden が
1 桁も動かないことが、この移設が正しいことの証拠**になる（`make golden` で差分が
出ないこと。出たら移設を間違えている）。

- [ ] **Step 2: 失敗するテストを書く**

```go
// ctrl+o opens the list of saved queries, and enter runs the one it lands on.
func TestCtrlOOpensTheSavedQueriesAndEnterRunsOne(t *testing.T) {
	t.Parallel()

	m := newTestModel(t, &fakeStore{})
	m = m.SetSavedQueries([]usecase.SavedQuery{
		{Name: "mine", Query: "is:open author:@me"},
		{Name: "reviews", Query: "is:open review-requested:@me"},
	})
	m, _ = press(m, "ctrl+o")
	if !strings.Contains(m.View(), "reviews") {
		t.Fatal("the popup does not list the saved queries")
	}
	m, _ = press(m, "j")
	m, cmd := press(m, "enter")
	if cmd == nil {
		t.Fatal("enter did not run the query")
	}
	if !strings.Contains(m.View(), "review-requested:@me") {
		t.Error("the raw query row does not show what was picked")
	}
}

// The root acts on q and 3 before a tab sees them. Typing over an open popup
// would quit octoscope or jump tabs.
func TestThePopupHoldsTheKeys(t *testing.T) {
	t.Parallel()

	m := newTestModel(t, &fakeStore{})
	m = m.SetSavedQueries([]usecase.SavedQuery{{Name: "mine", Query: "is:open"}})
	m, _ = press(m, "ctrl+o")
	if !m.Capturing() {
		t.Error("the popup does not capture keys")
	}
}

// Nothing saved yet is a state the popup has to say something about, not an
// empty box.
func TestThePopupSaysWhenNothingIsSaved(t *testing.T) {
	t.Parallel()

	m := newTestModel(t, &fakeStore{})
	m, _ = press(m, "ctrl+o")
	if !strings.Contains(m.View(), i18n.T("search.no_saved_queries")) {
		t.Error("the popup does not say the list is empty")
	}
}

// esc closes it and changes nothing.
func TestEscapeClosesThePopup(t *testing.T) {
	t.Parallel()

	m := newTestModel(t, &fakeStore{})
	m = m.SetSavedQueries([]usecase.SavedQuery{{Name: "mine", Query: "is:open author:@me"}})
	before := m.View()
	m, _ = press(m, "ctrl+o")
	m, _ = press(m, "esc")
	if m.Capturing() {
		t.Error("esc left the popup open")
	}
	if m.View() != before {
		t.Error("esc changed the tab")
	}
}
```

Run: `go test ./internal/tui/search/`
Expected: FAIL（`ctrl+o` が何もしない）

- [ ] **Step 3: ポップアップを書く**

`modePicker` と `m.pick int`（一覧のカーソル）をモデルに足す。`ctrl+o` は
どちらのペインからでも開く。`j`/`k` で移動、`enter` で `m.raw` に入れて `startSearch()`、
`esc` で閉じる。

`saved_render.go` の `pickerView()` は `theme.Popup()` と `layout.PopupWidth` で箱を描き、
1 行に `名前` と、余白があれば淡色でクエリを出す。**`layout.Clip` で桁を切る。**
一覧が空なら `search.no_saved_queries` の 1 行だけ。

`render.go` の `View()` は `modePicker` のとき本体の上にポップアップを重ねる
（`dialog` を Repos が重ねているのと同じ形。`internal/tui/repo/render.go` を見る）。
キーバーは `Capturing()` のとき `esc` と `enter` だけを出す既存の分岐に乗せる。

- [ ] **Step 4: テストが通ることを確認する**

Run: `go test ./internal/tui/...`
Expected: PASS

- [ ] **Step 5: 空振りしないことを確かめる**

`enter` が `m.raw` に入れる行を消して
`TestCtrlOOpensTheSavedQueriesAndEnterRunsOne` が FAIL することを目で見る。戻す。
`Capturing()` が `modePicker` を数えないように変えて `TestThePopupHoldsTheKeys` が
FAIL することも見る。戻す。

- [ ] **Step 6: `make check` を通してコミットする**

```bash
make check
git add -A internal
git commit -m "feat: call a saved search back with ctrl+o"
```

---

### Task 4: ポップアップで `x` 消す

**Files:**
- Modify: `internal/tui/search/saved.go`、`search.go`、`saved_render.go`、`search_test.go`
- Modify: `internal/i18n/locales/active.{en,ja}.yaml`

**Interfaces:**
- Consumes: Task 2 の `queryStore`、Task 3 の `modePicker`

- [ ] **Step 1: 失敗するテストを書く**

```go
// x removes the row and writes the rest back, the way the Repos sidebar's x
// does. Without it the only way to drop a query is to edit config.yaml.
func TestXRemovesASavedQueryAndSavesTheRest(t *testing.T) {
	t.Parallel()

	store := &fakeStore{}
	m := newTestModel(t, store)
	m = m.SetSavedQueries([]usecase.SavedQuery{
		{Name: "mine", Query: "is:open author:@me"},
		{Name: "reviews", Query: "is:open review-requested:@me"},
	})
	m, _ = press(m, "ctrl+o")
	m, cmd := press(m, "x")
	if cmd == nil {
		t.Fatal("x did not write the list back")
	}
	resolve(t, m, cmd)
	if len(store.saved) != 1 || store.saved[0].Name != "reviews" {
		t.Fatalf("saved = %+v, want only reviews", store.saved)
	}
	if strings.Contains(m.View(), "mine") {
		t.Error("the removed row is still drawn")
	}
}

// Removing the last row leaves the cursor on something that exists.
func TestRemovingTheLastRowKeepsTheCursorInRange(t *testing.T) {
	t.Parallel()

	store := &fakeStore{}
	m := newTestModel(t, store)
	m = m.SetSavedQueries([]usecase.SavedQuery{
		{Name: "a", Query: "is:open"},
		{Name: "b", Query: "is:pr"},
	})
	m, _ = press(m, "ctrl+o")
	m, _ = press(m, "j")
	m, cmd := press(m, "x")
	resolve(t, m, cmd)
	m, _ = press(m, "enter")
	if !strings.Contains(m.View(), "is:open") {
		t.Error("enter after the removal did not land on the row that is left")
	}
}

// x on an empty list must not panic or write an empty file for nothing.
func TestXOnAnEmptyListDoesNothing(t *testing.T) {
	t.Parallel()

	store := &fakeStore{}
	m := newTestModel(t, &fakeStore{})
	m, _ = press(m, "ctrl+o")
	m, cmd := press(m, "x")
	if cmd != nil {
		m = resolve(t, m, cmd)
	}
	if len(store.saved) != 0 {
		t.Error("x wrote to the settings file with nothing to remove")
	}
}
```

Run: `go test ./internal/tui/search/`
Expected: FAIL（`x` が何もしない）

- [ ] **Step 2: 実装する**

`handlePickerKey` に `x` を足す。**カーソルを一覧の範囲に収め直すこと**（最後の行を
消したらひとつ前に寄せる。`internal/tui/repo/repo.go:498` の `x` が同じことをしている）。
書き戻しは Task 2 と同じ `tea.Cmd`。キーバーに `footer.search.remove` を足す。

- [ ] **Step 3: テストが通ることを確認する**

Run: `go test ./internal/tui/...`
Expected: PASS

- [ ] **Step 4: 空振りしないことを確かめる**

カーソルを収め直す行を消して `TestRemovingTheLastRowKeepsTheCursorInRange` が
FAIL することを目で見る。戻す。

- [ ] **Step 5: `make check` を通してコミットする**

```bash
make check
git add -A internal
git commit -m "feat: drop a saved search from the popup"
```

---

### Task 5: golden と、キー入力だけで通すシナリオ

**このタスクは「見て確かめる」ためにある**（`.claude/rules/tui.md`、
`tui-never-ship-without-rendering`）。テストが緑でも桁ずれは見つからない。

**Files:**
- Modify: `internal/tui/search/golden_test.go`
- Create: `internal/tui/search/testdata/search_picker_*.golden`、`search_naming_*.golden`
- Modify: `internal/tui/app/app_test.go`

- [ ] **Step 1: golden に 2 状態を足す**

既存の 4 状態（loaded / editing / empty / candidates）に `picker`（保存クエリが 2 件
入ったポップアップ）と `naming`（名前の入力中）を足す。en / ja × 80 / 120 / 160 で
合計 12 枚増える。**保存クエリの名前は日本語を 1 つ含める**（全角で桁を 2 つ使うため）。

- [ ] **Step 2: 録って、目で見る**

```bash
make golden
cat -v internal/tui/search/testdata/search_picker_ja_80.golden
cat -v internal/tui/search/testdata/search_naming_ja_80.golden
```

**確かめること:** ja の 80 桁でポップアップが端末に収まること。キーバーが
`esc` / `enter` / `x` を出していること。名前とクエリが重ならないこと。
**ずれていたら `render` を直す。golden を受け入れない。**

- [ ] **Step 3: `app` にシナリオテストを足す**

`internal/tui/app/app_test.go` に、**キー入力だけ**で通すものを 1 本。

```go
// The whole path a user takes, through the root model that owns the tabs:
// nothing here reaches into a sub-model's fields.
func TestSavingAQueryAndCallingItBack(t *testing.T) {
	// 3 -> s -> "mine" -> enter -> ctrl+o -> enter
	// and assert the raw query row shows what was saved
}
```

**`3` と `q` がポップアップに吸われること**もここで確かめる（`Capturing()` の委譲が
`app` 側で効いているか。前提 5）。

- [ ] **Step 4: `make check` を通してコミットする**

```bash
make check
git add -A internal
git commit -m "test: record the saved-query popup at three widths in both languages"
```

---

### Task 6: 引き継ぎ文書

**Files:**
- Create: `docs/superpowers/2026-09-12-phase4-saved-queries-followups.md`

- [ ] **Step 1: 書く**

過去のスライスと同じ形（`docs/superpowers/2026-09-12-phase4-search-tab-followups.md` に倣う）。
**最低限:**

- **実端末で見てほしいもの**（TTY が要るので代行できない）: `Ctrl+O` のポップアップが
  ja の 80 桁で収まること、`s` → 名前 → `enter` が本当に `config.yaml` に書かれること、
  呼び出したクエリで検索が走ること、`x` が次の起動でも消えたままであること
- **設計 §4 の「`dialog` を共用する」を満たしていないこと**と、その理由（前提 1）。
  **§4 の該当文を「箱の寸法を共有する」に直す提案**を書く
- 前スライスの積み残しのうち、このスライスで解消したもの（4 番 `s` の予約、
  `config.Store` の `SaveQueries`、`default_tab: search`）と、**残っているもの**
  （11 番の生クエリの「未設定」と「空文字」、1 番のマウス、2・3 番のページングと `searchCap`）
- Repos の追加ダイアログの積み残し 8 番（候補のスクロール）が、**このスライスでは
  決着しなかった**こと。保存クエリのポップアップは一覧が短く、スクロールを必要と
  しなかったため

- [ ] **Step 2: コミットする**

```bash
git add docs/superpowers/2026-09-12-phase4-saved-queries-followups.md
git commit -m "docs: record what the saved queries left behind"
```

---

## 完了条件（このスライス）

1. `s` で名前を付けて保存でき、`config.yaml` の `saved_queries` に残る
2. `Ctrl+O` で一覧が出て、`enter` で選んだクエリの検索が走る
3. `x` で消え、消えたことが次の起動でも残る
4. `default_tab: search` で Search タブから起動する。知らない名前でも起動する
5. ポップアップが開いている間、`q` で終了せず `3` でタブが飛ばない
6. golden が picker / naming について en / ja × 80 / 120 / 160 で録れており、
   **ja の 80 桁で収まっている**
7. `internal/tui` が `internal/config` を import しておらず、`internal/usecase` が `internal/tui` を import していない（`make lint` の depguard）
8. `make check` が緑
