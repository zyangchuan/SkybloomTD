package controllers

import (
	"path/filepath"
	"regexp"
	"strings"
)

const (
	maxUploadBytes     = 50 << 20 // max upload size is 50MB
	maxGameNameLength  = 120
	gameNameField      = "game_name"
	gameNameMissingErr = "game_name is required"
)

var safePathPartPattern = regexp.MustCompile(`[^A-Za-z0-9_.=-]+`)

func safePathPart(value string) string {
	cleaned := safePathPartPattern.ReplaceAllString(strings.TrimSpace(value), "_")
	cleaned = strings.Trim(cleaned, "._")
	if cleaned == "" {
		return "unknown"
	}
	return cleaned
}

func safeFilename(value string) string {
	base := filepath.Base(value)
	if base == "." || base == "/" || strings.TrimSpace(base) == "" {
		base = "input.pdf"
	}
	ext := safeFilenameExt(filepath.Ext(base))
	if ext == "" {
		ext = ".pdf"
	}
	stem := safePathPart(strings.TrimSuffix(base, filepath.Ext(base)))
	if stem == "unknown" {
		stem = "input"
	}
	return stem + ext
}

func safeFilenameExt(value string) string {
	value = safePathPartPattern.ReplaceAllString(strings.TrimSpace(value), "_")
	if !strings.HasPrefix(value, ".") {
		value = "." + value
	}
	if value == "." || value == "._" {
		return ""
	}
	return value
}

func normalizeGameName(value string) string {
	return strings.Join(strings.Fields(value), " ")
}
