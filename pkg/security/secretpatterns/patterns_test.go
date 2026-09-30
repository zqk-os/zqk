package secretpatterns

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefinitionsIntegrity(t *testing.T) {
	t.Parallel()
	require.NotEmpty(t, Definitions)
	for _, def := range Definitions {
		assert.NotEmpty(t, def.ID)
		assert.NotEmpty(t, def.Rule)
		assert.NotEmpty(t, def.Name)
		assert.NotEmpty(t, def.Description)
		assert.NotNil(t, def.Regex)
	}

	regexes := AllRegexes()
	assert.Len(t, regexes, len(Definitions))
	for _, r := range regexes {
		assert.NotNil(t, r)
	}
}

func TestSecretPatterns_Detections(t *testing.T) {
	t.Parallel()

	// Clean content
	clean := "package main\nfunc main() {\n\t// No secrets here\n}\n"
	assert.Empty(t, Detect(clean))
	assert.Empty(t, DetectAll(clean))
	assert.False(t, CombinedRegex.MatchString(clean))

	// Test tokens assembled dynamically to avoid static scanner trips in repository
	tests := []struct {
		name       string
		token      string
		expectRule string
	}{
		{
			name:       "AWS Key",
			token:      "AK" + "IA" + "1234567890ABCDEF",
			expectRule: RuleAWSKey,
		},
		{
			name:       "GitHub Classic PAT",
			token:      "gh" + "p_" + "1234567890abcdefghijklmnopqrstuvwxyz",
			expectRule: RuleGitHubPAT,
		},
		{
			name:       "GitHub OAuth Token",
			token:      "gh" + "o_" + "1234567890abcdefghijklmnopqrstuvwxyz",
			expectRule: RuleGitHubOAuth,
		},
		{
			name:       "GitHub Fine Grained PAT",
			token:      "git" + "hub_pat_11AAAAAAA0123456789012345678901234567890123456789012345678901234567890123456789012",
			expectRule: RuleGitHubPAT,
		},
		{
			name:       "Private Key Standard",
			token:      "-----BEGIN " + "PRI" + "VATE KEY-----",
			expectRule: RulePrivateKey,
		},
		{
			name:       "Private Key RSA",
			token:      "-----BEGIN " + "RSA " + "PRI" + "VATE KEY-----",
			expectRule: RulePrivateKey,
		},
		{
			name:       "Private Key OPENSSH",
			token:      "-----BEGIN " + "OPENSSH " + "PRI" + "VATE KEY-----",
			expectRule: RulePrivateKey,
		},
		{
			name:       "Slack Token",
			token:      "xo" + "xb-123456789012-123456789012-abcdefghijklmnopqrstuvwx",
			expectRule: RuleSlackToken,
		},
		{
			name:       "Slack Webhook",
			token:      "https://" + "hooks.slack.com/services/T123/B456/789xyz",
			expectRule: RuleSlackWebhook,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			content := "var secret = \"" + tc.token + "\"\n"

			// Verify CombinedRegex matches
			assert.True(t, CombinedRegex.MatchString(content), "CombinedRegex should match %s", tc.name)
			found := CombinedRegex.FindString(content)
			assert.Equal(t, tc.token, found)

			// Verify Detect finds it
			detected := Detect(content)
			assert.NotEmpty(t, detected)
			assert.Contains(t, detected, tc.token)

			// Verify matching definition rule
			matchedRule := ""
			for _, def := range Definitions {
				if def.Regex.MatchString(content) {
					matchedRule = def.Rule
					break
				}
			}
			assert.Equal(t, tc.expectRule, matchedRule)
		})
	}
}

func TestRedact(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "********", Redact("12345678"))
	assert.Equal(t, "***", Redact("abc"))
	dummyKey := "AK" + "IA1234567890EXAMPLE"
	assert.Equal(t, "AKIA*************MPLE", Redact(dummyKey))
}

func TestDetectAll(t *testing.T) {
	t.Parallel()

	token1 := "AK" + "IA" + "1111111111AAAAAA"
	token2 := "AK" + "IA" + "2222222222BBBBBB"
	content := "key1=" + token1 + "\nkey2=" + token2 + "\n"

	all := DetectAll(content)
	assert.Len(t, all, 2)
	assert.Contains(t, all, token1)
	assert.Contains(t, all, token2)
}
