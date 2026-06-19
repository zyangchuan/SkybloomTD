package server

import (
	"context"
	"time"

	"skybloom/game-service/internal/gameobject"
	"skybloom/game-service/internal/gamesession"
	"skybloom/game-service/internal/mapgen"
)

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

type GameEndState struct {
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

type evolveTowerRequest struct {
	TowerID  string `json:"tower_id"`
	BirdType string `json:"bird_type"`
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
	EvolveTower   evolveTowerRequest
	EssenceReward int
	Result        chan actionResult

	// add multiplier to speed the pace of game
	SpeedMultiplier int
}

type actionResult struct {
	Essence int
	Err     error
}

type runningGameLoop struct {
	sessionID     string
	levelID       string
	generationID  string
	userID        string
	currentQuizID string
	stop          context.CancelFunc
	actions       chan clientAction
	done          chan struct{}
}

func (l *runningGameLoop) stopped() bool {
	if l == nil {
		return true
	}
	
	// we check the done channel in a non-blocking way to determine if the loop has stopped
	select {
	case <-l.done:
		return true
	default:
		return false
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
	//addd multiplier to speed up the pace of game
	speedMultiplier int
}

type placedBird struct {
	birdType string
	bird     gameobject.Bird
}

type waveDefinition struct {
	Wave   int
	Count  int
	Health int
	Speed  float64
}
