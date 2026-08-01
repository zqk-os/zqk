package bldr_lifecycle_v1

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/lifecycle_builders"
)

// QuestionLifecycleBuilder builds the question lifecycle at version v1_0_0
// File: bldr_lifecycle_v1/question_builder.go - version is encoded in package/directory name
type QuestionLifecycleBuilder struct {
	*lifecycle_builders.BaseLifecycleBuilder
}

// NewQuestionLifecycleBuilder creates a new builder for question lifecycle version v1_0_0
func NewQuestionLifecycleBuilder() *QuestionLifecycleBuilder {
	builder := &QuestionLifecycleBuilder{
		BaseLifecycleBuilder: lifecycle_builders.NewBaseLifecycleBuilder("question", "v1_0_0"),
	}

	// Configure the lifecycle
	builder.
		SetPercentComplete(objects.PercentCompleteConfig{
			Method: "status_based",
			DefaultByStatus: map[string]any{
				"answered": 50,
				"deferred": 0,
				"error":    0,
				"open":     0,
				"resolved": 100,
			},
		})

	// Add statuses and transitions
	builder.addQuestionLifecycleData()

	return builder
}

// addQuestionLifecycleData adds the question lifecycle statuses and transitions
func (b *QuestionLifecycleBuilder) addQuestionLifecycleData() {

	b.AddStatus(objects.Status{
		Value:   "open",
		Display: "Open",
		Origin:  true,
	})
	b.AddStatus(objects.Status{
		Value:   "answered",
		Display: "Answered",
	})
	b.AddStatus(objects.Status{
		Value:    "resolved",
		Display:  "Resolved",
		Terminal: true,
	})
	b.AddStatus(objects.Status{
		Value:    "deferred",
		Display:  "Deferred",
		Terminal: true,
	})
	b.AddStatus(objects.Status{
		Value:   "error",
		Display: "Error",
		System:  true,
	})

	b.AddTransition(objects.Transition{
		From:        "open",
		To:          "answered",
		Description: "Answer provided to the question",
		Manual:      true,
		Auto:        false,
		Preconditions: []string{
			"answer or answer_ref is set",
		},
	})

	b.AddTransition(objects.Transition{
		From:        "answered",
		To:          "resolved",
		Description: "Answer accepted and question closed",
		Manual:      true,
		Auto:        false,
		Preconditions: []string{
			"answer or answer_ref is set",
		},
	})

	b.AddTransition(objects.Transition{
		From:        "open",
		To:          "resolved",
		Description: "Question resolved directly (answer provided and accepted in one step)",
		Manual:      true,
		Auto:        false,
		Preconditions: []string{
			"answer or answer_ref is set",
		},
	})

	b.AddTransition(objects.Transition{
		From:        "open",
		To:          "deferred",
		Description: "Question postponed for later consideration",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "answered",
		To:          "open",
		Description: "Answer rejected or insufficient, question reopened",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "answered",
		To:          "deferred",
		Description: "Answer provided but question deferred for later resolution",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "deferred",
		To:          "open",
		Description: "Deferred question reopened",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "*",
		To:          "error",
		Description: "System-assigned when lifecycle incoherency detected",
		Manual:      false,
		Auto:        false,
	})
}

func init() {
	lifecycle_builders.RegisterBuilder(NewQuestionLifecycleBuilder())
}
