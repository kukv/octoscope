# Phase 3（checks）の積み残し

checks ビュー（`docs/superpowers/plans/2026-09-07-phase3-checks.md`）を入れたときに
見つかったが、そのブランチでは直さないと決めたもの。**直さないと決めた理由も書く。**

実端末での確認は別で、`docs/superpowers/2026-09-07-phase3-checks-handoff.md` にある。

**2026-09-08 に、直し方が決まっていたものを全部片付けた。**
`arrange` のバケツ分け、`enter` / `L` が出せないログを取りに行く件、`follow()` が
見出しの上まで戻らない件、全ログが空のときの案内、`headingLine()` の溢れ、
22 桁の一覧が所要時間を切る件、`keyPress` と `key` の重複、そして
`checks.go` の分割（ログペインを `log.go` へ）。`checks_mixed_*.golden` は録り直した。

残っているのは、下の設計判断 1 つと Phase 3 の残り 2 本である。

## 設計の判断が要るもの

### App が作った check run が、直上のワークフローの一員に見える

GitHub の check run は、`checkSuite.workflowRun` が `null` のことがある。
Codecov や Sonar のように **GitHub App が作った check run** がそれで、
`internal/gh/cli/checks.go` はこれを `Kind = CheckKindRun` / `RunID = 0` として返す。

このビューは「見出し＝再実行できるワークフローの run がある」という約束にしたので、
run の無い check には見出しを描かない。結果として、そういう check は
**どの群にも属さない行として、直上の群の続きに見える**。

同じ形は StatusContext（外部 CI）でも起きるが、そちらは spec §4.4.3 の
モックアップが `● ci/circleci` を `✗ sca` の下に見出し無しで描いており、
**その見せ方は設計として承認されている**。

位置がずれる原因だった `arrange` のバケツ分けは直したので、
`codecov/patch` は `test` の次、`ci/circleci` の隣に着く。
ログも再実行も、この 2 種類は同じように断るようになった。

残っているのは、**この 2 種類が画面上は他の check と見分けられない**ことである。
どちらも見出しを持たないだけで、行そのものはワークフローの check と同じ形をしている。
「再実行できる run が無い」ことを示す印を spec で決める必要がある。
グリフを分けるのか、行の右端に何か出すのか、群の外に置くのか。

## Phase 3 の残り

この計画は Phase 3 の 3 本のうち 1 本目である。

- **merge**（spec §4.4.4）— `mergePullRequest` / `enablePullRequestAutoMerge` /
  `disablePullRequestAutoMerge`
- **ページング** — 各スレッドの `comments`（`first: 50`）

どちらも `docs/superpowers/specs/2026-09-07-phase3-design.md` §7 にある。
