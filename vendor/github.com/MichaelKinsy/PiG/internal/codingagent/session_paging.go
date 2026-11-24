// session_paging.go owns display-history paging for the resumed transcript:
// bounded walks of the active root-to-leaf path behind the painted window.
// The model context and the retained-entry set are unaffected: a page decodes
// only the entries it returns and keeps none of them resident, so cold resume
// stays cold until the user scrolls.

package codingagent

import "slices"

// BranchEntriesBefore returns the newest limit entries on the active
// root-to-leaf path that precede beforeID and whose ids are not in shown, in
// branch order (oldest first). more reports whether non-shown entries remain
// older than the returned page.
//
// found is false when beforeID is not on the active path: the painted window
// then came from a different leaf or the leaf moved without a transcript
// rebuild, and the caller must not paint a page against the wrong window.
//
// The walk is index-only; entriesAtLocked decodes just the page. Paged
// entries are not retained resident: display history never joins the
// context's resident set, matching the lazy-slot contract the cold-resume
// regression test measures (EntryReads stays zero until the user pages).
func (s *Session) BranchEntriesBefore(beforeID string, shown map[string]struct{}, limit int) (page []SessionEntry, more, found bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.leafID == nil {
		return nil, false, false
	}
	path := s.pathIndexesLocked(*s.leafID)
	i := -1
	for k, idx := range path {
		if s.slots[idx].id == beforeID {
			i = k
		}
	}
	if i < 0 {
		return nil, false, false
	}
	var idxs []int32
	j := i - 1
	for ; j >= 0; j-- {
		if len(idxs) >= limit {
			break
		}
		if _, ok := shown[s.slots[path[j]].id]; !ok {
			idxs = append(idxs, path[j])
		}
	}
	for ; j >= 0; j-- {
		if _, ok := shown[s.slots[path[j]].id]; !ok {
			more = true
			break
		}
	}
	slices.Reverse(idxs)
	return s.entriesAtLocked(idxs), more, true
}
