package demo

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShowcase_AllScenesInstant(t *testing.T) {
	var buf bytes.Buffer
	in := strings.NewReader("\n")

	opts := DemoOptions{
		Step:           StepAll,
		Speed:          SpeedInstant,
		NonInteractive: true,
		Writer:         &buf,
		Reader:         in,
	}

	sc := NewShowcase(opts)
	report, err := sc.Run()
	require.NoError(t, err)
	require.NotNil(t, report)

	assert.True(t, report.Success)
	assert.Len(t, report.Scenes, 4)

	output := buf.String()
	assert.Contains(t, output, SceneNameAST)
	assert.Contains(t, output, SceneNameVDS)
	assert.Contains(t, output, SceneNameSwarm)
	assert.Contains(t, output, SceneNameStar)
	assert.Contains(t, output, GitHubRepoURL)
	assert.Contains(t, output, GitHubForkURL)
	assert.Contains(t, output, DocumentationURL)
}

func TestShowcase_IndividualScenes(t *testing.T) {
	steps := []string{StepAST, StepVDS, StepSwarm, StepStar}

	for _, step := range steps {
		t.Run(step, func(t *testing.T) {
			var buf bytes.Buffer
			opts := DemoOptions{
				Step:           step,
				Speed:          SpeedInstant,
				NonInteractive: true,
				Writer:         &buf,
			}

			sc := NewShowcase(opts)
			report, err := sc.Run()
			require.NoError(t, err)
			assert.True(t, report.Success)
			assert.Len(t, report.Scenes, 1)
		})
	}
}

func TestShowcase_InteractiveStarPrompt(t *testing.T) {
	var buf bytes.Buffer
	in := strings.NewReader("star\n")

	opts := DemoOptions{
		Step:           StepStar,
		Speed:          SpeedInstant,
		NonInteractive: false,
		Writer:         &buf,
		Reader:         in,
	}

	sc := NewShowcase(opts)
	report, err := sc.Run()
	require.NoError(t, err)
	assert.True(t, report.Success)
	assert.True(t, report.Starred)
	assert.Contains(t, buf.String(), "Thank you for starring ZQK")
}
