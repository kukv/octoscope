# アーキテクチャ再構成 設計書

**日付:** 2026-09-13
**状態:** 設計
**置き換える対象:** `2026-09-05-octoscope-standalone-design.md` §3.1（パッケージ構成）と §3.2（バックエンド抽象）、
`2026-09-08-phase4-design.md` §7（境界）。それ以外の章は有効なまま。

## 1. 目的

octoscope は GitHub を見るためのクライアントであり、TUI はその表示手段の一つにすぎない。
現在のパッケージ構成はその関係を逆に表している。

- アプリケーション全体が `internal/tui` の下にある。`internal/tui/app` が root model で、
  「TUI でなくなった瞬間に破綻する」形に見える
- GitHub アクセス層が `internal/gh` という一つの名前で、**ドメイン型と通信実装の両方**を指している
- 将来 GitLab や Forgejo を足す余地が構造に無い

この設計は、**アプリケーション本体**（domain / usecase / adapter / presentation）と
**特定サービスへの通信**（`internal/github`）を分け、両者の間に gateway を置く。

## 2. 実測（2026-09-13）

判断の根拠はすべてこのコードで数えた値である。

### 2.1 ドメイン境界は「まだら」に壊れている

`internal/gh` はパッケージ doc に「振る舞いを持たないドメイン型」と書かれているが、実際は不均一だった。

| 領域 | 状態 | 位置 |
|---|---|---|
| `PR` `Issue` `Checks` `WorkItem` など大きい型 | **分離済み** — backend private な `prNode` / `issueNode` から `toPR()` / `toIssue()` で変換 | `gql/items.go:64,117` |
| `Author` `Label` `Comment` | **ワイヤ型がドメインに居る** — json タグ付き、直接 Unmarshal | `gh.go:38-49`、デコード 8 箇所（`cli/cli.go:186,209`、`gql/items.go:45,48,109,112`、`gql/search.go:54`、`api/lists.go:82`） |
| `ParseItemState` `ParseReviewDecision` | GitHub の enum 綴りをドメイン内部で解釈 | `gh.go:65,145` |
| `ParseFilesAPI` + `prFileJSON` | GitHub REST のデコーダがドメインに同居 | `diff_parse.go:40-59` |
| `ErrGhNotFound` `Classify` | **gh CLI という特定実装**の失敗がドメインの sentinel | `gh.go:12`。tui から 8 参照 |
| `PullRequestID` `PendingID` | GraphQL node ID がドメイン型のフィールドとして View まで到達 | `merge.go:53`、`review.go:69,78` |
| `CheckRun.JobID` `RunID` | GitHub Actions の数値 ID | `gh.go:180-182` |
| `SplitRepo`（`owner/name` 前提） | ネストした group を持つサービスで破綻 | tui 2 / infra 5 |

つまり `prNode.toPR()` という正しいパターンは既に存在しており、下 7 行に適用されていない。
**新しいモデルの二重定義を作るのではなく、既存パターンの適用漏れを塞ぐ**のがこの設計の中身である。

### 2.2 規模

- tui から `gh.*` への参照: **約 1,400 箇所 / 66 ファイル**
- ドメインのエクスポート struct: **20 型**（`LogLine` `DiffLine` `Hunk` `FileDiff` `Author` `Label` `Comment`
  `PR` `Issue` `ItemRef` `CheckRun` `Checks` `WorkItem` `RepoCount` `RepoCandidate` `MergeContext`
  `ThreadComment` `ReviewThread` `PendingComment` `ReviewContext`）
- バックエンドの json タグ付きフィールド: **245 個**（gql 195 / api 45 / cli 5）
- バックエンドのメソッド: cli 26 / api 46 / gql 25（cli と api が gql を embed）
- 既存の変換関数: 9 個（`toComments` `toRun` `toPR` `toIssue` `threadNode.toDomain`
  `threadCommentNode.toDomain` `toWorkItem` `repoJSON.toDomain` `prFileJSON.toDomain`）

### 2.3 両バックエンドには既にバイト層の芯がある

- `cli`: `c.read(ctx, dir, args...) []byte` — gh を実行して bytes（`cli/cli.go:120`）
- `api`: `send` / `readURL` / `walkPages`（Link ヘッダ追跡、maxPages、タイムアウト）（`api/rest.go`）
- `gql`: `Transport func(ctx, doc, vars) ([]byte, error)` — 送信手段を外から差す seam が既にある

### 2.4 `config` は 2 つの役割が混ざっている

`internal/config/config.go` の実測。

| 内容 | 性質 |
|---|---|
| `Language` `Icons` `DefaultTab` | 起動時に 1 回読むだけの設定。cmd が読む |
| `Repositories` `SavedQueries` + `Store.SaveRepositories` / `SaveQueries` | **アプリが実行中に書き戻すデータ** |

`SavedQuery`（Name + Query）は `config` が定義し、`usecase` が re-export して tui に渡している。
これはアプリのデータであってファイル形式の都合ではない。

### 2.5 ブラウザを開く操作が GitHub 通信層を経由している

`usecase.OpenWeb` → `cli/api.OpenWeb` → `internal/browser`。
`internal/browser` は `cli` と `api` の両方から import されている（`cli/cli.go:146`）。
URL を開くことは GitHub への通信ではない。

## 3. 採用する形

**案 A を採用する。** `internal/github` は GitHub の型を返す通信層とし、
ドメインへの変換は `app/adapter/gateway` が行う。245 個の json タグ付きフィールドが
public API になるコストは許容すると判断した。

検討した他案と却下理由は §11 に置く。

### 3.1 パッケージ構成

```
internal/
  app/
    domain/                 モデルとルール。stdlib のみ import
    usecase/                port (interface) をここで宣言。domain だけ import
    adapter/
      gateway/gh/           internal/github の型 → domain 変換、エラー分類、port 実装
      gateway/gl/           （将来）
      datasource/           アプリ外のデータストアへの保存/読み込み
    presentation/tui/       現 internal/tui。tui/app の root model は tui/root へ
    config/                 起動時設定の読み取り
  github/{gql, cli, api}    GitHub の型を返す通信層。domain を知らない
  gitlab/                   （将来）
  i18n/  golden/  browser/  他の internal に依存しないライブラリ
cmd/octoscope/              合成ルート
```

### 3.2 依存の向き

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
                                ──→ （YAML などのファイル形式）

internal/github/**             ──→ （internal/app のどこにも依存しない）
internal/app/domain            ──→ （stdlib のみ）
internal/{i18n,golden,browser} ──→ （何にも依存しない）
```

**gateway は usecase を import しない。** Go の interface は暗黙に満たされるので、
gateway は domain だけを見て port を満たす。結線は `cmd/octoscope` が行う。

### 3.3 port は usecase が宣言する

domain に `repository` パッケージは置かない。port は**使う側**、すなわち `usecase` が宣言する。

- domain は純粋なモデルとルールだけになり、機械検査（§4）をそこに集中できる
- 「interface は利用側で定義する」という既存の方針と一貫する。`detail.Source` などの
  presentation 側の小さい interface はそのまま残る

## 4. ドメインの中立性を機械で守る

規約文ではなく、CI で落ちる形にする。§2.1 の漏れは「壁が規律でできていた」から起きたので、
リネームだけでは同じことが半年後に再発する。

1. **depguard**: `internal/app/domain` は `encoding/json` と
   すべての internal パッケージを import 禁止
2. **reflect テスト**: domain のエクスポート型を歩き、struct タグが 1 つでもあれば落ちる。
   このテストは **main では落ちる**（`Author` `Label` `Comment` にタグがある）。
   PR 2 で red のまま入れて、変換の移動で green にする
3. **depguard**: `internal/github/**` は `internal/app/**` を import 禁止

## 5. 語彙と形の切り分け

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
| | `StartReview` / `SubmitNewReview` → §6 |

## 6. レビュー port の扱い

現在 `usecase/review.go:21-46` が「pending review が無ければ先に作る」を持っている。
これは GitHub のレビュー API の仕様であってアプリの都合ではないので、**gateway に移す**。

ただし **UI は pending の有無を知る必要がある。** `tui/diff/review.go:57-70` は
`PullRequestID == ""` / `PendingID == ""` の 3 分岐で「まだ分からない」と
「pending が無い」を区別しており、`review.Model.Active()` もこれに依存している。

したがって:

- `StartReview` と `SubmitNewReview` は port から消す。`SubmitReview` 1 つにまとめ、
  pending が無ければ作る判断は `gateway/gh` が持つ
- `ReviewContext` は pending のハンドルを**公開したままにする**。名前は不透明なものへ改名する
- `AddReviewThread` も同様に、pending の作成は gateway 側に寄せる

これは既存規約「複数の API 呼び出しは `internal/usecase` に置く」の**部分的な反転**である。
gateway が無かった時点では usecase が唯一の置き場所だったが、
サービス固有の呼び出し順序は gateway が持つ方が正しい。
アプリの都合による順序（種別で呼ぶものが変わる、など）は usecase に残る。

## 7. 個別の漏れの行き先

| 項目 | 行き先 | PR |
|---|---|---|
| `Author` `Label` `Comment` の json タグ | タグを外し、デコードは `internal/github` の private 型で行う | 2 |
| `ParseItemState` `ParseReviewDecision` | `internal/github`（GitHub の綴りを読む処理） | 2 |
| `ParseFilesAPI` + `prFileJSON` | `internal/github` | 3 |
| `ParseDiff`（git の unified diff） | domain に残す。GitLab も同形式 | — |
| `ErrGhNotFound` | `internal/github/cli`。gateway が中立な sentinel に変換し、`IsFatal` は中立なものだけを見る | 3 |
| `PullRequestID` `PendingID` | 不透明ハンドルへ改名。domain には残す | 3 |
| `CheckRun.JobID` `RunID` | 不透明ハンドルへ | 3 |
| `SplitRepo` | domain から外す。呼び出し元は tui 2 / infra 5 | 3 |
| `opener` port と `OpenWeb` | 削除。tui が `internal/browser` を直接呼ぶ | 3 |
| `SavedQuery` | domain の型にする。YAML との変換は datasource | 2 |

## 8. `internal/github` の公開 API

`gql` の `Client` / `Transport` / `Var` / `VarKind` がそのまま公開 API になり、
`gateway/gh` がこれを使う。`VarPlaceholder`（gh がカレントディレクトリから埋める `{owner}`）は
gh CLI 固有の概念なので `internal/github` に属する。

`cli` と `api` は 2 つの具象型のまま残る（`chooseBackend` がどちらかを返す）。
その 2 つを束ねる interface は **`gateway/gh` が利用側として宣言する**。

## 9. ViewModel は導入しない

描画用に整形した値（桁数・色・アイコン・日本語の全角幅）を model から分離する層は、
**今回は作らない。** tui から domain 型への参照は約 1,400 箇所あり、そこに変換を挟むコストが
現時点の利益を上回る。整形は現在どおり各ビューの `render.go` が持つ。

**導入を検討する基準**（この条件が出たら再考する）:

- 同じ整形（幅計算・アイコン選択・桁揃え）が 3 つ以上のビューで重複し、
  かつ `theme` や `icon` のような既存の共有パッケージに置けない
- ビューが domain 型のフィールドを 2 つ以上組み合わせて別の意味の値を作り、
  その組み合わせ方をビュー間で一致させる必要がある
- domain 型に「表示のために持っているフィールド」を足したくなる

3 番目が出たら、それは ViewModel が無いことの直接の証拠なので、そこで導入する。

## 10. PR の分け方

一括にすると、約 1,400 箇所の import 差分の中に意味のある 30 行が埋もれる。3 本に分ける。
各 PR は単独で `make check` が通る。

### PR 1: 機械的な移動とリネーム（振る舞いの変更なし）

- `internal/gh`（ルート）→ `internal/app/domain`
- `internal/gh/{cli,api,gql}` → `internal/github/{cli,api,gql}` — **この時点ではまだ domain を import する**
- `internal/tui` → `internal/app/presentation/tui`、`tui/app` → `tui/root`
- `internal/usecase` → `internal/app/usecase`
- `internal/config` → `internal/app/config`
- depguard の既存ルールを新しいパスで書き直す。§4 の 3 つの検査は、
  この時点では `internal/github` がまだ domain を import しているため入れない（PR 2 で入れる）
- `.claude/rules/` の該当箇所を書き直す（§12）
- 本設計書を新しい正とし、旧設計の該当章に置き換えを明記する

検証: `make check` が通り、`go run ./cmd/octoscope` の画面が変わらないこと。
golden テストの差分がゼロであること。

### PR 2: gateway と datasource の導入、`internal/github` の domain 非依存化

- `internal/app/adapter/gateway/gh` を作り、既存の変換関数 9 個をそこへ移す
- `internal/github` の private ワイヤ型を public にし、domain の import を外す
- §4 の reflect テストと depguard ルールを red で入れ、この PR の中で green にする
- `Author` `Label` `Comment` の json タグを外す
- `ParseItemState` `ParseReviewDecision` を `internal/github` へ
- `internal/app/adapter/datasource` を作り、`config.Store` の
  `SaveRepositories` / `SaveQueries` と `Repositories` / `SavedQueries` の読み書きを移す
- `SavedQuery` を domain の型にする

検証: reflect テストが green。depguard が `internal/github` からの domain import を落とす。
golden テストの差分がゼロ。

### PR 3: ハンドルと port の形

- `PullRequestID` `PendingID` `JobID` `RunID` を不透明ハンドルへ
- `StartReview` / `SubmitNewReview` を畳み、pending の作成を gateway へ（§6）
- `ErrGhNotFound` を `internal/github/cli` へ移し、gateway で中立 sentinel に変換。
  `IsFatal` は中立な sentinel だけを見る
- `SplitRepo` を domain から外す
- `ParseFilesAPI` + `prFileJSON` を `internal/github` へ
- `opener` port を削除し、tui が `internal/browser` を直接呼ぶ

検証: `make check`。レビューの提出（pending あり / なし）と checks の再実行を
実際に起動して確認する。

## 11. 却下した案

- **案 B（`app/github` が domain 型を返し、gateway を作らない）**: 新規 public 型ゼロで、
  変換がクエリの隣に残る。ただし通信層が domain を知り続けるため、
  「差し替え可能なインフラ」という目的を満たさない
- **案 C（`internal/github` を bytes だけの transport にし、GraphQL ドキュメントも adapter に置く）**:
  DTO を public にせずに済むが、transport の seam が均一にならない。
  GraphQL は `doc + vars → bytes` で統一できるが、cli は gh のサブコマンド、api は REST パスで、
  「何を送るか」は結局 adapter 側で cli / api に分かれる

案 A を選んだのは、`internal/github` が単体で完結した GitHub クライアントになり、
サービスごとの差が gateway の 1 箇所に集まるため。

## 12. 書き換える規約

`.claude/rules/architecture.md`:

- 依存の向きの図 → §3.2 に差し替え
- 「interface は利用側で定義する」 → 維持。gateway が usecase を import せずに port を満たすことを追記
- 「複数の API 呼び出しは `internal/usecase` に置く」 → §6 のとおり部分改訂。
  サービス固有の順序は gateway、アプリの都合の順序は usecase
- 「GitHub API 固有の値はパッケージの外に出さない」 → §4 の機械検査に強化
- 「`usecase.Item` を画面の写しにしない」 → パス名のみ更新
- 「層を足す前に」 → 節は残す。今回この節を判断の根拠にしないと決めた経緯と、
  案 A のコスト（245 フィールドの公開、変換 1 段の追加）を記録する
- 「パッケージを増やす基準」 → 維持
- §5 の規則「名前は借りてよい、形は借りない」を新設

`.claude/rules/tui.md`: frontmatter の `paths` と 109 行目の `internal/tui/theme`
`.claude/rules/testing.md`: 62 行目の `internal/tui/app`
`.claude/rules/errors.md` / `go-style.md`: パス参照なし。frontmatter のみ確認

`.golangci.yml`: depguard の全ルール（現在すべて `internal/gh` を名指ししている）を
§3.2 と §4 の形で書き直す。

`.claude/hooks/require-plan.sh` は `internal/*` / `cmd/*` のプレフィックスで判定しているため、
今回のリネームで壊れない（確認済み）。

## 13. 完了条件

1. §3.1 のツリーになっている
2. §4 の 3 つの機械検査が CI で動いている
3. §7 の表の項目がすべて指定された行き先にある
4. `make check` が通る
5. `go run ./cmd/octoscope` と `--lang ja` を実際に起動し、画面が再構成前と変わらないこと
6. §12 の規約と `.golangci.yml` が新しい構成を指している
