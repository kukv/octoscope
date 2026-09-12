# REST バックエンドスライスの積み残し

Phase 4 スライス 4-3「REST 系」（`docs/superpowers/plans/2026-09-13-phase4-rest-backend.md`、
Task 1〜6、全部完了）で見つかったもの。**このスライスで直さなかったものは、
直さないと決めた理由も書く。**

## 繰り越し

- **Actions**（`RerunWorkflow` / `JobLog`）は 4-4。`gh` の
  `pkg/cmd/run/view/view.go` を読んでから実装方針を決めること。
- **`main.go` のバックエンド選択と認証エラー画面** も 4-4。設計 §10 が
  当初からここを 4-4 の行に置いている。
- **取得のタイムアウト。** `internal/gh/api` は `http.DefaultClient` を使っており
  タイムアウトが無い（4-2 の積み残しで既出）。**REST もこの Client を使うので、
  タイムアウトが無い対象が GraphQL だけでなく REST 系にも広がった。** 4-4 で
  クライアント側のタイムアウトを足すか、呼び出しごとに context のデッドラインを
  持たせるかを決める。
- **`parseRemote` が明示ポート付きの `ssh://` URL を拒む**
  （`ssh://git@github.com:22/owner/repo.git`。4-2 の積み残しの 4 番）。
  このスライスでは触っていない。次に `parseRemote` を触るときに一緒に直す。
- **`internal/gh/api/parity_test.go` は 4-4 で削除する。**
  `usecase.New(api.New(...))` がコンパイルできた時点で、コンパイラ自身が
  本物のパリティチェックになるため（4-2 の積み残しの 5 番で決定済み）。

## このスライスで決めたこと（記録する価値があるもの）

### `go-github` を足さなかった

設計 §6 の表は当初「`go-github` の REST で `api` 側に実装する」としていたが、
2026-09-13 にこれを採らないと決めた（設計 §6 の表を訂正済み）。理由は 3 つ:

1. `internal/gh/api/transport.go` の `classify` が既に REST の
   `{"message": ...}` を読んで `gh.ErrTransient` / `gh.ErrUnauthenticated` に
   振り分けている。`go-github` を足すと `*github.ErrorResponse` からの
   二度目の同じマッピングを書くことになる。
2. テストの継ぎ目が 1 つ（`Client.baseURL`）から 2 つ
   （それに加えて `github.Client.BaseURL`）に増える。
3. `go-github` が持っていて自前に無いのはページングと型付きボディだが、
   ページングが実際に要るのは `pulls/{n}/files` だけで、Link ヘッダの解析は
   15 行程度（`internal/gh/api/rest.go` の `nextLink`）で足りた。

依存は増えなかった。`go-github` も `githubv4` も `go.mod` に無い。

### ラベルは `id` 昇順に並べ替えて `gh` に合わせる

`gh label list` は内部で GraphQL に `orderBy: {field: CREATED_AT, direction: ASC}`
を渡しており、これは `id` の昇順と一致する（GitHub のラベル ID は作成順に
振られる連番のため）。REST の `GET /repos/{o}/{r}/labels` はデフォルトで
名前の昇順で返すため、`internal/gh/api/lists.go` の `ListLabels` は
`slices.SortFunc` で `id` 昇順に並べ替えてから返す。

`cli/cli`（83 件のラベルを持つ、突き合わせに使うには十分な数）に対して
2026-09-13 に実測し、`gh label list` の順序と完全一致することを確認した
（下の「実測」参照）。

### `gh pr diff` の正体

`GH_DEBUG=api gh pr diff <PR>` で 2026-09-13 に実測した。`gh pr diff` は
`GET /repos/{o}/{r}/pulls/{n}` に `Accept: application/vnd.github.v3.diff` を
付けて呼んでいるだけで、専用のエンドポイントは無い。だから `api` バックエンドの
`PRDiff`（`internal/gh/api/diff.go`）も `cli` と同じ「まず diff メディアタイプで
取り、失敗したら files API へフォールバック」という二段構えが取れた。

### `gh repo list` の正体

`GH_DEBUG=api gh repo list --limit 3` で 2026-09-13 に実測した。`gh repo list`
は GraphQL に `ownerAffiliations: OWNER` と
`orderBy: {field: PUSHED_AT, direction: DESC}` を渡している。REST の
`affiliation=owner&sort=pushed&direction=desc` が同じ条件に当たる
（`internal/gh/api/repos.go` の `ListOwnRepos`）。**これは実測であって推測では
ない。** GraphQL 側と REST 側それぞれの `--debug` 出力を突き合わせて確認した。

### `ListOwnRepos` の `owner` は空文字か Org のログインしか来ない

呼び出しは `internal/usecase/repos.go:32`（`owner` に空文字）と `:53`
（`owner` に Org のログイン）の 2 か所だけで、ユーザーの個人アカウント名が
渡ることは無い。だから `internal/gh/api/repos.go` の `ListOwnRepos` は
空文字のときは `user/repos`、そうでなければ `orgs/{owner}/repos` に投げる形で
成り立つ。この前提は interface の signature 自身は言っていないので、
`ListOwnRepos` の doc コメントに残してある。

### 録りものは 1 部だけ持つ

`sample.diff` を `internal/gh/cli` と `internal/gh` の両方に複製しかけたが、
正本は `internal/gh/testdata/sample.diff` の 1 部だけに置き、`internal/gh/cli`
側のテストは `../testdata/sample.diff` を読む形にした。中身を主張しているのは
`internal/gh` 側のパーステスト（`ParseDiff` / `ParseFilesAPI`）だけで、`cli` 側は
それを呼ぶ配線しか確認していないため。

### `.golangci.yml` の一時除外は期限どおり消した

Task 1（`rest.go`）と Task 3（`lists.go`）で `unparam` の除外を一時的に
2 件足していたが、Task 4 で `prFiles` の header 引数と `PRDiff` の
`diffMediaType` 引数の読み手が生まれたため、2 件とも削除した。これは積み残しでは
なく、「一時的な除外を約束どおり消した」という記録。

## 実測

`gh` のサブコマンドと REST が同じ結果を返すことを、実際のリポジトリに対して
突き合わせた（golden では確かめられない）。中間ファイルはスクラッチパッドに
落として `diff` を取った。

| 対象 | `gh` | REST | 一致 |
|---|---|---|---|
| `cli/cli` のラベル一覧（`id` 昇順、100 件上限） | 83 件 | 83 件 | 完全一致（順序含む、差分なし） |
| `cli/cli` の assignees 候補 | — | 21 件 | （`gh` 側に相当するサブコマンドが無いため件数のみ） |
| 自分の owner リポジトリ一覧（`--limit 20`） | 20 件 | 20 件 | 完全一致（順序含む、差分なし） |
| `octoscope` のリポジトリ検索（`--limit 5`） | 5 件 | 5 件 | 完全一致（順序含む、差分なし） |
| `kukv/octoscope#66` の PR diff のファイル数 | 8 件 | 8 件 | 完全一致 |

**すべて一致した。** ラベルの `id` 昇順の並べ替え、`ListOwnRepos` の
`affiliation=owner&sort=pushed&direction=desc`、`search/repositories` の
デフォルト順（`best match`）のいずれも、`gh` と食い違う点は見つからなかった。

assignees 候補には `gh` に直接のサブコマンドが無い（`gh pr edit --add-assignee`
の補完がこの内部で使っているだけ）ため、件数の確認にとどめた。件数自体は
`ListAssignees` が候補一覧をそのまま返すだけの薄い変換なので、リスクは小さい
と判断した。

PR diff のファイル数は 8 件で、計画段階の見込み（「5 ファイル程度」）より多い
（下の「見つかったが直さなかったこと」参照）。

## `internal/gh/cli` から移ったもの

- **`ParseDiff` と `ParseFilesAPI`**（旧 `parseDiff` / bare patch のパース）を
  `internal/gh/diff_parse.go` に移した。`cli.Client.PRDiff` と
  `api.Client.PRDiff` の両方がこれを呼ぶ。
- **`sample.diff`** は `internal/gh/testdata/` に 1 部だけ残し、`cli` 側は
  `../testdata/sample.diff` を読む（上の「録りものは 1 部だけ持つ」参照）。

## 見つかったが直さなかったこと（理由つき）

### 1. `classify` が `statusError` 呼び出し後に同じ body をもう一度 `json.Unmarshal` している

`internal/gh/api/transport.go` の `classify` は `statusError(status, body)` の
中で一度 body を `json.Unmarshal` し、GraphQL 特有の部分応答判定でもう一度
同じ body を unmarshal している。

**直さなかった理由:** 実害が無い（二重デコードのコストだけで、結果は変わらない）。
このスライスのスコープ（REST 系の実装）を超える。

### 2. テスト名の typo `TheWere` → `TheyWere`

`internal/gh/api/lists_test.go` の
`TestLabelsComeBackInTheOrderTheWereCreatedNotAlphabetically`。

**直さなかった理由:** テストの主張には影響しない。次にこのテストに触るときに
直す。

### 3. `slices.SortFunc` の比較を `cmp.Compare` で 1 行にできる

`internal/gh/api/lists.go` の `ListLabels` の並べ替えは
`slices.SortFunc(found, func(a, b labelJSON) int { ... })` を数行で書いている。
`cmp.Compare(a.ID, b.ID)` に置き換えれば 1 行にできる。

**直さなかった理由:** 挙動は同じで、読みやすさの好みの範囲。このスライスの
スコープ（振る舞いを変えない）から見ても触る理由が無い。

### 4. fixture に選んだ PR が 8 ファイルで、計画の「5 ファイル程度」より多い

Task 4 が `PRDiff` の fixture に選んだ `kukv/octoscope#66` は 8 ファイルの
変更を含み、計画段階の見込み（5 ファイル程度）より多い。

**直さなかった理由:** ファイル数が多いこと自体はテストの主張を弱めない
（`ParseFilesAPI` は件数に依存しない形状のテストのため）。録り直す実益が
無いと判断した。

### 5. `page(limit)` の 0 / 負数の境界が無テスト

`internal/gh/api/repos.go` の `page(limit)` は `limit <= 0` のときに
`pageSize` を返す分岐を持つが、これを直接確かめるテストが無い。

**直さなかった理由:** 呼び出し元（`internal/usecase/repos.go`）は常に
`seedLimit`（正の定数）しか渡していない。境界に実際に到達する経路が
無いので、テストを足しても「起こり得ないシナリオへのエラーハンドリング」に
近い。次に `page` の呼び出し元が増えたときに見直す。

## 実端末での確認の依頼

TTY が要るためこの環境では代行できない。4-2 の依頼と同じ内容
（`docs/superpowers/2026-09-12-phase4-api-backend-followups.md` 末尾）に加えて、
このスライスで新しく `api` バックエンドに乗ったコメント投稿・close/reopen・
ラベルと担当者の編集・PR diff 表示・リポジトリ検索が、実際の GitHub に対して
動くことを確認してほしい。認証エラー画面自体は 4-4 の作業であり、このスライスの
完了条件には入っていない。
