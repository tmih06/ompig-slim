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
// Reading is best effort: a mention that is not an existing path is prose, so a
// failed read never rejects the prompt.

import (
	"time"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
)

// fileMentionContext is the auto-read produced for one submission.
type fileMentionContext struct {
	mentions []FileMention
	message  agent.AgentMessage
}

// readPromptFileMentions auto-reads the `@path` mentions of text against the
// session's working directory. It returns nil when the text mentions nothing
// readable.
func (m *InteractiveMode) readPromptFileMentions(text string) []FileMention {
	if m.opts.CWD == "" {
		return nil
	}
	paths := ExtractFileMentions(text)
	if len(paths) == 0 {
		return nil
	}
	return ReadFileMentions(paths, m.opts.CWD)
}

// prepareFileMentions reads a prompt's mentions and builds the message that
// delivers them beside it. A nil result means nothing was read.
func (m *InteractiveMode) prepareFileMentions(text string) *fileMentionContext {
	mentions := m.readPromptFileMentions(text)
	if len(mentions) == 0 {
		return nil
	}
	return &fileMentionContext{mentions: mentions, message: FileMentionsMessage(mentions)}
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

// steerFileMentions and followUpFileMentions deliver an auto-read with the
// queued message it belongs to, so a mention typed while a run streams reaches
// the same call as the text. The custom message renders the card at its
// message_start, which follows the queued user message.
func (m *InteractiveMode) steerFileMentions(mentions *fileMentionContext) {
	if mentions == nil || m.agent == nil {
		return
	}
	m.agent.Steer(mentions.message)
}

func (m *InteractiveMode) followUpFileMentions(mentions *fileMentionContext) {
	if mentions == nil || m.agent == nil {
		return
	}
	m.agent.FollowUp(mentions.message)
}
