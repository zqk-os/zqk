package id_generation

import (
	"testing"
)

func TestInit_DefaultStrategiesRegistered(t *testing.T) {
	seq := GetStrategy("sequential")
	if seq == nil {
		t.Fatal("expected sequential strategy to be registered")
	}

	uuid := GetStrategy("uuid")
	if uuid == nil {
		t.Fatal("expected uuid strategy to be registered")
	}
}
