# testdata

## `sample.diff`

`git diff` 形式のパース用。unified diff の hunk ヘッダ、追加、削除、文脈行を含む。

`internal/gh/cli/diff_test.go` もこのファイルを読む（`../testdata/sample.diff`）。
パースそのものを見るテストは `internal/gh/diff_parse_test.go` にあり、`cli` 側は
`gh` の出力として妥当な何かが要るだけなので、録りものはここに 1 部だけ置く。
