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
const gameTicksPerSecond = 20.0

const placeTowerAction = "place_tower"
const evolveTowerAction = "evolve_tower"
const awardQuizEssenceAction = "award_quiz_essence"
const pauseGameAction = "pause_game"
const resumeGameAction = "resume_game"

const (
	waveClearDelayTicks     = int64(60)
	smogSpawnIntervalTicks  = int64(40)
	groupGapTicks           = int64(160)
	baseHealthDamage        = 10
	correctQuizEssenceAward = 30
	baseSmogHealth          = 60
	baseSmogSpeed           = 0.8
)

type LevelRepository interface {
	GetBootstrap(ctx context.Context, levelID string, userID string) (repository.LevelBootstrap, error)
	SaveQuizMistake(ctx context.Context, input repository.QuizMistakeInput) error
	ListQuizMistakes(ctx context.Context, userID string, levelID string) ([]repository.QuizMistakeSummaryItem, error)
	ClearQuizMistakes(ctx context.Context, userID string, levelID string) error
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
	Get(ctx context.Context, generationID string) (quizcache.LevelQuizzes, error)
	PeekRandom(ctx context.Context, generationID string) (quizcache.CachedQuiz, int, error)
	Take(ctx context.Context, generationID string, quizID string) (quizcache.CachedQuiz, int, error)
	Set(ctx context.Context, generationID string, quizzes quizcache.LevelQuizzes) error
	Delete(ctx context.Context, generationID string) error
}

type GameSessionStore interface {
	Start(ctx context.Context, options gamesession.StartOptions) (gamesession.State, error)
	LoadRuntimeState(ctx context.Context, sessionID string) (gamesession.RuntimeState, error)
	SaveRuntimeState(ctx context.Context, sessionID string, runtime gamesession.RuntimeState) error
	Delete(ctx context.Context, sessionID string) error
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

func stopGameLoop(loop *runningGameLoop) {
	if loop == nil {
		return
	}
	loop.stop()
	select {
	case <-loop.done:
	case <-time.After(2 * time.Second):
	}
}

func New(cfg config.Config, levels LevelRepository, maps MapCache) *Server {
	return NewWithGenerationCachesAndSessions(cfg, levels, maps, nil, nil, nil, nil, nil)
}

func NewWithGeneration(cfg config.Config, levels LevelRepository, maps MapCache, jobs GenerationRepository, starter GenerationStarter, statuses GenerationStatusStore) *Server {
	return NewWithGenerationCachesAndSessions(cfg, levels, maps, nil, jobs, starter, statuses, nil)
}

func NewWithGenerationAndCaches(cfg config.Config, levels LevelRepository, maps MapCache, quizzes QuizCache, jobs GenerationRepository, starter GenerationStarter, statuses GenerationStatusStore) *Server {
	return NewWithGenerationCachesAndSessions(cfg, levels, maps, quizzes, jobs, starter, statuses, nil)
}

func NewWithGenerationCachesAndSessions(cfg config.Config, levels LevelRepository, maps MapCache, quizzes QuizCache, jobs GenerationRepository, starter GenerationStarter, statuses GenerationStatusStore, sessions GameSessionStore) *Server {
	s := &Server{
		config:   cfg,
		levels:   levels,
		jobs:     jobs,
		starter:  starter,
		statuses: statuses,
		maps:     maps,
		quizzes:  quizzes,
		sessions: sessions,
	}
	s.upgrader = websocket.Upgrader{CheckOrigin: s.checkOrigin}
	return s
}

func (s *Server) Router() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.health)
	mux.HandleFunc("/ready", s.ready)
	mux.HandleFunc("/ws", s.websocket)
	mux.HandleFunc("/level-generation/", s.generationStatus)
	mux.HandleFunc("/quiz-mistakes", s.quizMistakes)
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
	if _, err := s.quizzes.Get(ctx, level.GenerationID); err == nil {
		return nil
	} else if !errors.Is(err, quizcache.ErrQuizzesNotFound) {
		return err
	}
	if len(level.Quizzes) == 0 {
		return nil
	}
	return s.quizzes.Set(ctx, level.GenerationID, quizcache.FromLevelBootstrap(level, level.GenerationID))
}

func (s *Server) readLoop(ctx context.Context, conn *websocket.Conn, writeMu *sync.Mutex, userID string) {
	conn.SetReadLimit(8192)
	var gameLoop *runningGameLoop
	defer func() {
		stopGameLoop(gameLoop)
	}()
	for {
		var message Message
		if err := conn.ReadJSON(&message); err != nil {
			if !isExpectedWebsocketClose(err) {
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
			if gameLoop != nil {
				stopGameLoop(gameLoop)
				gameLoop = nil
			}
			loop, err := s.handleSessionStart(ctx, conn, writeMu, userID, message.Data)
			if err != nil {
				log.Printf("game.session.start failed user_id=%s: %v", userID, err)
				if writeErr := writeWebsocketJSON(conn, writeMu, Message{Type: "error", Data: map[string]string{"error": err.Error()}}); writeErr != nil {
					log.Printf("websocket error write failed: %v", writeErr)
					return
				}
				continue
			}
			gameLoop = loop
		case "game.exit":
			request, err := decodeGameExit(message.Data)
			if err != nil {
				if writeErr := writeWebsocketJSON(conn, writeMu, Message{Type: "error", Data: map[string]string{"error": err.Error()}}); writeErr != nil {
					log.Printf("websocket error write failed: %v", writeErr)
					return
				}
				continue
			}
			exitedState, err := s.handleGameExit(ctx, gameLoop, request)
			if err != nil {
				log.Printf("game.exit failed user_id=%s: %v", userID, err)
				if writeErr := writeWebsocketJSON(conn, writeMu, Message{Type: "error", Data: map[string]string{"error": err.Error()}}); writeErr != nil {
					log.Printf("websocket error write failed: %v", writeErr)
					return
				}
				continue
			}
			gameLoop = nil
			if err := writeWebsocketJSON(conn, writeMu, Message{Type: "game.exited", Data: exitedState}); err != nil {
				log.Printf("game exited write failed: %v", err)
				return
			}
		case "game.quiz.request":
			if err := s.handleQuizRequest(ctx, conn, writeMu, gameLoop); err != nil {
				log.Printf("game.quiz.request failed user_id=%s: %v", userID, err)
				if writeErr := writeWebsocketJSON(conn, writeMu, Message{Type: "error", Data: map[string]string{"error": err.Error()}}); writeErr != nil {
					log.Printf("websocket error write failed: %v", writeErr)
					return
				}
			}
		case "game.quiz.answer":
			request, err := decodeQuizAnswer(message.Data)
			if err != nil {
				if writeErr := writeWebsocketJSON(conn, writeMu, Message{Type: "error", Data: map[string]string{"error": err.Error()}}); writeErr != nil {
					log.Printf("websocket error write failed: %v", writeErr)
					return
				}
				continue
			}
			if err := s.handleQuizAnswer(ctx, conn, writeMu, gameLoop, request); err != nil {
				log.Printf("game.quiz.answer failed user_id=%s: %v", userID, err)
				if writeErr := writeWebsocketJSON(conn, writeMu, Message{Type: "error", Data: map[string]string{"error": err.Error()}}); writeErr != nil {
					log.Printf("websocket error write failed: %v", writeErr)
					return
				}
			}
		case "game.pause":
			if gameLoop != nil && !gameLoop.stopped() {
				select {
				case gameLoop.actions <- clientAction{Type: pauseGameAction}:
				default:
				}
			}
		case "game.resume":
			if gameLoop != nil && !gameLoop.stopped() {
				select {
				case gameLoop.actions <- clientAction{Type: resumeGameAction}:
				default:
				}
			}
		case "game.action.place_tower":
			action, err := decodePlaceTowerAction(message.Data)
			if err != nil {
				if writeErr := writeActionRejected(conn, writeMu, placeTowerAction, err.Error()); writeErr != nil {
					log.Printf("websocket action rejection write failed: %v", writeErr)
					return
				}
				continue
			}
			if fatal := enqueueAction(conn, writeMu, &gameLoop, placeTowerAction, clientAction{Type: placeTowerAction, PlaceTower: action}); fatal {
				return
			}
		case "game.action.evolve_tower":
			action, err := decodeEvolveTowerAction(message.Data)
			if err != nil {
				if writeErr := writeActionRejected(conn, writeMu, evolveTowerAction, err.Error()); writeErr != nil {
					log.Printf("websocket action rejection write failed: %v", writeErr)
					return
				}
				continue
			}
			if fatal := enqueueAction(conn, writeMu, &gameLoop, evolveTowerAction, clientAction{Type: evolveTowerAction, EvolveTower: action}); fatal {
				return
			}
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

func isExpectedWebsocketClose(err error) bool {
	return websocket.IsCloseError(
		err,
		websocket.CloseNormalClosure,
		websocket.CloseGoingAway,
		websocket.CloseNoStatusReceived,
		websocket.CloseAbnormalClosure,
	) || strings.Contains(err.Error(), "unexpected EOF") ||
		strings.Contains(err.Error(), "connection reset by peer")
}

func decodePlaceTowerAction(data any) (placeTowerRequest, error) {
	var request placeTowerRequest
	if err := decodeMessageData(data, &request); err != nil {
		return placeTowerRequest{}, errors.New("place tower action data must include bird_type, x, and y")
	}
	if strings.TrimSpace(request.BirdType) == "" {
		return placeTowerRequest{}, errors.New("bird_type is required")
	}
	return request, nil
}

func decodeEvolveTowerAction(data any) (evolveTowerRequest, error) {
	var request evolveTowerRequest
	if err := decodeMessageData(data, &request); err != nil {
		return evolveTowerRequest{}, errors.New("evolve tower action data must include tower_id and bird_type")
	}
	if strings.TrimSpace(request.TowerID) == "" {
		return evolveTowerRequest{}, errors.New("tower_id is required")
	}
	if strings.TrimSpace(request.BirdType) == "" {
		return evolveTowerRequest{}, errors.New("bird_type is required")
	}
	return request, nil
}

func decodeGameExit(data any) (gameExitRequest, error) {
	var request gameExitRequest
	if data == nil {
		return request, nil
	}
	if err := decodeMessageData(data, &request); err != nil {
		return gameExitRequest{}, errors.New("game.exit data must be an object")
	}
	request.SessionID = strings.TrimSpace(request.SessionID)
	if request.SessionID != "" && !uuidPattern.MatchString(request.SessionID) {
		return gameExitRequest{}, errors.New("session_id must be a valid UUID")
	}
	return request, nil
}

func decodeQuizAnswer(data any) (quizAnswerRequest, error) {
	var request quizAnswerRequest
	if err := decodeMessageData(data, &request); err != nil {
		return quizAnswerRequest{}, errors.New("game.quiz.answer data must include quiz_id and selected_index")
	}
	request.QuizID = strings.TrimSpace(request.QuizID)
	if request.QuizID == "" {
		return quizAnswerRequest{}, errors.New("quiz_id is required")
	}
	if !uuidPattern.MatchString(request.QuizID) {
		return quizAnswerRequest{}, errors.New("quiz_id must be a valid UUID")
	}
	if request.SelectedIndex < 0 {
		return quizAnswerRequest{}, errors.New("selected_index is out of range")
	}
	return request, nil
}

func isInsideMap(levelMap mapgen.GeneratedMap, x int, y int) bool {
	return x >= 0 && y >= 0 && x < levelMap.Width && y < levelMap.Height
}

func isEnemyPath(levelMap mapgen.GeneratedMap, x int, y int) bool {
	for _, tile := range levelMap.EnemyPath {
		if tile.X == x && tile.Y == y {
			return true
		}
	}
	return false
}

func isOccupied(birds []placedBird, x int, y int) bool {
	for _, placed := range birds {
		if int(placed.bird.Position.X) == x && int(placed.bird.Position.Y) == y {
			return true
		}
	}
	return false
}

func enqueueAction(conn *websocket.Conn, writeMu *sync.Mutex, loop **runningGameLoop, actionType string, action clientAction) (fatal bool) {
	if *loop == nil || (*loop).stopped() {
		*loop = nil
		if err := writeActionRejected(conn, writeMu, actionType, "game session is not running"); err != nil {
			log.Printf("websocket action rejection write failed: %v", err)
			return true
		}
		return false
	}
	select {
	case (*loop).actions <- action:
	default:
		if err := writeActionRejected(conn, writeMu, actionType, "action queue is full"); err != nil {
			log.Printf("websocket action rejection write failed: %v", err)
			return true
		}
	}
	return false
}

func writeActionAccepted(conn *websocket.Conn, writeMu *sync.Mutex, action string, bird placedBird) error {
	return writeWebsocketJSON(conn, writeMu, Message{
		Type: "game.action.accepted",
		Data: map[string]any{
			"action":  action,
			"bird_id": bird.bird.ID,
			"bird": PlacedBirdState{
				ID:              bird.bird.ID,
				Type:            bird.birdType,
				Position:        bird.bird.Position,
				Stats:           bird.bird.Stats,
				LastFiredAtTick: bird.bird.LastFiredAtTick,
			},
		},
	})
}

func writeActionRejected(conn *websocket.Conn, writeMu *sync.Mutex, action string, message string) error {
	return writeWebsocketJSON(conn, writeMu, Message{
		Type: "game.action.rejected",
		Data: map[string]string{
			"action": action,
			"error":  message,
		},
	})
}

func writeGameEnd(conn *websocket.Conn, writeMu *sync.Mutex, msgType string, runtime runtimeSession, reason string) error {
	return writeWebsocketJSON(conn, writeMu, Message{
		Type: msgType,
		Data: GameEndState{
			SessionID: runtime.session.SessionID,
			LevelID:   runtime.session.LevelID,
			Health:    runtime.session.Health,
			Wave:      runtime.session.Wave,
			Tick:      runtime.session.Tick,
			Reason:    reason,
		},
	})
}

func writeGameOver(conn *websocket.Conn, writeMu *sync.Mutex, runtime runtimeSession, reason string) error {
	return writeGameEnd(conn, writeMu, "game.over", runtime, reason)
}

func writeGameVictory(conn *websocket.Conn, writeMu *sync.Mutex, runtime runtimeSession, reason string) error {
	return writeGameEnd(conn, writeMu, "game.victory", runtime, reason)
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

type QuizMistakeSummaryState struct {
	LevelID  string                   `json:"level_id"`
	Count    int                      `json:"count"`
	Mistakes []QuizMistakeSummaryItem `json:"mistakes"`
}

type QuizMistakeSummaryItem struct {
	ID                     string     `json:"id"`
	LevelID                string     `json:"level_id"`
	GenerationID           string     `json:"generation_id"`
	QuizID                 string     `json:"quiz_id"`
	QuizIndex              int        `json:"quiz_index"`
	QuizType               string     `json:"quiz_type"`
	QuestionMarkdown       string     `json:"question_markdown"`
	OptionsMarkdown        []string   `json:"options_markdown"`
	AnswerIndex            int        `json:"answer_index"`
	SelectedIndex          int        `json:"selected_index"`
	CorrectOptionMarkdown  string     `json:"correct_option_markdown"`
	SelectedOptionMarkdown string     `json:"selected_option_markdown"`
	CreatedAt              *time.Time `json:"created_at"`
}

func (s *Server) quizMistakes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	userID := s.authenticatedUserID(r)
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	if s.levels == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "quiz mistake summary is not configured"})
		return
	}

	levelID := strings.TrimSpace(r.URL.Query().Get("level_id"))
	if levelID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "level_id is required"})
		return
	}
	if !uuidPattern.MatchString(levelID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "level_id must be a valid UUID"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	mistakes, err := s.levels.ListQuizMistakes(ctx, userID, levelID)
	if err != nil {
		log.Printf("quiz mistake summary lookup failed level_id=%s user_id=%s: %v", levelID, userID, err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "failed to read quiz mistake summary"})
		return
	}

	writeJSON(w, http.StatusOK, quizMistakeSummaryState(levelID, mistakes))
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
