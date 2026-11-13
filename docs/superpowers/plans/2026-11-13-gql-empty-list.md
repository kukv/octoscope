# gql: 一覧が 0 件の応答のデコードを守る（#159）

## 問題

`gql.Client.ListPRs` / `ListIssues`（`internal/github/gql/items.go`）は、開いている PR / Issue が 0 件の
リポジトリでどう振る舞うかをテストしていない。fixture（`repo_prs.json`・`repo_issues.json`）はどちらも 1 件以上の応答で、
`nodes: []` を一度も通らない（出典: phase4-api-backend-followups #6）。

## 方針

**0 件の応答を実物で録り、エラーなく 0 件を返すことを確かめる。**

録る対象は `kukv/test-repo`（公開。2026-11-13 時点で開いている PR も Issue も 0 件）。
`.claude/rules/testing.md` の「外部レスポンスは実物を録る」に従い、合成した JSON は使わない。

**nil か空スライスかは検証しない。** 呼び出し側の `gateway/gh/lists.go` は `make([]domain.Item, len(nodes))` で
詰め直すので、どちらが返っても結果は同じになる。検証するのは「エラーにならない」「0 件」の 2 つだけ。
片方に固定すると、意味の無い違いでテストが落ちる。

却下: `{"data":{"repository":{"pullRequests":{"nodes":[]}}}}` を手で書く。Issue は合成でもよいとしているが、
実物を録れるリポジトリがある以上、規約に従う。

## タスク

1. **録る。** 次の 2 ファイルを `internal/github/gql/testdata/` に作る。

   ```bash
   D=internal/github/gql/testdata
   gh api graphql -F query=@internal/github/gql/repo_prs.graphql \
     -f owner=kukv -f name=test-repo > $D/repo_prs_empty.json
   gh api graphql -F query=@internal/github/gql/repo_issues.graphql \
     -f owner=kukv -f name=test-repo > $D/repo_issues_empty.json
   ```

   中身が `nodes: []` であることを目で確かめる。`testdata/README.md` に、`repo_prs.json` / `repo_issues.json` の
   節に並べて、録った日・対象・コマンド・「0 件であることがこの fixture の意味なので、0 件でなくなったら録り直さず
   別の 0 件のリポジトリを探す」を書く。

2. **テスト。** `items_test.go` に 2 つ足す。

   - `TestListPRsReadsAnEmptyAnswer`: `fixedTransport(t, "testdata/repo_prs_empty.json")` で `ListPRs` を呼び、
     `err == nil` かつ `len(prs) == 0`
   - `TestListIssuesReadsAnEmptyAnswer`: 同じ形で `repo_issues_empty.json` と `ListIssues`

   コメントには、このテストが無いと何が壊れるか（0 件のリポジトリを開くと Repos タブがエラーになる、
   といった退行に気づけない）を書く。

3. **空振りの確認。** `ListPRs` / `ListIssues` に一時的に `if len(nodes) == 0 { return nil, errors.New("empty") }` を
   入れて、2 つのテストが落ちることを見てから戻す。

4. `make check` を通し、`test(gql): ...`（`Closes #159`）でコミットする。
