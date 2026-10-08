# api: Link ヘッダを `<...>` の単位で読む（#157）

## 問題

`nextLink`（`internal/github/api/rest.go`）は `Link` ヘッダを `,` で分割してから各部を
`;` で分ける。URL の中に `,` がある（`?labels=a,b` のような問い合わせが GitHub から
そのまま next に載る）と、URL が途中で切れて `<` / `>` の検査に落ち、next が
見つからない。`walkPages` は 1 ページ目で止まり、残りを黙って失う。

## 方針

区切りを `,` ではなく `<` と `>` で取る。RFC 3986 は URL に生の `<` `>` を許さないので、
`<` から次の `>` までが URL、そこから次の `<` までがその URL の属性になる。
属性は今までどおり `;` で分け、`,` と空白を落として `rel="next"` と比べる。

範囲外: `rel="next last"` のような複数値の rel（GitHub は返さない）。

## タスク

1. テスト（赤）: next URL に `,` を含む Link ヘッダで、URL がそのまま返る。
   既存の `TestNextLinkFindsTheNextPageAmongTheOtherRelations` と並べる
2. `nextLink` を書き換えて緑にする。既存の 3 ケースと `walkPages` のテストが通ること
3. `make check`
