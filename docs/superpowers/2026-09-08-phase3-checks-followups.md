# Phase 3（checks）の積み残し

checks ビュー（`docs/superpowers/plans/2026-09-07-phase3-checks.md`）を入れたときに
見つかったが、そのブランチでは直さないと決めたもの。**直さないと決めた理由も書く。**

実端末での確認は別で、`docs/superpowers/2026-09-07-phase3-checks-handoff.md` にある。

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

**位置がずれる直接の原因は下の `arrange` のバグで、そちらは設計判断を要しない。**
それを直しても残るのが、**この 2 種類が画面上は見分けられないのに振る舞いが違う**ことである。

| | ログ | 再実行 |
|---|---|---|
| App が作った check run | `enter` は取りに行くが、`gh` はジョブを持たないので失敗する | 断る（`RunID == 0`） |
| StatusContext | 断る | 断る |

解くには「再実行できる run が無い」ことを示す印を spec で決める必要がある。
グリフを分けるのか、行の右端に何か出すのか、群の外に置くのか。
**印を決めれば、下の「`enter` が出せないログを取りに行く」も同時に解ける。**

## 直し方が決まっているもの

### `arrange` で、無関係な外部 CI の状態が App の check の位置を動かす

**上の「一員に見える」の直接の原因はこれで、設計判断は要らない。**

`arrange` は `worst` と `first` をワークフロー名で引く。名前を持たない check
（App が作った check run と StatusContext）はどちらも `Workflow == ""` なので
**同じ 1 つのバケツに入り、その中で最も悪い状態の順位を全員が継ぐ**。

外部 CI の状態だけを変えて他を固定した実測（2026-09-08）:

```
外部 CI が running : sca audit secrets deps  codecov/patch  build lint test  ci/circleci
外部 CI が success : sca audit secrets deps  build lint test  codecov/patch  ci/circleci
外部 CI が無し     : sca audit secrets deps  build lint test  codecov/patch
```

`worst[""] = max(codecov=成功 1, ci/circleci=実行中 2) = 2` が `CI` の 1 を上回るので、
**codecov/patch がワークフロー 1 つ分を飛び越して security の 5 件目の位置に着地する**。
無関係な外部 CI の状態が、別の check の位置を動かしている。

Task 6 から入っているバグで、以後のどの修正も `arrange` に触れていない。
上の「印」をどう決めるかとは無関係に、`codecov/patch` が security の群の中に
並ぶべきではない。直せば `test` の次、`ci/circleci` の隣に着き、
見出しの無い 2 件が並んで、そう読める。

**`checks_mixed_*.golden` は今この誤った順序を録っている。** 直したら録り直す。

### `internal/tui/checks/checks.go` を分割する

430 行あり、`.claude/rules/architecture.md` の「300 行を超えたら責務を疑う」を
超えている。**継ぎ目はログペイン**で、`rerun.go` が既に同じ形で切り出されている。

移すもの: `log` / `logJob` / `logRow` / `hscroll` / `failedOnly` / `logPhase` と、
`startLog` / `fetchLog` / `logArrived` / `logFailed` / `clearLog` / `moveHscroll`。
残りは 300 行ほどのビューの骨格になる。

### `follow()` が見出しの上まで戻らない

カーソルの行に合わせてスクロールするので、いちばん上の check に戻ったとき
`top` が 1 のままになり、**その群の見出しだけが画面の外に残る**。
先頭の check が、どの群にも属していないように見える。

`m.row` に見出しがあるなら 1 行上を狙う、で済む。

### 全ログが空のときに何も出ない

`L` で全ログに切り替えて、その出力が空だったとき、ログ欄が白紙になる。
「失敗したステップはありません」に当たる案内が全ログ側には無い。

### `headingLine()` が 35 桁未満で溢れる

このビューで唯一、幅で切っていない行。80 桁は守れているので実害は無いが、
23 桁を切ると一覧の固定幅（22 桁）ごと溢れる。

### 22 桁の一覧が所要時間を意味のない断片に切る

`checks_mixed_ja_80.golden` の `codecov/patch  0:…` がそれで、
check 名が 13 桁を超えると所要時間が `0:…` のような読めない形で残る。
**所要時間を落とすか、名前のほうを切る**かを決める。

### `keyPress` と `key` が同じもの

`internal/tui/checks/checks_test.go` に別名が 2 つあり、テストごとに
使い分けが揃っていない。どちらかに寄せる。

## Phase 3 の残り

この計画は Phase 3 の 3 本のうち 1 本目である。

- **merge**（spec §4.4.4）— `mergePullRequest` / `enablePullRequestAutoMerge` /
  `disablePullRequestAutoMerge`
- **ページング** — 各スレッドの `comments`（`first: 50`）

どちらも `docs/superpowers/specs/2026-09-07-phase3-design.md` §7 にある。
