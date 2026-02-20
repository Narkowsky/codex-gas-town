package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/runlog"
	"github.com/steveyegge/gastown/internal/workspace"
)

var runsCmd = &cobra.Command{
	Use:     "runs",
	GroupID: GroupDiag,
	Short:   "Inspect command run audit trails",
	RunE:    requireSubcommand,
}

var runsReplayCmd = &cobra.Command{
	Use:   "replay",
	Short: "Replay audit events for a run ID",
	RunE:  runRunsReplay,
}

var (
	runsReplayID   string
	runsReplayJSON bool
)

func init() {
	runsReplayCmd.Flags().StringVar(&runsReplayID, "run-id", "", "Run ID to replay")
	runsReplayCmd.Flags().BoolVar(&runsReplayJSON, "json", false, "Output JSON")

	runsCmd.AddCommand(runsReplayCmd)
	rootCmd.AddCommand(runsCmd)
}

func runRunsReplay(cmd *cobra.Command, args []string) error {
	runID := strings.TrimSpace(runsReplayID)
	if runID == "" && len(args) > 0 {
		runID = strings.TrimSpace(args[0])
	}
	if runID == "" {
		return fmt.Errorf("run ID is required via --run-id")
	}

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	store := runlog.NewStore(townRoot)
	events, err := store.ReadRun(runID)
	if err != nil {
		return err
	}
	if runsReplayJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]interface{}{
			"run_id": runID,
			"events": events,
		})
	}

	if len(events) == 0 {
		fmt.Printf("No events found for run %s\n", runID)
		return nil
	}

	fmt.Printf("Run %s\n", runID)
	for _, evt := range events {
		ts := evt.Timestamp.Format("2006-01-02 15:04:05")
		fmt.Printf("  %s  %-18s state=%s decision=%s\n", ts, evt.EventType, evt.State, evt.PolicyDecision)
		if len(evt.Payload) > 0 {
			payload, _ := json.Marshal(evt.Payload)
			fmt.Printf("    payload: %s\n", payload)
		}
	}
	return nil
}
