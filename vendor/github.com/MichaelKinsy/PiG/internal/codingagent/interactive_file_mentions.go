package codingagent

// Interactive wiring for auto-read `@path` mentions.
//
// oh-my-pi reads mentions inside AgentSession.prompt after expansion and pushes
// a `fileMention` message ahead of the prepared run
// (packages/coding-agent/src/session/agent-session.ts:9484-9499 with
// packages/coding-agent/src/utils/file-mentions.ts). This runtime has no
// fileMention role, so the mentions travel as a custom message:
//
//   - A prompt that starts a turn seeds the run with the user message and the
//     mention message together, the batch shape Agent.SendContent builds for a
//     lone prompt. Passing both in one call keeps the read atomic with the
//     prompt: a run the agent refuses leaves nothing queued for the next one.
//   - A steer or follow-up enqueues its own mention message, so a mention typed
//     while a run streams lands with the text in the same queue slot.
//
// Reading is best effort: a mention that is neither a URL nor an existing path
// is prose, so a failed read never rejects the prompt.
//
// A URL mention is a network read, and a prompt is dispatched on the owner loop,
// where a fetch would freeze the editor for its whole timeout. So a prompt whose
// mentions include a URL is read on a worker and its delivery is posted back to
// the loop; a prompt whose mentions are all local files is read inline, since a
// directory listing or file read cannot block on another host.

import (
	"context"
	"time"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
)

// fileMentionContext is the auto-read produced for one submission.
type fileMentionContext struct {
	mentions []FileMention
	message  agent.AgentMessage
}

// readPromptFileMentions auto-reads the `@path` and `@url` mentions of text
// against cwd. It returns nil when the text mentions nothing readable. It
// performs network I/O for a URL mention, so it must not run on the owner loop.
//
// cwd is a parameter rather than a read of m.opts.CWD because the worker that
// fetches a URL must not touch owner-loop state: a session swap or /cd landing
// mid-fetch would otherwise race the read.
func readPromptFileMentions(ctx context.Context, text, cwd string) []FileMention {
	paths := ExtractFileMentions(text)
	if len(paths) == 0 {
		return nil
	}
	return ReadFileMentions(ctx, paths, cwd)
}

// prepareFileMentions reads a prompt's mentions and builds the message that
// delivers them beside it. A nil result means nothing was read. It shares
// readPromptFileMentions' rule about the owner loop, so cwd must already have
// been captured there.
func prepareFileMentions(ctx context.Context, text, cwd string) *fileMentionContext {
	mentions := readPromptFileMentions(ctx, text, cwd)
	if len(mentions) == 0 {
		return nil
	}
	return &fileMentionContext{mentions: mentions, message: FileMentionsMessage(mentions)}
}

// mentionsNeedNetwork reports whether reading text's mentions fetches a URL, so
// the caller can move the read off the owner loop. A local-only read stays
// inline and keeps the prompt's ordering guarantees.
func mentionsNeedNetwork(text string) bool {
	for _, mention := range ExtractFileMentions(text) {
		if isFileMentionURL(mention) {
			return true
		}
	}
	return false
}

// promptStartingMessages is the batch a prompt turn starts from: the user
// message, then the auto-read of its mentions when there is one. It is
// Agent.SendContent's shape plus the mention message, so the model reads the
// prompt and its file contents in one request, as oh-my-pi's prompt() does.
func promptStartingMessages(prompt string, content ai.UserContentBlocks, mentions *fileMentionContext) []agent.AgentMessage {
	user := agent.AgentMessage{User: &agent.UserMessage{
		Role:      agent.RoleUser,
		Content:   content,
		Timestamp: time.Now().UnixMilli(),
	}}
	if mentions == nil {
		return []agent.AgentMessage{user}
	}
	return []agent.AgentMessage{user, mentions.message}
}

// queuedPromptMessages is the delivery unit a queued prompt enqueues: the user
// message, then its auto-read when there is one. The steering and follow-up
// queues drain one slot at a time by default, so enqueueing the two separately
// would let the model answer the prompt before reading the files it named;
// Agent.SteerGroup/FollowUpGroup keep them in one slot.
func queuedPromptMessages(text string, images []ai.ImageContent, mentions *fileMentionContext) []agent.AgentMessage {
	user := agent.AgentMessage{User: &agent.UserMessage{
		Role:      agent.RoleUser,
		Content:   promptContent(text, images),
		Timestamp: time.Now().UnixMilli(),
	}}
	if mentions == nil {
		return []agent.AgentMessage{user}
	}
	return []agent.AgentMessage{user, mentions.message}
}
