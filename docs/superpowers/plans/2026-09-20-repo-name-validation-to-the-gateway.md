# リポジトリ名の検証を gateway に移す実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development
> (recommended) or superpowers:executing-plans to implement this plan task-by-task.
> Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 追加ダイアログの `isOwnerSlashName`（`gql.SplitRepo` のほぼ逐語コピー）を消し、
「その名前がこのサービスのリポジトリ名として成立するか」を port 1 本で gateway に問う形にする。
ルールの実体を 1 つにする。

**Architecture:** presentation の `repoEditor` に `ValidRepoName(name string) bool` を足す。
`usecase` は薄く委譲し、`gateway/gh` が `gql.SplitRepo` を呼ぶだけで実装する。
ネットワークに出ないので `ctx` は取らない（`SaveRepositories` と同じ）。
**画面の振る舞いは変わらない**——同じ入力に同じ判定が返り、`dialog.invalid_name` の文言も変わらない。

**Tech Stack:** Go、golangci-lint、gotestsum、`internal/golden`

**Spec:** as-is モデリングの違和感 F-15
（https://claude.ai/artifact/7HMQJ6kbp2wVs1LoiNG3JU）。
設計書は書かない——bounded な変更として利用者の承認を得ている（案 B）。

---

## 着手前に調べた事実（2026-09-20）

### A. 重複は 2 箇所、完全に同じ判定

| 場所 | 中身 |
|---|---|
| `presentation/tui/repo/add.go:103` `isOwnerSlashName` | `TrimSpace` 一致 / `Cut("/")` / 両半が非空 / 後半に `/` を含まない |
| `internal/github/gql/gql.go:86` `SplitRepo` | 同じ 4 条件（戻り値が `owner, name, ok`） |

テストのテーブルも 7 ケースが**同じ**（`repo/add_test.go:17` と `gql/gql_test.go:116`）。

### B. なぜ共有できていないか

`.golangci.yml` の depguard が presentation → `internal/github` を禁じている。
規則の置き場所が両者の間に無い。**これは層の線が正しく引かれている結果**であって、
規約の不備ではない。

### C. domain に置く案（Artifact が挙げた方向性）を採らない理由

1. `internal/github` は `internal/app` を import できないので `gql.SplitRepo` は残る。
   **重複は消えず、コピーが移動するだけ**になる。
2. 「2 つ目の `/` を弾く」は GitHub の形である（GitLab は `group/subgroup/project`）。
   #124 で確立した線「**翻訳は ACL（gateway）、政策はドメイン**」に照らすと翻訳にあたる。
   GitHub の enum を domain から追い出した直後に、別の GitHub の形を入れ直すことになる。

### D. 対象外

- `internal/github/api/repo.go:74-77` の `const host = "github.com"`（remote 解析のホスト拒否）。
  拡張点が明示されており、Artifact も明示的に除外している
- F-12（識別子にサービス次元を足す）。保留中。本計画はそれを待たずに単一サービスとして完結する
- `config` / `datasource`

### E. 触るフェイク

`Source` を満たすフェイクは 3 つ。全部に 1 メソッド足す必要がある。

| ファイル | 型 |
|---|---|
| `internal/app/presentation/tui/repo/repo_test.go:21` | `fakeSource` |
| `internal/app/presentation/tui/root/root_test.go` | `fakeSource` |
| `internal/app/presentation/tui/root/scenario_test.go` | `scenarioSource` |

**フェイクは規則を写さない。** 既定で `true` を返し、エラー経路を見たいテストだけ
`false` を返させる。写すと今度はテストコードに重複が生まれる。
7 ケースのテーブルは gateway に 1 つだけ置く。

### F. golden への影響は無いはず

`dialog.invalid_name` を含む golden は無い（`add_test.go:133` が `View()` の
文字列包含で見ている）。**golden 354 枚が 1 枚も変わらないことが合格条件。**
動いたらこの前提が誤っていた証拠なので、そこで止める。

## Global Constraints

- **golden 354 枚と `testdata/` 全体を 1 バイトも変えない。** `OCTOSCOPE_UPDATE_GOLDEN` を使わない
- **i18n のカタログを触らない。** `dialog.invalid_name` はキーも文言もそのまま
- 各タスクの終わりに `make check` が通ること
- interface のメソッド数上限 6（`repoEditor` は 3 → 4 で収まる）

---

### Task 1: gateway に `ValidRepoName` を足す

**Files:**
- Modify: `internal/app/adapter/gateway/gh/repos.go`
- Test: `internal/app/adapter/gateway/gh/repos_test.go`

**Interfaces:**
- Consumes: `gql.SplitRepo(repo string) (owner, name string, ok bool)`
- Produces: `func (g *Gateway) ValidRepoName(name string) bool`

- [ ] **Step 1: 失敗するテストを書く**

`internal/app/adapter/gateway/gh/repos_test.go` の末尾に足す。
`gql` の import を追加する必要はない（`ValidRepoName` は `Gateway` のメソッド）。
`New(fakeRepoFinder{})` は nil backend を埋めているだけなので、
backend を一度も呼ばないこのメソッドでは panic しない。

```go
// ValidRepoName is the add dialog's guard. The table is the one place the
// shape of a name is written down for the view's sake; gql.SplitRepo holds
// the rule itself and has its own table.
func TestValidRepoNameAcceptsOwnerSlashNameAndNothingElse(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"owner and name", "kukv/octoscope", true},
		{"no slash", "octoscope", false},
		{"empty owner", "/octoscope", false},
		{"empty name", "kukv/", false},
		{"three parts", "github.com/kukv/octoscope", false},
		{"surrounding space", " kukv/octoscope ", false},
		{"empty", "", false},
	}
	g := New(fakeRepoFinder{})
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := g.ValidRepoName(c.in); got != c.want {
				t.Errorf("ValidRepoName(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}
```

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/app/adapter/gateway/gh/ -run TestValidRepoName`
Expected: コンパイルエラー `g.ValidRepoName undefined`

- [ ] **Step 3: 最小の実装を書く**

`internal/app/adapter/gateway/gh/repos.go` の `import` に
`"github.com/kukv/octoscope/internal/github/gql"` を足し、`toRepoCandidate` の手前に置く。

```go
// ValidRepoName reports whether name is one this service could have. The
// shape is GitHub's -- "owner/name", no second slash -- so the rule lives
// here rather than in the domain: another service spells it differently
// (GitLab nests groups) and the view must not have to know which.
func (g *Gateway) ValidRepoName(name string) bool {
	_, _, ok := gql.SplitRepo(name)
	return ok
}
```

- [ ] **Step 4: 通ることを確かめる**

Run: `go test ./internal/app/adapter/gateway/gh/ -run TestValidRepoName -v`
Expected: 7 サブテストすべて PASS

- [ ] **Step 5: コミット**

```bash
git add internal/app/adapter/gateway/gh/repos.go internal/app/adapter/gateway/gh/repos_test.go
git commit -m "$(cat <<'EOF'
feat: let the gateway say whether a name could be a repository

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: port を通し、presentation のコピーを消す

一度に行う。interface に 1 メソッド足すと 3 つのフェイクが同時にコンパイルを失うので、
途中でビルドが通る切り方が無い。

**Files:**
- Modify: `internal/app/usecase/repos.go`
- Modify: `internal/app/presentation/tui/repo/repo.go:35-39`
- Modify: `internal/app/presentation/tui/repo/add.go`（`isOwnerSlashName` と `strings` の import を削除）
- Test: `internal/app/presentation/tui/repo/repo_test.go`（フェイク）
- Test: `internal/app/presentation/tui/repo/add_test.go`（`TestIsOwnerSlashName` を削除、`TestAddingRefusesANameThatIsNotOwnerSlashName` を書き換え）
- Test: `internal/app/presentation/tui/root/root_test.go`（フェイク）
- Test: `internal/app/presentation/tui/root/scenario_test.go`（フェイク）

**Interfaces:**
- Consumes: `(*gh.Gateway).ValidRepoName(name string) bool`（Task 1）
- Produces: `(*usecase.Usecase).ValidRepoName(name string) bool`、
  `repo.Source` が `ValidRepoName(name string) bool` を要求するようになる

- [ ] **Step 1: 失敗するテストを書く（ビューがソースの判定に従うこと）**

`internal/app/presentation/tui/repo/repo_test.go` の `fakeSource` に
フィールドとメソッドを足す。既定 `true`、テストが求めたときだけ拒否する。

```go
	rejectName bool // ValidRepoName says no to everything
```

```go
// ValidRepoName answers what the test asked for. The rule itself lives in
// the gateway; copying it here would be the duplication this replaced.
func (f *fakeSource) ValidRepoName(string) bool { return !f.rejectName }
```

`internal/app/presentation/tui/repo/add_test.go` の `TestIsOwnerSlashName`
（17-41 行）を丸ごと消す。7 ケースのテーブルは Task 1 で gateway に置いた。

`TestAddingRefusesANameThatIsNotOwnerSlashName`（122 行）を**書き換える**。
**新しいテストは足さない**——このテストが既にその経路を通っており、
既定で `true` を返すフェイクのままでは落ちる。ビューが見るべきなのは
「名前の形」ではなく「ソースが拒んだこと」になった。

```go
// The settings file is hand-editable, and a malformed entry there is already
// read as an uncountable row. Writing one on purpose would be worse. What
// counts as malformed is the source's answer, not a rule kept here.
func TestAddingRefusesTheNameTheSourceRejects(t *testing.T) {
	f := &fakeSource{rejectName: true}
	m := sized(New(f, Options{}), 120)
	m, _ = m.Update(key("a"))
	m = typeInto(m, "octoscope")
	m, cmd := m.Update(key("enter"))
	drain(t, cmd)
	if f.saved != nil {
		t.Errorf("saved %v, want nothing written", f.saved)
	}
	if !strings.Contains(m.View(), i18n.T("dialog.invalid_name")) {
		t.Errorf("the dialog did not say why:\n%s", m.View())
	}
}
```

同ファイルの他のテスト（`kukv/koto` などを足すもの）は既定の `fakeSource` を
使っており、`rejectName` が偽なので `ValidRepoName` は `true` を返す。書き換え不要。

- [ ] **Step 2: 落ちることを確かめる**

Run: `go test ./internal/app/presentation/tui/repo/`
Expected: コンパイルエラー（`fakeSource` が `Source` を満たさない、
または `isOwnerSlashName` がまだ残っている）

- [ ] **Step 3: port を通す**

`internal/app/presentation/tui/repo/repo.go` の `repoEditor` に 1 行足す。

```go
type repoEditor interface {
	SearchRepos(ctx context.Context, query string, limit int) ([]domain.RepoCandidate, error)
	SeedCandidates(ctx context.Context) ([]domain.RepoCandidate, error)
	SaveRepositories(repos []string) error
	ValidRepoName(name string) bool
}
```

`internal/app/usecase/repos.go` の `repoFinder` に 1 行足す。

```go
type repoFinder interface {
	SearchRepos(ctx context.Context, query string, limit int) ([]domain.RepoCandidate, error)
	ListOwnRepos(ctx context.Context, owner string, limit int) ([]domain.RepoCandidate, error)
	ListOrgs(ctx context.Context) ([]string, error)
	ValidRepoName(name string) bool
}
```

同ファイルの `SearchRepos` の下に委譲を足す。

```go
// ValidRepoName reports whether the add dialog's input could name a
// repository. The shape belongs to the service, so the answer comes from
// the gateway rather than from a rule written in the view.
func (u *Usecase) ValidRepoName(name string) bool {
	return u.repos.ValidRepoName(name)
}
```

- [ ] **Step 4: presentation のコピーを消す**

`internal/app/presentation/tui/repo/add.go`:

- `addSelected` の `if !isOwnerSlashName(name) {` を `if !m.src.ValidRepoName(name) {` に変える
  （`m.src` は `Model` が持つソース。フィールド名は `repo.go` の `Model` 定義で確かめる）
- `isOwnerSlashName` 関数（100-109 行、doc コメントを含む）を削除
- `import` から `"strings"` を削除（この関数が唯一の利用者である）

- [ ] **Step 5: フェイクを 2 つ足す**

`internal/app/presentation/tui/root/root_test.go` の `fakeSource` と
`internal/app/presentation/tui/root/scenario_test.go` の `scenarioSource` に、
それぞれ `SaveRepositories` の隣へ置く。

```go
func (f *fakeSource) ValidRepoName(string) bool { return true }
```

```go
func (f *scenarioSource) ValidRepoName(string) bool { return true }
```

- [ ] **Step 6: 通ることを確かめる**

Run: `go test ./internal/app/... ./cmd/...`
Expected: すべて PASS

- [ ] **Step 7: golden が 1 枚も変わっていないことを確かめる**

Run: `git status --porcelain -- '*testdata*'`
Expected: **出力が空**。1 行でも出たら、事実 F の前提が誤っていたということなので
そこで止めて報告する。

- [ ] **Step 8: コミット**

```bash
git add -A internal/app
git commit -m "$(cat <<'EOF'
refactor: ask the source whether a repository name is well formed

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: 検査と記録

**Files:**
- Modify: `.claude/rules/architecture.md`（「名前は借りてよい、形は借りない」の表に 1 行）

- [ ] **Step 1: CI と同じ検査を全部通す**

Run: `make check`
Expected: tidy / lint / fmt / test すべて成功

- [ ] **Step 2: 規約の表に今回の判断を書く**

`.claude/rules/architecture.md` の「名前は借りてよい、形は借りない」の適用結果の表、
「変える」列に足す。

```markdown
| | `isOwnerSlashName`（presentation の判定）→ `ValidRepoName(name string) bool`。名前の形は GitHub のものなので gateway が答える |
```

その表の下に 1 段落足す。

```markdown
**リポジトリ名の形を gateway に移したのは 2026-09-20 である。** presentation にあった
`isOwnerSlashName` は `gql.SplitRepo` の逐語コピーで、depguard が
presentation → `internal/github` を禁じているために共有できずにいた。domain に置けば
層の線は通るが、`gql.SplitRepo` は消えないので重複は残り、しかも「2 つ目の `/` を弾く」
という GitHub の形を domain に入れることになる。#124 の線（翻訳は ACL、政策はドメイン）では
これは翻訳なので、gateway が答える。

**守れなくなるもの:** 純粋な述語 1 つのために port が 1 本増え、`Source` を満たす
フェイク 3 つがスタブを 1 つずつ背負う。検証にネットワークが要らないことは port を
読んでも分からない（`ctx` を取らないことが唯一の手がかりである）。
```

- [ ] **Step 3: 実機で見る**

**Claude 側からは実行できない**（pty の起動がガードに阻まれる）。利用者に依頼する。

```bash
go run ./cmd/octoscope          # a → "octoscope" だけ入力 → Enter でエラーが出る
go run ./cmd/octoscope --lang ja # 同じ操作で日本語の文言を確認
go run ./cmd/octoscope          # a → "kukv/koto" → Enter で追加できる
```

- [ ] **Step 4: コミットして PR**

```bash
git add .claude/rules/architecture.md
git commit -m "$(cat <<'EOF'
docs: record where the shape of a repository name lives

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
)"
```

PR 本文には、実機確認を利用者が行ったか未実施かを明記する。

## 完了条件

- [ ] `isOwnerSlashName` が存在しない（`grep -rn isOwnerSlashName` が 0 件）
- [ ] `gql.SplitRepo` を呼ぶ `Gateway.ValidRepoName` が 1 つだけある
- [ ] golden 354 枚と `testdata/` が 1 バイトも変わっていない
- [ ] `make check` が通る
- [ ] `.claude/rules/architecture.md` に判断と代償が書かれている
