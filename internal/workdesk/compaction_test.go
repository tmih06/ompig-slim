package workdesk_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ompig-slim/internal/workdesk"
)

const staleSummary = "## Goal\nFinish the work.\n\n## Constraints & Preferences\n- (none)\n\n## Progress\n### Done\n- [x] everything the assignment asked for\n\n### In Progress\n- [ ] create counter.txt\n\n## Next Steps\n1. Report completion\n"

func requestText(t *testing.T, body string) string {
	t.Helper()
	var payload struct {
		Messages []struct {
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("request body: %v", err)
	}
	var texts []string
	for _, message := range payload.Messages {
		var text string
		if json.Unmarshal(message.Content, &text) == nil {
			texts = append(texts, text)
			continue
		}
		var parts []struct {
			Text string `json:"text"`
		}
		if json.Unmarshal(message.Content, &parts) == nil {
			for _, part := range parts {
				texts = append(texts, part.Text)
			}
		}
	}
	return strings.Join(texts, "\n")
}

func mustContain(t *testing.T, text string, values ...string) {
	t.Helper()
	for _, value := range values {
		if !strings.Contains(text, value) {
			t.Errorf("model context lacks %q", value)
		}
	}
}

func fillerOutput(round int) map[string]any {
	return map[string]any{"command": fmt.Sprintf("for ((i=0;i<1200;i++)); do echo \"filler %d line $i padding padding padding padding padding\"; done", round)}
}

func TestCompactionReportsNewestCommittedProgress(t *testing.T) {
	var summaries, revisions, fillers atomic.Int32
	var firstBody, finalBody string
	state, cwd := fixtureModel(t, 64000, func(w http.ResponseWriter, r *http.Request) {
		body, _ := ioReadBody(r)
		if strings.Contains(body, "context summarization assistant") {
			n := summaries.Add(1)
			answer(w, staleSummary+fmt.Sprintf("\nsummary-generation-%d", n))
			return
		}
		switch {
		case revisions.Load() == 0:
			revisions.Add(1)
			toolCall(w, "update_progress", map[string]any{
				"criteria": []string{"fixture check passes"}, "completedWork": []string{"counter.txt is created"},
				"currentStep": "implement", "constraints": []string{"keep the assignment immutable"},
				"decisions": []string{}, "blockers": []string{"fixture check is failing"},
				"nextAction": "rerun the fixture check",
			})
		case strings.Contains(body, "summary-generation-1") && revisions.Load() == 1:
			firstBody = body
			revisions.Add(1)
			toolCall(w, "update_progress", map[string]any{
				"criteria": []string{"fixture check passes"}, "completedWork": []string{"counter.txt is created", "fixture check rerun once"},
				"currentStep": "report", "constraints": []string{"keep the assignment immutable"},
				"decisions": []string{"kept the canonical durable store"}, "blockers": []string{"fixture check is failing"},
				"nextAction": "rerun the fixture check",
			})
		case strings.Contains(body, "summary-generation-2"):
			finalBody = body
			answer(w, "fixture complete")
		default:
			fillers.Add(1)
			toolCall(w, "bash", fillerOutput(int(fillers.Load())))
		}
	})
	var out bytes.Buffer
	if err := workdesk.Run(context.Background(), []string{"run", "--state", state, "--cwd", cwd, "--model", "desk-fixture/fixture", "--request-id", "compact-1", "Finish the fixture"}, &out); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if summaries.Load() < 2 || revisions.Load() != 2 {
		t.Fatalf("compactions=%d progress revisions=%d: %s", summaries.Load(), revisions.Load(), out.String())
	}
	firstText := requestText(t, firstBody)
	mustContain(t, firstText,
		"summary-generation-1",
		`"revision":1`,
		`"criteria":["fixture check passes"]`,
		`"constraints":["keep the assignment immutable"]`,
		`"blockers":["fixture check is failing"]`,
		`"nextAction":"rerun the fixture check"`,
	)
	text := requestText(t, finalBody)
	mustContain(t, text, "- [x] everything the assignment asked for", "- [ ] create counter.txt", "summary-generation-2")
	mustContain(t, text,
		`"revision":2`,
		`"criteria":["fixture check passes"]`,
		`"completedWork":["counter.txt is created","fixture check rerun once"]`,
		`"constraints":["keep the assignment immutable"]`,
		`"blockers":["fixture check is failing"]`,
		`"nextAction":"rerun the fixture check"`,
		`"currentStep":"report"`,
		`"decisions":["kept the canonical durable store"]`,
		"A generated compaction summary and older updates are history",
		"reopen work this revision supersedes",
		"promote completed work to verified acceptance",
		"Only a later update_progress commit replaces this revision",
	)
	if strings.Contains(text, `"currentStep":"implement"`) {
		t.Errorf("a superseded revision is still reported: %s", text)
	}
	if got := strings.Count(text, `"revision":`); got != 1 {
		t.Errorf("progress revisions in context = %d, want 1", got)
	}
	if got := strings.Count(text, "<ompig_progress>"); got != 1 {
		t.Errorf("progress sections in context = %d, want 1", got)
	}
	if got := strings.Count(text, "<summary>"); got != 1 {
		t.Errorf("compaction summaries in context = %d, want 1", got)
	}
	if !strings.Contains(text, "filler") {
		t.Error("recent tool output was not kept")
	}
	var result workdesk.Result
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Progress.Checkpoint == nil || result.Progress.Checkpoint.Revision != 2 {
		t.Fatalf("receipt progress: %+v", result.Progress)
	}
}

func TestResumeAfterCompactionKeepsCommitments(t *testing.T) {
	check := `test "$(cat counter.txt)" = x && test "$(cat result.txt)" = fixed && printf checked > checked.txt`
	var summaries, fillers atomic.Int32
	var progressDone, wroteDone, held, implemented, checked atomic.Bool
	killedDuringSummary := make(chan struct{})
	afterCompaction := make(chan struct{}, 1)
	var recoveredBody string
	state, cwd := fixtureModel(t, 64000, func(w http.ResponseWriter, r *http.Request) {
		body, _ := ioReadBody(r)
		if strings.Contains(body, "context summarization assistant") {
			if summaries.Add(1) == 1 {
				close(killedDuringSummary)
				<-r.Context().Done()
				return
			}
			answer(w, staleSummary)
			return
		}
		summary := strings.Contains(body, "<summary>")
		switch {
		case !progressDone.Load():
			progressDone.Store(true)
			toolCall(w, "update_progress", map[string]any{
				"criteria": []string{"counter.txt holds one mark", "result.txt holds fixed"}, "completedWork": []string{},
				"currentStep": "write the mark", "constraints": []string{"never repeat a completed mutation"},
				"decisions": []string{}, "blockers": []string{"result.txt is not implemented"}, "nextAction": "implement result.txt then check both files",
			})
		case !wroteDone.Load():
			wroteDone.Store(true)
			toolCall(w, "bash", map[string]any{"command": "printf x >> counter.txt"})
		case !summary:
			fillers.Add(1)
			toolCall(w, "bash", fillerOutput(int(fillers.Load())))
		case held.CompareAndSwap(false, true):
			afterCompaction <- struct{}{}
			<-r.Context().Done()
		case !implemented.Load():
			implemented.Store(true)
			recoveredBody = body
			toolCall(w, "write", map[string]any{"path": "result.txt", "content": "fixed\n"})
		case !checked.Load():
			checked.Store(true)
			toolCall(w, "bash", map[string]any{"command": check})
		default:
			answer(w, "fixture complete")
		}
	})
	args := []string{"run", "--state", state, "--cwd", cwd, "--model", "desk-fixture/fixture", "--request-id", "compact-crash", "Create counter.txt with one x, implement result.txt containing fixed, and check both files. Never repeat a completed mutation."}
	first, _ := startHost(t, args)
	select {
	case <-killedDuringSummary:
	case <-time.After(30 * time.Second):
		t.Fatal("the model was not summarizing")
	}
	if err := first.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = first.Wait()
	second, secondOutput := startHost(t, []string{"resume", "--state", state})
	select {
	case <-afterCompaction:
	case <-time.After(40 * time.Second):
		t.Fatalf("no request followed the committed compaction: %s", secondOutput.String())
	}
	if err := second.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = second.Wait()
	last, output := startHost(t, []string{"resume", "--state", state})
	if err := last.Wait(); err != nil {
		t.Fatalf("recovery: %v\n%s", err, output.String())
	}
	if summaries.Load() < 2 {
		t.Fatalf("the interrupted summary was not generated again: %d", summaries.Load())
	}
	raw, err := os.ReadFile(filepath.Join(cwd, "counter.txt"))
	if err != nil || string(raw) != "x" {
		t.Fatalf("the completed mutation was replayed: %q %v", raw, err)
	}
	if !checked.Load() {
		t.Fatal("the resumed run did not perform the remaining check")
	}
	text := requestText(t, recoveredBody)
	mustContain(t, text,
		"<summary>",
		`"revision":1`,
		`"criteria":["counter.txt holds one mark","result.txt holds fixed"]`,
		`"constraints":["never repeat a completed mutation"]`,
		`"blockers":["result.txt is not implemented"]`,
		`"nextAction":"implement result.txt then check both files"`,
		"Only a later update_progress commit replaces this revision",
	)
	if got := strings.Count(text, "<ompig_progress>"); got != 1 {
		t.Errorf("progress sections in the resumed request = %d, want 1", got)
	}
	if got := strings.Count(text, "<summary>"); got != 1 {
		t.Errorf("compaction summaries in the resumed request = %d, want 1", got)
	}
	if raw, err := os.ReadFile(filepath.Join(cwd, "checked.txt")); err != nil || string(raw) != "checked" {
		t.Fatalf("the remaining check did not pass: %q %v", raw, err)
	}
}
