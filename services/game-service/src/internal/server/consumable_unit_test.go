package server

import (
	"context"
	"testing"
	"time"

	"skybloom/game-service/internal/gameobject"
	"skybloom/game-service/internal/gamesession"
	"skybloom/game-service/internal/mapgen"
)

type fakeSessionStore struct {
	saved gamesession.RuntimeState
}

func (s *fakeSessionStore) Start(context.Context, gamesession.StartOptions) (gamesession.State, error) {
	return gamesession.State{}, nil
}

func (s *fakeSessionStore) LoadRuntimeState(context.Context, string) (gamesession.RuntimeState, error) {
	return gamesession.RuntimeState{}, nil
}

func (s *fakeSessionStore) SaveRuntimeState(_ context.Context, _ string, runtime gamesession.RuntimeState) error {
	s.saved = runtime
	return nil
}

func (s *fakeSessionStore) SaveQuizMistake(context.Context, string, string, gamesession.QuizMistake) error {
	return nil
}

func (s *fakeSessionStore) ListQuizMistakes(context.Context, string, string) ([]gamesession.QuizMistake, error) {
	return nil, nil
}

func (s *fakeSessionStore) ClearQuizMistakes(context.Context, string, string) error {
	return nil
}

func (s *fakeSessionStore) Delete(context.Context, string) error {
	return nil
}

func TestUseAirstrikeImmediatelyAppliesDamageAndConsumesCharge(t *testing.T) {
	runtime := consumableTestRuntime()
	runtime.enemies = []gameobject.Enemy{
		{ID: "enemy-1", Type: gameobject.EnemyTypeSmog, Health: 20, Position: gameobject.Position{X: 2.2, Y: 2.2}},
		{ID: "enemy-2", Type: gameobject.EnemyTypeSmog, Health: 20, Position: gameobject.Position{X: 14, Y: 10}},
	}

	resolution, err := resolveConsumableAction(&runtime, useConsumableRequest{
		ItemType: airstrikeItemType,
		Targets:  []gameobject.Position{{X: 2, Y: 2}, {X: 6, Y: 6}, {X: 10, Y: 10}},
	})
	if err != nil {
		t.Fatalf("resolveConsumableAction failed: %v", err)
	}

	if resolution.DeploymentID == "" || len(resolution.Events) < 2 || resolution.Events[0].Type != "airstrike.impact" {
		t.Fatalf("expected immediate impact resolution, got %#v", resolution)
	}
	if runtime.consumables.Airstrike.Status != consumableStatusEmpty || runtime.consumables.Airstrike.Charges != 0 {
		t.Fatalf("expected airstrike charge to be consumed, got %#v", runtime.consumables.Airstrike)
	}
	if len(runtime.enemies) != 1 || runtime.enemies[0].ID != "enemy-2" {
		t.Fatalf("expected damage to be applied immediately, got %#v", runtime.enemies)
	}
}

func TestUseAirstrikeRejectsUnavailableChargeWithoutDamage(t *testing.T) {
	runtime := consumableTestRuntime()
	runtime.consumables.Airstrike = ConsumableItemState{Status: consumableStatusEmpty}
	runtime.enemies = []gameobject.Enemy{{
		ID: "enemy-1", Type: gameobject.EnemyTypeSmog, Health: 100,
		Position: gameobject.Position{X: 2, Y: 2},
	}}

	_, err := resolveConsumableAction(&runtime, useConsumableRequest{
		ItemType: airstrikeItemType,
		Targets:  []gameobject.Position{{X: 2, Y: 2}, {X: 6, Y: 6}, {X: 10, Y: 10}},
	})

	if err == nil {
		t.Fatal("expected unavailable airstrike to be rejected")
	}
	if runtime.enemies[0].Health != 100 {
		t.Fatalf("expected rejected action not to deal damage, got %#v", runtime.enemies)
	}
}

func TestUseAirstrikeRejectsActiveCooldown(t *testing.T) {
	runtime := consumableTestRuntime()
	runtime.consumableCooldownUntil = time.Now().UTC().Add(airstrikeUseCooldown)
	runtime.enemies = []gameobject.Enemy{{
		ID: "enemy-1", Type: gameobject.EnemyTypeSmog, Health: 100,
		Position: gameobject.Position{X: 2, Y: 2},
	}}

	_, err := resolveConsumableAction(&runtime, useConsumableRequest{
		ItemType: airstrikeItemType,
		Targets:  []gameobject.Position{{X: 2, Y: 2}, {X: 6, Y: 6}, {X: 10, Y: 10}},
	})

	if err == nil {
		t.Fatal("expected active Airstrike cooldown to reject damage")
	}
	if runtime.enemies[0].Health != 100 || runtime.consumables.Airstrike.Charges != 1 {
		t.Fatalf("expected rejected cooldown action not to mutate state")
	}
}

func TestAirstrikeOverlappingAreasApplyDamagePerArea(t *testing.T) {
	runtime := consumableTestRuntime()
	runtime.enemies = []gameobject.Enemy{{
		ID: "enemy-overlap", Type: gameobject.EnemyTypeSmog, Health: 300,
		Position: gameobject.Position{X: 2.5, Y: 2},
	}}
	deployment := ConsumableDeploymentState{
		DeploymentID: "55555555-5555-5555-5555-555555555555",
		ItemType:     airstrikeItemType,
		Targets:      []gameobject.Position{{X: 2, Y: 2}, {X: 3, Y: 2}, {X: 10, Y: 10}},
	}

	events := applyAirstrikeDeployment(&runtime, deployment)

	if len(runtime.enemies) != 1 || runtime.enemies[0].Health != 140 {
		t.Fatalf("expected two overlapping hits for 160 total damage, got %#v", runtime.enemies)
	}
	damageEvents := 0
	for _, event := range events {
		if event.Type == "enemy.damage" && event.EnemyID == "enemy-overlap" {
			damageEvents++
		}
	}
	if damageEvents != 2 {
		t.Fatalf("expected two damage events, got %#v", events)
	}
}

func TestAirstrikeDamageRadiusIsTwoTiles(t *testing.T) {
	runtime := consumableTestRuntime()
	runtime.enemies = []gameobject.Enemy{{
		ID: "enemy-edge", Type: gameobject.EnemyTypeSmog, Health: 100,
		Position: gameobject.Position{X: 4, Y: 2},
	}}
	deployment := ConsumableDeploymentState{
		DeploymentID: "66666666-6666-6666-6666-666666666666",
		ItemType:     airstrikeItemType,
		Targets:      []gameobject.Position{{X: 2, Y: 2}, {X: 10, Y: 2}, {X: 10, Y: 10}},
	}

	applyAirstrikeDeployment(&runtime, deployment)

	if len(runtime.enemies) != 1 || runtime.enemies[0].Health != 20 {
		t.Fatalf("expected enemy at radius boundary to take one hit, got %#v", runtime.enemies)
	}
}

func TestMarkAirstrikeQuizPendingPersistsPendingQuiz(t *testing.T) {
	store := &fakeSessionStore{}
	server := &Server{sessions: store}
	runtime := consumableTestRuntime()
	runtime.consumables.Airstrike = ConsumableItemState{Status: consumableStatusEmpty}

	err := server.markConsumableQuizPending(context.Background(), &runtime, consumableQuizPendingRequest{
		ItemType: airstrikeItemType,
		QuizID:   "33333333-3333-3333-3333-333333333333",
	})
	if err != nil {
		t.Fatalf("markConsumableQuizPending failed: %v", err)
	}

	if runtime.consumables.Airstrike.Status != consumableStatusQuizPending ||
		runtime.consumables.Airstrike.PendingQuizID != "33333333-3333-3333-3333-333333333333" {
		t.Fatalf("expected pending quiz state, got %#v", runtime.consumables.Airstrike)
	}
	if store.saved.Consumables.Airstrike.PendingQuizID != "33333333-3333-3333-3333-333333333333" {
		t.Fatalf("expected pending quiz to be saved, got %#v", store.saved.Consumables.Airstrike)
	}
}

func TestFinishAirstrikeQuizCorrectGrantsReadyCharge(t *testing.T) {
	store := &fakeSessionStore{}
	server := &Server{sessions: store}
	runtime := consumableTestRuntime()
	runtime.consumables.Airstrike = ConsumableItemState{
		Status:        consumableStatusQuizPending,
		PendingQuizID: "44444444-4444-4444-4444-444444444444",
	}

	consumables, err := server.finishConsumableQuiz(context.Background(), &runtime, finishConsumableQuizRequest{
		ItemType: airstrikeItemType,
		QuizID:   "44444444-4444-4444-4444-444444444444",
		Correct:  true,
		Charges:  1,
	})
	if err != nil {
		t.Fatalf("finishConsumableQuiz failed: %v", err)
	}

	if consumables.Airstrike.Status != consumableStatusReady ||
		consumables.Airstrike.Charges != 1 ||
		consumables.Airstrike.PendingQuizID != "" {
		t.Fatalf("expected ready airstrike reward, got %#v", consumables.Airstrike)
	}
	if store.saved.Consumables.Airstrike.Status != consumableStatusReady {
		t.Fatalf("expected ready airstrike to be saved, got %#v", store.saved.Consumables.Airstrike)
	}
}

func TestFinishAirstrikeQuizWrongStartsCooldown(t *testing.T) {
	store := &fakeSessionStore{}
	server := &Server{sessions: store}
	runtime := consumableTestRuntime()
	runtime.consumables.Airstrike = ConsumableItemState{
		Status:        consumableStatusQuizPending,
		PendingQuizID: "77777777-7777-7777-7777-777777777777",
	}

	_, err := server.finishConsumableQuiz(context.Background(), &runtime, finishConsumableQuizRequest{
		ItemType: airstrikeItemType,
		QuizID:   "77777777-7777-7777-7777-777777777777",
		Correct:  false,
	})
	if err != nil {
		t.Fatalf("finishConsumableQuiz failed: %v", err)
	}
	if got := consumableCooldownRemainingSeconds(runtime.consumableCooldownUntil, time.Now().UTC()); got < 14 || got > 15 {
		t.Fatal("expected wrong answer to start Airstrike cooldown")
	}
	if !store.saved.ConsumableCooldownUntil.Equal(runtime.consumableCooldownUntil) {
		t.Fatal("expected wrong-answer cooldown to be persisted")
	}
}

func TestAirstrikeCooldownRemainingSeconds(t *testing.T) {
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	if got := consumableCooldownRemainingSeconds(now.Add(airstrikeUseCooldown), now.Add(5*time.Second)); got != 25 {
		t.Fatalf("expected 25 seconds remaining, got %d", got)
	}
	if got := consumableCooldownRemainingSeconds(now.Add(airstrikeUseCooldown), now.Add(airstrikeUseCooldown)); got != 0 {
		t.Fatalf("expected cooldown to expire, got %d", got)
	}
}

func TestValidateAirstrikeAcquireRejectsReadyCharge(t *testing.T) {
	runtime := consumableTestRuntime()

	if err := validateConsumableAcquire(&runtime, airstrikeItemType); err == nil {
		t.Fatalf("expected ready airstrike acquisition to be rejected")
	}
}

func TestValidateAirstrikeAcquireRejectsCooldown(t *testing.T) {
	runtime := consumableTestRuntime()
	runtime.consumables.Airstrike = ConsumableItemState{Status: consumableStatusEmpty}
	runtime.consumableCooldownUntil = time.Now().UTC().Add(airstrikeUseCooldown)

	if err := validateConsumableAcquire(&runtime, airstrikeItemType); err == nil {
		t.Fatal("expected airstrike acquisition during cooldown to be rejected")
	}
}

func consumableTestRuntime() runtimeSession {
	return runtimeSession{
		session: gamesession.State{
			SessionID: "session-1",
			LevelID:   "level-1",
			Health:    gamesession.InitialHealth,
			Tick:      10,
		},
		economy:     gamesession.NewEconomy(0),
		levelMap:    mapgen.GeneratedMap{Width: 18, Height: 12},
		loopStarted: true,
		consumables: ConsumableState{
			Airstrike: ConsumableItemState{Status: consumableStatusReady, Charges: 1},
		},
	}
}
