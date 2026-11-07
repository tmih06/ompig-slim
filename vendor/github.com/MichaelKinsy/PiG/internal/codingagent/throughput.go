// Ports packages/coding-agent/src/core/throughput.ts of omp-slim.
package codingagent

import (
	"strconv"
	"strings"
	"time"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/internal/jsstring"
	"github.com/MichaelKinsy/PiG/tui"
)

// throughputNow is the clock showThroughput measures a run with; a variable so tests can pin it.
var throughputNow = time.Now

// formatThroughput is the throughput summary shown after a run in interactive mode, for example:
//
//	TPS 12.9 tok/s. out 76, in 210, cache r/w 192,256/0, total 192,542, 5.9s
//
// out is the generated tokens, so it is the numerator of the rate. in, cache r/w and total are sums over the run's assistant
// messages. Time is measured from agent_start, so it includes tool execution and provider waiting; it uses Go's compact duration
// form (for example, 25m50.7s) so long runs are easier to scan. It reports nothing when the run generated no tokens or has no
// measurable elapsed time.
func formatThroughput(messages []agent.AgentMessage, elapsedMs float64) string {
	input, output, cacheRead, cacheWrite, totalTokens := 0, 0, 0, 0, 0
	for _, message := range messages {
		// throughput.ts skips every message whose role is not "assistant"; a message assembled without usage
		// contributes nothing.
		if message.Role() != "assistant" || message.Assistant == nil || message.Assistant.Usage == nil {
			continue
		}
		usage := message.Assistant.Usage
		input += usage.Input
		output += usage.Output
		cacheRead += usage.CacheRead
		cacheWrite += usage.CacheWrite
		totalTokens += usage.TotalTokens
	}
	elapsedSeconds := elapsedMs / 1000
	if output <= 0 || elapsedSeconds <= 0 {
		return ""
	}
	tokensPerSecond := float64(output) / elapsedSeconds
	return "TPS " + jsstring.ToFixed(tokensPerSecond, 1) + " tok/s. out " + formatThousands(output) +
		", in " + formatThousands(input) +
		", cache r/w " + formatThousands(cacheRead) + "/" + formatThousands(cacheWrite) +
		", total " + formatThousands(totalTokens) +
		", " + formatElapsedDuration(time.Duration(elapsedMs*float64(time.Millisecond)).Round(100*time.Millisecond))
}

// formatElapsedDuration extends time.Duration's compact format with day units; Go's default prints whole days as large hour counts.
func formatElapsedDuration(d time.Duration) string {
	const day = 24 * time.Hour
	days := d / day
	remainder := d % day
	if days == 0 {
		return remainder.String()
	}
	if remainder == 0 {
		return strconv.FormatInt(int64(days), 10) + "d"
	}
	return strconv.FormatInt(int64(days), 10) + "d " + remainder.String()
}

// formatThousands is Number.prototype.toLocaleString for an integer: comma thousands separators and a leading minus sign,
// e.g. 87432 → "87,432" and -1234 → "-1,234".
func formatThousands(n int) string {
	s := strconv.Itoa(n)
	sign := ""
	if n < 0 {
		sign, s = "-", s[1:]
	}
	if len(s) <= 3 {
		return sign + s
	}
	var b strings.Builder
	b.Grow(len(sign) + len(s) + (len(s)-1)/3)
	b.WriteString(sign)
	for i, ch := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(ch)
	}
	return b.String()
}

// showThroughput shows the throughput summary for a finished run. A run that will retry is skipped: its agent_start resets the
// timer for the retried run, which reports once. Mirrors interactive-mode.ts showThroughput.
func (m *InteractiveMode) showThroughput(messages []agent.AgentMessage, willRetry bool) {
	if m.agentStartAt.IsZero() {
		return
	}
	elapsedMs := float64(throughputNow().Sub(m.agentStartAt).Milliseconds())
	m.agentStartAt = time.Time{}
	if willRetry {
		return
	}
	line := formatThroughput(messages, elapsedMs)
	if line == "" {
		return
	}
	m.appendChatBlock(tui.NewThemedText(func() string { return tui.ActiveTheme().Fg("dim", line) }, 1, 0))
}
