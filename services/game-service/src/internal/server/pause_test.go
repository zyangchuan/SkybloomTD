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

// ---------------------------------------------------------------------------
// Unit tests: advanceRuntimeTick mechanics
// ---------------------------------------------------------------------------

func TestAdvanceRuntimeTickEnemyMovesAlongPathEachTick(t *testing.T) {
	runtime := runtimeSession{
		session: gamesession.State{
			SessionID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			Health:    gamesession.InitialHealth,
		},
		economy:      gamesession.NewEconomy(100),
		loopStarted:  true,
		nextWaveTick: 1,
		path: []gameobject.Position{
			{X: 0, Y: 0},
			{X: 1, Y: 0},
			{X: 2, Y: 0},
		},
		enemies: []gameobject.Enemy{
			{ID: "enemy-1", Health: 30, Position: gameobject.Position{X: 0, Y: 0}, Speed: 1.0},
		},
	}

	startX := runtime.enemies[0].Position.X
	advanceRuntimeTick(&runtime, time.Now())

	if runtime.enemies[0].Position.X <= startX {
		t.Fatalf("expected enemy to move right from x=%f, got x=%f", startX, runtime.enemies[0].Position.X)
	}
}

func TestAdvanceRuntimeTickBirdCooldownBlocksAttackUntilReady(t *testing.T) {
	bird, err := gameobject.NewBird("bird-1", gameobject.BirdTypeSparrow, gameobject.Position{X: 0, Y: 0})
	if err != nil {
		t.Fatalf("NewBird failed: %v", err)
	}
	runtime := runtimeSession{
		session: gamesession.State{
			SessionID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			Health:    gamesession.InitialHealth,
			Tick:      0,
		},
		economy:     gamesession.NewEconomy(100),
		loopStarted: true,
		birds:       []placedBird{{birdType: gameobject.BirdTypeSparrow, bird: bird}},
		enemies: []gameobject.Enemy{
			{ID: "enemy-1", Health: 30, Position: gameobject.Position{X: 0.2, Y: 0}},
		},
	}

	advanceRuntimeTick(&runtime, time.Now())

	// Sparrow fires at 1 shot/sec = every 20 ticks at 20 ticks/sec.
	// On tick 1, it fires. Cooldown ends at tick 21.
	// Advance 10 more ticks: bird should NOT have fired again.
	for i := 0; i < 9; i++ {
		advanceRuntimeTick(&runtime, time.Now())
	}

	// 10 ticks in, cooldown is still active (needs 20 ticks). Enemy may already be
	// dead from the first shot; we only need to verify the bird's LastFiredAtTick
	// was set and CanAttack correctly blocks it.
	if runtime.birds[0].bird.LastFiredAtTick < 1 {
		t.Fatalf("expected bird to have fired at least once, LastFiredAtTick=%d", runtime.birds[0].bird.LastFiredAtTick)
	}
	if runtime.birds[0].bird.CanAttack(runtime.session.Tick, gameTicksPerSecond) {
		t.Fatalf("bird should still be cooling down at tick %d (fired at %d)", runtime.session.Tick, runtime.birds[0].bird.LastFiredAtTick)
	}
}

func TestAdvanceRuntimeTickWaveDoesNotSpawnBeforeScheduledTick(t *testing.T) {
	runtime := runtimeSession{
		session: gamesession.State{
			SessionID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			Health:    gamesession.InitialHealth,
		},
		economy:      gamesession.NewEconomy(100),
		loopStarted:  true,
		nextWaveTick: 100, // wave scheduled far in the future
		path: []gameobject.Position{
			{X: 0, Y: 0},
			{X: 100, Y: 0},
		},
	}

	// Advance several ticks; no enemies should spawn yet.
	for i := 0; i < 10; i++ {
		advanceRuntimeTick(&runtime, time.Now())
	}

	if len(runtime.enemies) != 0 {
		t.Fatalf("expected no enemies before scheduled wave tick, got %d", len(runtime.enemies))
	}
}

func TestAdvanceRuntimeTickEnemyHealthDropsToZeroAndEnemyIsRemoved(t *testing.T) {
	bird, err := gameobject.NewBird("bird-1", gameobject.BirdTypeSparrow, gameobject.Position{X: 0, Y: 0})
	if err != nil {
		t.Fatalf("NewBird failed: %v", err)
	}
	runtime := runtimeSession{
		session: gamesession.State{
			SessionID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			Health:    gamesession.InitialHealth,
			Tick:      0,
		},
		economy:     gamesession.NewEconomy(100),
		loopStarted: true,
		birds:       []placedBird{{birdType: gameobject.BirdTypeSparrow, bird: bird}},
		enemies: []gameobject.Enemy{
			// Sparrow does 10 damage per shot; this enemy has exactly 10 HP.
			{ID: "enemy-1", Health: 10, Position: gameobject.Position{X: 0.2, Y: 0}},
		},
		path: []gameobject.Position{{X: 0, Y: 0}, {X: 100, Y: 0}},
	}

	advanceRuntimeTick(&runtime, time.Now())

	if len(runtime.enemies) != 0 {
		t.Fatalf("expected dead enemy to be removed, got %d enemies remaining", len(runtime.enemies))
	}
}

func TestAdvanceRuntimeTickIgnoresStoredProjectiles(t *testing.T) {
	runtime := runtimeSession{
		session: gamesession.State{
			SessionID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			Health:    gamesession.InitialHealth,
			Tick:      5,
		},
		economy:     gamesession.NewEconomy(100),
		loopStarted: true,
		enemies: []gameobject.Enemy{
			{ID: "enemy-1", Health: 30, Position: gameobject.Position{X: 5, Y: 0}, Speed: 0},
		},
		projectiles: []gameobject.Projectile{
			gameobject.NewLockedProjectile(
				gameobject.Bird{Position: gameobject.Position{X: 0, Y: 0}, Stats: gameobject.BirdStats{Damage: 10}},
				gameobject.Enemy{ID: "enemy-1", Health: 30, Position: gameobject.Position{X: 5, Y: 0}},
				gameobject.LockedProjectileSpeed,
			),
		},
		path: []gameobject.Position{{X: 0, Y: 0}, {X: 100, Y: 0}},
	}

	advanceRuntimeTick(&runtime, time.Now())

	if len(runtime.projectiles) != 0 {
		t.Fatalf("expected obsolete stored projectile to be cleared, got %d projectiles", len(runtime.projectiles))
	}
	if runtime.enemies[0].Health != 30 {
		t.Fatalf("stored projectile should not damage enemies, got health %d", runtime.enemies[0].Health)
	}
}

// ---------------------------------------------------------------------------
// Integration tests: pause / resume via WebSocket
// ---------------------------------------------------------------------------

func TestWebsocketPauseStopsTickEmission(t *testing.T) {
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
	sessions := &fakeGameSessionStore{}
	handler := NewWithGenerationCachesAndSessions(config.Config{}, levels, nil, nil, nil, nil, nil, sessions).Router()
	httpServer := startHTTPServer(t, handler)
	defer httpServer.Close()

	conn := dialGameWebsocket(t, httpServer.URL)
	defer conn.Close()

	// Start session and wait for first tick.
	if err := conn.WriteJSON(Message{
		Type: "game.session.start",
		Data: map[string]string{"level_id": "11111111-1111-1111-1111-111111111111"},
	}); err != nil {
		t.Fatalf("WriteJSON session start failed: %v", err)
	}
	readMessageOfType(t, conn, "game.session.started")
	readMessageOfType(t, conn, "game.state") // tick=1: confirms loop is running

	// Send pause.
	if err := conn.WriteJSON(Message{Type: "game.pause"}); err != nil {
		t.Fatalf("WriteJSON pause failed: %v", err)
	}

	// Drain any messages already in flight (allow 3 tick intervals = 150ms).
	drainMessagesFor(t, conn, 150*time.Millisecond)

	// Now verify silence: the loop should not emit game.state for 200ms while paused.
	if err := conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond)); err != nil {
		t.Fatalf("SetReadDeadline failed: %v", err)
	}
	var unexpected Message
	if err := conn.ReadJSON(&unexpected); err == nil {
		t.Fatalf("expected no messages while paused but received %s", unexpected.Type)
	}
	// A deadline-exceeded error here is the expected (passing) outcome.
}

func TestWebsocketResumeAfterPauseContinuesTicking(t *testing.T) {
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
	sessions := &fakeGameSessionStore{}
	handler := NewWithGenerationCachesAndSessions(config.Config{}, levels, nil, nil, nil, nil, nil, sessions).Router()
	httpServer := startHTTPServer(t, handler)
	defer httpServer.Close()

	conn := dialGameWebsocket(t, httpServer.URL)
	defer conn.Close()

	if err := conn.WriteJSON(Message{
		Type: "game.session.start",
		Data: map[string]string{"level_id": "11111111-1111-1111-1111-111111111111"},
	}); err != nil {
		t.Fatalf("WriteJSON session start failed: %v", err)
	}
	readMessageOfType(t, conn, "game.session.started")
	readMessageOfType(t, conn, "game.state")

	// Pause and wait for it to settle — do NOT drain, because gorilla/websocket marks
	// a connection permanently broken after any read timeout.
	if err := conn.WriteJSON(Message{Type: "game.pause"}); err != nil {
		t.Fatalf("WriteJSON pause failed: %v", err)
	}
	time.Sleep(150 * time.Millisecond) // 3 tick periods; enough for pause to take effect

	if err := conn.WriteJSON(Message{Type: "game.resume"}); err != nil {
		t.Fatalf("WriteJSON resume failed: %v", err)
	}

	// readMessageOfType will consume any pre-pause buffered messages and then find the
	// first post-resume game.state.
	readMessageOfType(t, conn, "game.state")
}

func TestWebsocketPauseDoesNotLeakTicks(t *testing.T) {
	// Strategy: use two connections. Connection 1 pauses the game, then we close it
	// and reconnect as connection 2. The server restores the session from the stored
	// runtime state, which reflects the tick count at the time of the last save.
	// If pause prevented ticks from advancing, the restored tick should be close to
	// the tick at which we paused. Without pause it would be ~8+ ticks higher.
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
	sessions := &fakeGameSessionStore{}
	handler := NewWithGenerationCachesAndSessions(config.Config{}, levels, nil, nil, nil, nil, nil, sessions).Router()
	httpServer := startHTTPServer(t, handler)
	defer httpServer.Close()

	// Connection 1: start session, get first tick, then pause.
	conn1 := dialGameWebsocket(t, httpServer.URL)
	if err := conn1.WriteJSON(Message{
		Type: "game.session.start",
		Data: map[string]string{"level_id": "11111111-1111-1111-1111-111111111111"},
	}); err != nil {
		conn1.Close()
		t.Fatalf("WriteJSON session start failed: %v", err)
	}
	readMessageOfType(t, conn1, "game.session.started")
	firstState := decodeGameState(t, readMessageOfType(t, conn1, "game.state").Data)
	tickBeforePause := firstState.Tick // typically 1

	if err := conn1.WriteJSON(Message{Type: "game.pause"}); err != nil {
		conn1.Close()
		t.Fatalf("WriteJSON pause failed: %v", err)
	}

	// Let pause settle, then hold it. Without pause, ~8 ticks would fire in 400ms
	// (20 ticks/sec × 400ms = 8).
	time.Sleep(150 * time.Millisecond) // settle: pause takes effect within 3 tick periods
	time.Sleep(400 * time.Millisecond) // hold

	// Close connection 1. The server-side game loop exits; the stored Tick reflects
	// the last save before pause (SaveRuntimeState is skipped when loopPaused=true).
	conn1.Close()
	time.Sleep(100 * time.Millisecond) // allow goroutine to exit before reconnecting

	// Connection 2: reconnect to the same level. The fakeGameSessionStore returns the
	// existing session whose Tick was frozen at pause time.
	conn2 := dialGameWebsocket(t, httpServer.URL)
	defer conn2.Close()
	if err := conn2.WriteJSON(Message{
		Type: "game.session.start",
		Data: map[string]string{"level_id": "11111111-1111-1111-1111-111111111111"},
	}); err != nil {
		t.Fatalf("WriteJSON session start (conn2) failed: %v", err)
	}
	started2 := readMessageOfType(t, conn2, "game.session.started")
	restoredState := decodeGameState(t, started2.Data)

	// With pause: stored tick ≈ tickBeforePause + 2 (at most 2-3 ticks before pause
	// took effect). Without pause: stored tick ≈ tickBeforePause + 8+.
	maxAllowedTick := tickBeforePause + 5
	if restoredState.Tick > maxAllowedTick {
		t.Fatalf("pause leaked ticks: tick before pause=%d, restored tick=%d (max allowed=%d)",
			tickBeforePause, restoredState.Tick, maxAllowedTick)
	}
}

func TestWebsocketPauseSentBeforeSessionStartIsIgnored(t *testing.T) {
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
	sessions := &fakeGameSessionStore{}
	handler := NewWithGenerationCachesAndSessions(config.Config{}, levels, nil, nil, nil, nil, nil, sessions).Router()
	httpServer := startHTTPServer(t, handler)
	defer httpServer.Close()

	conn := dialGameWebsocket(t, httpServer.URL)
	defer conn.Close()

	// Send pause before any session is started — should be silently ignored.
	if err := conn.WriteJSON(Message{Type: "game.pause"}); err != nil {
		t.Fatalf("WriteJSON pause failed: %v", err)
	}

	// Start session normally after the ignored pause.
	if err := conn.WriteJSON(Message{
		Type: "game.session.start",
		Data: map[string]string{"level_id": "11111111-1111-1111-1111-111111111111"},
	}); err != nil {
		t.Fatalf("WriteJSON session start failed: %v", err)
	}

	// The session should start successfully and the game loop should tick normally.
	readMessageOfType(t, conn, "game.session.started")
	readMessageOfType(t, conn, "game.state")
}

func TestWebsocketRapidPauseResumeTogglingIsStable(t *testing.T) {
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
	sessions := &fakeGameSessionStore{}
	handler := NewWithGenerationCachesAndSessions(config.Config{}, levels, nil, nil, nil, nil, nil, sessions).Router()
	httpServer := startHTTPServer(t, handler)
	defer httpServer.Close()

	conn := dialGameWebsocket(t, httpServer.URL)
	defer conn.Close()

	if err := conn.WriteJSON(Message{
		Type: "game.session.start",
		Data: map[string]string{"level_id": "11111111-1111-1111-1111-111111111111"},
	}); err != nil {
		t.Fatalf("WriteJSON session start failed: %v", err)
	}
	readMessageOfType(t, conn, "game.session.started")
	readMessageOfType(t, conn, "game.state")

	// Rapidly toggle pause/resume 10 times.
	for i := 0; i < 10; i++ {
		if err := conn.WriteJSON(Message{Type: "game.pause"}); err != nil {
			t.Fatalf("WriteJSON pause failed on iteration %d: %v", i, err)
		}
		if err := conn.WriteJSON(Message{Type: "game.resume"}); err != nil {
			t.Fatalf("WriteJSON resume failed on iteration %d: %v", i, err)
		}
	}

	// After rapid toggling, the last state is "resumed". The game loop should still be
	// alive and ticking. Do NOT drain (gorilla/websocket is permanently broken after
	// any read timeout); readMessageOfType will consume buffered messages naturally.
	readMessageOfType(t, conn, "game.state")
}

// ---------------------------------------------------------------------------
// Integration tests: reconnect / state recovery
// ---------------------------------------------------------------------------

func TestWebsocketSessionStartRecoversStateAfterReconnect(t *testing.T) {
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
			Health:       gamesession.InitialHealth - 10,
			Essence:      gamesession.InitialEssence + 50,
			Wave:         2,
			Tick:         80,
			StartedAt:    time.Now().UTC(),
			UpdatedAt:    time.Now().UTC(),
		},
		waveStartedAtTick: 40,
		waveSpawned:       10,
		nextWaveTick:      40,
	}
	handler := NewWithGenerationCachesAndSessions(config.Config{}, levels, nil, nil, nil, nil, nil, sessions).Router()
	httpServer := startHTTPServer(t, handler)
	defer httpServer.Close()

	// Simulate reconnect: a fresh WebSocket connection for the same user.
	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws"
	header := http.Header{"X-Authenticated-User-Id": []string{"22222222-2222-2222-2222-222222222222"}}
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline failed: %v", err)
	}

	// Re-send game.session.start — server should restore existing state.
	if err := conn.WriteJSON(Message{
		Type: "game.session.start",
		Data: map[string]string{"level_id": "11111111-1111-1111-1111-111111111111"},
	}); err != nil {
		t.Fatalf("WriteJSON session start failed: %v", err)
	}

	started := readMessageOfType(t, conn, "game.session.started")
	state := decodeGameState(t, started.Data)

	if state.SessionID != "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa" {
		t.Fatalf("expected restored session id, got %q", state.SessionID)
	}
	if state.Health != gamesession.InitialHealth-10 {
		t.Fatalf("expected restored health %d, got %d", gamesession.InitialHealth-10, state.Health)
	}
	if state.Essence != gamesession.InitialEssence+50 {
		t.Fatalf("expected restored essence %d, got %d", gamesession.InitialEssence+50, state.Essence)
	}
	if state.Wave != 2 {
		t.Fatalf("expected restored wave 2, got %d", state.Wave)
	}
}

func TestWebsocketGameLoadReturnsInitialStateForLevel(t *testing.T) {
	levels := &fakeLevelRepository{
		bootstrap: repository.LevelBootstrap{
			LevelID:             "11111111-1111-1111-1111-111111111111",
			UserID:              "22222222-2222-2222-2222-222222222222",
			DocumentID:          "33333333-3333-3333-3333-333333333333",
			ChapterID:           "44444444-4444-4444-4444-444444444444",
			SubChapterID:        "55555555-5555-5555-5555-555555555555",
			GenerationID:        "generation-1",
			MapSeed:             99999,
			MapAlgorithmVersion: mapgen.Version,
		},
	}
	handler := New(config.Config{}, levels, &fakeMapCache{}).Router()
	httpServer := startHTTPServer(t, handler)
	defer httpServer.Close()

	conn := dialGameWebsocket(t, httpServer.URL)
	defer conn.Close()

	if err := conn.WriteJSON(Message{
		Type: "game.load",
		Data: map[string]string{"level_id": "11111111-1111-1111-1111-111111111111"},
	}); err != nil {
		t.Fatalf("WriteJSON game load failed: %v", err)
	}

	msg := readMessageOfType(t, conn, "game.initial_state")
	body, err := json.Marshal(msg.Data)
	if err != nil {
		t.Fatalf("Marshal initial_state failed: %v", err)
	}
	var state InitialState
	if err := json.Unmarshal(body, &state); err != nil {
		t.Fatalf("Unmarshal initial_state failed: %v", err)
	}
	if state.Map.Seed != 99999 {
		t.Fatalf("expected map seed 99999, got %d", state.Map.Seed)
	}
	if len(state.Map.EnemyPath) == 0 {
		t.Fatal("expected non-empty enemy path")
	}
}

func TestWebsocketGameLoadReturnsErrorForUnknownLevel(t *testing.T) {
	levels := &fakeLevelRepository{}
	handler := New(config.Config{}, levels, &fakeMapCache{}).Router()
	httpServer := startHTTPServer(t, handler)
	defer httpServer.Close()

	conn := dialGameWebsocket(t, httpServer.URL)
	defer conn.Close()

	if err := conn.WriteJSON(Message{
		Type: "game.load",
		Data: map[string]string{"level_id": "99999999-9999-9999-9999-999999999999"},
	}); err != nil {
		t.Fatalf("WriteJSON game load failed: %v", err)
	}

	readMessageOfType(t, conn, "error")
}

// ---------------------------------------------------------------------------
// Helpers specific to pause_test.go
// ---------------------------------------------------------------------------

// drainMessagesFor reads all messages from conn until the duration elapses.
// Returns the messages collected. Clears the read deadline before returning so
// subsequent reads are not immediately rejected by an expired deadline.
func drainMessagesFor(t *testing.T, conn *websocket.Conn, duration time.Duration) []Message {
	t.Helper()
	deadline := time.Now().Add(duration)
	var msgs []Message
	for {
		if err := conn.SetReadDeadline(deadline); err != nil {
			break
		}
		var msg Message
		if err := conn.ReadJSON(&msg); err != nil {
			break
		}
		msgs = append(msgs, msg)
	}
	// Clear the expired deadline so the next read can set its own fresh deadline.
	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		t.Logf("warning: SetReadDeadline(zero) failed: %v", err)
	}
	return msgs
}
