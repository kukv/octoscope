---
paths:
  - "internal/**"
  - "cmd/**"
  - ".golangci.yml"
---

# アーキテクチャ

## 依存の向き

<!-- TRANSIENT(Part1 Task 7): internal/usecase ができたらこの注記を消す -->
> **`internal/usecase` はまだ存在しない。** 導入する判断は済んでおり（「層を足す前に」
> 参照）、この図はその**到達点**である。作る作業は
> `docs/superpowers/plans/2026-09-07-phase2-remediation-part1.md` の Task 5〜7 にある。
> 規約を先に置いたのは、規約が設計に沿っていない状態で実装を始めると誤った方向に
> 進むためで、**この節の「〜しない」は今日書くコードにそのまま効く**
> （`internal/tui` が `internal/gh/cli` を import しないことは既に守られており、
> depguard も入っている）。

```
cmd/octoscope                     （合成ルート: cli.New → usecase.New → app.New）
     ↓
internal/tui  ──→  internal/usecase  ──→  internal/gh/cli  ──→  internal/browser
     │                    │                      │
     └────────────────────┴──────────────────────┴──→  internal/gh  （ドメイン型）
     ↓
internal/i18n                                     （誰にも依存しない）
```

**下の層は上の層を知らない。** GitHub アクセス層が TUI を import したら設計が壊れている。

- `internal/tui` は `internal/gh/cli` を **import しない**。どのバックエンドが
  動いているかを知っているのは `cmd/octoscope` だけである
- `internal/tui` は `internal/gh` を import してよい（ドメイン型を画面に出すため）
- `internal/usecase` は `internal/tui` と `internal/i18n` を import しない
- `internal/gh` と `internal/gh/cli` は `internal/usecase` を import しない
- `internal/i18n` と `internal/browser` は他の internal パッケージを import しない

この向きは目視ではなく lint で守る。`.golangci.yml` の `depguard` に禁止 import を
書き、CI で落とす。**パッケージを増やしたら、その場で depguard にも足す。**
足し忘れると、次に誰かが依存の向きを壊しても誰も気づかない。

## interface は利用側で定義する

**GitHub アクセス層は interface を export しない。** 具体型とドメイン型だけを公開する。

必要な操作の interface は、**それを使う側**が宣言する。

```go
// internal/tui/detail/detail.go
type source interface {
    GetPR(repo string, number int) (gh.PR, error)
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

<!-- TRANSIENT(Part1 Task 7): detail.Source が 6 メソッドになったらこの注記を消す -->
> **2026-09-07 時点で `internal/tui/detail` がこれを破っている。**
> `prSource` 7 / `issueSource` 6 で、同じ操作を PR と Issue で 2 本ずつ持っている
> だけである。`gh.ItemRef` は既に `Kind` を持つので、この振り分けは View の
> 仕事ではない。`internal/usecase` に移して 6 メソッドにする作業が
> `docs/superpowers/plans/2026-09-07-phase2-remediation-part1.md` の Task 7 にある。
> **新しく書く interface でこれを言い訳にしない。**

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

## 複数の API 呼び出しは `internal/usecase` に置く

**`tea.Cmd` のクロージャの中に、2 つ以上の API 呼び出しを並べない。**

「pending review が無ければ先に作る」は GitHub のレビュー API の仕様であって
TUI の都合ではない。ビューが知るべきなのは「行コメントを送る」という 1 操作だけで、
それが何回のリクエストになるかではない。

置き場所を分けると、順序のテストに Bubble Tea が要る。`internal/usecase` に置けば、
フェイクを 1 つ渡すだけで「pending があるとき / ないとき」を検証できる。

同じことが「種別（PR / Issue）で呼ぶものが変わる」にも当てはまる。
`gh.ItemRef.Kind` を View で `switch` しない。

<!-- TRANSIENT(Part1 Task 7): 呼び出し順序と振り分けを usecase に移したらこの注記を消す -->
> **2026-09-07 時点で `diff` と `review` と `detail` がこれを破っている。**
> `diff/comment.go` の `post()` が `StartReview` → `AddReviewThread` を、
> `review/review.go` の `submit()` が pending の有無で呼び分けを、
> `detail/detail.go` の `fetch` / `postComment` / `setState` / `applyPicker` が
> 種別の振り分けを持っている。移す作業は上の実装計画の Task 6〜7。
> **新しく書くコードでこれを言い訳にしない。**

## `usecase.Item` を画面の写しにしない

`usecase.Item` は PR と Issue の合流点であって DTO ではない。
**ここは UI の都合が下の層に漏れる唯一の穴**なので、太らせない。

<!-- TRANSIENT(Part1 Task 5): usecase.Item ができたらこの注記を消す -->
> **`usecase.Item` もまだ存在しない。** 作るのは上の実装計画の Task 5 で、
> この節はそのときに従う規則である。

- **`Item` に共通フィールドを足してよいのは、PR と Issue の両方に GitHub 側の
  対応物があるときだけ。**
- PR にしか無いものは `Item.PR`（`*gh.PR`）から読む。`Item` に写さない
- 「画面に出したいものが `Item` に無い」と思ったら、まず `internal/gh` の
  ドメイン型に無いのではないかを疑う。`gh.Issue` の公開フィールドは 10 個あり、
  `Item` はその全部を持つように作る（2026-09-07 に数えた）
  <!-- TRANSIENT(Part1 Task 5): 「作る」を「持っている」に直す -->

この規則があるかぎり、UI だけの修正（色・桁・文言・キー・状態遷移・
何を描くかの選び方）は `internal/usecase` に波及しない。波及するのは
GitHub への**新しい操作**を足すときだけで、それは元から UI だけの修正ではない。

## パッケージを増やす基準

責務が 1 つに言えるなら 1 パッケージ。「〜と〜をやる」と説明したくなったら分ける。

ファイルが 300 行を超えたら、責務が増えていないか疑う。行数そのものは規則ではなく、
「そろそろ見直せ」の合図として使う。

## 層を足す前に

このプロジェクトは TUI の単一バイナリであり、Web サービスではない。
DI コンテナ、ドメインモデルとインフラモデルの二重定義、Input/Output DTO は
**入れていない**。

<!-- TRANSIENT(Part1 Task 5): 「（パッケージ自体はまだ無い）」だけ消す。日付は残す -->
**`internal/usecase` を入れる判断を 2026-09-07 にした（パッケージ自体はまだ無い）。**
それまでは「Web サービス向けの構造だから入れない」という一般論で退けていたが、
その判断は `internal/tui` が 1 行も存在しない時点（Phase 0、`67ba0de`）に書かれ、
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
上の `internal/usecase` の記述が、書けたときの見本である。

足すと**増える**ものも書く。`internal/usecase` の場合は、GitHub への新しい操作を
足すときに触るファイルが `gh/cli` + ビューの 2 つから `gh/cli` + `usecase` + ビューの
3 つになる。これが唯一の実コストである。

## 規約そのものを変える

**この規約が邪魔だと感じたら、黙って逸脱しない。提案する。** 提案には次を含める。

- どの規約が、どの作業で、どう邪魔になったか（具体的な場面）
- 代わりにどうするか
- その変更で守れなくなるものは何か

規約を変えたら、**その場で該当する rules ファイルを更新する。**
更新されない合意は次のセッションには残らない。

lint で守っている規約（depguard など）を変えるときは、設定も一緒に変える。
片方だけ変えると、ドキュメントと CI が食い違ったまま放置される。
