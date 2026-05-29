package generator

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"skybloom/game-service/internal/source"
)

func TestGenerateLevelRetriesValidationFailure(t *testing.T) {
	client := NewClient(Config{
		APIKey:     "test-key",
		BaseURL:    "https://openai.test/v1",
		Model:      "test-model",
		Timeout:    5 * time.Second,
		MaxRetries: 1,
	})
	transport := &fakeOpenAITransport{t: t}
	client.httpClient = &http.Client{Transport: transport}

	generation, err := client.GenerateLevel(context.Background(), source.SourceContext{
		Status:          "retrieved",
		SubChapterID:    "sub-1",
		SourceText:      "Lesson text",
		SubChapterTitle: "Lesson",
	})
	if err != nil {
		t.Fatalf("GenerateLevel failed: %v", err)
	}
	if len(transport.prompts) != 2 {
		t.Fatalf("expected two model calls, got %d", len(transport.prompts))
	}
	if !strings.Contains(transport.prompts[1], "Previous attempt failed validation") {
		t.Fatalf("expected validation feedback in retry prompt, got %q", transport.prompts[1])
	}
	if len(generation.Quizzes) != quizCount {
		t.Fatalf("expected %d quizzes, got %d", quizCount, len(generation.Quizzes))
	}
}

func TestGenerateLevelRepairsTrueFalseOptions(t *testing.T) {
	client := NewClient(Config{
		APIKey:     "test-key",
		BaseURL:    "https://openai.test/v1",
		Model:      "test-model",
		Timeout:    5 * time.Second,
		MaxRetries: 1,
	})
	transport := &fakeOpenAITransport{t: t, firstGeneration: oneOptionTrueFalseGeneration()}
	client.httpClient = &http.Client{Transport: transport}

	generation, err := client.GenerateLevel(context.Background(), source.SourceContext{
		Status:          "retrieved",
		SubChapterID:    "sub-1",
		SourceText:      "Lesson text",
		SubChapterTitle: "Lesson",
	})
	if err != nil {
		t.Fatalf("GenerateLevel failed: %v", err)
	}
	if len(transport.prompts) != 1 {
		t.Fatalf("expected one model call after repair, got %d", len(transport.prompts))
	}
	got := generation.Quizzes[0].OptionsMarkdown
	if len(got) != 2 || !((got[0] == "True" && got[1] == "False") || (got[0] == "False" && got[1] == "True")) {
		t.Fatalf("expected normalized true/false options, got %#v", got)
	}
	correctOption := generation.Quizzes[0].OptionsMarkdown[generation.Quizzes[0].AnswerIndex]
	if correctOption != "True" {
		t.Fatalf("expected answer index to stay mapped to True, got %d (option %s)", generation.Quizzes[0].AnswerIndex, correctOption)
	}
}

type fakeOpenAITransport struct {
	t               *testing.T
	prompts         []string
	firstGeneration *LevelGeneration
}

func (t *fakeOpenAITransport) RoundTrip(request *http.Request) (*http.Response, error) {
	var body struct {
		Input []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"input"`
	}
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		t.t.Fatalf("decode request: %v", err)
	}
	for _, input := range body.Input {
		if input.Role == "user" {
			t.prompts = append(t.prompts, input.Content)
		}
	}

	generation := validGeneration()
	if len(t.prompts) == 1 {
		if t.firstGeneration != nil {
			generation = *t.firstGeneration
		} else {
			generation = invalidTrueFalseGeneration()
		}
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(openAIResponseBody(t.t, generation))),
		Request:    request,
	}, nil
}

func openAIResponseBody(t *testing.T, generation LevelGeneration) string {
	t.Helper()
	output, err := json.Marshal(generation)
	if err != nil {
		t.Fatalf("marshal generation: %v", err)
	}
	response := map[string]any{
		"output": []map[string]any{
			{
				"type": "message",
				"content": []map[string]string{
					{
						"type": "output_text",
						"text": string(output),
					},
				},
			},
		},
	}
	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	return string(data)
}

func invalidTrueFalseGeneration() LevelGeneration {
	generation := validGeneration()
	generation.Quizzes[0].OptionsMarkdown = []string{"Maybe"}
	return generation
}

func oneOptionTrueFalseGeneration() *LevelGeneration {
	generation := validGeneration()
	generation.Quizzes[0].OptionsMarkdown = []string{"True"}
	return &generation
}

func validGeneration() LevelGeneration {
	quizzes := make([]QuizItem, 0, quizCount)
	for i := 0; i < quizCount; i++ {
		quizzes = append(quizzes, QuizItem{
			QuizType:         "true_false",
			QuestionMarkdown: "Question?",
			OptionsMarkdown:  []string{"True", "False"},
			AnswerIndex:      i % 2,
		})
	}
	return LevelGeneration{
		SummaryMarkdown: "Summary",
		Quizzes:         quizzes,
	}
}
