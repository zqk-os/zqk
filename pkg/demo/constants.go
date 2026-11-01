package demo

import "time"

const (
	// GitHubRepoURL is the canonical open-source repository URL.
	GitHubRepoURL = "https://github.com/zqk-os/zqk"

	// GitHubForkURL is the direct fork URL for community contribution.
	GitHubForkURL = "https://github.com/zqk-os/zqk/fork"

	// DocumentationURL is the canonical documentation portal.
	DocumentationURL = "https://docs.zqk.dev"

	// StepAll runs all demonstration scenes in sequence.
	StepAll = "all"

	// StepAST runs the AST search showcase scene.
	StepAST = "ast"

	// StepVDS runs the Verifiable Delivery Standard showcase scene.
	StepVDS = "vds"

	// StepSwarm runs the multi-agent swarm orchestration scene.
	StepSwarm = "swarm"

	// StepStar runs the community star & fork invitation scene.
	StepStar = "star"

	// SpeedNormal is the standard delay between demo animations.
	SpeedNormal = "normal"

	// SpeedFast is a reduced delay for brisk showcases.
	SpeedFast = "fast"

	// SpeedInstant has zero artificial delays (recommended for CI/testing).
	SpeedInstant = "instant"

	// NormalStepDelay is the pacing delay for normal speed.
	NormalStepDelay = 350 * time.Millisecond

	// FastStepDelay is the pacing delay for fast speed.
	FastStepDelay = 80 * time.Millisecond

	// SceneNameAST is the human-readable title for the AST search scene.
	SceneNameAST = "Sub-15ms In-Process AST Search (zgrep)"

	// SceneNameVDS is the human-readable title for the VDS verification scene.
	SceneNameVDS = "Verifiable Delivery Standard (VDS) & Three-Fold Proof"

	// SceneNameSwarm is the human-readable title for the swarm coordination scene.
	SceneNameSwarm = "Multi-Agent Swarm Orchestration & MMORCH Pipeline"

	// SceneNameStar is the human-readable title for the GitHub star invitation.
	SceneNameStar = "Community Engagement & GitHub Star/Fork Invitation"
)
