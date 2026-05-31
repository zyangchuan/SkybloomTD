package quiztext

import "testing"

func TestSanitizeMarkdownRepairsStylizedLatexCommand(t *testing.T) {
	got := SanitizeMarkdown(`Differentiate $dy = \cos(u) \cdot 𝖿rac{du}{dx}$`)
	want := `Differentiate $dy = \cos(u) \cdot \frac{du}{dx}$`
	if got != want {
		t.Fatalf("SanitizeMarkdown() = %q, want %q", got, want)
	}
}

func TestSanitizeMarkdownRepairsJSONControlEscapes(t *testing.T) {
	got := SanitizeMarkdown("$\frac{du}{dx} + \textsin(x)$")
	want := `$\frac{du}{dx} + \sin(x)$`
	if got != want {
		t.Fatalf("SanitizeMarkdown() = %q, want %q", got, want)
	}
}

func TestSanitizeMarkdownStripsDanglingMathQuotes(t *testing.T) {
	got := SanitizeMarkdown("Solve $'x + 1`$")
	want := `Solve $x + 1$`
	if got != want {
		t.Fatalf("SanitizeMarkdown() = %q, want %q", got, want)
	}
}

func TestSanitizeMarkdownKeepsPrimeNotation(t *testing.T) {
	got := SanitizeMarkdown("$f'(x)$")
	want := "$f'(x)$"
	if got != want {
		t.Fatalf("SanitizeMarkdown() = %q, want %q", got, want)
	}
}
