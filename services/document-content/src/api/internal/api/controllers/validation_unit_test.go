package controllers

import "testing"

func TestSafePathPartSanitizesInput(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "keeps safe characters", input: "chapter_1-v2.pdf", want: "chapter_1-v2.pdf"},
		{name: "replaces unsafe characters", input: " ../Sky Bloom?! ", want: "Sky_Bloom"},
		{name: "uses fallback for empty result", input: " ... ", want: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := safePathPart(tt.input); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestSafeFilenameNormalizesUploadName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "keeps pdf name", input: "rulebook.pdf", want: "rulebook.pdf"},
		{name: "strips directory and unsafe characters", input: "../Sky Bloom?! 1.pdf", want: "Sky_Bloom_1.pdf"},
		{name: "adds pdf extension", input: "rulebook", want: "rulebook.pdf"},
		{name: "uses fallback for blank", input: " ", want: "input.pdf"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := safeFilename(tt.input); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestNormalizeGameName(t *testing.T) {
	got := normalizeGameName("  Sky   Bloom \n Tower   Defense  ")
	want := "Sky Bloom Tower Defense"

	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
