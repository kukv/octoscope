# checks: ステップの区切りが無いログを「failed steps」と呼ばない（#149）

## 問題

GitHub は run のログ zip にステップごとのエントリを入れなくなった。api バックエンドは
`failedOnly` に関わらずジョブ全体のログを返し、全行のステップ名が `UNKNOWN STEP` になる。
gh も同じ（`gh run view --log-failed` が同じ語を出す）。それでもログペインの見出し
（`internal/app/presentation/tui/checks/render.go` の `logModeLabel`）は既定で `failed steps` と出るので、
失敗したステップだけが表示されていると誤解させる。

**2026-10-18 の実測:** 今日の run（`kukv/octoscope` の CI、テスト用リポジトリの失敗する run）の zip には
ステップごとのエントリがあり、gh 2.101 の `--log-failed` もステップ名を付けて返す。9 月の run
（`34695737662`）は今ダウンロードしてもステップごとのエントリが無い。区切りの無いログになるのは、
少なくとも古い run である。

## 方針

ステップ名が分からない行を、domain では `LogLine.Step == ""` で表す。`UNKNOWN STEP` は gh の語で、
TUI が文字列で判定すると GitHub の値が UI に漏れる（`.claude/rules/architecture.md`）。

1. `internal/github` に `UnknownStep = "UNKNOWN STEP"` を置き、api の `joblog.go` の `unknownStep` を
   それに置き換える
2. gateway の `toLogLine` で、`Step == github.UnknownStep` を `""` にする
3. TUI: `failedOnly` でも、届いたログの全行で `Step == ""` なら、見出しを新しい文言
   `checks.log_whole_job`（en `whole job log` / ja `ジョブ全体のログ`）にする。`buildLogLines` は
   `Step == ""` の見出し行を描かない（`UNKNOWN STEP` という見出しに意味が無いため）

ステップ名が 1 行でも付いていれば、今までどおり `failed steps` と出す。ステップ境界の復元
（steps API の時刻との突き合わせ）は別 Issue（minor）に切り出す。

## タスク

1. テスト（赤）: gateway で `UNKNOWN STEP` の行が `Step == ""` になる
2. テスト（赤）: checks で、`failedOnly` のまま全行 `Step == ""` のログが届くと、見出しに
   `checks.log_whole_job` が出て `checks.log_failed_only` が出ない。ステップ名のある行が混ざれば
   `failed steps` のまま。`UNKNOWN STEP` の見出し行が描かれない
3. 実装して緑にする。カタログは en / ja の両方に足す
4. `make golden`。ログを描く golden があれば、`UNKNOWN STEP` の見出しが消えるかを diff で確かめる
5. `make check`
6. 実際に起動して確かめる。テスト用リポジトリに失敗するワークフローを置き、checks で `enter` を押して
   見出しを見る。`--lang ja` と 80 桁でも見る。gh / `--api` の両方で
