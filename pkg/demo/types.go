package demo

import "io"

// DemoOptions contains configuration for the interactive terminal showcase.
type DemoOptions struct {
	Step           string
	Speed          string
	NonInteractive bool
	Writer         io.Writer
	Reader         io.Reader
}

// DemoSceneResult records the execution result of a demo scene.
type DemoSceneResult struct {
	Name    string `json:"name"`
	Success bool   `json:"success"`
	Summary string `json:"summary"`
}

// DemoReport summarizes all executed showcase scenes.
type DemoReport struct {
	Scenes  []DemoSceneResult `json:"scenes"`
	Success bool              `json:"success"`
	Starred bool              `json:"starred"`
}
