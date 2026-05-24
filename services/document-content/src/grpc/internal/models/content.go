package models

type GetSubChapterRequest struct {
	UserID       string `json:"user_id"`
	SubChapterID string `json:"sub_chapter_id"`
	MaxChars     int32  `json:"max_chars"`
}

type GetSubChapterResponse struct {
	SubChapter SubChapterContent `json:"sub_chapter"`
}

type SubChapterContent struct {
	NormalizedUserID    string   `json:"normalized_user_id"`
	RequestedUserID     string   `json:"requested_user_id"`
	SubChapterID        string   `json:"sub_chapter_id"`
	DocumentID          string   `json:"document_id"`
	ChapterID           string   `json:"chapter_id"`
	SubChapterIndex     int32    `json:"sub_chapter_index"`
	Title               string   `json:"title"`
	StartLine           int32    `json:"start_line"`
	EndLine             int32    `json:"end_line"`
	SourceText          string   `json:"source_text"`
	SourceChunkIDs      []string `json:"source_chunk_ids"`
	ChunkCount          int32    `json:"chunk_count"`
	CandidateChunkCount int32    `json:"candidate_chunk_count"`
	ChunkLookupStrategy string   `json:"chunk_lookup_strategy"`
	SourceCharCount     int32    `json:"source_char_count"`
	SourceTruncated     bool     `json:"source_truncated"`
	MarkdownCacheHit    bool     `json:"markdown_cache_hit"`
	MarkdownCacheKey    string   `json:"markdown_cache_key"`
	SourceContentHash   string   `json:"source_content_hash"`
}
