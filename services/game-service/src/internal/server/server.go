package server

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"math"
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
	"skybloom/game-service/internal/quizflow"
	"skybloom/game-service/internal/quiztext"
	"skybloom/game-service/internal/repository"
)

var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

const gameTickInterval = 50 * time.Millisecond
const gameTicksPerSecond = 20.0

const placeTowerAction = "place_tower"
const mergeTowerAction = "merge_tower"
const useConsumableAction = "use_consumable"
const grantConsumableAction = "grant_consumable"
const validateConsumableAcquireAction = "validate_consumable_acquire"
const markConsumableQuizPendingAction = "mark_consumable_quiz_pending"
const finishConsumableQuizAction = "finish_consumable_quiz"
const awardQuizEssenceAction = "award_quiz_essence"
const markQuizStartedAction = "mark_quiz_started"
const pauseGameAction = "pause_game"
const resumeGameAction = "resume_game"
const startGameAction = "start_game"

const (
	waveClearDelayTicks          = int64(60)
	enemySpawnIntervalTicks      = int64(22)
	minEnemySpawnIntervalTicks   = int64(8)
	groupGapTicks                = int64(160)
	baseHealthDamage             = 10
	correctQuizEssenceAward      = 50
	quizRequestCooldown          = 30 * time.Second
	airstrikeUseCooldown         = 30 * time.Second
	airstrikeWrongAnswerCooldown = 15 * time.Second
	airstrikeItemType            = "airstrike"
	consumableStatusEmpty        = "empty"
	consumableStatusReady        = "ready"
	consumableStatusQuizPending  = "quiz_pending"
	airstrikeTargetCount         = 3
	airstrikeDamage              = 80.0
	airstrikeRadius              = 2.0
)

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
	AcquireRefillLease(ctx context.Context, generationID string, leaseValue string, ttl time.Duration) (bool, error)
	Get(ctx context.Context, generationID string) (quizcache.LevelQuizzes, error)
	PeekRandom(ctx context.Context, generationID string) (quizcache.CachedQuiz, int, error)
	ReleaseRefillLease(ctx context.Context, generationID string, leaseValue string) error
	Take(ctx context.Context, generationID string, quizID string) (quizcache.CachedQuiz, int, error)
	Set(ctx context.Context, generationID string, quizzes quizcache.LevelQuizzes) error
	Delete(ctx context.Context, generationID string) error
}

type GameSessionStore interface {
	Start(ctx context.Context, options gamesession.StartOptions) (gamesession.State, error)
	LoadRuntimeState(ctx context.Context, sessionID string) (gamesession.RuntimeState, error)
	SaveRuntimeState(ctx context.Context, sessionID string, runtime gamesession.RuntimeState) error
	SaveQuizMistake(ctx context.Context, sessionID string, userID string, mistake gamesession.QuizMistake) error
	ListQuizMistakes(ctx context.Context, sessionID string, userID string) ([]gamesession.QuizMistake, error)
	ClearQuizMistakes(ctx context.Context, sessionID string, userID string) error
	Delete(ctx context.Context, sessionID string) error
}

type QuizRefillPublisher interface {
	Publish(ctx context.Context, messageID string, value any) error
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
	refills  QuizRefillPublisher
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
	SessionID                    string            `json:"session_id"`
	LevelID                      string            `json:"level_id"`
	Health                       int               `json:"health"`
	Essence                      int               `json:"essence"`
	Wave                         int               `json:"wave"`
	Tick                         int64             `json:"tick"`
	ServerTime                   time.Time         `json:"server_time"`
	LoopStarted                  bool              `json:"loop_started"`
	LoopPaused                   bool              `json:"loop_paused"`
	QuizCooldownRemainingSeconds int               `json:"quiz_cooldown_remaining_seconds"`
	BirdTypes                    []BirdTypeInfo    `json:"bird_types,omitempty"`
	EnemyTypes                   []EnemyTypeInfo   `json:"enemy_types,omitempty"`
	Birds                        []PlacedBirdState `json:"birds"`
	Enemies                      []EnemyState      `json:"enemies"`
	Projectiles                  []ProjectileState `json:"projectiles"`
	Consumables                  ConsumableState   `json:"consumables"`
	Events                       []GameEvent       `json:"events,omitempty"`
}

type BirdTypeInfo struct {
	Type   string               `json:"type"`
	Stats  gameobject.BirdStats `json:"stats"`
	Attack string               `json:"attack"`
}

type EnemyTypeInfo struct {
	Type  string                `json:"type"`
	Stats gameobject.EnemyStats `json:"stats"`
}

type PlacedBirdState struct {
	ID              string               `json:"id"`
	Type            string               `json:"type"`
	Position        gameobject.Position  `json:"position"`
	Stats           gameobject.BirdStats `json:"stats"`
	LastFiredAtTick int64                `json:"last_fired_at_tick"`
}

type EnemyState struct {
	ID        string              `json:"id"`
	Type      string              `json:"type"`
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

type ConsumableState struct {
	Airstrike ConsumableItemState `json:"airstrike"`
}

type ConsumableItemState struct {
	Status                   string `json:"status"`
	Charges                  int    `json:"charges"`
	PendingQuizID            string `json:"pending_quiz_id,omitempty"`
	CooldownRemainingSeconds int    `json:"cooldown_remaining_seconds"`
}

type ConsumableDeploymentState struct {
	DeploymentID string                `json:"deployment_id"`
	ItemType     string                `json:"item_type"`
	Targets      []gameobject.Position `json:"targets"`
}

type GameEvent struct {
	Type          string                `json:"type"`
	BirdID        string                `json:"bird_id,omitempty"`
	EnemyID       string                `json:"enemy_id,omitempty"`
	ProjectileID  string                `json:"projectile_id,omitempty"`
	ProjectileIDs []string              `json:"projectile_ids,omitempty"`
	DeploymentID  string                `json:"deployment_id,omitempty"`
	ItemType      string                `json:"item_type,omitempty"`
	Targets       []gameobject.Position `json:"targets,omitempty"`
	Damage        float64               `json:"damage,omitempty"`
	Health        int                   `json:"health,omitempty"`
	Wave          int                   `json:"wave,omitempty"`
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

type consumableResolution struct {
	DeploymentID string                `json:"deployment_id"`
	ItemType     string                `json:"item_type"`
	Targets      []gameobject.Position `json:"targets"`
	Consumables  ConsumableState       `json:"consumables"`
	Enemies      []EnemyState          `json:"enemies"`
	Events       []GameEvent           `json:"events"`
}

type ConsumableQuizPromptState struct {
	ItemType string `json:"item_type"`
	quizflow.Prompt
}

type ConsumableQuizResultState struct {
	ItemType               string          `json:"item_type"`
	QuizID                 string          `json:"quiz_id"`
	Correct                bool            `json:"correct"`
	SelectedIndex          int             `json:"selected_index"`
	CorrectIndex           int             `json:"correct_index"`
	SelectedOptionMarkdown string          `json:"selected_option_markdown"`
	CorrectOptionMarkdown  string          `json:"correct_option_markdown"`
	ConsumableAwarded      int             `json:"consumable_awarded"`
	Consumables            ConsumableState `json:"consumables"`
	Remaining              int             `json:"remaining"`
}

type QuizPromptState = quizflow.Prompt

type QuizUnavailableState = quizflow.Unavailable

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

type useConsumableRequest struct {
	ItemType string                `json:"item_type"`
	Targets  []gameobject.Position `json:"targets"`
}

type consumableAcquireRequest struct {
	ItemType string `json:"item_type"`
}

type consumableQuizAnswerRequest struct {
	ItemType      string `json:"item_type"`
	QuizID        string `json:"quiz_id"`
	SelectedIndex int    `json:"selected_index"`
}

type grantConsumableRequest struct {
	ItemType string
	Charges  int
}

type finishConsumableQuizRequest struct {
	ItemType string
	QuizID   string
	Correct  bool
	Charges  int
}

type consumableQuizPendingRequest struct {
	ItemType string
	QuizID   string
}

type mergeTowerRequest struct {
	SourceBirdID   string `json:"source_bird_id,omitempty"`
	SourceBirdType string `json:"source_bird_type,omitempty"`
	TargetBirdID   string `json:"target_bird_id"`
}

type gameExitRequest struct {
	SessionID string `json:"session_id"`
}

type quizAnswerRequest struct {
	QuizID        string `json:"quiz_id"`
	SelectedIndex int    `json:"selected_index"`
}

type clientAction struct {
	Type                 string
	PlaceTower           placeTowerRequest
	MergeTower           mergeTowerRequest
	UseConsumable        useConsumableRequest
	GrantConsumable      grantConsumableRequest
	ConsumableQuiz       consumableQuizPendingRequest
	FinishConsumableQuiz finishConsumableQuizRequest
	EssenceReward        int
	QuizStartedAt        time.Time
	Result               chan actionResult
}

type actionResult struct {
	Essence     int
	Consumables ConsumableState
	Err         error
}

type runningGameLoop struct {
	sessionID               string
	levelID                 string
	generationID            string
	subChapterID            string
	userID                  string
	currentQuizID           string
	currentConsumableQuizID string
	lastQuizStartedAt       time.Time
	consumableCooldownUntil time.Time
	loopStarted             bool
	stop                    context.CancelFunc
	actions                 chan clientAction
	done                    chan struct{}
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
	session                 gamesession.State
	economy                 gamesession.Economy
	birds                   []placedBird
	enemies                 []gameobject.Enemy
	projectiles             []gameobject.Projectile
	consumables             ConsumableState
	levelMap                mapgen.GeneratedMap
	path                    []gameobject.Position
	loopStarted             bool
	loopPaused              bool
	lastQuizStartedAt       time.Time
	consumableCooldownUntil time.Time

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
	return NewWithGenerationCachesSessionsAndRefills(cfg, levels, maps, quizzes, jobs, starter, statuses, sessions, nil)
}

func NewWithGenerationCachesSessionsAndRefills(
	cfg config.Config,
	levels LevelRepository,
	maps MapCache,
	quizzes QuizCache,
	jobs GenerationRepository,
	starter GenerationStarter,
	statuses GenerationStatusStore,
	sessions GameSessionStore,
	refills QuizRefillPublisher,
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
		refills:  refills,
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
		case "game.consumable.acquire":
			request, err := decodeConsumableAcquire(message.Data)
			if err != nil {
				if writeErr := writeWebsocketJSON(conn, writeMu, Message{Type: "error", Data: map[string]string{"error": err.Error()}}); writeErr != nil {
					log.Printf("websocket error write failed: %v", writeErr)
					return
				}
				continue
			}
			if err := s.handleConsumableAcquire(ctx, conn, writeMu, gameLoop, request); err != nil {
				log.Printf("game.consumable.acquire failed user_id=%s: %v", userID, err)
				if writeErr := writeWebsocketJSON(conn, writeMu, Message{Type: "error", Data: map[string]string{"error": err.Error()}}); writeErr != nil {
					log.Printf("websocket error write failed: %v", writeErr)
					return
				}
			}
		case "game.consumable.quiz.answer":
			request, err := decodeConsumableQuizAnswer(message.Data)
			if err != nil {
				if writeErr := writeWebsocketJSON(conn, writeMu, Message{Type: "error", Data: map[string]string{"error": err.Error()}}); writeErr != nil {
					log.Printf("websocket error write failed: %v", writeErr)
					return
				}
				continue
			}
			if err := s.handleConsumableQuizAnswer(ctx, conn, writeMu, gameLoop, request); err != nil {
				log.Printf("game.consumable.quiz.answer failed user_id=%s: %v", userID, err)
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
		case "game.session.run":
			if gameLoop != nil && !gameLoop.stopped() {
				select {
				case gameLoop.actions <- clientAction{Type: startGameAction}:
					gameLoop.loopStarted = true
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
		case "game.action.merge_tower":
			action, err := decodeMergeTowerAction(message.Data)
			if err != nil {
				if writeErr := writeActionRejected(conn, writeMu, mergeTowerAction, err.Error()); writeErr != nil {
					log.Printf("websocket action rejection write failed: %v", writeErr)
					return
				}
				continue
			}
			if gameLoop == nil || gameLoop.stopped() {
				gameLoop = nil
				if writeErr := writeActionRejected(conn, writeMu, mergeTowerAction, "game session is not running"); writeErr != nil {
					log.Printf("websocket action rejection write failed: %v", writeErr)
					return
				}
				continue
			}
			select {
			case gameLoop.actions <- clientAction{Type: mergeTowerAction, MergeTower: action}:
			default:
				if writeErr := writeActionRejected(conn, writeMu, mergeTowerAction, "action queue is full"); writeErr != nil {
					log.Printf("websocket action rejection write failed: %v", writeErr)
					return
				}
			}
		case "game.action.use_consumable":
			action, err := decodeUseConsumableAction(message.Data)
			if err != nil {
				if writeErr := writeActionRejected(conn, writeMu, useConsumableAction, err.Error()); writeErr != nil {
					log.Printf("websocket action rejection write failed: %v", writeErr)
					return
				}
				continue
			}
			if gameLoop == nil || gameLoop.stopped() {
				gameLoop = nil
				if writeErr := writeActionRejected(conn, writeMu, useConsumableAction, "game session is not running"); writeErr != nil {
					log.Printf("websocket action rejection write failed: %v", writeErr)
					return
				}
				continue
			}
			select {
			case gameLoop.actions <- clientAction{Type: useConsumableAction, UseConsumable: action}:
			default:
				if writeErr := writeActionRejected(conn, writeMu, useConsumableAction, "action queue is full"); writeErr != nil {
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
	if err := s.sessions.ClearQuizMistakes(callCtx, session.SessionID, userID); err != nil && !errors.Is(err, gamesession.ErrSessionNotFound) {
		log.Printf("failed to clear quiz mistakes session_id=%s user_id=%s: %v", session.SessionID, userID, err)
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
	session.Health = storedRuntime.Health
	session.Essence = storedRuntime.Essence
	session.Wave = storedRuntime.Wave
	session.Tick = storedRuntime.Tick

	runtime := runtimeSession{
		session:     session,
		economy:     gamesession.NewEconomy(session.Essence),
		birds:       restoredBirds,
		enemies:     enemiesFromStored(storedRuntime.Enemies),
		projectiles: projectilesFromStored(storedRuntime.Projectiles),
		consumables: consumablesFromStored(storedRuntime.Consumables),
		levelMap:    levelMap,
		path:        gamePath(levelMap),
		loopStarted: storedRuntime.LoopStarted,
		loopPaused:  storedRuntime.LoopPaused,

		lastQuizStartedAt:       storedRuntime.LastQuizStartedAt,
		consumableCooldownUntil: storedRuntime.ConsumableCooldownUntil,
		waveStartedAtTick:       storedRuntime.WaveStartedAtTick,
		waveSpawned:             storedRuntime.WaveSpawned,
		nextWaveTick:            storedRuntime.NextWaveTick,
	}
	if runtime.session.Health > 0 && !gameWon(runtime) {
		runtime.loopStarted = false
		runtime.loopPaused = false
		if err := s.saveRuntimeState(callCtx, runtime); err != nil {
			log.Printf("game session runtime start save failed session_id=%s: %v", runtime.session.SessionID, err)
			return nil, errors.New("failed to start game session")
		}
	}
	state := gameStateFromRuntime(runtime, nil, session.UpdatedAt, birdTypeCatalog(), enemyTypeCatalog(), nil)
	if err := writeWebsocketJSON(conn, writeMu, Message{Type: "game.session.started", Data: state}); err != nil {
		return nil, err
	}
	if runtime.session.Health <= 0 {
		s.cleanupQuizCacheAfterGameEnd(ctx, runtime, "health_depleted")
		if err := writeGameOver(conn, writeMu, runtime, "health_depleted"); err != nil {
			return nil, err
		}
		return nil, nil
	}
	if gameWon(runtime) {
		s.cleanupQuizCacheAfterGameEnd(ctx, runtime, "all_waves_cleared")
		if err := writeGameVictory(conn, writeMu, runtime, "all_waves_cleared"); err != nil {
			return nil, err
		}
		return nil, nil
	}

	currentQuizID := ""
	if runtime.consumables.Airstrike.PendingQuizID == "" && s.quizzes != nil {
		if quizzes, err := s.quizzes.Get(callCtx, runtime.session.GenerationID); err == nil {
			currentQuizID = strings.TrimSpace(quizzes.CurrentQuizID)
		} else if !errors.Is(err, quizcache.ErrQuizzesNotFound) {
			log.Printf("failed to restore current quiz id session_id=%s generation_id=%s: %v", runtime.session.SessionID, runtime.session.GenerationID, err)
		}
	}

	loopCtx, stop := context.WithCancel(ctx)
	loop := &runningGameLoop{
		sessionID:               runtime.session.SessionID,
		levelID:                 runtime.session.LevelID,
		generationID:            runtime.session.GenerationID,
		subChapterID:            runtime.session.SubChapterID,
		userID:                  runtime.session.UserID,
		currentQuizID:           currentQuizID,
		currentConsumableQuizID: runtime.consumables.Airstrike.PendingQuizID,
		lastQuizStartedAt:       runtime.lastQuizStartedAt,
		consumableCooldownUntil: runtime.consumableCooldownUntil,
		loopStarted:             runtime.loopStarted,
		stop:                    stop,
		actions:                 make(chan clientAction, 64),
		done:                    make(chan struct{}),
	}
	go s.runGameLoop(loopCtx, conn, writeMu, runtime, loop)
	return loop, nil
}

func (s *Server) runGameLoop(ctx context.Context, conn *websocket.Conn, writeMu *sync.Mutex, runtime runtimeSession, loop *runningGameLoop) {
	ticker := time.NewTicker(gameTickInterval)
	defer ticker.Stop()
	defer close(loop.done)

	for {
		select {
		case <-ctx.Done():
			return
		case action := <-loop.actions:
			switch action.Type {
			case placeTowerAction, mergeTowerAction:
				if err := s.processClientAction(ctx, &runtime, action); err != nil {
					if writeErr := writeActionRejected(conn, writeMu, action.Type, err.Error()); writeErr != nil {
						log.Printf("websocket action rejection write failed: %v", writeErr)
						return
					}
					continue
				}
				removedBirdIDs := []string(nil)
				if action.Type == mergeTowerAction {
					removedBirdIDs = removedBirdIDsForMerge(action.MergeTower)
				}
				if err := writeActionAccepted(conn, writeMu, action.Type, runtime.birds[len(runtime.birds)-1], removedBirdIDs); err != nil {
					log.Printf("websocket action accepted write failed: %v", err)
					return
				}
			case useConsumableAction:
				_, err := resolveConsumableAction(&runtime, action.UseConsumable)
				if err != nil {
					if writeErr := writeActionRejected(conn, writeMu, action.Type, err.Error()); writeErr != nil {
						log.Printf("websocket action rejection write failed: %v", writeErr)
						return
					}
					continue
				}
			case grantConsumableAction:
				err := s.grantConsumable(ctx, &runtime, action.GrantConsumable)
				if action.Result != nil {
					action.Result <- actionResult{Consumables: runtime.consumables, Err: err}
				}
			case validateConsumableAcquireAction:
				err := validateConsumableAcquire(&runtime, action.ConsumableQuiz.ItemType)
				if action.Result != nil {
					action.Result <- actionResult{Err: err}
				}
			case markConsumableQuizPendingAction:
				err := s.markConsumableQuizPending(ctx, &runtime, action.ConsumableQuiz)
				if action.Result != nil {
					action.Result <- actionResult{Consumables: runtime.consumables, Err: err}
				}
			case finishConsumableQuizAction:
				consumables, err := s.finishConsumableQuiz(ctx, &runtime, action.FinishConsumableQuiz)
				if action.Result != nil {
					action.Result <- actionResult{Consumables: consumables, Err: err}
				}
			case awardQuizEssenceAction:
				essence, err := s.awardEssence(ctx, &runtime, action.EssenceReward)
				if action.Result != nil {
					action.Result <- actionResult{Essence: essence, Err: err}
				}
			case markQuizStartedAction:
				runtime.lastQuizStartedAt = action.QuizStartedAt.UTC()
				err := s.saveRuntimeState(ctx, runtime)
				if action.Result != nil {
					action.Result <- actionResult{Err: err}
				}
			case pauseGameAction:
				runtime.loopPaused = true
			case resumeGameAction:
				runtime.loopPaused = false
			case startGameAction:
				runtime.loopStarted = true
				s.saveRuntimeState(ctx, runtime)
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
			if err := writeWebsocketJSON(conn, writeMu, Message{Type: "game.state", Data: gameStateFromRuntime(runtime, loop, now, nil, nil, events)}); err != nil {
				log.Printf("game state write failed session_id=%s: %v", runtime.session.SessionID, err)
				return
			}
			if runtime.session.Health <= 0 {
				s.cleanupQuizCacheAfterGameEnd(ctx, runtime, "health_depleted")
				if err := writeGameOver(conn, writeMu, runtime, "health_depleted"); err != nil {
					log.Printf("game over write failed session_id=%s: %v", runtime.session.SessionID, err)
				}
				return
			}
			if gameWon(runtime) {
				s.cleanupQuizCacheAfterGameEnd(ctx, runtime, "all_waves_cleared")
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
	generationID := ""
	if loop == nil {
		if sessionID == "" {
			return GameExitedState{Deleted: false, Reason: "no_running_session"}, nil
		}
	} else {
		sessionID = loop.sessionID
		generationID = loop.generationID
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
	if loop == nil {
		if runtime, err := s.sessions.LoadRuntimeState(callCtx, sessionID); err == nil {
			generationID = runtime.GenerationID
		} else if !errors.Is(err, gamesession.ErrSessionNotFound) {
			log.Printf("failed to load game session runtime for quiz cache cleanup session_id=%s: %v", sessionID, err)
		}
	}

	if err := s.deleteQuizCache(callCtx, generationID); err != nil {
		log.Printf("failed to clear level quiz cache generation_id=%s: %v", generationID, err)
		return GameExitedState{}, errors.New("failed to clear level quizzes")
	}
	if err := s.sessions.Delete(callCtx, sessionID); err != nil {
		return GameExitedState{}, errors.New("failed to exit game session")
	}
	return GameExitedState{SessionID: sessionID, Deleted: true, Reason: "client_exit"}, nil
}

func (s *Server) deleteQuizCache(ctx context.Context, generationID string) error {
	generationID = strings.TrimSpace(generationID)
	if s.quizzes == nil || generationID == "" {
		return nil
	}
	return s.quizzes.Delete(ctx, generationID)
}

func (s *Server) cleanupQuizCacheAfterGameEnd(ctx context.Context, runtime runtimeSession, reason string) {
	if strings.TrimSpace(runtime.session.GenerationID) == "" {
		return
	}
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := s.deleteQuizCache(callCtx, runtime.session.GenerationID); err != nil {
		log.Printf(
			"failed to clear quiz cache after game end session_id=%s generation_id=%s reason=%s: %v",
			runtime.session.SessionID,
			runtime.session.GenerationID,
			reason,
			err,
		)
	}
}

func (s *Server) maybeTriggerQuizRefill(loop *runningGameLoop, remaining int) {
	if loop == nil || s.quizzes == nil {
		return
	}
	if s.refills == nil {
		log.Printf("quiz refill skipped because publisher is not configured generation_id=%s level_id=%s remaining=%d", loop.generationID, loop.levelID, remaining)
		return
	}
	threshold := s.config.QuizRefillThreshold
	if threshold < 0 || remaining > threshold {
		return
	}
	refillID := uuid.NewString()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		acquired, err := s.quizzes.AcquireRefillLease(ctx, loop.generationID, refillID, s.config.QuizRefillLeaseTTL)
		if err != nil {
			log.Printf("quiz refill lease acquire failed generation_id=%s level_id=%s: %v", loop.generationID, loop.levelID, err)
			return
		}
		if !acquired {
			log.Printf("quiz refill already pending generation_id=%s level_id=%s remaining=%d", loop.generationID, loop.levelID, remaining)
			return
		}

		job := models.LevelJob{
			JobType:             models.JobTypeQuizRefill,
			TaskID:              refillID,
			GenerationID:        loop.generationID,
			LevelID:             loop.levelID,
			UserID:              loop.userID,
			SubChapterID:        loop.subChapterID,
			RefillID:            refillID,
			MaxQuizCount:        s.config.MaxQuizzesPerLevel,
			MapAlgorithmVersion: mapgen.Version,
		}
		if err := s.refills.Publish(ctx, refillID, job); err != nil {
			log.Printf("quiz refill publish failed generation_id=%s level_id=%s: %v", loop.generationID, loop.levelID, err)
			if releaseErr := s.quizzes.ReleaseRefillLease(context.Background(), loop.generationID, refillID); releaseErr != nil {
				log.Printf("quiz refill lease release after publish failure failed generation_id=%s refill_id=%s: %v", loop.generationID, refillID, releaseErr)
			}
			return
		}
		log.Printf("quiz refill queued generation_id=%s level_id=%s remaining=%d threshold=%d refill_id=%s", loop.generationID, loop.levelID, remaining, threshold, refillID)
	}()
}

func (s *Server) quizService() *quizflow.Service {
	return quizflow.NewService(s.quizzes, quizRequestCooldown)
}

func (s *Server) handleQuizRequest(ctx context.Context, conn *websocket.Conn, writeMu *sync.Mutex, loop *runningGameLoop) error {
	if loop == nil || loop.stopped() {
		return writeWebsocketJSON(conn, writeMu, Message{
			Type: "game.quiz.unavailable",
			Data: QuizUnavailableState{Reason: quizflow.UnavailableGameNotStarted},
		})
	}
	if !loop.loopStarted {
		return writeWebsocketJSON(conn, writeMu, Message{
			Type: "game.quiz.unavailable",
			Data: QuizUnavailableState{Reason: quizflow.UnavailableGameNotStarted},
		})
	}
	if loop.currentConsumableQuizID != "" {
		return writeWebsocketJSON(conn, writeMu, Message{
			Type: "game.quiz.unavailable",
			Data: QuizUnavailableState{Reason: "consumable_quiz_pending"},
		})
	}
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	result, err := s.quizService().Present(callCtx, quizflow.PresentRequest{
		GenerationID:      loop.generationID,
		CurrentQuizID:     loop.currentQuizID,
		LastQuizStartedAt: loop.lastQuizStartedAt,
		Now:               time.Now().UTC(),
		RequireCooldown:   true,
	})
	if err != nil {
		return err
	}
	if result.Unavailable.Reason != "" {
		if result.Unavailable.Reason == quizflow.UnavailableNoQuizzesRemaining {
			loop.currentQuizID = ""
		}
		s.maybeTriggerQuizRefill(loop, result.Remaining)
		return writeWebsocketJSON(conn, writeMu, Message{
			Type: "game.quiz.unavailable",
			Data: result.Unavailable,
		})
	}
	if result.CurrentQuizID == "" {
		loop.currentQuizID = ""
		return writeWebsocketJSON(conn, writeMu, Message{
			Type: "game.quiz.unavailable",
			Data: QuizUnavailableState{Reason: quizflow.UnavailableNoQuizzesRemaining},
		})
	}
	loop.currentQuizID = result.CurrentQuizID
	s.maybeTriggerQuizRefill(loop, result.Remaining)
	return writeWebsocketJSON(conn, writeMu, Message{
		Type: "game.quiz.presented",
		Data: result.Prompt,
	})
}

func (s *Server) handleQuizAnswer(ctx context.Context, conn *websocket.Conn, writeMu *sync.Mutex, loop *runningGameLoop, request quizAnswerRequest) error {
	if loop == nil || loop.stopped() {
		return errors.New("game session is not running")
	}

	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	result, err := s.quizService().Answer(callCtx, quizflow.AnswerRequest{
		GenerationID:  loop.generationID,
		CurrentQuizID: loop.currentQuizID,
		QuizID:        request.QuizID,
		SelectedIndex: request.SelectedIndex,
	})
	if err != nil {
		return err
	}
	if result.Unavailable.Reason != "" {
		loop.currentQuizID = ""
		s.maybeTriggerQuizRefill(loop, result.Remaining)
		return writeWebsocketJSON(conn, writeMu, Message{
			Type: "game.quiz.unavailable",
			Data: result.Unavailable,
		})
	}
	if loop.currentQuizID == request.QuizID {
		loop.currentQuizID = ""
	}
	s.maybeTriggerQuizRefill(loop, result.Remaining)
	answeredAt := time.Now().UTC()
	if err := s.markQuizStartedThroughLoop(ctx, loop, answeredAt); err != nil {
		return err
	}
	loop.lastQuizStartedAt = answeredAt

	essenceAwarded := 0
	essence := 0
	if result.Correct {
		essenceAwarded = correctQuizEssenceAward
		essence, err = s.awardEssenceThroughLoop(ctx, loop, correctQuizEssenceAward)
		if err != nil {
			return err
		}
	}

	if err := writeWebsocketJSON(conn, writeMu, Message{
		Type: "game.quiz.result",
		Data: QuizResultState{
			QuizID:                 result.Quiz.ID,
			Correct:                result.Correct,
			SelectedIndex:          result.SelectedIndex,
			CorrectIndex:           result.CorrectIndex,
			SelectedOptionMarkdown: result.SelectedOptionMarkdown,
			CorrectOptionMarkdown:  result.CorrectOptionMarkdown,
			EssenceAwarded:         essenceAwarded,
			Essence:                essence,
			Remaining:              result.Remaining,
		},
	}); err != nil {
		return err
	}

	if !result.Correct {
		s.saveQuizMistakeAsync(loop, result.Quiz, request.SelectedIndex)
	}

	return nil
}

func (s *Server) handleConsumableAcquire(ctx context.Context, conn *websocket.Conn, writeMu *sync.Mutex, loop *runningGameLoop, request consumableAcquireRequest) error {
	if loop == nil || loop.stopped() {
		return writeWebsocketJSON(conn, writeMu, Message{
			Type: "game.quiz.unavailable",
			Data: QuizUnavailableState{Reason: quizflow.UnavailableGameNotStarted},
		})
	}
	if !loop.loopStarted {
		return writeWebsocketJSON(conn, writeMu, Message{
			Type: "game.quiz.unavailable",
			Data: QuizUnavailableState{Reason: quizflow.UnavailableGameNotStarted},
		})
	}
	if request.ItemType != airstrikeItemType {
		return errors.New("unknown consumable type")
	}
	if loop.currentQuizID != "" {
		return writeWebsocketJSON(conn, writeMu, Message{
			Type: "game.quiz.unavailable",
			Data: QuizUnavailableState{Reason: "progression_quiz_pending"},
		})
	}
	if loop.currentConsumableQuizID == "" {
		if err := s.validateConsumableAcquireThroughLoop(ctx, loop, request.ItemType); err != nil {
			if writeErr := writeActionRejected(conn, writeMu, validateConsumableAcquireAction, err.Error()); writeErr != nil {
				return writeErr
			}
			return nil
		}
	}

	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	result, err := s.quizService().Present(callCtx, quizflow.PresentRequest{
		GenerationID:    loop.generationID,
		CurrentQuizID:   loop.currentConsumableQuizID,
		Now:             time.Now().UTC(),
		RequireCooldown: false,
	})
	if err != nil {
		return err
	}
	if result.Unavailable.Reason != "" {
		if result.Unavailable.Reason == quizflow.UnavailableNoQuizzesRemaining {
			loop.currentConsumableQuizID = ""
		}
		s.maybeTriggerQuizRefill(loop, result.Remaining)
		return writeWebsocketJSON(conn, writeMu, Message{
			Type: "game.quiz.unavailable",
			Data: result.Unavailable,
		})
	}
	if result.CurrentQuizID == "" {
		loop.currentConsumableQuizID = ""
		return writeWebsocketJSON(conn, writeMu, Message{
			Type: "game.quiz.unavailable",
			Data: QuizUnavailableState{Reason: quizflow.UnavailableNoQuizzesRemaining},
		})
	}
	if loop.currentConsumableQuizID == "" {
		if _, err := s.markConsumableQuizPendingThroughLoop(ctx, loop, consumableQuizPendingRequest{ItemType: request.ItemType, QuizID: result.CurrentQuizID}); err != nil {
			return err
		}
	}
	loop.currentConsumableQuizID = result.CurrentQuizID
	s.maybeTriggerQuizRefill(loop, result.Remaining)
	return writeWebsocketJSON(conn, writeMu, Message{
		Type: "game.consumable.quiz.presented",
		Data: ConsumableQuizPromptState{
			ItemType: request.ItemType,
			Prompt:   result.Prompt,
		},
	})
}

func (s *Server) handleConsumableQuizAnswer(ctx context.Context, conn *websocket.Conn, writeMu *sync.Mutex, loop *runningGameLoop, request consumableQuizAnswerRequest) error {
	if loop == nil || loop.stopped() {
		return errors.New("game session is not running")
	}
	if request.ItemType != airstrikeItemType {
		return errors.New("unknown consumable type")
	}
	if loop.currentConsumableQuizID == "" {
		return errors.New("no consumable quiz is pending")
	}

	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	result, err := s.quizService().Answer(callCtx, quizflow.AnswerRequest{
		GenerationID:  loop.generationID,
		CurrentQuizID: loop.currentConsumableQuizID,
		QuizID:        request.QuizID,
		SelectedIndex: request.SelectedIndex,
	})
	if err != nil {
		return err
	}
	if result.Unavailable.Reason != "" {
		loop.currentConsumableQuizID = ""
		s.maybeTriggerQuizRefill(loop, result.Remaining)
		return writeWebsocketJSON(conn, writeMu, Message{
			Type: "game.quiz.unavailable",
			Data: result.Unavailable,
		})
	}
	s.maybeTriggerQuizRefill(loop, result.Remaining)

	award := 0
	if result.Correct {
		award = 1
	}
	consumables, err := s.finishConsumableQuizThroughLoop(ctx, loop, finishConsumableQuizRequest{
		ItemType: request.ItemType,
		QuizID:   request.QuizID,
		Correct:  result.Correct,
		Charges:  award,
	})
	if err != nil {
		return err
	}
	if loop.currentConsumableQuizID == request.QuizID {
		loop.currentConsumableQuizID = ""
	}

	if err := writeWebsocketJSON(conn, writeMu, Message{
		Type: "game.consumable.quiz.result",
		Data: ConsumableQuizResultState{
			ItemType:               request.ItemType,
			QuizID:                 result.Quiz.ID,
			Correct:                result.Correct,
			SelectedIndex:          result.SelectedIndex,
			CorrectIndex:           result.CorrectIndex,
			SelectedOptionMarkdown: result.SelectedOptionMarkdown,
			CorrectOptionMarkdown:  result.CorrectOptionMarkdown,
			ConsumableAwarded:      award,
			Consumables:            consumables,
			Remaining:              result.Remaining,
		},
	}); err != nil {
		return err
	}

	if !result.Correct {
		s.saveQuizMistakeAsync(loop, result.Quiz, request.SelectedIndex)
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

func (s *Server) markQuizStartedThroughLoop(ctx context.Context, loop *runningGameLoop, startedAt time.Time) error {
	if loop == nil || loop.stopped() {
		return errors.New("game session is not running")
	}
	result := make(chan actionResult, 1)
	action := clientAction{
		Type:          markQuizStartedAction,
		QuizStartedAt: startedAt,
		Result:        result,
	}
	select {
	case loop.actions <- action:
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(2 * time.Second):
		return errors.New("game action queue timed out")
	}

	select {
	case response := <-result:
		return response.Err
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(2 * time.Second):
		return errors.New("game action response timed out")
	}
}

func (s *Server) validateConsumableAcquireThroughLoop(ctx context.Context, loop *runningGameLoop, itemType string) error {
	if loop == nil || loop.stopped() {
		return errors.New("game session is not running")
	}
	result := make(chan actionResult, 1)
	action := clientAction{
		Type:           validateConsumableAcquireAction,
		ConsumableQuiz: consumableQuizPendingRequest{ItemType: itemType},
		Result:         result,
	}
	select {
	case loop.actions <- action:
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(2 * time.Second):
		return errors.New("game action queue timed out")
	}

	select {
	case response := <-result:
		return response.Err
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(2 * time.Second):
		return errors.New("game action response timed out")
	}
}

func (s *Server) markConsumableQuizPendingThroughLoop(ctx context.Context, loop *runningGameLoop, request consumableQuizPendingRequest) (ConsumableState, error) {
	if loop == nil || loop.stopped() {
		return ConsumableState{}, errors.New("game session is not running")
	}
	result := make(chan actionResult, 1)
	action := clientAction{
		Type:           markConsumableQuizPendingAction,
		ConsumableQuiz: request,
		Result:         result,
	}
	select {
	case loop.actions <- action:
	case <-ctx.Done():
		return ConsumableState{}, ctx.Err()
	case <-time.After(2 * time.Second):
		return ConsumableState{}, errors.New("game action queue timed out")
	}

	select {
	case response := <-result:
		return response.Consumables, response.Err
	case <-ctx.Done():
		return ConsumableState{}, ctx.Err()
	case <-time.After(2 * time.Second):
		return ConsumableState{}, errors.New("game action response timed out")
	}
}

func (s *Server) finishConsumableQuizThroughLoop(ctx context.Context, loop *runningGameLoop, request finishConsumableQuizRequest) (ConsumableState, error) {
	if loop == nil || loop.stopped() {
		return ConsumableState{}, errors.New("game session is not running")
	}
	result := make(chan actionResult, 1)
	action := clientAction{
		Type:                 finishConsumableQuizAction,
		FinishConsumableQuiz: request,
		Result:               result,
	}
	select {
	case loop.actions <- action:
	case <-ctx.Done():
		return ConsumableState{}, ctx.Err()
	case <-time.After(2 * time.Second):
		return ConsumableState{}, errors.New("game action queue timed out")
	}

	select {
	case response := <-result:
		return response.Consumables, response.Err
	case <-ctx.Done():
		return ConsumableState{}, ctx.Err()
	case <-time.After(2 * time.Second):
		return ConsumableState{}, errors.New("game action response timed out")
	}
}

func (s *Server) saveQuizMistakeAsync(loop *runningGameLoop, quiz quizcache.CachedQuiz, selectedIndex int) {
	if s.sessions == nil {
		log.Printf("quiz mistake save skipped quiz_id=%s: game session store is not configured", quiz.ID)
		return
	}
	input := quizMistakeInput(loop, quiz, selectedIndex)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.sessions.SaveQuizMistake(ctx, loop.sessionID, loop.userID, input); err != nil {
			log.Printf("quiz mistake save failed quiz_id=%s session_id=%s user_id=%s: %v", input.QuizID, loop.sessionID, loop.userID, err)
		}
	}()
}

func quizMistakeInput(loop *runningGameLoop, quiz quizcache.CachedQuiz, selectedIndex int) gamesession.QuizMistake {
	return gamesession.QuizMistake{
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
	case mergeTowerAction:
		return s.mergeTower(ctx, runtime, action.MergeTower)
	default:
		return errors.New("unsupported action")
	}
}

func resolveConsumableAction(runtime *runtimeSession, request useConsumableRequest) (consumableResolution, error) {
	if runtime == nil {
		return consumableResolution{}, errors.New("game session is not running")
	}
	if !runtime.loopStarted || runtime.session.Health <= 0 || gameWon(*runtime) {
		return consumableResolution{}, errors.New("game session is not running")
	}
	if strings.TrimSpace(request.ItemType) != airstrikeItemType {
		return consumableResolution{}, errors.New("unknown consumable type")
	}
	if err := validateAirstrikeTargets(runtime.levelMap, request.Targets); err != nil {
		return consumableResolution{}, err
	}
	if consumableCooldownRemainingSeconds(runtime.consumableCooldownUntil, time.Now().UTC()) > 0 {
		return consumableResolution{}, errors.New("airstrike is on cooldown")
	}
	if runtime.consumables.Airstrike.Status != consumableStatusReady || runtime.consumables.Airstrike.Charges <= 0 {
		return consumableResolution{}, errors.New("airstrike is not ready")
	}

	runtime.consumables.Airstrike.Charges--
	runtime.consumableCooldownUntil = time.Now().UTC().Add(airstrikeUseCooldown)
	if runtime.consumables.Airstrike.Charges <= 0 {
		runtime.consumables.Airstrike.Charges = 0
		runtime.consumables.Airstrike.Status = consumableStatusEmpty
	}
	deployment := ConsumableDeploymentState{
		DeploymentID: uuid.NewString(),
		ItemType:     airstrikeItemType,
		Targets:      copyPositions(request.Targets),
	}
	events := applyAirstrikeDeployment(runtime, deployment)
	resolution := consumableResolution{
		DeploymentID: deployment.DeploymentID,
		ItemType:     deployment.ItemType,
		Targets:      deployment.Targets,
		Consumables:  runtime.consumables,
		Enemies:      enemyStates(runtime.enemies),
		Events:       events,
	}

	return resolution, nil
}

func applyAirstrikeDeployment(runtime *runtimeSession, deployment ConsumableDeploymentState) []GameEvent {
	if runtime == nil {
		return nil
	}
	events := []GameEvent{{
		Type:         "airstrike.impact",
		DeploymentID: deployment.DeploymentID,
		ItemType:     deployment.ItemType,
		Targets:      deployment.Targets,
	}}
	for i := range runtime.enemies {
		enemy := &runtime.enemies[i]
		if !enemy.IsAlive() {
			continue
		}
		for _, target := range deployment.Targets {
			if !enemy.IsAlive() || enemy.Position.DistanceTo(target) > airstrikeRadius {
				continue
			}
			beforeHealth := enemy.Health
			enemy.TakeDamage(airstrikeDamage)
			damage := float64(beforeHealth - enemy.Health)
			if damage <= 0 {
				continue
			}
			events = append(events, GameEvent{
				Type:         "enemy.damage",
				EnemyID:      enemy.ID,
				DeploymentID: deployment.DeploymentID,
				ItemType:     deployment.ItemType,
				Damage:       damage,
				Health:       enemy.Health,
			})
			if !enemy.IsAlive() {
				awardEssenceForEnemyKill(runtime, enemy.Type)
			}
		}
	}
	runtime.enemies = aliveEnemies(runtime.enemies)
	return events
}

func (s *Server) grantConsumable(ctx context.Context, runtime *runtimeSession, request grantConsumableRequest) error {
	if runtime == nil {
		return errors.New("game session is not running")
	}
	if request.ItemType != airstrikeItemType {
		return errors.New("unknown consumable type")
	}
	charges := request.Charges
	if charges <= 0 {
		charges = 1
	}
	previousConsumables := runtime.consumables
	runtime.consumables.Airstrike.Charges += charges
	runtime.consumables.Airstrike.Status = consumableStatusReady
	if err := s.saveRuntimeState(ctx, *runtime); err != nil {
		runtime.consumables = previousConsumables
		return errors.New("failed to save consumable reward")
	}
	return nil
}

func validateConsumableAcquire(runtime *runtimeSession, itemType string) error {
	if runtime == nil {
		return errors.New("game session is not running")
	}
	if itemType != airstrikeItemType {
		return errors.New("unknown consumable type")
	}
	if runtime.consumables.Airstrike.PendingQuizID != "" || runtime.consumables.Airstrike.Status == consumableStatusQuizPending {
		return errors.New("airstrike quiz is already pending")
	}
	if runtime.consumables.Airstrike.Charges > 0 || runtime.consumables.Airstrike.Status == consumableStatusReady {
		return errors.New("airstrike is already ready")
	}
	if consumableCooldownRemainingSeconds(runtime.consumableCooldownUntil, time.Now().UTC()) > 0 {
		return errors.New("airstrike is on cooldown")
	}
	return nil
}

func (s *Server) markConsumableQuizPending(ctx context.Context, runtime *runtimeSession, request consumableQuizPendingRequest) error {
	if runtime == nil {
		return errors.New("game session is not running")
	}
	if request.ItemType != airstrikeItemType {
		return errors.New("unknown consumable type")
	}
	if strings.TrimSpace(request.QuizID) == "" {
		return errors.New("quiz_id is required")
	}
	previousConsumables := runtime.consumables
	runtime.consumables.Airstrike.Status = consumableStatusQuizPending
	runtime.consumables.Airstrike.PendingQuizID = request.QuizID
	if err := s.saveRuntimeState(ctx, *runtime); err != nil {
		runtime.consumables = previousConsumables
		return errors.New("failed to save consumable quiz")
	}
	return nil
}

func (s *Server) finishConsumableQuiz(ctx context.Context, runtime *runtimeSession, request finishConsumableQuizRequest) (ConsumableState, error) {
	if runtime == nil {
		return ConsumableState{}, errors.New("game session is not running")
	}
	if request.ItemType != airstrikeItemType {
		return ConsumableState{}, errors.New("unknown consumable type")
	}
	if runtime.consumables.Airstrike.PendingQuizID != "" && runtime.consumables.Airstrike.PendingQuizID != request.QuizID {
		return ConsumableState{}, errors.New("quiz_id is not the pending consumable quiz")
	}

	previousConsumables := runtime.consumables
	previousCooldownUntil := runtime.consumableCooldownUntil
	runtime.consumables.Airstrike.PendingQuizID = ""
	if request.Correct {
		charges := request.Charges
		if charges <= 0 {
			charges = 1
		}
		runtime.consumables.Airstrike.Charges += charges
		runtime.consumables.Airstrike.Status = consumableStatusReady
	} else {
		runtime.consumableCooldownUntil = time.Now().UTC().Add(airstrikeWrongAnswerCooldown)
		runtime.consumables.Airstrike.Status = consumableStatusEmpty
		if runtime.consumables.Airstrike.Charges > 0 {
			runtime.consumables.Airstrike.Status = consumableStatusReady
		}
	}
	if err := s.saveRuntimeState(ctx, *runtime); err != nil {
		runtime.consumables = previousConsumables
		runtime.consumableCooldownUntil = previousCooldownUntil
		return previousConsumables, errors.New("failed to save consumable quiz result")
	}
	return runtime.consumables, nil
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
	if gameobject.IsHybridBirdType(birdType) {
		return errors.New("hybrid birds must be created by merging")
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

func (s *Server) mergeTower(ctx context.Context, runtime *runtimeSession, request mergeTowerRequest) error {
	if runtime == nil {
		return errors.New("game session is not running")
	}
	sourceID := strings.TrimSpace(request.SourceBirdID)
	sourceType := strings.TrimSpace(request.SourceBirdType)
	targetID := strings.TrimSpace(request.TargetBirdID)
	if targetID == "" {
		return errors.New("target_bird_id is required")
	}
	if (sourceID == "") == (sourceType == "") {
		return errors.New("merge must include exactly one of source_bird_id or source_bird_type")
	}
	if sourceType != "" {
		return s.mergeBoughtBirdWithTower(ctx, runtime, sourceType, targetID)
	}
	if sourceID == targetID {
		return errors.New("source and target birds must be different")
	}
	return s.mergeExistingTowers(ctx, runtime, sourceID, targetID)
}

func (s *Server) mergeExistingTowers(ctx context.Context, runtime *runtimeSession, sourceID string, targetID string) error {
	sourceIndex := findPlacedBirdIndexByID(runtime.birds, sourceID)
	if sourceIndex < 0 {
		return errors.New("source bird not found")
	}
	targetIndex := findPlacedBirdIndexByID(runtime.birds, targetID)
	if targetIndex < 0 {
		return errors.New("target bird not found")
	}

	source := runtime.birds[sourceIndex]
	target := runtime.birds[targetIndex]
	hybridType, ok := hybridBirdTypeForPair(source.birdType, target.birdType)
	if !ok {
		return errors.New("bird types cannot be merged")
	}
	hybridStats, err := gameobject.BirdStatsForType(hybridType)
	if err != nil {
		return errors.New("failed to create hybrid bird")
	}

	nextEconomy := runtime.economy
	if !nextEconomy.Consume(hybridStats.Cost) {
		return errors.New("insufficient essence")
	}

	hybrid, err := gameobject.NewBird(uuid.NewString(), hybridType, target.bird.Position)
	if err != nil {
		return errors.New("failed to create hybrid bird")
	}

	previousEconomy := runtime.economy
	previousBirds := runtime.birds
	nextBirds := make([]placedBird, 0, len(runtime.birds)-1)
	for i, placed := range runtime.birds {
		if i == sourceIndex || i == targetIndex {
			continue
		}
		nextBirds = append(nextBirds, placed)
	}
	nextBirds = append(nextBirds, placedBird{birdType: hybridType, bird: hybrid})
	runtime.economy = nextEconomy
	runtime.session.Essence = nextEconomy.Essence
	runtime.birds = nextBirds

	if err := s.saveRuntimeState(ctx, *runtime); err != nil {
		runtime.economy = previousEconomy
		runtime.session.Essence = previousEconomy.Essence
		runtime.birds = previousBirds
		log.Printf("game session merge save failed session_id=%s: %v", runtime.session.SessionID, err)
		return errors.New("failed to save tower merge")
	}

	return nil
}

func (s *Server) mergeBoughtBirdWithTower(ctx context.Context, runtime *runtimeSession, sourceType string, targetID string) error {
	sourceStats, err := gameobject.BirdStatsForType(sourceType)
	if err != nil {
		return errors.New("unknown bird type")
	}
	if gameobject.IsHybridBirdType(sourceType) {
		return errors.New("hybrid birds must be created by merging")
	}

	targetIndex := findPlacedBirdIndexByID(runtime.birds, targetID)
	if targetIndex < 0 {
		return errors.New("target bird not found")
	}
	target := runtime.birds[targetIndex]
	hybridType, ok := hybridBirdTypeForPair(sourceType, target.birdType)
	if !ok {
		return errors.New("bird types cannot be merged")
	}
	hybridStats, err := gameobject.BirdStatsForType(hybridType)
	if err != nil {
		return errors.New("failed to create hybrid bird")
	}

	nextEconomy := runtime.economy
	if !nextEconomy.Consume(sourceStats.Cost) {
		return errors.New("insufficient essence")
	}
	if !nextEconomy.Consume(hybridStats.Cost) {
		return errors.New("insufficient essence")
	}

	hybrid, err := gameobject.NewBird(uuid.NewString(), hybridType, target.bird.Position)
	if err != nil {
		return errors.New("failed to create hybrid bird")
	}

	previousEconomy := runtime.economy
	previousBirds := runtime.birds
	nextBirds := make([]placedBird, 0, len(runtime.birds))
	for i, placed := range runtime.birds {
		if i == targetIndex {
			continue
		}
		nextBirds = append(nextBirds, placed)
	}
	nextBirds = append(nextBirds, placedBird{birdType: hybridType, bird: hybrid})
	runtime.economy = nextEconomy
	runtime.session.Essence = nextEconomy.Essence
	runtime.birds = nextBirds

	if err := s.saveRuntimeState(ctx, *runtime); err != nil {
		runtime.economy = previousEconomy
		runtime.session.Essence = previousEconomy.Essence
		runtime.birds = previousBirds
		log.Printf("game session bought merge save failed session_id=%s: %v", runtime.session.SessionID, err)
		return errors.New("failed to save tower merge")
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
		Health:                  runtime.session.Health,
		Essence:                 runtime.economy.Essence,
		Wave:                    runtime.session.Wave,
		Tick:                    runtime.session.Tick,
		LoopStarted:             runtime.loopStarted,
		LoopPaused:              false,
		WaveStartedAtTick:       runtime.waveStartedAtTick,
		WaveSpawned:             runtime.waveSpawned,
		NextWaveTick:            runtime.nextWaveTick,
		LastQuizStartedAt:       runtime.lastQuizStartedAt,
		ConsumableCooldownUntil: runtime.consumableCooldownUntil,
		Birds:                   storedBirds(runtime.birds),
		Enemies:                 storedEnemies(runtime.enemies),
		Projectiles:             storedProjectiles(runtime.projectiles),
		Consumables:             consumablesToStored(runtime.consumables),
	})
}

func gameStateFromRuntime(runtime runtimeSession, loop *runningGameLoop, serverTime time.Time, birdTypes []BirdTypeInfo, enemyTypes []EnemyTypeInfo, events []GameEvent) GameState {
	session := runtime.session
	cooldown := 0
	if loop != nil && loop.currentQuizID == "" {
		cooldown = quizCooldownRemainingSeconds(runtime.lastQuizStartedAt, serverTime)
	}
	consumables := runtime.consumables
	if loop == nil || loop.currentConsumableQuizID == "" {
		consumables.Airstrike.CooldownRemainingSeconds = consumableCooldownRemainingSeconds(runtime.consumableCooldownUntil, serverTime)
	}

	return GameState{
		SessionID:                    session.SessionID,
		LevelID:                      session.LevelID,
		Health:                       session.Health,
		Essence:                      runtime.economy.Essence,
		Wave:                         session.Wave,
		Tick:                         session.Tick,
		ServerTime:                   serverTime.UTC(),
		LoopStarted:                  runtime.loopStarted,
		LoopPaused:                   runtime.loopPaused,
		QuizCooldownRemainingSeconds: cooldown,
		BirdTypes:                    birdTypes,
		EnemyTypes:                   enemyTypes,
		Birds:                        placedBirdStates(runtime.birds),
		Enemies:                      enemyStates(runtime.enemies),
		Projectiles:                  projectileStates(runtime.projectiles),
		Consumables:                  consumables,
		Events:                       events,
	}
}

func quizCooldownRemainingSeconds(lastQuizStartedAt time.Time, now time.Time) int {
	return quizflow.CooldownRemainingSeconds(quizRequestCooldown, lastQuizStartedAt, now)
}

func consumableCooldownRemainingSeconds(cooldownUntil time.Time, now time.Time) int {
	if cooldownUntil.IsZero() || !now.Before(cooldownUntil) {
		return 0
	}
	return int(math.Ceil(cooldownUntil.Sub(now).Seconds()))
}

type spawnGroup struct {
	Type   string
	Count  int
	Health int
	Speed  float64
}

type waveDefinition struct {
	Wave   int
	Groups []spawnGroup
}

// count how many groups (all waves)
func (w waveDefinition) Count() int {
	total := 0
	for _, g := range w.Groups {
		total += g.Count
	}
	return total
}

// return the particular enemy at what group
func (w waveDefinition) enemyAt(index int) (spawnGroup, bool) {
	for _, g := range w.Groups {
		if index < g.Count {
			return g, true
		}
		index -= g.Count
	}
	return spawnGroup{}, false
}

func waveDefinitions() []waveDefinition {
	return []waveDefinition{
		{Wave: 1, Groups: []spawnGroup{
			scaledGroup(1, gameobject.EnemyTypeSmog, 20),
		}},
		{Wave: 2, Groups: []spawnGroup{
			scaledGroup(2, gameobject.EnemyTypeSmog, 10),
			scaledGroup(2, gameobject.EnemyTypeNoise, 10),
			scaledGroup(2, gameobject.EnemyTypeSmog, 10),
			scaledGroup(2, gameobject.EnemyTypeNoise, 10),
		}},
		{Wave: 3, Groups: []spawnGroup{
			scaledGroup(3, gameobject.EnemyTypeJunk, 1),
			scaledGroup(3, gameobject.EnemyTypeSmog, 10),
			scaledGroup(3, gameobject.EnemyTypeNoise, 10),
			scaledGroup(3, gameobject.EnemyTypeJunk, 1),
			scaledGroup(3, gameobject.EnemyTypeSmog, 10),
			scaledGroup(3, gameobject.EnemyTypeNoise, 10),
			scaledGroup(3, gameobject.EnemyTypeJunk, 1),
			scaledGroup(3, gameobject.EnemyTypeSmog, 10),
			scaledGroup(3, gameobject.EnemyTypeNoise, 10),
		}},
		{Wave: 4, Groups: []spawnGroup{
			scaledGroup(4, gameobject.EnemyTypeJunk, 1),
			scaledGroup(4, gameobject.EnemyTypeSmog, 15),
			scaledGroup(4, gameobject.EnemyTypeNoise, 20),
			scaledGroup(4, gameobject.EnemyTypeJunk, 2),
			scaledGroup(4, gameobject.EnemyTypeSmog, 15),
			scaledGroup(4, gameobject.EnemyTypeNoise, 20),
			scaledGroup(4, gameobject.EnemyTypeJunk, 2),
			scaledGroup(4, gameobject.EnemyTypeSmog, 15),
			scaledGroup(4, gameobject.EnemyTypeNoise, 20),
		}},
		{Wave: 5, Groups: []spawnGroup{
			scaledGroup(5, gameobject.EnemyTypeJunk, 3),
			scaledGroup(5, gameobject.EnemyTypeSmog, 20),
			scaledGroup(5, gameobject.EnemyTypeNoise, 20),
			scaledGroup(5, gameobject.EnemyTypeJunk, 3),
			scaledGroup(5, gameobject.EnemyTypeSmog, 20),
			scaledGroup(5, gameobject.EnemyTypeNoise, 20),
			scaledGroup(5, gameobject.EnemyTypeJunk, 3),
			scaledGroup(5, gameobject.EnemyTypeSmog, 20),
			scaledGroup(5, gameobject.EnemyTypeNoise, 20),
		}},
	}
}

func scaledGroup(wave int, enemyType string, count int) spawnGroup {
	return spawnGroup{
		Type:   enemyType,
		Count:  count,
		Health: scaledEnemyHealth(wave, enemyType),
		Speed:  scaledEnemySpeed(wave, enemyType),
	}
}

func scaledEnemyHealth(wave int, enemyType string) int {
	waveOffset := wave - 1
	waveOffset = max(0, waveOffset)
	stats, err := gameobject.EnemyStatsForType(enemyType)
	if err != nil {
		return 0
	}
	return stats.Health + (waveOffset * 15) + (waveOffset * waveOffset * 2)
}

func scaledEnemySpeed(wave int, enemyType string) float64 {
	waveOffset := float64(wave - 1)
	waveOffset = max(0, waveOffset)
	stats, err := gameobject.EnemyStatsForType(enemyType)
	if err != nil {
		return 0
	}

	return stats.Speed + (waveOffset * 0.1) + (waveOffset * waveOffset * 0.01)
}

func advanceRuntimeTick(runtime *runtimeSession, now time.Time) []GameEvent {
	if runtime == nil {
		return nil
	}
	if !runtime.loopStarted {
		return nil
	}
	runtime.projectiles = nil
	runtime.session.Tick++
	runtime.session.Essence = runtime.economy.Essence
	runtime.session.UpdatedAt = now.UTC()

	events := make([]GameEvent, 0)
	events = append(events, moveEnemies(runtime, gameTickInterval.Seconds())...)
	if runtime.session.Health <= 0 {
		return events
	}
	events = append(events, spawnEnemies(runtime)...)
	events = append(events, fireBirds(runtime)...)
	runtime.enemies = aliveEnemies(runtime.enemies)
	events = append(events, scheduleNextWaveIfCleared(runtime)...)
	return events
}

func spawnEnemies(runtime *runtimeSession) []GameEvent {
	if len(runtime.path) == 0 || runtime.nextWaveTick <= 0 || runtime.session.Tick < runtime.nextWaveTick {
		return nil
	}
	wave, ok := activeWaveDefinition(runtime)
	if !ok || runtime.waveSpawned >= wave.Count() {
		return nil
	}

	events := make([]GameEvent, 0)
	if runtime.waveSpawned == 0 {
		runtime.session.Wave = wave.Wave
		runtime.waveStartedAtTick = runtime.session.Tick
		events = append(events, GameEvent{Type: "wave.started", Wave: wave.Wave})
	}

	// Calculate subwave/group spawn ticks dynamically
	groupSize := wave.Count()

	// first wave easy
	if wave.Wave == 1 {
		groupSize = wave.Count() / 2
	}
	if groupSize < 1 {
		groupSize = 1
	}

	groupIndex := int64(runtime.waveSpawned / groupSize)
	indexInGroup := int64(runtime.waveSpawned % groupSize)

	spawnIntervalTicks := enemySpawnIntervalTicksForWave(wave.Wave)
	groupDurationTicks := int64(groupSize) * spawnIntervalTicks
	groupStartIntervalTicks := groupDurationTicks + groupGapTicks

	nextSpawnTick := runtime.waveStartedAtTick + (groupIndex * groupStartIntervalTicks) + (indexInGroup * spawnIntervalTicks)

	if runtime.session.Tick < nextSpawnTick {
		return events
	}

	group, found := wave.enemyAt(runtime.waveSpawned)
	if !found {
		return events
	}
	enemy := gameobject.Enemy{
		ID:        uuid.NewString(),
		Type:      group.Type,
		Health:    group.Health,
		Position:  runtime.path[0],
		Speed:     group.Speed,
		PathIndex: 0,
	}
	runtime.waveSpawned++
	runtime.enemies = append(runtime.enemies, enemy)
	events = append(events, GameEvent{Type: "enemy.spawned", EnemyID: enemy.ID, Wave: wave.Wave, Health: enemy.Health})
	return events
}

func enemySpawnIntervalTicksForWave(wave int) int64 {
	waveOffset := int64(wave - 1)
	if waveOffset < 0 {
		waveOffset = 0
	}
	interval := enemySpawnIntervalTicks - (waveOffset * 3)
	if interval < minEnemySpawnIntervalTicks {
		return minEnemySpawnIntervalTicks
	}
	return interval
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
	if runtime.session.Wave <= 0 || len(runtime.enemies) > 0 {
		return nil
	}
	currentWave, ok := currentWaveDefinition(runtime.session.Wave)
	if !ok || runtime.waveSpawned < currentWave.Count() {
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
	if runtime.session.Health <= 0 || runtime.session.Wave < len(waveDefinitions()) || len(runtime.enemies) > 0 {
		return false
	}
	finalWave, ok := currentWaveDefinition(runtime.session.Wave)
	return ok && runtime.waveSpawned >= finalWave.Count() && runtime.nextWaveTick == 0
}

func moveEnemies(runtime *runtimeSession, deltaSeconds float64) []GameEvent {
	if runtime.session.Health <= 0 {
		return nil
	}
	events := make([]GameEvent, 0)
	nextEnemies := make([]gameobject.Enemy, 0, len(runtime.enemies))
	for i := range runtime.enemies {
		enemy := runtime.enemies[i]
		enemy.Move(deltaSeconds, runtime.path)
		if enemyReachedEnd(enemy, runtime.path) {
			runtime.session.Health -= baseHealthDamage
			if runtime.session.Health < 0 {
				runtime.session.Health = 0
			}
			events = append(events, GameEvent{
				Type:    "enemy.escaped",
				EnemyID: enemy.ID,
				Damage:  float64(baseHealthDamage),
				Health:  runtime.session.Health,
			})
			continue
		}
		nextEnemies = append(nextEnemies, enemy)
	}
	runtime.enemies = nextEnemies
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
		targetIndex := targetEnemyIndex(*bird, runtime.enemies)
		if targetIndex < 0 {
			continue
		}
		target := runtime.enemies[targetIndex]
		hits := bird.Attack(target, runtime.enemies, runtime.session.Tick)
		if len(hits) == 0 {
			continue
		}
		events = append(events, GameEvent{
			Type:    "bird.attack",
			BirdID:  bird.ID,
			EnemyID: target.ID,
		})
		events = append(events, applyAttackHits(runtime, hits)...)
	}
	return events
}

func applyAttackHits(runtime *runtimeSession, hits []gameobject.AttackHit) []GameEvent {
	events := make([]GameEvent, 0, len(hits))
	for _, hit := range hits {
		target := findEnemyByID(runtime.enemies, hit.EnemyID)
		if target == nil || !target.IsAlive() {
			continue
		}
		beforeHealth := target.Health
		target.TakeDamage(hit.Damage)
		damage := float64(beforeHealth - target.Health)
		if damage <= 0 {
			continue
		}
		events = append(events, GameEvent{
			Type:    "enemy.damage",
			EnemyID: target.ID,
			Damage:  damage,
			Health:  target.Health,
		})
		if !target.IsAlive() {
			awardEssenceForEnemyKill(runtime, target.Type)
		}
	}
	return events
}

func awardEssenceForEnemyKill(runtime *runtimeSession, enemyType string) {
	if runtime == nil {
		return
	}
	reward, err := gameobject.EnemyEssenceRewardForType(enemyType)
	if err != nil || reward <= 0 {
		return
	}
	runtime.economy.Add(reward)
	runtime.session.Essence = runtime.economy.Essence
}

func targetEnemyIndex(bird gameobject.Bird, enemies []gameobject.Enemy) int {
	bestIndex := -1
	for i := range enemies {
		if !bird.TargetInRange(enemies[i]) {
			continue
		}
		if bestIndex < 0 || enemies[i].PathIndex > enemies[bestIndex].PathIndex {
			bestIndex = i
		}
	}
	return bestIndex
}

func aliveEnemies(enemies []gameobject.Enemy) []gameobject.Enemy {
	active := make([]gameobject.Enemy, 0, len(enemies))
	for _, enemy := range enemies {
		if enemy.IsAlive() {
			active = append(active, enemy)
		}
	}
	return active
}

func findEnemyByID(enemies []gameobject.Enemy, id string) *gameobject.Enemy {
	for i := range enemies {
		if enemies[i].ID == id {
			return &enemies[i]
		}
	}
	return nil
}

func enemyReachedEnd(enemy gameobject.Enemy, path []gameobject.Position) bool {
	if len(path) == 0 {
		return false
	}
	return enemy.PathIndex >= len(path)-1 && enemy.Position.DistanceTo(path[len(path)-1]) < 0.000001
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
			runtime.WaveSpawned = wave.Count()
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

func decodeMergeTowerAction(data any) (mergeTowerRequest, error) {
	var request mergeTowerRequest
	if err := decodeMessageData(data, &request); err != nil {
		return mergeTowerRequest{}, errors.New("merge tower action data must include target_bird_id and one source field")
	}
	request.SourceBirdID = strings.TrimSpace(request.SourceBirdID)
	request.SourceBirdType = strings.TrimSpace(request.SourceBirdType)
	request.TargetBirdID = strings.TrimSpace(request.TargetBirdID)
	if request.TargetBirdID == "" {
		return mergeTowerRequest{}, errors.New("target_bird_id is required")
	}
	if (request.SourceBirdID == "") == (request.SourceBirdType == "") {
		return mergeTowerRequest{}, errors.New("merge must include exactly one of source_bird_id or source_bird_type")
	}
	return request, nil
}

func decodeUseConsumableAction(data any) (useConsumableRequest, error) {
	var request useConsumableRequest
	if err := decodeMessageData(data, &request); err != nil {
		return useConsumableRequest{}, errors.New("use consumable action data must include item_type and targets")
	}
	request.ItemType = strings.TrimSpace(request.ItemType)
	if request.ItemType == "" {
		return useConsumableRequest{}, errors.New("item_type is required")
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

func decodeConsumableAcquire(data any) (consumableAcquireRequest, error) {
	var request consumableAcquireRequest
	if err := decodeMessageData(data, &request); err != nil {
		return consumableAcquireRequest{}, errors.New("game.consumable.acquire data must include item_type")
	}
	request.ItemType = strings.TrimSpace(request.ItemType)
	if request.ItemType == "" {
		return consumableAcquireRequest{}, errors.New("item_type is required")
	}
	return request, nil
}

func decodeConsumableQuizAnswer(data any) (consumableQuizAnswerRequest, error) {
	var request consumableQuizAnswerRequest
	if err := decodeMessageData(data, &request); err != nil {
		return consumableQuizAnswerRequest{}, errors.New("game.consumable.quiz.answer data must include item_type, quiz_id, and selected_index")
	}
	request.ItemType = strings.TrimSpace(request.ItemType)
	request.QuizID = strings.TrimSpace(request.QuizID)
	if request.ItemType == "" {
		return consumableQuizAnswerRequest{}, errors.New("item_type is required")
	}
	if request.QuizID == "" {
		return consumableQuizAnswerRequest{}, errors.New("quiz_id is required")
	}
	if !uuidPattern.MatchString(request.QuizID) {
		return consumableQuizAnswerRequest{}, errors.New("quiz_id must be a valid UUID")
	}
	if request.SelectedIndex < 0 {
		return consumableQuizAnswerRequest{}, errors.New("selected_index is out of range")
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

func enemyTypeCatalog() []EnemyTypeInfo {
	enemyTypes := gameobject.EnemyTypes()
	catalog := make([]EnemyTypeInfo, 0, len(enemyTypes))
	for _, enemyType := range enemyTypes {
		stats, err := gameobject.EnemyStatsForType(enemyType)
		if err != nil {
			continue
		}
		catalog = append(catalog, EnemyTypeInfo{
			Type:  enemyType,
			Stats: stats,
		})
	}
	return catalog
}

func isInsideMap(levelMap mapgen.GeneratedMap, x int, y int) bool {
	return x >= 0 && y >= 0 && x < levelMap.Width && y < levelMap.Height
}

func isPositionInsideMap(levelMap mapgen.GeneratedMap, position gameobject.Position) bool {
	return position.X >= 0 && position.Y >= 0 && position.X < float64(levelMap.Width) && position.Y < float64(levelMap.Height)
}

func validateAirstrikeTargets(levelMap mapgen.GeneratedMap, targets []gameobject.Position) error {
	if len(targets) != airstrikeTargetCount {
		return errors.New("airstrike requires exactly 3 targets")
	}
	for i, target := range targets {
		if math.IsNaN(target.X) || math.IsNaN(target.Y) || math.IsInf(target.X, 0) || math.IsInf(target.Y, 0) {
			return errors.New("airstrike target must be finite")
		}
		if !isPositionInsideMap(levelMap, target) {
			return errors.New("airstrike target is outside the map")
		}
		for j := 0; j < i; j++ {
			if target.DistanceTo(targets[j]) < 0.001 {
				return errors.New("airstrike targets must be distinct")
			}
		}
	}
	return nil
}

func isEnemyPath(levelMap mapgen.GeneratedMap, x int, y int) bool {
	for _, tile := range levelMap.EnemyPath {
		if tile.X == x && tile.Y == y {
			return true
		}
	}
	return false
}

func copyPositions(positions []gameobject.Position) []gameobject.Position {
	copied := make([]gameobject.Position, len(positions))
	copy(copied, positions)
	return copied
}

func isOccupied(birds []placedBird, x int, y int) bool {
	for _, placed := range birds {
		if int(placed.bird.Position.X) == x && int(placed.bird.Position.Y) == y {
			return true
		}
	}
	return false
}

func findPlacedBirdIndexByID(birds []placedBird, id string) int {
	for i := range birds {
		if birds[i].bird.ID == id {
			return i
		}
	}
	return -1
}

func hybridBirdTypeForPair(first string, second string) (string, bool) {
	if first > second {
		first, second = second, first
	}
	switch first + "+" + second {
	case gameobject.BirdTypeEagle + "+" + gameobject.BirdTypeSparrow:
		return gameobject.BirdTypeFalcon, true
	case gameobject.BirdTypePeacock + "+" + gameobject.BirdTypeWoodpecker:
		return gameobject.BirdTypeKingfisher, true
	case gameobject.BirdTypeEagle + "+" + gameobject.BirdTypePeacock:
		return gameobject.BirdTypePhoenix, true
	case gameobject.BirdTypeEagle + "+" + gameobject.BirdTypeKingfisher:
		return gameobject.BirdTypeSunGod, true
	default:
		return "", false
	}
}

func removedBirdIDsForMerge(request mergeTowerRequest) []string {
	if request.SourceBirdID != "" {
		return []string{request.SourceBirdID, request.TargetBirdID}
	}
	return []string{request.TargetBirdID}
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

func enemyStates(enemies []gameobject.Enemy) []EnemyState {
	states := make([]EnemyState, 0, len(enemies))
	for _, enemy := range enemies {
		states = append(states, EnemyState{
			ID:        enemy.ID,
			Type:      enemy.Type,
			Health:    enemy.Health,
			Position:  enemy.Position,
			Speed:     enemy.Speed,
			PathIndex: enemy.PathIndex,
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
	return quizflow.PromptState(quiz, remaining)
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

func storedEnemies(enemies []gameobject.Enemy) []gamesession.StoredEnemy {
	stored := make([]gamesession.StoredEnemy, 0, len(enemies))
	for _, enemy := range enemies {
		stored = append(stored, gamesession.StoredEnemy{
			ID:        enemy.ID,
			Type:      enemy.Type,
			Health:    enemy.Health,
			Position:  enemy.Position,
			Speed:     enemy.Speed,
			PathIndex: enemy.PathIndex,
		})
	}
	return stored
}

func enemiesFromStored(stored []gamesession.StoredEnemy) []gameobject.Enemy {
	enemies := make([]gameobject.Enemy, 0, len(stored))
	for _, item := range stored {
		enemies = append(enemies, gameobject.Enemy{
			ID:        item.ID,
			Type:      item.Type,
			Health:    item.Health,
			Position:  item.Position,
			Speed:     item.Speed,
			PathIndex: item.PathIndex,
		})
	}
	return enemies
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

func consumablesFromStored(stored gamesession.ConsumableInventory) ConsumableState {
	return ConsumableState{
		Airstrike: ConsumableItemState{
			Status:        normalizeConsumableStatus(stored.Airstrike.Status, stored.Airstrike.Charges, stored.Airstrike.PendingQuizID),
			Charges:       max(0, stored.Airstrike.Charges),
			PendingQuizID: strings.TrimSpace(stored.Airstrike.PendingQuizID),
		},
	}
}

func consumablesToStored(state ConsumableState) gamesession.ConsumableInventory {
	return gamesession.ConsumableInventory{
		Airstrike: gamesession.ConsumableItemState{
			Status:        normalizeConsumableStatus(state.Airstrike.Status, state.Airstrike.Charges, state.Airstrike.PendingQuizID),
			Charges:       max(0, state.Airstrike.Charges),
			PendingQuizID: strings.TrimSpace(state.Airstrike.PendingQuizID),
		},
	}
}

func normalizeConsumableStatus(status string, charges int, pendingQuizID string) string {
	status = strings.TrimSpace(status)
	switch status {
	case consumableStatusReady, consumableStatusQuizPending:
		return status
	default:
		if strings.TrimSpace(pendingQuizID) != "" {
			return consumableStatusQuizPending
		}
		if charges > 0 {
			return consumableStatusReady
		}
		return consumableStatusEmpty
	}
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

func writeActionAccepted(conn *websocket.Conn, writeMu *sync.Mutex, action string, bird placedBird, removedBirdIDs []string) error {
	data := map[string]any{
		"action":  action,
		"bird_id": bird.bird.ID,
		"bird": PlacedBirdState{
			ID:              bird.bird.ID,
			Type:            bird.birdType,
			Position:        bird.bird.Position,
			Stats:           bird.bird.Stats,
			LastFiredAtTick: bird.bird.LastFiredAtTick,
		},
	}
	if len(removedBirdIDs) > 0 {
		data["removed_bird_ids"] = removedBirdIDs
	}
	return writeWebsocketJSON(conn, writeMu, Message{
		Type: "game.action.accepted",
		Data: data,
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
	SessionID string                   `json:"session_id"`
	LevelID   string                   `json:"level_id"`
	Count     int                      `json:"count"`
	Mistakes  []QuizMistakeSummaryItem `json:"mistakes"`
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
	if s.sessions == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "quiz mistake summary is not configured"})
		return
	}

	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
	if sessionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session_id is required"})
		return
	}
	if !uuidPattern.MatchString(sessionID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session_id must be a valid UUID"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	mistakes, err := s.sessions.ListQuizMistakes(ctx, sessionID, userID)
	if errors.Is(err, gamesession.ErrSessionNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "game session not found"})
		return
	}
	if err != nil {
		log.Printf("quiz mistake summary lookup failed session_id=%s user_id=%s: %v", sessionID, userID, err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "failed to read quiz mistake summary"})
		return
	}

	writeJSON(w, http.StatusOK, quizMistakeSummaryState(sessionID, mistakes))
}

func quizMistakeSummaryState(sessionID string, mistakes []gamesession.QuizMistake) QuizMistakeSummaryState {
	items := make([]QuizMistakeSummaryItem, 0, len(mistakes))
	levelID := ""
	for _, mistake := range mistakes {
		if levelID == "" {
			levelID = mistake.LevelID
		}
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
		SessionID: sessionID,
		LevelID:   levelID,
		Count:     len(items),
		Mistakes:  items,
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
