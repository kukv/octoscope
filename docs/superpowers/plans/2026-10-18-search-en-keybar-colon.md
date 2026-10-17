# Search: en のキーバーを `key:label` の形に揃える（#182）

## 問題

en の `footer.search.*` だけが `j/k: field` のようにコロンの後に空白を置いている。
他のタブ（`footer.list.*` の `j/k:move`）と ja（`j/k:項目`）はどれも空白を置かない。
ヒント 1 つにつき 1 桁多く使うので、`layout.FitKeyBar` が他のタブより早くヒントを落とす。

また en の Search だけ `o` のラベルが `o: open` で、Repos（`o:web`）と食い違っている。
ja はどちらも `o:Web` で揃っている。

## 方針

`internal/i18n/locales/active.en.yaml` の `footer.search.*` 17 個から、コロンの後の空白を消す。
あわせて `footer.search.web` を `o:web` にする。Go のコードと ja には手を入れない。

範囲外: `r: search again`（Repos は `r:refresh`）。Search の `r` は再検索であって
一覧の再取得ではないので、ラベルが違うのは正しい。

## タスク

1. `active.en.yaml` の `footer.search.*` を書き換える
2. `make golden` で録り直し、diff が en の Search の golden のキーバー行だけであることを確かめる。
   80 桁で落ちていたヒントが戻る golden があれば、それも期待どおりの変化として読む
3. `make check`
4. `go run ./cmd/octoscope` を 80 桁で起動し、Search タブのキーバーを見る
