# diff: 書き込みのたびに全体を取り直さない設計（#227）

## 1. 問題

diff ビューは、次の 3 つの操作のあとで `fetchReview`（`review.graphql`）を呼び、スレッドを全部取り直す
（`internal/app/presentation/tui/diff/diff.go`）。

| 操作 | キー | 取り直す場所 |
|---|---|---|
| 行コメントを送る | `c` → `Ctrl+S` | `commentPosted` |
| 未送信のコメントを破棄する | `X` → `y` | `discarded` |
| レビューを送る | `v` | `submitDone` |

取り直しは操作ごとに独立して飛び、届いた順に `reviewArrived` が画面を置き換える。回線が遅いと届く順番が
入れ替わり、古い状態で上書きされる。2026-10-18 にテスト用リポジトリ #2 で、コメントを 3 件続けて付けた直後に
破棄すると、GitHub では破棄できているのに 3 件とも「未送信」のまま残った（`r` で消える）。

操作 1 回につき、GitHub との通信も 2 回かかる（書き込みと取り直し）。

## 2. 方針

**全体を取り直すのは、diff を開いたときと `r` を押したときだけにする。** 3 つの操作では、書き込みの結果から
画面の状態を作る。取り直しが重ならなくなるので、届く順番の入れ替わりはこの 3 つの操作からは起きなくなる。

| 操作 | 変えたあと |
|---|---|
| 行コメントを送る | `addPullRequestReviewThread` の返事に作ったスレッドの中身を選び、それをスレッドの一覧に足す |
| 破棄する | 未送信のコメントを一覧から消し、コメントが残らないスレッドも消す。`Pending` を空にする |
| レビューを送る | 未送信のコメントを送信済みにする。`Pending` を空にする |

破棄と送信をアプリの中で済ませてよいのは、GitHub がすることが決まっているからである。
`deletePullRequestReview` が消すのは未提出のレビューとそのコメントだけで、未提出のレビューは本人にしか
見えないので、一覧の未送信コメントはすべて本人のものである。レビューを送ると、そのコメントがそのまま
公開される。

却下: 送ったコメントの表示を、送信の返事を使わずにアプリの中で組み立てる。投稿者のログイン名と投稿時刻を
アプリが埋めることになり、GitHub が実際に保存したものと食い違い得る。

却下: 取り直しに世代番号を振り、古い返事を捨てる（Search タブの `gen` と同じ形）。届く順番の問題は直るが、
通信の回数は減らない。取り直しをやめれば要らない。

## 3. 層ごとの変更

### 3.1 gql（`internal/github/gql`）

- `add_thread.graphql` の `thread` で、`review.graphql` のスレッドと同じフィールドを選ぶ
  （`id isResolved isOutdated path line originalLine diffSide comments(first: 50) { pageInfo nodes { ... } }`）
- `Client.AddReviewThread` は `(ReviewThread, error)` を返す

作ったばかりのスレッドのコメントは 1 件なので、コメントのページ送り（`threadComments`）は要らない。

### 3.2 gateway（`internal/app/adapter/gateway/gh`）

- `backend` の `reviewer.AddReviewThread` を `(gql.ReviewThread, error)` にする
- `Gateway.AddReviewThread` は `(domain.ReviewHandle, domain.ReviewThread, error)` を返す。スレッドは
  既存の `toReviewThread` で変換する

### 3.3 usecase（`internal/app/usecase`）

- `reviewer.AddReviewThread` と `Usecase.PostLineComment` を、スレッドも返す形にする

### 3.4 domain（`internal/app/domain`）

破棄と送信で一覧がどう変わるかは、レビューという概念の決まりなので domain に置く。

- `func (c ReviewContext) Discarded() ReviewContext`: 未送信のコメントを消し、空になったスレッドを消し、
  `Pending` を空にする。送信済みのコメントに未送信の返信が付いたスレッドは、送信済みのほうを残す
- `func (c ReviewContext) Submitted() ReviewContext`: 未送信のコメントを送信済みにし、`Pending` を空にする

どちらも元の `Threads` を書き換えずに新しいスライスを作る（Bubble Tea のモデルは値で渡るため）。

### 3.5 diff（`internal/app/presentation/tui/diff`）

- `commentPostedMsg` にスレッドを持たせる。`commentPosted` はスレッドを `m.review.Threads` に足し、
  `reviewArrived` と同じく行を作り直す（`withRows` → `clamp` → `follow`）。`fetchReview` は呼ばない
- `discarded` は `m.review = m.review.Discarded()` にして行を作り直す。`fetchReview` は呼ばない
- `submitDone` は `m.review = m.review.Submitted()` にして行を作り直す。`fetchReview` は呼ばない

`#203` で入れた「レビューが `null` のコメントを捨てる」は残す。破棄の直後に `r` を押すと、同じ窓に当たり得るため。

## 4. 守れなくなるもの

- 他の人が同じ時間に付けたコメントは、3 つの操作のあとでは画面に出ない。出すには `r` を押す。
  今は操作のたびに取り直しているので、ついでに出ていた
- 書き込みが成功して返事を読めなかった場合（返事の形が想定と違う）、画面に足すものが無い。
  エラーとして出し、`r` で取り直せば画面は正しくなる

## 5. テスト

- gql: 録った返事（`testdata/add_thread.json`、テスト用リポジトリ #2 で 2026-10-18 に録った）を読むと、
  コメント 1 件・PENDING のスレッドになる
- gateway: `AddReviewThread` が変換したスレッドを返す
- domain: `Discarded` / `Submitted` の表テスト。送信済みと未送信が混ざったスレッド、未送信だけのスレッド、
  送信済みだけのスレッド。元の `ReviewContext` が書き換わっていないこと
- diff: フェイクの `Source` で `PRReviewContext` の呼び出し回数を数える。3 つの操作のあとで回数が増えず、
  `View()` にコメントが出る（送信）・消える（破棄）・「未送信」が外れる（送信）
- 3 つの操作のそれぞれで、`fetchReview` を呼ぶように戻すと回数のテストが落ちることを確かめる
- 実際に起動して、§1 の再現手順でコメントが消えることを確かめる
