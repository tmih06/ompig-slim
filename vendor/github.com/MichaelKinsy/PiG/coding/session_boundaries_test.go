package coding

import "testing"

func TestInvalidBoundaryContinuationReportsOnlyExtensionRequests(t *testing.T) {
	if shouldReportInvalidBoundaryContinuation(false) {
		t.Fatal("native settle hooks must not report an extension error")
	}
	if !shouldReportInvalidBoundaryContinuation(true) {
		t.Fatal("explicit extension continuation should remain diagnosable")
	}
}
