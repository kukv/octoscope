# api バックエンド: GitHub から来た文字列の制御文字を caret 表記にする

## 問題

セキュリティ監査 run-1 の確定指摘 `octoscope:adapter/gateway/gh:remote-text-control-chars-to-terminal`（Low）。

- `--api` のときに限り、PR・Issue のタイトルや本文、コメント、ラベル、diff に入っている C1 制御文字
  （U+0080〜U+009F）や、DCS/APC/SOS/PM（`ESC P` `ESC _` `ESC X` `ESC ^`）が、端末への出力にそのまま残る
  （監査時にループバックのハーネスで確認済み）。書けるのは閲覧しているリポジトリに PR や Issue を書ける人で、
  公開リポジトリなら誰でもよい
- gh バックエンドでは起きない。gh は `gh api` の出力の制御文字を caret 表記（`^[`）に変えて返し、
  `gh pr diff` はエスケープを含む diff の出力を拒否する
- api バックエンドで無害化しているのはジョブログ（`internal/github/api/joblog.go` の `sanitizeControls`）だけ

## 方針

- **gh と同じことを api バックエンドの入口でする。** gh が `gh api` の出力に対してしているのと同じく、
  JSON の本文の中の制御文字を caret 表記にしてからデコードする。デコード後の型を 1 つずつ見て回るのではなく、
  バイト列の段階で 1 か所で済ませる
- `internal/github/api/sanitize.go` を作り、`joblog.go` の `caret` をそこへ移す。足す関数は 2 つ:
  - `caretJSON(b []byte) []byte`: JSON の本文を先頭から読み、`\uXXXX` のエスケープが制御文字
    （C0 のうち `\t` `\n` `\v` `\r` 以外と、C1）を表すなら、そのエスケープを caret 表記の文字に置き換える。
    UTF-8 の C1（`0xC2 0x80`〜`0xC2 0x9F`）も同じく置き換える。`\\` などの 2 文字のエスケープは
    2 文字ごと写す（`\\u001b` を誤って置き換えないため）。caret 表記が `^\`（0x1C）になるときは、
    JSON の中なので `^\\` と書く
  - `caretText(s string) string`: 生のテキスト（diff）の制御文字を caret 表記にする。`\t` `\n` `\v` `\r`
    は残す（`caret` と同じ）。UTF-8 として読めない 0x80〜0x9F の 1 バイトも C1 として caret にする
- 掛ける場所:
  - `transport.go` の `post` の応答（GraphQL の全応答）
  - `diff.go` の `PRDiff` の生の diff（`Raw`）
  - `prFiles` の各ページ（ファイル名とパッチを PR の作者が書ける）
  - ジョブログ（`joblog.go`）とジョブの JSON（`checks.go:67`）は今のまま。ステップ名は zip の中の
    ファイル名と突き合わせてから `sanitizeControls` に通しているので、先に caret にすると突き合わせが外れる。
    zip の本文にも掛けない
  - リポジトリ名・組織名・担当者のログイン名の REST 応答には掛けない。GitHub がこれらの名前に使える文字を
    英数字と `-` `_` `.` に限っているため
- ESC で始まる CSI と OSC は Bubble Tea が既に落としているが、caret 表記にすれば一緒に見えるようになる。
  gh バックエンドと同じ見え方になる

却下: 描画の直前（presentation）で無害化する。描画する箇所は多く、1 つ漏れると元に戻る。
gh バックエンドは入口で無害化しているので、それに揃える。

## タスク

各タスクは、まず失敗するテストを書き、実装して緑にし、検証している行を一時的に壊してテストが落ちることを
確かめてから戻す（`.claude/rules/testing.md`）。

1. **`caretJSON` / `caretText` の単体テスト。** `\u001b`→`^[`、`\u009b`→`^[`、UTF-8 の U+009D→`^]`、
   `\u001c`→`^\`（デコード後）、`\\u001b` は変わらない、`\n` `\t` は残る、日本語と絵文字（サロゲートペアの
   `😀`）は変わらない。結果が `json.Valid` であること
2. **入口のテスト。** 既存の httptest のテストに倣い、タイトルに `\u009b6n` `\u001bP$qm\u001b\\` を含む PR を返す
   サーバーに `Client` を向け、`gql` のデコード結果のタイトルが caret 表記になること。diff の `Raw` も同様
3. **ジョブログを変えないこと。** `sanitizeControls` の既存テストがそのまま通ること（`caret` を移しただけ）
4. `make check`（lint の出力が `0 issues.` であることも見る）
5. **動かして確かめる。** `--api` は tmux で試せない（GH_TOKEN が要る）ので、2 の入口のテストを
   `detail` の `View()` まで通したテストで代える
