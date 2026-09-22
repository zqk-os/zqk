package objects

import (
	"testing"
)

func TestStorageType_IsStreamStorage(t *testing.T) {
	if !IsStreamStorage("stream") {
		t.Errorf("IsStreamStorage(\"stream\") = false, want true")
	}
	if !IsStreamStorage(string(StorageTypeStream)) {
		t.Errorf("IsStreamStorage(StorageTypeStream) = false, want true")
	}
	if IsStreamStorage("cas_entity") {
		t.Errorf("IsStreamStorage(\"cas_entity\") = true, want false")
	}
	if IsStreamStorage("light_file") {
		t.Errorf("IsStreamStorage(\"light_file\") = true, want false")
	}
	if IsStreamStorage("") {
		t.Errorf("IsStreamStorage(\"\") = true, want false")
	}
}

func TestStorageType_IsBypassStorageType(t *testing.T) {
	if !IsBypassStorageType("stream") {
		t.Errorf("IsBypassStorageType(\"stream\") = false, want true")
	}
	if IsBypassStorageType("cas_entity") {
		t.Errorf("IsBypassStorageType(\"cas_entity\") = true, want false")
	}
	if IsBypassStorageType("") {
		t.Errorf("IsBypassStorageType(\"\") = true, want false")
	}
}

func TestStorageType_IsBypassKind(t *testing.T) {
	// Directly passing "stream" storage type
	if !IsBypassKind("stream") {
		t.Errorf("IsBypassKind(\"stream\") = false, want true")
	}
	// Direct stream kind
	if !IsBypassKind(KindAuditEvent) {
		t.Errorf("IsBypassKind(%q) = false, want true (storage_profile: stream)", KindAuditEvent)
	}
	if !IsBypassKind(KindAuditAggregationMetric) {
		t.Errorf("IsBypassKind(%q) = false, want true (storage_profile: stream)", KindAuditAggregationMetric)
	}
	if !IsBypassKind(KindChangeJournalEntry) {
		t.Errorf("IsBypassKind(%q) = false, want true (storage_profile: stream)", KindChangeJournalEntry)
	}
	// Non-stream CAS kind
	if IsBypassKind(KindBacklogItem) {
		t.Errorf("IsBypassKind(%q) = true, want false (storage_profile: cas_entity)", KindBacklogItem)
	}
	if IsBypassKind(KindRequirement) {
		t.Errorf("IsBypassKind(%q) = true, want false (storage_profile: cas_entity)", KindRequirement)
	}
	if IsBypassKind("") {
		t.Errorf("IsBypassKind(\"\") = true, want false")
	}
}

func TestStorageType_ResolveStorageProfile(t *testing.T) {
	if profile := ResolveStorageProfile(""); profile != "" {
		t.Errorf("expected empty for empty kind, got %q", profile)
	}
	if profile := ResolveStorageProfile(KindAuditEvent); profile != "stream" {
		t.Errorf("expected stream for audit_event, got %q", profile)
	}
	if profile := ResolveStorageProfile(KindBacklogItem); profile != "cas_entity" {
		t.Errorf("expected cas_entity for backlog_item, got %q", profile)
	}
	if profile := ResolveStorageProfile("nonexistent_kind_xyz"); profile != "" {
		t.Errorf("expected empty for unknown kind, got %q", profile)
	}
}
