package tui

// Ports packages/coding-agent/src/core/usage-activity.ts: the local session
// history scanner behind the /usage heatmap.
//
// The scanner walks each session root, re-reading only files whose mtime or
// size changed, and aggregates billed provider requests per local calendar
// day. It never writes: OMP's stats and credential databases stay untouched.

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// DailyActivityPoint is one local calendar day of recorded provider requests,
// shared with the usage heatmap.
type DailyActivityPoint struct {
	Day      string  `json:"day"` // YYYY-MM-DD
	Cost     float64 `json:"cost"`
	Requests int     `json:"requests"`
}

// ActivityRetention is how far back the heatmap looks: 53 weeks (371 days).
const ActivityRetention = 371 * 24 * time.Hour

// activityRequest is one billed provider call recovered from a session file.
type activityRequest struct {
	identity  string
	timestamp int64 // epoch ms
	cost      float64
}

// cachedActivityFile is everything a session file contributed, keyed by its
// mtime and size so an unchanged file is never re-read.
type cachedActivityFile struct {
	mtimeMs  int64
	size     int64
	requests []activityRequest
}

// UsageActivity scans local session history without changing shared stats or
// credential databases. It is not safe for concurrent use; the dashboard owns
// one instance and calls Load from the UI loop.
type UsageActivity struct {
	roots  []string
	files  map[string]*cachedActivityFile
	points []DailyActivityPoint
}

// NewUsageActivity creates a scanner over the given session roots. Missing
// roots are skipped.
func NewUsageActivity(sessionRoots []string) *UsageActivity {
	return &UsageActivity{roots: append([]string(nil), sessionRoots...), files: map[string]*cachedActivityFile{}}
}

// SetRoots replaces the scanned session roots; the next Load rescans them.
func (a *UsageActivity) SetRoots(sessionRoots []string) {
	a.roots = append([]string(nil), sessionRoots...)
}

// Load publishes the last computed points, rescans every root, then publishes
// the fresh aggregate. Pushes stop as soon as ctx is cancelled.
func (a *UsageActivity) Load(ctx context.Context, push func([]DailyActivityPoint)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if push != nil && a.points != nil {
		push(a.points)
	}
	cutoff := time.Now().Add(-ActivityRetention).UnixMilli()
	if err := a.scan(ctx, cutoff); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	a.points = aggregateActivityDays(a.files, cutoff)
	if push != nil {
		push(a.points)
	}
	return nil
}

// scan rescans every root, re-reading only files whose mtime or size changed.
func (a *UsageActivity) scan(ctx context.Context, cutoff int64) error {
	visited := map[string]bool{}
	present := map[string]bool{}
	pending := append([]string(nil), a.roots...)
	for len(pending) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		directory := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		canonical, err := filepath.EvalSymlinks(directory)
		if err != nil {
			if isMissingActivityFile(err) {
				continue
			}
			return err
		}
		if visited[canonical] {
			continue
		}
		visited[canonical] = true
		entries, err := os.ReadDir(canonical)
		if err != nil {
			if isMissingActivityFile(err) {
				continue
			}
			return err
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			path := filepath.Join(canonical, entry.Name())
			switch {
			case entry.IsDir():
				pending = append(pending, path)
			case entry.Type()&fs.ModeSymlink != 0:
				info, statErr := os.Stat(path)
				if statErr != nil {
					if isMissingActivityFile(statErr) {
						continue
					}
					return statErr
				}
				if info.IsDir() {
					pending = append(pending, path)
				} else if strings.HasSuffix(entry.Name(), ".jsonl") {
					resolved, resolveErr := filepath.EvalSymlinks(path)
					if resolveErr != nil {
						if isMissingActivityFile(resolveErr) {
							continue
						}
						return resolveErr
					}
					if err := a.readFile(ctx, resolved, present, cutoff); err != nil {
						return err
					}
				}
			case entry.Type().IsRegular() && strings.HasSuffix(entry.Name(), ".jsonl"):
				if err := a.readFile(ctx, path, present, cutoff); err != nil {
					return err
				}
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	for path := range a.files {
		if !present[path] {
			delete(a.files, path)
		}
	}
	return nil
}

// readFile parses one session file unless its cached mtime and size still match.
func (a *UsageActivity) readFile(ctx context.Context, path string, present map[string]bool, cutoff int64) error {
	if present[path] {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		if isMissingActivityFile(err) {
			delete(present, path)
			return nil
		}
		return err
	}
	present[path] = true
	mtimeMs := info.ModTime().UnixMilli()
	if cached, ok := a.files[path]; ok && cached.mtimeMs == mtimeMs && cached.size == info.Size() {
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		if isMissingActivityFile(err) {
			delete(present, path)
			return nil
		}
		return err
	}
	defer file.Close()
	requests := make([]activityRequest, 0, 16)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		if request, ok := parseActivityRequest(scanner.Bytes()); ok && request.timestamp >= cutoff {
			requests = append(requests, request)
		}
	}
	if err := scanner.Err(); err != nil {
		if isMissingActivityFile(err) {
			delete(present, path)
			return nil
		}
		// A crash-truncated tail is not a readable file; keep the file's prior
		// contribution rather than failing the whole board.
		return nil
	}
	a.files[path] = &cachedActivityFile{mtimeMs: mtimeMs, size: info.Size(), requests: requests}
	return nil
}

// aggregateActivityDays groups cached requests by local calendar day, counting
// each billed request once across forked session copies.
func aggregateActivityDays(files map[string]*cachedActivityFile, cutoff int64) []DailyActivityPoint {
	days := map[string]*DailyActivityPoint{}
	seen := map[string]bool{}
	for _, file := range files {
		for _, request := range file.requests {
			if request.timestamp < cutoff || seen[request.identity] {
				continue
			}
			seen[request.identity] = true
			day := localActivityDay(request.timestamp)
			point, ok := days[day]
			if !ok {
				point = &DailyActivityPoint{Day: day}
				days[day] = point
			}
			point.Cost += request.cost
			point.Requests++
		}
	}
	points := make([]DailyActivityPoint, 0, len(days))
	for _, point := range days {
		points = append(points, *point)
	}
	sort.Slice(points, func(i, j int) bool { return points[i].Day < points[j].Day })
	return points
}

// localActivityDay formats an epoch-millisecond timestamp as YYYY-MM-DD in the
// machine's local timezone.
func localActivityDay(timestamp int64) string {
	date := time.UnixMilli(timestamp).Local()
	return date.Format("2006-01-02")
}

// isMissingActivityFile reports ENOENT for a path or its parent.
func isMissingActivityFile(err error) bool {
	return errors.Is(err, fs.ErrNotExist)
}

// parseActivityRequest reads one session JSONL line. Session JSONL is a disk
// trust boundary: foreign versions and crash-truncated lines are ignored.
func parseActivityRequest(line []byte) (activityRequest, bool) {
	trimmed := strings.TrimSpace(string(line))
	if trimmed == "" {
		return activityRequest{}, false
	}
	var entry map[string]any
	if err := json.Unmarshal([]byte(trimmed), &entry); err != nil {
		return activityRequest{}, false
	}
	if entry == nil {
		return activityRequest{}, false
	}
	id, ok := entry["id"].(string)
	if !ok {
		return activityRequest{}, false
	}
	source := entry
	switch entry["type"] {
	case "message":
		message, ok := entry["message"].(map[string]any)
		if !ok || message["role"] != "assistant" {
			return activityRequest{}, false
		}
		source = message
	case "usage", "model_usage", "compaction", "branch_summary":
	default:
		return activityRequest{}, false
	}
	usage, ok := source["usage"].(map[string]any)
	if !ok {
		return activityRequest{}, false
	}
	timestamp, ok := activityTimestamp(source["timestamp"], entry["timestamp"])
	if !ok {
		return activityRequest{}, false
	}
	var cost float64
	if costs, ok := usage["cost"].(map[string]any); ok {
		if amount, ok := jsonNumber(costs["total"]); ok && amount >= 0 {
			cost = amount
		}
	}
	// Forks copy entry ids and request timestamps: one billed request must
	// count only once.
	return activityRequest{identity: id + ":" + strconv.FormatInt(timestamp, 10), timestamp: timestamp, cost: cost}, true
}

// activityTimestamp prefers a finite positive numeric source timestamp, then
// an ISO entry timestamp.
func activityTimestamp(source, entry any) (int64, bool) {
	if value, ok := jsonNumber(source); ok && value > 0 && !math.IsInf(value, 0) {
		return int64(value), true
	}
	if text, ok := entry.(string); ok && text != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, text); err == nil {
			return parsed.UnixMilli(), true
		}
	}
	return 0, false
}

// jsonNumber reads a JSON number without losing integral precision.
func jsonNumber(value any) (float64, bool) {
	switch number := value.(type) {
	case float64:
		return number, true
	case json.Number:
		parsed, err := number.Float64()
		return parsed, err == nil
	}
	return 0, false
}
