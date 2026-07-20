package server

import (
	"context"
	"testing"

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

func TestPrepareAirstrikeReservesDeployment(t *testing.T) {
	store := &fakeSessionStore{}
	server := &Server{sessions: store}
	runtime := consumableTestRuntime()

	deployment, err := server.prepareConsumableDeployment(context.Background(), &runtime, useConsumableRequest{
		ItemType: airstrikeItemType,
		Targets: []gameobject.Position{
			{X: 2, Y: 2},
			{X: 4, Y: 4},
			{X: 6, Y: 6},
		},
	})
	if err != nil {
		t.Fatalf("prepareConsumableDeployment failed: %v", err)
	}

	if deployment.DeploymentID == "" || deployment.AutoCommitAtTick != runtime.session.Tick+airstrikeAutoCommitTicks {
		t.Fatalf("unexpected deployment: %#v", deployment)
	}
	if runtime.consumables.Airstrike.Status != consumableStatusPending || runtime.consumables.Airstrike.Charges != 0 {
		t.Fatalf("expected airstrike to be reserved, got %#v", runtime.consumables.Airstrike)
	}
	if len(runtime.pendingConsumables) != 1 {
		t.Fatalf("expected one pending deployment, got %d", len(runtime.pendingConsumables))
	}
	if store.saved.Consumables.Airstrike.Status != consumableStatusPending {
		t.Fatalf("expected reserved consumable to be saved, got %#v", store.saved.Consumables)
	}
}

func TestCommitAirstrikeAppliesDamageAndClearsPendingDeployment(t *testing.T) {
	store := &fakeSessionStore{}
	server := &Server{sessions: store}
	runtime := consumableTestRuntime()
	runtime.consumables.Airstrike = ConsumableItemState{Status: consumableStatusPending}
	runtime.pendingConsumables = []ConsumableDeploymentState{{
		DeploymentID:     "11111111-1111-1111-1111-111111111111",
		ItemType:         airstrikeItemType,
		Targets:          []gameobject.Position{{X: 2, Y: 2}, {X: 6, Y: 6}, {X: 10, Y: 10}},
		PreparedAtTick:   10,
		AutoCommitAtTick: 50,
		Status:           consumableStatusPending,
	}}
	runtime.enemies = []gameobject.Enemy{
		{ID: "enemy-1", Type: gameobject.EnemyTypeSmog, Health: 20, Position: gameobject.Position{X: 2.2, Y: 2.2}},
		{ID: "enemy-2", Type: gameobject.EnemyTypeSmog, Health: 20, Position: gameobject.Position{X: 14, Y: 10}},
	}

	resolution, err := server.resolveConsumableDeployment(context.Background(), &runtime, "11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatalf("resolveConsumableDeployment failed: %v", err)
	}

	if len(runtime.pendingConsumables) != 0 {
		t.Fatalf("expected pending deployment to clear, got %#v", runtime.pendingConsumables)
	}
	if len(runtime.enemies) != 1 || runtime.enemies[0].ID != "enemy-2" {
		t.Fatalf("expected only outside enemy to remain, got %#v", runtime.enemies)
	}
	if len(resolution.Events) < 2 || resolution.Events[0].Type != "airstrike.impact" {
		t.Fatalf("expected impact and damage events, got %#v", resolution.Events)
	}
	if runtime.consumables.Airstrike.Status != consumableStatusEmpty {
		t.Fatalf("expected airstrike to become empty, got %#v", runtime.consumables.Airstrike)
	}
	if len(store.saved.PendingConsumables) != 0 {
		t.Fatalf("expected cleared pending deployment to be saved, got %#v", store.saved.PendingConsumables)
	}
}

func TestAirstrikeAutoResolvesWhenCommitDoesNotArrive(t *testing.T) {
	server := &Server{}
	runtime := consumableTestRuntime()
	runtime.session.Tick = 50
	runtime.consumables.Airstrike = ConsumableItemState{Status: consumableStatusPending}
	runtime.pendingConsumables = []ConsumableDeploymentState{{
		DeploymentID:     "22222222-2222-2222-2222-222222222222",
		ItemType:         airstrikeItemType,
		Targets:          []gameobject.Position{{X: 2, Y: 2}, {X: 6, Y: 6}, {X: 10, Y: 10}},
		PreparedAtTick:   10,
		AutoCommitAtTick: 50,
		Status:           consumableStatusPending,
	}}
	runtime.enemies = []gameobject.Enemy{
		{ID: "enemy-1", Type: gameobject.EnemyTypeSmog, Health: 20, Position: gameobject.Position{X: 2.2, Y: 2.2}},
	}

	events := server.resolveDueConsumableDeployments(context.Background(), &runtime)

	if len(events) < 2 || events[0].Type != "airstrike.impact" {
		t.Fatalf("expected auto-resolve impact and damage events, got %#v", events)
	}
	if len(runtime.pendingConsumables) != 0 {
		t.Fatalf("expected pending deployment to clear, got %#v", runtime.pendingConsumables)
	}
	if len(runtime.enemies) != 0 {
		t.Fatalf("expected enemy to be removed by airstrike, got %#v", runtime.enemies)
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

func TestValidateAirstrikeAcquireRejectsReadyCharge(t *testing.T) {
	runtime := consumableTestRuntime()

	if err := validateConsumableAcquire(&runtime, airstrikeItemType); err == nil {
		t.Fatalf("expected ready airstrike acquisition to be rejected")
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
