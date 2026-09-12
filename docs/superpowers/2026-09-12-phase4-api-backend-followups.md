# api バックエンドスライスの積み残し

Phase 4 スライス 4-1「共通 GraphQL 層」に続くスライス 4-2「`internal/gh/api` 本体」
（`docs/superpowers/plans/2026-09-12-phase4-api-backend.md`、Task 1〜7、全部完了。
実装は Task 1〜7、コミット 15 本）で見つかったが、このスライスでは直さなかったもの。
**直さないと決めた理由も書く。**

## 繰り越し

- **REST 系**（`label list` / assignees の候補 / コメント / close・reopen / ラベルと
  担当者の編集 / `pulls/{n}/files` / `search repos` / `repo list` / `user/orgs`）は 4-3。
- **Actions**（`RerunWorkflow` / `JobLog`）は 4-4。`gh` の `pkg/cmd/run/view/view.go` を
  読んでから実装方針を決めること（実測資料に推定が 2 行残っている）。
- **`main.go` のバックエンド選択と認証エラー画面** も 4-4。設計 §10 は当初これを
  4-2 の行に書いていたが、`usecase.New(src source, ...)` の `source` interface
  （`internal/usecase/usecase.go:113`）を `api.Client` が満たすのは REST 系（4-3）と
  Actions（4-4）が揃ってからで、4-2 終了時点では配線がコンパイルできない。
  2026-09-12 に承認を得て §10 を訂正した（同じ PR、Task 8）。

## このスライスで決めたこと（記録する価値があるもの）

### `.git/config` を直接読まず `git remote get-url origin` を実行する

このリポジトリ自身がそうであるように、worktree では `.git` はディレクトリではなく
ファイルになる。`.git/config` を直接パースする実装は worktree で壊れる
（`internal/gh/api/repo_test.go` の `TestParseRemoteReadsEveryShapeGitWritesTheURLIn`
の docコメントに経緯を残した）。`git remote get-url origin` はどちらの形でも
正しく動くので、サブプロセス経由に倒した。

### カレントリポジトリの解決は 1 度だけキャッシュするが、デッドラインで打ち切られた
lookup はキャッシュせず再試行する

`sync.Once` は「git が実際に答えた（恒久的な結果）」と「このパッケージ自身の
デッドラインで打ち切られた（一時的で、再試行の価値がある）」を区別できない。
review が両者を混同する mutation を通したため、`once sync.Once` を
`mu sync.Mutex` + `done bool` に置き換え、`ctx.Err() != nil`（デッドライン経過）の
ときだけ `done` を立てずに返すようにした（`internal/gh/api/repo.go`）。**これは
`cli` バックエンドの挙動に合わせるためでもある。** `cli` は `resolveRepo` のたびに
毎回 `gh` を呼んでおり、一時的な失敗は次のリフレッシュで自然に回復する。`api` 側が
一度の打ち切りを恒久扱いしてしまうと、同じ操作で `cli` は直り `api` は直らない、
という 2 バックエンド間の非対称が生まれる。

### `parseRemote` は github.com 以外を拒む

設計 §6 の「境界: `github.com` のみを相手にする。`GH_HOST` /
`GH_ENTERPRISE_TOKEN` は見ない」（Phase 4 の範囲外）をそのままコードに落とした。
scp-like ssh / `ssh://` / `https://` のどの形でも host 部分を取り出し、
`github.com` と一致しなければエラーにする。

### transport はエラーと一緒に応答本文を返す

`internal/gh/api/transport.go` の `post` はエラーが起きても `nil` を返さず、
受け取った body をエラーと並べて返す。設計 §2 の「部分的な失敗を捨てない」
（GraphQL は一部のフィールドが解決できなくても `errors` と一緒に `data` を返すことが
あり、alias を並べた件数クエリで 1 リポジトリだけ消えても残りの件数は読みたい）を
transport の契約として持たせた。`TestAPartialAnswerComesBackWithItsBody` が唯一の
見張りで、`return nil, classify(...)` に変えると `RepoCounts` の部分応答の救済が
壊れることを確認済み。

### `internal/gh/api/parity_test.go` は `package api_test`（外部テストパッケージ）

`api` パッケージの中（`package api`）に置くと、`*cli.Client` を参照するために
`api` が `cli` を import することになり、2 つのバックエンドが互いに依存する形に
なってしまう。外部テストパッケージなら `api` と `cli` の両方を読者として import
でき、依存の向きを増やさずに「どちらも同じ `graphQLSource` を満たす」ことを
コンパイル時に確認できる。

## `internal/gh/cli` から消えたもの

4-1 で GraphQL の文書とデコードが `internal/gh/gql` に移ったのに続き、このスライスは
`cli.Client` がそれを埋め込みで使うだけの形に切り替えた。

- **メソッド 4 本:** `ListPRs` / `ListIssues` / `GetPR` / `GetIssue`
  （埋め込んだ `*gql.Client` の同名メソッドが代わりに見える）。`RepoName` は
  削除ではなく `c.Client.RepoName(ctx, c.repo)` への委譲に置き換わった。
- **`item.go`:** ファイルごと削除（`prJSON` / `issueJSON` / `toPRs` / `toIssues`）。
  他に読み手が無いことを `grep` で確認済み。
- **定数:** `prListFields` / `prViewFields` / `issueListFields` / `issueViewFields`。
- **testdata:** `pr_list.json` / `pr_view.json` / `issue_list.json` / `issue_view.json`
  の 4 fixture。`README.md` からも該当節と `--state all` の段落を削除した。
- **それらを見ていたテスト:** `TestListPRs` / `TestListPRsEmpty` /
  `TestGetPRParsesDetailFields` / `TestGetPRWithRepoOverride` / `TestListIssues` /
  `TestGetIssueWithRepoOverride` / `TestRepoName` / `TestGetPRParsesAssignees` /
  `TestListPRsParsesARecordedResponse` / `TestGetPRParsesARecordedResponse` /
  `TestRepoNameUsesPositionalArgument`、ほか。**それぞれの主張を 1 件ずつ `gql` 側の
  テストに引き取れているか追跡した**（Task 7 の消したテスト表）。2 件は
  引き取り手が無いことが判明し、`gql` の fixture（`pr.json` を #61 → #59、
  `issue.json` を #50 → #54 に録り直し）でラベル・assignee・コメントを持つ
  item に差し替えて回収した。中身は録る前に読み、私有情報・インフラ構成への
  言及が無いことを確認済み（Task 7 fix round 1）。

## 実測

`gh` のサブコマンドと新しい `.graphql` 文書が同じ結果を返すことを、実際の
リポジトリに対して突き合わせた（golden では確かめられない）。

| 対象 | `gh` | 文書 | 一致 |
|---|---|---|---|
| `cli/cli` の PR 一覧（`--limit 100`） | 56 件 | 56 件 | 完全一致（差分なし） |
| `cli/cli` の issue 一覧（`--limit 100`） | 100 件 | 100 件 | 完全一致（差分なし） |
| `kukv/octoscope` の issue 一覧 | 2 件 | 2 件 | 完全一致（差分なし） |

`cli/cli` の issue 一覧は `first: 100` の天井ちょうどに当たっており、**天井と
順序の両方を実規模で確かめられた。** 一方 PR 側は `cli/cli` でも 56 件までしか
無く、天井には届いていない。**この実測は「天井に届くまでは一致する」ことを
示すもので、「天井を超えたときに同じ 100 件目で切れる」ことまでは検証していない**
（`first: 100` が GraphQL の 1 ページの上限であり `gh pr list --limit 100` と
同じ数字である、という設計上の前提は計画段階の読解であって、この実測が
確かめたのは PR 側では 56 件、issue 側では 100 件ちょうどの一致のみ）。

## 見つかったが直さなかったこと（理由つき）

### 1. `topLevelConnectionArgs` の「かっこの対応が取れていない」失敗メッセージがどの文書かを名指ししない

Task 1 で文書テキストの検証に使うヘルパー。かっこの対応が壊れているときの
失敗メッセージが「この文書」としか言わず、複数の文書を検証する呼び出し元では
どれが壊れたか分からない。

**直さなかった理由:** 見つけたのは Task 1 の再レビューで、Minor として deferred
のまま残った。実害は「デバッグ時にもう一歩ログを読む」程度で、この波の
スコープ（文書と decode の整合）を超える。

### 2. `items.go` が 301 行になり、`RepoName` が「アイテムではなくリポジトリ」の関心事を抱えている

Task 3 のブリーフが `RepoName` の置き場所として `items.go` を指定したが、
意味的には `repo_counts.go`（`internal/gh/gql`）のほうが近い。

**直さなかった理由:** ブリーフの指示どおりに置いており、動くコードを動機なく
動かす変更になる。ファイルの再編は振る舞いを変えない純粋な整理であり、
この波（機能追加）のスコープではない。

### 3. `TestTokenWithoutOneIsUnauthenticated` の `gh.IsFatal` の主張が独立して落ちない

`ErrUnauthenticated` であることの主張が既に成り立てば `IsFatal` の主張も
自動的に成り立つため、この行だけを削っても他の壊し方をしない限り RED にならない。

**直さなかった理由:** 計画の文章がこの形を指定しており、実装者はそのまま
書き写した。実害はなく、「なぜ `ErrUnauthenticated` というセンチネルを選んだか」を
示す記録として読めるので、直す価値より残す価値のほうが大きいと判断した。

### 4. `parseRemote` が `.git` サフィックスと host の大文字小文字に厳密すぎる

`".GIT"` や `"GitHub.com"`、`:443` のような明示ポート付きの host を拒否・
誤解析する。

**直さなかった理由:** `git` と GitHub 自身がこれらの形で remote URL を書くことは
無い。実在しない入力への防御であり、`.claude/rules/testing.md` の
「起こり得ないシナリオに対するエラーハンドリングは書かない」と同じ理由で
見送った。

### 5. `parity_test.go` の主張の大半が同語反復になっている

`graphQLSource` の大半のメソッドは `cli.Client` と `api.Client` がどちらも
埋め込んだ `*gql.Client` からそのまま昇格しているので、「両方が interface を
満たす」という主張はコンパイラの型検査と同じことを言っているだけになる。
実質的に確かめているのは各クライアントが自前で実装した `RepoName` と
`OpenWeb` だけである。加えて `graphQLSource` は `internal/usecase` の
`source` interface を手でコピーした部分集合であり、`source` 側の変化に
自動で追随しない。

**直さなかった理由:** テストとしては無害（誤って緑になる方向の欠陥ではない）で、
「両方が同じ具体型を返す」という設計 §7 の要求をコンパイル時に固定する役割は
果たしている。`source` との自動追随は `internal/usecase` 側に生成的な仕組みを
足す話になり、この波の範囲を超える。

### 6. 一覧が空のときの経路に、それを守るデコードのテストが無い

`gql` の fixture はどれも 1 件以上の要素を持つ録りものしか無く、
「ノード 0 件」を録ったものが無い。`len == 0` のときに `nil` を返すか
空スライスを返すかの区別も含めて未検証。

**直さなかった理由:** 実データを録るには実在する「PR/issue が 0 件の
リポジトリ」が要り、それを検証のためだけに用意するのは録りものの趣旨
（実物の応答を保存する）から外れる。実害も小さい（`make([]gh.PR, 0)` の
自明な経路）。Task 7 の実装者・レビューとも同じ結論で deferred にした。

### 7. `testdata/README.md` の `pr_list.json` に関する記述が空振りだった

`pr_list.json`（削除済み）を `--state all` で録った理由を「`ParseItemState` の
3 状態をまとめて確かめるため」と書いていたが、実際にはどのテストも state の
値を主張していなかった。

**直さなかった理由:** ファイル自体を削除したのでこの記述は既に無くなっており、
「元から空振りだった」という事実だけを記録に残す。record 済みの他の README
記述への波及は無い。

## 実端末での確認の依頼

TTY が要るためこの環境では代行できない。`gh` を PATH から外し `GH_TOKEN` だけを
設定した状態で、次を確かめてほしい。

- Work 板・Repos タブ・Search タブがどれも実際に引けること
- `--repo` を付けない状態で、カレントのリポジトリが Repos タブに出ること
- git リポジトリでないディレクトリで起動しても落ちず、Repos タブが
  カレントのリポジトリ無しで出ること
- `GH_TOKEN` を空にしたときの挙動。**4-4 まで認証エラー画面は無いので、
  この時点では「エラーが出る」ことまでしか確認できない。** `main.go` の
  バックエンド選択と認証エラー画面自体が 4-4 の作業であり、このスライスの
  完了条件には入っていない。
