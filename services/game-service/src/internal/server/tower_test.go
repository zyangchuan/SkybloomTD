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
	"skybloom/game-service/internal/mapgen"
	"skybloom/game-service/internal/repository"
)

func TestWebsocketPlaceTowerConsumesEssenceAndPersistsBird(t *testing.T) {
	levels := standardLevels()
	maps := smallMap()
	stats, err := gameobject.BirdStatsForType(gameobject.BirdTypeSparrow)
	if err != nil {
		t.Fatalf("load sparrow stats: %v", err)
	}
	startingEssence := stats.Cost + 10
	sessions := quietSession()
	sessions.state.Essence = startingEssence
	handler := NewWithGenerationCachesAndSessions(config.Config{}, levels, maps, nil, nil, nil, nil, sessions).Router()
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

	if err := conn.WriteJSON(Message{
		Type: "game.action.place_tower",
		Data: map[string]any{"bird_type": gameobject.BirdTypeSparrow, "x": 1, "y": 1},
	}); err != nil {
		t.Fatalf("WriteJSON place tower failed: %v", err)
	}

	accepted := readMessageOfType(t, conn, "game.action.accepted")
	body, err := json.Marshal(accepted.Data)
	if err != nil {
		t.Fatalf("Marshal accepted action failed: %v", err)
	}
	var acceptedData struct {
		Action string `json:"action"`
		BirdID string `json:"bird_id"`
		Bird   struct {
			Type     string              `json:"type"`
			Position gameobject.Position `json:"position"`
		} `json:"bird"`
	}
	if err := json.Unmarshal(body, &acceptedData); err != nil {
		t.Fatalf("Unmarshal accepted action failed: %v", err)
	}
	if acceptedData.Action != placeTowerAction {
		t.Fatalf("unexpected accepted action %q", acceptedData.Action)
	}
	if acceptedData.BirdID == "" {
		t.Fatal("expected accepted action to include bird_id")
	}
	if acceptedData.Bird.Type != gameobject.BirdTypeSparrow {
		t.Fatalf("unexpected bird type %q", acceptedData.Bird.Type)
	}
	if acceptedData.Bird.Position.X != 1 || acceptedData.Bird.Position.Y != 1 {
		t.Fatalf("unexpected bird position %+v", acceptedData.Bird.Position)
	}

	if sessions.economy.Essence != startingEssence-stats.Cost {
		t.Fatalf("unexpected persisted essence %d", sessions.economy.Essence)
	}
	if len(sessions.birds) != 1 {
		t.Fatalf("expected one persisted bird, got %d", len(sessions.birds))
	}
	if sessions.birds[0].Type != gameobject.BirdTypeSparrow {
		t.Fatalf("unexpected persisted bird type %q", sessions.birds[0].Type)
	}
}

func TestWebsocketPlaceTowerRejectsEnemyPath(t *testing.T) {
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
	maps := &fakeMapCache{
		cached: mapgen.GeneratedMap{
			Version:   mapgen.Version,
			Seed:      99,
			Width:     4,
			Height:    4,
			EnemyPath: []mapgen.PathTile{{X: 1, Y: 1, Kind: "straight"}},
		},
	}
	handler := NewWithGenerationCachesAndSessions(config.Config{}, levels, maps, nil, nil, nil, nil, &fakeGameSessionStore{}).Router()
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

	if err := conn.WriteJSON(Message{
		Type: "game.action.place_tower",
		Data: map[string]any{"bird_type": gameobject.BirdTypeSparrow, "x": 1, "y": 1},
	}); err != nil {
		t.Fatalf("WriteJSON place tower failed: %v", err)
	}

	rejected := readMessageOfType(t, conn, "game.action.rejected")
	body, err := json.Marshal(rejected.Data)
	if err != nil {
		t.Fatalf("Marshal rejected action failed: %v", err)
	}
	var rejectedData struct {
		Action string `json:"action"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal(body, &rejectedData); err != nil {
		t.Fatalf("Unmarshal rejected action failed: %v", err)
	}
	if rejectedData.Action != placeTowerAction {
		t.Fatalf("unexpected rejected action %q", rejectedData.Action)
	}
	if rejectedData.Error != "tower cannot be placed on the enemy path" {
		t.Fatalf("unexpected rejection error %q", rejectedData.Error)
	}
}
