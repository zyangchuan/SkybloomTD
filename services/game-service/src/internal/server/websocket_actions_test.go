package server

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"skybloom/game-service/internal/config"
	"skybloom/game-service/internal/gameobject"
	"skybloom/game-service/internal/gamesession"
	"skybloom/game-service/internal/mapgen"
	"skybloom/game-service/internal/quizcache"
	"skybloom/game-service/internal/repository"
)

// ---------------------------------------------------------------------------
// Helpers shared across tests in this file
// ---------------------------------------------------------------------------

// standardLevels returns a bootstrap with a level the fakeGameSessionStore expects.
func standardLevels() *fakeLevelRepository {
	return &fakeLevelRepository{
		bootstrap: repository.LevelBootstrap{
			LevelID:             "11111111-1111-1111-1111-111111111111",
			UserID:              "22222222-2222-2222-2222-222222222222",
			SubChapterID:        "55555555-5555-5555-5555-555555555555",
			GenerationID:        "generation-1",
			MapSeed:             12345,
			MapAlgorithmVersion: mapgen.Version,
		},
	}
}

// smallMap returns a 4×4 map with the enemy path on column 0 so towers can be
// placed safely at any (x≥1, y) position without hitting path or bounds issues.
func smallMap() *fakeMapCache {
	return &fakeMapCache{
		cached: mapgen.GeneratedMap{
			Version: mapgen.Version,
			Seed:    12345,
			Width:   4,
			Height:  4,
			EnemyPath: []mapgen.PathTile{
				{X: 0, Y: 0, Kind: "start"},
				{X: 0, Y: 1, Kind: "straight"},
				{X: 0, Y: 2, Kind: "straight"},
				{X: 0, Y: 3, Kind: "end"},
			},
		},
	}
}

// quietSession returns a store configured with a session that has no active waves
// and sufficient essence, so the game loop runs without spawning enemies during tests.
func quietSession() *fakeGameSessionStore {
	return &fakeGameSessionStore{
		state: gamesession.State{
			SessionID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			UserID:    "22222222-2222-2222-2222-222222222222",
			LevelID:   "11111111-1111-1111-1111-111111111111",
			Health:    gamesession.InitialHealth,
			Essence:   gamesession.InitialEssence,
		},
		nextWaveTick: 100000, // far enough that no enemies spawn during the test
	}
}

// startSession sends game.session.start and waits for game.session.started.
func startSession(t *testing.T, conn interface {
	WriteJSON(any) error
}) {
	t.Helper()
	if err := conn.WriteJSON(Message{
		Type: "game.session.start",
		Data: map[string]string{"level_id": "11111111-1111-1111-1111-111111111111"},
	}); err != nil {
		t.Fatalf("WriteJSON session start failed: %v", err)
	}
}

func storedBirdForTest(id string, birdType string, position gameobject.Position) gamesession.StoredBird {
	stats, err := gameobject.BirdStatsForType(birdType)
	if err != nil {
		panic(err)
	}
	return gamesession.StoredBird{
		ID:              id,
		Type:            birdType,
		Position:        position,
		Stats:           stats,
		LastFiredAtTick: -1,
	}
}

type acceptedActionForTest struct {
	Action         string   `json:"action"`
	BirdID         string   `json:"bird_id"`
	RemovedBirdIDs []string `json:"removed_bird_ids"`
	Bird           struct {
		ID       string              `json:"id"`
		Type     string              `json:"type"`
		Position gameobject.Position `json:"position"`
	} `json:"bird"`
}

func decodeAcceptedAction(t *testing.T, data any) acceptedActionForTest {
	t.Helper()
	body, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("Marshal accepted action failed: %v", err)
	}
	var accepted acceptedActionForTest
	if err := json.Unmarshal(body, &accepted); err != nil {
		t.Fatalf("Unmarshal accepted action failed: %v", err)
	}
	return accepted
}

// ---------------------------------------------------------------------------
// Ping / unknown message
// ---------------------------------------------------------------------------

func TestWebsocketPingReturnsPong(t *testing.T) {
	handler := New(config.Config{}, standardLevels(), &fakeMapCache{}).Router()
	httpServer := startHTTPServer(t, handler)
	defer httpServer.Close()

	conn := dialGameWebsocket(t, httpServer.URL)
	defer conn.Close()

	if err := conn.WriteJSON(Message{Type: "ping"}); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	msg := readMessageOfType(t, conn, "pong")
	if msg.Type != "pong" {
		t.Fatalf("expected pong, got %q", msg.Type)
	}
}

func TestWebsocketUnknownMessageTypeReturnsError(t *testing.T) {
	handler := New(config.Config{}, standardLevels(), &fakeMapCache{}).Router()
	httpServer := startHTTPServer(t, handler)
	defer httpServer.Close()

	conn := dialGameWebsocket(t, httpServer.URL)
	defer conn.Close()

	if err := conn.WriteJSON(Message{Type: "game.does_not_exist"}); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	errMsg := readMessageOfType(t, conn, "error")
	body, _ := json.Marshal(errMsg.Data)
	if !jsonContains(body, "unsupported message type") {
		t.Fatalf("expected 'unsupported message type' in error, got: %s", body)
	}
}

// ---------------------------------------------------------------------------
// Place tower edge cases
// ---------------------------------------------------------------------------

func TestWebsocketPlaceTowerNoSessionReturnsRejection(t *testing.T) {
	handler := New(config.Config{}, standardLevels(), &fakeMapCache{}).Router()
	httpServer := startHTTPServer(t, handler)
	defer httpServer.Close()

	conn := dialGameWebsocket(t, httpServer.URL)
	defer conn.Close()

	// Send place_tower without starting a session first.
	if err := conn.WriteJSON(Message{
		Type: "game.action.place_tower",
		Data: map[string]any{"bird_type": gameobject.BirdTypeSparrow, "x": 1, "y": 1},
	}); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	rejected := readMessageOfType(t, conn, "game.action.rejected")
	assertRejection(t, rejected, placeTowerAction, "game session is not running")
}

func TestWebsocketPlaceTowerUnknownBirdTypeReturnsRejection(t *testing.T) {
	sessions := quietSession()
	handler := NewWithGenerationCachesAndSessions(config.Config{}, standardLevels(), smallMap(), nil, nil, nil, nil, sessions).Router()
	httpServer := startHTTPServer(t, handler)
	defer httpServer.Close()

	conn := dialGameWebsocket(t, httpServer.URL)
	defer conn.Close()

	startSession(t, conn)
	readMessageOfType(t, conn, "game.session.started")

	if err := conn.WriteJSON(Message{
		Type: "game.action.place_tower",
		Data: map[string]any{"bird_type": "fire_dragon", "x": 2, "y": 2},
	}); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	rejected := readMessageOfType(t, conn, "game.action.rejected")
	assertRejection(t, rejected, placeTowerAction, "unknown bird type")
}

func TestWebsocketPlaceTowerOutsideBoundsReturnsRejection(t *testing.T) {
	sessions := quietSession()
	handler := NewWithGenerationCachesAndSessions(config.Config{}, standardLevels(), smallMap(), nil, nil, nil, nil, sessions).Router()
	httpServer := startHTTPServer(t, handler)
	defer httpServer.Close()

	conn := dialGameWebsocket(t, httpServer.URL)
	defer conn.Close()

	startSession(t, conn)
	readMessageOfType(t, conn, "game.session.started")

	// (10, 10) is outside the 4×4 map.
	if err := conn.WriteJSON(Message{
		Type: "game.action.place_tower",
		Data: map[string]any{"bird_type": gameobject.BirdTypeSparrow, "x": 10, "y": 10},
	}); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	rejected := readMessageOfType(t, conn, "game.action.rejected")
	assertRejection(t, rejected, placeTowerAction, "tower position is outside the map")
}

func TestWebsocketPlaceTowerOccupiedCellReturnsRejection(t *testing.T) {
	sessions := &fakeGameSessionStore{
		state: gamesession.State{
			SessionID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			UserID:    "22222222-2222-2222-2222-222222222222",
			LevelID:   "11111111-1111-1111-1111-111111111111",
			Health:    gamesession.InitialHealth,
			Essence:   gamesession.InitialEssence,
		},
		birds: []gamesession.StoredBird{
			{
				ID:       "existing-bird",
				Type:     gameobject.BirdTypeSparrow,
				Position: gameobject.Position{X: 2, Y: 2},
				Stats: gameobject.BirdStats{
					Damage:          10,
					ProjectileSpeed: gameobject.StandardProjectileSpeed,
					FireRate:        1,
					Range:           2.1,
					Cost:            50,
				},
			},
		},
		nextWaveTick: 100000,
	}
	handler := NewWithGenerationCachesAndSessions(config.Config{}, standardLevels(), smallMap(), nil, nil, nil, nil, sessions).Router()
	httpServer := startHTTPServer(t, handler)
	defer httpServer.Close()

	conn := dialGameWebsocket(t, httpServer.URL)
	defer conn.Close()

	startSession(t, conn)
	readMessageOfType(t, conn, "game.session.started")

	// Try to place a second bird at the already-occupied (2,2).
	if err := conn.WriteJSON(Message{
		Type: "game.action.place_tower",
		Data: map[string]any{"bird_type": gameobject.BirdTypeSparrow, "x": 2, "y": 2},
	}); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	rejected := readMessageOfType(t, conn, "game.action.rejected")
	assertRejection(t, rejected, placeTowerAction, "tower position is occupied")
}

func TestWebsocketPlaceTowerInsufficientEssenceReturnsRejection(t *testing.T) {
	// Sparrow costs 50 essence; set up the session with only 10.
	sessions := &fakeGameSessionStore{
		state: gamesession.State{
			SessionID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			UserID:    "22222222-2222-2222-2222-222222222222",
			LevelID:   "11111111-1111-1111-1111-111111111111",
			Health:    gamesession.InitialHealth,
			Essence:   10, // less than Sparrow cost (50)
		},
		nextWaveTick: 100000,
	}
	handler := NewWithGenerationCachesAndSessions(config.Config{}, standardLevels(), smallMap(), nil, nil, nil, nil, sessions).Router()
	httpServer := startHTTPServer(t, handler)
	defer httpServer.Close()

	conn := dialGameWebsocket(t, httpServer.URL)
	defer conn.Close()

	startSession(t, conn)
	readMessageOfType(t, conn, "game.session.started")

	if err := conn.WriteJSON(Message{
		Type: "game.action.place_tower",
		Data: map[string]any{"bird_type": gameobject.BirdTypeSparrow, "x": 2, "y": 2},
	}); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	rejected := readMessageOfType(t, conn, "game.action.rejected")
	assertRejection(t, rejected, placeTowerAction, "insufficient essence")
}

func TestWebsocketPlaceTowerHybridReturnsRejection(t *testing.T) {
	sessions := quietSession()
	handler := NewWithGenerationCachesAndSessions(config.Config{}, standardLevels(), smallMap(), nil, nil, nil, nil, sessions).Router()
	httpServer := startHTTPServer(t, handler)
	defer httpServer.Close()

	conn := dialGameWebsocket(t, httpServer.URL)
	defer conn.Close()

	startSession(t, conn)
	readMessageOfType(t, conn, "game.session.started")

	if err := conn.WriteJSON(Message{
		Type: "game.action.place_tower",
		Data: map[string]any{"bird_type": gameobject.BirdTypeFalcon, "x": 2, "y": 2},
	}); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	rejected := readMessageOfType(t, conn, "game.action.rejected")
	assertRejection(t, rejected, placeTowerAction, "hybrid birds must be created by merging")
}

func TestWebsocketMergeTowerRecipesSucceedInEitherOrder(t *testing.T) {
	cases := []struct {
		name       string
		sourceType string
		targetType string
		wantType   string
	}{
		{
			name:       "sparrow eagle creates falcon",
			sourceType: gameobject.BirdTypeSparrow,
			targetType: gameobject.BirdTypeEagle,
			wantType:   gameobject.BirdTypeFalcon,
		},
		{
			name:       "eagle sparrow creates falcon",
			sourceType: gameobject.BirdTypeEagle,
			targetType: gameobject.BirdTypeSparrow,
			wantType:   gameobject.BirdTypeFalcon,
		},
		{
			name:       "woodpecker peacock creates kingfisher",
			sourceType: gameobject.BirdTypeWoodpecker,
			targetType: gameobject.BirdTypePeacock,
			wantType:   gameobject.BirdTypeKingfisher,
		},
		{
			name:       "peacock woodpecker creates kingfisher",
			sourceType: gameobject.BirdTypePeacock,
			targetType: gameobject.BirdTypeWoodpecker,
			wantType:   gameobject.BirdTypeKingfisher,
		},
		{
			name:       "peacock eagle creates phoenix",
			sourceType: gameobject.BirdTypePeacock,
			targetType: gameobject.BirdTypeEagle,
			wantType:   gameobject.BirdTypePhoenix,
		},
		{
			name:       "eagle peacock creates phoenix",
			sourceType: gameobject.BirdTypeEagle,
			targetType: gameobject.BirdTypePeacock,
			wantType:   gameobject.BirdTypePhoenix,
		},
		{
			name:       "kingfisher eagle creates sun god",
			sourceType: gameobject.BirdTypeKingfisher,
			targetType: gameobject.BirdTypeEagle,
			wantType:   gameobject.BirdTypeSunGod,
		},
		{
			name:       "eagle kingfisher creates sun god",
			sourceType: gameobject.BirdTypeEagle,
			targetType: gameobject.BirdTypeKingfisher,
			wantType:   gameobject.BirdTypeSunGod,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sessions := quietSession()
			sessions.state.Essence = 500
			sessions.birds = []gamesession.StoredBird{
				storedBirdForTest("source-bird", tc.sourceType, gameobject.Position{X: 1, Y: 1}),
				storedBirdForTest("target-bird", tc.targetType, gameobject.Position{X: 2, Y: 2}),
			}
			handler := NewWithGenerationCachesAndSessions(config.Config{}, standardLevels(), smallMap(), nil, nil, nil, nil, sessions).Router()
			httpServer := startHTTPServer(t, handler)
			defer httpServer.Close()

			conn := dialGameWebsocket(t, httpServer.URL)
			defer conn.Close()

			startSession(t, conn)
			readMessageOfType(t, conn, "game.session.started")

			if err := conn.WriteJSON(Message{
				Type: "game.action.merge_tower",
				Data: map[string]any{"source_bird_id": "source-bird", "target_bird_id": "target-bird"},
			}); err != nil {
				t.Fatalf("WriteJSON failed: %v", err)
			}

			accepted := readMessageOfType(t, conn, "game.action.accepted")
			acceptedData := decodeAcceptedAction(t, accepted.Data)
			if acceptedData.Action != mergeTowerAction {
				t.Fatalf("unexpected accepted action %q", acceptedData.Action)
			}
			if acceptedData.Bird.Type != tc.wantType {
				t.Fatalf("expected merged bird type %q, got %q", tc.wantType, acceptedData.Bird.Type)
			}
			if acceptedData.Bird.Position.X != 2 || acceptedData.Bird.Position.Y != 2 {
				t.Fatalf("expected merged bird at target position, got %+v", acceptedData.Bird.Position)
			}
			if len(acceptedData.RemovedBirdIDs) != 2 {
				t.Fatalf("expected two removed bird ids, got %v", acceptedData.RemovedBirdIDs)
			}
			hybridStats, err := gameobject.BirdStatsForType(tc.wantType)
			if err != nil {
				t.Fatalf("load hybrid stats: %v", err)
			}
			if sessions.economy.Essence != 500-hybridStats.Cost {
				t.Fatalf("expected merge to consume %d essence, got %d", hybridStats.Cost, sessions.economy.Essence)
			}
			if len(sessions.birds) != 1 {
				t.Fatalf("expected one persisted bird after merge, got %d", len(sessions.birds))
			}
			if sessions.birds[0].Type != tc.wantType {
				t.Fatalf("expected persisted bird type %q, got %q", tc.wantType, sessions.birds[0].Type)
			}
			if sessions.birds[0].ID == "source-bird" || sessions.birds[0].ID == "target-bird" {
				t.Fatalf("expected new hybrid bird id, got %q", sessions.birds[0].ID)
			}
			if sessions.birds[0].Position.X != 2 || sessions.birds[0].Position.Y != 2 {
				t.Fatalf("expected persisted hybrid at target position, got %+v", sessions.birds[0].Position)
			}
		})
	}
}

func TestWebsocketMergeTowerBoughtSourceConsumesBaseAndUpgradeEssenceAndRemovesTarget(t *testing.T) {
	sourceStats, err := gameobject.BirdStatsForType(gameobject.BirdTypeEagle)
	if err != nil {
		t.Fatalf("load eagle stats: %v", err)
	}
	hybridStats, err := gameobject.BirdStatsForType(gameobject.BirdTypeFalcon)
	if err != nil {
		t.Fatalf("load falcon stats: %v", err)
	}
	sessions := quietSession()
	sessions.state.Essence = sourceStats.Cost + hybridStats.Cost + 20
	sessions.birds = []gamesession.StoredBird{
		storedBirdForTest("target-bird", gameobject.BirdTypeSparrow, gameobject.Position{X: 2, Y: 2}),
	}
	handler := NewWithGenerationCachesAndSessions(config.Config{}, standardLevels(), smallMap(), nil, nil, nil, nil, sessions).Router()
	httpServer := startHTTPServer(t, handler)
	defer httpServer.Close()

	conn := dialGameWebsocket(t, httpServer.URL)
	defer conn.Close()

	startSession(t, conn)
	readMessageOfType(t, conn, "game.session.started")

	if err := conn.WriteJSON(Message{
		Type: "game.action.merge_tower",
		Data: map[string]any{"source_bird_type": gameobject.BirdTypeEagle, "target_bird_id": "target-bird"},
	}); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	accepted := readMessageOfType(t, conn, "game.action.accepted")
	acceptedData := decodeAcceptedAction(t, accepted.Data)
	if acceptedData.Action != mergeTowerAction {
		t.Fatalf("unexpected accepted action %q", acceptedData.Action)
	}
	if acceptedData.Bird.Type != gameobject.BirdTypeFalcon {
		t.Fatalf("expected falcon, got %q", acceptedData.Bird.Type)
	}
	if len(acceptedData.RemovedBirdIDs) != 1 || acceptedData.RemovedBirdIDs[0] != "target-bird" {
		t.Fatalf("expected target bird removal, got %v", acceptedData.RemovedBirdIDs)
	}
	if sessions.economy.Essence != 20 {
		t.Fatalf("expected essence 20 after buying source bird and upgrade, got %d", sessions.economy.Essence)
	}
	if len(sessions.birds) != 1 || sessions.birds[0].Type != gameobject.BirdTypeFalcon {
		t.Fatalf("expected one persisted falcon, got %+v", sessions.birds)
	}
	if sessions.birds[0].Position.X != 2 || sessions.birds[0].Position.Y != 2 {
		t.Fatalf("expected hybrid at target position, got %+v", sessions.birds[0].Position)
	}
}

func TestWebsocketMergeTowerBoughtSourceRejectsWithOnlyUpgradeEssence(t *testing.T) {
	hybridStats, err := gameobject.BirdStatsForType(gameobject.BirdTypeFalcon)
	if err != nil {
		t.Fatalf("load falcon stats: %v", err)
	}
	sessions := quietSession()
	sessions.state.Essence = hybridStats.Cost
	sessions.birds = []gamesession.StoredBird{
		storedBirdForTest("target-bird", gameobject.BirdTypeSparrow, gameobject.Position{X: 2, Y: 2}),
	}
	handler := NewWithGenerationCachesAndSessions(config.Config{}, standardLevels(), smallMap(), nil, nil, nil, nil, sessions).Router()
	httpServer := startHTTPServer(t, handler)
	defer httpServer.Close()

	conn := dialGameWebsocket(t, httpServer.URL)
	defer conn.Close()

	startSession(t, conn)
	readMessageOfType(t, conn, "game.session.started")

	if err := conn.WriteJSON(Message{
		Type: "game.action.merge_tower",
		Data: map[string]any{"source_bird_type": gameobject.BirdTypeEagle, "target_bird_id": "target-bird"},
	}); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	rejected := readMessageOfType(t, conn, "game.action.rejected")
	assertRejection(t, rejected, mergeTowerAction, "insufficient essence")
}

func TestWebsocketMergeTowerRejections(t *testing.T) {
	cases := []struct {
		name      string
		birds     []gamesession.StoredBird
		data      map[string]any
		wantError string
	}{
		{
			name:      "unknown source",
			birds:     []gamesession.StoredBird{storedBirdForTest("target-bird", gameobject.BirdTypeEagle, gameobject.Position{X: 2, Y: 2})},
			data:      map[string]any{"source_bird_id": "missing-bird", "target_bird_id": "target-bird"},
			wantError: "source bird not found",
		},
		{
			name:      "unknown target",
			birds:     []gamesession.StoredBird{storedBirdForTest("source-bird", gameobject.BirdTypeSparrow, gameobject.Position{X: 1, Y: 1})},
			data:      map[string]any{"source_bird_id": "source-bird", "target_bird_id": "missing-bird"},
			wantError: "target bird not found",
		},
		{
			name: "same bird",
			birds: []gamesession.StoredBird{
				storedBirdForTest("same-bird", gameobject.BirdTypeSparrow, gameobject.Position{X: 1, Y: 1}),
			},
			data:      map[string]any{"source_bird_id": "same-bird", "target_bird_id": "same-bird"},
			wantError: "source and target birds must be different",
		},
		{
			name: "invalid recipe",
			birds: []gamesession.StoredBird{
				storedBirdForTest("source-bird", gameobject.BirdTypeSparrow, gameobject.Position{X: 1, Y: 1}),
				storedBirdForTest("target-bird", gameobject.BirdTypeWoodpecker, gameobject.Position{X: 2, Y: 2}),
			},
			data:      map[string]any{"source_bird_id": "source-bird", "target_bird_id": "target-bird"},
			wantError: "bird types cannot be merged",
		},
		{
			name: "existing towers insufficient essence",
			birds: []gamesession.StoredBird{
				storedBirdForTest("source-bird", gameobject.BirdTypeSparrow, gameobject.Position{X: 1, Y: 1}),
				storedBirdForTest("target-bird", gameobject.BirdTypeEagle, gameobject.Position{X: 2, Y: 2}),
			},
			data:      map[string]any{"source_bird_id": "source-bird", "target_bird_id": "target-bird"},
			wantError: "insufficient essence",
		},
		{
			name: "bought source insufficient essence",
			birds: []gamesession.StoredBird{
				storedBirdForTest("target-bird", gameobject.BirdTypeSparrow, gameobject.Position{X: 2, Y: 2}),
			},
			data:      map[string]any{"source_bird_type": gameobject.BirdTypeEagle, "target_bird_id": "target-bird"},
			wantError: "insufficient essence",
		},
		{
			name: "bought source hybrid rejected",
			birds: []gamesession.StoredBird{
				storedBirdForTest("target-bird", gameobject.BirdTypeEagle, gameobject.Position{X: 2, Y: 2}),
			},
			data:      map[string]any{"source_bird_type": gameobject.BirdTypeKingfisher, "target_bird_id": "target-bird"},
			wantError: "hybrid birds must be created by merging",
		},
		{
			name: "both source fields rejected",
			birds: []gamesession.StoredBird{
				storedBirdForTest("source-bird", gameobject.BirdTypeSparrow, gameobject.Position{X: 1, Y: 1}),
				storedBirdForTest("target-bird", gameobject.BirdTypeEagle, gameobject.Position{X: 2, Y: 2}),
			},
			data:      map[string]any{"source_bird_id": "source-bird", "source_bird_type": gameobject.BirdTypeSparrow, "target_bird_id": "target-bird"},
			wantError: "merge must include exactly one of source_bird_id or source_bird_type",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sessions := quietSession()
			sessions.birds = tc.birds
			if tc.name == "existing towers insufficient essence" {
				sessions.state.Essence = 49
			}
			if tc.name == "bought source insufficient essence" {
				sessions.state.Essence = 10
			}
			handler := NewWithGenerationCachesAndSessions(config.Config{}, standardLevels(), smallMap(), nil, nil, nil, nil, sessions).Router()
			httpServer := startHTTPServer(t, handler)
			defer httpServer.Close()

			conn := dialGameWebsocket(t, httpServer.URL)
			defer conn.Close()

			startSession(t, conn)
			readMessageOfType(t, conn, "game.session.started")

			if err := conn.WriteJSON(Message{
				Type: "game.action.merge_tower",
				Data: tc.data,
			}); err != nil {
				t.Fatalf("WriteJSON failed: %v", err)
			}

			rejected := readMessageOfType(t, conn, "game.action.rejected")
			assertRejection(t, rejected, mergeTowerAction, tc.wantError)
		})
	}
}

func TestPlacedBirdsFromStoredRestoresHybridAttackBehaviour(t *testing.T) {
	birds, err := placedBirdsFromStored([]gamesession.StoredBird{
		storedBirdForTest("phoenix-bird", gameobject.BirdTypePhoenix, gameobject.Position{X: 2, Y: 2}),
		storedBirdForTest("falcon-bird", gameobject.BirdTypeFalcon, gameobject.Position{X: 1, Y: 1}),
	})
	if err != nil {
		t.Fatalf("placedBirdsFromStored failed: %v", err)
	}
	if len(birds) != 2 {
		t.Fatalf("expected two restored birds, got %d", len(birds))
	}
	if _, ok := birds[0].bird.AttackBehaviour.(gameobject.SplashAttack); !ok {
		t.Fatalf("expected phoenix to restore splash attack, got %T", birds[0].bird.AttackBehaviour)
	}
	if _, ok := birds[1].bird.AttackBehaviour.(gameobject.SingleAttack); !ok {
		t.Fatalf("expected falcon to restore single attack, got %T", birds[1].bird.AttackBehaviour)
	}
}

// ---------------------------------------------------------------------------
// Quiz edge cases
// ---------------------------------------------------------------------------

func TestWebsocketQuizRequestWithoutSessionReturnsUnavailable(t *testing.T) {
	handler := New(config.Config{}, standardLevels(), &fakeMapCache{}).Router()
	httpServer := startHTTPServer(t, handler)
	defer httpServer.Close()

	conn := dialGameWebsocket(t, httpServer.URL)
	defer conn.Close()

	if err := conn.WriteJSON(Message{Type: "game.quiz.request"}); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	unavailable := readMessageOfType(t, conn, "game.quiz.unavailable")
	body, err := json.Marshal(unavailable.Data)
	if err != nil {
		t.Fatalf("Marshal quiz unavailable failed: %v", err)
	}
	var state QuizUnavailableState
	if err := json.Unmarshal(body, &state); err != nil {
		t.Fatalf("Unmarshal quiz unavailable failed: %v", err)
	}
	if state.Reason != "game_not_started" {
		t.Fatalf("expected game_not_started, got %+v", state)
	}
}

func TestWebsocketQuizAnswerWithoutSessionReturnsError(t *testing.T) {
	handler := New(config.Config{}, standardLevels(), &fakeMapCache{}).Router()
	httpServer := startHTTPServer(t, handler)
	defer httpServer.Close()

	conn := dialGameWebsocket(t, httpServer.URL)
	defer conn.Close()

	if err := conn.WriteJSON(Message{
		Type: "game.quiz.answer",
		Data: map[string]any{"quiz_id": "66666666-6666-6666-6666-666666666666", "selected_index": 0},
	}); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	errMsg := readMessageOfType(t, conn, "error")
	body, _ := json.Marshal(errMsg.Data)
	if !jsonContains(body, "game session is not running") {
		t.Fatalf("expected 'game session is not running' error, got: %s", body)
	}
}

func TestWebsocketQuizAnswerWrongQuizIDReturnsError(t *testing.T) {
	// Use a fresh session (no pre-configured SessionID) so Start() populates
	// session.GenerationID from the level bootstrap options. loop.generationID
	// is derived from session.GenerationID, which must match the quiz cache.
	sessions := &fakeGameSessionStore{}
	quizzes := &fakeQuizCache{
		quizzes: quizcache.LevelQuizzes{
			GenerationID: "generation-1",
			LevelID:      "11111111-1111-1111-1111-111111111111",
			UserID:       "22222222-2222-2222-2222-222222222222",
			SubChapterID: "55555555-5555-5555-5555-555555555555",
			Quizzes: []quizcache.CachedQuiz{
				{
					ID:               "66666666-6666-6666-6666-666666666666",
					QuizIndex:        0,
					QuizType:         "true_false",
					QuestionMarkdown: "The sky is blue?",
					OptionsMarkdown:  []string{"True", "False"},
					AnswerIndex:      0,
				},
			},
		},
	}
	handler := NewWithGenerationCachesAndSessions(config.Config{}, standardLevels(), nil, quizzes, nil, nil, nil, sessions).Router()
	httpServer := startHTTPServer(t, handler)
	defer httpServer.Close()

	conn := dialGameWebsocket(t, httpServer.URL)
	defer conn.Close()

	startSession(t, conn)
	readMessageOfType(t, conn, "game.session.started")

	// Send the correct UUID format but a different ID than the current quiz.
	if err := conn.WriteJSON(Message{
		Type: "game.quiz.answer",
		Data: map[string]any{
			"quiz_id":        "77777777-7777-7777-7777-777777777777", // wrong ID
			"selected_index": 0,
		},
	}); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	errMsg := readMessageOfType(t, conn, "error")
	body, _ := json.Marshal(errMsg.Data)
	if !jsonContains(body, "quiz_id is not the current quiz") {
		t.Fatalf("expected 'quiz_id is not the current quiz', got: %s", body)
	}
}

func TestWebsocketQuizAnswerOutOfRangeIndexReturnsError(t *testing.T) {
	sessions := &fakeGameSessionStore{}
	quizzes := &fakeQuizCache{
		quizzes: quizcache.LevelQuizzes{
			GenerationID: "generation-1",
			LevelID:      "11111111-1111-1111-1111-111111111111",
			UserID:       "22222222-2222-2222-2222-222222222222",
			SubChapterID: "55555555-5555-5555-5555-555555555555",
			Quizzes: []quizcache.CachedQuiz{
				{
					ID:               "66666666-6666-6666-6666-666666666666",
					QuizIndex:        0,
					QuizType:         "true_false",
					QuestionMarkdown: "The sky is blue?",
					OptionsMarkdown:  []string{"True", "False"}, // 2 options: valid range [0,1]
					AnswerIndex:      0,
				},
			},
		},
	}
	handler := NewWithGenerationCachesAndSessions(config.Config{}, standardLevels(), nil, quizzes, nil, nil, nil, sessions).Router()
	httpServer := startHTTPServer(t, handler)
	defer httpServer.Close()

	conn := dialGameWebsocket(t, httpServer.URL)
	defer conn.Close()

	startSession(t, conn)
	readMessageOfType(t, conn, "game.session.started")

	// selected_index=5 is beyond the 2 available options.
	if err := conn.WriteJSON(Message{
		Type: "game.quiz.answer",
		Data: map[string]any{
			"quiz_id":        "66666666-6666-6666-6666-666666666666",
			"selected_index": 5,
		},
	}); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	errMsg := readMessageOfType(t, conn, "error")
	body, _ := json.Marshal(errMsg.Data)
	if !jsonContains(body, "selected_index is out of range") {
		t.Fatalf("expected 'selected_index is out of range', got: %s", body)
	}
}

// ---------------------------------------------------------------------------
// Unit tests: advanceRuntimeTick / game loop functions
// ---------------------------------------------------------------------------

func TestAdvanceRuntimeTickMultipleEnemiesEscapeHealthClampsAtZero(t *testing.T) {
	// baseHealthDamage=10; start at Health=5 so the first escape clamps to 0
	// and subsequent escapes keep it at 0 (not negative).
	path := []gameobject.Position{{X: 0, Y: 0}, {X: 1, Y: 0}}
	runtime := runtimeSession{
		session: gamesession.State{
			SessionID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			Health:    5, // less than baseHealthDamage(10)
		},
		economy:     gamesession.NewEconomy(100),
		loopStarted: true,
		path:        path,
		// Three enemies at the very end of the path — all escape on the next tick.
		enemies: []gameobject.Enemy{
			{ID: "enemy-1", Health: 30, Speed: 0, Position: gameobject.Position{X: 1, Y: 0}, PathIndex: 1},
			{ID: "enemy-2", Health: 30, Speed: 0, Position: gameobject.Position{X: 1, Y: 0}, PathIndex: 1},
			{ID: "enemy-3", Health: 30, Speed: 0, Position: gameobject.Position{X: 1, Y: 0}, PathIndex: 1},
		},
	}

	events := advanceRuntimeTick(&runtime, time.Now())

	if runtime.session.Health != 0 {
		t.Fatalf("expected health clamped to 0, got %d", runtime.session.Health)
	}
	if len(runtime.enemies) != 0 {
		t.Fatalf("expected all escaped enemies removed, got %d", len(runtime.enemies))
	}
	escapedCount := 0
	for _, e := range events {
		if e.Type == "enemy.escaped" {
			escapedCount++
		}
	}
	if escapedCount != 3 {
		t.Fatalf("expected 3 enemy.escaped events, got %d", escapedCount)
	}
}

func TestFireBirdsReturnsNilWhenHealthIsZero(t *testing.T) {
	bird, err := gameobject.NewBird("bird-1", gameobject.BirdTypeSparrow, gameobject.Position{X: 0, Y: 0})
	if err != nil {
		t.Fatalf("NewBird failed: %v", err)
	}
	runtime := runtimeSession{
		session: gamesession.State{
			SessionID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			Health:    0, // game over
			Tick:      5,
		},
		economy:     gamesession.NewEconomy(100),
		loopStarted: true,
		birds:       []placedBird{{birdType: gameobject.BirdTypeSparrow, bird: bird}},
		enemies: []gameobject.Enemy{
			{ID: "enemy-1", Health: 30, Position: gameobject.Position{X: 0.2, Y: 0}},
		},
	}

	events := fireBirds(&runtime)

	if len(events) != 0 {
		t.Fatalf("expected no bird attack events when health is 0, got %d", len(events))
	}
}

func TestTargetEnemyIndexPrioritizesHighestPathIndex(t *testing.T) {
	// Sparrow range=2.1; bird is at origin. All three enemies are within range.
	// The one with the highest PathIndex should be selected.
	bird, err := gameobject.NewBird("bird-1", gameobject.BirdTypeSparrow, gameobject.Position{X: 0, Y: 0})
	if err != nil {
		t.Fatalf("NewBird failed: %v", err)
	}

	enemies := []gameobject.Enemy{
		{ID: "near-start", Health: 10, Position: gameobject.Position{X: 1, Y: 0}, PathIndex: 1},
		{ID: "far-ahead", Health: 10, Position: gameobject.Position{X: 2, Y: 0}, PathIndex: 5},
		{ID: "closest", Health: 10, Position: gameobject.Position{X: 0.5, Y: 0}, PathIndex: 0},
	}

	idx := targetEnemyIndex(bird, enemies)

	if idx < 0 {
		t.Fatal("expected a valid target index, got -1 (no target found)")
	}
	if enemies[idx].ID != "far-ahead" {
		t.Fatalf("expected target 'far-ahead' (PathIndex=5), got %q (PathIndex=%d)",
			enemies[idx].ID, enemies[idx].PathIndex)
	}
}

func TestTargetEnemyIndexReturnsNegativeOneWhenNoEnemiesInRange(t *testing.T) {
	bird, err := gameobject.NewBird("bird-1", gameobject.BirdTypeSparrow, gameobject.Position{X: 0, Y: 0})
	if err != nil {
		t.Fatalf("NewBird failed: %v", err)
	}

	// All enemies are far beyond the Sparrow's 2.1 range.
	enemies := []gameobject.Enemy{
		{ID: "too-far", Health: 10, Position: gameobject.Position{X: 10, Y: 0}, PathIndex: 9},
	}

	idx := targetEnemyIndex(bird, enemies)
	if idx != -1 {
		t.Fatalf("expected -1 when no enemies in range, got %d", idx)
	}
}

func TestGameWonReturnsFalseWhenHealthIsZero(t *testing.T) {
	// Even if all waves are cleared, a dead game is not a victory.
	runtime := runtimeSession{
		session:      gamesession.State{Health: 0, Wave: len(waveDefinitions())},
		waveSpawned:  100, // more than enough
		nextWaveTick: 0,
	}

	if gameWon(runtime) {
		t.Fatal("expected gameWon false when health is 0")
	}
}

func TestGameWonReturnsFalseWhenEnemiesAreStillAlive(t *testing.T) {
	runtime := runtimeSession{
		session:      gamesession.State{Health: gamesession.InitialHealth, Wave: len(waveDefinitions())},
		waveSpawned:  100,
		nextWaveTick: 0,
		enemies:      []gameobject.Enemy{{ID: "last-one", Health: 1}},
	}

	if gameWon(runtime) {
		t.Fatal("expected gameWon false when enemies are still alive")
	}
}

func TestGameWonReturnsTrueWhenAllConditionsMet(t *testing.T) {
	waves := waveDefinitions()
	finalWave := waves[len(waves)-1]

	runtime := runtimeSession{
		session:      gamesession.State{Health: gamesession.InitialHealth, Wave: len(waves)},
		waveSpawned:  finalWave.Count(),
		nextWaveTick: 0,
		enemies:      nil,
	}

	if !gameWon(runtime) {
		t.Fatal("expected gameWon true when all conditions are met")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// assertRejection verifies a game.action.rejected message has the expected
// action name and error string.
func assertRejection(t *testing.T, msg Message, wantAction, wantError string) {
	t.Helper()
	body, err := json.Marshal(msg.Data)
	if err != nil {
		t.Fatalf("Marshal rejected data failed: %v", err)
	}
	var data struct {
		Action string `json:"action"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		t.Fatalf("Unmarshal rejected data failed: %v", err)
	}
	if data.Action != wantAction {
		t.Fatalf("expected action %q, got %q", wantAction, data.Action)
	}
	if data.Error != wantError {
		t.Fatalf("expected error %q, got %q", wantError, data.Error)
	}
}

// jsonContains reports whether the raw JSON bytes contain the given substring.
func jsonContains(body []byte, substr string) bool {
	return strings.Contains(string(body), substr)
}
