package subagents

// Ports src/schedule.ts and schedule-store.ts. A session owns its timer and jobs.
import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gofrs/flock"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Job struct {
	ID        string `json:"id"`
	Schedule  string `json:"schedule"`
	Params    Params `json:"params"`
	Next      int64  `json:"nextFireAt"`
	Runs      int    `json:"runs"`
	Error     string `json:"error,omitempty"`
	Cancelled bool   `json:"cancelled,omitempty"`
}

func parseInterval(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		n, err := strconv.ParseFloat(strings.TrimSuffix(s, "d"), 64)
		if err != nil || n <= 0 || n > 36500 {
			return 0, fmt.Errorf("invalid day interval")
		}
		return time.Duration(n * float64(24*time.Hour)), nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("invalid interval %q", s)
	}
	return d, nil
}
func nextFire(spec string, now time.Time) (time.Time, bool, error) {
	if strings.HasPrefix(spec, "+") {
		d, err := parseInterval(spec[1:])
		return now.Add(d), true, err
	}
	if t, err := time.Parse(time.RFC3339Nano, spec); err == nil {
		if !t.After(now) {
			return time.Time{}, true, fmt.Errorf("scheduled timestamp is in the past")
		}
		return t, true, nil
	}
	if !strings.Contains(spec, " ") {
		d, err := parseInterval(spec)
		return now.Add(d), false, err
	}
	fields := strings.Fields(spec)
	if len(fields) != 6 {
		return time.Time{}, false, fmt.Errorf("cron requires six fields: second minute hour day month weekday")
	}
	mins := []int{0, 0, 0, 1, 1, 0}
	maxs := []int{59, 59, 23, 31, 12, 7}
	allowed := make([]map[int]bool, 6)
	for i, field := range fields {
		allowed[i] = map[int]bool{}
		for _, part := range strings.Split(field, ",") {
			step := 1
			pair := strings.Split(part, "/")
			if len(pair) > 2 {
				return time.Time{}, false, fmt.Errorf("invalid cron field %q", field)
			}
			if len(pair) == 2 {
				n, err := strconv.Atoi(pair[1])
				if err != nil || n < 1 {
					return time.Time{}, false, fmt.Errorf("invalid cron step")
				}
				step = n
			}
			lo, hi := mins[i], maxs[i]
			base := pair[0]
			if base != "*" {
				bounds := strings.Split(base, "-")
				var err error
				lo, err = strconv.Atoi(bounds[0])
				if err != nil {
					return time.Time{}, false, fmt.Errorf("invalid cron field %q", field)
				}
				hi = lo
				if len(bounds) == 2 {
					hi, err = strconv.Atoi(bounds[1])
					if err != nil {
						return time.Time{}, false, err
					}
				} else if len(bounds) > 2 {
					return time.Time{}, false, fmt.Errorf("invalid cron range")
				} else if len(pair) == 2 {
					hi = maxs[i]
				}
			}
			if lo < mins[i] || hi > maxs[i] || hi < lo {
				return time.Time{}, false, fmt.Errorf("cron field out of range: %s", field)
			}
			for n := lo; n <= hi; n += step {
				allowed[i][n] = true
			}
		}
	}
	// Advance by calendar fields rather than scanning every second of a year.
	t := now.Truncate(time.Second).Add(time.Second)
	limit := t.AddDate(5, 0, 0)
	for t.Before(limit) {
		if !allowed[4][int(t.Month())] {
			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, t.Location())
			continue
		}
		dom, dow := allowed[3][t.Day()], allowed[5][int(t.Weekday())] || t.Weekday() == 0 && allowed[5][7]
		dayOK := dom && dow
		if fields[3] != "*" && fields[5] != "*" {
			dayOK = dom || dow
		}
		if !dayOK {
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, t.Location())
			continue
		}
		if !allowed[2][t.Hour()] {
			t = t.Truncate(time.Hour).Add(time.Hour)
			continue
		}
		if !allowed[1][t.Minute()] {
			t = t.Truncate(time.Minute).Add(time.Minute)
			continue
		}
		if !allowed[0][t.Second()] {
			t = t.Add(time.Second)
			continue
		}
		return t, false, nil
	}
	return time.Time{}, false, fmt.Errorf("cron has no occurrence in the next five years")
}
func (m *Manager) schedule(p Params) (Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.settings.Enabled || !m.settings.SchedulingEnabled || m.closed {
		return Record{}, fmt.Errorf("subagent scheduling is disabled")
	}
	if err := m.acquireScheduleLock(); err != nil {
		return Record{}, err
	}
	if value(p.InheritContext, false) || p.Resume != "" || p.Background != nil && !*p.Background {
		return Record{}, fmt.Errorf("schedules cannot use inherit_context, resume, or foreground execution")
	}
	next, _, err := nextFire(p.Schedule, time.Now())
	if err != nil {
		return Record{}, err
	}
	spec := p.Schedule
	p.Schedule = ""
	p.Background = new(true)
	p.Detached = true
	j := &Job{ID: newID("job_"), Schedule: spec, Params: p, Next: next.UnixMilli()}
	m.jobs = append(m.jobs, j)
	if err = m.saveJobsLocked(); err != nil {
		m.jobs = m.jobs[:len(m.jobs)-1]
		return Record{}, err
	}
	m.wg.Add(1)
	go m.runJob(j)
	return Record{ID: j.ID, Type: p.Type, Description: p.Description, Status: "scheduled", Result: "Scheduled " + spec + "; next fire " + next.Format(time.RFC3339)}, nil
}
func (m *Manager) Jobs() []Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Job, len(m.jobs))
	for i, j := range m.jobs {
		out[i] = *j
	}
	return out
}
func (m *Manager) CancelJob(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, j := range m.jobs {
		if j.ID == id {
			j.Cancelled = true
			return m.saveJobsLocked()
		}
	}
	return fmt.Errorf("scheduled job not found: %s", id)
}
func (m *Manager) jobPath() string {
	return filepath.Join(m.host.CWD, ".pi", "subagent-schedules", m.host.SessionID+".json")
}
func (m *Manager) saveJobsLocked() error {
	path := m.jobPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(m.jobs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0600)
}
func (m *Manager) restoreJobs() {
	b, err := os.ReadFile(m.jobPath())
	if err != nil {
		return
	}
	if json.Unmarshal(b, &m.jobs) != nil {
		m.jobs = nil
		return
	}
	if err := m.acquireScheduleLock(); err != nil {
		m.warnings = append(m.warnings, err.Error())
		m.jobs = nil
		return
	}
	for _, j := range m.jobs {
		if !j.Cancelled {
			m.wg.Add(1)
			go m.runJob(j)
		}
	}
}

func (m *Manager) acquireScheduleLock() error {
	if m.scheduleLock != nil {
		return nil
	}
	path := m.jobPath() + ".lock"
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	lock := flock.New(path)
	ok, err := lock.TryLock()
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("another process owns the schedules for session %s", m.host.SessionID)
	}
	m.scheduleLock = lock
	return nil
}
func (m *Manager) runJob(j *Job) {
	defer m.wg.Done()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case now := <-ticker.C:
			m.mu.Lock()
			if j.Cancelled {
				m.mu.Unlock()
				return
			}
			if !m.settings.Enabled || !m.settings.SchedulingEnabled || now.UnixMilli() < j.Next {
				m.mu.Unlock()
				continue
			}
			p := j.Params
			next, once, err := nextFire(j.Schedule, now)
			if once || err != nil {
				j.Cancelled = true
			} else {
				j.Next = next.UnixMilli()
			}
			j.Runs++
			m.mu.Unlock()
			_, spawnErr := m.Spawn(context.Background(), p, "", "")
			m.mu.Lock()
			if spawnErr != nil {
				j.Error = spawnErr.Error()
			}
			if e := m.saveJobsLocked(); e != nil {
				j.Error = e.Error()
			}
			m.mu.Unlock()
			m.changed()
			if once || err != nil {
				return
			}
		}
	}
}
