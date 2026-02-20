package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/policy"
	"github.com/steveyegge/gastown/internal/workspace"
)

var policyCmd = &cobra.Command{
	Use:     "policy",
	GroupID: GroupServices,
	Short:   "Evaluate command policy decisions",
	Long: `Evaluate policy decisions for proposed commands.

Examples:
  gt policy eval --cmd "git push origin main"
  gt policy eval --agent witness --repo /path/to/repo --cmd "gt rig boot myrig" --json`,
	RunE: requireSubcommand,
}

var policyEvalCmd = &cobra.Command{
	Use:   "eval",
	Short: "Evaluate policy for a command",
	RunE:  runPolicyEval,
}

var (
	policyEvalAgent string
	policyEvalRepo  string
	policyEvalCmdIn string
	policyEvalJSON  bool
)

func init() {
	policyEvalCmd.Flags().StringVar(&policyEvalAgent, "agent", "dashboard", "Agent identity to evaluate for")
	policyEvalCmd.Flags().StringVar(&policyEvalRepo, "repo", "", "Repository/workspace path")
	policyEvalCmd.Flags().StringVar(&policyEvalCmdIn, "cmd", "", "Command to evaluate")
	policyEvalCmd.Flags().BoolVar(&policyEvalJSON, "json", false, "Output JSON")

	policyCmd.AddCommand(policyEvalCmd)
	rootCmd.AddCommand(policyCmd)
}

func runPolicyEval(cmd *cobra.Command, args []string) error {
	command := strings.TrimSpace(policyEvalCmdIn)
	if command == "" && len(args) > 0 {
		command = strings.Join(args, " ")
	}
	if command == "" {
		return fmt.Errorf("command is required via --cmd")
	}

	repo := strings.TrimSpace(policyEvalRepo)
	townRoot, _ := workspace.FindFromCwd()
	if repo == "" {
		repo, _ = os.Getwd()
	}
	repo = policy.NormalizeRepo(repo)

	evaluator := policy.NewDefaultEvaluator()
	if townRoot != "" {
		evaluator = policy.LoadOrDefault(townRoot)
	}

	result := evaluator.Evaluate(policy.EvalRequest{
		Agent:       strings.TrimSpace(policyEvalAgent),
		Repo:        repo,
		Command:     command,
		RequestedBy: "cli",
		Timestamp:   time.Now().UTC(),
	})

	if policyEvalJSON {
		return json.NewEncoder(os.Stdout).Encode(result)
	}

	fmt.Printf("Decision: %s\n", result.Decision)
	fmt.Printf("Class: %s\n", result.Class)
	if result.Reason != "" {
		fmt.Printf("Reason: %s\n", result.Reason)
	}
	if result.RuleID != "" {
		fmt.Printf("Rule: %s\n", result.RuleID)
	}
	return nil
}
