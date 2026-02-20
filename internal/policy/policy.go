// Package policy provides policy-as-code evaluation for command execution.
package policy

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Decision is the policy decision returned by the evaluator.
type Decision string

const (
	DecisionAllow                  Decision = "allow"
	DecisionAllowWithJustification Decision = "allow_with_justification"
	DecisionRequireApproval        Decision = "require_approval"
	DecisionDeny                   Decision = "deny"
)

// RiskClass is the command risk class.
type RiskClass string

const (
	Class0Safe            RiskClass = "class0_safe"
	Class1ControlledWrite RiskClass = "class1_controlled_write"
	Class2Sensitive       RiskClass = "class2_sensitive"
	Class3Critical        RiskClass = "class3_critical"
)

// EvalRequest describes a command to evaluate.
type EvalRequest struct {
	Agent       string    `json:"agent,omitempty"`
	Repo        string    `json:"repo,omitempty"`
	Command     string    `json:"command"`
	Args        []string  `json:"args,omitempty"`
	RequestedBy string    `json:"requested_by,omitempty"`
	Timestamp   time.Time `json:"timestamp,omitempty"`
}

// EvalResult describes the policy decision for a command.
type EvalResult struct {
	Decision Decision  `json:"decision"`
	Class    RiskClass `json:"class"`
	Reason   string    `json:"reason,omitempty"`
	RuleID   string    `json:"rule_id,omitempty"`
}

// Document is a policy document loaded from mayor/policy.json.
type Document struct {
	Version         int      `json:"version"`
	DefaultDecision Decision `json:"default_decision,omitempty"`
	Rules           []Rule   `json:"rules,omitempty"`
}

// Rule defines a conditional override.
type Rule struct {
	ID       string     `json:"id"`
	Decision Decision   `json:"decision"`
	Reason   string     `json:"reason,omitempty"`
	Match    RuleMatch  `json:"match"`
	Enabled  *bool      `json:"enabled,omitempty"`
	Until    *time.Time `json:"until,omitempty"`
}

// RuleMatch defines command metadata predicates.
type RuleMatch struct {
	Agents          []string    `json:"agents,omitempty"`
	Repos           []string    `json:"repos,omitempty"`
	CommandPrefixes []string    `json:"command_prefixes,omitempty"`
	CommandRegex    string      `json:"command_regex,omitempty"`
	Classes         []RiskClass `json:"classes,omitempty"`
}

// Evaluator evaluates commands against default classification + document rules.
type Evaluator struct {
	doc *Document
}

// NewDefaultEvaluator creates an evaluator with default rule behavior.
func NewDefaultEvaluator() *Evaluator {
	return &Evaluator{doc: &Document{Version: 1}}
}

// NewEvaluator creates an evaluator from an explicit document.
func NewEvaluator(doc *Document) *Evaluator {
	if doc == nil {
		doc = &Document{Version: 1}
	}
	return &Evaluator{doc: doc}
}

// DefaultPolicyPath returns the default policy document path for a town.
func DefaultPolicyPath(townRoot string) string {
	return filepath.Join(townRoot, "mayor", "policy.json")
}

// LoadDocument loads a policy document from disk.
func LoadDocument(path string) (*Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing policy document: %w", err)
	}
	if doc.Version == 0 {
		doc.Version = 1
	}
	return &doc, nil
}

// LoadOrDefault returns a default evaluator when policy file is missing.
func LoadOrDefault(townRoot string) *Evaluator {
	path := DefaultPolicyPath(townRoot)
	doc, err := LoadDocument(path)
	if err != nil {
		return NewDefaultEvaluator()
	}
	return NewEvaluator(doc)
}

// Evaluate applies risk classification and policy rules to a command.
func (e *Evaluator) Evaluate(req EvalRequest) EvalResult {
	class, classReason := ClassifyCommand(req.Command, req.Args)
	baseDecision := defaultDecisionForClass(class)
	result := EvalResult{
		Decision: baseDecision,
		Class:    class,
		Reason:   classReason,
	}

	if e == nil || e.doc == nil || len(e.doc.Rules) == 0 {
		return result
	}

	now := req.Timestamp
	if now.IsZero() {
		now = time.Now().UTC()
	}

	for _, rule := range e.doc.Rules {
		if !ruleEnabled(rule, now) {
			continue
		}
		if !ruleMatches(rule.Match, req, class) {
			continue
		}
		result.Decision = rule.Decision
		result.RuleID = rule.ID
		if rule.Reason != "" {
			result.Reason = rule.Reason
		}
		return result
	}

	if e.doc.DefaultDecision != "" {
		result.Decision = e.doc.DefaultDecision
	}
	return result
}

func defaultDecisionForClass(class RiskClass) Decision {
	switch class {
	case Class0Safe:
		return DecisionAllow
	case Class1ControlledWrite:
		return DecisionAllowWithJustification
	case Class2Sensitive:
		return DecisionRequireApproval
	case Class3Critical:
		return DecisionDeny
	default:
		return DecisionAllowWithJustification
	}
}

func ruleEnabled(rule Rule, now time.Time) bool {
	if rule.Enabled != nil && !*rule.Enabled {
		return false
	}
	if rule.Until != nil && now.After(*rule.Until) {
		return false
	}
	return true
}

func ruleMatches(match RuleMatch, req EvalRequest, class RiskClass) bool {
	if len(match.Classes) > 0 && !containsClass(match.Classes, class) {
		return false
	}

	if len(match.Agents) > 0 && !matchesPatternList(match.Agents, req.Agent) {
		return false
	}
	if len(match.Repos) > 0 && !matchesPatternList(match.Repos, req.Repo) {
		return false
	}

	command := normalizeCommand(req.Command, req.Args)
	if len(match.CommandPrefixes) > 0 {
		ok := false
		for _, prefix := range match.CommandPrefixes {
			p := strings.ToLower(strings.TrimSpace(prefix))
			if p != "" && strings.HasPrefix(command, p) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}

	if match.CommandRegex != "" {
		re, err := regexp.Compile(match.CommandRegex)
		if err != nil || !re.MatchString(command) {
			return false
		}
	}

	return true
}

func containsClass(classes []RiskClass, target RiskClass) bool {
	for _, c := range classes {
		if c == target {
			return true
		}
	}
	return false
}

func matchesPatternList(patterns []string, value string) bool {
	v := strings.ToLower(strings.TrimSpace(value))
	if v == "" {
		return false
	}
	for _, raw := range patterns {
		p := strings.ToLower(strings.TrimSpace(raw))
		if p == "" {
			continue
		}
		if p == "*" || p == v {
			return true
		}
		if strings.HasPrefix(p, "*.") && strings.HasSuffix(v, p[1:]) {
			return true
		}
		if strings.HasSuffix(p, "*") && strings.HasPrefix(v, strings.TrimSuffix(p, "*")) {
			return true
		}
	}
	return false
}

// ClassifyCommand classifies a command into one of the risk classes.
// Defaults to Class1 (allow-by-default with high-risk blocks).
func ClassifyCommand(command string, args []string) (RiskClass, string) {
	normalized := normalizeCommand(command, args)
	if normalized == "" {
		return Class3Critical, "empty command is denied"
	}
	unprefixed := strings.TrimSpace(strings.TrimPrefix(normalized, "gt "))

	if matchesPrefix(normalized, class3Prefixes) || matchesPrefix(unprefixed, class3Prefixes) ||
		matchesRegex(normalized, class3Patterns) || matchesRegex(unprefixed, class3Patterns) {
		return Class3Critical, "critical/destructive command pattern"
	}
	if matchesPrefix(normalized, class2Prefixes) || matchesPrefix(unprefixed, class2Prefixes) ||
		matchesRegex(normalized, class2Patterns) || matchesRegex(unprefixed, class2Patterns) {
		return Class2Sensitive, "sensitive command requires approval"
	}
	if matchesPrefix(normalized, class0Prefixes) || matchesPrefix(unprefixed, class0Prefixes) {
		return Class0Safe, "read-only command"
	}
	if matchesPrefix(normalized, class1Prefixes) || matchesPrefix(unprefixed, class1Prefixes) {
		return Class1ControlledWrite, "controlled repo-local write operation"
	}
	return Class1ControlledWrite, "default controlled-write policy (allow with audit)"
}

func normalizeCommand(command string, args []string) string {
	c := strings.TrimSpace(command)
	if c != "" {
		return strings.ToLower(c)
	}
	if len(args) == 0 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(strings.Join(args, " ")))
}

func matchesPrefix(command string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(command, prefix) {
			return true
		}
	}
	return false
}

func matchesRegex(command string, patterns []*regexp.Regexp) bool {
	for _, pattern := range patterns {
		if pattern.MatchString(command) {
			return true
		}
	}
	return false
}

var class0Prefixes = []string{
	"activity",
	"audit",
	"cat ",
	"date",
	"doctor",
	"echo ",
	"find ",
	"git branch",
	"git diff",
	"git log",
	"git show",
	"git status",
	"gt activity",
	"gt audit",
	"gt doctor",
	"gt info",
	"gt log",
	"gt status",
	"head ",
	"info",
	"log",
	"ls",
	"ps ",
	"pwd",
	"rg ",
	"status",
	"tail ",
	"wc ",
	"whoami",
}

var class1Prefixes = []string{
	"git add",
	"git checkout ",
	"git commit",
	"git restore ",
	"hook ",
	"mail ",
	"notify ",
	"gt hook ",
	"gt mail ",
	"gt notify ",
	"gt rig ",
	"gt sling",
	"gt unsling",
	"make ",
	"npm test",
	"node --test",
	"rig ",
	"sling",
	"unsling",
}

var class2Prefixes = []string{
	"apt ",
	"brew ",
	"curl ",
	"deacon start",
	"docker push",
	"git fetch",
	"git pull",
	"git push",
	"go get ",
	"gt deacon start",
	"gt refinery start",
	"gt rig boot",
	"gt rig start",
	"gt witness start",
	"kubectl apply",
	"launchctl ",
	"npm install",
	"pip install",
	"pnpm install",
	"refinery start",
	"rig boot",
	"rig start",
	"service ",
	"systemctl ",
	"wget ",
	"witness start",
	"yarn add ",
}

var class3Prefixes = []string{
	"chown ",
	"dd ",
	"git clean -fd",
	"git reset --hard",
	"mkfs ",
	"reboot",
	"rm ",
	"shutdown",
	"sudo ",
}

var class2Patterns = []*regexp.Regexp{
	regexp.MustCompile(`\b(--token|--password|--secret)\b`),
	regexp.MustCompile(`\b(npm|pnpm|yarn)\s+add\b`),
}

var class3Patterns = []*regexp.Regexp{
	regexp.MustCompile(`\brm\s+-rf\b`),
	regexp.MustCompile(`\bchmod\s+777\b`),
	regexp.MustCompile(`\b(?:cat|less|more)\s+~?/.+/(?:\.ssh|\.aws|\.gnupg)/`),
	regexp.MustCompile(`\bexport\s+[^=\s]*(?:token|secret|password|key)[^=\s]*=`),
}

// NormalizeRepo coerces file paths and URLs into a comparable repo value.
func NormalizeRepo(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if u, err := url.Parse(s); err == nil && u.Host != "" {
		return strings.ToLower(strings.TrimSuffix(u.Host+u.Path, "/"))
	}
	return strings.ToLower(filepath.Clean(s))
}
