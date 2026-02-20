package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/approvals"
	"github.com/steveyegge/gastown/internal/policy"
	"github.com/steveyegge/gastown/internal/runlog"
)

type policyEvaluateRequest struct {
	Agent       string   `json:"agent,omitempty"`
	Repo        string   `json:"repo,omitempty"`
	Command     string   `json:"command"`
	Args        []string `json:"args,omitempty"`
	RequestedBy string   `json:"requested_by,omitempty"`
}

type policyEvaluateResponse struct {
	Decision string `json:"decision"`
	Class    string `json:"class"`
	Reason   string `json:"reason,omitempty"`
	RuleID   string `json:"rule_id,omitempty"`
}

type approvalCreateRequest struct {
	RunID       string `json:"run_id,omitempty"`
	Command     string `json:"command"`
	RequestedBy string `json:"requested_by,omitempty"`
	Repo        string `json:"repo,omitempty"`
	Reason      string `json:"reason,omitempty"`
	TTLSeconds  int    `json:"ttl_seconds,omitempty"`
}

type approvalDecisionRequest struct {
	Decision  string `json:"decision"`
	Approver  string `json:"approver,omitempty"`
	Rationale string `json:"rationale,omitempty"`
}

func (h *APIHandler) handlePolicyEvaluate(w http.ResponseWriter, r *http.Request) {
	var req policyEvaluateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendError(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Command) == "" && len(req.Args) == 0 {
		h.sendError(w, "Command is required", http.StatusBadRequest)
		return
	}

	eval := h.policyEvaluator.Evaluate(policy.EvalRequest{
		Agent:       strings.TrimSpace(req.Agent),
		Repo:        policy.NormalizeRepo(defaultValue(req.Repo, h.workDir)),
		Command:     req.Command,
		Args:        req.Args,
		RequestedBy: strings.TrimSpace(req.RequestedBy),
		Timestamp:   time.Now().UTC(),
	})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(policyEvaluateResponse{
		Decision: string(eval.Decision),
		Class:    string(eval.Class),
		Reason:   eval.Reason,
		RuleID:   eval.RuleID,
	})
}

func (h *APIHandler) handleApprovalCreate(w http.ResponseWriter, r *http.Request) {
	if h.approvalStore == nil {
		h.sendError(w, "Approvals not available outside a workspace", http.StatusServiceUnavailable)
		return
	}

	var req approvalCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendError(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	req.Command = strings.TrimSpace(req.Command)
	if req.Command == "" {
		h.sendError(w, "command is required", http.StatusBadRequest)
		return
	}

	eval := h.policyEvaluator.Evaluate(policy.EvalRequest{
		Agent:       "dashboard",
		Repo:        policy.NormalizeRepo(defaultValue(req.Repo, h.workDir)),
		Command:     req.Command,
		RequestedBy: strings.TrimSpace(req.RequestedBy),
		Timestamp:   time.Now().UTC(),
	})

	ttl := 15 * time.Minute
	if req.TTLSeconds > 0 {
		ttl = time.Duration(req.TTLSeconds) * time.Second
	}

	approval, err := h.approvalStore.Create(approvals.CreateInput{
		RunID:          strings.TrimSpace(req.RunID),
		Command:        req.Command,
		Class:          eval.Class,
		RequestedBy:    defaultValue(req.RequestedBy, "dashboard"),
		Repo:           policy.NormalizeRepo(defaultValue(req.Repo, h.workDir)),
		PolicyDecision: eval.Decision,
		Reason:         defaultValue(req.Reason, eval.Reason),
		TTL:            ttl,
	})
	if err != nil {
		h.sendError(w, "Failed to create approval: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if h.runLog != nil {
		_ = h.runLog.Append(runlog.Event{
			RunID:          defaultValue(approval.RunID, runlog.NewRunID()),
			EventType:      "approval_requested",
			State:          "awaiting_approval",
			PolicyDecision: string(eval.Decision),
			Payload: map[string]interface{}{
				"approval_id": approval.ID,
				"command":     approval.Command,
				"class":       approval.Class,
			},
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(approval)
}

func (h *APIHandler) handleApprovalList(w http.ResponseWriter, r *http.Request) {
	if h.approvalStore == nil {
		h.sendError(w, "Approvals not available outside a workspace", http.StatusServiceUnavailable)
		return
	}

	filter := approvals.Status(strings.TrimSpace(r.URL.Query().Get("status")))
	reqs, err := h.approvalStore.List(filter)
	if err != nil {
		h.sendError(w, "Failed to list approvals: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(reqs)
}

func (h *APIHandler) handleApprovalDecision(w http.ResponseWriter, r *http.Request, approvalID string) {
	if h.approvalStore == nil {
		h.sendError(w, "Approvals not available outside a workspace", http.StatusServiceUnavailable)
		return
	}
	if strings.TrimSpace(approvalID) == "" {
		h.sendError(w, "Missing approval ID", http.StatusBadRequest)
		return
	}

	var req approvalDecisionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var status approvals.Status
	switch strings.ToLower(strings.TrimSpace(req.Decision)) {
	case "approve", "approved":
		status = approvals.StatusApproved
	case "deny", "denied":
		status = approvals.StatusDenied
	default:
		h.sendError(w, "decision must be approve or deny", http.StatusBadRequest)
		return
	}

	updated, err := h.approvalStore.Decide(approvals.DecideInput{
		ID:        approvalID,
		Decision:  status,
		Approver:  defaultValue(req.Approver, "dashboard"),
		Rationale: strings.TrimSpace(req.Rationale),
	})
	if err != nil {
		h.sendError(w, "Failed to decide approval: "+err.Error(), http.StatusBadRequest)
		return
	}

	if h.runLog != nil {
		_ = h.runLog.Append(runlog.Event{
			RunID:          defaultValue(updated.RunID, runlog.NewRunID()),
			EventType:      "approval_decided",
			State:          "approval_decided",
			PolicyDecision: string(updated.PolicyDecision),
			Payload: map[string]interface{}{
				"approval_id": updated.ID,
				"status":      updated.Status,
				"approver":    updated.DecidedBy,
				"rationale":   updated.DecisionRationale,
			},
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(updated)
}

func (h *APIHandler) handleRunAudit(w http.ResponseWriter, runID string) {
	if h.runLog == nil {
		h.sendError(w, "Run log not available outside a workspace", http.StatusServiceUnavailable)
		return
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		h.sendError(w, "Missing run ID", http.StatusBadRequest)
		return
	}

	events, err := h.runLog.ReadRun(runID)
	if err != nil {
		h.sendError(w, fmt.Sprintf("Failed to read run audit: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"run_id": runID,
		"events": events,
	})
}

func parseApprovalDecisionPath(path string) (approvalID string, ok bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	// /v1/approvals/{id}/decision
	if len(parts) == 4 && parts[0] == "v1" && parts[1] == "approvals" && parts[3] == "decision" {
		return parts[2], true
	}
	return "", false
}

func parseRunAuditPath(path string) (runID string, ok bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	// /v1/runs/{id}/audit
	if len(parts) == 4 && parts[0] == "v1" && parts[1] == "runs" && parts[3] == "audit" {
		return parts[2], true
	}
	return "", false
}

func defaultValue(value, fallback string) string {
	v := strings.TrimSpace(value)
	if v == "" {
		return fallback
	}
	return v
}
