package gamesession

import "testing"

func TestEconomyConsume(t *testing.T) {
	economy := NewEconomy(100)
	if !economy.Consume(40) {
		t.Fatal("expected consume to succeed")
	}
	if economy.Essence != 60 {
		t.Fatalf("expected 60 essence, got %d", economy.Essence)
	}
}

func TestEconomyConsumeInsufficientEssence(t *testing.T) {
	economy := NewEconomy(30)
	if economy.Consume(40) {
		t.Fatal("expected consume to fail")
	}
	if economy.Essence != 30 {
		t.Fatalf("expected essence to remain 30, got %d", economy.Essence)
	}
}
