# domain の積み残し 3 件

2026-09-19

## 1. 何を直すのか

`docs/superpowers/specs/2026-09-19-domain-per-model-files-design.md` の §3 と §7 で
「振る舞いを変えないため別件にする」と書いて残した 3 件を片付ける。

| | 内容 |
|---|---|
| 1 | `ReviewContext.PendingCount()` などにテストが無い |
| 2 | View と usecase に `Kind ==` の比較が 14 箇所ある |
| 3 | `WorkItem.Author` の型が `PR.Author` / `Issue.Author` と違う |

3 件とも `domain` とその呼び出し側で閉じるので、1 つの spec にまとめる。

## 2. 調べて前提が変わったこと

**3 は「型の不一致」ではなく「読み手がいない」。**

```
書く: internal/app/adapter/gateway/gh/work.go:69   Author: n.Author.Login,
読む: 0 箇所（非テスト、2026-09-19 実測）
```

Work board も Search も、カードに作成者を描いていない。読み手はテスト 2 箇所
（`gh/work_test.go:164, 217`）だけである。型を `domain.Author` に揃えても、
読み手のいないフィールドに struct の皮をかぶせるだけになる。

**2 は 3 種類に分かれる。**

| 種類 | 箇所 | 扱い |
|---|---|---|
| `ItemRef.Kind == ItemPR` | 11 | `IsPR()` でそのまま置ける |
| `ItemRef.Kind == ItemIssue` | 2 | `!IsPR()` だと意味が反転する（後述） |
| `usecase.Item.Kind == ItemPR` | 1 | `Item` は `ItemRef` ではないので `IsPR()` が使えない |

## 3. 変えないもの

| | |
|---|---|
| 振る舞い | 1 つも変えない。3 件とも描画にも通信にも触らない |
| golden ファイル | 1 つも変更しない。変わったら実装が間違っている |
| `usecase.Item` の契約 | `Kind` フィールドは残す（§5 参照） |

## 4. 変更の内容

### 4.1 テストを足す（実装は変えない）

既存の振る舞いを固定するだけ。

| ファイル | 対象 | 確かめること |
|---|---|---|
| `review_context_test.go` | `PendingCount()` | 複数スレッドをまたいで未送信だけを数える / 未送信ゼロなら 0 / スレッドが空なら 0 |
| `review_thread_test.go` | `Pending()` | 公開スレッド内の未送信の返信も pending と答える |
| `review_thread_test.go` | `Collapsed()` | `Resolved` か `Outdated` のどちらか一方でも true なら畳む |

**`PendingCount()` には「スレッドをまたいで数える」ケースを必ず入れる。**
二重ループなので、1 スレッドだけのテストでは内側のループしか守れない。

この 3 メソッドを選んだのは、Task 7 のレビューで危険度を順に並べた結果である。
`PendingCount()` は「何件送られるか」を利用者に直接見せる集計で、壊れても
気づきにくい。`Pending()` は `slices.ContainsFunc` 一発、`Collapsed()` は
`||` 一発なので、ついでに書く。

### 4.2 `Kind ==` を型のメソッドに寄せる

`domain.ItemRef.IsIssue()` を足す。

```go
// IsIssue reports whether this reference names an issue.
func (r ItemRef) IsIssue() bool { return r.Kind == ItemIssue }
```

そのうえで置き換える。

| 置き換え | 箇所 |
|---|---|
| `ref.Kind == domain.ItemPR` → `ref.IsPR()` | 11 |
| `it.Ref.Kind == domain.ItemIssue` → `it.Ref.IsIssue()` | 2 |
| `it.Kind == domain.ItemPR` → `m.ref.IsPR()` | 1（`tui/detail/update.go:108`） |

**`detail/update.go:108` が `m.ref` に変わる理由。** ここは読み込んだ
`usecase.Item` の `Kind` を見ているが、その `Item` は `fetch(m.src, m.ref)` で
取りに行った当のアイテムなので、`m.ref` と同じものを指す。detail の他の
4 箇所はすでに `m.ref.Kind` を見ているので、これで detail 内の表現が 1 つに揃う。

### 4.3 `WorkItem.Author` を消す

| 変更 | 場所 |
|---|---|
| `Author string` フィールドを削除 | `domain/work_item.go` |
| `Author: n.Author.Login,` を削除 | `gh/work.go:69` |
| 期待値から `Author` を削除 | `gh/work_test.go:164, 217` |

読み手が 1 箇所も無いので振る舞いは変わらない。`go vet` と `golangci-lint` が
取り残しを捕まえる。

## 5. 判断

### ① `IsIssue()` を足す

`ItemKind` は 2 値なので `!IsPR()` で表現できる。それでも足すのは、
**置き換える 2 箇所が「Issue なら」と読める形で書かれている**ためである。

```go
func stateMarker(it domain.WorkItem) string {
	if it.Ref.Kind == domain.ItemIssue {
		return theme.Issue().Render(icon.Issue())
	}
```

```go
	// Issues have no checks at all, so they get no pane.
	if it.Ref.Kind == domain.ItemIssue {
		return nil
	}
```

`!it.Ref.IsPR()` にすると「PR でなければ」に変わり、直上のコメント
（`Issues have no checks`）と本文がずれる。

**2026-09-19 のレビューで「`IsIssue()` を作らない」判断を一度している。**
そのときの論拠は「2 値なので `!IsPR()` で足りる、両方生やすと二重管理になる」
で、`IsPR()` を足した時点では `IsIssue()` に読み手が 1 つも無かった。今は
2 箇所ある。**先回りで作らない**という判断と、**読み手ができたから作る**という
判断は矛盾しない。

`ItemKind` に 3 値目が増えたら、そのときは両方を消して `Kind` を直接見る設計に
戻す。それは今 `IsIssue()` を作らない理由にはならない。

### ② `usecase.Item.Kind` は残す

4.2 の置き換えのあと、`usecase.Item.Kind` を読む非テストコードがゼロになる。
それでもフィールドは残す。

`usecase.Item` は PR と Issue の合流点であり、`Kind` はその合流点が
「これはどちらだったか」を運ぶ契約である。読み手が今いないのは、たまたま
detail が `m.ref` からも同じことを知れるからにすぎない。`architecture.md` は
`Item` を「UI の都合が下の層に漏れる唯一の穴」と呼んでおり、その穴を狭める
話と、合流点の契約を削る話は別である。

**この事実は記録しておく。** 将来 `Item` を見直すときの材料になる。

### ③ `WorkItem.Author` は消す、`domain.Author` に揃えない

`architecture.md` の「`usecase.Item` を画面の写しにしない」と同じ考え方で、
**画面が要らないものをドメイン型に持たない**。

揃える案（`domain.Author` にする）を採らないのは、読み手が 1 つも無いため
実利がゼロだからである。将来 Work board のカードに作成者を出すことになったら、
そのときに型を決めて足す。そのほうが、そのとき必要な形（ログイン名だけか、
アバターも要るか）に合わせられる。

## 6. 検証

1. `make check` が終了コード 0 → 検証: 終了コード
2. `testdata/*.golden` の変更が 0 件 → 検証: `git diff --stat` に `.golden` が出ない
3. `grep -rn 'Kind == domain.Item' --include='*.go' internal | grep -v _test` が 0 件
   → 検証: 出力が空
4. `grep -rn 'WorkItem' --include='*.go' internal | grep Author` が 0 件
   → 検証: 出力が空
5. 実際に起動して Work board と詳細画面を見る（既定と `--lang ja`）
   → 検証: 4 列が出る / カードにアイコンとチェックバーが出る / 詳細のタイトルが
   PR と Issue で正しく出し分けられる

5 を外さない。4.2 は詳細画面のタイトルと Work board のアイコンを決めている
箇所を触るので、テストが通っただけでは完了としない（`CLAUDE.md`）。

## 7. この先

この spec の範囲外として残るもの。

- `internal/app/usecase` の名前と境界（`Usecase` が何も指していない、
  29 メソッド中 21 個が 1 行の委譲）
- `internal/app/adapter/datasource` の改名（中身は設定ファイル）
- `gateway/gh` の `work.go` / `lists.go` の境界（3 つの名詞が同居）
- TUI の `Model` に接頭辞付きフィールドで同居している別責務
  （`checks.Model` の rerun / log など）
