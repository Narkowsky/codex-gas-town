package policy

import "testing"

func TestClassifyCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		command string
		want    RiskClass
	}{
		{name: "safe status", command: "gt status --json", want: Class0Safe},
		{name: "controlled write", command: "git commit -m test", want: Class1ControlledWrite},
		{name: "sensitive network", command: "git push origin main", want: Class2Sensitive},
		{name: "sensitive unprefixed gt command", command: "rig boot testrig", want: Class2Sensitive},
		{name: "critical destructive", command: "rm -rf /tmp/foo", want: Class3Critical},
		{name: "default allow by default", command: "go test ./...", want: Class1ControlledWrite},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, _ := ClassifyCommand(tt.command, nil)
			if got != tt.want {
				t.Fatalf("ClassifyCommand(%q) = %s, want %s", tt.command, got, tt.want)
			}
		})
	}
}

func TestEvaluateRuleOverride(t *testing.T) {
	t.Parallel()

	enabled := true
	e := NewEvaluator(&Document{
		Version: 1,
		Rules: []Rule{
			{
				ID:       "allow-git-push-for-mayor",
				Decision: DecisionAllowWithJustification,
				Enabled:  &enabled,
				Match: RuleMatch{
					Agents:          []string{"mayor"},
					CommandPrefixes: []string{"git push"},
				},
			},
		},
	})

	got := e.Evaluate(EvalRequest{
		Agent:   "mayor",
		Command: "git push origin main",
	})

	if got.Decision != DecisionAllowWithJustification {
		t.Fatalf("decision = %s, want %s", got.Decision, DecisionAllowWithJustification)
	}
	if got.RuleID != "allow-git-push-for-mayor" {
		t.Fatalf("rule id = %q, want override rule", got.RuleID)
	}
}
