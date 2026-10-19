# オーバーレイ（詳細・diff・checks）の取得失敗を 1 行の通知に出す（#164）

## 問題

設計 §4（`docs/superpowers/specs/2026-09-11-fetch-failures-design.md`）は、`ErrBackendUnavailable` /
`ErrUnauthenticated` 以外の失敗をその画面の 1 行に出すとする。オーバーレイだけは例外で、取得の失敗が
`fetchFailed` → `ErrorMsg` → root の `failOverlay` → `showError` で全画面のエラーになる
（`docs/superpowers/2026-09-11-fetch-failures-followups.md` で「esc で戻れるので」と見送った）。

困るのは取り直しの失敗。詳細・diff・checks で `r` を押して失敗すると、見えていた内容が全画面のエラーに
替わり、`esc` でそのオーバーレイごと閉じて板まで戻される。#152・#155 で 16 秒の時間切れが入り、
この場面は増えた。

## 方針（利用者が決めたこと: オーバーレイにも 1 行の通知を出す）

- 3 つのオーバーレイ（`internal/app/presentation/tui/{detail,diff,checks}`）に、Work 板と同じ
  `notice string` を持たせる。`fetchFailed` で振り分ける:
  - `domain.IsFatal(err)` なら今までどおり `ErrorMsg` を返す（root が全画面にする）
  - それ以外は `notice = err.Error()`。画面に出ているものは消さない。`ErrorMsg` は返さない
- 取得が成功したら `notice` を消す（Work 板の `workMsg` と同じ。古い苦情を残さない）
- 描画は Work 板の `noticeLine` と同じ形: キーバーの 1 行上に
  `theme.Error().Render(layout.Notice(i18n.T("notice.fetch_failed") + " · " + notice, m.width))`。
  その 1 行ぶん本文の高さを減らす（diff の `reviewErrHeight` / `declinedHeight` と同じやり方）。
  文言はカタログの既存の `notice.fetch_failed` を使い、新しいキーは足さない
- 開いた直後の取得が失敗したら、本文は空のまま（読み込み中の表示は止める）通知を出す。`r` で取り直せる
  （3 つとも `r` は既にある）
- **ブラウザが開けなかった（`o`）ときは全画面のまま。** 今は取得と同じ `errMsg` を通っているので、
  `openWeb` の失敗を自前のメッセージ型（例 `openFailedMsg`）に分け、そちらは今までどおり `ErrorMsg` を返す。
  全画面なら手で開くための URL を折り返さずに見せられる（root の `showError` が `browser.NoneError` を
  翻訳している）。`.claude/rules/tui.md`「メッセージ型は用途ごとに分ける」にも沿う
- root は変えない。オーバーレイからの `ErrorMsg` が致命的な失敗とブラウザの失敗だけになる
- 各ビューの中で既に 1 行に出しているもの（detail の `errText`、diff の `reviewErr`、checks のログの失敗
  `logErrMsg`）は触らない。通知の行はそれらと別の行として、同じ高さの勘定に足す
- 設計 §4 に、オーバーレイも 1 行に出す（ブラウザの失敗は全画面）ことを追記し、followups の
  「オーバーレイは全画面のまま」に #164 で直したことを 1 行追記する

却下: 設計 §4 に「オーバーレイは例外」と書くだけ。取り直しで内容が消える問題が残る。

却下: root に通知の行を持たせる。§4 が退けた形（どのオーバーレイの失敗かを表す状態が root に増える）。

範囲外: 設計 §4 は「ブラウザが開けなかったのは 1 行で足りる」とも書くが、オーバーレイのブラウザの失敗は
この PR では全画面のまま（上の理由）。

## タスク

各タスクは、まず失敗するテストを書き、実装して緑にし、検証している行を一時的に壊してテストが落ちることを
確かめてから戻す（`.claude/rules/testing.md`）。

1. **detail。** `detail_test.go` に:
   - 致命的でない取得の失敗で `ErrorMsg` を出さず、`View()` に通知が出る
   - 表示中に取り直し（`r`）が失敗しても、本文（タイトルなど）が残る
   - 取り直しが成功すると通知が消える
   - `domain.ErrUnauthenticated` を包んだ失敗は今までどおり `ErrorMsg`
   - `openWeb` の失敗は今までどおり `ErrorMsg`
   - golden: 通知が出ている状態を en/ja × 80/120/160 で録る（`make golden`、diff を目で見る）
2. **diff。** 1 と同じ項目（`openWeb` が無ければその項目は無し）。`reviewErr` と通知が同時に出ても、
   本文の高さが 2 行ぶん減り、キーバーが画面内に残ること
3. **checks。** 1 と同じ項目。ジョブのログを開いている状態で一覧の取り直しが失敗しても、ログが残ること
4. **root のシナリオ。** `root` に、キー入力だけで「Work → enter で詳細 → `r` が失敗 → 全画面に行かない
   （`View()` に通知、エラー画面ではない）→ `esc` で板に戻る」を通すテスト。フェイクの `GetItem` を
   2 回目だけ失敗させる
5. **ドキュメント。** 設計 §4 と followups に追記（上の方針の最後の項）
6. `make check`（lint の出力が `0 issues.` であることも見る）
7. 実際に起動して確かめる。#152 の shim（PATH の先頭に置いた `gh`。印のファイルがあると `sleep 600`）で、
   詳細・diff・checks を開いてから印を置いて `r` → 16 秒後に通知が出て内容が残る → 印を消して `r` で
   通知が消える、を見る。開く前に印を置いた場合（開いた直後の失敗）も見る。`--lang ja` の 80 桁でも見る
