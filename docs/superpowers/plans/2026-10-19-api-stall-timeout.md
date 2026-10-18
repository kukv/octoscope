# api: 本文が止まったら打ち切り、時間切れを 1 つの形で返す（#155）

## 問題

api バックエンドの上限（`responseTimeout` = 16 秒）は接続・TLS・ヘッダ待ちにしか掛からない
（`internal/github/api/api.go` の `New`）。ヘッダが返ったあとに回線が止まると、`send` と `post` の
`io.ReadAll` が本文を待ち続ける。画面側はほとんどが `context.Background()` を渡すので、読み込み中の
表示が終わらない。`http.Client.Timeout` は本文全体を縛り、大きいログ zip を途中で切るので採らなかった
（`docs/superpowers/2026-09-13-phase4-actions-followups.md`）。

もう 1 つ。ヘッダ待ちの時間切れは `reach GitHub: net/http: timeout awaiting response headers` のまま返り、
`ErrTransient` にも `DeadlineExceeded` にもならない。そのため 1 行の通知に出ず、`readURL` は引き直して
待ち時間が倍になり、起動時のリポジトリ取得（root、20 秒）は「時間切れ」ではなく「リポジトリなし」と出す
（#152 の最終レビューで見つけた穴）。利用者の判断で、この PR で揃える。

## 方針

- **止まりの判定:** 全体の時間ではなく、本文が進まない時間で切る。`stallTimeout = 16 * time.Second`
  （`responseTimeout` と同じ値・同じ根拠。どちらも「GitHub が何も返さない時間」の上限）。`var`
- `send` と `post` の `c.http.Do` から `io.ReadAll` までを、新しい 1 つの関数にまとめる:

  ```go
  // exchange sends req and reads the whole answer. resp.Body is closed when it
  // returns; the body is in the second result.
  func (c *Client) exchange(req *http.Request) (*http.Response, []byte, error)
  ```

  - `ctx, cancel := context.WithCancelCause(req.Context())`、`defer cancel(nil)`、`req = req.WithContext(ctx)`
  - `Do` のエラー: 呼び出し側の ctx（`req.Context()` の元）が終わっていなければ、`net.Error` で
    `Timeout()` が真のものを時間切れにする（接続・TLS・ヘッダ待ちのどれも）。
    文面は `fmt.Sprintf("reach GitHub: no answer in %s", responseTimeout)`。それ以外は今までどおり
    `fmt.Errorf("reach GitHub: %w", err)`
  - ヘッダが届いたら `timer := time.AfterFunc(stallTimeout, func() { cancel(errStalled) })`、`defer timer.Stop()`。
    本文を `progress` で包み、`Read` が 1 バイト以上返すたびに `timer.Reset(stallTimeout)`
  - `io.ReadAll` のエラー: `context.Cause(ctx) == errStalled`（`errors.Is`）なら時間切れ。
    文面は `fmt.Sprintf("read answer: nothing arrived for %s", stallTimeout)`。それ以外は今までどおり
    `fmt.Errorf("read answer: %w", err)`
  - 判定は読み取りのエラーではなく `context.Cause` で行う。ctx を止めたときに本文の読み取りが返すのは
    `context canceled` で、それが呼び出し側のキャンセルか、こちらの止まり判定かはエラーからは区別できない
- **エラーの形:** cli と同じ形の型を api に置く（`transport.go`）。共有しない。置き場所の候補が
  `internal/github` しかなく、2 つの利用者のために型を公開する理由がまだ弱い（4 行の重複を取る）

  ```go
  // timedOut is a request this package gave up on. It reads as transient to the
  // UI, which puts it on the one-line notice like a 502, and as a deadline to
  // the read paths, which must not ask again: a connection that stalled once is
  // not one a second try will rescue, and a retry doubles the wait.
  type timedOut struct{ msg string }

  func (e *timedOut) Error() string   { return e.msg }
  func (e *timedOut) Unwrap() []error { return []error{github.ErrTransient, context.DeadlineExceeded} }
  ```

  gateway の `wrap` が `domain.ErrTransient` に移し、root の `resolved` は #152 で `DeadlineExceeded` を
  見るようにしたので、画面側の変更は無い
- **再試行しない:** `readURL` の条件に `errors.Is(err, context.DeadlineExceeded)` を足す。
  `gql.Read` は #152 で足してある
- `send` と `post` が返すものは変えない（`send` は `resp.Header` と本文をエラーのときも返す、
  `post` は本文と `classify` の結果）
- `responseTimeout` のコメントの「本文は縛らない」を、`stallTimeout` が縛ることに合わせて直す

却下: `http.Client.Timeout`。本文全体を縛るので、進み続けている大きい zip まで切る。

却下: `net.Conn` の読み取り期限で止まりを見る。HTTP/2 では 1 本の接続を複数のリクエストが共有するので、
接続の期限は 1 つのリクエストの止まりを表さない。

却下: 止まったら 1 回引き直す。固まった回線は 2 回目も固まりやすく、待ち時間が倍になる（#152 と同じ判断）。

範囲外: 本物の GitHub で止まりを起こす手動確認。止まる回線を作れないので、`httptest` のテストで確かめる。

## タスク

1. テスト（赤）。`internal/github/api/transport_test.go` に、`stallTimeout` / `responseTimeout` をテスト中だけ
   短くして（`t.Cleanup` で戻す。既存の `TestAServerThatNeverAnswersDoesNotHangForever` と同じ形）次を書く。
   サーバーは `httptest`。リクエスト数を数え、ハンドラはテスト終了時に抜けられるようにする（`r.Context()` か
   閉じるチャンネルで待つ。`srv.Close` が待ち続けないように）
   - REST（`c.read`）: ヘッダと本文の一部を `Flush` してから止まるサーバー → `stallTimeout` で返り、
     `errors.Is(err, github.ErrTransient)` と `errors.Is(err, context.DeadlineExceeded)`、リクエストは 1 回
   - GraphQL（`c.post`）: 同じく止まるサーバー → 同じく時間切れ
   - 少しずつ書き続けるサーバー（`stallTimeout` より短い間隔で 1 バイトずつ、合計は `stallTimeout` の
     3 倍以上かかる）→ 切られずに全部届く
   - ヘッダを返さないサーバー → `responseTimeout` で時間切れの形、リクエストは 1 回
     （今は引き直すので 2 回になり、`ErrTransient` でもない）
   - 呼び出し側のキャンセル・呼び出し側の短い期限で終わったときは `ErrTransient` にならない
     （ヘッダ待ちの間と、本文の途中の両方）
   - エラー文が上限時間を名乗る（`strings.Contains` で `16s` のような `stallTimeout` / `responseTimeout` の
     文字列）
2. 実装して緑にする（`exchange`、`progress`、`errStalled`、`timedOut`、`stallTimeout`、`send` / `post` の
   置き換え、`readURL` の条件、`responseTimeout` のコメント）。既存の `TestASlowBodyIsNotCutOff` が
   緑のままであることも見る
3. テストが空振りしていないことを確かめる。タイマーの `Reset` をやめる・`readURL` の条件を戻す・
   `Do` の時間切れの判定で呼び出し側の ctx を見るのをやめる、をそれぞれ一時的に入れて、対応するテストが
   落ちることを見る
4. `docs/superpowers/2026-09-13-phase4-actions-followups.md` の「タイムアウトは本文のダウンロードを縛らない」に、
   #155 で入れたことを 1 行追記する
5. `make check`（lint の出力が `0 issues.` であることも見る）
6. 実際に起動して確かめる。`GH_TOKEN=$(gh auth token)` で `--api` を付けて、テスト用リポジトリ
   （bright-room-sandbox/octoscope-test-repository）の Work 板・詳細・diff・失敗したジョブのログが今までどおり
   開けることを見る。`--lang ja` の 80 桁でも見る。起動できない環境なら、そう記録してテストだけにする
