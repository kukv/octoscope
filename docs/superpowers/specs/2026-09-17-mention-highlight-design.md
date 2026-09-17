# 自分宛てメンションの強調

**日付:** 2026-09-17
**状態:** 設計
**対象:** `internal/app/presentation/tui/detail`、および `viewer { login }` を運ぶ各層
**前提:** `2026-09-16-detail-redesign-design.md`（詳細画面の再設計）。その §7「やらないこと」の
第 1 項がこの設計である。本設計が入ったら、§7 からその項を落とす。

## 1. 目的

詳細画面のコメントは全部同じ見た目をしている。10 件並んだうちの 1 件が自分を名指ししていても、
本文を読むまで分からない。読む順を決めるのは「自分に用があるか」なので、そこは開いた瞬間に
見えていてほしい。

octoscope は今のところ**自分のログイン名を知らない**。Work ボードは GitHub へ `mentions:@me` と
投げているだけで、`@me` が誰かを解決していない（`gateway/gh/work.go:17`）。強調の前に、
その 1 つの事実を運ぶ配管が要る。

## 2. 何を強調するか

自分宛てのメンションを**含むブロック**を強調する。ブロックとは次の 2 つ。

- 1 件のコメント（罫 + `@author · 日時` の見出し行）
- 説明（`Description` のセクション見出し行）

強調しないもの:

- **glamour が色を付けた本文そのもの。** 描画済みの文字列に色を差し込むと、リセットで
  glamour 側の色まで切れて行の後半がにじむ。本文は 1 バイトも触らない
- **担当が自分のときのメタ行**（再設計の §7 は候補に挙げていたが、今回は入れない。
  担当かどうかは左ペインのメタ行を読めば分かり、コメントのように流れて消えない）
- メンションの語そのもの（`@kukv` の 5 文字だけを塗る案）。ブロック単位に留めるのは
  上と同じ理由で、本文に手を入れないため

## 3. ログイン名をどう運ぶか

### 3.1 取得

GraphQL の `viewer { login }` を 1 回引く。

```graphql
query { viewer { login } }
```

`internal/github/gql/viewer.go` に `func (c *Client) Viewer(ctx context.Context) (string, error)` を置く。
リポジトリを取らないので `repoVars` は要らず、`RepoName` のような `repo` 引数も無い。

`cli.Client` と `api.Client` はどちらも `*gql.Client` を埋め込んでいるため、**両方に自動で生える**。
`RepoName` のようなラッパを 2 つ書く必要はない（`cli/cli.go:136` と `api/repo.go:107` がラッパを
持つのは `repo` を埋めるためで、`Viewer` にはその事情が無い）。

### 3.2 いつ引くか

**root が最初の `WindowSizeMsg` を受けたときに、`resolveRepo` と並べて非同期に飛ばす。**

再設計の §7 は「起動時に 1 回引き、gateway → usecase → `root.Options` → detail と渡す」と
書いていた。ここは意図的に外す。`root.go` は `RepoName` で一度それをやり、
「UI が始まる前に待つと端末がその間ずっと真っ白」だったので非同期コマンドへ移した経緯がある
（`repoLookupTimeout` のコメント。cold な `gh` で 6 秒超の実測）。`viewer { login }` も
同じ種類のネットワーク呼び出しであり、同じ結末になる。

```
root.Update(WindowSizeMsg)  --> resolveRepo(src)    --> repoResolvedMsg
                            --> resolveViewer(src)  --> viewerResolvedMsg{login}
```

`viewerResolvedMsg` は `m.viewer` に入るだけ。失敗は**捨てる**。ログイン名が引けないことは
エラー画面に値しない — 強調は付加物であり、無ければ今までどおりの画面になる。
`repoResolvedMsg` がタイムアウトを区別して報告するのは、それが「このディレクトリの事実」を
左右するからで、強調にはその重みが無い。

### 3.3 detail へ渡す

`detail.New(src, ref)` に第 3 引数を足す。

```go
func New(src Source, ref domain.ItemRef, viewer string) Model
```

root は `openDetail` で `detail.New(m.src, ref, m.viewer)` と書く。詳細画面は開くたびに
作り直されるので、開いた時点の `m.viewer` を持てばよく、開いている最中に流し込む経路
（`SetViewer` のようなもの）は要らない。

答えがまだ来ていないうちに詳細画面を開いたら `viewer` は空文字で、その回は強調が出ない。
起動直後の 1 回だけ起こりうる取りこぼしであり、`r` で読み直せば付く。これを消すために
配管を増やす価値は無いと判断する。

### 3.4 層ごとの追加

| 層 | ファイル | 追加 |
|---|---|---|
| GitHub | `internal/github/gql/viewer.go` + `viewer.graphql` | `Viewer(ctx) (string, error)` |
| gateway | `internal/app/adapter/gateway/gh/backend.go` | `backend` interface に `Viewer` を足す。変換が要らないので promotion で通り、`gh` 側にメソッドは書かない |
| usecase | `internal/app/usecase/usecase.go` | ポート `viewerFetcher` と、それを呼ぶ `Viewer(ctx)` |
| root | `internal/app/presentation/tui/root/root.go` | `Source` に `detail.Source` 経由で `Viewer` が入る。`resolveViewer` / `viewerResolvedMsg` / `m.viewer` |
| detail | `detail.go` | `Source` に `viewerFetcher` 相当は**不要**（root が引くので、detail は結果を受け取るだけ） |

最後の行が要点である。`Viewer` を呼ぶのは root だけなので、`detail.Source` には足さない。
root の `Source` に直接足す（`repoNamer` と同じ扱い。「root の一人のもの」）。

## 4. 何を自分宛てとみなすか

`internal/app/presentation/tui/detail/mention.go` に純関数を置く。

```go
func mentionsViewer(src, login string) bool
```

**glamour に渡す前の生 markdown** を見る。描画後の文字列には色と折り返しが入っていて、
`@login` が行をまたいで割れている可能性がある。

判定:

- 大小を区別しない。GitHub のログイン名は大小を区別しない
- 語境界を見る。`@kukv` は `@kukvx` や `@kukv-bot` に当たらない。`@` の直後から続く
  「英数字とハイフン」がログイン名の綴りと完全に一致したときだけ当たり
- 次の 3 つの中は見ない
  - フェンス付きコードブロック（` ``` ` と `~~~`。開いたまま閉じていない場合は文末まで）
  - インラインのコードスパン（`` ` `` で囲まれた範囲）
  - 引用行（行頭の空白を除いて `>` で始まる行）
- `login` が空文字なら常に false

**markdown を解析するのではない。** 上の 3 つは「人に向けて書いた `@name` ではない」ことが
はっきりしている範囲であり、それ以外（リンクの中、HTML の中、太字の中）は当たりとして扱う。
取りこぼすより余計に光る方がましで、そして完全な markdown 解析は強調 1 つに見合わない。

## 5. 描き方

### 5.1 コメント

`body.go` の `commentLines` が、当たったコメントだけ色と罫を変える。

```
│ @alice · 2026-09-15 10:03      ← theme.Dim()、罫は icon.CommentBar()
│ LGTM です

┃ @bob · 2026-09-15 11:20        ← theme.Accent()、罫は icon.MentionBar()
┃ @kukv ここ見てもらえますか
```

罫の幅は変わらない（`commentBarWidth` は 2 のまま）。`icon` の全グリフが 1 桁である
規約（`TestEveryGlyphIsOneColumn`）に新しいグリフも従う。

| セット | `commentBar` | `mentionBar`（新） |
|---|---|---|
| Unicode | `▌` | `█` |
| Nerd | `▌` | `█` |
| ASCII | `\|` | `#` |

色が出ない端末でも差が残るように、色だけでなく罫も変える。

### 5.2 説明

`bodyLines` が、説明の本文に当たりがあるときだけ `Description` の見出し行を
`theme.Heading()` から `theme.Accent().Bold(true)` に変える。罫（見出しの右へ伸びる線）は
`theme.Rule()` のまま — そこを塗ると画面の横幅いっぱいが光り、コメントの強調より強くなる。

### 5.3 文字列

**i18n のカタログは触らない。** 新しい語は 1 つも要らない。

## 6. テスト

| 対象 | 確かめること |
|---|---|
| `mentionsViewer` | 素の `@kukv` に当たる / `@kukvx`・`@kukv-bot` に当たらない / `@KUKV` に当たる / フェンス・コードスパン・引用行の中は当たらない / 閉じていないフェンスは文末まで無視 / `login` が空なら false |
| `body_test` | コメント 2 件のうち、自分宛てを含む 1 件だけが太い罫になる |
| ゴールデン | `viewer` を入れた記録を 1 本足す |
| 既存ゴールデン | `viewer` が空のとき **1 バイトも動かない** |
| root | `viewerResolvedMsg` が `m.viewer` に入り、`openDetail` がそれを渡す / 失敗が画面に出ない |

## 7. 同時に片づけるもの — `handleKey` の分割

再設計の分割（`detail.go` → `commands.go` / `update.go` / `keys.go`）で見つけて手を付けなかった
もの。`keys.go` の `handleKey` は 114 行・15 分岐あり、うち 7 つ（`m` `c` `x` `v` `l` `a` `r`）が
本体を持っている。それぞれを `Model` のメソッドに切り出し、`switch` は振り分けだけにする。

```
handleKey  114 行 / 15 分岐  -->  handleKey 30 行前後 + openMerge / openCompose /
                                  openConfirm / openSubmit / openPicker(kind) / refetch
```

6 箇所で繰り返している `if m.phase == phaseLoading { return m.stillLoading(), nil }` は
各メソッドの冒頭へ移す。キーごとに書いてあるコメント（「An issue has no diff.」など）は
動かさない。

**振る舞いは 1 つも変えない。** ゴールデンが 1 バイトでも動いたら、切り出しで何かを
書き換えている。強調より先にやる（振る舞いを変える変更と混ぜない）。

## 8. やらないこと

- メンションの語そのものの強調（§2）
- 担当が自分のときのメタ行（§2）
- Work ボードのカードへの強調。今回は詳細画面の中で閉じる
- チームメンション（`@org/team`）。自分が属するチームを引くにはもう 1 本 API が要る
- ログイン名の保存。起動ごとに引く。1 回の GraphQL であり、保存すると
  `gh auth switch` でアカウントを切り替えたとき古い名前が残る
