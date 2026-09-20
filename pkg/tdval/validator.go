package tdval

import (
	"fmt"
	"time"
)

// ValidationError describes a validation failure on an envelope field.
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Code    int    `json:"code"`
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("validation error on %q: %s (code %d)", e.Field, e.Message, e.Code)
}

// Envelope encapsulates the standard fields of a traceable data envelope.
type Envelope struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Category  string    `json:"category"`
	Status    string    `json:"status"`
	Priority  int       `json:"priority"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	CreatedBy string    `json:"created_by"`
	UpdatedBy string    `json:"updated_by"`
	Title     string    `json:"title"`
	ParentID  string    `json:"parent_id"`
	SkillSeal string    `json:"skill_seal,omitempty"`
}

// TDEResult holds the outcome of envelope validation.
type TDEResult struct {
	TotalValidated int               `json:"total_validated"`
	Errors         []ValidationError `json:"errors,omitempty"`
	Warnings       []string          `json:"warnings,omitempty"`
}

// Valid returns true if there are no validation errors.
func (r *TDEResult) Valid() bool {
	return len(r.Errors) == 0
}

// AddError records a validation error.
func (r *TDEResult) AddError(field, message string, code int) {
	r.Errors = append(r.Errors, ValidationError{
		Field:   field,
		Message: message,
		Code:    code,
	})
}

// AddWarning records a non-fatal warning.
func (r *TDEResult) AddWarning(warning string) {
	r.Warnings = append(r.Warnings, warning)
}

// TDEValidator validates Envelope objects against schema and integrity rules.
type TDEValidator struct {
	enforceSeal bool
}

// NewTDEValidator returns a new validator instance with default settings.
func NewTDEValidator() *TDEValidator {
	return &TDEValidator{}
}

// WithEnforceSeal enables or disables mandatory seal validation (min length 8).
func (v *TDEValidator) WithEnforceSeal(enforce bool) *TDEValidator {
	v.enforceSeal = enforce
	return v
}

// Validate validates an envelope according to validation rules.
func (v *TDEValidator) Validate(env *Envelope) *TDEResult {
	res := &TDEResult{
		TotalValidated: 1,
	}
	if env == nil {
		res.AddError("envelope", "envelope is nil", 1000)
		return res
	}

	if env.ID == "" {
		res.AddError("id", "ID cannot be empty", 1001)
	}
	if env.Kind == "" {
		res.AddError("kind", "Kind cannot be empty", 1002)
	}
	if env.ParentID == "" {
		res.AddError("parent_id", "ParentID cannot be empty", 1003)
	}
	if v.enforceSeal {
		if len(env.SkillSeal) < 8 {
			res.AddError("skill_seal", "SkillSeal must be at least 8 characters when seal enforcement is enabled", 1004)
		}
	}

	return res
}
