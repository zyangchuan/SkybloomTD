package quiztext

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

var controlEscapeReplacer = strings.NewReplacer(
	"\b"+"eta", `\beta`,
	"\f"+"rac", `\frac`,
	"\n"+"abla", `\nabla`,
	"\n"+"eq", `\neq`,
	"\r"+"ight", `\right`,
	"\r"+"ho", `\rho`,
	"\t"+"an", `\tan`,
	"\t"+"ext", `\text`,
	"\t"+"heta", `\theta`,
	"\t"+"imes", `\times`,
	"\v"+"arepsilon", `\varepsilon`,
	"\v"+"arphi", `\varphi`,
)

var trigTextAliases = []struct {
	alias   string
	command string
}{
	{`\text{arcsin}`, `\arcsin`},
	{`\text{arccos}`, `\arccos`},
	{`\text{arctan}`, `\arctan`},
	{`\text{sin}`, `\sin`},
	{`\text{cos}`, `\cos`},
	{`\text{tan}`, `\tan`},
	{`\text{cot}`, `\cot`},
	{`\text{sec}`, `\sec`},
	{`\text{csc}`, `\csc`},
	{`\text{ln}`, `\ln`},
	{`\text{log}`, `\log`},
	{`\textarcsin`, `\arcsin`},
	{`\textarccos`, `\arccos`},
	{`\textarctan`, `\arctan`},
	{`\textsin`, `\sin`},
	{`\textcos`, `\cos`},
	{`\texttan`, `\tan`},
	{`\textcot`, `\cot`},
	{`\textsec`, `\sec`},
	{`\textcsc`, `\csc`},
	{`\textln`, `\ln`},
	{`\textlog`, `\log`},
	{`textarcsin`, `\arcsin`},
	{`textarccos`, `\arccos`},
	{`textarctan`, `\arctan`},
	{`textsin`, `\sin`},
	{`textcos`, `\cos`},
	{`texttan`, `\tan`},
	{`textcot`, `\cot`},
	{`textsec`, `\sec`},
	{`textcsc`, `\csc`},
	{`textln`, `\ln`},
	{`textlog`, `\log`},
	{`extarcsin`, `\arcsin`},
	{`extarccos`, `\arccos`},
	{`extarctan`, `\arctan`},
	{`extsin`, `\sin`},
	{`extcos`, `\cos`},
	{`exttan`, `\tan`},
	{`extcot`, `\cot`},
	{`extsec`, `\sec`},
	{`extcsc`, `\csc`},
	{`extln`, `\ln`},
	{`extlog`, `\log`},
}

var latexCommands = []string{
	"varepsilon",
	"operatorname",
	"arccos",
	"arcsin",
	"arctan",
	"approx",
	"cdots",
	"delta",
	"Delta",
	"frac",
	"geq",
	"int",
	"lambda",
	"left",
	"leq",
	"lim",
	"log",
	"mu",
	"nabla",
	"neq",
	"omega",
	"Omega",
	"phi",
	"Phi",
	"pi",
	"prod",
	"rho",
	"right",
	"sigma",
	"Sigma",
	"sqrt",
	"theta",
	"Theta",
	"times",
	"alpha",
	"beta",
	"cdot",
	"cos",
	"cot",
	"csc",
	"div",
	"exp",
	"gamma",
	"Gamma",
	"ln",
	"sec",
	"sin",
	"sum",
	"tan",
}

// SanitizeMarkdown repairs common LLM-generated LaTeX issues before the text is
// saved or sent to the KaTeX frontend.
func SanitizeMarkdown(value string) string {
	if value == "" {
		return value
	}
	return norm.NFKC.String(rewriteMathSpans(value))
}

func SanitizeMarkdownSlice(values []string) []string {
	if values == nil {
		return nil
	}
	sanitized := make([]string, len(values))
	for index, value := range values {
		sanitized[index] = SanitizeMarkdown(value)
	}
	return sanitized
}

func rewriteMathSpans(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))

	for index := 0; index < len(value); {
		switch {
		case strings.HasPrefix(value[index:], "$$"):
			next := findClosingDelimiter(value, index+2, "$$")
			if next == -1 {
				builder.WriteString(value[index:])
				return builder.String()
			}
			builder.WriteString("$$")
			builder.WriteString(sanitizeMathContent(value[index+2 : next]))
			builder.WriteString("$$")
			index = next + 2
		case value[index] == '$':
			next := findClosingDollar(value, index+1)
			if next == -1 {
				builder.WriteByte(value[index])
				index++
				continue
			}
			builder.WriteByte('$')
			builder.WriteString(sanitizeMathContent(value[index+1 : next]))
			builder.WriteByte('$')
			index = next + 1
		case strings.HasPrefix(value[index:], `\(`):
			next := findClosingDelimiter(value, index+2, `\)`)
			if next == -1 {
				builder.WriteString(value[index:])
				return builder.String()
			}
			builder.WriteString(`\(`)
			builder.WriteString(sanitizeMathContent(value[index+2 : next]))
			builder.WriteString(`\)`)
			index = next + 2
		case strings.HasPrefix(value[index:], `\[`):
			next := findClosingDelimiter(value, index+2, `\]`)
			if next == -1 {
				builder.WriteString(value[index:])
				return builder.String()
			}
			builder.WriteString(`\[`)
			builder.WriteString(sanitizeMathContent(value[index+2 : next]))
			builder.WriteString(`\]`)
			index = next + 2
		default:
			r, size := utf8.DecodeRuneInString(value[index:])
			builder.WriteRune(r)
			index += size
		}
	}

	return builder.String()
}

func findClosingDelimiter(value string, start int, delimiter string) int {
	for index := start; index < len(value); index++ {
		if strings.HasPrefix(value[index:], delimiter) && !isEscaped(value, index) {
			return index
		}
	}
	return -1
}

func findClosingDollar(value string, start int) int {
	for index := start; index < len(value); index++ {
		if value[index] == '$' && !isEscaped(value, index) {
			return index
		}
	}
	return -1
}

func isEscaped(value string, index int) bool {
	backslashes := 0
	for cursor := index - 1; cursor >= 0 && value[cursor] == '\\'; cursor-- {
		backslashes++
	}
	return backslashes%2 == 1
}

func sanitizeMathContent(value string) string {
	value = controlEscapeReplacer.Replace(value)
	value = strings.TrimSpace(value)
	value = stripLeadingMathQuotes(value)
	value = stripTrailingMathQuotes(value)
	value = normalizeMathQuotes(value)
	value = norm.NFKC.String(value)
	value = replaceTextAliases(value)
	value = addMissingCommandSlashes(value)
	return value
}

func stripLeadingMathQuotes(value string) string {
	for value != "" {
		r, size := utf8.DecodeRuneInString(value)
		if !isLeadingMathQuote(r) {
			return value
		}
		value = strings.TrimLeftFunc(value[size:], unicode.IsSpace)
	}
	return value
}

func stripTrailingMathQuotes(value string) string {
	for value != "" {
		r, size := utf8.DecodeLastRuneInString(value)
		if !isTrailingMathQuote(r) {
			return value
		}
		value = strings.TrimRightFunc(value[:len(value)-size], unicode.IsSpace)
	}
	return value
}

func isLeadingMathQuote(r rune) bool {
	switch r {
	case '\'', '"', '`', '‘', '’', '“', '”':
		return true
	default:
		return false
	}
}

func isTrailingMathQuote(r rune) bool {
	switch r {
	case '"', '`', '‘', '’', '“', '”':
		return true
	default:
		return false
	}
}

func normalizeMathQuotes(value string) string {
	replacer := strings.NewReplacer(
		"‘", "'",
		"’", "'",
		"′", "'",
		"‵", "'",
		"“", `"`,
		"”", `"`,
		"″", `"`,
	)
	return replacer.Replace(value)
}

func replaceTextAliases(value string) string {
	for _, alias := range trigTextAliases {
		value = replaceAliasAtBoundary(value, alias.alias, alias.command)
	}
	return value
}

func replaceAliasAtBoundary(value string, alias string, command string) string {
	var builder strings.Builder
	builder.Grow(len(value))

	for index := 0; index < len(value); {
		if strings.HasPrefix(value[index:], alias) &&
			isCommandBoundaryBefore(value, index) &&
			isCommandBoundaryAfter(value, index+len(alias)) {
			builder.WriteString(command)
			index += len(alias)
			continue
		}
		r, size := utf8.DecodeRuneInString(value[index:])
		builder.WriteRune(r)
		index += size
	}

	return builder.String()
}

func addMissingCommandSlashes(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))

	for index := 0; index < len(value); {
		matched := ""
		for _, command := range latexCommands {
			if strings.HasPrefix(value[index:], command) &&
				isCommandBoundaryBefore(value, index) &&
				isCommandBoundaryAfter(value, index+len(command)) {
				matched = command
				break
			}
		}
		if matched != "" {
			builder.WriteByte('\\')
			builder.WriteString(matched)
			index += len(matched)
			continue
		}
		r, size := utf8.DecodeRuneInString(value[index:])
		builder.WriteRune(r)
		index += size
	}

	return builder.String()
}

func isCommandBoundaryBefore(value string, index int) bool {
	if index == 0 {
		return true
	}
	r, _ := utf8.DecodeLastRuneInString(value[:index])
	return r != '\\' && !isASCIIAlpha(r)
}

func isCommandBoundaryAfter(value string, index int) bool {
	if index >= len(value) {
		return true
	}
	r, _ := utf8.DecodeRuneInString(value[index:])
	return !isASCIIAlpha(r)
}

func isASCIIAlpha(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}
