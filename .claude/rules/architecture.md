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
     ↓
internal/tui  ──→  internal/gh      （GitHub へのアクセス）──→ internal/browser
     ↓        ──→  internal/config   （設定ファイル）
internal/i18n     internal/browser   （どちらも誰にも依存しない）
```

**下の層は上の層を知らない。** GitHub アクセス層が TUI を import したら設計が壊れている。
`internal/i18n` と `internal/browser` は他の internal パッケージを import しない。

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

判定は簡単で、テスト用のフェイクを書いたときに、そのテストが呼ばないメソッドの
スタブをいくつ書かされたかを見る。数個で済むなら適切、十個を超えるなら大きすぎる。

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

## パッケージを増やす基準

責務が 1 つに言えるなら 1 パッケージ。「〜と〜をやる」と説明したくなったら分ける。

ファイルが 300 行を超えたら、責務が増えていないか疑う。行数そのものは規則ではなく、
「そろそろ見直せ」の合図として使う。

## 層を足す前に

このプロジェクトは TUI の単一バイナリであり、Web サービスではない。
UseCase 層、DI コンテナ、ドメインモデルとインフラモデルの二重定義といった
Web サービス向けの構造は、現時点では**入れていない**。素通しの関数が並ぶだけになり、
変更のたびに触る場所が増えるからである。

これは「一生入れない」という意味ではない。層を足したくなったときは、
次の 2 つを書けるか確かめる。

1. **この層が無いと何が壊れるか。** 実際に起きた、または確実に起きる問題を挙げる
2. **足したあと何が減るか。** 触る場所、重複、テストの手間のどれが減るか

両方書けるなら提案する価値がある。書けないなら足さない。

## 規約そのものを変える

**この規約が邪魔だと感じたら、黙って逸脱しない。提案する。** 提案には次を含める。

- どの規約が、どの作業で、どう邪魔になったか（具体的な場面）
- 代わりにどうするか
- その変更で守れなくなるものは何か

規約を変えたら、**その場で該当する rules ファイルを更新する。**
更新されない合意は次のセッションには残らない。

lint で守っている規約（depguard など）を変えるときは、設定も一緒に変える。
片方だけ変えると、ドキュメントと CI が食い違ったまま放置される。
