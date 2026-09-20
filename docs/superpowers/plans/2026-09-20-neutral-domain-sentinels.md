# ドメインのセンチネルから対処方法の文言を外す実装計画

**Goal:** `domain.ErrBackendUnavailable` と `domain.ErrUnauthenticated` の文言を
**種別を名乗るだけ**にする。対処方法（「gh CLI を入れて gh auth login」）は
既に i18n にあり、ドメイン側のものは表示されない残骸である。

**Architecture:** 文字列を 2 つ変え、doc コメントを直し、逆戻りを捕まえるテストを 1 本足す。
振る舞いは変わらない——どちらの文言も現状どこにも表示されていない（下の事実 A）。

**Tech Stack:** Go、golangci-lint、gotestsum、`internal/golden`

**Spec:** as-is モデリングの違和感 F-06
（https://claude.ai/artifact/7HMQJ6kbp2wVs1LoiNG3JU）。
設計書は書かない——bounded な変更として利用者の承認を得ている。

---

## 着手前に調べた事実（2026-09-20）

### A. この 2 つの文言は、どこにも表示されていない

| 経路 | 何が出るか |
|---|---|
| `root.showError`（root.go:577-580） | `errors.Is` で捕まえ `i18n.T("error.gh_not_found")` / `error.unauthenticated` に差し替える |
| `domain.Classify` が返す `classified` | `Error()` は gh の原文だけ。センチネルは `Unwrap()` の先にいるので前に付かない |
| `cmd/octoscope/backend.go:30` の `fmt.Errorf("%w: %w", ...)` | main.go:93 が `i18n.T("error.no_backend")` を出して捨てる |

**つまり消しても画面は 1 文字も変わらない。** golden が動いたら、この前提が誤っていた証拠なのでそこで止める。

### B. doc コメントが既に文言と矛盾している

`ErrBackendUnavailable` の doc は「gh バイナリが無いのは**あるバックエンドの**問題であって、
アプリケーション自身が知ることではない」と書いているのに、文言はその gh を名指ししている。
`ErrUnauthenticated` の doc は「gh has no usable credentials」で、
api バックエンド（`GH_TOKEN`）にも当たる今は事実として不正確。

### C. 文言に依存しているテストは無い

- `gateway/gh/writes_test.go:262` は `github.ErrUnauthenticated`（**インフラ側**）の文言を比べている。無関係
- `domain/errors_test.go` は `errors.Is` だけ
- `cmd/octoscope/backend_test.go` はコメントで「センチネルの文言」に触れるが、主張は `errors.Is`

### D. 規約の例が、まさに今消す文言である

`.claude/rules/errors.md` の「センチネルエラー」の例:

```go
// ErrGhNotFound is returned when the gh binary is not on PATH.
var ErrGhNotFound = errors.New("gh CLI not found; install it and run: gh auth login")
```

`ErrGhNotFound` は**現存しない名前**（`ErrBackendUnavailable` に改名済み、
`2026-09-13-restructure-pr2b-gateway.md`）。文言も今回消すもの。
放っておくと規約が F-06 のアンチパターンを教え続ける。**利用者が例の差し替えを承認済み。**

## Global Constraints

- **golden 354 枚と `testdata/` 全体を 1 バイトも変えない**
- **i18n のカタログを触らない。** `error.gh_not_found` はキー名も文言も gh を名指しするが、
  プレゼン層は対処方法を言ってよい。改名は範囲外
- **`internal/github` を触らない。** `github.ErrUnauthenticated` の "run: gh auth login" は
  インフラ層の文言。api バックエンドには少しずれているが F-06 の対象外
- **`domain.ErrTransient` を触らない。** "GitHub did not answer" は F-06 に挙がっていない
- `make check` が通ること
- コミットメッセージの末尾に `Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>` を付ける

---

### Task 1: センチネルを中立にし、逆戻りをテストで留める

**Files:**
- `internal/app/domain/errors.go`（変更）
- `internal/app/domain/errors_test.go`（変更）

- [ ] **Step 1: 先にテストを書く（落ちることを確認する）**

`errors_test.go` に足す:

```go
// A domain sentinel names what kind of failure it is and stops there. The
// remedy ("install gh and run gh auth login") belongs to the UI, which has
// the user's language: i18n's error.gh_not_found and error.unauthenticated
// are what the error screen actually shows (root.go showError). A sentinel
// that carries the remedy is a second, untranslated copy that nothing
// displays -- and that goes stale the moment a second backend exists.
func TestSentinelsNameTheKindAndNotTheRemedy(t *testing.T) {
	t.Parallel()

	remedies := []string{"gh CLI", "gh auth", "install", "run:"}
	for _, err := range []error{
		domain.ErrBackendUnavailable,
		domain.ErrUnauthenticated,
	} {
		for _, r := range remedies {
			if strings.Contains(err.Error(), r) {
				t.Errorf("%q carries a remedy (%q); it belongs in i18n", err.Error(), r)
			}
		}
	}
}
```

`go test ./internal/app/domain/` で**落ちること**を確認する。落ちなければ、
すでに文言が変わっているか、テストが空振りしている。

- [ ] **Step 2: 文言と doc コメントを直す**

```go
// ErrBackendUnavailable is returned when the backend that talks to GitHub
// cannot be reached at all -- the gh binary missing is one backend's
// problem, not a thing the application itself knows about. The text names
// only the kind: what the user should do about it is the UI's to say, in
// their language (i18n error.gh_not_found).
var ErrBackendUnavailable = errors.New("backend unavailable")

// ErrUnauthenticated is returned when the backend has no usable credentials
// -- gh not signed in, or no token for the API client. Same rule as above:
// the remedy is i18n error.unauthenticated, not this string.
var ErrUnauthenticated = errors.New("not authenticated")
```

- [ ] **Step 3: 検査**

```bash
go test ./internal/app/domain/
make check
git status --porcelain | grep -c testdata    # 0 であること
```

**golden が 1 枚でも動いたら止める。** 事実 A が誤っていたということなので、
どの経路で文言が画面に届いていたのかを調べてから続ける。

- [ ] **Step 4: コミット**

```bash
git add internal/app/domain
git commit -m "refactor: let the domain sentinels name the kind, not the remedy" \
  -m "ErrBackendUnavailable said \"gh CLI not found; install it and run: gh
auth login\" while its own doc comment said the gh binary is one backend's
problem and not the application's. Nothing displayed that string: the error
screen matches the sentinel with errors.Is and shows i18n's text instead, and
Classify keeps the client's own words in Error(). It was an untranslated
second copy of a remedy, waiting to go stale." \
  -m "Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: 規約の例を現行の形に差し替える

**Files:**
- `.claude/rules/errors.md`（変更）

- [ ] **Step 1: 「センチネルエラー」節の例を差し替える**

`ErrGhNotFound`（現存しない名前 + 今消した文言）を `ErrBackendUnavailable` にし、
**文言は種別だけ、対処方法は i18n** という原則を 1 段落で足す。
既存の「分岐に使わないエラーをセンチネルにしない」以下は**そのまま残す**。

差し替え後の形:

```markdown
## センチネルエラー

利用側が種類で分岐する必要があるものだけ、パッケージ変数にする。

```go
// ErrBackendUnavailable is returned when the backend that talks to GitHub
// cannot be reached at all.
var ErrBackendUnavailable = errors.New("backend unavailable")
```

判定は `errors.Is`。文字列比較しない。

**ドメインのセンチネルの文言は、種別を名乗るだけにする。** 対処方法
（「gh を入れて gh auth login」）は i18n のカタログに置く——利用者の言語で出すのは
UI の仕事で、センチネルに書くとどこにも表示されない英文の二重管理になる。
インフラ層（`internal/github`）のセンチネルはそのサービスの言葉で書いてよい。
```

- [ ] **Step 2: 例と実物が一致していることを確かめる**

```bash
grep -n "ErrBackendUnavailable" .claude/rules/errors.md internal/app/domain/errors.go
```

両方に同じ文言が出ること。**`ErrGhNotFound` が規約に残っていないこと。**

- [ ] **Step 3: コミット**

```bash
git add .claude/rules/errors.md
git commit -m "docs: fix the sentinel example the rule teaches" \
  -m "The example was ErrGhNotFound, a name renamed away in the architecture
restructure, carrying the exact remedy text the previous commit removed. It
taught the thing F-06 is about." \
  -m "Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## この PR の完了条件

- `domain.ErrBackendUnavailable` / `domain.ErrUnauthenticated` の文言に
  `gh CLI` / `gh auth` / `install` / `run:` が含まれない
- それを主張するテストがあり、文言を戻すと落ちる（Step 1 で落ちることを確認済み）
- 両者の doc コメントが文言と矛盾しない
- `.claude/rules/errors.md` の例が現存する名前と文言になっている
- i18n カタログ・`internal/github`・`domain.ErrTransient` は無変更
- golden 354 枚と `testdata/` 全体が無変更
- `make check` が通る
- **利用者が実機で確認する**: gh を PATH から外したときと未認証のときのエラー画面が、
  これまでどおり翻訳された案内を出すこと（`--lang ja` も）
