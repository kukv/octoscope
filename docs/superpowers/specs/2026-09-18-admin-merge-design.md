# 設計: 保護ルールを押し切る admin マージ

merge ポップアップは、保護ルールに止められた PR に対して理由を出したまま何もできない。
`enter` は塞がれ、利用者はターミナルを離れて `gh pr merge --admin` か GitHub の Web UI に
行くしかない。バイパスする権限を持っている利用者にとって、これは octoscope が
「見渡すだけで、最後の一押しだけ他所に行かせる」道具になっているということである。

この文書は、その一押しを octoscope の中に入れる方法を決める。

`docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md` の §4.4.4 が
merge ポップアップを決めており、この文書はそれを**覆さない**。
§4.4.4 の「`BLOCKED` / `BEHIND` は `enter` を塞ぎ、理由を出す」はそのまま残り、
「確認はこのポップアップ 1 段だけ」もそのまま守る。足すのは**別のキー 1 つ**である。

## 1. 実測して確かめたこと（2026-09-18）

**推測ではなく、gh のソースと GitHub の introspection で確認した。**

### 1.1 `gh pr merge --admin` は API を変えていない

`cli/cli` の `pkg/cmd/pr/merge/http.go` の `mergePullRequest` は、`--admin` の有無に
関わらず GraphQL の `mergePullRequest` ミューテーションを投げる。送る変数は
`pullRequestId` / `mergeMethod` / `commitHeadline` / `commitBody` / `authorEmail` /
`expectedHeadOid` で、**admin に相当する引数は無い。**

`--admin` が変えているのは `pkg/cmd/pr/merge/merge.go` の 2 箇所だけである。

- `blockedReason(status, useAdmin)` が `MergeStateStatusBlocked` と
  `MergeStateStatusBehind` のとき空文字を返す（＝ gh 自身の事前チェックが黙る）
- `shouldAddToMergeQueue()` が false になる（merge queue を迂回する）

バイパスの可否はサーバ側が判定している。**つまり octoscope の
`internal/github/gql/merge.go` の `MergePR` は、今のまま admin マージに使える。**
必要なのは新しい API 経路ではなく、`internal/app/presentation/tui/merge` の
`send()` がクライアント側で握り潰しているのを、別のキーで迂回することだけである。

これは 2026-09-18 の会話で「REST の別ルートが要る」と述べた見立ての訂正でもある。

### 1.2 `viewerCanMergeAsAdmin` は存在する

```
$ gh api graphql -f query='{__type(name:"PullRequest"){fields{name}}}'
...
viewerCanEnableAutoMerge
viewerCanMergeAsAdmin      ◀
viewerCanReact
```

### 1.3 ただし ruleset 下での挙動は未確認

`kukv/octoscope` の `main` は classic branch protection ではなく **ruleset**
（`protect-default-branch`、id 18805721）で守られている。
`viewerCanMergeAsAdmin` は ruleset より前からあるフィールドで、
ruleset に止められた PR で `true` を返すかは**確かめていない**。

確かめられなかった理由は、この時点で open な PR が 1 つも無かったからである。
merged な #98 では `mergeStateStatus: "UNKNOWN"`、`viewerCanMergeAsAdmin: false` が
返るだけで、これは判断材料にならない。

**この検証を実装の最初のステップに置く**（§6）。

## 2. 何を足すか

```
┌─ Merge #123 ──────────────────┐
│ (•) Squash and merge          │
│ ( ) Create a merge commit     │
│ ( ) Rebase and merge          │
│                               │
│ [ ] auto-merge（checks の…）  │
│ ブランチは残ります            │
│                               │
│ ⚠ 保護ルールに止められています│
│   admin 権限で押し切れます    │  ◀ 足す行
│ a:adminでマージ r:更新 esc:中止│  ◀ 足すキー
└───────────────────────────────┘
```

`enter` は今まで通り何も送らない。押し切るのは `a` だけである。

**なぜ `enter` に相乗りさせないか。** blocked な PR で `enter` に admin マージを
割り当てると、保護を破る操作が通常のマージとまったく同じ打鍵になる。
誤爆が「押し間違い」ではなく「いつも通り押した」になり、事後に
「なぜ押したのか」が本人にも説明できない。キーを分けることが確認を 1 段
増やさずに済ませるための条件であり、§4.4.4 の「確認はこのポップアップ 1 段だけ」を
守りながら安全側に倒す唯一の手である。

`a` を選ぶのは、ポップアップが既に使っている `esc` / `r` / `j` / `k` / `space` /
`enter` と衝突せず、`admin` の頭文字だからである。

## 3. 変更する箇所

### 3.1 `internal/github/gql`

`merge.graphql` の `pullRequest` に `viewerCanMergeAsAdmin` を 1 行足し、
`mergeContextResponse` と `MergeContext` に対応する bool を足す。

### 3.2 `internal/app/adapter/gateway/gh/merge.go`

`toMergeContext` で素通しする。`MergePR` / `EnableAutoMerge` /
`DisableAutoMerge` は**変更しない**。

### 3.3 `internal/app/domain/merge.go`

```go
// CanMergeAsAdmin reports whether the viewer can push the merge through what
// is holding it. Only two blocks give way. A draft or a conflict is not a
// rule to bypass: it is work that is not finished, and no permission makes
// it finished.
func (c MergeContext) CanMergeAsAdmin() bool {
	if !c.ViewerCanMergeAsAdmin {
		return false
	}
	switch c.Block() {
	case BlockProtected, BlockBehind:
		return true
	}
	return false
}
```

譲る 2 つは gh の `blockedReason` が `--admin` で黙るのと同じ集合である
（§1.1）。`BlockNone` で false を返すのは、押し切るものが無いところに
2 つ目のマージキーを出さないためである。

`AutoMergeEnabled` は見ない。auto-merge の列に並んだまま止まっている PR を
今すぐ押し切りたいのは、ふつうの要求である。

### 3.4 `internal/app/presentation/tui/merge`

`handleKey` に分岐を 1 つ足す。`send()` には触らない。

```go
case "a":
	if !m.ctx.CanMergeAsAdmin() {
		return m, nil
	}
	method := m.method()
	return m.sendCmd(true, func() error { return m.src.MergePR(m.ctx.PullRequest, method) })
```

`sending` 中に全キーを捨てる既存のガードが `a` にもそのまま効く。
`Source` interface は変更しない。

`render.go` は 2 箇所。

- `reason()`: ブロックの行を出したあと、`CanMergeAsAdmin()` なら
  `theme.Dim()` で `merge.admin_offer` を 1 行足す
- `hints()`: 今は blocked のとき `enter` 系のヒントを何も出さない分岐がある。
  そこで `CanMergeAsAdmin()` なら `merge.key_admin` を足す

### 3.5 `internal/i18n/locales`

`en` と `ja` の両方に 2 キー足す（片方だけだとカタログ整合のテストが落ちる）。

| キー | en | ja |
|---|---|---|
| `merge.key_admin` | `a:admin merge` | `a:adminでマージ` |
| `merge.admin_offer` | `you can merge it anyway as an admin` | `admin 権限で押し切れます` |

## 4. 失敗したとき

バイパスできないルールは残る。`kukv/octoscope` には `required_signatures`
（別 ruleset、id 18805699）があり、これは bypass actor の設定とは無関係に効く。
他にも、他人が先にマージした、権限が実は無かった、といった失敗がある。

いずれも `sendCmd` が既に持っている経路——`ErrorMsg` を投げ、詳細ビューが
`Owns` で確かめてフッターに GitHub のメッセージをそのまま出す——で扱う。
**新しいエラーハンドリングは書かない。** §4.4.4 の
「失敗は GitHub のメッセージをそのまま出す」がそのまま当てはまる。

## 5. テスト

| 対象 | 確かめること |
|---|---|
| `domain` | `CanMergeAsAdmin` を 7 つの `MergeBlock` × フラグ 2 値の表で |
| `gql` | `viewerCanMergeAsAdmin` がレスポンスから読めること |
| `tui/merge` | blocked で `enter` が**今まで通り**何も送らないこと |
| `tui/merge` | `a` が `MergePR` を呼ぶこと（blocked / behind） |
| `tui/merge` | draft / conflict / 計算中 / dirty で `a` が何も送らないこと |
| `tui/merge` | `ViewerCanMergeAsAdmin` が false なら `a` が何も送らないこと |
| golden | `merge_admin` を追加（既存の golden と同じ言語・幅の組で） |

`enter` が blocked で沈黙し続けることを明示的に守るのは、この変更で
いちばん壊してはいけない性質だからである。

実機確認は `go run ./cmd/octoscope --lang ja`。
キーバー行が 1 つ増えるので、50 桁のボックスで折り返さないかを見る。

## 6. 実装の順番

1. **`viewerCanMergeAsAdmin` を ruleset 下で実測する。** blocked な PR を 1 つ
   作り、`mergeStateStatus` と合わせてクエリする。
   - `true` が返れば §3.1 以降をそのまま進める
   - `false` が返るなら判定を `repository.viewerPermission == "ADMIN"` に替える。
     bypass actor に入っていない admin にもキーが出るようになるが、押せば
     GitHub が断り、§4 の経路でフッターに理由が出る。**どちらを採ったかを
     この文書に追記してから先へ進む**
2. gql → gateway → domain → tui → i18n の順。各段でテストを足してから次へ
3. `make check`
4. `--lang ja` と `--lang en` で実機確認

## 7. 範囲外

- **merge queue の迂回。** gh の `--admin` は merge queue も飛ばすが、octoscope は
  merge queue に対応していない（`isInMergeQueue` を読んでいない）。
  ここで足すと未対応の機能の半分だけを実装することになる
- **commit message の編集。** `commitHeadline` / `commitBody` は admin マージに
  固有の話ではなく、通常のマージにも同じだけ欠けている。別の設計で扱う
- **ポップアップ以外からの admin マージ。** 一覧から直接押し切る経路は作らない
