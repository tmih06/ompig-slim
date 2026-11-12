package coding

// Ports packages/coding-agent/src/core/agent-session.ts (state accessors).

import (
	"github.com/MichaelKinsy/PiG/agent"
	icodingagent "github.com/MichaelKinsy/PiG/internal/codingagent"
)

// SteeringMode reports how the Agent drains queued steering messages (agent-session.ts get steeringMode).
func (s *Session) SteeringMode() agent.QueueMode { return s.agent.SteeringMode() }

// FollowUpMode reports how the Agent drains queued follow-up messages (agent-session.ts get followUpMode).
func (s *Session) FollowUpMode() agent.QueueMode { return s.agent.FollowUpMode() }

// SupportsThinking reports whether the current model reasons (agent-session.ts supportsThinking: `!!this.model?.reasoning`).
// A model also reasons when its capabilities name a maximum thinking level.
func (s *Session) SupportsThinking() bool {
	model := s.Model()
	return model != nil && (model.ProviderMeta.Reasoning || model.Capabilities.MaxThinking != "")
}

// AutoCompactionEnabled reports the global compaction setting (agent-session.ts get autoCompactionEnabled).
func (s *Session) AutoCompactionEnabled() bool {
	return s.SettingsManager().GetCompactionEnabled()
}

// SetAutoCompactionEnabled persists the global compaction setting (agent-session.ts setAutoCompactionEnabled). Like Pi's settings setters
// it does not fail: a write failure is recorded by the settings manager for the settings diagnostics.
func (s *Session) SetAutoCompactionEnabled(enabled bool) {
	if sm := s.SettingsManager(); sm != nil {
		// upstream: packages/coding-agent/src/core/settings-manager.ts:enqueueWrite
		_ = sm.SetCompactionEnabled(enabled)
	}
}

// AutoRetryEnabled reports the global retry setting (agent-session.ts get autoRetryEnabled).
func (s *Session) AutoRetryEnabled() bool {
	return s.SettingsManager().GetRetryEnabled()
}

// IsRetrying reports whether an automatic retry wait is in progress (agent-session.ts get isRetrying: the retry abort controller exists).
func (s *Session) IsRetrying() bool {
	s.retryMu.Lock()
	defer s.retryMu.Unlock()
	return s.retryCancel != nil
}

// RetryAttempt is the current automatic retry attempt, 0 when not retrying (agent-session.ts get retryAttempt).
func (s *Session) RetryAttempt() int { return int(s.retryAttempt.Load()) }

// IsBashRunning reports whether a bash command is running (agent-session.ts get isBashRunning).
func (s *Session) IsBashRunning() bool {
	s.bashMu.Lock()
	defer s.bashMu.Unlock()
	return len(s.bashCancels) > 0
}

// HasPendingBashMessages reports whether bash results wait to be flushed after the agent turn (agent-session.ts get hasPendingBashMessages).
func (s *Session) HasPendingBashMessages() bool {
	s.pendingBashMu.Lock()
	defer s.pendingBashMu.Unlock()
	return len(s.pendingBashMessages) > 0
}

// ExportToJsonl writes the current branch (the session header, then every entry on the branch path) to outputPath and
// returns the path written; an empty path becomes a timestamped file in the working directory (agent-session.ts exportToJsonl).
func (s *Session) ExportToJsonl(outputPath string) (string, error) {
	return icodingagent.ExportSessionToJsonl(s.inner, outputPath, nil)
}
