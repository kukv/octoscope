# Phase 3（checks）の積み残し

checks ビュー（`docs/superpowers/plans/2026-09-07-phase3-checks.md`）を入れたときに
見つかったが、そのブランチでは直さないと決めたもの。**直さないと決めた理由も書く。**

実端末での確認は別で、checks は `docs/superpowers/2026-09-07-phase3-checks-handoff.md`、
merge は `docs/superpowers/2026-09-08-phase3-merge-handoff.md` にある。

**2026-09-08 に、直し方が決まっていたものを全部片付けた。**
`arrange` のバケツ分け、`enter` / `L` が出せないログを取りに行く件、`follow()` が
見出しの上まで戻らない件、全ログが空のときの案内、`headingLine()` の溢れ、
22 桁の一覧が所要時間を切る件、`keyPress` と `key` の重複、そして
`checks.go` の分割（ログペインを `log.go` へ）。`checks_mixed_*.golden` は録り直した。

**2026-09-08 に、最後に残っていた設計判断も決まった。この文書に未決の項目は無い。**

## 決まったこと

### run を持たない check は「その他」の見出しでまとめる

GitHub の check run は、`checkSuite.workflowRun` が `null` のことがある。
Codecov や Sonar のように **GitHub App が作った check run** がそれで、
`internal/gh/cli/checks.go` はこれを `Kind = CheckKindRun` / `RunID = 0` として返す。

このビューは当初「見出し＝再実行できるワークフローの run がある」という約束にしたので、
run の無い check には見出しを描かなかった。結果として、そういう check は
**どの群にも属さない行として、直上の群の続きに見えていた**。
同じ形は StatusContext（外部 CI）でも起きる。

**決めたこと（2026-09-08）:** run を持たない check を、**「その他」という 1 つの
見出しの下にまとめて末尾に置く**。App が作った check run と外部 CI の StatusContext は
分けない。どちらも「run が無いので再実行もログも無い」という同じ理由で同じ扱いを
受けるので、利用者から見た違いが無く、2 つに割ると見出しが増えるだけである。

**見出しの意味を変えた。** 見出しは「再実行できる run がある」の印ではなく、
**群の名前**である。再実行とログは check ごとの性質で、run を持たない check では
`R` と `enter` が理由を出して断る。この振る舞いは変わっていない。
spec §4.4.3 のモックアップと箇条書きも、この意味に合わせて直した。

**採らなかった案（見出しでまとめる形を選んだのはユーザーの判断で、以下は実装側から
見た理由である）:** 行の右端に「再実行できない」印を出す案は、22 桁しかない一覧で
所要時間と場所を奪い合う。インデントを外して群の外に置く案は、見出しが無いままなので
「直上の群の続きに見える」という元の問題が残る。並び順（`arrange`）は変えていない。

## 見つかったが直していないこと

### リポジトリ名の解決に短い競合がある

`internal/tui/repo/repo.go` の `fetchRepoName` はリポジトリ名を非同期に取ってくる。
それが届く前に `s`（checks）や `d`（diff）を押すと、タイトルは名前の無いまま開く。
これはこのブランチが直したはずの「名前が無い」状態そのもので、届くまでの一瞬だけ
窓が残っている。

**直さなかった理由:** 次に開き直せば名前は付く（`fetchRepoName` が終わっていれば
タイトルは正しく描かれる）ので窓は狭く、実害は小さい。加えて、直すならこのブランチの
外に置くべき変更になる。`internal/tui/app/app.go` の `resolveRepo` は起動時に
同じ `gh repo view` をすでに叩いており、その結果（`repoResolvedMsg`）は
2 つの bool しか残さず名前を捨てている。名前を持たせて Repos ビューに渡せば、
この競合を閉じるのと同時に `gh repo view` の重複呼び出しも消える。

これは `resolveRepo` 側の設計を変える話で、checks 側の範囲ではないので別の作業とする。

## Phase 3 の残り

Phase 3 は完了した。checks、merge（spec §4.4.4）、ページング（各スレッドの
`comments` が 50 件を超えても取れること）の 3 本が全部入った。
上の設計判断も決まったので、残りは無い。

`internal/gh/cli/thread_comments.graphql` は PR #59 のスレッド
`PRRT_kwDOTVXF-M6fwhR-` に対して実際に叩いて確認した。
`data.node.comments.nodes` にコメント 1 件（`body` / `createdAt` /
`author.login` / `pullRequestReview.state` を含む）が返り、`pageInfo` も
一緒に返ってきた。
