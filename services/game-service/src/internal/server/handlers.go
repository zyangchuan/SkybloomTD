package server

import (
	"context"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"skybloom/game-service/internal/gamesession"
	"skybloom/game-service/internal/models"
	"skybloom/game-service/internal/quizcache"
	"skybloom/game-service/internal/repository"
	"skybloom/game-service/internal/quiztext"
)

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
	if loop != nil {
		if err := s.levels.ClearQuizMistakes(callCtx, loop.userID, loop.levelID); err != nil {
			log.Printf("failed to clear quiz mistakes on exit level_id=%s user_id=%s: %v", loop.levelID, loop.userID, err)
		}
	} else if runtime, err := s.sessions.LoadRuntimeState(callCtx, sessionID); err == nil {
		generationID = runtime.GenerationID
	} else if !errors.Is(err, gamesession.ErrSessionNotFound) {
		log.Printf("failed to load game session runtime for quiz cache cleanup session_id=%s: %v", sessionID, err)
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

func (s *Server) handleQuizRequest(ctx context.Context, conn *websocket.Conn, writeMu *sync.Mutex, loop *runningGameLoop) error {
	if loop == nil || loop.stopped() {
		return errors.New("game session is not running")
	}
	if s.quizzes == nil {
		return errors.New("quiz cache is not configured")
	}
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	quiz, remaining, err := s.quizzes.PeekRandom(callCtx, loop.generationID)
	if errors.Is(err, quizcache.ErrQuizzesNotFound) {
		loop.currentQuizID = ""
		return writeWebsocketJSON(conn, writeMu, Message{
			Type: "game.quiz.unavailable",
			Data: QuizUnavailableState{Reason: "no_quizzes_remaining"},
		})
	}
	if err != nil {
		return errors.New("failed to load quiz")
	}
	loop.currentQuizID = quiz.ID
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

	quizzes, err := s.quizzes.Get(callCtx, loop.generationID)
	if errors.Is(err, quizcache.ErrQuizzesNotFound) {
		loop.currentQuizID = ""
		return writeWebsocketJSON(conn, writeMu, Message{
			Type: "game.quiz.unavailable",
			Data: QuizUnavailableState{Reason: "no_quizzes_remaining"},
		})
	}
	if err != nil {
		return errors.New("failed to load quiz")
	}
	expectedQuizID := loop.currentQuizID
	if expectedQuizID == "" {
		expectedQuizID = quizzes.CurrentQuizID
	}
	if expectedQuizID == "" && len(quizzes.Quizzes) > 0 {
		expectedQuizID = quizzes.Quizzes[0].ID
	}
	if expectedQuizID != "" && expectedQuizID != request.QuizID {
		return errors.New("quiz_id is not the current quiz")
	}
	currentQuiz, ok := cachedQuizByID(quizzes.Quizzes, request.QuizID)
	if !ok {
		return errors.New("quiz not found")
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
	if loop.currentQuizID == request.QuizID {
		loop.currentQuizID = ""
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

func (s *Server) deleteQuizCache(ctx context.Context, generationID string) error {
	generationID = strings.TrimSpace(generationID)
	if s.quizzes == nil || generationID == "" {
		return nil
	}
	return s.quizzes.Delete(ctx, generationID)
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
