// Package approvals provides durable approval request storage.
package approvals

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gofrs/flock"
	"github.com/google/uuid"
	"github.com/steveyegge/gastown/internal/policy"
	"github.com/steveyegge/gastown/internal/util"
)

// Status is the approval request status.
type Status string

const (
	StatusPending  Status = "pending"
	StatusApproved Status = "approved"
	StatusDenied   Status = "denied"
	StatusExpired  Status = "expired"
	StatusExecuted Status = "executed"
)

// Request captures an approval request lifecycle.
type Request struct {
	ID                string           `json:"id"`
	RunID             string           `json:"run_id,omitempty"`
	Command           string           `json:"command"`
	CommandHash       string           `json:"command_hash"`
	Class             policy.RiskClass `json:"class"`
	RequestedBy       string           `json:"requested_by"`
	Repo              string           `json:"repo,omitempty"`
	Status            Status           `json:"status"`
	PolicyDecision    policy.Decision  `json:"policy_decision,omitempty"`
	Reason            string           `json:"reason,omitempty"`
	CreatedAt         time.Time        `json:"created_at"`
	ExpiresAt         time.Time        `json:"expires_at"`
	DecisionAt        *time.Time       `json:"decision_at,omitempty"`
	DecidedBy         string           `json:"decided_by,omitempty"`
	DecisionRationale string           `json:"decision_rationale,omitempty"`
}

// CreateInput is the input for creating an approval request.
type CreateInput struct {
	RunID          string
	Command        string
	Class          policy.RiskClass
	RequestedBy    string
	Repo           string
	PolicyDecision policy.Decision
	Reason         string
	TTL            time.Duration
}

// DecideInput is the input for recording a decision.
type DecideInput struct {
	ID        string
	Decision  Status
	Approver  string
	Rationale string
}

type storeFile struct {
	Version  int        `json:"version"`
	Requests []*Request `json:"requests"`
}

// Store persists approval requests in daemon/approvals.json.
type Store struct {
	path     string
	lockPath string
}

// NewStore creates a new approval store bound to a town root.
func NewStore(townRoot string) *Store {
	path := filepath.Join(townRoot, "daemon", "approvals.json")
	return &Store{
		path:     path,
		lockPath: path + ".lock",
	}
}

// Path returns the underlying file path.
func (s *Store) Path() string {
	return s.path
}

// Create creates a new pending approval request.
func (s *Store) Create(input CreateInput) (*Request, error) {
	if strings.TrimSpace(input.Command) == "" {
		return nil, fmt.Errorf("command is required")
	}
	if input.TTL <= 0 {
		input.TTL = 15 * time.Minute
	}
	now := time.Now().UTC()
	req := &Request{
		ID:             "apr-" + shortID(),
		RunID:          strings.TrimSpace(input.RunID),
		Command:        strings.TrimSpace(input.Command),
		CommandHash:    HashCommand(input.Command),
		Class:          input.Class,
		RequestedBy:    defaultValue(input.RequestedBy, "system"),
		Repo:           strings.TrimSpace(input.Repo),
		Status:         StatusPending,
		PolicyDecision: input.PolicyDecision,
		Reason:         strings.TrimSpace(input.Reason),
		CreatedAt:      now,
		ExpiresAt:      now.Add(input.TTL),
	}

	err := s.withLockedStore(func(sf *storeFile) error {
		expireLocked(sf, now)
		sf.Requests = append(sf.Requests, req)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return req, nil
}

// List returns approval requests sorted newest-first.
func (s *Store) List(filter Status) ([]*Request, error) {
	sf, err := s.readLockedStore()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	expireLocked(sf, now)

	out := make([]*Request, 0, len(sf.Requests))
	for _, req := range sf.Requests {
		if filter != "" && req.Status != filter {
			continue
		}
		clone := *req
		out = append(out, &clone)
	}

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

// Get returns a single approval request by ID.
func (s *Store) Get(id string) (*Request, error) {
	sf, err := s.readLockedStore()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	expireLocked(sf, now)

	for _, req := range sf.Requests {
		if req.ID == id {
			clone := *req
			return &clone, nil
		}
	}
	return nil, fmt.Errorf("approval %q not found", id)
}

// Decide marks a pending request as approved or denied.
func (s *Store) Decide(input DecideInput) (*Request, error) {
	if input.Decision != StatusApproved && input.Decision != StatusDenied {
		return nil, fmt.Errorf("invalid decision %q", input.Decision)
	}

	var decided *Request
	now := time.Now().UTC()
	err := s.withLockedStore(func(sf *storeFile) error {
		expireLocked(sf, now)
		for _, req := range sf.Requests {
			if req.ID != input.ID {
				continue
			}
			if req.Status != StatusPending {
				return fmt.Errorf("approval %q is %s (expected pending)", req.ID, req.Status)
			}
			req.Status = input.Decision
			req.DecidedBy = defaultValue(input.Approver, "operator")
			req.DecisionRationale = strings.TrimSpace(input.Rationale)
			req.DecisionAt = ptrTime(now)
			clone := *req
			decided = &clone
			return nil
		}
		return fmt.Errorf("approval %q not found", input.ID)
	})
	if err != nil {
		return nil, err
	}
	return decided, nil
}

// MarkExecuted marks an approved request as executed.
func (s *Store) MarkExecuted(id, runID string) (*Request, error) {
	var executed *Request
	now := time.Now().UTC()
	err := s.withLockedStore(func(sf *storeFile) error {
		expireLocked(sf, now)
		for _, req := range sf.Requests {
			if req.ID != id {
				continue
			}
			if req.Status != StatusApproved && req.Status != StatusPending {
				return fmt.Errorf("approval %q is %s (cannot mark executed)", req.ID, req.Status)
			}
			req.Status = StatusExecuted
			if strings.TrimSpace(runID) != "" {
				req.RunID = runID
			}
			clone := *req
			executed = &clone
			return nil
		}
		return fmt.Errorf("approval %q not found", id)
	})
	if err != nil {
		return nil, err
	}
	return executed, nil
}

// HashCommand returns a stable hash for command deduplication/audit.
func HashCommand(command string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(command)))
	return hex.EncodeToString(sum[:])
}

func (s *Store) withLockedStore(fn func(sf *storeFile) error) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return err
	}

	lock := flock.New(s.lockPath)
	if err := lock.Lock(); err != nil {
		return fmt.Errorf("locking approvals store: %w", err)
	}
	defer lock.Unlock() //nolint:errcheck // best effort

	sf, err := s.loadUnlocked()
	if err != nil {
		return err
	}
	if err := fn(sf); err != nil {
		return err
	}
	return util.AtomicWriteJSONWithPerm(s.path, sf, 0600)
}

func (s *Store) readLockedStore() (*storeFile, error) {
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return nil, err
	}

	lock := flock.New(s.lockPath)
	if err := lock.Lock(); err != nil {
		return nil, fmt.Errorf("locking approvals store: %w", err)
	}
	defer lock.Unlock() //nolint:errcheck // best effort

	return s.loadUnlocked()
}

func (s *Store) loadUnlocked() (*storeFile, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return &storeFile{Version: 1, Requests: []*Request{}}, nil
		}
		return nil, err
	}

	var sf storeFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return nil, fmt.Errorf("parsing approvals store: %w", err)
	}
	if sf.Version == 0 {
		sf.Version = 1
	}
	if sf.Requests == nil {
		sf.Requests = []*Request{}
	}
	return &sf, nil
}

func expireLocked(sf *storeFile, now time.Time) {
	for _, req := range sf.Requests {
		if req.Status == StatusPending && !req.ExpiresAt.IsZero() && now.After(req.ExpiresAt) {
			req.Status = StatusExpired
		}
	}
}

func defaultValue(value, fallback string) string {
	v := strings.TrimSpace(value)
	if v == "" {
		return fallback
	}
	return v
}

func shortID() string {
	id := strings.ReplaceAll(uuid.NewString(), "-", "")
	if len(id) > 10 {
		return id[:10]
	}
	return id
}

func ptrTime(t time.Time) *time.Time {
	return &t
}
