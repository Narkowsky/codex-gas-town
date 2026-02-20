package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/approvals"
	"github.com/steveyegge/gastown/internal/workspace"
)

var approvalsCmd = &cobra.Command{
	Use:     "approvals",
	GroupID: GroupServices,
	Short:   "Manage approval requests for policy-gated commands",
	Long: `Manage approval requests for commands that require human approval.

Examples:
  gt approvals list
  gt approvals show apr-123
  gt approvals approve apr-123 --reason "approved in change window"
  gt approvals deny apr-123 --reason "outside policy scope"`,
	RunE: requireSubcommand,
}

var approvalsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List approval requests",
	RunE:  runApprovalsList,
}

var approvalsShowCmd = &cobra.Command{
	Use:   "show <approval-id>",
	Short: "Show a single approval request",
	Args:  cobra.ExactArgs(1),
	RunE:  runApprovalsShow,
}

var approvalsApproveCmd = &cobra.Command{
	Use:   "approve <approval-id>",
	Short: "Approve a pending request",
	Args:  cobra.ExactArgs(1),
	RunE:  runApprovalsApprove,
}

var approvalsDenyCmd = &cobra.Command{
	Use:   "deny <approval-id>",
	Short: "Deny a pending request",
	Args:  cobra.ExactArgs(1),
	RunE:  runApprovalsDeny,
}

var (
	approvalsStatus   string
	approvalsJSON     bool
	approvalsApprover string
	approvalsReason   string
)

func init() {
	approvalsListCmd.Flags().StringVar(&approvalsStatus, "status", "", "Filter by status (pending|approved|denied|expired|executed)")
	approvalsListCmd.Flags().BoolVar(&approvalsJSON, "json", false, "Output JSON")
	approvalsShowCmd.Flags().BoolVar(&approvalsJSON, "json", false, "Output JSON")

	defaultApprover := strings.TrimSpace(os.Getenv("USER"))
	if defaultApprover == "" {
		defaultApprover = "operator"
	}
	approvalsApproveCmd.Flags().StringVar(&approvalsApprover, "by", defaultApprover, "Approver identity")
	approvalsApproveCmd.Flags().StringVar(&approvalsReason, "reason", "", "Approval rationale")
	approvalsApproveCmd.Flags().BoolVar(&approvalsJSON, "json", false, "Output JSON")
	approvalsDenyCmd.Flags().StringVar(&approvalsApprover, "by", defaultApprover, "Approver identity")
	approvalsDenyCmd.Flags().StringVar(&approvalsReason, "reason", "", "Denial rationale")
	approvalsDenyCmd.Flags().BoolVar(&approvalsJSON, "json", false, "Output JSON")

	approvalsCmd.AddCommand(approvalsListCmd)
	approvalsCmd.AddCommand(approvalsShowCmd)
	approvalsCmd.AddCommand(approvalsApproveCmd)
	approvalsCmd.AddCommand(approvalsDenyCmd)
	rootCmd.AddCommand(approvalsCmd)
}

func runApprovalsList(cmd *cobra.Command, args []string) error {
	store, err := approvalsStoreFromCwd()
	if err != nil {
		return err
	}

	filter := approvals.Status(strings.TrimSpace(approvalsStatus))
	reqs, err := store.List(filter)
	if err != nil {
		return err
	}

	if approvalsJSON {
		return json.NewEncoder(os.Stdout).Encode(reqs)
	}
	if len(reqs) == 0 {
		fmt.Println("No approval requests found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tSTATUS\tCLASS\tREQUESTED BY\tCREATED\tCOMMAND")
	for _, req := range reqs {
		created := req.CreatedAt.Format("2006-01-02 15:04:05")
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			req.ID, req.Status, req.Class, req.RequestedBy, created, req.Command)
	}
	return w.Flush()
}

func runApprovalsShow(cmd *cobra.Command, args []string) error {
	store, err := approvalsStoreFromCwd()
	if err != nil {
		return err
	}
	req, err := store.Get(args[0])
	if err != nil {
		return err
	}
	if approvalsJSON {
		return json.NewEncoder(os.Stdout).Encode(req)
	}

	fmt.Printf("ID: %s\n", req.ID)
	fmt.Printf("Status: %s\n", req.Status)
	fmt.Printf("Class: %s\n", req.Class)
	fmt.Printf("Command: %s\n", req.Command)
	fmt.Printf("Requested by: %s\n", req.RequestedBy)
	if req.DecidedBy != "" {
		fmt.Printf("Decided by: %s\n", req.DecidedBy)
	}
	if req.DecisionRationale != "" {
		fmt.Printf("Rationale: %s\n", req.DecisionRationale)
	}
	fmt.Printf("Created: %s\n", req.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Expires: %s\n", req.ExpiresAt.Format("2006-01-02 15:04:05"))
	return nil
}

func runApprovalsApprove(cmd *cobra.Command, args []string) error {
	store, err := approvalsStoreFromCwd()
	if err != nil {
		return err
	}
	updated, err := store.Decide(approvals.DecideInput{
		ID:        args[0],
		Decision:  approvals.StatusApproved,
		Approver:  approvalsApprover,
		Rationale: approvalsReason,
	})
	if err != nil {
		return err
	}
	if approvalsJSON {
		return json.NewEncoder(os.Stdout).Encode(updated)
	}
	fmt.Printf("Approved %s\n", updated.ID)
	return nil
}

func runApprovalsDeny(cmd *cobra.Command, args []string) error {
	store, err := approvalsStoreFromCwd()
	if err != nil {
		return err
	}
	updated, err := store.Decide(approvals.DecideInput{
		ID:        args[0],
		Decision:  approvals.StatusDenied,
		Approver:  approvalsApprover,
		Rationale: approvalsReason,
	})
	if err != nil {
		return err
	}
	if approvalsJSON {
		return json.NewEncoder(os.Stdout).Encode(updated)
	}
	fmt.Printf("Denied %s\n", updated.ID)
	return nil
}

func approvalsStoreFromCwd() (*approvals.Store, error) {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return nil, fmt.Errorf("not in a Gas Town workspace: %w", err)
	}
	return approvals.NewStore(townRoot), nil
}
