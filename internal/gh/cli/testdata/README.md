# testdata

ここのファイルは**実際の `gh` の出力を録ったもの**。手で書き足さない。
GitHub の返し方が変わったときに気づけることがこの testdata の目的なので、
テストを通すために中身を編集すると意味がなくなる。

録る対象は自分の公開リポジトリ（`kukv/octoscope`）に限る。秘密情報を入れない。

## レスポンスの録りもの

`pr_list.json` / `issue_list.json` / `pr_view.json` / `issue_view.json` /
`work.json` / `pr_files.json` は、`gh` の実出力をそのまま録ったもの。
録った日: 2026-09-07、対象: `kukv/octoscope`（PR #55、Issue #50）。

パースのテストは手書きの JSON ではなくこれを読む。手書きだと「GitHub が実際には
そう返さない形」でも通ってしまい、返し方が変わったときに気づけない。

```bash
D=internal/gh/cli/testdata
gh pr list --repo kukv/octoscope --state all --json \
  'number,title,author,state,isDraft,updatedAt,reviewDecision,url,labels,headRefName,baseRefName,additions,deletions,statusCheckRollup' \
  --limit 100 | jq . > $D/pr_list.json

gh issue list --repo kukv/octoscope --state all --json \
  'number,title,author,state,updatedAt,labels,url' --limit 100 | jq . > $D/issue_list.json

gh pr view 55 --repo kukv/octoscope --json \
  'number,title,author,state,isDraft,updatedAt,reviewDecision,url,labels,headRefName,baseRefName,additions,deletions,statusCheckRollup,body,comments,assignees' \
  | jq . > $D/pr_view.json

gh issue view 50 --repo kukv/octoscope --json \
  'number,title,author,state,updatedAt,labels,url,body,comments,assignees' | jq . > $D/issue_view.json

gh api 'repos/kukv/octoscope/pulls/55/files?per_page=100' --paginate | jq . > $D/pr_files.json
```

`work.json` は `@me` を含む検索なので、**録った人が見えるリポジトリを全部なめる**。
録った時点の `kukv/octoscope` にはこの 4 つの検索に該当する項目が無かったので、
`kukv/octoscope` だけに絞ると 4 エイリアスとも `nodes: []` になり、`ListWork` の
アイテム変換（`__typename` ごとの分岐、ラベル、チェックの集約）を一つも通さない
テストになる。守るべきは「秘密情報が入らないこと」であって「対象が
`kukv/octoscope` であること」自体ではないので、絞り方を**公開リポジトリすべて**に
広げた。ノードの `repository.nameWithOwner` を集め、`gh repo view <owner/name>
--json nameWithOwner,isPrivate` で 1 つずつ private かどうかを確認し、
`isPrivate: false` のものだけを残す。

```bash
gh api graphql -F query=@internal/gh/cli/work.graphql > /tmp/work-raw.json
jq -r '[.data[].nodes[]?.repository.nameWithOwner] | unique[]' /tmp/work-raw.json
# 出てきたリポジトリを 1 つずつ確認する
gh repo view <owner>/<repo> --json nameWithOwner,isPrivate
# isPrivate:false だったものだけを allowlist に入れ、それだけを残す
jq --argjson pub '["kukv/os-setup", ...]' \
  '.data |= with_entries(.value.nodes |= map(select(.repository.nameWithOwner as $r | $pub | index($r))))' \
  /tmp/work-raw.json > /tmp/work-public.json
```

**ノード自身が公開リポジトリでも、そのタイトルや本文が別の（私有の）リポジトリの
中身を書き写していることがある。** 実際、公開リポジトリの PR 本文の中に、私有の
Terraform リポジトリ名・内部サブネット・1Password のアイテム名が出てきた実例が
あった。`gh repo view` の判定は「ノードがどのリポジトリのものか」しか見ないので、
**残す前に全ノードの `title` と `bodyText` を人間が読む。** インフラ・ホスト名・
IP レンジ・認証情報・私有リポジトリ名が出てくるノードは、公開リポジトリのもの
であっても除外し、次の候補に差し替える。

件数はテストの主張に使わない（録り直すたびに変わる）。使うのは
「4 つのエイリアスが揃っていること」と「`__typename` ごとの変換結果」だけ。
それでも testdata は人が読んで確認できる大きさに保つため、**各エイリアス最大 5
件**まで残し、`kukv/*`（このプロジェクト自身のアカウント）を `bright-room/*`
より優先する。

```bash
jq '.data.reviewRequested.nodes |= map(select(.number as $n | [<採用した番号...>] | index($n)))
  | .data.yourPRs.nodes |= map(select(.number as $n | [...] | index($n)))
  | .data.assigned.nodes |= map(select(.number as $n | [...] | index($n)))' \
  /tmp/work-public.json > internal/gh/cli/testdata/work.json
```

**これは秘密情報の除去であって、「テストを通すための編集」ではない。**
前者は必須、後者は禁止。allowlist と件数の絞り込みは、その都度 `gh repo view` の
結果と本文の実読で判断する。「動くから入れる」「テストが通るから残す」は禁止の側。

`pr_list.json` は `--state all` で録っている（`ListPRs` 自身は open だけを取る）。
open / closed / merged の 3 状態が 1 ファイルに入るほうが、`ParseItemState` の
変換をまとめて確かめられるため。

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
