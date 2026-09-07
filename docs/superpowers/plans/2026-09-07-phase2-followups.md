# Phase 2 追補 実装計画: 読み込み中の入力と、`--repo` のときの初期タブ

Part 1 / Part 2 のあとに残った 2 件。どちらも `internal/tui` だけで閉じる。

## 分かっていること（2026-09-07 に実測）

python の `pty.fork()` + `TIOCSWINSZ` で実端末を作り、実 API を相手に確認した。

- **詳細画面の `c` は「本文の読み込み中」だけ無視される。** 同じ pty・同じキーで、
  `enter` の 4 秒後に押した `c` は何も起きず、25 秒後に押した `c` は入力欄を開いた。
  読み込み中の画面はスピナー 1 行だけで、なぜ無視されたかはどこにも出ない。
  これが「`c` が反応しない」という報告の説明になる（設計 §3.4 の
  「キーイベントが `"c"` にならない」という仮説より単純で、証拠がある）
- **`--repo` を渡しても Work タブから始まる。** これは spec §4 の
  「起動直後は Work タブ」どおりの動作である

## Task 1: 読み込み中に断ったことを画面に出す

`diff` は同じ状況を `declined` の 1 行で見せている（`decline_loading`）。
`detail` にも同じ流儀を入れる。

- `internal/tui/detail/detail.go`
  - `Model` に `declined string` を足す（`diff` と同じ名前・同じ役割）
  - `handleKey` の `phase == phaseLoading` で断る 5 箇所（`c` / `x` / `v` / `l` / `a`）で
    `m.declined = i18n.T("detail.decline_loading")`
  - `itemArrived` で `m.declined = ""`。断った理由が消えないまま残らないようにする
- `internal/tui/detail/render.go`
  - `phaseLoading` の画面（スピナー 1 行）の下に `declined` を 1 行足す
- `internal/i18n/locales/active.{en,ja}.yaml`
  - `detail.decline_loading` を両方に足す
- テスト（`internal/tui/detail/detail_test.go`）
  - 読み込み中の `c` で `View()` にその 1 行が出る
  - `itemMsg` が届いたら消える
  - 書いた直後に、`declined` の代入を消して落ちることを確認する
- golden: `detail_loading_*` は表示が変わるので録り直す。`make golden` の diff を目で見る

**検証**: `make check` が通り、pty で読み込み中に `c` を押してその行が出ること。

## Task 2: `--repo` のときは Repos タブから始める

**先に spec を直す。** `docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md`
§4 の「起動直後は Work タブ」を「対象リポジトリが `--repo` で決まっているときは
Repos タブ、それ以外は Work タブ」に変える。§3.4 の優先順とも読み合わせる。

- `internal/tui/app/app.go`
  - `New` で `opts.HasRepo` なら `tab = tabRepos`
  - `repoResolvedMsg` で後から Repos タブが増えたときは**タブを移さない**。
    利用者が既に見ている画面を奪わないため
- テスト（`internal/tui/app/app_test.go`）
  - `HasRepo: true` の初期タブが `tabRepos`、`false` は `tabWork` のまま
  - 既存の `app_test.go:180` はこの新しい規則に合わせて書き換える
- golden: `app` の初期フレーム（`withRepo`）が変わるので録り直す
- README（`README.md` / `README.ja.md`）に `--repo` の説明があれば追随する

**検証**: `make check`、`git diff` した golden を目で見る、pty で `--repo` 付きの
起動が Repos タブで始まること。

## この計画に入れないもの

- `commentPostedMsg` / `commentErrorMsg` / `stateChangedMsg` / `stateErrorMsg` の
  ref ガード（picker と同じ穴だが、今回の指摘は picker だった）
- `--debug-keys`。Task 1 の再現で原因の説明が付いたので、まだ足さない
