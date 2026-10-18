# cli: gh サブプロセスに上限時間を付ける（#152）

## 問題

api バックエンドには 16 秒の上限（`responseTimeout`）があるが、cli バックエンドは呼び出し側の ctx を
`exec.CommandContext` に渡すだけ（`internal/github/cli/cli.go:102`）。画面側はほとんどが
`context.Background()` を渡すので、gh が固まると読み込み中の表示がいつまでも終わらない。

## 実測（2026-10-18）

ジョブログ（`gh run view --job <id> --log`、gh のキャッシュ `~/.cache/gh/run-log-*.zip` を消してから）。
microsoft/vscode の Code OSS ワークフロー（1 run あたり 38 ジョブ）:

| run | zip | 秒 |
|---|---|---|
| 39130439385 | 18,042,201 B | 18.5 / 21.2 |
| 39127852612 | 9,333,706 B | 10.8 |
| 39124533082 | 9,128,755 B | 17.1 |
| 39124010563 | 9,138,015 B | 10.8 |
| 39120176119 | 9,151,909 B | 11.3 |

gh は 1 ジョブ分を見るのにも run 全体の zip を落とすので、16 秒では普通のログが切れる。

diff（kubernetes/kubernetes）: `gh pr diff` は 142410（326 ファイル、406 で失敗）・141658・142414 とも 1.1〜1.4 秒、
ファイル API（`--paginate`）は 1.2〜2.1 秒（最大 920 KB）。16 秒の枠で足りる。

## 方針

- cli が起動する gh には、すべて上限時間を付ける。普段は `callTimeout = 16 * time.Second`
  （api と同じ値・同じ根拠）、`JobLog` だけ `logTimeout = 2 * time.Minute`
  （実測の最大 21.2 秒のおよそ 5 倍。同じ回線で 18 MB の 5 倍強の zip まで切らない）。どちらも `var`
- 上限は `c.run`（テストの差し替え点）の**外側**で付ける。`runGh` の中で付けると、`c.run` を差し替えたテストからは
  見えない。新しい `func (c *Client) exec(ctx context.Context, limit time.Duration, args ...string) ([]byte, error)`
  が `context.WithTimeoutCause(ctx, limit, errNoAnswer)` で ctx を作って `c.run(ctx, c.dir, args...)` を呼ぶ。
  `c.run(` を直接呼んでいる箇所（書き込み 8 か所と gql の `Do` クロージャ）と `read` は、すべて `exec` を通す
- `read` の署名は `read(ctx, limit, args...)` にする（`dir` はどの呼び出しも `c.dir` なので外す）。`JobLog` だけが
  `logTimeout` を渡す
- 時間切れの判定は、exec のエラーではなく **作った ctx の `context.Cause`** で行う。gh を止めたときに `cmd.Run` が
  返すのは `signal: killed` で、`DeadlineExceeded` ではない。cause が `errNoAnswer` のときだけ、こちらの上限で
  止めたことになる。呼び出し側の期限切れ・キャンセルは今までどおりそのまま返す
- 時間切れのエラーは cli の中の型で表す:

  ```go
  // timedOut is a gh call this package stopped. It reads as transient to the
  // UI, which puts it on the one-line notice like a 502, and as a deadline to
  // the read paths, which must not ask again: a gh that hung once is not one a
  // second try will rescue, and a retry doubles the wait.
  type timedOut struct{ msg string }

  func (e *timedOut) Error() string   { return e.msg }
  func (e *timedOut) Unwrap() []error { return []error{github.ErrTransient, context.DeadlineExceeded} }
  ```

  文面は `fmt.Sprintf("gh %s: no answer in %s", args[0], limit)`（例 `gh run: no answer in 2m0s`）。
  技術的な内容なので翻訳しない（`.claude/rules/errors.md`）。gateway の `wrap` が
  `domain.ErrTransient` に移すので、画面側の変更は無い
- 再試行しない: cli の `read` と `gql.Read` の条件に `errors.Is(err, context.DeadlineExceeded)` を足す。
  `gql.Read` は api と共有だが、api の時間切れは `reach GitHub: %w` で net のエラーを包むだけで
  `DeadlineExceeded` にはならないので、api の動きは変わらない
- `runGh` に `cmd.WaitDelay = time.Second` を足す。gh が子プロセスを残して標準出力を握っていると、
  gh を止めても `cmd.Run` が出力のコピーを待ち続け、上限が効かない
- `PRDiff` は今までどおり、`gh pr diff` がどう失敗してもファイル API へ回る。時間切れでも回る
  （それぞれに 16 秒。最悪 32 秒）。gh pr diff だけが固まる場合にファイル API で見られるほうを取る

却下: `JobLog` も 16 秒にそろえる。上の実測で普通のログが切れる。

却下: 出力が止まった時間で判定する。gh は終わるまで何も書かないので、ログの取得中ずっと「止まっている」に見える。

却下: 書き込みに上限を掛けない。固まった書き込みが永遠に終わらないままになる。時間切れでも反映されたかは
`r` で確かめられる（利用者が決めた）。

範囲外: 本物の gh を止める部分（kill と `WaitDelay`）の単体テスト。テストで外部プロセスを起動しない
（`.claude/rules/testing.md`）ので、手で確かめる（タスク 6）。

## タスク

1. テスト（赤）。`internal/github/cli/cli_test.go` に、`callTimeout` / `logTimeout` をテスト中だけ短くする
   ヘルパー（`t.Cleanup` で戻す）と、ctx が終わるまで返らない偽の `run` を用意して、次を書く
   - 読み取り（`ListPRs`＝gql 経由、`ListLabels`＝cli の `read` 経由）が上限で返り、
     `errors.Is(err, github.ErrTransient)`、gh は 1 回しか呼ばれない
   - 書き込み（`ClosePR`、`MergePR`）も上限で返り、`ErrTransient`
   - `JobLog` は `callTimeout` を過ぎても切られない（偽の `run` が `callTimeout` の 2 倍待ってから返す → 成功）。
     `logTimeout` を過ぎたら `ErrTransient`
   - 呼び出し側のキャンセル・呼び出し側の短い期限で終わったときは、`ErrTransient` にならない
   - エラー文が gh のサブコマンドと上限時間を名乗る（`strings.Contains` で `"gh pr"` と時間）
   - `internal/github/gql` に、`Do` が `DeadlineExceeded` を包んだ `ErrTransient` を返したら `Read` が引き直さないテスト
2. 実装して緑にする（`exec`、`timedOut`、`read` の署名、`c.run(` の置き換え、`gql.Read` の条件、
   `WaitDelay`、`callTimeout` / `logTimeout` のコメントに上の実測を書く）
3. テストが空振りしていないことを確かめる。`exec` で期限を付けるのをやめる・`read` の条件を戻す・
   `JobLog` に `callTimeout` を渡す、をそれぞれ一時的に入れて、対応するテストが落ちることを見る
4. `docs/superpowers/2026-09-13-phase4-actions-followups.md` の「`cli` バックエンドには依然タイムアウトが無い」に、
   #152 で入れたことを 1 行追記する
5. `make check`（lint の出力が `0 issues.` であることも見る）
6. 実際に起動して確かめる。PATH の先頭に置いた shim の `gh` が、印のファイルがあるときだけ
   `sleep 600` してから本物の gh を呼ぶようにする。印を置いて起動し、16 秒後に Work 板がキーバーの上の 1 行で
   失敗を出すこと、印を消して `r` で取り直せることを見る。`--lang ja` の 80 桁でも見る。
   shim の sh は sleep を子プロセスとして残すので、これが `WaitDelay` の確認を兼ねる（sleep は孤児として
   残るが、octoscope が 16 秒で返ってくれば `WaitDelay` は効いている）
