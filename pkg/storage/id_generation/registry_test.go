package id_generation

import (
	"context"
	"testing"
)

type dummyStrategy struct {
	name string
}

func (d *dummyStrategy) Name() string {
	return d.name
}

func (d *dummyStrategy) Description() string {
	return "dummy strategy for testing"
}

func (d *dummyStrategy) GenerateNextID(ctx context.Context, kind string, prefix string, existingIDs []string) (string, error) {
	return prefix + "-123", nil
}

func TestRegisterStrategy_Errors(t *testing.T) {
	// 1. Nil strategy should return error, not panic
	err := RegisterStrategy(nil)
	if err == nil {
		t.Fatal("expected error registering nil strategy, got nil")
	}

	// 2. Empty name should return error, not panic
	err = RegisterStrategy(&dummyStrategy{name: ""})
	if err == nil {
		t.Fatal("expected error registering strategy with empty name, got nil")
	}

	// 3. Successful registration
	strat := &dummyStrategy{name: "test_strat_unique"}
	err = RegisterStrategy(strat)
	if err != nil {
		t.Fatalf("unexpected error registering strategy: %v", err)
	}

	// 4. Duplicate registration should return error, not panic
	err = RegisterStrategy(strat)
	if err == nil {
		t.Fatal("expected error registering duplicate strategy, got nil")
	}
}
