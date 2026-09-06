# testdata

ここのファイルは**実際の `gh` の出力を録ったもの**。手で書き足さない。
GitHub の返し方が変わったときに気づけることがこの testdata の目的なので、
テストを通すために中身を編集すると意味がなくなる。

録る対象は自分の公開リポジトリ（`kukv/octoscope`）に限る。秘密情報を入れない。

## `schema.json`

`internal/gh/cli/*.graphql` が選んでいるフィールドが実在するかを
`schema_test.go` が突き合わせるための、GraphQL スキーマの抜粋。

`{ 型名: { kind, possibleTypes, fields: { フィールド名: 型名 } } }` の形。

**新しい型を選ぶクエリを書いたら、下の `--argjson types` に型名を足して録り直す。**
足し忘れると `type X is not in testdata/schema.json` でテストが落ちる。

```bash
gh api graphql -f query='
{ __schema { types { name kind
  possibleTypes { name }
  fields(includeDeprecated:true) {
    name type { kind name ofType { kind name ofType { kind name ofType { kind name } } } }
  }
} } }' > /tmp/schema-full.json

cat > /tmp/trim.jq <<'JQ'
def named: if .name != null then .name else (.ofType | named) end;
[ .data.__schema.types[]
  | select(.name as $n | $types | index($n))
  | { key: .name,
      value: {
        kind: .kind,
        possibleTypes: [ (.possibleTypes // [])[].name ],
        fields: ( [ (.fields // [])[] | { key: .name, value: (.type | named) } ] | from_entries )
      } } ]
| from_entries
JQ

jq --argjson types '["Query","Mutation","Repository","PullRequest","Issue","Actor","Label","LabelConnection","PullRequestReviewConnection","PullRequestReview","PullRequestReviewThreadConnection","PullRequestReviewThread","PullRequestReviewCommentConnection","PullRequestReviewComment","SearchResultItemConnection","SearchResultItem","PullRequestCommitConnection","PullRequestCommit","Commit","StatusCheckRollup","StatusCheckRollupContextConnection","StatusCheckRollupContext","CheckRun","StatusContext","AddPullRequestReviewPayload","AddPullRequestReviewThreadPayload","SubmitPullRequestReviewPayload","DeletePullRequestReviewPayload","PageInfo"]' \
  -f /tmp/trim.jq /tmp/schema-full.json > internal/gh/cli/testdata/schema.json
```

## `review_context.json`

`review.graphql` に対する実レスポンス。録った日: 2026-09-07、対象: `kukv/octoscope#55`。

未送信（pending）のレビューが 1 つと、RIGHT / LEFT に 1 本ずつのスレッドが入っている。
`PullRequestReviewComment` に `diffSide` が無いことが分かったときの実測で作ったもので、
**録り終えたあと pending レビューは削除済み**（`deletePullRequestReview`）。

```bash
gh api graphql -F query=@internal/gh/cli/review.graphql \
  -f owner=kukv -f name=octoscope -F number=55 | jq . \
  > internal/gh/cli/testdata/review_context.json
```

同じ内容を録り直すには pending レビューを作る必要がある。

```bash
# id は gh api graphql で pullRequest { id } を引く
gh api graphql -f query='mutation($prid:ID!){addPullRequestReview(input:{pullRequestId:$prid}){pullRequestReview{id}}}' -f prid=<PR node id>
gh api graphql -f query='mutation($rid:ID!){addPullRequestReviewThread(input:{pullRequestReviewId:$rid,path:"...",line:1,side:RIGHT,body:"..."}){thread{id}}}' -f rid=<review id>
# 録り終えたら消す
gh api graphql -f query='mutation($rid:ID!){deletePullRequestReview(input:{pullRequestReviewId:$rid}){pullRequestReview{id}}}' -f rid=<review id>
```

## `sample.diff`

`git diff` 形式のパース用。unified diff の hunk ヘッダ、追加、削除、文脈行を含む。
