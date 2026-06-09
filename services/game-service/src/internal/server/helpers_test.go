package server

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
	"github.com/gorilla/websocket"
	"skybloom/game-service/internal/gamesession"
	"skybloom/game-service/internal/generation"
	"skybloom/game-service/internal/mapcache"
	"skybloom/game-service/internal/mapgen"
	"skybloom/game-service/internal/models"
	"skybloom/game-service/internal/quizcache"
	"skybloom/game-service/internal/repository"
)

// ---------------------------------------------------------------------------
// Fake repositories / caches / stores
// ---------------------------------------------------------------------------

type fakeLevelRepository struct {
	bootstrap           repository.LevelBootstrap
	mistakeMu           sync.Mutex
	mistakeStartedOnce  sync.Once
	saveMistakeStarted  chan struct{}
	saveMistakeContinue chan struct{}
	mistakes            []repository.QuizMistakeInput
}

func (r *fakeLevelRepository) GetBootstrap(_ context.Context, levelID string, userID string) (repository.LevelBootstrap, error) {
	if r.bootstrap.LevelID != levelID || r.bootstrap.UserID != userID {
		return repository.LevelBootstrap{}, models.ErrLevelNotFound
	}
	return r.bootstrap, nil
}

func (r *fakeLevelRepository) Ping(context.Context) error {
	return nil
}

func (r *fakeLevelRepository) SaveQuizMistake(ctx context.Context, input repository.QuizMistakeInput) error {
	if r.saveMistakeStarted != nil {
		r.mistakeStartedOnce.Do(func() {
			close(r.saveMistakeStarted)
		})
	}
	if r.saveMistakeContinue != nil {
		select {
		case <-r.saveMistakeContinue:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	r.mistakeMu.Lock()
	defer r.mistakeMu.Unlock()
	r.mistakes = append(r.mistakes, input)
	return nil
}

func (r *fakeLevelRepository) ListQuizMistakes(_ context.Context, userID string, levelID string) ([]repository.QuizMistakeSummaryItem, error) {
	r.mistakeMu.Lock()
	defer r.mistakeMu.Unlock()

	items := make([]repository.QuizMistakeSummaryItem, 0, len(r.mistakes))
	for _, mistake := range r.mistakes {
		if mistake.UserID != userID || mistake.LevelID != levelID {
			continue
		}
		items = append(items, repository.QuizMistakeSummaryItem{
			ID:               mistake.QuizID,
			UserID:           mistake.UserID,
			LevelID:          mistake.LevelID,
			GenerationID:     mistake.GenerationID,
			QuizID:           mistake.QuizID,
			QuizIndex:        mistake.QuizIndex,
			QuizType:         mistake.QuizType,
			QuestionMarkdown: mistake.QuestionMarkdown,
			OptionsMarkdown:  append([]string(nil), mistake.OptionsMarkdown...),
			AnswerIndex:      mistake.AnswerIndex,
			SelectedIndex:    mistake.SelectedIndex,
		})
	}
	return items, nil
}

func (r *fakeLevelRepository) ClearQuizMistakes(_ context.Context, userID string, levelID string) error {
	r.mistakeMu.Lock()
	defer r.mistakeMu.Unlock()

	filtered := make([]repository.QuizMistakeInput, 0, len(r.mistakes))
	for _, mistake := range r.mistakes {
		if mistake.UserID == userID && mistake.LevelID == levelID {
			continue
		}
		filtered = append(filtered, mistake)
	}
	r.mistakes = filtered
	return nil
}

func (r *fakeLevelRepository) mistakesSnapshot() []repository.QuizMistakeInput {
	r.mistakeMu.Lock()
	defer r.mistakeMu.Unlock()
	return append([]repository.QuizMistakeInput(nil), r.mistakes...)
}

type fakeMapCache struct {
	cached mapgen.GeneratedMap
}

func (c *fakeMapCache) Get(context.Context, int, string) (mapgen.GeneratedMap, error) {
	if c.cached.Seed == 0 {
		return mapgen.GeneratedMap{}, mapcache.ErrMapNotFound
	}
	return c.cached, nil
}

func (c *fakeMapCache) Set(_ context.Context, _ string, levelMap mapgen.GeneratedMap) error {
	c.cached = levelMap
	return nil
}

type fakeQuizCache struct {
	quizzes             quizcache.LevelQuizzes
	deletedGenerationID string
	peekRandomIndex     int
}

func (c *fakeQuizCache) Get(_ context.Context, generationID string) (quizcache.LevelQuizzes, error) {
	if c.quizzes.GenerationID != generationID {
		return quizcache.LevelQuizzes{}, quizcache.ErrQuizzesNotFound
	}
	return c.quizzes, nil
}

func (c *fakeQuizCache) Set(_ context.Context, generationID string, quizzes quizcache.LevelQuizzes) error {
	quizzes.GenerationID = generationID
	c.quizzes = quizzes
	return nil
}

func (c *fakeQuizCache) PeekNext(_ context.Context, generationID string) (quizcache.CachedQuiz, int, error) {
	if c.quizzes.GenerationID != generationID || len(c.quizzes.Quizzes) == 0 {
		return quizcache.CachedQuiz{}, 0, quizcache.ErrQuizzesNotFound
	}
	return c.quizzes.Quizzes[0], len(c.quizzes.Quizzes), nil
}

func (c *fakeQuizCache) PeekRandom(_ context.Context, generationID string) (quizcache.CachedQuiz, int, error) {
	if c.quizzes.GenerationID != generationID || len(c.quizzes.Quizzes) == 0 {
		return quizcache.CachedQuiz{}, 0, quizcache.ErrQuizzesNotFound
	}
	for _, quiz := range c.quizzes.Quizzes {
		if quiz.ID == c.quizzes.CurrentQuizID {
			return quiz, len(c.quizzes.Quizzes), nil
		}
	}
	index := c.peekRandomIndex
	if index < 0 || index >= len(c.quizzes.Quizzes) {
		index = 0
	}
	quiz := c.quizzes.Quizzes[index]
	c.quizzes.CurrentQuizID = quiz.ID
	return quiz, len(c.quizzes.Quizzes), nil
}

func (c *fakeQuizCache) Take(_ context.Context, generationID string, quizID string) (quizcache.CachedQuiz, int, error) {
	if c.quizzes.GenerationID != generationID {
		return quizcache.CachedQuiz{}, 0, quizcache.ErrQuizzesNotFound
	}
	for index, quiz := range c.quizzes.Quizzes {
		if quiz.ID != quizID {
			continue
		}
		c.quizzes.Quizzes = append(append([]quizcache.CachedQuiz{}, c.quizzes.Quizzes[:index]...), c.quizzes.Quizzes[index+1:]...)
		if c.quizzes.CurrentQuizID == quizID {
			c.quizzes.CurrentQuizID = ""
		}
		return quiz, len(c.quizzes.Quizzes), nil
	}
	return quizcache.CachedQuiz{}, len(c.quizzes.Quizzes), quizcache.ErrQuizNotFound
}

func (c *fakeQuizCache) Delete(_ context.Context, generationID string) error {
	c.deletedGenerationID = generationID
	if c.quizzes.GenerationID == generationID {
		c.quizzes = quizcache.LevelQuizzes{}
	}
	return nil
}

type fakeStarter struct {
	result       generation.StartResult
	userID       string
	subChapterID string
}

func (s *fakeStarter) Start(_ context.Context, userID string, subChapterID string) (generation.StartResult, error) {
	s.userID = userID
	s.subChapterID = subChapterID
	return s.result, nil
}

type fakeGameSessionStore struct {
	options           gamesession.StartOptions
	state             gamesession.State
	economy           gamesession.Economy
	birds             []gamesession.StoredBird
	smogs             []gamesession.StoredSmog
	projectiles       []gamesession.StoredProjectile
	waveStartedAtTick int64
	waveSpawned       int
	nextWaveTick      int64
	deletedSessionID  string
}

func (s *fakeGameSessionStore) Start(_ context.Context, options gamesession.StartOptions) (gamesession.State, error) {
	s.options = options
	now := time.Now().UTC()
	if s.state.SessionID != "" {
		return s.state, nil
	}
	s.state = gamesession.State{
		SessionID:    "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		UserID:       options.UserID,
		LevelID:      options.LevelID,
		GenerationID: options.GenerationID,
		SubChapterID: options.SubChapterID,
		Health:       gamesession.InitialHealth,
		Essence:      gamesession.InitialEssence,
		Wave:         gamesession.InitialWave,
		Tick:         0,
		StartedAt:    now,
		UpdatedAt:    now,
	}
	s.nextWaveTick = 1
	return s.state, nil
}

func (s *fakeGameSessionStore) LoadRuntimeState(_ context.Context, _ string) (gamesession.RuntimeState, error) {
	return gamesession.RuntimeState{
		GenerationID:      s.state.GenerationID,
		Health:            s.state.Health,
		Essence:           s.state.Essence,
		Wave:              s.state.Wave,
		Tick:              s.state.Tick,
		LoopStarted:       true,
		WaveStartedAtTick: s.waveStartedAtTick,
		WaveSpawned:       s.waveSpawned,
		NextWaveTick:      s.nextWaveTick,
		Birds:             append([]gamesession.StoredBird{}, s.birds...),
		Smogs:             append([]gamesession.StoredSmog{}, s.smogs...),
		Projectiles:       append([]gamesession.StoredProjectile{}, s.projectiles...),
	}, nil
}

func (s *fakeGameSessionStore) SaveRuntimeState(_ context.Context, _ string, runtime gamesession.RuntimeState) error {
	s.economy = gamesession.NewEconomy(runtime.Essence)
	s.birds = append([]gamesession.StoredBird{}, runtime.Birds...)
	s.smogs = append([]gamesession.StoredSmog{}, runtime.Smogs...)
	s.projectiles = append([]gamesession.StoredProjectile{}, runtime.Projectiles...)
	s.state.Health = runtime.Health
	s.state.Essence = runtime.Essence
	s.state.Wave = runtime.Wave
	s.state.Tick = runtime.Tick
	s.waveStartedAtTick = runtime.WaveStartedAtTick
	s.waveSpawned = runtime.WaveSpawned
	s.nextWaveTick = runtime.NextWaveTick
	return nil
}

func (s *fakeGameSessionStore) Delete(_ context.Context, sessionID string) error {
	s.deletedSessionID = sessionID
	s.state = gamesession.State{}
	s.economy = gamesession.Economy{}
	s.birds = nil
	s.smogs = nil
	s.projectiles = nil
	s.waveStartedAtTick = 0
	s.waveSpawned = 0
	s.nextWaveTick = 0
	return nil
}

// ---------------------------------------------------------------------------
// HTTP / WebSocket test utilities
// ---------------------------------------------------------------------------

func startHTTPServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Skipf("local test listener unavailable: %v", err)
	}
	server := httptest.NewUnstartedServer(handler)
	server.Listener = listener
	server.Start()
	return server
}

func dialGameWebsocket(t *testing.T, serverURL string) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(serverURL, "http") + "/ws"
	header := http.Header{"X-Authenticated-User-Id": []string{"22222222-2222-2222-2222-222222222222"}}
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline failed: %v", err)
	}
	return conn
}

func readMessageOfType(t *testing.T, conn *websocket.Conn, messageType string) Message {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	seen := make([]string, 0, 10)
	for time.Now().Before(deadline) {
		if err := conn.SetReadDeadline(deadline); err != nil {
			t.Fatalf("SetReadDeadline failed: %v", err)
		}
		var message Message
		if err := conn.ReadJSON(&message); err != nil {
			t.Fatalf("ReadJSON failed looking for %s after seeing %v: %v", messageType, seen, err)
		}
		if message.Type == messageType {
			return message
		}
		if len(seen) < cap(seen) {
			seen = append(seen, message.Type)
		}
	}
	t.Fatalf("did not receive message type %s after seeing %v", messageType, seen)
	return Message{}
}

func waitForMistakes(t *testing.T, levels *fakeLevelRepository, want int) []repository.QuizMistakeInput {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		mistakes := levels.mistakesSnapshot()
		if len(mistakes) == want {
			return mistakes
		}
		time.Sleep(10 * time.Millisecond)
	}
	mistakes := levels.mistakesSnapshot()
	t.Fatalf("expected %d recorded mistakes, got %d", want, len(mistakes))
	return nil
}

func decodeGameState(t *testing.T, data any) GameState {
	t.Helper()
	body, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("Marshal game state failed: %v", err)
	}
	var state GameState
	if err := json.Unmarshal(body, &state); err != nil {
		t.Fatalf("Unmarshal game state failed: %v", err)
	}
	return state
}

func decodeQuizPrompt(t *testing.T, data any) QuizPromptState {
	t.Helper()
	body, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("Marshal quiz prompt failed: %v", err)
	}
	var state QuizPromptState
	if err := json.Unmarshal(body, &state); err != nil {
		t.Fatalf("Unmarshal quiz prompt failed: %v", err)
	}
	return state
}

