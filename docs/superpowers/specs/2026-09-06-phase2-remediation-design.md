# Phase 2 立て直し 設計書

- 日付: 2026-09-06
- 対象: `internal/gh` / `internal/tui` 全体 + `.claude/rules`
- 前提設計: `docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md`
- 機能チェック表: `docs/2026-09-06-feature-checklist.md`

## 1. 背景

Phase 2 を取り込んだ時点で、テストは全パッケージ緑・カバレッジ 87〜96% でありながら、
実機では動かない機能がある。コードレビューでの指摘を実測で検証した結果、
次の 5 つが確認された。数字はすべて計測値である。

| # | 問題 | 実測 |
|---|---|---|
| 1 | 関数の責務が大きい | `detail.Update` 161 行 / `diff.Update` 123 行 / `app.Update` 85 行 |
| 2 | UI とロジックの結合 | API 呼び出し順序が `tea.Cmd` 内に 2 箇所。`detail.Source` が 19 メソッド |
| 3 | 挙動テストが薄い | キー列だけで画面をまたぐシナリオテストが 0 本 |
| 4 | テストの妥当性 | `testdata/` は `sample.diff` 1 本のみ。外部レスポンスは全部テスト内の手書き |
| 5 | コメント過多 | `diff/render.go` 27%、`app.go` 26%。実装計画への参照が非テストコードに 26 箇所 |

さらに、上記の調査中に**実バグを 1 件と潜在バグを 5 件**特定した（第 3 章）。

### 1.1 なぜこうなったか

`.claude/rules/architecture.md` の「層を足す前に」は、
**現行の `internal/tui` が 1 行も存在しない時点（`67ba0de`、Phase 0）**に書かれた。
規約自身が「層を足したくなったら 2 つの問いに答えろ」と定めているにもかかわらず、
Phase 1・Phase 2 でその問いを立て直す機会を一度も設けなかった。
本設計はその再検証であり、結論として**判断を覆す**。

## 2. 現状のアーキテクチャ評価

### 2.1 できている部分 — ACL は成立している

`internal/gh/cli` は腐敗防止層（Anti-Corruption Layer）として機能している。

| ACL の要素 | 該当 |
|---|---|
| Adapter | `cli.Client`（`gh` プロセスの実行） |
| Translator | `graphql.go` / `review.go` / `diff.go` の JSON 構造体 → ドメイン型変換 |
| 意味の隔離 | `"APPROVED"` → `gh.ReviewApproved` 等の enum 化 |

その結果 `internal/gh` は表示の関心から完全に独立している（検証済み）。

```
internal/gh の i18n import   : 0
internal/gh の時刻整形       : 0（相対時刻は internal/tui/detail/render.go）
internal/gh の Sprintf       : 2（REST の URL パス、スレッドのマップキー）
```

ドメイン型も enum で持てている: `ItemState` / `ItemKind` / `ReviewState` /
`CheckState` / `WorkSection` / `FileStatus` / `DiffLineKind`。

`internal/gh` に残る bool 7 個（`IsDraft` / `Binary` / `PatchOmitted` /
`Pending` / `Resolved` / `Outdated`）は独立した属性であり、mode ではない。
`Resolved` と `Outdated` は同時に真になり得るため enum にしてはならない。
**この層は変更しない。**

### 2.2 欠けている部分 — UseCase 層が無い

参照した設計（[UseCase層は本当に必要か](https://zenn.dev/135yshr/articles/5de289f64ec515)、
[Go の設計、どこまでやる？](https://zenn.dev/135yshr/books/go-service-design)）の
レイヤーに現状を当てはめると次のようになる。

| 層 | 該当 | 状態 |
|---|---|---|
| Handler | `internal/tui/*` の `Update` / `View` | ある |
| UseCase | — | **無い。仕事が `tea.Cmd` のクロージャに散っている** |
| Domain | `internal/gh` の型 | ある |
| Infrastructure / ACL | `internal/gh/cli` | ある |

`app.Model` は Bubble Tea の MVU における Model、すなわち UI の状態と
画面遷移を持つルートコンポーネントであって、UseCase ではない。

#### 証拠 1: 呼び出し順序が View にある

```go
// internal/tui/diff/comment.go:73
return m, func() tea.Msg {
    if reviewID == "" {
        id, err := src.StartReview(pullRequestID)      // 1 回目
        if err != nil { return commentErrorMsg{...} }
        reviewID = id
    }
    if err := src.AddReviewThread(reviewID, comment); err != nil {  // 2 回目
        return commentErrorMsg{...}
    }
    return commentPostedMsg{ref: ref, reviewID: reviewID}
}
```

「pending review が無ければ先に作る」は GitHub のレビュー API の仕様であって
TUI の都合ではない。同じ性質のものが `internal/tui/review/review.go:135` にもある
（pending の有無で `SubmitReview` / `SubmitNewReview` を切り替える）。

#### 証拠 2: 種別の振り分けが View にある

`detail.Source` は 19 メソッド。内訳は PR 用 8 + Issue 用 7 + 候補 2 + 提出 2 で、
うち 14 は同じ操作を PR / Issue で 2 本ずつ持っているだけである。

```
GetPR / GetIssue、OpenPRWeb / OpenIssueWeb、AddPRComment / AddIssueComment、
ClosePR / CloseIssue、ReopenPR / ReopenIssue、
EditPRLabels / EditIssueLabels、EditPRAssignees / EditIssueAssignees
```

`gh.ItemRef` は既に `Kind` を持っているため、この振り分けは View がやる必要がない。

他ビューの Source メソッド数は `work` 1 / `review` 2 / `candidateSource` 2 / `diff` 5 で、
19 は detail だけが突出している。

### 2.3 判断

`.claude/rules/architecture.md`「層を足す前に」の 2 つの問いに、今は答えられる。

**Q1. この層が無いと何が壊れるか**

1. API 呼び出し順序が `tea.Cmd` に漏れる（実際に 2 箇所）
2. 種別の振り分けが View に漏れ、interface が 19 メソッドに膨らむ（実際に 1 箇所）
3. 1 と 2 のどちらも Bubble Tea を起動しないとテストできない

**Q2. 足したあと何が減るか**

1. `detail.Source` 19 → 6、テスト用フェイクのスタブも 19 → 6
2. 呼び出し順序のテストが Bubble Tea 抜きで書ける
3. 次に複数呼び出しの操作が増えたときの置き場所が 1 つに決まる

両方書けるため、**`internal/usecase` を導入する。**

## 3. 特定済みのバグ

機能チェック（`docs/2026-09-06-feature-checklist.md`、2026-09-07 実施）の結果を反映済み。

### 3.0 最優先バグ: レビュー系 GraphQL クエリが必ず失敗する

`internal/gh/cli/review.graphql:17`。

```graphql
reviews(states: [PENDING], first: 1) {
  nodes {
    comments(first: 100) {
      nodes {
        path
        line
        diffSide   ← PullRequestReviewComment には存在しない
        body
```

GitHub の introspection（`gh api graphql` で実測）で確認した
`PullRequestReviewComment` のフィールド:

```
author authorAssociation body bodyHTML bodyText commit createdAt createdViaEmail
diffHunk draftedAt editor fullDatabaseId id includesCreatedEdit isMinimized
lastEditedAt line minimizedReason originalCommit originalLine originalStartLine
outdated path publishedAt pullRequest pullRequestReview reactionGroups reactions
replyTo repository resourcePath startLine state subjectType updatedAt url
userContentEdits viewerCanDelete viewerCanMinimize viewerCanReact
viewerCanUnminimize viewerCanUpdate viewerCannotUpdateReasons viewerDidAuthor
```

`diffSide` も `side` も無い。同名のフィールドは `PullRequestReviewThread` にのみ存在し、
同ファイル 30 行目の `diffSide` は正しい。GraphQL はクエリ全体を検証してから実行するため、
**`PRReviewContext` は 100% 失敗する**。

実 PR（`kukv/octoscope#55`）に対して実際に走らせ、`diff.reviewErrMsg` が返ることを確認済み。

影響: Phase 2 で追加した機能が丸ごと動かない。

- diff の既存レビュースレッド表示（チェック表 5-16）
- diff の行コメント（5a-* すべて。`c` が「読み込み中」から進まない）
- レビューの送信・破棄（5b-* すべて）
- 詳細画面の `v`

**テストが見逃した理由**: `run` を差し替えたフェイクはクエリ文字列を捨てて
手書きの JSON を返すため、クエリが構文的に不正でも検出できない。
`work.graphql` には `TestTheQueryAsksForEveryFieldWeParse` があるが、
`review.graphql` には無く、あってもスキーマとの整合は見ていない。

**修正方針**: pending コメントの side は API から直接取れないため、単純な削除では済まない。
実装時に次を実測してから決める。

- A: `reviewThreads` 側から pending を拾う（`comments.nodes.state == PENDING` で判定）
- B: pending コメントから `diffSide` を落とし、side は `reviewThreads` と突き合わせる
- C: `subjectType` / `startLine` で代替できるか

### 3.1 実バグ: Repos タブが 30 件で打ち切られる

```go
// internal/gh/cli/cli.go:74
args := appendRepo([]string{"pr", "list", "--json", prListFields}, c.repo)
// :87 も同様（issue list）
// :185 は同じファイルで --limit 100 を付けている
```

`gh pr list` / `gh issue list` は `--limit` 省略時のデフォルトが 30。
31 件目以降が黙って消える。エラーも警告も出ない。

**テストが見逃した理由**（`cli_test.go:46`）:

```go
wantArgs := []string{"pr", "list", "--json", prListFields}
```

実装が組み立てた引数をそのまま期待値に書いており、仕様ではなく実装の鏡になっている。
実装が間違っているときに落ちないため、このテストは何も守っていない。

### 3.2 潜在バグ

| 場所 | 内容 | 影響 |
|---|---|---|
| `internal/gh/gh.go:207` | `type Work [4][]WorkItem` の 4 が `WorkSections()` と二重定義 | 列を足すと実行時 panic |
| `internal/gh/cli/*.graphql` | `first:` が 8 箇所。`pageInfo` / `hasNextPage` が 1 つも無い | `reviewThreads(first:100)`、`labels(first:10)` が大きい PR で黙って切れる |
| `cli/diff.go:145,163` | `64*1024` / `8*1024*1024` が名前無しで 2 箇所に重複 | 8MiB 超の diff が読めない |
| `cli/diff.go:270` | `fields[:min(3, len(fields))]` の 3 が裸 | hunk ヘッダの解析が読めない |
| `cli/review.go:141` | `pr.Reviews.Nodes[0]` | `first: 1` に暗黙に依存 |

### 3.3 実バグ: `o`（ブラウザで開く）が WSL で動かない

チェック表 3-7 / 4-5。

```
exec: "xdg-open,x-www-browser,www-browser,wslview": executable file not found in $PATH
```

`gh ... --web` は上記 4 つを順に探すが、確認環境（Ubuntu 26.04 on WSL2）には
どれも無い。`wslu`（`wslview` の提供元）は上流の保守終了により
**Ubuntu 26.04 の apt から削除されており**、導入を前提にできない。

利用できる手段は実測で次のとおり:

```
explorer.exe    /mnt/c/WINDOWS/explorer.exe
powershell.exe  /mnt/c/WINDOWS/System32/WindowsPowerShell/v1.0/powershell.exe
cmd.exe         /mnt/c/WINDOWS/system32/cmd.exe
wslview         なし
xdg-open        なし
```

**方針**: `gh ... --web` に任せるのをやめ、octoscope が自分で URL を開く。
`gh.PR.URL` / `gh.Issue.URL` は既に取得済みで、`--web` を使う理由が無い。
そのため `OpenWeb` は ref ではなく **URL を受け取る**。Repos タブの `SelectedRef` は
`Repo` を空のままにする設計であり、ref から URL を組み立てると `gh repo view` の
往復と GitHub の URL 体系の直書きが UI 側に生まれる。

| 環境 | 起動方法 |
|---|---|
| Windows | `rundll32 url.dll,FileProtocolHandler <url>` |
| macOS | `open <url>` |
| WSL（`/proc/sys/kernel/osrelease` に `microsoft`、または `WSL_DISTRO_NAME`） | `powershell.exe -NoProfile -Command Start-Process <url>` |
| Linux | `xdg-open <url>` |

いずれの場合も `$BROWSER` が設定されていればそちらを優先する。
引数はシェルを経由させない（`.claude/rules/go-style.md`）。

どれも使えないときは `.claude/rules/errors.md` の「環境の不備」として、
**URL をそのまま画面に出して手で開けるようにする**。`wslu` の導入は案内しない
（保守終了しており、利用者が意図的に入れていない）。

`explorer.exe` は成功時も終了コード 1 を返すことが知られているため、
`powershell.exe` の `Start-Process` を採る。この事情はコードにコメントで残す。

### 3.4 再現しなかったもの: 詳細画面の `c`（チェック表 4a-1）

「`c` を押しても何も起きない」との報告。実 API を使って次を再現したが、
いずれも入力欄は正常に開いた。

- Issue #50 の詳細を開いて `c`
- PR #55 の詳細 → `d` で diff → `esc` で戻る → `c`（チェック表の実施順と同じ）

`app` 経由・`detail` 単体の両方で確認。`handleKey` の `case "c"` に到達し、
`composing` が真になり、`View()` が入力欄に変わることを確認している。
カタログの `footer.detail.comment` も `"c:comment"` で正しい。

**残る仮説**は「端末から届くキーイベントが `"c"` にならない」ことである。
確認環境は WSL 上の日本語環境であり、IME の状態や端末のキーボードプロトコル
（Kitty keyboard protocol 等）で `KeyPressMsg.String()` が変わる余地がある。

**次の手**: `--debug-keys`（受け取ったキーの `String()` をファイルに落とす）を
一時的に足して実機で確認する。原因が特定できるまでこの項目は未解決として残す。

## 4. 設計

### 4.1 パッケージ構成と依存の向き

```
cmd/octoscope                          （合成ルート: 配線 3 行）
     ↓
internal/tui  ──────→  internal/usecase  ──→  internal/gh/cli （ACL）
     │                       │
     └───────────────────────┴──────────→  internal/gh      （ドメイン型）
     ↓
internal/i18n                                              （誰にも依存しない）
```

- `internal/tui` は `internal/gh/cli` を **import しない**（現状は `cmd` 経由なので実質守られているが、明文化する）
- `internal/tui` は `internal/gh` を import してよい（ドメイン型を画面に出すため）
- `internal/usecase` は `internal/tui` と `internal/i18n` を import しない
- `internal/gh` と `internal/gh/cli` は `internal/usecase` を import しない

`.golangci.yml` の depguard に上記を追加する。**ドキュメントと lint を同時に変える。**

### 4.2 `internal/usecase` に置くもの

**(a) 種別の振り分け** — `ItemRef.Kind` で PR / Issue を分ける。

```go
type Item struct {
    Kind      gh.ItemKind
    Number    int
    Title     string
    State     gh.ItemState
    Body      string
    Labels    []gh.Label
    Assignees []gh.Author
    Comments  []gh.Comment
    UpdatedAt time.Time
    PR        *gh.PR    // Kind == ItemPR のときだけ
}

func (u *Usecase) GetItem(ctx context.Context, ref gh.ItemRef) (Item, error)
func (u *Usecase) OpenWeb(url string) error
func (u *Usecase) AddComment(ref gh.ItemRef, body string) error
func (u *Usecase) SetState(ref gh.ItemRef, closing bool) error
func (u *Usecase) EditLabels(ref gh.ItemRef, add, remove []string) error
func (u *Usecase) EditAssignees(ref gh.ItemRef, add, remove []string) error
```

`Item` は Input/Output DTO ではなく「PR と Issue の合流点」である。
`gh.PR` / `gh.Issue` をそのまま返せない（型が違う）ため必要になる型で、
これ以外に DTO は作らない。

**(b) 複数呼び出しのオーケストレーション**

```go
// pending review が無ければ作ってから、スレッドを足す。
// 戻り値の reviewID は呼び出し側が次のコメントで再利用する。
func (u *Usecase) PostLineComment(ctx context.Context, t ReviewTarget, c gh.PendingComment) (reviewID string, err error)

// pending review があれば submit、無ければ新規 review として一発で出す。
func (u *Usecase) SubmitReview(ctx context.Context, t ReviewTarget, event gh.ReviewEvent, body string) error
```

**(c) 素通しのもの** — `ListWork` / `ListPRs` / `ListIssues` / `PRDiff` /
`PRReviewContext` / `ListLabels` / `ListAssignees` / `DiscardReview`。
現時点では 1 対 1 の委譲になるが、**置き場所を統一する**ために usecase を通す。
これは参照設計の「ビジネスロジックの置き場所を統一する」に従う判断であり、
「パススルーだから省く」とはしない。

### 4.3 View 側の interface は今までどおり利用側で宣言する

`.claude/rules/architecture.md` の「interface は利用側で定義する」は変更しない。
`*usecase.Usecase` がそれらを満たす。

```go
// internal/tui/detail/detail.go
type Source interface {
    GetItem(ctx context.Context, ref gh.ItemRef) (usecase.Item, error)
    OpenWeb(url string) error
    AddComment(ref gh.ItemRef, body string) error
    SetState(ref gh.ItemRef, closing bool) error
    EditLabels(ref gh.ItemRef, add, remove []string) error
    EditAssignees(ref gh.ItemRef, add, remove []string) error
}
// 6 メソッド。候補取得(2)とレビュー提出(2)は今までどおり別 interface。
```

### 4.4 UI の状態を enum にする

`detail.Model` は bool 10 個 + エラー文字列 3 本を持っている。
名目上 1024 通りの状態のうち正しいのは 8 つほどで、
`Update` / `handleKey` / `View` がそれぞれ違う組み合わせを暗黙に前提にしている。

これを **2 つの enum に畳む**。

```go
// mode は今どのオーバーレイが出ているか。
type mode uint8
const (
    modeView    mode = iota // 本文だけ
    modeCompose             // コメント入力
    modeConfirm             // close / reopen の確認
    modePick                // ラベル / アサイニーのピッカー
    modeSubmit              // レビュー提出ポップアップ
)

// phase は現在の mode における通信状態。
type phase uint8
const (
    phaseIdle    phase = iota
    phaseLoading             // その mode に入るための取得中
    phaseWorking             // 送信中
)
```

対応表（これが機能チェック表の各行と 1 対 1 で対応する）:

| 状態 | mode | phase | 旧フィールド |
|---|---|---|---|
| 初回 / 再取得 | View | Loading | `loading` |
| 本文表示 | View | Idle | — |
| コメント入力中 | Compose | Idle | `composing` |
| コメント送信中 | Compose | Working | `posting` |
| close/reopen 確認 | Confirm | Idle | `confirming` |
| close/reopen 実行中 | Confirm | Working | `working` |
| 候補取得中 | Pick | Loading | `pickerLoading` |
| ピッカー表示 | Pick | Idle | `picking` |
| 適用中 | Pick | Working | `applying` |
| レビュー情報取得中 | Submit | Loading | `openingReview` |
| ポップアップ表示 | Submit | Idle | `submitting` |
| 送信中 | Submit | Working | `submitting` + `submit.sending` |

エラー文字列 3 本（`postErr` / `actionErr` / `submitErr`）は **1 本に統合する**。
どこに描くかは `mode` が決めるため、種類を分けて持つ必要がない。

`diff.Model` にも同じ形を適用する（`modeView` / `modeCompose` / `modeSubmit` / `modeDiscard`）。

**この変更は表示を変えない。** golden が変わらないことをもって等価性を確認する。

### 4.5 関数の分割

`Update` の `switch` の各 `case` から、状態遷移を名前のある method に出す。

```go
case commentPostedMsg:
    return m.commentPosted()
case commentErrorMsg:
    return m.commentFailed(msg.err)
```

**目安**: `Update` の 1 つの `case` は 3 行以内（呼び出しと `return` のみ）。
これは行数のための規則ではなく、「関数名を読めば何をするか分かる」ための形である。

### 4.6 GitHub API 仕様の確認

実装前に**公式ドキュメントで確認してから書く**。確認対象:

1. `gh pr list` / `gh issue list` の `--limit` の既定値と上限
2. GraphQL の Connection のページング（`pageInfo.hasNextPage` / `endCursor`）と、
   `search` / `reviewThreads` / `labels` / `comments` それぞれの上限
3. `addPullRequestReviewThread` の必須引数（`line` / `side` / `subjectType` の要否）
4. `search(type: ISSUE)` の 1000 件上限と、それを超えたときの挙動
5. REST `pulls/{n}/files` の `per_page` 上限と 300 ファイル制限

確認結果は該当箇所のコメントに**外部の事情として**残す（`.claude/rules/go-style.md` が
コメントを認める 1 つ目のケース）。

### 4.7 テスト方針

**(a) 実物を録った testdata を使う**

`internal/gh/cli/testdata/` に、実際の `gh` の出力を録って置く。

```
testdata/
  pr_list.json           gh pr list --json ... の実出力
  issue_list.json
  pr_view.json
  work.graphql.json      gh api graphql ... の実レスポンス
  review_context.json
  pr_files.json
  sample.diff            （既存）
```

録り方と、録った日・対象リポジトリを `testdata/README.md` に残す。
秘密情報は含めない（自分のリポジトリの公開 PR を使う）。

**(b) 引数のテストは「仕様」を書く**

実装が組み立てた引数をコピーした期待値を書かない。
「なぜその引数が要るのか」がテスト名から分かる形にする。

```go
func TestPRListAsksForMoreThanTheDefaultThirty(t *testing.T) {
    // gh pr list defaults to 30 items. The Repos tab must not silently
    // truncate a repository with more open PRs than that.
    ...args に "--limit" が含まれ、値が 30 より大きいことを確認...
}
```

**(c) 画面をまたぐシナリオテストを足す**

`internal/tui/app` に、**キー入力だけで**通すテストを置く。tty は要らない。

```
Work で j → enter → 詳細 → d → diff → j → c → 入力 → ctrl+s → スレッドが出る
Repos で tab → j → enter → 詳細 → x → y → 状態が変わる
詳細で l → space → enter → ラベルが変わる
```

**(d) usecase のテスト**

`PostLineComment` / `SubmitReview` の分岐（pending の有無）を
Bubble Tea を起動せずに検証する。

**(e) 空振りの確認**

新しく書いたテストは、検証対象を一時的に壊して落ちることを確認してからコミットする
（`.claude/rules/testing.md`）。

### 4.8 コメント

削除するもの:

1. **実装計画 / spec への参照** — `Task 10`、`spec 4.4.1` 等。非テストコードに 26 箇所。
   実装計画は `docs/` にあり、コードから参照する意味がない
2. **名前を言い直しているだけの doc コメント** — 非公開関数に付いた 3〜5 行の説明

残すもの:

3. **外部の事情** — 例: `repoArgs`（`review.go:41`）の
   「`gh` は `-F` の値でのみ `{owner}` を置換する」。コードからは読めない
4. **一見おかしいコードが正しい理由**
5. **エクスポートした識別子の doc コメント**

`Model` のフィールドコメントの多くは bool の山を説明するためのものであり、
4.4 の enum 化で不要になる。

## 5. 規約の変更

**この設計を実装したら、同じ PR で rules を更新する。**

| # | ファイル | 変更内容 | 根拠 |
|---|---|---|---|
| 1 | `architecture.md` | 依存図に `internal/usecase` を追加 | 2.3 |
| 2 | `architecture.md` | 「層を足す前に」を書き直す。2 つの問いのテストは残し、「Web サービス向けだから入れない」という一般論を 2.3 の実測に置き換える | 1.1 |
| 3 | `architecture.md` | interface のメソッドは **6 個まで**（判定基準ではなく上限） | detail 以外の全ビューが既に満たしている |
| 4 | `architecture.md` | 複数 API 呼び出しの順序は `internal/usecase` に置く。`tea.Cmd` の中に書かない | 2.2 証拠 1 |
| 5 | `tui.md` | UI の mode は enum。並行する bool にしない | 4.4 |
| 6 | `go-style.md` | 実装計画・spec への参照をコードコメントに書かない | 4.8 |
| 7 | `testing.md` | 外部レスポンスのパーステストは実物を録った testdata を使う | 4.7(a) |
| 8 | `testing.md` | 外部 API の引数を検証するテストは、実装の写しではなく仕様を書く | 3.1 |
| 9 | `testing.md` | 画面をまたぐ操作はキー入力だけのシナリオテストで担保する。tty は不要 | 4.7(c) |
| 10 | `.golangci.yml` | depguard に `usecase` の出入りを追加 | 4.1 |

## 6. 作業順

| # | 内容 | 検証 |
|---|---|---|
| 0 | `review.graphql` の `diffSide` 修正（3.0）+ クエリをスキーマに突き合わせるテスト | 実 PR で 5-16 / 5a / 5b が動く |
| 1 | `--limit` バグ修正 + 仕様テストへの置き換え | 実機で 31 件以上のリポジトリを開いて全部出る |
| 1.5 | `o` を octoscope 自身で開く（3.3） | WSL / Linux / macOS で 3-7 と 4-5 が通る |
| 2 | GitHub API 仕様の確認（4.6）→ ページング対応 | 100 件超のラベル / スレッドを持つ PR で切れない |
| 3 | 規約 10 項目を rules と `.golangci.yml` に反映 | `make lint` が通る |
| 4 | `internal/usecase` 導入。`detail.Source` 19 → 6、オーケストレーション 2 箇所を移動 | golden 不変。`make check` |
| 5 | `testdata` を実物で録り直す | 既存のパーステストが実物で通る |
| 6 | `detail` / `diff` の bool → enum | golden 不変。機能チェック表を再走 |
| 7 | `Update` の case を名前つき method に分割 | golden 不変 |
| 8 | コメント削除（26 箇所 + 言い直し） | `make check` |
| 9 | シナリオテスト追加 | 新テストが空振りでないことを確認 |

4 と 6 は同じファイルを触るため、4 を完了してから 6 に入る。
各段階で `make check` を通し、通らない状態でコミットしない。

## 7. スコープ外

- **DI コンテナは作らない。** `cmd/octoscope` で `cli.New → usecase.New → app.New` と
  つなぐ。これが合成ルートである
- **Input / Output DTO は作らない。** `usecase.Item` は PR と Issue の合流点であって
  DTO ではない。ドメイン型の二重定義はしない
- **`app.Model` はリネームしない。** Bubble Tea の MVU における Model という
  フレームワークの慣習であり、ドメインモデルではない
- **`internal/gh` / `internal/gh/cli` の構造は変えない。** ACL として成立している（2.1）
- **Phase 3 以降の機能（checks / merge / Search タブ）は入れない**

## 8. 完了条件

1. `docs/2026-09-06-feature-checklist.md` の全項目が `OK`
2. `make check` が通る
3. `detail.Source` が 6 メソッド以下
4. `internal/tui` に bool の mode フラグが無い
5. 非テストコードに実装計画・spec への参照が 0 箇所
6. `internal/gh/cli` のパーステストが実物の testdata を使っている
7. キー入力だけで画面をまたぐシナリオテストが 3 本以上ある
8. rules 10 項目が更新され、depguard が対応している
