package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"skybloom/game-service/internal/config"
	"skybloom/game-service/internal/gameobject"
	"skybloom/game-service/internal/gamesession"
	"skybloom/game-service/internal/mapgen"
	"skybloom/game-service/internal/repository"
)

func TestAdvanceRuntimeTickDamagesHealthWhenSmogEscapes(t *testing.T) {
	runtime := runtimeSession{
		session: gamesession.State{
			SessionID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			LevelID:   "11111111-1111-1111-1111-111111111111",
			Health:    10,
			Wave:      3,
			Tick:      240,
		},
		economy:           gamesession.NewEconomy(100),
		loopStarted:       true,
		waveStartedAtTick: 120,
		waveSpawned:       36,
		nextWaveTick:      120,
		path: []gameobject.Position{
			{X: 0, Y: 0},
			{X: 1, Y: 0},
		},
		smogs: []gameobject.Smog{
			{ID: "smog-1", Health: 10, Position: gameobject.Position{X: 1, Y: 0}, PathIndex: 1},
		},
	}

	events := advanceRuntimeTick(&runtime, time.Now().UTC())

	if runtime.session.Health != 0 {
		t.Fatalf("expected health to drop to 0, got %d", runtime.session.Health)
	}
	if len(runtime.smogs) != 0 {
		t.Fatalf("expected escaped smog to be removed, got %d", len(runtime.smogs))
	}
	if len(events) == 0 || events[0].Type != "smog.escaped" {
		t.Fatalf("expected smog escaped event, got %+v", events)
	}
}

func TestAdvanceRuntimeTickWaitsThreeSecondsAfterWaveCleared(t *testing.T) {
	runtime := runtimeSession{
		session: gamesession.State{
			SessionID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			LevelID:   "11111111-1111-1111-1111-111111111111",
			Health:    gamesession.InitialHealth,
			Wave:      1,
			Tick:      10,
		},
		economy:           gamesession.NewEconomy(100),
		loopStarted:       true,
		waveStartedAtTick: 1,
		waveSpawned:       15,
		nextWaveTick:      1,
		path: []gameobject.Position{
			{X: 0, Y: 0},
			{X: 1, Y: 0},
		},
	}

	events := advanceRuntimeTick(&runtime, time.Now().UTC())

	if runtime.session.Wave != 1 {
		t.Fatalf("expected wave 1 to remain current while waiting, got %d", runtime.session.Wave)
	}
	if runtime.nextWaveTick != 71 {
		t.Fatalf("expected wave 2 to be scheduled for tick 71, got %d", runtime.nextWaveTick)
	}
	if runtime.waveSpawned != 0 {
		t.Fatalf("expected spawned count reset for next wave, got %d", runtime.waveSpawned)
	}
	if len(runtime.smogs) != 0 {
		t.Fatalf("expected no smogs while waiting, got %d", len(runtime.smogs))
	}
	if len(events) != 1 || events[0].Type != "wave.cleared" {
		t.Fatalf("expected only wave cleared event, got %+v", events)
	}

	runtime.session.Tick = 69
	events = advanceRuntimeTick(&runtime, time.Now().UTC())
	if runtime.session.Wave != 1 || len(runtime.smogs) != 0 {
		t.Fatalf("wave 2 should not start before tick 71: wave=%d smogs=%d events=%+v", runtime.session.Wave, len(runtime.smogs), events)
	}

	events = advanceRuntimeTick(&runtime, time.Now().UTC())
	if runtime.session.Wave != 2 {
		t.Fatalf("expected wave 2 to start after delay, got %d", runtime.session.Wave)
	}
	if len(runtime.smogs) != 1 {
		t.Fatalf("expected wave 2 to spawn one smog, got %d", len(runtime.smogs))
	}
	if len(events) < 2 || events[0].Type != "wave.started" || events[1].Type != "smog.spawned" {
		t.Fatalf("expected wave started and smog spawned events, got %+v", events)
	}
}

func TestAdvanceRuntimeTickSpawnsSmogsEverySecond(t *testing.T) {
	runtime := runtimeSession{
		session: gamesession.State{
			SessionID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			LevelID:   "11111111-1111-1111-1111-111111111111",
			Health:    gamesession.InitialHealth,
		},
		economy:      gamesession.NewEconomy(100),
		loopStarted:  true,
		nextWaveTick: 1,
		path: []gameobject.Position{
			{X: 0, Y: 0},
			{X: 100, Y: 0},
		},
	}

	advanceRuntimeTick(&runtime, time.Now().UTC())
	if len(runtime.smogs) != 1 {
		t.Fatalf("expected first tick to spawn one smog, got %d", len(runtime.smogs))
	}

	for i := 0; i < 39; i++ {
		advanceRuntimeTick(&runtime, time.Now().UTC())
	}
	if len(runtime.smogs) != 1 {
		t.Fatalf("expected no second smog before two seconds, got %d", len(runtime.smogs))
	}

	advanceRuntimeTick(&runtime, time.Now().UTC())
	if len(runtime.smogs) != 2 {
		t.Fatalf("expected second smog after two seconds, got %d", len(runtime.smogs))
	}
}

func TestWaveDefinitionsScaleEnemyStatsByWave(t *testing.T) {
	waves := waveDefinitions()
	expected := []waveDefinition{
		{Wave: 1, Count: 15, Health: 60, Speed: 0.8},
		{Wave: 2, Count: 24, Health: 105, Speed: 1.13},
		{Wave: 3, Count: 36, Health: 160, Speed: 1.52},
	}

	if len(waves) != len(expected) {
		t.Fatalf("expected %d waves, got %d", len(expected), len(waves))
	}
	for i := range expected {
		if waves[i].Wave != expected[i].Wave {
			t.Fatalf("wave %d: expected wave number %d, got %d", i, expected[i].Wave, waves[i].Wave)
		}
		if waves[i].Count != expected[i].Count {
			t.Fatalf("wave %d: expected count %d, got %d", waves[i].Wave, expected[i].Count, waves[i].Count)
		}
		if waves[i].Health != expected[i].Health {
			t.Fatalf("wave %d: expected health %d, got %d", waves[i].Wave, expected[i].Health, waves[i].Health)
		}
		if diff := waves[i].Speed - expected[i].Speed; diff < -0.0000001 || diff > 0.0000001 {
			t.Fatalf("wave %d: expected speed %.2f, got %.2f", waves[i].Wave, expected[i].Speed, waves[i].Speed)
		}
	}

	for i := 1; i < len(waves); i++ {
		if waves[i].Health <= waves[i-1].Health {
			t.Fatalf("expected wave %d health to exceed wave %d: %+v", waves[i].Wave, waves[i-1].Wave, waves)
		}
		if waves[i].Speed <= waves[i-1].Speed {
			t.Fatalf("expected wave %d speed to exceed wave %d: %+v", waves[i].Wave, waves[i-1].Wave, waves)
		}
	}
}

func TestWebsocketSendsVictoryWhenFinalWaveClears(t *testing.T) {
	levels := &fakeLevelRepository{
		bootstrap: repository.LevelBootstrap{
			LevelID:             "11111111-1111-1111-1111-111111111111",
			UserID:              "22222222-2222-2222-2222-222222222222",
			SubChapterID:        "55555555-5555-5555-5555-555555555555",
			GenerationID:        "generation-1",
			MapSeed:             12345,
			MapAlgorithmVersion: mapgen.Version,
		},
	}
	sessions := &fakeGameSessionStore{
		state: gamesession.State{
			SessionID:    "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			UserID:       "22222222-2222-2222-2222-222222222222",
			LevelID:      "11111111-1111-1111-1111-111111111111",
			GenerationID: "generation-1",
			SubChapterID: "55555555-5555-5555-5555-555555555555",
			Health:       gamesession.InitialHealth,
			Essence:      gamesession.InitialEssence,
			Wave:         3,
			Tick:         200,
			StartedAt:    time.Now().UTC(),
			UpdatedAt:    time.Now().UTC(),
		},
		waveStartedAtTick: 120,
		waveSpawned:       36,
		nextWaveTick:      120,
	}
	handler := NewWithGenerationCachesAndSessions(config.Config{}, levels, nil, nil, nil, nil, nil, sessions).Router()
	httpServer := startHTTPServer(t, handler)
	defer httpServer.Close()

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws"
	header := http.Header{"X-Authenticated-User-Id": []string{"22222222-2222-2222-2222-222222222222"}}
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline failed: %v", err)
	}

	if err := conn.WriteJSON(Message{
		Type: "game.session.start",
		Data: map[string]string{"level_id": "11111111-1111-1111-1111-111111111111"},
	}); err != nil {
		t.Fatalf("WriteJSON session start failed: %v", err)
	}
	readMessageOfType(t, conn, "game.session.started")

	victory := readMessageOfType(t, conn, "game.victory")
	body, err := json.Marshal(victory.Data)
	if err != nil {
		t.Fatalf("Marshal victory failed: %v", err)
	}
	var victoryState GameEndState
	if err := json.Unmarshal(body, &victoryState); err != nil {
		t.Fatalf("Unmarshal victory failed: %v", err)
	}
	if victoryState.Reason != "all_waves_cleared" {
		t.Fatalf("unexpected victory reason %q", victoryState.Reason)
	}
	if victoryState.Wave != 3 {
		t.Fatalf("expected victory on wave 3, got %d", victoryState.Wave)
	}
}

func TestAdvanceRuntimeTickReportsBirdAttackAndSmogDamage(t *testing.T) {
	bird, err := gameobject.NewBird("bird-1", gameobject.BirdTypeSparrow, gameobject.Position{X: 0, Y: 0})
	if err != nil {
		t.Fatalf("NewBird failed: %v", err)
	}
	runtime := runtimeSession{
		session: gamesession.State{
			SessionID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			LevelID:   "11111111-1111-1111-1111-111111111111",
			Health:    gamesession.InitialHealth,
			Tick:      10,
		},
		economy:     gamesession.NewEconomy(100),
		loopStarted: true,
		birds:       []placedBird{{birdType: gameobject.BirdTypeSparrow, bird: bird}},
		smogs: []gameobject.Smog{
			{ID: "smog-1", Health: 30, Position: gameobject.Position{X: 0.1, Y: 0}},
		},
	}

	events := advanceRuntimeTick(&runtime, time.Now().UTC())

	if len(runtime.projectiles) != 0 {
		t.Fatalf("expected locked projectile to resolve in one tick, got %d active projectiles", len(runtime.projectiles))
	}
	if len(runtime.smogs) != 1 || runtime.smogs[0].Health != 20 {
		t.Fatalf("expected smog health 20, got %+v", runtime.smogs)
	}
	var sawAttack, sawDamage bool
	for _, event := range events {
		if event.Type == "bird.attack" && event.BirdID == "bird-1" && event.SmogID == "smog-1" {
			sawAttack = true
		}
		if event.Type == "smog.damage" && event.SmogID == "smog-1" && event.Health == 20 {
			sawDamage = true
		}
	}
	if !sawAttack || !sawDamage {
		t.Fatalf("expected attack and damage events, got %+v", events)
	}
}
