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

**`internal/github` は現時点ではまだ `internal/app/domain` を import している。**
これを断つのは PR 2 である。それまでは `internal/github` が domain を知っていても
設計が壊れているわけではない。

同様に、上の図にある `internal/app/adapter/gateway/gh` はまだ存在しない。PR 2 / PR 3 で作られる。

この向きは目視ではなく lint で守る。`.golangci.yml` の `depguard` に禁止 import を
書き、CI で落とす。**パッケージを増やしたら、その場で depguard にも足す。**
足し忘れると、次に誰かが依存の向きを壊しても誰も気づかない。

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

## 複数の API 呼び出しは `internal/app/usecase` に置く

**`tea.Cmd` のクロージャの中に、2 つ以上の API 呼び出しを並べない。**

「pending review が無ければ先に作る」は GitHub のレビュー API の仕様であって
TUI の都合ではない。ビューが知るべきなのは「行コメントを送る」という 1 操作だけで、
それが何回のリクエストになるかではない。

置き場所を分けると、順序のテストに Bubble Tea が要る。`internal/app/usecase` に置けば、
フェイクを 1 つ渡すだけで「pending があるとき / ないとき」を検証できる。

同じことが「種別（PR / Issue）で呼ぶものが変わる」にも当てはまる。
`domain.ItemRef.Kind` を View で `switch` しない。

## `usecase.Item` を画面の写しにしない

`usecase.Item` は PR と Issue の合流点であって DTO ではない。
**ここは UI の都合が下の層に漏れる唯一の穴**なので、太らせない。

- **`Item` に共通フィールドを足してよいのは、PR と Issue の両方に GitHub 側の
  対応物があるときだけ。**
- PR にしか無いものは `Item.PR`（`*domain.PR`）から読む。`Item` に写さない
- 「画面に出したいものが `Item` に無い」と思ったら、まず `internal/app/domain` の
  ドメイン型に無いのではないかを疑う。`domain.Issue` の公開フィールドは 10 個あり、
  `Item` はその全部を持っている（2026-09-07 に数えた）

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
| `AddPRComment` `ClosePR` `EditPRLabels` `ListPRs` `PRChecks` `PRMergeContext` `EnableAutoMerge` | `JobLog(jobID int64)` → 不透明ハンドル |
| `CheckRun.Workflow` `RunNumber`（Forgejo Actions は同型、GitLab の pipeline も対応物がある） | `RerunWorkflow(runID int64)` → 不透明ハンドル |
| `PR` → `ChangeRequest` のような改名は**しない**（227 参照、他サービス対応は未確定） | `PullRequestID` `PendingID` → 不透明ハンドル名へ改名 |
| | `StartReview` / `SubmitNewReview` → `SubmitReview` 1 つに畳む。pending review の作成は gateway が持つ（本規約の部分的な反転。設計 §6） |

## 規約そのものを変える

**この規約が邪魔だと感じたら、黙って逸脱しない。提案する。** 提案には次を含める。

- どの規約が、どの作業で、どう邪魔になったか（具体的な場面）
- 代わりにどうするか
- その変更で守れなくなるものは何か

規約を変えたら、**その場で該当する rules ファイルを更新する。**
更新されない合意は次のセッションには残らない。

lint で守っている規約（depguard など）を変えるときは、設定も一緒に変える。
片方だけ変えると、ドキュメントと CI が食い違ったまま放置される。
