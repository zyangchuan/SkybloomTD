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
- exactly 30 quizzes for the level

Each quiz can be either:
- mcq: exactly 3 markdown option strings
- true_false: exactly two markdown option strings, True and False

Every question_markdown value and every options_markdown item must be a markdown
string. answer_index must be the zero-based integer index of the correct option
in options_markdown. correct_option_markdown must exactly equal the correct
string from options_markdown. Before finalizing each quiz, solve it yourself and
verify that the chosen correct option is actually correct. If the OCR-derived
answer appears wrong, rewrite the options and answer key to the corrected answer.

When using math, produce KaTeX-compatible LaTeX only:
- wrap math in $...$, $$...$$, \(...\), or \[...\]
- use ASCII command names such as \frac, \sqrt, \sin, \cos, \tan, \ln, \log, and \cdot
- never use Unicode mathematical alphabet characters inside commands, for example 𝖿rac
- never write trig functions as \textsin or \text{sin}; use \sin
- do not put dangling quotes or backticks inside math delimiters
- because this is JSON, every displayed LaTeX backslash must be escaped as \\ in the JSON string`

const quizCount = 30

type Config struct {
	APIKey      string
	BaseURL     string
	Model       string
	Temperature float64
	Timeout     time.Duration
	MaxRetries  int
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
		"Generate the level for this sub-chapter.\n\nSub-chapter title: %s\nSub-chapter id: %s\n\nSource text:\n%s",
		firstNonEmpty(sourceContext.SubChapterTitle, "Untitled"),
		sourceContext.SubChapterID,
		sourceContext.SourceText,
	)

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
			if err := validateGeneration(generation); err == nil {
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
		"%s\n\nPrevious attempt failed validation: %s\nRegenerate the full level and obey every schema rule exactly. For true_false quizzes, options_markdown must contain exactly two strings: True and False. For mcq quizzes, options_markdown must contain exactly three strings. For every quiz, correct_option_markdown must exactly equal one option, and answer_index must point to that same option after you independently solve and verify the question.",
		prompt,
		validationErr.Error(),
	)
}

func (c *Client) callOpenAI(ctx context.Context, prompt string, generation *LevelGeneration) error {
	requestBody := map[string]any{
		"model":       c.config.Model,
		"temperature": c.config.Temperature,
		"input": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": prompt},
		},
		"text": map[string]any{
			"format": map[string]any{
				"type":   "json_schema",
				"name":   "level_generation",
				"strict": true,
				"schema": levelGenerationSchema(),
			},
		},
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
	if err := json.Unmarshal([]byte(outputText), generation); err != nil {
		return fmt.Errorf("decode level generation: %w", err)
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

func levelGenerationSchema() map[string]any {
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

func validateGeneration(generation LevelGeneration) error {
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
