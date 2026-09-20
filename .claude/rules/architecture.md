---
paths:
  - "internal/**"
  - "cmd/**"
  - ".golangci.yml"
---

# アーキテクチャ

## 依存の向き

```
cmd/octoscope
     │
     ├──→ internal/github/{cli,api}        （どのバックエンドかを知る唯一の場所）
     ├──→ internal/app/adapter/gateway/gh
     ├──→ internal/app/adapter/datasource
     ├──→ internal/app/config
     └──→ internal/app/presentation/tui

internal/app/presentation/tui ──→ internal/app/usecase ──→ internal/app/domain
                              ──→ internal/app/domain
                              ──→ internal/i18n, internal/browser

internal/app/adapter/gateway/gh ──→ internal/github/{cli,api,gql}
                                ──→ internal/app/domain

internal/app/adapter/datasource ──→ internal/app/domain
                                ──→ internal/app/config
                                ──→ （YAML などのファイル形式）

internal/github/**             ──→ （internal/app のどこにも依存しない）
internal/app/domain            ──→ （stdlib のみ）
internal/{i18n,golden,browser} ──→ （何にも依存しない）
```

**下の層は上の層を知らない。** GitHub アクセス層が UI を import したら設計が壊れている。

**gateway は usecase を import しない。** Go の interface は暗黙に満たされるので、
gateway は domain だけを見て port を満たす。結線は `cmd/octoscope` が行う。

この向きは目視ではなく lint で守る。`.golangci.yml` の `depguard` に禁止 import を
書き、CI で落とす。**パッケージを増やしたら、その場で depguard にも足す。**
足し忘れると、次に誰かが依存の向きを壊しても誰も気づかない。

（`i18n-layer` と `browser-layer` の deny は今も `internal/app`（末尾スラッシュ無し）のままで、
`github-layer` は `internal/app/`（末尾スラッシュ付き）に直した。前者は `internal/appfoo` のような
無関係なパッケージ名も誤って拾う over-match の余地を残しているが、実害はまだ無い。
直すなら 3 つ揃えて直す。）

## interface は利用側で定義する

**GitHub アクセス層は interface を export しない。** 具体型とドメイン型だけを公開する。

必要な操作の interface は、**それを使う側**が宣言する。

```go
// internal/app/presentation/tui/detail/detail.go
type source interface {
    GetPR(repo string, number int) (domain.PR, error)
    AddPRComment(repo string, number int, body string) error
}
```

こうすると、そのビューが何を必要としているかがビューのファイルを読むだけで分かり、
テスト用のフェイクも必要なメソッドだけ書けば済む。

## interface は小さく保つ

**画面ごとに、その画面が使う分だけ宣言する。** 全メソッドを 1 つの interface にまとめない。

**1 つの interface 宣言に直接並べるメソッドは 6 個まで。**
embed した interface のメソッドは数に含めない — embed 先が同じ上限を独立に負う。

判定は簡単で、テスト用のフェイクを書いたときに、そのテストが呼ばないメソッドの
スタブをいくつ書かされたかを見る。6 を超えたら、その interface は
「1 つの画面が使う分」より大きい。

## GitHub API 固有の値はパッケージの外に出さない

`"APPROVED"`、`"CHANGES_REQUESTED"`、`"OPEN"` のような GitHub API の文字列を
TUI 側で `switch` しない。GitHub アクセス層でドメインの値に変換して返す。

```go
type ReviewState int

const (
    ReviewPending ReviewState = iota
    ReviewApproved
    ReviewChangesRequested
)
```

理由は 2 つ。GraphQL と REST で綴りが違う場合にバックエンドの差が UI に漏れないこと、
そして UI 側が「知らない文字列」を握りつぶす分岐を持たずに済むこと。

**この境界はもう規約文ではなく CI が守る。** レビューで気づく必要はない。3 つの検査が
それぞれ違う漏れ方を塞いでいる。

- `internal/app/domain` の 21 個の公開 struct を reflect で再帰的に歩き、struct タグが
  1 つでもあれば落ちる（ポインタ・スライス・配列を経由した無名 struct の中も見る）
- その一覧が漏れなく網羅されているかを `go/ast` でパッケージを解析して照合するテスト
- depguard で `internal/app/domain` の `encoding/json` import と、
  `internal/github/**` の `internal/app/` import を禁止する

## 複数の API 呼び出しは `internal/app/usecase` に置く

**`tea.Cmd` のクロージャの中に、2 つ以上の API 呼び出しを並べない。**

順序をどちらの層に置くかは、その順序が何の都合かで決まる。

**サービス固有の順序**（pending review が無ければ先に作る、など GitHub のレビュー
API の仕様に由来するもの）は `internal/app/adapter/gateway` に置く。ビューはおろか
usecase も、それが何回のリクエストになるかを知らない。`Gateway.SubmitReview` は
pending の有無で `SubmitNewReview` 1 回と `SubmitReview` 1 回を切り替え、
`Gateway.AddReviewThread` は pending が無ければ `StartReview` を先に呼ぶ
（`internal/app/adapter/gateway/gh/review.go`）。

**アプリの都合による順序**は `internal/app/usecase` に残る。
`domain.ItemRef.Kind` を View で `switch` しない——この指示は変わらないが、
**2026-09-20 に移る先が usecase から gateway になった。** 種別の振り分けは
`gateway/gh` の `GetItem` / `ListItems` / `writes.go` が持つ。PR と Issue を
別々に呼ぶのはサービス固有の事情だからである。

置き場所を分けると、順序のテストに Bubble Tea が要る。順序を持つ層に
フェイクを 1 つ渡すだけで検証できるようにする。

**この層は今は薄い。** 振り分けが降りた後 `internal/app/usecase` に残るのは、
`SeedCandidates`（組織スコープの無いトークンでも残りを失わない、というアプリ側の判断を
持つ唯一のメソッド）、ポートの束ね方、各ビューへの薄い委譲だけである。

**それでも残す。** 廃止すると、複数のサービスを横断して見る（複数の gateway に投げて
結果を束ねる）を入れるときに同じ層を再導入することになる。

**実コストも書いておく。** 振り分けのテストが usecase から gateway に降りた結果、
フェイクがドメイン型から `gql` のワイヤ型になり、テストの準備が重くなった。
これは port を 1 本化したことの代償である。

## `domain.Item` を画面の写しにしない

`domain.Item` は PR と Issue の合流点であって DTO ではない。
**ここは UI の都合が下の層に漏れる唯一の穴**なので、太らせない。

- **`Item` に共通フィールドを足してよいのは、PR と Issue の両方に GitHub 側の
  対応物があるときだけ。**
- PR にしか無いものは `Item.Change`（`*domain.Change`）から読む。`Item` に写さない。
  `Change` が非 nil であることと `Ref.Kind == ItemPR` は同値で、保証するのは gateway である
- 「画面に出したいものが `Item` に無い」と思ったら、まず `internal/app/domain` の
  ドメイン型に無いのではないかを疑う。`Item` の公開フィールドは 12 個、
  `Change` は 7 個ある（2026-09-20 に数えた）

この規則があるかぎり、UI だけの修正（色・桁・文言・キー・状態遷移・
何を描くかの選び方）は `internal/app/usecase` に波及しない。波及するのは
GitHub への**新しい操作**を足すときだけで、それは元から UI だけの修正ではない。

## パッケージを増やす基準

責務が 1 つに言えるなら 1 パッケージ。「〜と〜をやる」と説明したくなったら分ける。

ファイルが 300 行を超えたら、責務が増えていないか疑う。行数そのものは規則ではなく、
「そろそろ見直せ」の合図として使う。

## 層を足す前に

このプロジェクトは TUI の単一バイナリであり、Web サービスではない。
DI コンテナ、ドメインモデルとインフラモデルの二重定義、Input/Output DTO は
**入れていない**。

**`internal/app/usecase` を入れる判断を 2026-09-07 にした。**
それまでは「Web サービス向けの構造だから入れない」という一般論で退けていたが、
その判断は `internal/app/presentation/tui` が 1 行も存在しない時点（Phase 0、`67ba0de`）に書かれ、
以後一度も再検証されていなかった。再検証したときの実測は次のとおりである。

- API 呼び出し順序が `tea.Cmd` のクロージャに漏れていた（2 箇所）
- 種別（PR / Issue）の振り分けが View に漏れ、`detail.Source` が 19 メソッドに
  膨らんでいた
- そのどちらも Bubble Tea を起動しないとテストできなかった

**規約は書いた時点のコードに対する判断である。コードが育ったら再検証が要る。**

層を足したくなったときは、次の 2 つを書けるか確かめる。

1. **この層が無いと何が壊れるか。** 実際に起きた、または確実に起きる問題を、
   一般論ではなく**このコードの実測値**で挙げる
2. **足したあと何が減るか。** 触る場所、重複、テストの手間のどれが減るか

両方書けるなら提案する価値がある。書けないなら足さない。
上の `internal/app/usecase` の記述が、書けたときの見本である。

足すと**増える**ものも書く。`internal/app/usecase` の場合は、GitHub への新しい操作を
足すときに触るファイルが `internal/github/cli` + ビューの 2 つから
`internal/github/cli` + `usecase` + ビューの 3 つになる。これが唯一の実コストである。

**`internal/app/adapter/datasource` を入れる判断を 2026-09-13 にした。**

- **無いと何が壊れるか（実測）:** `SavedQuery` が `config`（ファイル形式）→
  `usecase`（再定義）→ `root.Options` の 3 段を経由していた。tui が `config` を
  見られないためだけの中継で、変換関数 `SavedQueriesFrom` が `usecase/search.go`
  に置かれていた（呼び出しだけが `cmd/octoscope` の起動処理にあった）
- **足すと何が減るか:** その 3 段が 1 段になり、`usecase.SavedQuery` と
  `SavedQueriesFrom` が消えた。`config.Config` から `Repositories` と
  `SavedQueries` の両フィールドが消える日が来れば、設定の保存先を変えるときに
  触るのは `datasource` だけになる
- **足すと何が増えるか:** 設定ファイルを触るパッケージが 1 つから 2 つになった。
  `config` が形を持ち、`datasource` が書き戻す。両者が食い違うと設定が静かに
  壊れるので、`datasource_test.go` に「片方を保存しても、もう片方と起動時設定が
  消えない」テストを置いた

**`internal/app/adapter/gateway/gh` を入れる判断を 2026-09-13 にした。**

- **無いと何が壊れるか（実測）:** `internal/github` から domain への参照が
  430 箇所 / 81 シンボルあった。`Author` `Label` `Comment` は json タグ付きのまま
  domain とワイヤ型を兼ねており、`gql/search.go` は Work ボードの 4 カラムという
  アプリの概念を GitHub の検索文字列へ直接変換していた
- **足すと何が減るか:** domain がワイヤ形式を一切知らなくなり、それを CI が守る
  （前節）。別サービスを足すときに書くのは `internal/<service>` と `gateway/<service>`
  だけになり、domain・usecase・tui のどれも動かさずに済む
- **足すと何が増えるか:** GitHub への新しい操作を足すときに触るファイルが
  `internal/github` + `usecase` + ビューの 3 つから、`internal/github` + `gateway` +
  `usecase` + ビューの 4 つになった。ワイヤ型が public API になった

## 名前は借りてよい、形は借りない

port の中立化は行う。ただし**名前**と**形**を分けて扱う。

> **規則「名前は借りてよい、形は借りない」**
>
> - **名詞と操作名は GitHub のものでよい。** PR / Issue / Review / Label / Assignee /
>   AutoMerge / Workflow は Forgejo でも同じ語で、GitLab でも対応物がある
> - **中立でなければならないのは port の形である。**
>   (i) 識別子は不透明な `string` ハンドル。`int64` の Actions ID や GraphQL node ID を
>   その名前のまま port に出さない
>   (ii) **GitHub が余計に 1 回呼ぶ必要があるから存在する port メソッドを作らない**

適用結果:

| 変えない | 変える |
|---|---|
| `PRChecks` `PRMergeContext` `EnableAutoMerge` | `JobLog(jobID int64)` → `JobLog(job domain.JobHandle)` |
| `CheckRun.Workflow` `RunNumber`（Forgejo Actions は同型、GitLab の pipeline も対応物がある） | `RerunWorkflow(runID int64)` → `RerunWorkflow(run domain.RunHandle)` |
| `PR` → `ChangeRequest` のような改名は**しない**（227 参照、他サービス対応は未確定） | `PullRequestID` `PendingID` → `domain.PullRequestHandle` `domain.ReviewHandle`（`ReviewContext.PullRequest` / `ReviewTarget.Pending`） |
| | `StartReview` / `SubmitNewReview` を `SubmitReview` 1 つに畳んだ。pending review の作成は gateway が持つ（本規約の部分的な反転。設計 §6） |
| | `AddPRComment` / `AddIssueComment` → `AddComment(ctx, ItemRef, body)`。`ClosePR` / `ReopenPR` / `CloseIssue` / `ReopenIssue` → `SetState`。`EditPRLabels` / `EditIssueLabels` → `EditLabels`。`EditPRAssignees` / `EditIssueAssignees` → `EditAssignees`。`GetPR` / `GetIssue` → `GetItem`。`ListPRs` / `ListIssues` → `ListItems(ctx, repo, kind)` |
| | `isOwnerSlashName`（presentation の判定）→ `ValidRepoName(name string) bool`。名前の形は GitHub のものなので gateway が答える |

**リポジトリ名の形を gateway に移したのは 2026-09-20 である。** presentation にあった
`isOwnerSlashName` は `gql.SplitRepo` の逐語コピーで、depguard が
presentation → `internal/github` を禁じているために共有できずにいた。domain に置けば
層の線は通るが、`gql.SplitRepo` は消えないので重複は残り、しかも「2 つ目の `/` を弾く」
という GitHub の形を domain に入れることになる。#124 の線（翻訳は ACL、政策はドメイン）では
これは翻訳なので、gateway が答える。

**守れなくなるもの:** 純粋な述語 1 つのために port が 1 本増え、`Source` を満たす
フェイク 3 つがスタブを 1 つずつ背負う。検証にネットワークが要らないことは port を
読んでも分からない（`ctx` を取らないことが唯一の手がかりである）。

**対 14 本を 6 本に畳んだのは 2026-09-20 である。** PR と Issue を別々のエンドポイントで
呼び分けるのは **GitHub の都合**であって、アプリの都合ではない。上の (ii)
「GitHub が余計に 1 回呼ぶ必要があるから存在する port メソッドを作らない」が、
まさにこれを禁じている。`ListItems` の `kind` は分岐ではなく**クエリの引数**である——
Repos タブは PR の一覧と Issue の一覧を別のペインに描くので、呼ぶ側は最初から
どちらが欲しいか決まっている。

**守れなくなるもの:** port を読んでも GitHub への呼び出しが何回になるか分からなくなる。
これは `StartReview` / `SubmitNewReview` を畳んだときと同じ性質の反転で、前例に揃えた。

**`backend` 側は別である。** `internal/github` のクライアントは今も `AddPRComment` と
`AddIssueComment` を別々に持ち、`gateway/gh/writes.go` がその間で振り分ける。
`Gateway` は `backend` を埋め込んでいるので、それらはメソッド昇格で `Gateway` にも生えている。
この規約が言う「形」は **usecase に向いた port の形**であって、ACL の内側ではない
（`gateway/gh/backend.go` のパッケージコメントを見よ）。

## 規約そのものを変える

**この規約が邪魔だと感じたら、黙って逸脱しない。提案する。** 提案には次を含める。

- どの規約が、どの作業で、どう邪魔になったか（具体的な場面）
- 代わりにどうするか
- その変更で守れなくなるものは何か

規約を変えたら、**その場で該当する rules ファイルを更新する。**
更新されない合意は次のセッションには残らない。

lint で守っている規約（depguard など）を変えるときは、設定も一緒に変える。
片方だけ変えると、ドキュメントと CI が食い違ったまま放置される。
