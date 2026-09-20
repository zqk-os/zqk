package storage

import (
	"context"
	"testing"
)

func TestGetFallbackChangeJournalGenerator_Caching(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	projectRoot := t.TempDir()
	journalDir := t.TempDir()

	g1 := getFallbackChangeJournalGenerator(ctx, projectRoot, journalDir)
	if g1 == nil {
		t.Fatalf("getFallbackChangeJournalGenerator returned nil")
	}

	g2 := getFallbackChangeJournalGenerator(ctx, projectRoot, journalDir)
	if g1 != g2 {
		t.Errorf("getFallbackChangeJournalGenerator failed to cache generator: got %v, want %v", g2, g1)
	}
}

func TestGetFallbackChangeJournalGenerator_EmptyProjectRoot(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	journalDir := t.TempDir()

	g := getFallbackChangeJournalGenerator(ctx, "", journalDir)
	if g == nil {
		t.Fatalf("getFallbackChangeJournalGenerator with empty projectRoot returned nil")
	}
}

func TestGetFallbackChangeJournalGenerator_LifetimeCounters(t *testing.T) {
	ctx := context.Background()
	projectRoot := t.TempDir()
	journalDir := t.TempDir()

	createdBefore, _ := GetFallbackJournalGeneratorStats()

	g1 := getFallbackChangeJournalGenerator(ctx, projectRoot, journalDir)
	if g1 == nil {
		t.Fatalf("expected non-nil generator")
	}

	createdAfter, reusedAfter := GetFallbackJournalGeneratorStats()
	if createdAfter != createdBefore+1 {
		t.Errorf("expected created to increase by 1, got before=%d after=%d", createdBefore, createdAfter)
	}

	g2 := getFallbackChangeJournalGenerator(ctx, projectRoot, journalDir)
	if g2 != g1 {
		t.Errorf("expected same generator instance")
	}

	_, reusedFinal := GetFallbackJournalGeneratorStats()
	if reusedFinal != reusedAfter+1 {
		t.Errorf("expected reused to increase by 1, got before=%d after=%d", reusedAfter, reusedFinal)
	}
}
