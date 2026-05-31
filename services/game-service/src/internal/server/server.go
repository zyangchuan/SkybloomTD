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

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"skybloom/game-service/internal/config"
	"skybloom/game-service/internal/gameobject"
	"skybloom/game-service/internal/gamesession"
	"skybloom/game-service/internal/generation"
	"skybloom/game-service/internal/mapcache"
	"skybloom/game-service/internal/mapgen"
	"skybloom/game-service/internal/models"
	"skybloom/game-service/internal/quizcache"
	"skybloom/game-service/internal/quiztext"
	"skybloom/game-service/internal/repository"
)

var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

const gameTickInterval = 50 * time.Millisecond
const gameTicksPerSecond = 20.0

const placeTowerAction = "place_tower"
const awardQuizEssenceAction = "award_quiz_essence"
const pauseGameAction = "pause_game"
const resumeGameAction = "resume_game"

const (
	waveClearDelayTicks     = int64(60)
	smogSpawnIntervalTicks  = int64(40)
	groupGapTicks           = int64(160)
	baseHealthDamage        = 10
	correctQuizEssenceAward = 30
	baseSmogHealth          = 30
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
	PeekNext(ctx context.Context, generationID string) (quizcache.CachedQuiz, int, error)
	Take(ctx context.Context, generationID string, quizID string) (quizcache.CachedQuiz, int, error)
	Set(ctx context.Context, generationID string, quizzes quizcache.LevelQuizzes) error
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

type Message struct {
	Type string `json:"type"`
	Data any    `json:"data,omitempty"`
}

type InitialState struct {
	Map mapgen.GeneratedMap `json:"map"`
}

type GameState struct {
	SessionID   string            `json:"session_id"`
	LevelID     string            `json:"level_id"`
	Health      int               `json:"health"`
	Essence     int               `json:"essence"`
	Wave        int               `json:"wave"`
	Tick        int64             `json:"tick"`
	ServerTime  time.Time         `json:"server_time"`
	BirdTypes   []BirdTypeInfo    `json:"bird_types,omitempty"`
	Birds       []PlacedBirdState `json:"birds"`
	Smogs       []SmogState       `json:"smogs"`
	Projectiles []ProjectileState `json:"projectiles"`
	Events      []GameEvent       `json:"events,omitempty"`
}

type BirdTypeInfo struct {
	Type   string               `json:"type"`
	Stats  gameobject.BirdStats `json:"stats"`
	Attack string               `json:"attack"`
}

type PlacedBirdState struct {
	ID              string               `json:"id"`
	Type            string               `json:"type"`
	Position        gameobject.Position  `json:"position"`
	Stats           gameobject.BirdStats `json:"stats"`
	LastFiredAtTick int64                `json:"last_fired_at_tick"`
}

type SmogState struct {
	ID        string              `json:"id"`
	Health    int                 `json:"health"`
	Position  gameobject.Position `json:"position"`
	Speed     float64             `json:"speed"`
	PathIndex int                 `json:"path_index"`
}

type ProjectileState struct {
	ID              string                    `json:"id"`
	Type            gameobject.ProjectileType `json:"type"`
	Damage          float64                   `json:"damage"`
	ProjectileSpeed float64                   `json:"projectile_speed"`
	Position        gameobject.Position       `json:"position"`
	Direction       gameobject.Vector         `json:"direction"`
	TargetID        string                    `json:"target_id,omitempty"`
	RemainingRange  float64                   `json:"remaining_range"`
	HitRadius       float64                   `json:"hit_radius"`
}

type GameEvent struct {
	Type          string   `json:"type"`
	BirdID        string   `json:"bird_id,omitempty"`
	SmogID        string   `json:"smog_id,omitempty"`
	ProjectileID  string   `json:"projectile_id,omitempty"`
	ProjectileIDs []string `json:"projectile_ids,omitempty"`
	Damage        float64  `json:"damage,omitempty"`
	Health        int      `json:"health,omitempty"`
	Wave          int      `json:"wave,omitempty"`
}

type GameOverState struct {
	SessionID string `json:"session_id"`
	LevelID   string `json:"level_id"`
	Health    int    `json:"health"`
	Wave      int    `json:"wave"`
	Tick      int64  `json:"tick"`
	Reason    string `json:"reason"`
}

type GameVictoryState struct {
	SessionID string `json:"session_id"`
	LevelID   string `json:"level_id"`
	Health    int    `json:"health"`
	Wave      int    `json:"wave"`
	Tick      int64  `json:"tick"`
	Reason    string `json:"reason"`
}

type GameExitedState struct {
	SessionID string `json:"session_id,omitempty"`
	Deleted   bool   `json:"deleted"`
	Reason    string `json:"reason"`
}

type QuizPromptState struct {
	QuizID           string   `json:"quiz_id"`
	QuizType         string   `json:"quiz_type"`
	QuestionMarkdown string   `json:"question_markdown"`
	OptionsMarkdown  []string `json:"options_markdown"`
	Remaining        int      `json:"remaining"`
}

type QuizUnavailableState struct {
	Reason string `json:"reason"`
}

type QuizResultState struct {
	QuizID                 string `json:"quiz_id"`
	Correct                bool   `json:"correct"`
	SelectedIndex          int    `json:"selected_index"`
	CorrectIndex           int    `json:"correct_index"`
	SelectedOptionMarkdown string `json:"selected_option_markdown"`
	CorrectOptionMarkdown  string `json:"correct_option_markdown"`
	EssenceAwarded         int    `json:"essence_awarded"`
	Essence                int    `json:"essence,omitempty"`
	Remaining              int    `json:"remaining"`
}

type placeTowerRequest struct {
	BirdType string `json:"bird_type"`
	X        int    `json:"x"`
	Y        int    `json:"y"`
}

type gameExitRequest struct {
	SessionID string `json:"session_id"`
}

type quizAnswerRequest struct {
	QuizID        string `json:"quiz_id"`
	SelectedIndex int    `json:"selected_index"`
}

type clientAction struct {
	Type          string
	PlaceTower    placeTowerRequest
	EssenceReward int
	Result        chan actionResult
}

type actionResult struct {
	Essence int
	Err     error
}

type runningGameLoop struct {
	sessionID    string
	levelID      string
	generationID string
	userID       string
	stop         context.CancelFunc
	actions      chan clientAction
	done         chan struct{}
}

func (l *runningGameLoop) stopped() bool {
	if l == nil {
		return true
	}
	select {
	case <-l.done:
		return true
	default:
		return false
	}
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

type runtimeSession struct {
	session     gamesession.State
	economy     gamesession.Economy
	birds       []placedBird
	smogs       []gameobject.Smog
	projectiles []gameobject.Projectile
	levelMap    mapgen.GeneratedMap
	path        []gameobject.Position
	loopStarted bool
	loopPaused  bool

	waveStartedAtTick int64
	waveSpawned       int
	nextWaveTick      int64
}

type placedBird struct {
	birdType string
	bird     gameobject.Bird
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
			if gameLoop == nil || gameLoop.stopped() {
				gameLoop = nil
				if writeErr := writeActionRejected(conn, writeMu, placeTowerAction, "game session is not running"); writeErr != nil {
					log.Printf("websocket action rejection write failed: %v", writeErr)
					return
				}
				continue
			}
			select {
			case gameLoop.actions <- clientAction{Type: placeTowerAction, PlaceTower: action}:
			default:
				if writeErr := writeActionRejected(conn, writeMu, placeTowerAction, "action queue is full"); writeErr != nil {
					log.Printf("websocket action rejection write failed: %v", writeErr)
					return
				}
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

func (s *Server) handleSessionStart(ctx context.Context, conn *websocket.Conn, writeMu *sync.Mutex, userID string, data any) (*runningGameLoop, error) {
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

	// Flush/Clear old quiz mistakes for this level and user so they start the new run with a clean slate
	if err := s.levels.ClearQuizMistakes(callCtx, userID, levelID); err != nil {
		log.Printf("failed to clear quiz mistakes level_id=%s user_id=%s: %v", levelID, userID, err)
	}

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
	levelMap, err := s.loadMap(callCtx, level)
	if err != nil {
		log.Printf("level map load failed level_id=%s generation_id=%s: %v", levelID, level.GenerationID, err)
		return nil, errors.New("failed to load level map")
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
	storedRuntime, err := s.sessions.LoadRuntimeState(callCtx, session.SessionID)
	if err != nil {
		log.Printf("game session runtime load failed session_id=%s: %v", session.SessionID, err)
		return nil, errors.New("failed to load game session")
	}
	storedRuntime = normalizeRuntimeState(storedRuntime)
	restoredBirds, err := placedBirdsFromStored(storedRuntime.Birds)
	if err != nil {
		log.Printf("game session birds restore failed session_id=%s: %v", session.SessionID, err)
		return nil, errors.New("failed to restore game session")
	}
	restoredProjectiles := projectilesFromStored(storedRuntime.Projectiles)
	session.Health = storedRuntime.Health
	session.Essence = storedRuntime.Essence
	session.Wave = storedRuntime.Wave
	session.Tick = storedRuntime.Tick

	runtime := runtimeSession{
		session:     session,
		economy:     gamesession.NewEconomy(session.Essence),
		birds:       restoredBirds,
		smogs:       smogsFromStored(storedRuntime.Smogs),
		projectiles: restoredProjectiles,
		levelMap:    levelMap,
		path:        gamePath(levelMap),
		loopStarted: storedRuntime.LoopStarted,

		waveStartedAtTick: storedRuntime.WaveStartedAtTick,
		waveSpawned:       storedRuntime.WaveSpawned,
		nextWaveTick:      storedRuntime.NextWaveTick,
	}
	state := gameStateFromRuntime(runtime, session.UpdatedAt, birdTypeCatalog(), nil)
	if err := writeWebsocketJSON(conn, writeMu, Message{Type: "game.session.started", Data: state}); err != nil {
		return nil, err
	}
	if runtime.session.Health <= 0 {
		if err := writeGameOver(conn, writeMu, runtime, "health_depleted"); err != nil {
			return nil, err
		}
		return nil, nil
	}
	if gameWon(runtime) {
		if err := writeGameVictory(conn, writeMu, runtime, "all_waves_cleared"); err != nil {
			return nil, err
		}
		return nil, nil
	}

	loopCtx, stop := context.WithCancel(ctx)
	loop := &runningGameLoop{
		sessionID:    runtime.session.SessionID,
		levelID:      runtime.session.LevelID,
		generationID: runtime.session.GenerationID,
		userID:       runtime.session.UserID,
		stop:         stop,
		actions:      make(chan clientAction, 64),
		done:         make(chan struct{}),
	}
	go s.runGameLoop(loopCtx, conn, writeMu, runtime, loop.actions, loop.done)
	return loop, nil
}

func (s *Server) runGameLoop(ctx context.Context, conn *websocket.Conn, writeMu *sync.Mutex, runtime runtimeSession, actions <-chan clientAction, done chan<- struct{}) {
	ticker := time.NewTicker(gameTickInterval)
	defer ticker.Stop()
	defer close(done)

	for {
		select {
		case <-ctx.Done():
			return
		case action := <-actions:
			switch action.Type {
			case placeTowerAction:
				if err := s.processClientAction(ctx, &runtime, action); err != nil {
					if writeErr := writeActionRejected(conn, writeMu, action.Type, err.Error()); writeErr != nil {
						log.Printf("websocket action rejection write failed: %v", writeErr)
						return
					}
					continue
				}
				if err := writeActionAccepted(conn, writeMu, action.Type, runtime.birds[len(runtime.birds)-1]); err != nil {
					log.Printf("websocket action accepted write failed: %v", err)
					return
				}
			case awardQuizEssenceAction:
				runtime.loopStarted = true
				essence, err := s.awardEssence(ctx, &runtime, action.EssenceReward)
				if action.Result != nil {
					action.Result <- actionResult{Essence: essence, Err: err}
				}
			case pauseGameAction:
				runtime.loopPaused = true
			case resumeGameAction:
				runtime.loopPaused = false
			default:
				if action.Result != nil {
					action.Result <- actionResult{Err: errors.New("unsupported action")}
				}
			}
		case now := <-ticker.C:
			if runtime.loopPaused {
				continue
			}
			events := advanceRuntimeTick(&runtime, now)
			if err := s.saveRuntimeState(ctx, runtime); err != nil {
				log.Printf("game session runtime save failed session_id=%s: %v", runtime.session.SessionID, err)
			}
			if err := writeWebsocketJSON(conn, writeMu, Message{Type: "game.state", Data: gameStateFromRuntime(runtime, runtime.session.UpdatedAt, nil, events)}); err != nil {
				log.Printf("game state write failed session_id=%s: %v", runtime.session.SessionID, err)
				return
			}
			if runtime.session.Health <= 0 {
				if err := writeGameOver(conn, writeMu, runtime, "health_depleted"); err != nil {
					log.Printf("game over write failed session_id=%s: %v", runtime.session.SessionID, err)
				}
				return
			}
			if gameWon(runtime) {
				if err := writeGameVictory(conn, writeMu, runtime, "all_waves_cleared"); err != nil {
					log.Printf("game victory write failed session_id=%s: %v", runtime.session.SessionID, err)
				}
				return
			}
		}
	}
}

func (s *Server) handleGameExit(ctx context.Context, loop *runningGameLoop, request gameExitRequest) (GameExitedState, error) {
	sessionID := strings.TrimSpace(request.SessionID)
	if loop == nil {
		if sessionID == "" {
			return GameExitedState{Deleted: false, Reason: "no_running_session"}, nil
		}
	} else {
		sessionID = loop.sessionID
		stopGameLoop(loop)
	}
	if strings.TrimSpace(sessionID) == "" {
		return GameExitedState{Deleted: false, Reason: "no_running_session"}, nil
	}
	if s.sessions == nil {
		return GameExitedState{}, errors.New("game session store is not configured")
	}
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if loop != nil {
		if err := s.levels.ClearQuizMistakes(callCtx, loop.userID, loop.levelID); err != nil {
			log.Printf("failed to clear quiz mistakes on exit level_id=%s user_id=%s: %v", loop.levelID, loop.userID, err)
		}
	}

	if err := s.sessions.Delete(callCtx, sessionID); err != nil {
		return GameExitedState{}, errors.New("failed to exit game session")
	}
	return GameExitedState{SessionID: sessionID, Deleted: true, Reason: "client_exit"}, nil
}

func (s *Server) handleQuizRequest(ctx context.Context, conn *websocket.Conn, writeMu *sync.Mutex, loop *runningGameLoop) error {
	if loop == nil || loop.stopped() {
		return errors.New("game session is not running")
	}
	if s.quizzes == nil {
		return errors.New("quiz cache is not configured")
	}
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	quiz, remaining, err := s.quizzes.PeekNext(callCtx, loop.generationID)
	if errors.Is(err, quizcache.ErrQuizzesNotFound) {
		return writeWebsocketJSON(conn, writeMu, Message{
			Type: "game.quiz.unavailable",
			Data: QuizUnavailableState{Reason: "no_quizzes_remaining"},
		})
	}
	if err != nil {
		return errors.New("failed to load quiz")
	}
	return writeWebsocketJSON(conn, writeMu, Message{
		Type: "game.quiz.presented",
		Data: quizPromptState(quiz, remaining),
	})
}

func (s *Server) handleQuizAnswer(ctx context.Context, conn *websocket.Conn, writeMu *sync.Mutex, loop *runningGameLoop, request quizAnswerRequest) error {
	if loop == nil || loop.stopped() {
		return errors.New("game session is not running")
	}
	if s.quizzes == nil {
		return errors.New("quiz cache is not configured")
	}

	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	currentQuiz, _, err := s.quizzes.PeekNext(callCtx, loop.generationID)
	if errors.Is(err, quizcache.ErrQuizzesNotFound) {
		return writeWebsocketJSON(conn, writeMu, Message{
			Type: "game.quiz.unavailable",
			Data: QuizUnavailableState{Reason: "no_quizzes_remaining"},
		})
	}
	if err != nil {
		return errors.New("failed to load quiz")
	}
	if currentQuiz.ID != request.QuizID {
		return errors.New("quiz_id is not the current quiz")
	}
	if request.SelectedIndex < 0 || request.SelectedIndex >= len(currentQuiz.OptionsMarkdown) {
		return errors.New("selected_index is out of range")
	}

	answeredQuiz, remaining, err := s.quizzes.Take(callCtx, loop.generationID, request.QuizID)
	if errors.Is(err, quizcache.ErrQuizNotFound) || errors.Is(err, quizcache.ErrQuizzesNotFound) {
		return errors.New("quiz not found")
	}
	if err != nil {
		return errors.New("failed to remove answered quiz")
	}

	correct := request.SelectedIndex == answeredQuiz.AnswerIndex
	essenceAwarded := 0
	essence := 0
	if correct {
		essenceAwarded = correctQuizEssenceAward
		essence, err = s.awardEssenceThroughLoop(ctx, loop, correctQuizEssenceAward)
		if err != nil {
			return err
		}
	}

	if err := writeWebsocketJSON(conn, writeMu, Message{
		Type: "game.quiz.result",
		Data: QuizResultState{
			QuizID:                 answeredQuiz.ID,
			Correct:                correct,
			SelectedIndex:          request.SelectedIndex,
			CorrectIndex:           answeredQuiz.AnswerIndex,
			SelectedOptionMarkdown: optionMarkdown(answeredQuiz.OptionsMarkdown, request.SelectedIndex),
			CorrectOptionMarkdown:  optionMarkdown(answeredQuiz.OptionsMarkdown, answeredQuiz.AnswerIndex),
			EssenceAwarded:         essenceAwarded,
			Essence:                essence,
			Remaining:              remaining,
		},
	}); err != nil {
		return err
	}

	if !correct {
		s.saveQuizMistakeAsync(loop, answeredQuiz, request.SelectedIndex)
	}

	return nil
}

func (s *Server) awardEssenceThroughLoop(ctx context.Context, loop *runningGameLoop, amount int) (int, error) {
	if loop == nil || loop.stopped() {
		return 0, errors.New("game session is not running")
	}
	result := make(chan actionResult, 1)
	action := clientAction{
		Type:          awardQuizEssenceAction,
		EssenceReward: amount,
		Result:        result,
	}
	select {
	case loop.actions <- action:
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-time.After(2 * time.Second):
		return 0, errors.New("game action queue timed out")
	}

	select {
	case response := <-result:
		return response.Essence, response.Err
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-time.After(2 * time.Second):
		return 0, errors.New("game action response timed out")
	}
}

func (s *Server) saveQuizMistakeAsync(loop *runningGameLoop, quiz quizcache.CachedQuiz, selectedIndex int) {
	if s.levels == nil {
		log.Printf("quiz mistake save skipped quiz_id=%s: level repository is not configured", quiz.ID)
		return
	}
	input := quizMistakeInput(loop, quiz, selectedIndex)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.levels.SaveQuizMistake(ctx, input); err != nil {
			log.Printf("quiz mistake save failed quiz_id=%s user_id=%s: %v", input.QuizID, input.UserID, err)
		}
	}()
}

func quizMistakeInput(loop *runningGameLoop, quiz quizcache.CachedQuiz, selectedIndex int) repository.QuizMistakeInput {
	return repository.QuizMistakeInput{
		UserID:           loop.userID,
		LevelID:          loop.levelID,
		GenerationID:     loop.generationID,
		QuizID:           quiz.ID,
		QuizIndex:        quiz.QuizIndex,
		QuizType:         quiz.QuizType,
		QuestionMarkdown: quiztext.SanitizeMarkdown(quiz.QuestionMarkdown),
		OptionsMarkdown:  quiztext.SanitizeMarkdownSlice(quiz.OptionsMarkdown),
		AnswerIndex:      quiz.AnswerIndex,
		SelectedIndex:    selectedIndex,
	}
}

func (s *Server) processClientAction(ctx context.Context, runtime *runtimeSession, action clientAction) error {
	switch action.Type {
	case placeTowerAction:
		return s.placeTower(ctx, runtime, action.PlaceTower)
	default:
		return errors.New("unsupported action")
	}
}

func (s *Server) placeTower(ctx context.Context, runtime *runtimeSession, request placeTowerRequest) error {
	if runtime == nil {
		return errors.New("game session is not running")
	}
	birdType := strings.TrimSpace(request.BirdType)
	stats, err := gameobject.BirdStatsForType(birdType)
	if err != nil {
		return errors.New("unknown bird type")
	}
	if !isInsideMap(runtime.levelMap, request.X, request.Y) {
		return errors.New("tower position is outside the map")
	}
	if isEnemyPath(runtime.levelMap, request.X, request.Y) {
		return errors.New("tower cannot be placed on the enemy path")
	}
	if isOccupied(runtime.birds, request.X, request.Y) {
		return errors.New("tower position is occupied")
	}

	nextEconomy := runtime.economy
	if !nextEconomy.Consume(stats.Cost) {
		return errors.New("insufficient essence")
	}

	bird, err := gameobject.NewBird(uuid.NewString(), birdType, gameobject.Position{X: float64(request.X), Y: float64(request.Y)})
	if err != nil {
		return errors.New("failed to create bird")
	}
	previousEconomy := runtime.economy
	previousBirds := runtime.birds
	runtime.economy = nextEconomy
	runtime.session.Essence = nextEconomy.Essence
	runtime.birds = append(append([]placedBird{}, runtime.birds...), placedBird{birdType: birdType, bird: bird})
	if err := s.saveRuntimeState(ctx, *runtime); err != nil {
		runtime.economy = previousEconomy
		runtime.session.Essence = previousEconomy.Essence
		runtime.birds = previousBirds
		log.Printf("game session placement save failed session_id=%s: %v", runtime.session.SessionID, err)
		return errors.New("failed to save tower placement")
	}

	return nil
}

func (s *Server) awardEssence(ctx context.Context, runtime *runtimeSession, amount int) (int, error) {
	if runtime == nil {
		return 0, errors.New("game session is not running")
	}
	if amount <= 0 {
		return runtime.economy.Essence, nil
	}
	previousEconomy := runtime.economy
	runtime.economy.Add(amount)
	runtime.session.Essence = runtime.economy.Essence
	if err := s.saveRuntimeState(ctx, *runtime); err != nil {
		runtime.economy = previousEconomy
		runtime.session.Essence = previousEconomy.Essence
		return previousEconomy.Essence, err
	}
	return runtime.economy.Essence, nil
}

func (s *Server) saveRuntimeState(ctx context.Context, runtime runtimeSession) error {
	return s.sessions.SaveRuntimeState(ctx, runtime.session.SessionID, gamesession.RuntimeState{
		Health:            runtime.session.Health,
		Essence:           runtime.economy.Essence,
		Wave:              runtime.session.Wave,
		Tick:              runtime.session.Tick,
		LoopStarted:       runtime.loopStarted,
		LoopPaused:        false,
		WaveStartedAtTick: runtime.waveStartedAtTick,
		WaveSpawned:       runtime.waveSpawned,
		NextWaveTick:      runtime.nextWaveTick,
		Birds:             storedBirds(runtime.birds),
		Smogs:             storedSmogs(runtime.smogs),
		Projectiles:       storedProjectiles(runtime.projectiles),
	})
}

func gameStateFromRuntime(runtime runtimeSession, serverTime time.Time, birdTypes []BirdTypeInfo, events []GameEvent) GameState {
	session := runtime.session
	return GameState{
		SessionID:   session.SessionID,
		LevelID:     session.LevelID,
		Health:      session.Health,
		Essence:     runtime.economy.Essence,
		Wave:        session.Wave,
		Tick:        session.Tick,
		ServerTime:  serverTime.UTC(),
		BirdTypes:   birdTypes,
		Birds:       placedBirdStates(runtime.birds),
		Smogs:       smogStates(runtime.smogs),
		Projectiles: projectileStates(runtime.projectiles),
		Events:      events,
	}
}

type waveDefinition struct {
	Wave   int
	Count  int
	Health int
	Speed  float64
}

func waveDefinitions() []waveDefinition {
	return []waveDefinition{
		scaledWaveDefinition(1, 15),
		scaledWaveDefinition(2, 24),
		scaledWaveDefinition(3, 36),
	}
}

func scaledWaveDefinition(wave int, count int) waveDefinition {
	return waveDefinition{
		Wave:   wave,
		Count:  count,
		Health: scaledSmogHealth(wave),
		Speed:  scaledSmogSpeed(wave),
	}
}

func scaledSmogHealth(wave int) int {
	waveOffset := wave - 1
	if waveOffset < 0 {
		waveOffset = 0
	}
	return baseSmogHealth + (waveOffset * 20) + (waveOffset * waveOffset * 5)
}

func scaledSmogSpeed(wave int) float64 {
	waveOffset := float64(wave - 1)
	if waveOffset < 0 {
		waveOffset = 0
	}
	return baseSmogSpeed + (waveOffset * 0.17) + (waveOffset * waveOffset * 0.03)
}

func advanceRuntimeTick(runtime *runtimeSession, now time.Time) []GameEvent {
	if runtime == nil {
		return nil
	}
	if !runtime.loopStarted {
		return nil
	}
	runtime.session.Tick++
	runtime.session.Essence = runtime.economy.Essence
	runtime.session.UpdatedAt = now.UTC()

	events := make([]GameEvent, 0)
	events = append(events, moveSmogs(runtime, gameTickInterval.Seconds())...)
	if runtime.session.Health <= 0 {
		return events
	}
	events = append(events, spawnSmogs(runtime)...)
	events = append(events, fireBirds(runtime)...)
	events = append(events, resolveProjectiles(runtime, gameTickInterval.Seconds())...)
	runtime.smogs = aliveSmogs(runtime.smogs)
	events = append(events, scheduleNextWaveIfCleared(runtime)...)
	return events
}

func spawnSmogs(runtime *runtimeSession) []GameEvent {
	if len(runtime.path) == 0 || runtime.nextWaveTick <= 0 || runtime.session.Tick < runtime.nextWaveTick {
		return nil
	}
	wave, ok := activeWaveDefinition(runtime)
	if !ok || runtime.waveSpawned >= wave.Count {
		return nil
	}

	events := make([]GameEvent, 0)
	if runtime.waveSpawned == 0 {
		runtime.session.Wave = wave.Wave
		runtime.waveStartedAtTick = runtime.session.Tick
		events = append(events, GameEvent{Type: "wave.started", Wave: wave.Wave})
	}

	// Calculate subwave/group spawn ticks dynamically
	groupSize := wave.Count / 3
	if groupSize < 1 {
		groupSize = 1
	}
	groupIndex := int64(runtime.waveSpawned / groupSize)
	indexInGroup := int64(runtime.waveSpawned % groupSize)

	groupDurationTicks := int64(groupSize) * smogSpawnIntervalTicks
	groupStartIntervalTicks := groupDurationTicks + groupGapTicks

	nextSpawnTick := runtime.waveStartedAtTick + (groupIndex * groupStartIntervalTicks) + (indexInGroup * smogSpawnIntervalTicks)

	if runtime.session.Tick < nextSpawnTick {
		return events
	}

	smog := gameobject.Smog{
		ID:        uuid.NewString(),
		Health:    wave.Health,
		Position:  runtime.path[0],
		Speed:     wave.Speed,
		PathIndex: 0,
	}
	runtime.waveSpawned++
	runtime.smogs = append(runtime.smogs, smog)
	events = append(events, GameEvent{Type: "smog.spawned", SmogID: smog.ID, Wave: wave.Wave, Health: smog.Health})
	return events
}

func activeWaveDefinition(runtime *runtimeSession) (waveDefinition, bool) {
	waves := waveDefinitions()
	if runtime.session.Wave <= 0 || runtime.waveSpawned == 0 {
		nextIndex := runtime.session.Wave
		if nextIndex < 0 || nextIndex >= len(waves) {
			return waveDefinition{}, false
		}
		return waves[nextIndex], true
	}
	for _, wave := range waves {
		if wave.Wave == runtime.session.Wave {
			return wave, true
		}
	}
	return waveDefinition{}, false
}

func currentWaveDefinition(waveNumber int) (waveDefinition, bool) {
	for _, wave := range waveDefinitions() {
		if wave.Wave == waveNumber {
			return wave, true
		}
	}
	return waveDefinition{}, false
}

func scheduleNextWaveIfCleared(runtime *runtimeSession) []GameEvent {
	if runtime.session.Wave <= 0 || len(runtime.smogs) > 0 {
		return nil
	}
	currentWave, ok := currentWaveDefinition(runtime.session.Wave)
	if !ok || runtime.waveSpawned < currentWave.Count {
		return nil
	}
	if runtime.session.Wave >= len(waveDefinitions()) && runtime.nextWaveTick == 0 {
		return nil
	}

	events := []GameEvent{{Type: "wave.cleared", Wave: runtime.session.Wave}}
	if runtime.session.Wave >= len(waveDefinitions()) {
		runtime.nextWaveTick = 0
		return events
	}

	runtime.waveStartedAtTick = 0
	runtime.waveSpawned = 0
	runtime.nextWaveTick = runtime.session.Tick + waveClearDelayTicks
	return events
}

func gameWon(runtime runtimeSession) bool {
	if runtime.session.Health <= 0 || runtime.session.Wave < len(waveDefinitions()) || len(runtime.smogs) > 0 {
		return false
	}
	finalWave, ok := currentWaveDefinition(runtime.session.Wave)
	return ok && runtime.waveSpawned >= finalWave.Count && runtime.nextWaveTick == 0
}

func moveSmogs(runtime *runtimeSession, deltaSeconds float64) []GameEvent {
	if runtime.session.Health <= 0 {
		return nil
	}
	events := make([]GameEvent, 0)
	nextSmogs := make([]gameobject.Smog, 0, len(runtime.smogs))
	for i := range runtime.smogs {
		smog := runtime.smogs[i]
		smog.Move(deltaSeconds, runtime.path)
		if smogReachedEnd(smog, runtime.path) {
			runtime.session.Health -= baseHealthDamage
			if runtime.session.Health < 0 {
				runtime.session.Health = 0
			}
			events = append(events, GameEvent{
				Type:   "smog.escaped",
				SmogID: smog.ID,
				Damage: float64(baseHealthDamage),
				Health: runtime.session.Health,
			})
			continue
		}
		nextSmogs = append(nextSmogs, smog)
	}
	runtime.smogs = nextSmogs
	return events
}

func fireBirds(runtime *runtimeSession) []GameEvent {
	if runtime.session.Health <= 0 {
		return nil
	}
	events := make([]GameEvent, 0)
	for i := range runtime.birds {
		bird := &runtime.birds[i].bird
		if !bird.CanAttack(runtime.session.Tick, gameTicksPerSecond) {
			continue
		}
		targetIndex := targetSmogIndex(*bird, runtime.smogs)
		if targetIndex < 0 {
			continue
		}
		target := runtime.smogs[targetIndex]
		projectiles := bird.Attack(target, runtime.session.Tick)
		if len(projectiles) == 0 {
			continue
		}
		runtime.projectiles = append(runtime.projectiles, projectiles...)
		events = append(events, GameEvent{
			Type:          "bird.attack",
			BirdID:        bird.ID,
			SmogID:        target.ID,
			ProjectileIDs: projectileIDs(projectiles),
		})
	}
	return events
}

func resolveProjectiles(runtime *runtimeSession, deltaSeconds float64) []GameEvent {
	events := make([]GameEvent, 0)
	active := make([]gameobject.Projectile, 0, len(runtime.projectiles))
	for i := range runtime.projectiles {
		projectile := runtime.projectiles[i]
		projectile.Move(deltaSeconds)
		switch projectile.Type {
		case gameobject.ProjectileTypeLocked:
			if !projectile.HasArrived() {
				active = append(active, projectile)
				continue
			}
			target := findSmogByID(runtime.smogs, projectile.TargetID)
			if target == nil {
				continue
			}
			beforeHealth := target.Health
			if projectile.ApplyLockedDamage(target) {
				events = append(events, GameEvent{
					Type:         "smog.damage",
					SmogID:       target.ID,
					ProjectileID: projectile.ID,
					Damage:       float64(beforeHealth - target.Health),
					Health:       target.Health,
				})
			}
		case gameobject.ProjectileTypeDirectional:
			hit, damage := collideDirectionalProjectile(&projectile, runtime.smogs)
			if hit != nil {
				events = append(events, GameEvent{
					Type:         "smog.damage",
					SmogID:       hit.ID,
					ProjectileID: projectile.ID,
					Damage:       damage,
					Health:       hit.Health,
				})
			}
			if !projectile.IsExpired() {
				active = append(active, projectile)
			}
		default:
			if !projectile.IsExpired() {
				active = append(active, projectile)
			}
		}
	}
	runtime.projectiles = active
	return events
}

func collideDirectionalProjectile(projectile *gameobject.Projectile, smogs []gameobject.Smog) (*gameobject.Smog, float64) {
	if projectile == nil || projectile.Type != gameobject.ProjectileTypeDirectional || projectile.IsExpired() {
		return nil, 0
	}
	for i := range smogs {
		if !smogs[i].IsAlive() {
			continue
		}
		if projectile.Position.DistanceTo(smogs[i].Position) > projectile.HitRadius {
			continue
		}
		beforeHealth := smogs[i].Health
		smogs[i].TakeDamage(projectile.Damage)
		projectile.RemainingRange = 0
		return &smogs[i], float64(beforeHealth - smogs[i].Health)
	}
	return nil, 0
}

func targetSmogIndex(bird gameobject.Bird, smogs []gameobject.Smog) int {
	bestIndex := -1
	for i := range smogs {
		if !bird.TargetInRange(smogs[i]) {
			continue
		}
		if bestIndex < 0 || smogs[i].PathIndex > smogs[bestIndex].PathIndex {
			bestIndex = i
		}
	}
	return bestIndex
}

func aliveSmogs(smogs []gameobject.Smog) []gameobject.Smog {
	active := make([]gameobject.Smog, 0, len(smogs))
	for _, smog := range smogs {
		if smog.IsAlive() {
			active = append(active, smog)
		}
	}
	return active
}

func findSmogByID(smogs []gameobject.Smog, id string) *gameobject.Smog {
	for i := range smogs {
		if smogs[i].ID == id {
			return &smogs[i]
		}
	}
	return nil
}

func projectileIDs(projectiles []gameobject.Projectile) []string {
	ids := make([]string, 0, len(projectiles))
	for _, projectile := range projectiles {
		ids = append(ids, projectile.ID)
	}
	return ids
}

func smogReachedEnd(smog gameobject.Smog, path []gameobject.Position) bool {
	if len(path) == 0 {
		return false
	}
	return smog.PathIndex >= len(path)-1 && smog.Position.DistanceTo(path[len(path)-1]) < 0.000001
}

func gamePath(levelMap mapgen.GeneratedMap) []gameobject.Position {
	path := make([]gameobject.Position, 0, len(levelMap.EnemyPath))
	for _, tile := range levelMap.EnemyPath {
		path = append(path, gameobject.Position{X: float64(tile.X), Y: float64(tile.Y)})
	}
	return path
}

func normalizeRuntimeState(runtime gamesession.RuntimeState) gamesession.RuntimeState {
	if runtime.NextWaveTick == 0 && runtime.Wave <= 0 {
		runtime.NextWaveTick = 1
	}
	if runtime.NextWaveTick == 0 && runtime.Wave > 0 && runtime.WaveSpawned == 0 {
		if wave, ok := currentWaveDefinition(runtime.Wave); ok {
			runtime.WaveSpawned = wave.Count
		}
	}
	if runtime.WaveStartedAtTick == 0 && runtime.Wave > 0 && runtime.WaveSpawned > 0 {
		runtime.WaveStartedAtTick = runtime.Tick
	}
	return runtime
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

func birdTypeCatalog() []BirdTypeInfo {
	birdTypes := gameobject.BirdTypes()
	catalog := make([]BirdTypeInfo, 0, len(birdTypes))
	for _, birdType := range birdTypes {
		stats, err := gameobject.BirdStatsForType(birdType)
		if err != nil {
			continue
		}
		attack, err := gameobject.AttackTypeForBirdType(birdType)
		if err != nil {
			continue
		}
		catalog = append(catalog, BirdTypeInfo{
			Type:   birdType,
			Stats:  stats,
			Attack: attack,
		})
	}
	return catalog
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

func placedBirdStates(birds []placedBird) []PlacedBirdState {
	states := make([]PlacedBirdState, 0, len(birds))
	for _, placed := range birds {
		states = append(states, PlacedBirdState{
			ID:              placed.bird.ID,
			Type:            placed.birdType,
			Position:        placed.bird.Position,
			Stats:           placed.bird.Stats,
			LastFiredAtTick: placed.bird.LastFiredAtTick,
		})
	}
	return states
}

func smogStates(smogs []gameobject.Smog) []SmogState {
	states := make([]SmogState, 0, len(smogs))
	for _, smog := range smogs {
		states = append(states, SmogState{
			ID:        smog.ID,
			Health:    smog.Health,
			Position:  smog.Position,
			Speed:     smog.Speed,
			PathIndex: smog.PathIndex,
		})
	}
	return states
}

func projectileStates(projectiles []gameobject.Projectile) []ProjectileState {
	states := make([]ProjectileState, 0, len(projectiles))
	for _, projectile := range projectiles {
		states = append(states, ProjectileState{
			ID:              projectile.ID,
			Type:            projectile.Type,
			Damage:          projectile.Damage,
			ProjectileSpeed: projectile.ProjectileSpeed,
			Position:        projectile.Position,
			Direction:       projectile.Direction,
			TargetID:        projectile.TargetID,
			RemainingRange:  projectile.RemainingRange,
			HitRadius:       projectile.HitRadius,
		})
	}
	return states
}

func quizPromptState(quiz quizcache.CachedQuiz, remaining int) QuizPromptState {
	return QuizPromptState{
		QuizID:           quiz.ID,
		QuizType:         quiz.QuizType,
		QuestionMarkdown: quiztext.SanitizeMarkdown(quiz.QuestionMarkdown),
		OptionsMarkdown:  quiztext.SanitizeMarkdownSlice(quiz.OptionsMarkdown),
		Remaining:        remaining,
	}
}

func storedBirds(birds []placedBird) []gamesession.StoredBird {
	stored := make([]gamesession.StoredBird, 0, len(birds))
	for _, placed := range birds {
		stored = append(stored, gamesession.StoredBird{
			ID:              placed.bird.ID,
			Type:            placed.birdType,
			Position:        placed.bird.Position,
			Stats:           placed.bird.Stats,
			LastFiredAtTick: placed.bird.LastFiredAtTick,
		})
	}
	return stored
}

func storedSmogs(smogs []gameobject.Smog) []gamesession.StoredSmog {
	stored := make([]gamesession.StoredSmog, 0, len(smogs))
	for _, smog := range smogs {
		stored = append(stored, gamesession.StoredSmog{
			ID:        smog.ID,
			Health:    smog.Health,
			Position:  smog.Position,
			Speed:     smog.Speed,
			PathIndex: smog.PathIndex,
		})
	}
	return stored
}

func smogsFromStored(stored []gamesession.StoredSmog) []gameobject.Smog {
	smogs := make([]gameobject.Smog, 0, len(stored))
	for _, item := range stored {
		smogs = append(smogs, gameobject.Smog{
			ID:        item.ID,
			Health:    item.Health,
			Position:  item.Position,
			Speed:     item.Speed,
			PathIndex: item.PathIndex,
		})
	}
	return smogs
}

func storedProjectiles(projectiles []gameobject.Projectile) []gamesession.StoredProjectile {
	stored := make([]gamesession.StoredProjectile, 0, len(projectiles))
	for _, projectile := range projectiles {
		stored = append(stored, gamesession.StoredProjectile{
			ID:              projectile.ID,
			Type:            projectile.Type,
			Damage:          projectile.Damage,
			ProjectileSpeed: projectile.ProjectileSpeed,
			Position:        projectile.Position,
			Direction:       projectile.Direction,
			TargetID:        projectile.TargetID,
			RemainingRange:  projectile.RemainingRange,
			HitRadius:       projectile.HitRadius,
		})
	}
	return stored
}

func projectilesFromStored(stored []gamesession.StoredProjectile) []gameobject.Projectile {
	projectiles := make([]gameobject.Projectile, 0, len(stored))
	for _, item := range stored {
		projectiles = append(projectiles, gameobject.Projectile{
			ID:              item.ID,
			Type:            item.Type,
			Damage:          item.Damage,
			ProjectileSpeed: item.ProjectileSpeed,
			Position:        item.Position,
			Direction:       item.Direction,
			TargetID:        item.TargetID,
			RemainingRange:  item.RemainingRange,
			HitRadius:       item.HitRadius,
		})
	}
	return projectiles
}

func placedBirdsFromStored(stored []gamesession.StoredBird) ([]placedBird, error) {
	birds := make([]placedBird, 0, len(stored))
	for _, item := range stored {
		behaviour, err := gameobject.AttackBehaviourForType(item.Type)
		if err != nil {
			return nil, err
		}
		birds = append(birds, placedBird{
			birdType: item.Type,
			bird: gameobject.Bird{
				ID:              item.ID,
				Position:        item.Position,
				Stats:           item.Stats,
				AttackBehaviour: behaviour,
				LastFiredAtTick: item.LastFiredAtTick,
			},
		})
	}
	return birds, nil
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

func writeGameOver(conn *websocket.Conn, writeMu *sync.Mutex, runtime runtimeSession, reason string) error {
	return writeWebsocketJSON(conn, writeMu, Message{
		Type: "game.over",
		Data: GameOverState{
			SessionID: runtime.session.SessionID,
			LevelID:   runtime.session.LevelID,
			Health:    runtime.session.Health,
			Wave:      runtime.session.Wave,
			Tick:      runtime.session.Tick,
			Reason:    reason,
		},
	})
}

func writeGameVictory(conn *websocket.Conn, writeMu *sync.Mutex, runtime runtimeSession, reason string) error {
	return writeWebsocketJSON(conn, writeMu, Message{
		Type: "game.victory",
		Data: GameVictoryState{
			SessionID: runtime.session.SessionID,
			LevelID:   runtime.session.LevelID,
			Health:    runtime.session.Health,
			Wave:      runtime.session.Wave,
			Tick:      runtime.session.Tick,
			Reason:    reason,
		},
	})
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

func quizMistakeSummaryState(levelID string, mistakes []repository.QuizMistakeSummaryItem) QuizMistakeSummaryState {
	items := make([]QuizMistakeSummaryItem, 0, len(mistakes))
	for _, mistake := range mistakes {
		options := quiztext.SanitizeMarkdownSlice(mistake.OptionsMarkdown)
		items = append(items, QuizMistakeSummaryItem{
			ID:                     mistake.ID,
			LevelID:                mistake.LevelID,
			GenerationID:           mistake.GenerationID,
			QuizID:                 mistake.QuizID,
			QuizIndex:              mistake.QuizIndex,
			QuizType:               mistake.QuizType,
			QuestionMarkdown:       quiztext.SanitizeMarkdown(mistake.QuestionMarkdown),
			OptionsMarkdown:        options,
			AnswerIndex:            mistake.AnswerIndex,
			SelectedIndex:          mistake.SelectedIndex,
			CorrectOptionMarkdown:  optionMarkdown(options, mistake.AnswerIndex),
			SelectedOptionMarkdown: optionMarkdown(options, mistake.SelectedIndex),
			CreatedAt:              mistake.CreatedAt,
		})
	}
	return QuizMistakeSummaryState{
		LevelID:  levelID,
		Count:    len(items),
		Mistakes: items,
	}
}

func optionMarkdown(options []string, index int) string {
	if index < 0 || index >= len(options) {
		return ""
	}
	return options[index]
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
