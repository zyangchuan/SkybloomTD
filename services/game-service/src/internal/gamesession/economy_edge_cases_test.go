package gamesession

import "testing"

func TestNewEconomyWithNegativeEssenceClampsToZero(t *testing.T) {
	e := NewEconomy(-100)
	if e.Essence != 0 {
		t.Fatalf("expected essence 0 for negative input, got %d", e.Essence)
	}
}

func TestEconomyConsumeReturnsFalseForNegativeAmount(t *testing.T) {
	e := NewEconomy(100)
	if e.Consume(-1) {
		t.Fatal("expected Consume(-1) to return false")
	}
	if e.Essence != 100 {
		t.Fatalf("expected essence unchanged at 100, got %d", e.Essence)
	}
}

func TestEconomyConsumeZeroSucceedsWithoutChangingEssence(t *testing.T) {
	e := NewEconomy(50)
	if !e.Consume(0) {
		t.Fatal("expected Consume(0) to return true")
	}
	if e.Essence != 50 {
		t.Fatalf("expected essence unchanged at 50, got %d", e.Essence)
	}
}

func TestEconomyAddIgnoresNonPositiveAmounts(t *testing.T) {
	e := NewEconomy(100)
	e.Add(0)
	if e.Essence != 100 {
		t.Fatalf("expected Add(0) to leave essence at 100, got %d", e.Essence)
	}
	e.Add(-50)
	if e.Essence != 100 {
		t.Fatalf("expected Add(-50) to leave essence at 100, got %d", e.Essence)
	}
}

func TestEconomyAddIncreasesEssenceByAmount(t *testing.T) {
	e := NewEconomy(100)
	e.Add(30)
	if e.Essence != 130 {
		t.Fatalf("expected essence 130 after Add(30), got %d", e.Essence)
	}
}
