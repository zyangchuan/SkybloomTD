package server

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"skybloom/game-service/internal/config"
	"skybloom/game-service/internal/gamesession"
	"skybloom/game-service/internal/generation"
	"skybloom/game-service/internal/mapcache"
	"skybloom/game-service/internal/mapgen"
	"skybloom/game-service/internal/models"
	"skybloom/game-service/internal/quizcache"
	"skybloom/game-service/internal/repository"
)

var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

const gameTickInterval = 50 * time.Millisecond

type LevelRepository interface {
	GetBootstrap(ctx context.Context, levelID string, userID string) (repository.LevelBootstrap, error)
	Ping(ctx context.Context) error
}

type GenerationRepository interface {
	GetGeneration(ctx context.Context, generationID string) (models.LevelGenerationRecord, error)
}

type GenerationStarter interface {
	Start(ctx context.Context, userID string, subChapterID string) (generation.StartResult, error)
}

type GenerationStatusStore interface {
	Get(ctx context.Context, generationID string) (models.GenerationStatus, error)
}

type MapCache interface {
	Get(ctx context.Context, version int, generationID string) (mapgen.GeneratedMap, error)
	Set(ctx context.Context, generationID string, levelMap mapgen.GeneratedMap) error
}

type QuizCache interface {
	Set(ctx context.Context, generationID string, quizzes quizcache.LevelQuizzes) error
}

type GameSessionStore interface {
	Start(ctx context.Context, options gamesession.StartOptions) (gamesession.State, error)
}

type Server struct {
	config   config.Config
	levels   LevelRepository
	jobs     GenerationRepository
	starter  GenerationStarter
	statuses GenerationStatusStore
	maps     MapCache
	quizzes  QuizCache
	sessions GameSessionStore
	upgrader websocket.Upgrader
}

type Message struct {
	Type string `json:"type"`
	Data any    `json:"data,omitempty"`
}

type InitialState struct {
	Map mapgen.GeneratedMap `json:"map"`
}

type GameState struct {
	SessionID  string    `json:"session_id"`
	LevelID    string    `json:"level_id"`
	Health     int       `json:"health"`
	Essence    int       `json:"essence"`
	Wave       int       `json:"wave"`
	Tick       int64     `json:"tick"`
	ServerTime time.Time `json:"server_time"`
}

func New(cfg config.Config, levels LevelRepository, maps MapCache) *Server {
	return NewWithGeneration(cfg, levels, maps, nil, nil, nil)
}

func NewWithGeneration(
	cfg config.Config,
	levels LevelRepository,
	maps MapCache,
	jobs GenerationRepository,
	starter GenerationStarter,
	statuses GenerationStatusStore,
) *Server {
	return NewWithGenerationAndCaches(cfg, levels, maps, nil, jobs, starter, statuses)
}

func NewWithGenerationAndCaches(
	cfg config.Config,
	levels LevelRepository,
	maps MapCache,
	quizzes QuizCache,
	jobs GenerationRepository,
	starter GenerationStarter,
	statuses GenerationStatusStore,
) *Server {
	return NewWithGenerationCachesAndSessions(cfg, levels, maps, quizzes, jobs, starter, statuses, nil)
}

func NewWithGenerationCachesAndSessions(
	cfg config.Config,
	levels LevelRepository,
	maps MapCache,
	quizzes QuizCache,
	jobs GenerationRepository,
	starter GenerationStarter,
	statuses GenerationStatusStore,
	sessions GameSessionStore,
) *Server {
	server := &Server{
		config:   cfg,
		levels:   levels,
		jobs:     jobs,
		starter:  starter,
		statuses: statuses,
		maps:     maps,
		quizzes:  quizzes,
		sessions: sessions,
	}
	server.upgrader = websocket.Upgrader{
		CheckOrigin: server.checkOrigin,
	}
	return server
}

func (s *Server) Router() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.health)
	mux.HandleFunc("/ready", s.ready)
	mux.HandleFunc("/ws", s.websocket)
	mux.HandleFunc("/level-generation/", s.generationStatus)
	return mux
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := s.levels.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) websocket(w http.ResponseWriter, r *http.Request) {
	userID := s.authenticatedUserID(r)
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	var writeMu sync.Mutex
	s.readLoop(r.Context(), conn, &writeMu, userID)
}

func (s *Server) loadMap(ctx context.Context, level repository.LevelBootstrap) (mapgen.GeneratedMap, error) {
	if s.maps != nil {
		levelMap, err := s.maps.Get(ctx, level.MapAlgorithmVersion, level.GenerationID)
		if err == nil {
			return levelMap, nil
		}
		if !errors.Is(err, mapcache.ErrMapNotFound) {
			return mapgen.GeneratedMap{}, err
		}
	}

	levelMap, err := mapgen.Generate(level.MapSeed, level.MapAlgorithmVersion)
	if err != nil {
		return mapgen.GeneratedMap{}, err
	}
	if s.maps != nil {
		if err := s.maps.Set(ctx, level.GenerationID, levelMap); err != nil {
			log.Printf("level map cache write failed generation_id=%s: %v", level.GenerationID, err)
		}
	}
	return levelMap, nil
}

func (s *Server) cacheQuizzes(ctx context.Context, level repository.LevelBootstrap) error {
	if s.quizzes == nil {
		return nil
	}
	if len(level.Quizzes) == 0 {
		return errors.New("level has no quizzes")
	}
	return s.quizzes.Set(ctx, level.GenerationID, quizcache.FromLevelBootstrap(level, level.GenerationID))
}

func (s *Server) readLoop(ctx context.Context, conn *websocket.Conn, writeMu *sync.Mutex, userID string) {
	conn.SetReadLimit(8192)
	var stopGameLoop context.CancelFunc
	defer func() {
		if stopGameLoop != nil {
			stopGameLoop()
		}
	}()
	for {
		var message Message
		if err := conn.ReadJSON(&message); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("websocket read failed: %v", err)
			}
			return
		}

		switch message.Type {
		case "game.start":
			if err := s.handleStart(ctx, conn, writeMu, userID, message.Data); err != nil {
				log.Printf("game.start failed user_id=%s: %v", userID, err)
				if writeErr := writeWebsocketJSON(conn, writeMu, Message{Type: "error", Data: map[string]string{"error": err.Error()}}); writeErr != nil {
					log.Printf("websocket error write failed: %v", writeErr)
					return
				}
			}
		case "game.load":
			if err := s.handleLoad(ctx, conn, writeMu, userID, message.Data); err != nil {
				log.Printf("game.load failed user_id=%s: %v", userID, err)
				if writeErr := writeWebsocketJSON(conn, writeMu, Message{Type: "error", Data: map[string]string{"error": err.Error()}}); writeErr != nil {
					log.Printf("websocket error write failed: %v", writeErr)
					return
				}
			}
		case "game.session.start":
			if stopGameLoop != nil {
				stopGameLoop()
				stopGameLoop = nil
			}
			stop, err := s.handleSessionStart(ctx, conn, writeMu, userID, message.Data)
			if err != nil {
				log.Printf("game.session.start failed user_id=%s: %v", userID, err)
				if writeErr := writeWebsocketJSON(conn, writeMu, Message{Type: "error", Data: map[string]string{"error": err.Error()}}); writeErr != nil {
					log.Printf("websocket error write failed: %v", writeErr)
					return
				}
				continue
			}
			stopGameLoop = stop
		case "ping":
			if err := writeWebsocketJSON(conn, writeMu, Message{Type: "pong"}); err != nil {
				log.Printf("websocket pong write failed: %v", err)
				return
			}
		default:
			if err := writeWebsocketJSON(conn, writeMu, Message{Type: "error", Data: map[string]string{"error": "unsupported message type"}}); err != nil {
				log.Printf("websocket error write failed: %v", err)
				return
			}
		}
	}
}

func (s *Server) handleStart(ctx context.Context, conn *websocket.Conn, writeMu *sync.Mutex, userID string, data any) error {
	if s.starter == nil {
		return errors.New("level generation is not configured")
	}
	var request struct {
		SubChapterID string `json:"sub_chapter_id"`
	}
	if err := decodeMessageData(data, &request); err != nil {
		return errors.New("game.start data must include sub_chapter_id")
	}
	subChapterID := strings.TrimSpace(request.SubChapterID)
	if !uuidPattern.MatchString(subChapterID) {
		return errors.New("sub_chapter_id must be a valid UUID")
	}

	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	result, err := s.starter.Start(callCtx, userID, subChapterID)
	if err != nil {
		return err
	}
	return writeWebsocketJSON(conn, writeMu, Message{Type: "level_generation.started", Data: result})
}

func (s *Server) handleLoad(ctx context.Context, conn *websocket.Conn, writeMu *sync.Mutex, userID string, data any) error {
	var request struct {
		LevelID string `json:"level_id"`
	}
	if err := decodeMessageData(data, &request); err != nil {
		return errors.New("game.load data must include level_id")
	}
	levelID := strings.TrimSpace(request.LevelID)
	if !uuidPattern.MatchString(levelID) {
		return errors.New("level_id must be a valid UUID")
	}
	return s.writeInitialState(ctx, conn, writeMu, userID, levelID)
}

func (s *Server) writeInitialState(ctx context.Context, conn *websocket.Conn, writeMu *sync.Mutex, userID string, levelID string) error {
	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	level, err := s.levels.GetBootstrap(callCtx, levelID, userID)
	if errors.Is(err, models.ErrLevelNotFound) {
		return errors.New("level not found")
	}
	if err != nil {
		log.Printf("level bootstrap lookup failed level_id=%s user_id=%s: %v", levelID, userID, err)
		return errors.New("failed to load level")
	}

	levelMap, err := s.loadMap(callCtx, level)
	if err != nil {
		log.Printf("level map load failed level_id=%s generation_id=%s: %v", levelID, level.GenerationID, err)
		return errors.New("failed to load level map")
	}
	if err := s.cacheQuizzes(callCtx, level); err != nil {
		log.Printf("level quiz cache write failed level_id=%s generation_id=%s: %v", levelID, level.GenerationID, err)
		return errors.New("failed to load level quizzes")
	}

	initialState := InitialState{
		Map: levelMap,
	}
	return writeWebsocketJSON(conn, writeMu, Message{Type: "game.initial_state", Data: initialState})
}

func (s *Server) handleSessionStart(ctx context.Context, conn *websocket.Conn, writeMu *sync.Mutex, userID string, data any) (context.CancelFunc, error) {
	if s.sessions == nil {
		return nil, errors.New("game session store is not configured")
	}
	var request struct {
		LevelID string `json:"level_id"`
	}
	if err := decodeMessageData(data, &request); err != nil {
		return nil, errors.New("game.session.start data must include level_id")
	}
	levelID := strings.TrimSpace(request.LevelID)
	if !uuidPattern.MatchString(levelID) {
		return nil, errors.New("level_id must be a valid UUID")
	}

	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	level, err := s.levels.GetBootstrap(callCtx, levelID, userID)
	if errors.Is(err, models.ErrLevelNotFound) {
		return nil, errors.New("level not found")
	}
	if err != nil {
		log.Printf("level bootstrap lookup failed level_id=%s user_id=%s: %v", levelID, userID, err)
		return nil, errors.New("failed to load level")
	}
	if err := s.cacheQuizzes(callCtx, level); err != nil {
		log.Printf("level quiz cache write failed level_id=%s generation_id=%s: %v", levelID, level.GenerationID, err)
		return nil, errors.New("failed to load level quizzes")
	}

	session, err := s.sessions.Start(callCtx, gamesession.StartOptions{
		UserID:       userID,
		LevelID:      level.LevelID,
		GenerationID: level.GenerationID,
		SubChapterID: level.SubChapterID,
	})
	if err != nil {
		log.Printf("game session create failed level_id=%s user_id=%s: %v", levelID, userID, err)
		return nil, errors.New("failed to start game session")
	}

	state := gameStateFromSession(session, session.UpdatedAt)
	if err := writeWebsocketJSON(conn, writeMu, Message{Type: "game.session.started", Data: state}); err != nil {
		return nil, err
	}

	loopCtx, stop := context.WithCancel(ctx)
	go s.runGameLoop(loopCtx, conn, writeMu, session)
	return stop, nil
}

func (s *Server) runGameLoop(ctx context.Context, conn *websocket.Conn, writeMu *sync.Mutex, session gamesession.State) {
	ticker := time.NewTicker(gameTickInterval)
	defer ticker.Stop()

	current := session
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			current.Tick++
			current.UpdatedAt = now.UTC()
			if err := writeWebsocketJSON(conn, writeMu, Message{Type: "game.state", Data: gameStateFromSession(current, current.UpdatedAt)}); err != nil {
				log.Printf("game state write failed session_id=%s: %v", current.SessionID, err)
				return
			}
		}
	}
}

func gameStateFromSession(session gamesession.State, serverTime time.Time) GameState {
	return GameState{
		SessionID:  session.SessionID,
		LevelID:    session.LevelID,
		Health:     session.Health,
		Essence:    session.Essence,
		Wave:       session.Wave,
		Tick:       session.Tick,
		ServerTime: serverTime.UTC(),
	}
}

func (s *Server) generationStatus(w http.ResponseWriter, r *http.Request) {
	userID := s.authenticatedUserID(r)
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	if s.jobs == nil || s.statuses == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "level generation status is not configured"})
		return
	}

	generationID, ok := generationIDFromStatusPath(r.URL.Path)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "level generation status not found"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	job, err := s.jobs.GetGeneration(ctx, generationID)
	if errors.Is(err, models.ErrGenerationNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "level generation not found"})
		return
	}
	if err != nil {
		log.Printf("level generation lookup failed generation_id=%s: %v", generationID, err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "failed to read level generation"})
		return
	}
	if job.UserID != userID {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "level generation not found"})
		return
	}

	status, err := s.statuses.Get(ctx, generationID)
	if errors.Is(err, models.ErrStatusNotFound) && job.LevelID != nil && *job.LevelID != "" {
		status = models.GenerationStatus{
			GenerationID:        job.ID,
			UserID:              job.UserID,
			SubChapterID:        job.SubChapterID,
			Status:              models.GenerationStatusComplete,
			MapStatus:           models.StepStatusComplete,
			QuizStatus:          models.StepStatusComplete,
			MapSeed:             job.MapSeed,
			MapAlgorithmVersion: job.MapAlgorithmVersion,
			LevelID:             job.LevelID,
			UpdatedAt:           job.UpdatedAt,
		}
		writeJSON(w, http.StatusOK, status)
		return
	}
	if errors.Is(err, models.ErrStatusNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "level generation status not found"})
		return
	}
	if err != nil {
		log.Printf("level generation status lookup failed generation_id=%s: %v", generationID, err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "failed to read level generation status"})
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) authenticatedUserID(r *http.Request) string {
	userID := strings.TrimSpace(r.Header.Get("X-Authenticated-User-Id"))
	if userID != "" {
		return userID
	}
	if s.config.AllowInsecureUserQueryAuth {
		return strings.TrimSpace(r.URL.Query().Get("user_id"))
	}
	return ""
}

func (s *Server) checkOrigin(r *http.Request) bool {
	if len(s.config.AllowedOrigins) == 0 {
		return true
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	for _, allowed := range s.config.AllowedOrigins {
		if origin == allowed {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("json response write failed: %v", err)
	}
}

func writeWebsocketJSON(conn *websocket.Conn, writeMu *sync.Mutex, message Message) error {
	writeMu.Lock()
	defer writeMu.Unlock()
	if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	return conn.WriteJSON(message)
}

func decodeMessageData(data any, target any) error {
	body, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, target)
}

func generationIDFromStatusPath(path string) (string, bool) {
	const prefix = "/level-generation/"
	const suffix = "/status"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return "", false
	}
	generationID := strings.Trim(strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix), "/")
	return generationID, generationID != ""
}
