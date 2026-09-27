# auto-merge を保護ルール待ちの PR で使えるようにする（#136）

## 問題

`merge.Model.send()` は `Block != BlockNone` を `m.auto` より先に見る。
`mergeStateStatus: BLOCKED`（必須 check 待ちの典型）の PR で auto-merge にチェックしても
`enter` が何もしない。一方で `CanAutoMerge()` は `Block` を見ないので、チェックボックスと
`space` は差し出されている——**差し出したものを押させてから断っている**のが本当の不具合。

## 決めたこと

auto-merge を許すのは `BlockNone`（CLEAN を除く、従来どおり）に加えて
**`BlockProtected` と `BlockBehind`**。`CanMergeAsAdmin` と同じ線である——
この 2 つは「規則が待たせている」状態で、待てば（checks が通れば、ベースに追いつけば）解ける。
draft / conflict / dirty / 計算中は「作業が終わっていない」か「分かっていない」なので許さない。

参考: gh CLI は `--auto` のとき状態を問わず送り、断るかを GitHub に任せる
（`pkg/cmd/pr/merge/merge.go` の `isImmediatelyMergeable`）。octoscope は
「選べない項目を押させてから断らない」（設計 §4.4.4）ので、許す状態を自分で決める。

判定は**政策**なので `domain.MergeContext.CanAutoMerge()` に置く（翻訳は ACL、政策はドメイン）。

## タスク

1. **domain** — `TestAutoMergeNeedsSomethingToWaitFor` に Block ごとのケースを足して赤にし、
   `CanAutoMerge()` に Block の条件を足して緑にする。
   検証: `go test ./internal/app/domain/`
2. **merge ビュー**
   - `send()`: `m.auto`（`CanAutoMerge()` で守る）を `Block != BlockNone` より先に見る
   - `hints()`: Block がある分岐でも `m.auto` なら `key_queue` を出す
   - `autoLine()`: `CanAutoMerge()` が Block のせいで false のとき
     「待つものが無い」（`auto_unavailable_clean`）を出さない。新キー
     `merge.auto_unavailable_block` を en / ja に足す
   - テスト: `TestEnterStillSendsNothingOnABlockedPullRequest` は auto 無しのまま残し、
     「BLOCKED + auto で enter → `EnableAutoMerge`」「conflict では space が効かない」を足す。
     golden に `merge_blocked_auto`（保護ルール待ち + auto にチェック）を足す
   - 検証: `go test ./internal/app/presentation/tui/merge/`、既存 golden は不変
3. **設計 §4.4.4** — auto-merge を許す状態を明記。直接マージは従来どおり BLOCKED で塞ぐ
4. `make check`

## 画面の変化

既存の golden は変わらない見込み（既存の Block 付き fixture はどれも
`AutoMergeAllowed` が false）。変わったら止めて理由を確かめる。

## 実装中に決めたこと

- **auto にチェックしている間はキーバーから `a:admin` を外す。** `enter:queue` と両方出すと
  ポップアップの 46 桁に収まらず `esc` が落ちる（`TestTheKeyBarNeverDropsTheWayOut`）。
  チェックした時点で「待つ」を選んでいるので、押し切る鍵は外しても迷わない。
  `a` 自体は効き続け、本文の「admin 権限で押し切れます」も残る
- 既存 golden は 1 枚も変わらなかった。追加は `merge_blocked_auto` の 6 枚だけ
