package domain

import "testing"

func TestDiffLineNamesTheSideToCommentOn(t *testing.T) {
	tests := []struct {
		name string
		line DiffLine
		num  int
		side DiffSide
	}{
		{"removed lines quote the left", DiffLine{Kind: LineRemoved, OldLine: 14}, 14, SideLeft},
		{"added lines quote the right", DiffLine{Kind: LineAdded, NewLine: 15}, 15, SideRight},
		{"context quotes the right", DiffLine{Kind: LineContext, OldLine: 12, NewLine: 12}, 12, SideRight},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			num, side := tt.line.Line()
			if num != tt.num || side != tt.side {
				t.Errorf("Line() = %d %v, want %d %v", num, side, tt.num, tt.side)
			}
		})
	}
}
