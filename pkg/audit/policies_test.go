package audit

import (
	"context"
	"testing"
)

func TestPolicyEngine_Validate(t *testing.T) {
	engine := NewPolicyEngine()

	// Register a simple mock policy
	engine.RegisterPolicy(&mockPolicy{
		name: "POL-MOCK-001",
		isValid: func(record AuditRecord) bool {
			return record.Action != "forbidden_action"
		},
	})

	tests := []struct {
		name    string
		record  AuditRecord
		wantErr bool
	}{
		{
			name: "Valid action",
			record: AuditRecord{
				ID:     "1",
				Action: "allowed_action",
				Target: "sys",
			},
			wantErr: false,
		},
		{
			name: "Invalid action",
			record: AuditRecord{
				ID:     "2",
				Action: "forbidden_action",
				Target: "sys",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := engine.Validate(context.Background(), tt.record)
			if (err != nil) != tt.wantErr {
				t.Errorf("PolicyEngine.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

type mockPolicy struct {
	name    string
	isValid func(AuditRecord) bool
}

func (m *mockPolicy) Name() string { return m.name }
func (m *mockPolicy) Evaluate(ctx context.Context, record AuditRecord) error {
	if !m.isValid(record) {
		return &PolicyViolationError{PolicyName: m.name, Reason: "action forbidden"}
	}
	return nil
}
