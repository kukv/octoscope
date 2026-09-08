# Phase 4 設計: Repos タブ / Search タブ / 設定ファイル / API フォールバック

画面の設計は
`docs/superpowers/specs/2026-09-05-octoscope-standalone-design.md`
の §4.2（Repos タブ）・§4.3（Search タブ）・§5（設定ファイル）にある。
**画面と設定項目についてはそちらが正。**
この文書は、そこに書ききれないバックエンドと境界、テスト、計画の割り方を扱う。

## 1. 範囲

| 入れる | 入れない |
|---|---|
| 設定ファイル（`config.yaml` の 5 項目） | スレッドへの返信・解決（Phase 2 の範囲外のまま） |
| Repos タブのサイドバーと追加ダイアログ | 変更量のスパークライン、リポジトリ名の副題（Phase 1 で見送り済み） |
| Search タブ（フィルタ・生クエリ編集・保存クエリ） | Work 板の `first: 50` / `labels: 100` の切り詰め（Phase 3 で記録済み、別の機会） |
| `internal/gh/api` の API フォールバック（全機能パリティ） | 生クエリからフィルタへの逆解析（§5） |

Phase 4 で spec §7 のフェーズ分割は終わる。

## 2. 実測して確かめたこと（2026-09-08）

**推測ではなく `gh` と GitHub の実出力で確認した。**
この節の値が設計の前提であり、外れたら設計を直す。

| 確かめたこと | 結果 |
|---|---|
| サイドバーの件数を 1 リクエストで引けるか | **引ける。** `repository` を alias で並べ、各々 `pullRequests(states: OPEN) { totalCount }` / `issues(states: OPEN) { totalCount }` を選ぶ。3 リポジトリで 1 リクエスト・6.5 秒（回線込み） |
| 一覧に消えた（または改名された）リポジトリが混ざったとき | **`data` は部分的に返り、その alias だけ `null` + `errors[].path` にその alias が入る。** ただし `gh api graphql` は**終了コード 1** を返す |
| その場合の `gh` の標準出力 | **JSON は出ている。** 落ちるのは `runGh` が終了コード 1 で stdout を捨てる側（§4） |
| 追加ダイアログの候補 | `gh search repos <語> --limit N --json fullName,description,isPrivate`。5 件で 1.6 秒 |
| 初回投入の材料 | `gh repo list --limit N --json nameWithOwner`（6.4 秒）と `gh api user/orgs`。Org のリポジトリは `gh repo list <org>` |
| トークンだけで GraphQL を叩けるか | **叩ける。** `https://api.github.com/graphql` に `Authorization: bearer <token>` で POST し、`{"query": "..."}` を送ると同じ JSON が返る |

件数を `search(type: ISSUE, query: "repo:a/b repo:c/d")` の 1 発で引く案は採らない。
`issueCount` は**クエリ全体の合計**であり、リポジトリごとには割れないため。

## 3. 設定ファイル（スライス 1）

`os.UserConfigDir()` 配下の `octoscope/config.yaml`。パーサは i18n と同じ
`go.yaml.in/yaml/v3` を使い、YAML の実装を 1 つに保つ。項目は spec §5 の 5 つ
（`repositories` / `saved_queries` / `default_tab` / `nerd_font` / `language`）から
増やさない。

- **無くても全機能が動く。** ファイルが無いのは既定値であって失敗ではない
- **壊れていても起動を止めない。** 解析に失敗したら既定値で続け、その旨を画面に出す。
  読めない設定のために GitHub を見られなくなるのは割に合わない
- **書き込みは temp + rename。** 追加や保存の途中で落ちても既存の設定を壊さない。
  書き込む呼び出し元（一覧への追加、クエリの保存）が生まれるのはスライス 2・3 なので、
  **書き込みもそこで足す。** スライス 1 は読み込みだけを持つ
- **`nerd_font` は `icons` に改名する。** グリフの集合は `unicode` / `nerd` / `ascii`
  の 3 つあり（`internal/tui/icon`）、真偽値の名前では `ascii` を指定できない。
  standalone spec §5 の表もこの名前に直す
- **言語の決定順に組み込む**（spec §6.3）: `--lang` → `language` → OS ロケール → en
- **`--icons` の恒久指定を `icons` に寄せる**（spec §5.1）。環境変数
  `OCTOSCOPE_ICONS` は後方互換として残し、優先順は `--icons` → `OCTOSCOPE_ICONS`
  → `icons` → 自動判定とする

`internal/config` は読み書きだけを持ち、既定値の解決（フラグ・環境変数との優先順）は
呼び出し側に置く。設定ファイルが「どこから来た値か」を知る必要はない。

## 4. Repos タブ（スライス 2）

### タブの存在条件が変わる

今の Repos タブは、起動時の `resolveRepo` がカレントのリポジトリを見つけたときだけ
現れる。一覧を設定ファイルに持つ以上、**Repos タブは常に存在する**形に変える。

カレントのリポジトリ（`--repo` または git remote）が一覧に無い場合は、
**一時的な行として先頭に並べ、選択済みにする。** 設定ファイルには書かない。
一覧に加えるのは利用者が `a` を押したときだけである（spec §4.2 の
「利用者が育てる一覧」）。

一覧が空でカレントも無いときは、自分と所属 Org のリポジトリを初期投入する導線を出す。

### `internal/gh` 側の変更

`ListPRs` / `ListIssues` は今、クライアントが握っている 1 つの
リポジトリを見る（`c.repo` を直接読む）。サイドバーは複数のリポジトリを引くので、
**`GetPR` と同じく `repo string` を先頭に取る形に変える。** 空文字はこれまでどおり
クライアントのリポジトリに落ちる（`effectiveRepo`）ので、既存の呼び出しは意味を変えない。

件数バッジは §2 のとおり alias を並べた 1 リクエストで引く。**部分的な失敗を
捨てない**: 消えたリポジトリが 1 つ混じると `gh api graphql` は終了コード 1 を返すが、
残りの件数は JSON に入っている。`runGh` は終了コード 1 のとき stdout を捨てるので、
GraphQL の呼び出し口では**終了コードではなく `errors` と `data` の中身で判断する**
経路を用意する。件数を引けなかったリポジトリはバッジを「—」で描き、行は消さない。

### ダイアログ

`internal/tui/dialog` を新設し、Repos の追加ダイアログと Search の保存クエリ
ポップアップで共用する。入力中の候補は `gh search repos`（§2）。**1 打鍵ごとには
叩かない**（1.6 秒かかる）。入力が止まってから引く。

削除は `x`（Phase 2 で予約済み。`X` は diff のコメント破棄）。

## 5. Search タブ（スライス 3）

左ペインにフィルタ項目（type / state / org / repo / author / label / review / sort）、
右ペインに結果、上段に生クエリを常時表示する。検索は
`search(type: ISSUE, query:)` の GraphQL 1 発で、Work 板と同じ形である。

**生クエリの編集は一方向とする。** `e` で編集した文字列は左のフィルタに戻さない。
戻すには GitHub の検索構文を解析する必要があり、spec §4.3 が「構文を網羅することは
目的としない」と書いた線を越える。編集後はフィルタを触った時点で組み立て直しになる
（利用者にそう見えるよう、編集中はフィルタ側を淡く描く）。

label / author の候補チップは既存の `ListLabels` / `ListAssignees` を使う。
**`repo:` が定まっていないときは候補を出さない。** GitHub にリポジトリ横断の
ラベル一覧は無く、候補の出しようがないためである。

結果の表は Repos の右ペインと同じ描き方をする。`internal/tui/repo` の表の部分を
切り出して両方から使う（今 `render.go` と `table_test.go` にある桁の割り当て）。

## 6. API フォールバック（スライス 4）

**spec §3.2 の `githubv4` を採らない。** 理由を先に書く。

`internal/gh/cli` は `.graphql` 文書を embed して `gh api graphql` に渡す形で、
`schema_test.go` が録画したスキーマに対して**全文書のフィールドを検証している**。
`githubv4` の struct クエリに書き直すと、同じクエリが 2 つの表現で二重に存在し、
検証が効くのは片方だけになる。「レビューが `diffSide` を選んで毎回失敗した」という
`schema_test.go` の由来そのものを、もう一度招く形である。

代わりにこうする。

| 面 | 手段 |
|---|---|
| GraphQL（クエリと mutation の全部） | `.graphql` 文書とデコードを共通の場所に置き、**transport だけ差し替える**。`gh api graphql` を実行する / `https://api.github.com/graphql` に POST する（§2 で実測） |
| `gh` のサブコマンドに依存している面 | `go-github` の REST で `api` 側に実装する。`pr list` / `issue list` / `repo view` / `label list` / コメント・close/reopen・ラベルと担当者の編集 / `run rerun` / `run view --log` |
| `OpenWeb` | 既存の `internal/browser`。`gh` に依存しない |

依存に増えるのは `go-github` だけで、`githubv4` は増えない。

**認証**: `GH_TOKEN` → `GITHUB_TOKEN`。起動時に `exec.LookPath("gh")` を試み、
見つかれば `cli`、見つからなければトークンで `api` を組む。どちらも無ければ、
`gh auth login` かトークンの設定を促すエラー画面を出す（spec §3.2）。

**このスライスで一番重いのは `run view --log` の代替である。** `gh` は Actions の
ログ zip を取って `ジョブ名 \t ステップ名 \t タイムスタンプ 本文` に整形しており
（Phase 3 設計 §2）、api 側はこの整形を自分で書くことになる。計画ではここを
独立したタスクに切る。

## 7. 境界

Phase 2・Phase 3 と同じ。`internal/tui` は `internal/gh/cli` も `internal/gh/api` も
import しない（depguard が落とす）。サブモデルが自分の要る操作を interface で宣言し、
`internal/usecase` がそれを実装する。**`cli` と `api` はどちらも同じドメイン型を返す
具体型**であり、共通の `Client` interface は `internal/gh` に置かない（spec §3.2、
`.claude/rules/architecture.md`「interface は利用側で定義する」）。

`internal/config` は `internal/tui` から直接読まない。設定は起動時に読んで
`app.Options` に載せ、書き戻しは Repos / Search のサブモデルが宣言する小さな
interface（一覧の保存、クエリの保存）を `internal/usecase` 側で満たす形にする。

## 8. テスト

`.claude/rules/testing.md` に従う。Phase 4 で特に効くもの:

- **ネットワークもサブプロセスも叩かない。** GraphQL の応答は実物から録った
  `testdata` の fixture を使う（個人情報とトークンは伏せる）。api の transport は
  差し替えてテストする（`cli` の `run` 差し替えと同じ形）
- **状態は `Update` にメッセージとキーを渡して到達させる。** フィールドを直接
  組み立てない（`tests-that-cannot-fail`）
- **新しく書いたテストは、検証対象を一時的に壊して落ちることを確認してから
  コミットする**
- 設定ファイルは `t.TempDir()` に対して読み書きする。壊れた YAML、無いファイル、
  書き込み中の中断（temp が残る形）をそれぞれ持つ
- golden はサイドバー付きの Repos タブ、追加ダイアログ、Search タブを
  en / ja × 80 / 120 / 160 で録る。**ja の 80 桁でキーバーが収まること**
- 新しい文字列は `active.en.yaml` と `active.ja.yaml` の両方に足す
- **`api` と `cli` が同じドメイン型を返すことを、両方に同じ検証を当てて確かめる**

## 9. 幅への対応

spec §4.6 の劣化に従う。サイドバー（Repos / Search）は **100 桁未満で畳む**。
畳んだときの Repos は現在のリポジトリ名をヘッダに残し、Search はフィルタを隠して
生クエリと結果だけにする。ダイアログは画面幅に対する相対幅で描き、80 桁でも
入力欄と候補が読めること。

## 10. 計画の割り方

4 本 → 4 PR。順に **設定ファイル → Repos → Search → api**。
設定ファイルは Repos の一覧と Search の保存クエリの共通の土台なので先頭に置く。
api を最後に置く理由は spec §7 のとおりで、先に並走させると全機能で二重実装の
負担が生じるためである。

1. **設定ファイル**（§3）
2. **Repos サイドバーと追加ダイアログ**（§4）
3. **Search タブ**（§5）
4. **api バックエンド**（§6）

**2 は 3 本に割った**（2026-09-08、計画に落とす段で数えたところ 1 本に収まらなかった）。
2-1 バックエンド（設定の `repositories`、一覧取得の per-repo 化、件数クエリ）、
2-2 サイドバー本体（Repos タブ常設・一時行・幅の劣化・golden）、
2-3 追加ダイアログ・削除・初回投入の導線。**設定ファイルの書き込み（`Save`）は
2-3 に置く。** 書き込む呼び出し元がそこで初めて生まれるためである。

各 PR は `make check` が緑であること。

## 11. 完了条件

1. `config.yaml` の 5 項目を読み書きでき、ファイルが無くても壊れていても起動する
2. 言語の決定順が `--lang` → `language` → OS ロケール → en になっている
3. Repos タブが常に存在し、一覧の追加と削除が次の起動でも残る
4. カレントのリポジトリが一覧に無くても先頭に出て、設定ファイルには書かれない
5. 件数バッジが 1 リクエストで引け、消えたリポジトリが 1 つあっても他の件数が出る
6. Search タブがフィルタから組んだクエリで検索でき、`s` 保存と `Ctrl+O` 呼び出しが
   設定ファイルに残る
7. `internal/tui` が `internal/gh/cli` も `internal/gh/api` も import していない
8. `.graphql` 文書が 1 組のまま両バックエンドから使われ、`schema_test.go` が
   その 1 組を検証している
9. golden が en / ja × 80 / 120 / 160 で録れており、ja の 80 桁でキーバーが収まる

10 は人手。**`gh` を PATH から外し `GH_TOKEN` だけで全機能が動く**こと（spec §7）。
TTY と実在のリポジトリが要るのでこの環境では代行できない。受け渡しの手順は
Phase 2・Phase 3 と同じ形で `docs/superpowers/` に書く。
