package secretpatterns

import (
	"regexp"
	"strings"
)

// Rule identifier constants matching security audit policies.
const (
	RuleAWSKey       = "AWS_KEY_LEAK"
	RuleGitHubPAT    = "GITHUB_PAT_LEAK"
	RuleGitHubOAuth  = "GITHUB_OAUTH_LEAK"
	RulePrivateKey   = "PRIVATE_KEY_LEAK"
	RuleSlackToken   = "SLACK_TOKEN_LEAK"
	RuleSlackWebhook = "SLACK_WEBHOOK_LEAK"
)

// Definition describes a canonical secret pattern detector.
type Definition struct {
	ID          string         `json:"id"`
	Rule        string         `json:"rule"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Regex       *regexp.Regexp `json:"-"`
}

// Canonical individual regular expressions for known secret patterns.
var (
	AWSKeyRegex               = regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)
	GitHubPATRegex            = regexp.MustCompile(`\bghp_[0-9a-zA-Z]{36}\b`)
	GitHubOAuthRegex          = regexp.MustCompile(`\bgho_[0-9a-zA-Z]{36}\b`)
	GitHubFineGrainedPATRegex = regexp.MustCompile(`\bgithub_pat_[a-zA-Z0-9_]{82}\b`)
	PrivateKeyRegex           = regexp.MustCompile(`-----BEGIN (?:[A-Z0-9_-]+ )*PRIVATE KEY-----`)
	SlackTokenRegex           = regexp.MustCompile(`\bxox[baprs]-[0-9]{12}-[0-9]{12}-[a-zA-Z0-9]{24}\b`)
	SlackWebhookRegex         = regexp.MustCompile(slackWebhookPattern())
)

func slackWebhookPattern() string {
	return strings.Join([]string{
		`\bhttps://`,
		`hooks\.slack\.com/services/`,
		`T[0-9a-zA-Z]+/B[0-9a-zA-Z]+/[0-9a-zA-Z]+\b`,
	}, "")
}

// Definitions holds the ordered list of canonical secret pattern definitions.
var Definitions = []Definition{
	{
		ID:          "aws-key",
		Rule:        RuleAWSKey,
		Name:        "AWS Access Key",
		Description: "detected AWS access key identifier",
		Regex:       AWSKeyRegex,
	},
	{
		ID:          "github-pat",
		Rule:        RuleGitHubPAT,
		Name:        "GitHub Personal Access Token",
		Description: "detected GitHub Personal Access Token",
		Regex:       GitHubPATRegex,
	},
	{
		ID:          "github-oauth",
		Rule:        RuleGitHubOAuth,
		Name:        "GitHub OAuth Access Token",
		Description: "detected GitHub OAuth Access Token",
		Regex:       GitHubOAuthRegex,
	},
	{
		ID:          "github-fine-grained-pat",
		Rule:        RuleGitHubPAT,
		Name:        "GitHub Fine-Grained Personal Access Token",
		Description: "detected GitHub Fine-Grained Personal Access Token",
		Regex:       GitHubFineGrainedPATRegex,
	},
	{
		ID:          "private-key",
		Rule:        RulePrivateKey,
		Name:        "Private Key Block",
		Description: "detected private key block",
		Regex:       PrivateKeyRegex,
	},
	{
		ID:          "slack-token",
		Rule:        RuleSlackToken,
		Name:        "Slack Token",
		Description: "detected Slack API token",
		Regex:       SlackTokenRegex,
	},
	{
		ID:          "slack-webhook",
		Rule:        RuleSlackWebhook,
		Name:        "Slack Webhook URL",
		Description: "detected Slack incoming webhook URL",
		Regex:       SlackWebhookRegex,
	},
}

// CombinedRegex is a single unified regular expression matching any of the canonical secret patterns.
// Designed for high-throughput single-pass line scanning in policy gates and file walkers.
var CombinedRegex = regexp.MustCompile(`(` +
	`\bghp_[0-9a-zA-Z]{36}\b|` +
	`\bgho_[0-9a-zA-Z]{36}\b|` +
	`\bgithub_pat_[a-zA-Z0-9_]{82}\b|` +
	`\bAKIA[0-9A-Z]{16}\b|` +
	`-----BEGIN (?:[A-Z0-9_-]+ )*PRIVATE KEY-----|` +
	`\bxox[baprs]-[0-9]{12}-[0-9]{12}-[a-zA-Z0-9]{24}\b|` +
	slackWebhookPattern() +
	`)`)

// AllRegexes returns an ordered slice of all individual compiled regular expressions.
func AllRegexes() []*regexp.Regexp {
	regexes := make([]*regexp.Regexp, len(Definitions))
	for i, d := range Definitions {
		regexes[i] = d.Regex
	}
	return regexes
}

// Detect scans content against all canonical secret patterns and returns the first match for each pattern found.
func Detect(content string) []string {
	var detected []string
	for _, d := range Definitions {
		if loc := d.Regex.FindString(content); loc != "" {
			detected = append(detected, loc)
		}
	}
	return detected
}

// DetectAll scans content and returns all occurrences of any canonical secret pattern.
func DetectAll(content string) []string {
	var detected []string
	for _, d := range Definitions {
		matches := d.Regex.FindAllString(content, -1)
		detected = append(detected, matches...)
	}
	return detected
}

// Redact masks sensitive credentials, leaving only boundary characters visible for diagnostic identification.
func Redact(secret string) string {
	if len(secret) <= 8 {
		return strings.Repeat("*", len(secret))
	}
	return secret[:4] + strings.Repeat("*", len(secret)-8) + secret[len(secret)-4:]
}
