package approvals

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/policy"
)

func TestStoreCreateAndDecide(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	store := NewStore(tmp)

	req, err := store.Create(CreateInput{
		RunID:          "run-1",
		Command:        "git push origin main",
		Class:          policy.Class2Sensitive,
		RequestedBy:    "dashboard",
		PolicyDecision: policy.DecisionRequireApproval,
		TTL:            10 * time.Minute,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if req.Status != StatusPending {
		t.Fatalf("status = %s, want pending", req.Status)
	}
	if got := filepath.Base(store.Path()); got != "approvals.json" {
		t.Fatalf("unexpected store file basename: %s", got)
	}

	got, err := store.Decide(DecideInput{
		ID:        req.ID,
		Decision:  StatusApproved,
		Approver:  "ops",
		Rationale: "change window open",
	})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if got.Status != StatusApproved {
		t.Fatalf("status = %s, want approved", got.Status)
	}
	if got.DecidedBy != "ops" {
		t.Fatalf("decided by = %q, want ops", got.DecidedBy)
	}
}

func TestStoreExpiration(t *testing.T) {
	t.Parallel()

	store := NewStore(t.TempDir())
	req, err := store.Create(CreateInput{
		Command:     "git push origin main",
		Class:       policy.Class2Sensitive,
		RequestedBy: "dashboard",
		TTL:         time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	time.Sleep(5 * time.Millisecond)
	got, err := store.Get(req.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != StatusExpired {
		t.Fatalf("status = %s, want expired", got.Status)
	}
}
