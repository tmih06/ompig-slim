package codingagent

import (
	"context"
	"encoding/json"
	"maps"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MichaelKinsy/PiG/internal/changeobservers"
)

// FooterDataProvider is data a footer reads about its surroundings (footer-data-provider.ts FooterDataProvider): the working directory and
// git branch of the repository bound to it, working-tree, upstream-divergence and GitHub pull-request summaries, extension statuses, available
// providers, and branch-change subscribers. A footer built with NewFooterComponentForSession shares one provider with the host that feeds it.
//
// Ports packages/coding-agent/src/core/footer-data-provider.ts (FooterDataProvider).
type FooterDataProvider struct {
	mu                sync.RWMutex
	cwd               string
	gitBranch         string    // cached; resolved from the initial repository binding
	gitPaths          *gitPaths // repository binding captured before extension startup
	gitStatus         gitStatusCounts
	gitStatusAt       time.Time
	pullRequest       pullRequestStatus
	pullRequestAt     time.Time
	extensionStatuses map[string]string
	providerCount     int
	// changeObservers run after the git branch or the provider count changed (ObserveChange).
	changeObservers changeobservers.Observers

	// branchChangeMu guards the OnBranchChange subscribers (footer-data-provider.ts branchChangeCallbacks).
	branchChangeMu     sync.Mutex
	branchChangeHooks  map[int]func()
	branchChangeNextID int
	// disposed and stopWatcher are footer-data-provider.ts `disposed` and its git watchers; Dispose sets and runs them.
	disposed    bool
	stopWatcher func()
}

// NewFooterDataProvider creates a provider bound to no repository, with no statuses and no providers.
func NewFooterDataProvider() *FooterDataProvider {
	return &FooterDataProvider{extensionStatuses: make(map[string]string), branchChangeHooks: make(map[int]func())}
}

// SetCwd binds the provider to the repository present at cwd, resolving its branch.
func (d *FooterDataProvider) SetCwd(cwd string) {
	d.mu.Lock()
	previous := d.gitBranch
	d.cwd = cwd
	d.gitPaths = nil
	d.gitBranch = ""
	d.gitStatus = gitStatusCounts{}
	d.gitStatusAt = time.Time{}
	d.pullRequest = pullRequestStatus{}
	d.pullRequestAt = time.Time{}
	if paths, ok := findGitPaths(cwd); cwd != "" && ok {
		d.gitPaths = &paths
		d.gitBranch = resolveGitBranchFromPaths(paths)
	}
	changed := d.gitBranch != previous
	d.mu.Unlock()
	if changed {
		d.changeObservers.Notify()
	}
}

// GetGitBranch returns the cached git branch, empty outside a repo (getGitBranch).
func (d *FooterDataProvider) GetGitBranch() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.gitBranch
}

// GetAvailableProviderCount returns the number of authenticated, reachable providers (getAvailableProviderCount).
func (d *FooterDataProvider) GetAvailableProviderCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.providerCount
}

// SetProviderCount records the number of authenticated, reachable providers (setAvailableProviderCount).
func (d *FooterDataProvider) SetProviderCount(n int) {
	d.mu.Lock()
	changed := d.providerCount != n
	d.providerCount = n
	d.mu.Unlock()
	if changed {
		d.changeObservers.Notify()
	}
}

// GetExtensionStatuses returns a snapshot of all extension statuses (getExtensionStatuses).
func (d *FooterDataProvider) GetExtensionStatuses() map[string]string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return maps.Clone(d.extensionStatuses)
}

// SetExtensionStatus sets, or with empty text clears, a keyed status entry (setExtensionStatus, clearExtensionStatuses).
func (d *FooterDataProvider) SetExtensionStatus(key, text string) {
	d.mu.Lock()
	if text == "" {
		delete(d.extensionStatuses, key)
	} else {
		d.extensionStatuses[key] = text
	}
	d.mu.Unlock()
}

// OnBranchChange subscribes to git-branch updates and returns the unsubscribe function (onBranchChange).
func (d *FooterDataProvider) OnBranchChange(fn func()) func() {
	d.branchChangeMu.Lock()
	id := d.branchChangeNextID
	d.branchChangeNextID++
	d.branchChangeHooks[id] = fn
	d.branchChangeMu.Unlock()
	return func() {
		d.branchChangeMu.Lock()
		delete(d.branchChangeHooks, id)
		d.branchChangeMu.Unlock()
	}
}

// notifyBranchChange runs the OnBranchChange subscribers in subscription order, like upstream's branchChangeCallbacks.
func (d *FooterDataProvider) notifyBranchChange() {
	d.branchChangeMu.Lock()
	ids := slices.Sorted(maps.Keys(d.branchChangeHooks))
	hooks := make([]func(), len(ids))
	for i, id := range ids {
		hooks[i] = d.branchChangeHooks[id]
	}
	d.branchChangeMu.Unlock()
	for _, fn := range hooks {
		fn()
	}
}

// setGitBranch records a branch the watcher resolved.
func (d *FooterDataProvider) setGitBranch(branch string) {
	d.mu.Lock()
	changed := d.gitBranch != branch
	d.gitBranch = branch
	if changed {
		d.pullRequest = pullRequestStatus{}
		d.pullRequestAt = time.Time{}
	}
	d.mu.Unlock()
	if changed {
		d.changeObservers.Notify()
	}
}

// ObserveChange registers observe to run after the git branch or the number of available providers changed, the values a footer reads that no extension call sets. observe runs on the goroutine that made the change and must return at once. The returned function removes it and can be called repeatedly.
func (d *FooterDataProvider) ObserveChange(observe func()) func() {
	return d.changeObservers.Add(observe)
}

// boundGitPaths returns the repository binding, or nil outside a repository.
func (d *FooterDataProvider) boundGitPaths() *gitPaths {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.gitPaths
}

// snapshot reads what one footer render needs and refreshes the short-lived git status cache.
func (d *FooterDataProvider) snapshot() (cwd, gitBranch string, providerCount int, extensionStatuses map[string]string, status gitStatusCounts, pullRequest pullRequestStatus) {
	d.mu.RLock()
	cwd, gitBranch, providerCount, extensionStatuses = d.cwd, d.gitBranch, d.providerCount, maps.Clone(d.extensionStatuses)
	d.mu.RUnlock()
	return cwd, gitBranch, providerCount, extensionStatuses, d.currentGitStatus(), d.currentPullRequest()
}

// gitStatusCounts is the compact branch summary shown beside the branch: the
// working-tree counts, and how far the branch has diverged from its upstream.
type gitStatusCounts struct {
	staged, unstaged, untracked int
	// ahead and behind count the commits this branch has that its upstream does
	// not, and the reverse. Both are what the remote-tracking ref last reported:
	// nothing here fetches, so another clone's push stays invisible until this
	// workspace fetches too. Both are zero when the branch has no upstream, or
	// when its upstream's remote-tracking ref is gone.
	ahead, behind int
}

// pullRequestStatus is the compact GitHub pull-request and check summary shown beside the branch.
type pullRequestStatus struct {
	number, green, red, pending int
}

// ghPullRequest is the subset of `gh pr view --json` needed for the footer.
type ghPullRequest struct {
	Number            int       `json:"number"`
	StatusCheckRollup []ghCheck `json:"statusCheckRollup"`
}

// ghCheck covers both CheckRun and StatusContext entries returned by GitHub CLI.
type ghCheck struct {
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	State      string `json:"state"`
}

// currentGitStatus refreshes at most once per second so a footer repaint never
// launches git for every spinner frame, while edits become visible promptly.
// The one command answers both halves of the summary: --porcelain=v2 is the
// format git documents for machines, and --branch is what makes the upstream
// divergence available at all, which is why an unpushed commit is visible here.
func (d *FooterDataProvider) currentGitStatus() gitStatusCounts {
	now := time.Now()
	d.mu.RLock()
	if now.Sub(d.gitStatusAt) < time.Second {
		status := d.gitStatus
		d.mu.RUnlock()
		return status
	}
	paths := d.gitPaths
	d.mu.RUnlock()
	if paths == nil {
		d.mu.Lock()
		d.gitStatus, d.gitStatusAt = gitStatusCounts{}, now
		d.mu.Unlock()
		return gitStatusCounts{}
	}

	status := gitStatusCounts{}
	command := exec.Command("git", "--no-optional-locks", "status", "--porcelain=v2", "--branch", "--untracked-files=normal")
	command.Dir = paths.repoDir
	if output, err := command.Output(); err == nil {
		status = parseGitStatusPorcelainV2(string(output))
	}
	d.mu.Lock()
	d.gitStatus, d.gitStatusAt = status, now
	d.mu.Unlock()
	return status
}

// parseGitStatusPorcelainV2 summarizes `git status --porcelain=v2 --branch`.
//
// Every line is classified by its first byte: "#" is a header, "1" a changed
// entry, "2" a rename or copy, "u" an unmerged entry, and "?" an untracked
// file. The header carries the divergence, which no entry line reports:
// "# branch.ab +<ahead> -<behind>", printed only while the upstream's
// remote-tracking ref exists. A branch with no upstream, and one whose upstream
// was deleted, therefore report no divergence rather than zero divergence, and
// both read as no markers — the footer cannot tell them apart, and inventing a
// marker for a state it cannot see would be worse than silence.
//
// On the entry lines the second field is "XY": X is the index's state and Y the
// working tree's, with "." meaning unchanged, so a file staged and edited again
// counts once in each. An unmerged entry carries conflict codes in both columns
// and counts as both, as it must: the conflict is in the index and in the tree.
func parseGitStatusPorcelainV2(output string) gitStatusCounts {
	status := gitStatusCounts{}
	for _, line := range strings.Split(strings.TrimSuffix(output, "\n"), "\n") {
		if len(line) < 2 {
			continue
		}
		switch line[0] {
		case '#':
			// "# branch.ab" is the only header this summary reads; branch.oid,
			// branch.head and branch.upstream describe a branch the footer already
			// names from its own watcher.
			counts, ok := strings.CutPrefix(line, "# branch.ab ")
			if !ok {
				continue
			}
			aheadText, behindText, ok := strings.Cut(counts, " ")
			if !ok {
				continue
			}
			// The format documents the signs: "+" on ahead, "-" on behind. Each
			// field already says which side it counts, so the sign is stripped and
			// only the magnitude kept.
			ahead, aheadErr := strconv.Atoi(strings.TrimPrefix(aheadText, "+"))
			behind, behindErr := strconv.Atoi(strings.TrimPrefix(behindText, "-"))
			if aheadErr == nil && behindErr == nil {
				status.ahead, status.behind = ahead, behind
			}
		case '?':
			status.untracked++
		case '1', '2', 'u':
			// "<kind> <XY> …": the two status columns sit at offsets 2 and 3,
			// so a line cut short of them counts for nothing.
			if len(line) < 4 {
				continue
			}
			if line[2] != '.' {
				status.staged++
			}
			if line[3] != '.' {
				status.unstaged++
			}
		}
	}
	return status
}

// currentPullRequest refreshes the current branch's GitHub pull request at most every 30 seconds.
// It treats a missing gh CLI, missing authentication, and branches without a PR as no result so the footer remains local-only.
func (d *FooterDataProvider) currentPullRequest() pullRequestStatus {
	now := time.Now()
	d.mu.RLock()
	if now.Sub(d.pullRequestAt) < 30*time.Second {
		status := d.pullRequest
		d.mu.RUnlock()
		return status
	}
	paths := d.gitPaths
	d.mu.RUnlock()
	if paths == nil {
		d.mu.Lock()
		d.pullRequest, d.pullRequestAt = pullRequestStatus{}, now
		d.mu.Unlock()
		return pullRequestStatus{}
	}

	status := pullRequestStatus{}
	if gh, err := exec.LookPath("gh"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, gh, "pr", "view", "--json", "number,statusCheckRollup")
		command.Dir = paths.repoDir
		if output, err := command.Output(); err == nil {
			var response ghPullRequest
			if json.Unmarshal(output, &response) == nil && response.Number > 0 {
				status.number = response.Number
				for _, check := range response.StatusCheckRollup {
					conclusion := strings.ToUpper(check.Conclusion)
					state := strings.ToUpper(check.State)
					switch {
					case conclusion == "SUCCESS" || conclusion == "SKIPPED" || conclusion == "NEUTRAL" || state == "SUCCESS":
						status.green++
					case conclusion == "FAILURE" || conclusion == "ERROR" || conclusion == "CANCELLED" || conclusion == "TIMED_OUT" || conclusion == "ACTION_REQUIRED" || state == "FAILURE" || state == "ERROR":
						status.red++
					default:
						status.pending++
					}
				}
			}
		}
	}
	d.mu.Lock()
	d.pullRequest, d.pullRequestAt = status, now
	d.mu.Unlock()
	return status
}

// Dispose is footer-data-provider.ts dispose (:187), which FooterComponent.dispose (footer.ts:93) leaves the cleanup to: it stops the git
// branch watcher and drops the OnBranchChange subscribers.
func (d *FooterDataProvider) Dispose() {
	d.branchChangeMu.Lock()
	d.disposed = true
	stop := d.stopWatcher
	d.stopWatcher = nil
	clear(d.branchChangeHooks)
	d.branchChangeMu.Unlock()
	if stop != nil {
		stop()
	}
}

// setWatcherStop registers the function that stops the git branch watcher, or runs it at once when Dispose already ran.
func (d *FooterDataProvider) setWatcherStop(stop func()) {
	d.branchChangeMu.Lock()
	disposed := d.disposed
	if !disposed {
		d.stopWatcher = stop
	}
	d.branchChangeMu.Unlock()
	if disposed {
		stop()
	}
}

func (d *FooterDataProvider) isDisposed() bool {
	d.branchChangeMu.Lock()
	defer d.branchChangeMu.Unlock()
	return d.disposed
}
