// Package nav holds the messages a tab sends when the user asks for another
// screen. A tab knows what the user picked, not what opens: root decides
// that, and these three types are the whole vocabulary between them.
//
// The detail view has its own OpenDiffMsg and OpenChecksMsg rather than
// using these. That is not duplication: root answers them with
// openDiffOverDetail and openChecksOverDetail, so the sending package is
// what says whether the new screen stacks on top of a detail view.
package nav

import "github.com/kukv/octoscope/internal/app/domain"

type OpenDetailMsg struct{ Ref domain.ItemRef }

type OpenDiffMsg struct{ Ref domain.ItemRef }

type OpenChecksMsg struct{ Ref domain.ItemRef }
