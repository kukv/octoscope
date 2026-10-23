# ラベル一覧を両バックエンドで同じ GraphQL 文書から取る（#156）

## 問題

ラベルピッカーの一覧は、バックエンドごとに取り方が違う。

- cli: `gh label list --json name,color --limit 100`（`internal/github/cli/cli.go`）。gh は内部で GraphQL に
  `orderBy: {field: CREATED_AT, direction: ASC}` を渡す
- api: REST の `GET /repos/{o}/{r}/labels` を全ページたどり、`id` 昇順に並べて先頭 100 件に切る
  （`internal/github/api/lists.go`）。REST は `created_at` を返さないので、`id` は作成順の近似でしかない

2026-09-13 の実測（`docs/superpowers/2026-09-13-phase4-rest-backend-followups.md`「ラベルは `id` 昇順に並べ替えて
`gh` に近づける」）で、集合は一致したが順序が 1 件ずれた。REST だけでは直せない。

## 方針（利用者が決めたこと: 共通の GraphQL 文書を 1 本足し、両バックエンドで使う）

- `internal/github/gql/labels.graphql` を足す

  ```graphql
  query ($owner: String!, $name: String!) {
    repository(owner: $owner, name: $name) {
      labels(first: 100, orderBy: {field: CREATED_AT, direction: ASC}) {
        nodes { name color }
      }
    }
  }
  ```

- `gql.Client.ListLabels(ctx, repo) ([]Label, error)` を `RepoName` と同じ形で置く（`repoVars` → `Read` → 解析）
- cli と api の `ListLabels` を消す。どちらも `*gql.Client` を埋め込んでいて、リポジトリの解決
  （cli の `effectiveRepo`・`{owner}`/`{repo}`、api の作業ディレクトリの remote）は `RepoVars` が持つので、
  埋め込みの `ListLabels` がそのまま使われる。同じ文書を投げるので、順序は作りから一致する
- 使われなくなるものを消す: cli の `listLimit`、api の `Label` 型・`id` 並べ替え・`cmp`/`slices` の import
  （他で使っていなければ）。`walkPages` は `prFiles` が使うので残す
- followups の記述（繰り越しの項目と「ラベルは `id` 昇順に〜」）に、#156 で共通文書にしたことを 1 行ずつ追記する

挙動の変化:

- api: 100 件を超えるリポジトリで、全ページの取得（最大 50 リクエスト）が 1 リクエストになる
- cli: `gql.Read` を通るので、502 のとき 1 回だけ聞き直す（他の読み取りと同じ）。時間の上限は今と同じ
  `callTimeout`（gql の transport が使うもの）

## タスク

各タスクは、まず失敗するテストを書き、実装して緑にし、検証している行を一時的に壊してテストが落ちることを
確かめてから戻す（`.claude/rules/testing.md`）。

1. **文書と解析。** `labels.graphql` と `gql.Client.ListLabels` を置く。`schema_test.go` が文書を schema に照らす。
   足りない型（`LabelOrder`・`LabelOrderField`・`OrderDirection` など）があれば、`testdata/README.md` の型リストに
   足して `schema.json` を録り直す。`testdata/labels.json` を `bright-room-sandbox/octoscope-test-repository` から
   README の手順で録り、README に対象・日付・理由を書く。テスト:
   - 録った応答から、名前と色が応答の順のまま返る
   - 送った文書が `labels.graphql` で、変数が `owner`/`name` である（`orderBy` と `first` は文書に書かれているので、
     文書の同一性で確かめる）
2. **バックエンドの差し替え。** cli と api の `ListLabels` を消す。既存テストの扱い:
   - cli の `TestListLabels`・`--limit` を見るテスト、api の `id` 並べ替え・全ページのテストは、確かめていた挙動が
     無くなるので消す
   - 時間切れ・一時的な失敗などを `ListLabels` を例に確かめているテストは、確かめている挙動（transport・classify）が
     ラベルに固有でなければ、同じ経路を通る別の読み取りに乗り換える。gql 経由になって経路そのものが変わるものは、
     中身を読んで判断する
   - 各バックエンドで `ListLabels` が `labels.graphql` を送ることを 1 本ずつ確かめる（埋め込みが効いていること）
3. **ドキュメント。** followups に追記する
4. `make check`（lint の出力が `0 issues.` であることも見る）
5. 実際に起動して確かめる。`--repo bright-room-sandbox/octoscope-test-repository` で、cli と `--api` の両方で
   ラベルピッカーを開き、並びが同じで `gh label list` とも一致することを見る。`--lang ja` の 80 桁でも見る。
   他人のリポジトリは使わない
