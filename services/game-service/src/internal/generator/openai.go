package generator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"skybloom/game-service/internal/quiztext"
	"skybloom/game-service/internal/source"
)

const systemPrompt = `You are an educational level generator.

The source text provided by the user is extracted with OCR and may contain
malformed symbols, broken spacing, incorrect LaTeX, or obvious recognition
mistakes. Treat the source as the lesson topic and evidence, but not as
infallible. Use your stable subject-matter knowledge to repair OCR mistakes,
especially in formulas, notation, definitions, and answer choices. If the OCR
content conflicts with well-known facts or rules, use the corrected fact or
rule. Do not invent a new topic that is unrelated to the source; when a source
line is too damaged to recover confidently, generate quizzes from clearer
nearby content instead.

Produce a complete level made of:
- a markdown summary for the level
- exactly the requested number of quizzes for the level

All quizzes in the output must be unique. Do not repeat the same question,
same tested fact, same calculation, or same learning check with different
wording. Each quiz should assess a distinct concept, detail, procedure,
example, or implication from the sub-chapter.

Each quiz can be either:
- mcq: exactly 3 markdown option strings
- true_false: exactly two markdown option strings, True and False

Every question_markdown value and every options_markdown item must be a markdown
string. answer_index must be the zero-based integer index of the correct option
in options_markdown. correct_option_markdown must exactly equal the correct
string from options_markdown. Before finalizing each quiz, solve it yourself and
verify that the chosen correct option is actually correct. If the OCR-derived
answer appears wrong, rewrite the options and answer key to the corrected answer.

Every quiz must be self-contained from a learner's perspective. Do not require
the player to see the original document, source order, an earlier example, an
unnamed equation, a figure, an author's note, or the surrounding passage to
understand what is being asked. When testing a specific expression, claim,
definition, or scenario, restate the needed context directly in the question.
Prefer questions that teach or check the sub-chapter concept over questions
about where something appeared in the source document.

When using math, produce KaTeX-compatible LaTeX only:
- wrap math in $...$, $$...$$, \(...\), or \[...\]
- use ASCII command names such as \frac, \sqrt, \sin, \cos, \tan, \ln, \log, and \cdot
- never use Unicode mathematical alphabet characters inside commands, for example 𝖿rac
- never write trig functions as \textsin or \text{sin}; use \sin
- do not put dangling quotes or backticks inside math delimiters
- because this is JSON, every displayed LaTeX backslash must be escaped as \\ in the JSON string`

const defaultQuizCount = 5
const defaultVerifierConcurrency = 6

const blindVerifierSystemPrompt = `You are a quiz quality reviewer acting like a learner.

You see only one quiz, the level summary, and the sub-chapter title. Judge
whether a real player could answer the question from the quiz itself and normal
learning context, without seeing the original document. Reject questions that
depend on hidden source references, document position, unnamed examples,
unstated equations, figures, passages, author intent, or missing scenario
details. Also reject questions that are vague, not educationally useful, or have
multiple plausible answers from the visible wording alone.

Return strict JSON only.`

const groundedVerifierSystemPrompt = `You are a quiz correctness reviewer.

You receive one quiz and the source text used to create the level. Judge whether
the quiz is relevant to the selected sub-chapter, whether the marked answer is
supported, and whether the distractors are clearly incorrect. The source may
contain OCR errors, so allow obvious repairs to notation and formatting, but
reject unsupported facts or questions about irrelevant/non-learning material.

Return strict JSON only.`

type Config struct {
	APIKey              string
	BaseURL             string
	Model               string
	Temperature         float64
	Timeout             time.Duration
	MaxRetries          int
	QuizCount           int
	VerifierConcurrency int
}

type Client struct {
	config     Config
	httpClient *http.Client
}

type LevelGeneration struct {
	SummaryMarkdown string     `json:"summary_markdown"`
	Quizzes         []QuizItem `json:"quizzes"`
}

type QuizItem struct {
	QuizType              string   `json:"quiz_type"`
	QuestionMarkdown      string   `json:"question_markdown"`
	OptionsMarkdown       []string `json:"options_markdown"`
	AnswerIndex           int      `json:"answer_index"`
	CorrectOptionMarkdown string   `json:"correct_option_markdown"`
}

type ExistingQuiz struct {
	QuizIndex        int      `json:"quiz_index"`
	QuestionMarkdown string   `json:"question_markdown"`
	OptionsMarkdown  []string `json:"options_markdown"`
}

type quizVerification struct {
	Pass          bool   `json:"pass"`
	Answerable    bool   `json:"answerable"`
	Correct       bool   `json:"correct"`
	Useful        bool   `json:"useful"`
	Relevant      bool   `json:"relevant"`
	FailureReason string `json:"failure_reason"`
}

func NewClient(cfg Config) *Client {
	return &Client{
		config: cfg,
		httpClient: &http.Client{
			Timeout: cfg.Timeout + 15*time.Second,
		},
	}
}

func (c *Client) GenerateLevel(ctx context.Context, sourceContext source.SourceContext) (LevelGeneration, error) {
	if sourceContext.Status != "retrieved" {
		return LevelGeneration{}, errors.New("source context was not retrieved successfully")
	}

	prompt := fmt.Sprintf(
		"Generate the level for this sub-chapter.\n\nQuiz count: %d\nSub-chapter title: %s\nSub-chapter id: %s\n\nSource text:\n%s",
		c.quizCount(),
		firstNonEmpty(sourceContext.SubChapterTitle, "Untitled"),
		sourceContext.SubChapterID,
		sourceContext.SourceText,
	)
	return c.generateWithPrompt(ctx, sourceContext, prompt, nil)
}

func (c *Client) GenerateQuizRefill(ctx context.Context, sourceContext source.SourceContext, existingQuizzes []ExistingQuiz) (LevelGeneration, error) {
	if sourceContext.Status != "retrieved" {
		return LevelGeneration{}, errors.New("source context was not retrieved successfully")
	}
	existingBody, err := json.Marshal(existingQuizzes)
	if err != nil {
		return LevelGeneration{}, err
	}
	prompt := fmt.Sprintf(
		"Generate an additional quiz refill batch for this existing level.\n\nQuiz count: %d\nSub-chapter title: %s\nSub-chapter id: %s\n\nEvery new quiz must be unique. Do not repeat any existing quiz word-for-word, and do not create a near-duplicate that tests the same fact, calculation, definition, or concept with different wording. Use different parts or angles of the source text for each new quiz.\n\nExisting quizzes to avoid repeating:\n%s\n\nSource text:\n%s",
		c.quizCount(),
		firstNonEmpty(sourceContext.SubChapterTitle, "Untitled"),
		sourceContext.SubChapterID,
		string(existingBody),
		sourceContext.SourceText,
	)
	return c.generateWithPrompt(ctx, sourceContext, prompt, refillDuplicateValidator(existingQuizzes))
}

func (c *Client) generateWithPrompt(
	ctx context.Context,
	sourceContext source.SourceContext,
	prompt string,
	extraValidator func(LevelGeneration) error,
) (LevelGeneration, error) {
	var generation LevelGeneration
	var lastErr error
	var lastValidationErr error
	attempts := c.config.MaxRetries + 1
	if attempts < 1 {
		attempts = 1
	}
	for attempt := 1; attempt <= attempts; attempt++ {
		attemptPrompt := prompt
		if lastValidationErr != nil {
			attemptPrompt = promptWithValidationFailure(prompt, lastValidationErr)
		}
		generation = LevelGeneration{}
		callCtx, cancel := context.WithTimeout(ctx, c.config.Timeout)
		err := c.callOpenAI(callCtx, attemptPrompt, &generation)
		cancel()
		validationFailed := false
		if err == nil {
			normalizeGeneration(&generation)
			if err := c.validateGeneration(generation); err == nil {
				if extraValidator != nil {
					if err := extraValidator(generation); err != nil {
						lastValidationErr = err
						validationFailed = true
						lastErr = fmt.Errorf("validate level generation: %w", err)
						continue
					}
				}
				if err := c.verifyGeneration(ctx, sourceContext, generation); err != nil {
					lastValidationErr = err
					validationFailed = true
					lastErr = fmt.Errorf("verify level generation: %w", err)
					continue
				}
				return generation, nil
			} else {
				lastValidationErr = err
				validationFailed = true
				lastErr = fmt.Errorf("validate level generation: %w", err)
			}
		} else {
			lastErr = err
		}
		if attempt < attempts {
			if !validationFailed {
				time.Sleep(time.Duration(attempt) * time.Second)
			}
		}
	}
	return LevelGeneration{}, lastErr
}

func promptWithValidationFailure(prompt string, validationErr error) string {
	return fmt.Sprintf(
		"%s\n\nPrevious attempt failed validation or learner-perspective verification: %s\nRegenerate the full level and obey every schema rule exactly. For true_false quizzes, options_markdown must contain exactly two strings: True and False. For mcq quizzes, options_markdown must contain exactly three strings. For every quiz, correct_option_markdown must exactly equal one option, and answer_index must point to that same option after you independently solve and verify the question. Make every question self-contained: restate any needed expression, equation, definition, scenario, or claim directly in the question, and avoid questions that depend on the learner seeing the original source document. Ensure every quiz is unique and does not repeat an existing question or another generated question.",
		prompt,
		validationErr.Error(),
	)
}

func (c *Client) callOpenAI(ctx context.Context, prompt string, generation *LevelGeneration) error {
	return c.callStructured(ctx, systemPrompt, prompt, c.levelGenerationFormat(), generation)
}

func (c *Client) callStructured(ctx context.Context, system string, prompt string, format map[string]any, target any) error {
	requestBody := map[string]any{
		"model":       c.config.Model,
		"temperature": c.config.Temperature,
		"input": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": prompt},
		},
		"text": map[string]any{"format": format},
	}
	body, err := json.Marshal(requestBody)
	if err != nil {
		return err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.BaseURL+"/responses", bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	request.Header.Set("Content-Type", "application/json")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("openai response status %d: %s", response.StatusCode, string(responseBody))
	}

	outputText, err := responseOutputText(responseBody)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(outputText), target); err != nil {
		return fmt.Errorf("decode structured OpenAI response: %w", err)
	}
	return nil
}

func responseOutputText(data []byte) (string, error) {
	var response struct {
		Output []struct {
			Type    string `json:"type"`
			Content []struct {
				Type    string `json:"type"`
				Text    string `json:"text"`
				Refusal string `json:"refusal"`
			} `json:"content"`
		} `json:"output"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return "", err
	}
	if response.Error != nil && response.Error.Message != "" {
		return "", errors.New(response.Error.Message)
	}
	for _, output := range response.Output {
		if output.Type != "message" {
			continue
		}
		for _, content := range output.Content {
			if content.Type == "refusal" {
				return "", fmt.Errorf("model refusal: %s", content.Refusal)
			}
			if content.Type == "output_text" && strings.TrimSpace(content.Text) != "" {
				return content.Text, nil
			}
		}
	}
	return "", errors.New("OpenAI response did not include output_text")
}

func (c *Client) verifyGeneration(ctx context.Context, sourceContext source.SourceContext, generation LevelGeneration) error {
	concurrency := c.config.VerifierConcurrency
	if concurrency <= 0 {
		concurrency = defaultVerifierConcurrency
	}
	if concurrency > len(generation.Quizzes) {
		concurrency = len(generation.Quizzes)
	}
	if concurrency < 1 {
		concurrency = 1
	}

	type verificationJob struct {
		index int
		quiz  QuizItem
	}

	jobs := make(chan verificationJob)
	results := make(chan error, len(generation.Quizzes))
	var wg sync.WaitGroup

	for workerIndex := 0; workerIndex < concurrency; workerIndex++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				results <- c.verifyQuiz(ctx, sourceContext, generation.SummaryMarkdown, job.index, job.quiz)
			}
		}()
	}

	for index, quiz := range generation.Quizzes {
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return ctx.Err()
		case jobs <- verificationJob{index: index, quiz: quiz}:
		}
	}
	close(jobs)
	wg.Wait()
	close(results)

	failures := make([]string, 0)
	for err := range results {
		if err != nil {
			failures = append(failures, err.Error())
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("%d quiz quality checks failed: %s", len(failures), strings.Join(failures, "; "))
	}
	return nil
}

func (c *Client) verifyQuiz(ctx context.Context, sourceContext source.SourceContext, summary string, index int, quiz QuizItem) error {
	blindPrompt := fmt.Sprintf(
		"Review this quiz from the learner's perspective. The learner does not see the original source text.\n\nSub-chapter title: %s\nLevel summary:\n%s\n\nQuiz index: %d\n%s",
		firstNonEmpty(sourceContext.SubChapterTitle, "Untitled"),
		summary,
		index,
		quizReviewPayload(quiz),
	)
	blind, err := c.callQuizVerifier(ctx, blindVerifierSystemPrompt, blindPrompt)
	if err != nil {
		return fmt.Errorf("quiz %d blind verifier error: %w", index, err)
	}
	if !blind.Pass || !blind.Answerable || !blind.Useful {
		return fmt.Errorf("quiz %d is not self-contained/useful: %s", index, firstNonEmpty(blind.FailureReason, "blind verifier rejected it"))
	}

	groundedPrompt := fmt.Sprintf(
		"Review this quiz against the source text.\n\nSub-chapter title: %s\nSource text:\n%s\n\nQuiz index: %d\n%s",
		firstNonEmpty(sourceContext.SubChapterTitle, "Untitled"),
		sourceContext.SourceText,
		index,
		quizReviewPayload(quiz),
	)
	grounded, err := c.callQuizVerifier(ctx, groundedVerifierSystemPrompt, groundedPrompt)
	if err != nil {
		return fmt.Errorf("quiz %d grounded verifier error: %w", index, err)
	}
	if !grounded.Pass || !grounded.Correct || !grounded.Relevant {
		return fmt.Errorf("quiz %d is not correct/relevant: %s", index, firstNonEmpty(grounded.FailureReason, "grounded verifier rejected it"))
	}

	return nil
}

func (c *Client) callQuizVerifier(ctx context.Context, system string, prompt string) (quizVerification, error) {
	var verification quizVerification
	callCtx, cancel := context.WithTimeout(ctx, c.config.Timeout)
	defer cancel()
	if err := c.callStructured(callCtx, system, prompt, quizVerificationFormat(), &verification); err != nil {
		return quizVerification{}, err
	}
	verification.FailureReason = strings.TrimSpace(verification.FailureReason)
	return verification, nil
}

func quizReviewPayload(quiz QuizItem) string {
	payload := map[string]any{
		"quiz_type":               quiz.QuizType,
		"question_markdown":       quiz.QuestionMarkdown,
		"options_markdown":        quiz.OptionsMarkdown,
		"answer_index":            quiz.AnswerIndex,
		"correct_option_markdown": quiz.CorrectOptionMarkdown,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func (c *Client) levelGenerationFormat() map[string]any {
	return map[string]any{
		"type":   "json_schema",
		"name":   "level_generation",
		"strict": true,
		"schema": levelGenerationSchema(c.quizCount()),
	}
}

func quizVerificationFormat() map[string]any {
	return map[string]any{
		"type":   "json_schema",
		"name":   "quiz_verification",
		"strict": true,
		"schema": quizVerificationSchema(),
	}
}

func levelGenerationSchema(quizCount int) map[string]any {
	requiredQuizFields := []string{"quiz_type", "question_markdown", "options_markdown", "answer_index", "correct_option_markdown"}
	mcqQuizSchema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             requiredQuizFields,
		"properties": map[string]any{
			"quiz_type": map[string]any{
				"type": "string",
				"enum": []string{"mcq"},
			},
			"question_markdown": map[string]any{"type": "string"},
			"options_markdown": map[string]any{
				"type":     "array",
				"minItems": 3,
				"maxItems": 3,
				"items":    map[string]any{"type": "string"},
			},
			"answer_index": map[string]any{
				"type":    "integer",
				"minimum": 0,
				"maximum": 2,
			},
			"correct_option_markdown": map[string]any{"type": "string"},
		},
	}
	trueFalseQuizSchema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             requiredQuizFields,
		"properties": map[string]any{
			"quiz_type": map[string]any{
				"type": "string",
				"enum": []string{"true_false"},
			},
			"question_markdown": map[string]any{"type": "string"},
			"options_markdown": map[string]any{
				"type":     "array",
				"minItems": 2,
				"maxItems": 2,
				"items": map[string]any{
					"type": "string",
					"enum": []string{"True", "False"},
				},
			},
			"answer_index": map[string]any{
				"type":    "integer",
				"minimum": 0,
				"maximum": 1,
			},
			"correct_option_markdown": map[string]any{
				"type": "string",
				"enum": []string{"True", "False"},
			},
		},
	}
	quizSchema := map[string]any{
		"anyOf": []map[string]any{
			mcqQuizSchema,
			trueFalseQuizSchema,
		},
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"summary_markdown", "quizzes"},
		"properties": map[string]any{
			"summary_markdown": map[string]any{"type": "string"},
			"quizzes": map[string]any{
				"type":     "array",
				"minItems": quizCount,
				"maxItems": quizCount,
				"items":    quizSchema,
			},
		},
	}
}

func quizVerificationSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"pass", "answerable", "correct", "useful", "relevant", "failure_reason"},
		"properties": map[string]any{
			"pass":           map[string]any{"type": "boolean"},
			"answerable":     map[string]any{"type": "boolean"},
			"correct":        map[string]any{"type": "boolean"},
			"useful":         map[string]any{"type": "boolean"},
			"relevant":       map[string]any{"type": "boolean"},
			"failure_reason": map[string]any{"type": "string"},
		},
	}
}

func normalizeGeneration(generation *LevelGeneration) {
	generation.SummaryMarkdown = quiztext.SanitizeMarkdown(generation.SummaryMarkdown)
	for index := range generation.Quizzes {
		quiz := &generation.Quizzes[index]
		quiz.QuestionMarkdown = quiztext.SanitizeMarkdown(quiz.QuestionMarkdown)
		quiz.OptionsMarkdown = quiztext.SanitizeMarkdownSlice(quiz.OptionsMarkdown)
		quiz.CorrectOptionMarkdown = quiztext.SanitizeMarkdown(quiz.CorrectOptionMarkdown)
	}

	// First, normalize true_false options to exact "True" and "False" values
	for index := range generation.Quizzes {
		quiz := &generation.Quizzes[index]
		if quiz.QuizType == "true_false" {
			normalizeTrueFalseQuiz(quiz)
		} else {
			normalizeAnswerIndexFromCorrectOption(quiz)
		}
	}

	// Shuffle options for all quizzes to eliminate any positional/LLM bias
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	for index := range generation.Quizzes {
		shuffleQuizOptions(r, &generation.Quizzes[index])
	}
}

type quizOption struct {
	markdown string
	correct  bool
}

func shuffleQuizOptions(r *rand.Rand, quiz *QuizItem) {
	if len(quiz.OptionsMarkdown) <= 1 {
		return
	}
	correctIndex := matchingOptionIndex(quiz.OptionsMarkdown, quiz.CorrectOptionMarkdown)
	if correctIndex < 0 {
		return
	}
	quiz.AnswerIndex = correctIndex

	options := make([]quizOption, 0, len(quiz.OptionsMarkdown))
	for index, option := range quiz.OptionsMarkdown {
		options = append(options, quizOption{
			markdown: option,
			correct:  index == quiz.AnswerIndex,
		})
	}

	r.Shuffle(len(options), func(i, j int) {
		options[i], options[j] = options[j], options[i]
	})

	for index, option := range options {
		quiz.OptionsMarkdown[index] = option.markdown
		if option.correct {
			quiz.AnswerIndex = index
			quiz.CorrectOptionMarkdown = option.markdown
		}
	}
}

func normalizedTrueFalseOption(option string) string {
	value := strings.ToLower(strings.TrimSpace(option))
	value = strings.Trim(value, " \t\n\r`*_:.!?")
	switch value {
	case "true":
		return "true"
	case "false":
		return "false"
	default:
		return ""
	}
}

func normalizeTrueFalseQuiz(quiz *QuizItem) {
	correct := normalizedTrueFalseOption(quiz.CorrectOptionMarkdown)
	if correct == "" && quiz.AnswerIndex >= 0 && quiz.AnswerIndex < len(quiz.OptionsMarkdown) {
		correct = normalizedTrueFalseOption(quiz.OptionsMarkdown[quiz.AnswerIndex])
	}
	if correct == "" {
		return
	}

	quiz.OptionsMarkdown = []string{"True", "False"}
	if correct == "true" {
		quiz.AnswerIndex = 0
		quiz.CorrectOptionMarkdown = "True"
	} else {
		quiz.AnswerIndex = 1
		quiz.CorrectOptionMarkdown = "False"
	}
}

func normalizeAnswerIndexFromCorrectOption(quiz *QuizItem) {
	if index := matchingOptionIndex(quiz.OptionsMarkdown, quiz.CorrectOptionMarkdown); index >= 0 {
		quiz.AnswerIndex = index
		quiz.CorrectOptionMarkdown = quiz.OptionsMarkdown[index]
	}
}

func (c *Client) validateGeneration(generation LevelGeneration) error {
	quizCount := c.quizCount()
	if strings.TrimSpace(generation.SummaryMarkdown) == "" {
		return errors.New("summary_markdown cannot be empty")
	}
	if len(generation.Quizzes) != quizCount {
		return fmt.Errorf("generate exactly %d quizzes", quizCount)
	}
	for index, quiz := range generation.Quizzes {
		if strings.TrimSpace(quiz.QuestionMarkdown) == "" {
			return fmt.Errorf("quiz %d question_markdown cannot be empty", index)
		}
		if quiz.QuizType != "mcq" && quiz.QuizType != "true_false" {
			return fmt.Errorf("quiz %d has invalid quiz_type", index)
		}
		if quiz.QuizType == "mcq" && len(quiz.OptionsMarkdown) != 3 {
			return fmt.Errorf("quiz %d mcq must include exactly three options", index)
		}
		if quiz.QuizType == "true_false" {
			if len(quiz.OptionsMarkdown) != 2 {
				return fmt.Errorf("quiz %d true_false must include exactly two options", index)
			}
			options := map[string]bool{}
			for _, option := range quiz.OptionsMarkdown {
				options[strings.ToLower(strings.TrimSpace(option))] = true
			}
			if !options["true"] || !options["false"] || len(options) != 2 {
				return fmt.Errorf("quiz %d true_false options must be True and False", index)
			}
		}
		if hasDuplicateOptions(quiz.OptionsMarkdown) {
			return fmt.Errorf("quiz %d options_markdown must contain unique options", index)
		}
		if quiz.AnswerIndex < 0 || quiz.AnswerIndex >= len(quiz.OptionsMarkdown) {
			return fmt.Errorf("quiz %d answer_index must point to an option", index)
		}
		correctIndex := matchingOptionIndex(quiz.OptionsMarkdown, quiz.CorrectOptionMarkdown)
		if correctIndex < 0 {
			return fmt.Errorf("quiz %d correct_option_markdown must exactly match one option", index)
		}
		if quiz.AnswerIndex != correctIndex {
			return fmt.Errorf("quiz %d answer_index must point to correct_option_markdown", index)
		}
	}
	return nil
}

func refillDuplicateValidator(existingQuizzes []ExistingQuiz) func(LevelGeneration) error {
	existingQuestions := map[string]struct{}{}
	for _, quiz := range existingQuizzes {
		key := normalizeQuestionForMatch(quiz.QuestionMarkdown)
		if key != "" {
			existingQuestions[key] = struct{}{}
		}
	}
	return func(generation LevelGeneration) error {
		batchQuestions := map[string]struct{}{}
		for index, quiz := range generation.Quizzes {
			key := normalizeQuestionForMatch(quiz.QuestionMarkdown)
			if key == "" {
				continue
			}
			if _, ok := existingQuestions[key]; ok {
				return fmt.Errorf("refill quiz %d repeats an existing quiz question; generate a different question", index)
			}
			if _, ok := batchQuestions[key]; ok {
				return fmt.Errorf("refill quiz %d repeats another refill quiz question; generate varied questions", index)
			}
			batchQuestions[key] = struct{}{}
		}
		return nil
	}
}

func (c *Client) quizCount() int {
	if c.config.QuizCount > 0 {
		return c.config.QuizCount
	}
	return defaultQuizCount
}

func matchingOptionIndex(options []string, option string) int {
	normalizedOption := normalizeOptionForMatch(option)
	if normalizedOption == "" {
		return -1
	}
	for index, candidate := range options {
		if normalizeOptionForMatch(candidate) == normalizedOption {
			return index
		}
	}
	return -1
}

func normalizeOptionForMatch(option string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(option)), " "))
}

func normalizeQuestionForMatch(question string) string {
	value := quiztext.SanitizeMarkdown(question)
	value = strings.ToLower(value)
	replacer := strings.NewReplacer(
		"`", "",
		"*", "",
		"_", "",
		"#", "",
		"$", "",
		"\\(", "",
		"\\)", "",
		"\\[", "",
		"\\]", "",
	)
	value = replacer.Replace(value)
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func hasDuplicateOptions(options []string) bool {
	seen := map[string]struct{}{}
	for _, option := range options {
		key := normalizeOptionForMatch(option)
		if _, ok := seen[key]; ok {
			return true
		}
		seen[key] = struct{}{}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
