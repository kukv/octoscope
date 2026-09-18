# domain をモデル単位のファイルに割り直す

2026-09-19

## 1. 何を直すのか

`internal/app/domain` は 7 ファイル 792 行（非テスト）。ファイル名は
`domain.go` / `review.go` / `merge.go` / `checks.go` / `diff.go` /
`diff_parse.go` / `handle.go` で、最大の `domain.go` は 274 行に
Item・Checks・Work board・Repos・失敗の 5 系統が同居している。

ファイル名から中身が引けない。`Author` がどこにあるか知るには
`domain.go` を開くしかなく、`checks.go` に `Checks` が**無い**
（`Checks` は `domain.go`、`checks.go` にあるのは `CheckKind` と
`LogLine` と `RerunScope`）。

### なぜ今なのか

`.claude/rules/architecture.md` は「ファイルが 300 行を超えたら、責務が
増えていないか疑う」と書いている。この合図は行数の話に読まれがちだが、
疑うべきは**責務**であって行数ではない。`domain.go` はその 274 行で
まだ合図に達していないのに、既に 5 系統を抱えている。

行数が境界を決めている実例がこのリポジトリにある。`internal/app/usecase`
の `repos.go` は `usecase.go` が **299 行**のときに、`search.go` は
**300 行**のときに切り出された（`d86423a` / `3dd04ef`）。`search.go` は
2 メソッド 17 行で、`SearchItems`（GitHub に問い合わせる）と
`SaveQueries`（設定ファイルに書く）という別々の名詞が同居している。
責務で割ったならこの境界にはならない。

domain を先に直すのは、ここが**依存の葉**だからである。何も import せず、
誰からも import される。割り直しても他の層のロジックは動かない。

## 2. 制約

ユーザーが出した制約は 3 つ。

1. 1 モデル 1 ファイル
2. モデルごとに struct にする
3. モデルごとに意味のある単位にする（struct とメソッドで説明できる）

制約 2 は「モデル」に掛かるのであって「型」ではない、と読む。理由は
§5 に書く。

## 3. 変えないもの

| | |
|---|---|
| 振る舞い | 1 つも変えない。フィールド名も、メソッドの実装もそのまま移す |
| 公開 API | 型名・フィールド名・メソッド名は変えない。追加のみ |
| 依存 | `domain` は引き続き何にも依存しない。import は標準ライブラリのみ |
| struct タグ | 引き続きゼロ。`tags_test.go` が守る |

**このリファクタリングで直さないもの**（見つけたが、振る舞いを変えないため
別件にする）:

- `WorkItem.Author` は `string` だが `PR.Author` / `Issue.Author` は
  `domain.Author` struct。同じ概念が 2 つの型で表れている
- View に `ref.Kind == domain.ItemPR` の分岐が 8 箇所ある。
  `architecture.md` は「`domain.ItemRef.Kind` を View で `switch` しない」
  と定めているので、規約と実装が食い違っている

## 4. ファイル構成

29 ファイル。ファイル名は struct 名の snake_case。

### Item — 8 ファイル

| ファイル | 型 | 同居する語彙 | メソッド |
|---|---|---|---|
| `item_ref.go` | `ItemRef` | `ItemKind` | `IsPR()` **新** |
| `pull_request.go` | `PR` | — | — |
| `issue.go` | `Issue` | — | — |
| `author.go` | `Author` | — | — |
| `label.go` | `Label` | — | — |
| `comment.go` | `Comment` | — | — |
| `item_state.go` | `ItemState` | — | — |
| `review_state.go` | `ReviewState` | — | — |

`ItemState` と `ReviewState` が独立ファイルなのは、所有者が 1 つに
決まらないため。`ItemState` は PR・Issue・WorkItem が、`ReviewState` は
PR・WorkItem・MergeContext が共有する。

### Checks — 4 ファイル

| ファイル | 型 | 同居する語彙 | メソッド |
|---|---|---|---|
| `checks.go` | `Checks` | `CheckState` | — |
| `check_run.go` | `CheckRun` | `CheckKind` | `Duration()` / `HasWorkflow()` **新** |
| `log_line.go` | `LogLine` | — | — |
| `rerun.go` | `Rerun` **新** | `RerunScope` | — |

### Review — 5 ファイル

| ファイル | 型 | 同居する語彙 | メソッド |
|---|---|---|---|
| `review_context.go` | `ReviewContext` | — | `PendingCount()` |
| `review_thread.go` | `ReviewThread` | — | `Pending()` / `Collapsed()` |
| `thread_comment.go` | `ThreadComment` | — | — |
| `pending_comment.go` | `PendingComment` | — | — |
| `review_target.go` | `ReviewTarget` | `ReviewEvent` | — |

### Diff — 4 ファイル

| ファイル | 型 | 同居する語彙 | メソッド |
|---|---|---|---|
| `file_diff.go` | `FileDiff` | `FileStatus` | — |
| `hunk.go` | `Hunk` | — | — |
| `diff_line.go` | `DiffLine` | `DiffLineKind` / `DiffSide` | `Line()` |
| `diff_parser.go` | `diffParser`（非公開） | — | `ParseDiff()` / `ParseBarePatch()` |

`DiffSide` を共有 enum にしない理由は §5 の③。

### Merge — 1 ファイル

| ファイル | 型 | 同居する語彙 | メソッド |
|---|---|---|---|
| `merge_context.go` | `MergeContext` | `MergeMethod` / `Mergeable` / `MergeState` / `MergeBlock` | `Block()` / `CanAutoMerge()` / `CanMergeAsAdmin()` |

### Work board / Repos — 5 ファイル

| ファイル | 型 | 同居する語彙 | メソッド |
|---|---|---|---|
| `work.go` | `Work`（**struct 化**） | `WorkSection` | `Section()` / `SetSection()` / `WorkSections()` |
| `work_item.go` | `WorkItem` | — | — |
| `repo_count.go` | `RepoCount` | — | — |
| `repo_candidate.go` | `RepoCandidate` | — | — |
| `saved_query.go` | `SavedQuery` | — | — |

### 境界 — 2 ファイル

| ファイル | 中身 |
|---|---|
| `handle.go` | `PullRequestHandle` / `ReviewHandle` / `JobHandle` / `RunHandle` |
| `error.go` | `ErrBackendUnavailable` / `ErrUnauthenticated` / `ErrTransient` / `classified` / `Classify()` / `IsFatal()` |

最大は `merge_context.go` の約 118 行。300 行の合図に触るファイルは残らない。

## 5. 制約が衝突する 4 箇所

制約をそのまま当てると収まらない型がある。判断を明示する。

### ① enum 15 個は struct にならない

`ItemState` や `MergeBlock` は `int` の別名である。struct 化すると
`switch` が書けなくなり、比較のためだけにメソッドが増える。

**採る形:** enum はモデルではなく**モデルの属性**と見なし、所有する
モデルのファイルに同居させる。所有者が 1 つに決まらない `ItemState` と
`ReviewState` の 2 つだけ独立ファイルにする。

つまり制約 2 の適用対象は「モデル」であって「型」ではない。

#### 「所有者」の決め方

複数の型が同じ enum を使うことは珍しくない。独立ファイルにするかどうかは
**使う型の数ではなく、その概念の持ち主が言えるかどうか**で決める。

| 基準 | 例 |
|---|---|
| 値を**決める**メソッドを持つ型があるなら、その型が所有者 | `DiffSide` → `diff_line.go`。`DiffLine.Line()` が唯一「どちら側か」を決め、`ReviewThread` と `PendingComment` はその答えを運ぶだけ |
| 決めるメソッドは無いが、**その概念の本体**と言える型があるなら、その型が所有者 | `CheckState` → `checks.go`。`Checks` はチェック全体を 1 つの状態に畳んだものそのもので、`CheckRun.State` はその同じ語彙で 1 件を言ったもの |
| 対等な型が並ぶだけで本体が無いなら、所有者なし → 独立ファイル | `ItemState`（`PR` / `Issue` / `WorkItem`）、`ReviewState`（`PR` / `WorkItem` / `MergeContext`） |

`CheckState` と `ItemState` の違いは、使う型の数ではない。`Checks` と
`CheckRun` は包含関係にあり（`Checks.Runs []CheckRun`）、どちらも同じ
Checks のまとまりの中にいる。対して `PR` と `Issue` と `WorkItem` は
対等で、しかも 3 つのまとまりにまたがる。

### ② `Work` が配列型

今は `type Work [WorkSectionCount][]WorkItem` で、`w[SectionYourPRs]` と
添字で引いている。

```go
type Work struct {
    sections [WorkSectionCount][]WorkItem
}

func (w Work) Section(s WorkSection) []WorkItem
func (w *Work) SetSection(s WorkSection, items []WorkItem)
```

**採る形:** struct 化する。

- **得るもの:** フィールドが非公開になり、列の数と添字の範囲を `Work`
  自身が守る
- **払うもの:** 呼び出し側の添字アクセスが全部メソッドになる

### ③ `DiffSide` の所有者

`DiffLine` / `ReviewThread` / `PendingComment` の 3 つが使う。共有 enum に
見えるが、**どちら側かを決めているのは `DiffLine.Line()` ただ 1 つ**で、
他の 2 つはその答えを運んでいるだけである。

**採る形:** 決定を持つ型が所有者、という基準で `diff_line.go` に置く
（§5 ① の「所有者の決め方」の 1 行目）。`ItemState` / `ReviewState` と
扱いが違うのは、あちらには決定を持つ型も概念の本体も無いため。

### ④ 所有者のいない enum が 2 つ

`RerunScope`（どこまで再実行するか）と `ReviewEvent`（提出時に何と言うか）
は、モデルの属性ではなく**操作の引数**である。

**採る形:**

- `ReviewEvent` は `review_target.go` に同居させる。提出の宛先と一緒に
  渡るもののため
- `RerunScope` は `Rerun{Run RunHandle; Scope RerunScope}` を新設して
  `rerun.go` に置く。宛先と組にすると型として意味を持つ

`Rerun` は新設なので、要らないと判断するなら `RerunScope` を
`check_run.go` に同居させるだけでも制約は満たす。その場合「再実行の
宛先と範囲」は型にならず、呼ぶ側が 2 つの値を並べて持ち続ける。

### 例外: `handle.go` だけ struct にしない

4 つの handle は `string` の別名で、**中身に構造が無いことが型の意味**で
ある。フィールドを持たせるとその意味が壊れる。制約 2 の明示的な例外と
する。

## 6. 新規に足すもの

移動以外はこの 4 つだけ。

| | 何をするか | 根拠 |
|---|---|---|
| `ItemRef.IsPR() bool` | `Kind == ItemPR` を返す | 今この比較が 14 箇所に散っている（usecase 6・detail 5・work 2・search 1、2026-09-19 実測） |
| `CheckRun.HasWorkflow() bool` | `WorkflowRun` が空でないかを返す | 今 `checks` パッケージの非公開関数 `hasWorkflow()`（`checks.go:331`）にある。判断の対象は `CheckRun` なので型に返す |
| `Rerun` struct | 再実行の宛先と範囲 | `RerunScope` に所有者が無い（§5 ④） |
| `Work.Section()` / `SetSection()` | 添字アクセスの代わり | `Work` の struct 化に伴う（§5 ②） |

**`IsPR()` を足しても View の分岐は消えない。** PR と Issue の違いは
振る舞いではなくフィールドの有無なので、「PR なら Head を描く」という
判断は View に残る。減るのは API の綴りに近い `Kind ==` の比較が
14 箇所に散っていることであって、分岐の数ではない。

## 7. 呼び出し側への波及

`domain` の外に変更が要るのは 2 つだけ。

| 変更 | 波及先 |
|---|---|
| `Work` の struct 化 | `Work` を添字で引いている箇所すべて |
| `hasWorkflow()` の移設 | `internal/app/presentation/tui/checks` |

`IsPR()` と `Rerun` は**追加**なので、既存の呼び出しは壊れない。
14 箇所の `Kind ==` を `IsPR()` に置き換えるかどうかは、この spec の
範囲外とする（振る舞いを変えない移設と、呼び出し側の書き換えを
1 つの変更に混ぜない）。

## 8. テスト

既存のテスト 6 本（`domain_test.go` / `checks_test.go` / `diff_test.go` /
`diff_parse_test.go` / `merge_test.go` / `tags_test.go`）を、対応する
モデル単位に割り直す。中身は移動のみで、ケースの追加も削除もしない。

新規に足すテストは 3 つ。

- `ItemRef.IsPR()` が `ItemPR` で true、`ItemIssue` で false を返す
- `CheckRun.HasWorkflow()` が `WorkflowRun` の有無で答える
- `Work.Section()` / `SetSection()` が列を取り違えない

`tags_test.go` の公開 struct 一覧に `Rerun` を足す。`Work` は struct に
なるので、一覧に載せるかどうかを実装時に判断する（`WorkItem` を経由して
既に歩かれている可能性がある）。

## 9. 検証

1. `make check` が通る → 検証: 終了コード 0
2. `git diff --stat` で `internal/app/domain` 以外の変更が §7 の 2 つに
   限られている → 検証: 差分に出るパスを目で確認
3. `wc -l internal/app/domain/*.go` でどのファイルも 150 行以下
   → 検証: `merge_context.go` が最大で約 118 行
4. `go run ./cmd/octoscope` と `go run ./cmd/octoscope --lang ja` が
   従来どおり動く → 検証: Work board・diff・checks・merge を開いて見る

4 を外さない。`Work` の struct 化は Work board の描画に触るので、
テストが通っただけでは完了としない（`CLAUDE.md`）。

## 10. この先

この spec の範囲は `internal/app/domain` のみ。続きとして残っているもの:

- `internal/app/usecase` の名前と境界（`Usecase` が何も指していない、
  29 メソッド中 21 個が 1 行の委譲）
- `internal/app/adapter/datasource` の改名（中身は設定ファイル）
- `gateway/gh` の `work.go` / `lists.go` の境界（3 つの名詞が同居）
- TUI の `Model` に接頭辞付きフィールドで同居している別責務
  （`checks.Model` の rerun / log など）

どれも別の spec にする。
