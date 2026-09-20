# Item 統合 設計書

**日付:** 2026-09-20
**状態:** 設計
**置き換える対象:** `.claude/rules/architecture.md` の 3 節
（「名前は借りてよい、形は借りない」の表、「`usecase.Item` を画面の写しにしない」、
「複数の API 呼び出しは `internal/app/usecase` に置く」）。§7 に変更案を書く。

## 1. 目的

現在のコードから sudo モデリング（システム関連図 / ユースケース図 / ドメインモデル図 /
オブジェクト図）を起こしたところ、**「PR または Issue」という 1 つの概念が 3 つの型に散っている**
ことが最大の引っかかりだった。この設計はそれを 1 つに畳み、その帰結として種別の振り分けと
ポートの形を決め直す。

対象は違和感 F-01 / F-02 / F-03 の 3 件。

- **F-01** 同じものが `domain.PR` / `domain.Issue` / `usecase.Item` / `domain.WorkItem` の
  4 形態で存在し、どれが本体かモデル上どこにも書かれていない
- **F-02** `ref.Kind` による分岐が usecase の 5 メソッドに重複し、その先のポートが
  `AddPRComment` / `AddIssueComment` のように対で並ぶ
- **F-03** `Usecase` の 30 メソッドのうち 20 以上が 1 行の委譲で、層の責務が言えない

画面の出力は 1 バイトも変えない。これは内部構造だけの変更である。

## 2. 実測（2026-09-20）

### 2.1 同じものを指す 4 つの型

| 型 | 位置 | フィールド数 | 由来 |
|---|---|---|---|
| `domain.PR` | `domain/domain.go:53` | 18 | 1 件取得（`GetPR`）と一覧（`ListPRs`） |
| `domain.Issue` | `domain/domain.go:80` | 11 | 同上 |
| `usecase.Item` | `usecase/usecase.go:171` | 12 | 上 2 つの合流点。presentation が import する |
| `domain.WorkItem` | `domain/domain.go:179` | 14 | 検索クエリ由来の read model。`Author` が `string` |

`domain.Issue` の 11 フィールドは全部 `domain.PR` にもある。
`domain.PR` だけが持つのは 7 つ（`IsDraft` `Review` `Head` `Base` `Additions` `Deletions` `Checks`）。
`usecase.Item` は 11 の共通フィールドのうち `BodyText` だけを持たない。

### 2.2 参照数（非テスト）

| 型 | 参照 | ファイル |
|---|---|---|
| `domain.PR` | 13 | — |
| `domain.Issue` | 12 | — |
| `usecase.Item` | 6 | — |
| `domain.ItemRef` | 83 | — |

畳む 3 つの型（`domain.PR` / `domain.Issue` / `usecase.Item`）を触るファイルは **7 つ**。

```
internal/app/usecase/usecase.go
internal/app/adapter/gateway/gh/items.go
internal/app/adapter/gateway/gh/lists.go
internal/app/presentation/tui/detail/detail.go
internal/app/presentation/tui/detail/body.go
internal/app/presentation/tui/detail/meta.go
internal/app/presentation/tui/repo/repo.go
```

残る型（`domain.ItemRef`）は 83 箇所から参照されるが、この設計では**中身を変えない**。
2026-09-13 の再構成が約 1,400 箇所を動かしたのに比べれば小さい。

### 2.3 種別の振り分けとポート対

usecase の 5 メソッドが `ref.Kind == domain.ItemPR` で分岐する
（`usecase/usecase.go:189, 213, 221, 234, 241`）。その先のポートは 14 本。

```
GetPR / GetIssue
AddPRComment / AddIssueComment
ClosePR / ReopenPR / CloseIssue / ReopenIssue
EditPRLabels / EditIssueLabels
EditPRAssignees / EditIssueAssignees
ListPRs / ListIssues
```

### 2.4 usecase の中身

- メソッド 30（`usecase.go` 23 / `repos.go` 3 / `review.go` 2 / `search.go` 2）
- インターフェース・フィールド 15。`New` はその**全部に同じ `src`** を代入する（`usecase.go:149`）
- 実ロジックがあるのは `SeedCandidates`（`repos.go:31`）と、§2.3 の振り分け 5 本、
  `GetItem` の詰め替えのみ

### 2.5 画面出力の固定

golden ファイル（`*.golden`）は **354 枚 / 9 ディレクトリ**。
`internal/golden` は `OCTOSCOPE_UPDATE_GOLDEN` が無ければ差分で落ちる。

`testdata/` 配下には録った API レスポンスなども入っており、そこまで数えると
386 枚 / 13 ディレクトリになる。**検査は `git status --porcelain -- '*testdata*'` で
`testdata/` 全体を見る**——golden だけでなく、録ったレスポンスも変わってはいけない。

### 2.6 ドメイン型の検査

`internal/app/domain/tags_test.go:19` の `exported` に公開 struct 22 個が手書きで並び、
reflect で json タグの不在を、`go/ast` で一覧の網羅を検査している。
型を足す / 消すときはこのリストも動かす。

## 3. 決定事項

ブレインストーミングで 3 つ決めた。

1. **PR と Issue を統合する。** `domain.Item` をユビキタス言語として正式に採用し、
   `domain.PR` / `domain.Issue` / `usecase.Item` を 1 つに畳む
2. **種別の振り分けは gateway に落とす。** ポート対 14 本を 6 本にする
3. **`internal/app/usecase` は薄いまま残す。** 廃止しない。理由と将来の接合点を規約に明記する

横断（複数サービスを同時に見る要求。違和感 F-11 / F-12 / F-13）は**この設計の対象外**とした。
ただし後からサービスの次元を足すときの変更点が 1 箇所で済むよう、
同一性の表現は `ItemRef` に閉じ込める（§4.2）。

## 4. 採用するモデル

### 4.1 `Item` と `Change`

```go
// Item is one pull request or issue.
type Item struct {
	Ref       ItemRef
	Title     string
	Author    Author
	State     ItemState
	URL       string
	Body      string
	BodyText  string
	Comments  []Comment
	Labels    []Label
	Assignees []Author
	UpdatedAt time.Time

	// Change is the part only a pull request has: it proposes a change to
	// the repository. It is non-nil if and only if Ref.Kind is ItemPR.
	Change *Change
}

// Change is what a pull request adds to an item: the branches it moves
// between, how big it is, and what has been said about it by review and CI.
type Change struct {
	IsDraft   bool
	Review    ReviewState
	Head      string
	Base      string
	Additions int
	Deletions int
	Checks    Checks
}
```

`Number` は `Ref` の中にあるので `Item` は持たない。

**不変条件:** `Change != nil` ⟺ `Ref.Kind == ItemPR`。
これは gateway が保証し、`domain` にその旨のテストを置く。
octoscope が自前で守るルールは少ないが（§10 参照）、これはその 1 つである。

`Item` に共通フィールドを足してよい条件は現行規約のまま維持する
（PR と Issue の両方にサービス側の対応物があるときだけ。PR 固有のものは `Change` から読む）。

### 4.2 `ItemRef` は同一性の値オブジェクト

新しい `ItemID` は作らない。既存の `ItemRef{Kind, Repo, Number}` をそのまま
「Item の同一性を表す値オブジェクト」として位置づけ、doc にそう書く。
83 箇所の参照を意味なく動かさないためと、後でサービスの次元を足すときに
触るのがこの型だけになるためである。

`Kind` は `ItemRef` の中に残す。**Item を取得する前に ref を持つ**場面があるからで
（Work ボードのカードが `OpenDetailMsg{Ref}` を投げる時点ではまだ何も取得していない）、
ref 自身が何を指すか知っている必要がある。

### 4.3 スコープ外

| 対象 | 理由 |
|---|---|
| `domain.WorkItem` | 別のクエリ（GitHub の issue search）由来の read model で、1 件取得とは取れる情報が違う。F-01 の 3 つ目だが、他の 3 つとは違って**分かれている正当な理由がある**。統合しないことをここで明示する |
| `domain.ReviewContext` / `domain.MergeContext` | 別クエリで取得し、独自の `PullRequestHandle` を持ち、diff / merge ビューは安定している。`Change` に畳み込むと F-01 が全面書き直しになる |
| F-11 / F-12 / F-13（横断） | §3 のとおり対象外 |

### 4.4 ファイル構成

現在 `domain` と `usecase` はどちらも 1 ファイルに全部を書く形になっており、読みにくい。

- `domain/domain.go` は **281 行に 8 概念**が同居する。しかも兄弟が離れている——
  `checks.go` があるのに `Checks` / `CheckRun` / `CheckState` は `domain.go` 側、
  `review.go` があるのに `ReviewState` は `domain.go` 側
- `usecase/usecase.go` は **315 行**で、`architecture.md` の「300 行を超えたら責務を疑う」を
  既に超えている。ポートの interface 13 種とメソッド 23 本が 1 ファイルに並ぶ

**概念単位で分ける。** 一緒に使う型を 1 ファイルにまとめる粒度で、
Go の標準ライブラリと同じ（`net/http` の `request.go` / `response.go` / `cookie.go`）。
1 struct = 1 ファイルにはしない——`Author` のような 3 行の型までファイルになり、
一緒に読むべき型（`Item` と `Change`、`Checks` と `CheckRun`）が分断されるため。

```
internal/app/domain/
  doc.go        パッケージ doc のみ
  item.go       Item, Change, ItemRef, ItemKind, ItemState, Author, Label, Comment
  work.go       Work, WorkItem, WorkSection, WorkSections()
  checks.go     CheckState, CheckRun, Checks, CheckKind, LogLine, RerunScope
  review.go     ReviewState, ReviewContext, ReviewTarget, ReviewThread,
                ThreadComment, PendingComment, ReviewEvent
  merge.go      （現状のまま）
  diff.go       （現状のまま）
  diff_parse.go （現状のまま）
  handle.go     （現状のまま）
  repo.go       RepoCount, RepoCandidate
  query.go      SavedQuery
  errors.go     sentinel 3 つ, classified, Classify, IsFatal
```

`domain.go` は無くなる。各ファイルは 100 行前後に収まる。

```
internal/app/usecase/
  usecase.go    Usecase, New, source, settingsStore
  item.go       アイテム操作の port とメソッド（GetItem, AddComment, SetState,
                EditLabels, EditAssignees, ListItems）
  lists.go      RepoName, ListLabels, ListAssignees, Viewer
  work.go       ListWorkSection, RepoCounts
  search.go     （現状のまま）
  repos.go      （現状のまま）
  review.go     PRDiff, PRReviewContext, PostLineComment, SubmitReview, DiscardReview
  checks.go     PRChecks, JobLog, RerunWorkflow
  merge.go      PRMergeContext, MergePR, EnableAutoMerge, DisableAutoMerge
```

**ポートの interface は、それを使うメソッドと同じファイルに置く。**
「この操作が何を必要としているか」がファイル 1 つを読めば分かる形にする。
これは既存の規約（「interface は利用側で定義する」「interface は小さく保つ」）の
置き場所を具体化したもので、規約の変更ではない。

この分割は**振る舞いを一切変えない純粋な移動**なので、他の変更と混ぜず独立した PR にする（§8 PR 1）。

## 5. ポートの形

ポート対 14 本を 6 本にする。宣言する場所は現行どおり利用側（usecase）。

| 現在 | 変更後 |
|---|---|
| `GetPR(ctx, repo, number)` / `GetIssue(ctx, repo, number)` | `GetItem(ctx, ItemRef) (Item, error)` |
| `AddPRComment(repo, number, body)` / `AddIssueComment(...)` | `AddComment(ctx, ItemRef, body) error` |
| `ClosePR` / `ReopenPR` / `CloseIssue` / `ReopenIssue` | `SetState(ctx, ItemRef, closing bool) error` |
| `EditPRLabels` / `EditIssueLabels` | `EditLabels(ctx, ItemRef, add, remove []string) error` |
| `EditPRAssignees` / `EditIssueAssignees` | `EditAssignees(ctx, ItemRef, add, remove []string) error` |
| `ListPRs(ctx, repo)` / `ListIssues(ctx, repo)` | `ListItems(ctx, repo string, kind ItemKind) ([]Item, error)` |

`ListItems` の `kind` は分岐ではなく**クエリの引数**である。Repos タブは PR の一覧と
Issue の一覧を別々のペインに描くので、呼ぶ側は最初からどちらが欲しいか決まっている。

**新ポートは全部 `ctx` を取る。** 現在は読み取りだけが `ctx` を持ち、書き込みは持たない
（違和感 F-07）。どうせ署名を書き直すので、ここで揃える。揃えないと同じ行を二度触ることになる。

## 6. usecase に残すもの

振り分けが gateway に落ちた後、`internal/app/usecase` に残るのは次のとおり。

- `SeedCandidates` — 自分のリポジトリと所属組織のリポジトリを集める。組織スコープの無い
  トークンでも残りを失わない、というアプリ側の判断を持つ唯一のメソッド
- ポートの束ね方（どのビューが何を必要とするかの宣言）
- 各ビューへ渡す薄い委譲

**層は薄くなるが残す。** 廃止すると、横断（F-11: 複数の gateway に投げて結果を束ねる）を
入れるときに同じ層を再導入することになる。その判断を `architecture.md` に書き残す（§7.3）。

## 7. 規約変更提案

`.claude/rules/architecture.md` の 3 節を書き換える。CLAUDE.md の手順に従い、
「どの規約が / 代わりにどうするか / 守れなくなるもの」を挙げる。

### 7.1 「名前は借りてよい、形は借りない」の表

**現状:** `AddPRComment` `ClosePR` `EditPRLabels` `ListPRs` が「変えない」欄にある。

**変更後:** これらを「変える」欄へ移す。理由は、**PR と Issue を別々のエンドポイントで
呼び分けるのは GitHub の都合**であって、アプリの都合ではないため。規約が禁じているのは
まさにそれ（「GitHub が余計に 1 回呼ぶ必要があるから存在する port メソッドを作らない」）で、
今回はその原則を適用する側に回る。

**守れなくなるもの:** port を読んでも GitHub への呼び出しが何回になるか分からなくなる。
ただしこれは既に一度起きている反転で、`StartReview` / `SubmitNewReview` を
`SubmitReview` 1 本に畳んだときと同じ性質である（設計 §6、`gateway/gh/review.go:40,58`）。前例に揃える。

### 7.2 「`usecase.Item` を画面の写しにしない」

**現状:** 節の主語が `usecase.Item` で、PR 固有のものは `Item.PR`（`*domain.PR`）から読む、と書かれている。

**変更後:** 主語を `domain.Item` に、`Item.PR` を `Item.Change` に差し替える。
**規則の本体は変えない** — 共通フィールドを足してよいのは両方にサービス側の対応物があるときだけ、
という条件も、「画面に出したいものが無いと思ったらまずドメイン型を疑え」という指示もそのまま残す。

**守れなくなるもの:** 無い。型の置き場所が `usecase` から `domain` に変わるだけで、
規則が守っている性質（UI の都合が下の層に漏れないこと）は同じである。

### 7.3 「複数の API 呼び出しは `internal/app/usecase` に置く」

**現状:** 「アプリの都合による順序は usecase に残る。`domain.ItemRef.Kind` を View で `switch` しない」
とあり、**この節の唯一の例が種別の振り分け**である。その振り分けが gateway に移ると例が消える。

**変更後:** 例を差し替え、この層が今は薄いこと、それでも残す理由（§6）、
将来ここに何が入る予定か（横断の束ね）を明記する。
「View で `switch` しない」という指示自体は残す — 移る先が usecase から gateway に変わるだけである。

**守れなくなるもの:** ここが実コストになる。
**今 usecase でフェイク 1 つ（ドメイン型を返すだけ）で検証できている振り分けのテストが、
gateway に降りてフェイクが `gql` のワイヤ型になる。** テストの準備が重くなる。
これは §5 の形を選んだことの代償であり、隠さずに規約へ書く。

## 8. 移行

5 つの PR に分ける。**各段階で `make check` が通り、golden 354 枚と `testdata/` 全体が無変更であること**を条件にする。

どこかで `OCTOSCOPE_UPDATE_GOLDEN` が要るなら、それは作業ではなく**画面出力を変えてしまった証拠**である。
その段階を止めて原因を調べる。

### PR 1: ファイル分割（振る舞いの変更なし）

§4.4 のとおり `domain` と `usecase` を概念単位のファイルに割る。**型も関数も 1 行も書き換えない。**
移動と、移動に伴う import の整理だけ。この時点ではまだ `domain.PR` / `domain.Issue` のままである。

2026-09-13 の再構成でも同じ順序を採った（PR 1 が「機械的な移動とリネーム」）。
先に割っておくと、以降の PR の差分が「何が変わったか」だけになる。

**成功条件:** `make check`。golden 無変更。
`git diff --stat` 以外に意味のある差分が無いこと（`git diff -M` で移動として検出される）。

**完了: 2026-09-20。**

### PR 2: `domain.Item` と新ポートの追加（旧は残す）

- `domain.Item` / `domain.Change` を足す
- `tags_test.go` の `exported` に 2 型を追加（22 → 24）
- `ItemRef` の doc を「Item の同一性」に書き直す
- `gateway/gh` に §5 の 6 メソッドを実装する。種別の振り分けはここに置く
- gateway に振り分けのテストを足す（`Change != nil` ⟺ `Kind == ItemPR` の検査を含む）
- 旧ポート 14 本はまだ残す。誰も新ポートを呼んでいない状態で終わる

**成功条件:** `make check`。golden 無変更。新旧どちらのテストも通る。

**完了: 2026-09-20。**

### PR 3: usecase を新ポートへ

- usecase のポート宣言を §5 の形に差し替える
- `usecase.Item` を削除し、`GetItem` は `domain.Item` を返す
- 振り分け 5 本（`GetItem` `AddComment` `SetState` `EditLabels` `EditAssignees`）から
  `Kind` の分岐を削除する
- usecase にあった振り分けのテストを消す（PR 2 で gateway 側に置いた分が代わりになる）

**成功条件:** `make check`。golden 無変更。usecase に `Kind` の `switch` が 0 箇所。

**PR 1 からの申し送り（2026-09-20、最終レビューで挙がった 3 件）。**
どれも PR 1 の欠陥ではなく、PR 3 がどのみち同じファイルを触るので、そのときに片付くもの。

- **`crossRepoLister` の `SearchItems` を `search.go` へ。**
  この interface は `work.go` にあるが、唯一の呼び出し元 `Usecase.SearchItems` は
  `search.go` にある。§4.4 の「ポートの interface は、それを使うメソッドと同じファイルに置く」に
  唯一背く箇所で、PR 1 では「interface を分割しない（移動ではなく変更になる）」として
  意図的に残した。PR 3 はポート宣言を書き換えるので、そこで 1 メソッドの独立ポートとして切り出す
- **`fake_test.go` の `fakeSource` をポート単位に割る。**
  item / viewer / checks / merge の 4 ポートにまたがる 10 メソッドを持ち、
  `architecture.md` の「呼ばないメソッドのスタブを何本書かされたか」判定に引っかかる大きさ。
  分割前からの姿で PR 1 が作ったものではない。PR 3 で `GetPR` / `GetIssue` が抜けるのが割る機会
- **`usecase/item_test.go` がちょうど 300 行。** 計画の行数チェックはテストを対象外にしているので
  違反ではないが、PR 2 / PR 3 はこのファイルを書き換える。これ以上太らせない
- **`ctx` を `backend` の書き込みメソッドに通す。** 新ポート 6 本は `ctx` を取るが、
  gateway の書き込み 4 本は受け取って使っていない（`writes.go` の `_ = ctx`）。
  `internal/github` の `AddPRComment` などが `ctx` を取らないためで、PR 2 の範囲外とした。
  書き残さないと `ctx` は飾りのまま残る

### PR 4: ビューの移行

- `domain.PR` / `domain.Issue` / `usecase.Item` を参照する presentation の 4 ファイル
  （`detail/detail.go` `detail/body.go` `detail/meta.go` `repo/repo.go`）を `domain.Item` に移す
- PR 固有の読み出しは `item.Change.X` になる。`Change == nil` の分岐は
  「Issue には無い」の意味で、種別の分岐ではない

**成功条件:** golden 354 枚が 1 バイトも変わらないこと。ここが本 PR の唯一かつ最強の検査である。

### PR 5: 旧型と旧ポートの削除、規約の更新

- `domain.PR` / `domain.Issue` を削除、`tags_test.go` の `exported` から除去（24 → 22）
- gateway の旧ポート 14 本を削除
- `.claude/rules/architecture.md` の 3 節を §7 のとおり更新
- `.golangci.yml` の depguard に変更が要らないことを確認する（パッケージは増減しない）

**成功条件:** `make check` と `make release-check`。golden 無変更。

## 9. 完了条件

- `domain/domain.go` が存在せず、`domain` と `usecase` が §4.4 のファイル構成になっている
- `domain` と `usecase` に 300 行を超えるファイルが無い
- `domain.PR` と `domain.Issue` が存在しない
- `usecase.Item` が存在しない
- ポート対 14 本が 6 本になっている
- `internal/app/usecase` に `ItemKind` の `switch` が 0 箇所
- 新ポート 6 本すべてが `ctx` を取る
- golden 354 枚と `testdata/` 全体が無変更
- `make check` / `make release-check` が通る
- `.claude/rules/architecture.md` の 3 節が更新済み

## 10. この設計が DDD として何をしているか

octoscope は不変条件をほとんど持たない。PR をマージできるか、レビューを提出できるか、
Issue を閉じられるか——決めるのは GitHub であって octoscope ではない。
だから**集約ルートに不変条件を集める**という戦術的 DDD の中心は、ここでは空回りしやすい。

この設計が効かせているのは戦略面である。

- **ユビキタス言語** — 「Item（PR または Issue）」を正式な語として採用した。
  今までコードはこれを 4 つの名前で呼んでいた
- **腐敗防止層** — サービスが PR と Issue を別物として扱う事情を gateway に閉じ込めた
- **数少ない不変条件の明示** — `Change != nil` ⟺ `Kind == ItemPR` を型とテストで表した

## 11. 却下した案

| 案 | 却下理由 |
|---|---|
| **分離維持** — PR と Issue を別集約のまま残し、一覧用 read model だけ共通化 | `Kind` の分岐がビューに戻る。`internal/app/usecase` を入れる前の状態（`detail.Source` が 19 メソッド）に逆戻りする |
| **中間** — domain は分離したままポートだけ 1 本化 | F-02 と F-03 は片付くが、F-01（同じものが複数の型）が半分残る。今回の主目的が達成されない |
| **一括 1 PR** | 参照数（13 + 12 + 6）だけ見れば収まるが、7 ファイル + golden 354 枚 + 規約 3 節 + ポート 14 本の削除が 1 回のレビューに乗る。見落としが混ざる形 |
| **`ItemID` を新設** | `ItemRef` 83 参照を意味なく動かすことになる。`ItemRef` 自体を値オブジェクトと位置づければ足りる |
| **`WorkItem` も統合** | §4.3 のとおり、分かれている正当な理由がある |
| **厳密に struct 単位のファイル分割**（1 型 = 1 ファイル） | `domain` だけで 20 ファイル前後になる。`Author` のような 3 行の型までファイルになり、一緒に読むべき型（`Item` と `Change`、`Checks` と `CheckRun`）が分断される。Go の標準ライブラリにも無い粒度 |
| **ファイル分割を他の PR に混ぜる** | 移動と書き換えが同じ差分に乗ると、レビューで「何が変わったか」が読めなくなる。§8 PR 1 として独立させる |
| **横断（F-11 / F-12）を同時に入れる** | 利用者の判断で保留。ただし `ItemRef` に同一性を閉じ込めることで、後から足すときの変更点は 1 箇所に収まる |
