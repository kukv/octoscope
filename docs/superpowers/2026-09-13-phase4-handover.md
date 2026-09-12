# Phase 4 の受け渡し — 実端末での確認

Phase 4 の完了条件 10（`docs/superpowers/specs/2026-09-08-phase4-design.md` §11）は
**`gh` を PATH から外し `GH_TOKEN` だけで全機能が動く**こと。

**そのうち取得系は 2026-09-13 に機械的に確認済みである。** 残っているのは
画面・キー操作・書き込み系で、これは TTY と実在のリポジトリが要るため
代行できない。下の「確認済み」を読んでから「確認してほしいこと」に進むこと。
**確認済みのものをもう一度やる必要は無い。**

## 確認済み（2026-09-13、この環境で実測）

### `api` バックエンドは起動する

`PATH` から `gh` を外し `GH_TOKEN` だけを与えて 25 秒間動かし、alt screen に
入ったまま落ちなかった。`pty` は `script -qec ... /dev/null` で割り当てた。

**ただし画面の内容は取り込めていない。** `script` 経由では alt screen に
入った後の描画が捕まらず、145 バイト（エスケープのみ）しか残らない。
`cli` バックエンドでも同じ結果なので、capture 側の制約であって `api` の
問題ではない。**だから画面そのものは未確認で、下の「確認してほしいこと」に残る。**

### `api` は 16 の取得メソッドを実データで全部答える

一時的な smoke テスト（`//go:build smoke`、確認後に削除済み）を書き、
`GH_TOKEN` だけで実際の GitHub に対して走らせた。全部成功した。

```
RepoName ok          ListPRs ok           ListIssues ok        ListLabels ok(19)
ListAssignees ok     ListOrgs ok          ListOwnRepos ok(5)   SearchRepos ok(5)
SearchItems ok(50)   RepoCounts ok(2)     ListWorkSection ok(50)

using cli/cli#14398:
GetPR ok             PRDiff ok            PRChecks ok(7)
PRReviewContext ok   PRMergeContext ok(3)
```

対象は `kukv/octoscope` と、open PR のある `cli/cli`。`kukv/octoscope` には
open PR が無かったので、PR 単位の取得（一番複雑な 5 つ）は `cli/cli` で通した。

**つまり「トークンだけで GitHub からデータが取れる」ことは確認できている。**

### 起動できないときの案内

`gh` もトークンも無い状態で起動すると、TUI に入る前に stderr へ案内を出して
exit 1 する。en / ja 両方で実行して文言を確認済み。

## 確認してほしいこと

### 準備

`gh` を PATH から外し、トークンだけを渡す。`go` と端末が要るので
`env -i` ではなく PATH を絞る形にする。

```bash
go build -o /tmp/octoscope ./cmd/octoscope
GH_TOKEN=$(gh auth token)   # 先に控えておく。この後 gh は使えない
env PATH=/usr/bin:/bin GH_TOKEN="$GH_TOKEN" /tmp/octoscope --repo kukv/octoscope
```

`gh` が `/usr/bin` にある環境では隠れないので、その場合は `gh` を含まない
ディレクトリだけを PATH にする。**起動したら、まず `api` が選ばれていることを
確かめる**（`gh` が見つかっていればそちらが使われるため、この確認にならない）。

### 画面

取得が動くことは確認済みなので、**見るのは描画とキー操作である。**

1. 3 つのタブ（Work / Repos / Search）が出て、それぞれ実データが描かれること
2. PR / Issue の詳細、diff、checks、review、merge の各画面が出ること
3. `--lang ja` で桁がずれないこと。**日本語は全角で桁を 2 つ使う。**
   80 桁の端末でキーバーが収まること

### 書き込み系（ここが主眼）

**smoke で確認したのは取得だけで、書き込みは 1 つも試していない。**
`api` バックエンドで実際に効くことを確かめてほしい。

- コメントの投稿（PR / Issue の両方）
- close / reopen（PR / Issue の両方）
- ラベルと担当者の編集。**追加と削除の両方**（片方だけ通って片方が失敗する
  経路が未解決のまま残っている。下記参照）
- レビューの送信（コメント / approve / changes requested）
- マージ（squash / merge commit / rebase のうち、そのリポジトリが許すもの）

### ワークフローの再実行

`R` キー。**失敗のみと全体の両方。** この環境では本物の CI を走らせない
ために実行しなかったので、実際に GitHub 側で再実行が始まることを
Web UI で確かめてほしい。

### 設定ファイル

リポジトリの追加・削除と、クエリの保存（`s`）・呼び出し（`Ctrl+O`）が
**次の起動でも残る**こと。

## 見たときに誤解しないための注意

### ログのステップ別表示と「失敗のみ」は現在効かない

赤いチェックのログを開くと、**全行が `UNKNOWN STEP` になり、`--log-failed`
相当の絞り込みも効かない。** ジョブ全体のログがそのまま出る。

**これは octoscope の不具合ではない。** GitHub が run ログの zip に
ステップ別エントリを入れなくなったためで、`gh` 自身も同じ状態である
（`gh run view --job <id> --log-failed` が全行 `UNKNOWN STEP` を出すことを
2026-09-13 に確認した）。詳細と、ステップ境界を steps API から復元する案は
`docs/superpowers/2026-09-13-phase4-actions-followups.md` にある。

### ラベル・担当者の編集は部分適用があり得る

追加が成功したあと削除が失敗すると、追加だけが適用された状態が残る
（`internal/gh/api/items.go`）。呼び出し側はどちらが通ったか知る手段が無い。
4-3 からの繰り越しで、UI 側の再読み込みと一緒に決めることになっている。

## 報告してほしいこと

- **桁がずれた場合**: 端末の種類とフォント、`--icons` の指定、`--lang`
- **取得が遅い / タイムアウトした場合**: そのリポジトリと run の規模。
  `api` のタイムアウトは 16 秒（`internal/gh/api/api.go` の `responseTimeout`）で、
  30 リポジトリ分の件数クエリを 8.13 秒で実測した値を切り上げたもの。
  大きい run で足りないなら、この値を見直す材料になる
- **書き込みが効かなかった場合**: どの操作か、GitHub が何と答えたか
  （画面に GitHub の原文がそのまま出る）

## 参考

- 積み残し: `docs/superpowers/2026-09-13-phase4-actions-followups.md`
- スライス 4-4 の計画: `docs/superpowers/plans/2026-09-13-phase4-actions-and-wiring.md`
- 設計: `docs/superpowers/specs/2026-09-08-phase4-design.md`
- キーとサイドバー幅は README（`README.md` / `README.ja.md`）
