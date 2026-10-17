// Package conformance checks that every way of making a workspace from a template behaves,
// under what an agent does with git, exactly as a written checkout does — and never
// changes the template. A strategy is chosen automatically only if it passes here (roadmap
// D5). The tests need what each strategy needs — root for an overlay, an XFS directory for
// reflink (GITD_CONFORMANCE_XFS) — and skip without it; CI gives them all.
package conformance
