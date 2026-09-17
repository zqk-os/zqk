package interactive

import "github.com/lanceman/zqk/pkg/errfmt"

const (
	errGenerateTemplateFmt = "failed to generate template: %w"
	errReplaceTokensFmt    = "failed to replace tokens: %w" //nolint:gosec
)

// TemplateLoop manages the validation loop for template-based object creation
// This is the core loop logic that processes templates, validates fields, and determines
// what fields need to be elicited from the client
type TemplateLoop struct {
	generator *TemplateGenerator
}

// NewTemplateLoop creates a new streaming template loop manager
func NewTemplateLoop(generator *TemplateGenerator) *TemplateLoop {
	return &TemplateLoop{
		generator: generator,
	}
}

// LoopState represents the state of the validation loop
type LoopState struct {
	Kind            string              // Object kind
	Template        *TemplateWithTokens // Template with tokens
	ProvidedValues  map[string]any      // Values provided by client
	MissingRequired []string            // Required fields that are missing
	InvalidFields   map[string]string   // Field name -> error message
	IsComplete      bool                // Whether all required fields are provided and valid
	FilledTemplate  string              // Template with tokens replaced (if complete)
}

// ProcessLoop processes one iteration of the validation loop
// Returns the loop state and whether the loop should continue
func (stl *TemplateLoop) ProcessLoop(kind string, providedValues map[string]any) (*LoopState, error) {
	// Generate template if not already generated (first iteration)
	// In a stateful implementation, we'd cache this, but for stateless we generate each time
	templateWithTokens, err := stl.generator.GenerateTemplateWithTokens(kind)
	if err != nil {
		return nil, errfmt.Errorf(errGenerateTemplateFmt, err)
	}

	// Validate field completeness
	missingRequired, invalidFields := ValidateFieldCompleteness(templateWithTokens, providedValues)

	// Check if loop is complete (all required fields provided and valid)
	isComplete := len(missingRequired) == 0 && len(invalidFields) == 0

	var filledTemplate string
	if isComplete {
		// Replace tokens with provided values
		filledTemplate, err = ReplaceTokensInTemplate(templateWithTokens.Template, providedValues)
		if err != nil {
			return nil, errfmt.Errorf(errReplaceTokensFmt, err)
		}
	}

	return &LoopState{
		Kind:            kind,
		Template:        templateWithTokens,
		ProvidedValues:  providedValues,
		MissingRequired: missingRequired,
		InvalidFields:   invalidFields,
		IsComplete:      isComplete,
		FilledTemplate:  filledTemplate,
	}, nil
}

// GetFieldsToElicit returns the fields that need to be elicited from the client
// This is used to generate elicitation parameters for MCP
func (stl *TemplateLoop) GetFieldsToElicit(loopState *LoopState) []*FieldTokenInfo {
	fieldsToElicit := []*FieldTokenInfo{}

	// Add missing required fields
	for _, fieldName := range loopState.MissingRequired {
		if tokenInfo, exists := loopState.Template.FieldInfo[fieldName]; exists {
			fieldsToElicit = append(fieldsToElicit, tokenInfo)
		}
	}

	// Add invalid fields (so client can fix them)
	for fieldName := range loopState.InvalidFields {
		if tokenInfo, exists := loopState.Template.FieldInfo[fieldName]; exists {
			// Check if not already added
			alreadyAdded := false
			for _, added := range fieldsToElicit {
				if added.Name == fieldName {
					alreadyAdded = true
					break
				}
			}
			if !alreadyAdded {
				fieldsToElicit = append(fieldsToElicit, tokenInfo)
			}
		}
	}

	return fieldsToElicit
}
