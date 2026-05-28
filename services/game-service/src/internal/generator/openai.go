package generator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"skybloom/game-service/internal/source"
)

const systemPrompt = `You are an educational level generator.

Use only the source text provided by the user. Produce a complete level made of:
- a markdown summary for the level
- at least 10 quizzes for the level

Each quiz can be either:
- mcq: exactly 3 markdown option strings
- true_false: exactly two markdown option strings, True and False

Every question_markdown value and every options_markdown item must be a markdown
string. answer_index must be the zero-based integer index of the correct option
in options_markdown. Do not include facts that are not supported by the source
text.`

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
	QuizType         string   `json:"quiz_type"`
	QuestionMarkdown string   `json:"question_markdown"`
	OptionsMarkdown  []string `json:"options_markdown"`
	AnswerIndex      int      `json:"answer_index"`
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
		"%s\n\nPrevious attempt failed validation: %s\nRegenerate the full level and obey every schema rule exactly. For true_false quizzes, options_markdown must contain exactly two strings: True and False. For mcq quizzes, options_markdown must contain exactly three strings.",
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
	requiredQuizFields := []string{"quiz_type", "question_markdown", "options_markdown", "answer_index"}
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
				"minItems": 10,
				"items":    quizSchema,
			},
		},
	}
}

func normalizeGeneration(generation *LevelGeneration) {
	for index := range generation.Quizzes {
		quiz := &generation.Quizzes[index]
		if quiz.QuizType != "true_false" {
			continue
		}
		correct := ""
		if quiz.AnswerIndex >= 0 && quiz.AnswerIndex < len(quiz.OptionsMarkdown) {
			correct = normalizedTrueFalseOption(quiz.OptionsMarkdown[quiz.AnswerIndex])
		}
		if correct == "" {
			continue
		}
		quiz.OptionsMarkdown = []string{"True", "False"}
		if correct == "true" {
			quiz.AnswerIndex = 0
		} else {
			quiz.AnswerIndex = 1
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

func validateGeneration(generation LevelGeneration) error {
	if strings.TrimSpace(generation.SummaryMarkdown) == "" {
		return errors.New("summary_markdown cannot be empty")
	}
	if len(generation.Quizzes) < 10 {
		return errors.New("generate at least 10 quizzes")
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
		if quiz.AnswerIndex < 0 || quiz.AnswerIndex >= len(quiz.OptionsMarkdown) {
			return fmt.Errorf("quiz %d answer_index must point to an option", index)
		}
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
