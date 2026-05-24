// Package agent — multi-role coordinator.
//
// This file contains the per-turn role classifier and the role-specific
// system-prompt addenda. The classifier is intentionally a pure function
// (no I/O, no logging) so it's trivially testable and side-effect free.
//
// Routing rules (first-match wins):
//
//   1. Forced role (user typed `/role <name>`)   → that role
//   2. Matched skill declares a preferred-role   → that role
//   3. Auto-summarize step                       → summarizer
//   4. User message regex matches review intent  → reviewer
//   5. Last assistant turn called edit/write/...  → coder
//   6. Last assistant turn called fetch/web tool → fetcher
//   7. No prior tool calls yet (fresh task)      → planner
//   8. Default                                   → planner
//
// This implements the orchestrator-worker pattern documented in
// Anthropic's 2026 multi-agent research and the Patronus routing guide:
// one decision per turn, no coordination overhead, fallback always safe.
package agent

import (
	"regexp"
	"strings"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/config"
)

// reviewIntent matches user messages that explicitly request code
// review or critique, in English or Chinese. Generous matching: we'd
// rather route a few extra turns to the reviewer than miss them.
var reviewIntent = regexp.MustCompile(
	`(?i)(\breview\b|\bcritique\b|\baudit\b|\binspect\b|\blook\s+for\s+(bugs?|issues?|problems?)\b|审核|检查|代码评审|挑错|找(?:个|出)?(?:bug|错误|问题))`,
)

// Tools whose presence in the last assistant turn indicates the agent
// is in code-editing mode. coder takes over from here.
var coderToolNames = map[string]struct{}{
	"edit":       {},
	"write":      {},
	"multiedit":  {},
	"bash":       {},
	"shell":      {},
	"apply_diff": {},
}

// Tools whose presence indicates a context-gathering step. fetcher
// handles these on the cheaper / faster small-tier model.
var fetcherToolNames = map[string]struct{}{
	"fetch":          {},
	"agentic_fetch":  {},
	"sourcegraph":    {},
	"web_search":     {},
	"web_fetch":      {},
}

// ClassifyInputs is everything the classifier needs. It's a struct so
// the call site can add new signals (e.g. cost budget hints, model
// availability) without churning every caller.
type ClassifyInputs struct {
	// UserMessage is the latest user turn's text (concatenated content
	// blocks). Empty when the turn was triggered by a queued message or
	// system signal.
	UserMessage string

	// History is the message history about to be sent upstream. The
	// classifier reads the LAST assistant turn's tool calls to detect
	// continuation patterns.
	History []fantasy.Message

	// ForcedRole comes from `/role <name>`. When non-empty it always
	// wins. Cleared after one consumption — caller responsibility.
	ForcedRole config.RoleType

	// SkillRole is the `preferred-role` frontmatter value from any
	// skill matched by the current turn. Empty when no skill matched
	// or the skill didn't declare a preference.
	SkillRole config.RoleType

	// ShouldSummarize is true on the auto-summarize trigger path. When
	// true the classifier short-circuits to summarizer regardless of
	// other signals.
	ShouldSummarize bool
}

// ClassifyRole picks the role for the next agent turn. Pure function;
// no I/O. Falls back to RolePlanner when no signal matches.
func ClassifyRole(in ClassifyInputs) config.RoleType {
	// 1. Auto-summarize wins absolutely.
	if in.ShouldSummarize {
		return config.RoleSummarizer
	}

	// 2. Forced role beats everything except summarize. The classifier
	//    doesn't consume the forced value; it's the caller's job to
	//    clear it after the turn (one-shot semantics).
	if isKnownRole(in.ForcedRole) {
		return in.ForcedRole
	}

	// 3. Skill-declared role.
	if isKnownRole(in.SkillRole) {
		return in.SkillRole
	}

	// 4. Explicit review intent in user message.
	if reviewIntent.MatchString(in.UserMessage) {
		return config.RoleReviewer
	}

	// 5/6. Look at the last assistant turn's tool calls.
	if lastTools := lastAssistantToolNames(in.History); len(lastTools) > 0 {
		// Edit tools dominate (a fetch+edit turn is still a coder turn).
		for name := range lastTools {
			if _, ok := coderToolNames[strings.ToLower(name)]; ok {
				return config.RoleCoder
			}
		}
		for name := range lastTools {
			if _, ok := fetcherToolNames[strings.ToLower(name)]; ok {
				return config.RoleFetcher
			}
		}
	}

	// 7/8. No prior tool calls or unrecognized tool — fresh planning.
	return config.RolePlanner
}

// lastAssistantToolNames returns the set of tool names called by the
// most recent assistant message. Empty when no assistant message has
// issued tool calls yet (or the history is empty).
func lastAssistantToolNames(history []fantasy.Message) map[string]struct{} {
	for i := len(history) - 1; i >= 0; i-- {
		m := history[i]
		if m.Role != fantasy.MessageRoleAssistant {
			continue
		}
		out := make(map[string]struct{})
		for _, part := range m.Content {
			if tc, ok := fantasy.AsMessagePart[fantasy.ToolCallPart](part); ok {
				out[tc.ToolName] = struct{}{}
			}
		}
		if len(out) > 0 {
			return out
		}
		// Found an assistant message with no tool calls — stop
		// scanning, the agent just produced text last.
		return nil
	}
	return nil
}

func isKnownRole(r config.RoleType) bool {
	switch r {
	case config.RolePlanner, config.RoleCoder, config.RoleReviewer, config.RoleFetcher, config.RoleSummarizer:
		return true
	}
	return false
}

// DefaultRolePromptAddenda are the per-role system-prompt fragments
// appended after the user's systemPromptPrefix. Each is short and
// directive — long addenda dilute against Crush's own coder.md.tpl
// prompt downstream. Sourced from Anthropic's published role-split
// guidance + Augment Code's 2026 routing guide.
//
// Config.RolePromptAddenda overrides individual entries when set.
var DefaultRolePromptAddenda = map[config.RoleType]string{
	config.RolePlanner: "ROLE: PLANNER. Decompose the user's request into concrete steps before doing anything. Identify the files you'll need to read, the changes you'll make, and the verification you'll run. Don't edit code — list the plan and proceed step by step. Keep the plan compact (≤8 bullets).",

	config.RoleCoder: "ROLE: CODER. You are now executing the plan with file edits, reads, and shell commands. Match existing code style exactly. Read before you edit. After every change, prefer to run the relevant tests or a focused command (lint / build / typecheck) to confirm the change works. Keep response text terse — actions speak.",

	config.RoleReviewer: "ROLE: REVIEWER. Audit the work just performed. Look for: bugs, edge cases, off-by-ones, security issues, missing error handling, broken tests, inconsistent style, dead code, accidental scope creep. Be specific — cite file:line. If you find nothing serious, say so plainly. Don't make changes unless explicitly asked — your job is to surface issues, not fix them.",

	config.RoleFetcher: "ROLE: FETCHER. Gather context efficiently: use glob for paths, grep for content, fetch for URLs. Don't over-explore — collect what's needed, then summarize what you found in ≤6 lines so the next step can act on it. Avoid speculation; report only what tools returned.",

	config.RoleSummarizer: "ROLE: SUMMARIZER. Compact this conversation while preserving the load-bearing decisions, in-flight work, file paths, exact next steps, and error messages encountered. Drop pleasantries and acknowledgements; keep the technical scaffolding the next-turn agent will need to continue without re-asking.",
}
