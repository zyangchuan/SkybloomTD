package source

import (
	"context"
	"encoding/json"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/encoding"
)

type jsonCodec struct{}

func (jsonCodec) Name() string { return "json" }

func (jsonCodec) Marshal(value any) ([]byte, error) {
	return json.Marshal(value)
}

func (jsonCodec) Unmarshal(data []byte, value any) error {
	return json.Unmarshal(data, value)
}

type Client struct {
	addr     string
	timeout  time.Duration
	maxChars int32
}

type SourceContext struct {
	Status              string
	UserID              string
	DBUserID            string
	SubChapterID        string
	DocumentID          string
	ChapterID           string
	SubChapterIndex     int32
	SubChapterTitle     string
	StartLine           int32
	EndLine             int32
	ChunkIDs            []string
	ChunkCount          int32
	CandidateChunkCount int32
	ChunkLookupStrategy string
	MarkdownCacheHit    bool
	MarkdownCacheKey    string
	SourceCharCount     int32
	SourceTruncated     bool
	SourceContentHash   string
	SourceText          string
}

type getSubChapterRequest struct {
	UserID       string `json:"user_id"`
	SubChapterID string `json:"sub_chapter_id"`
	MaxChars     int32  `json:"max_chars"`
}

type getSubChapterResponse struct {
	SubChapter subChapterContent `json:"sub_chapter"`
}

type subChapterContent struct {
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

func NewClient(addr string, timeout time.Duration, maxChars int32) *Client {
	return &Client{addr: addr, timeout: timeout, maxChars: maxChars}
}

func RegisterJSONCodec() {
	encoding.RegisterCodec(jsonCodec{})
}

func (c *Client) FetchSubChapterContent(ctx context.Context, userID string, subChapterID string) (SourceContext, error) {
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	conn, err := grpc.NewClient(
		c.addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.ForceCodec(jsonCodec{}),
			grpc.CallContentSubtype("json"),
		),
	)
	if err != nil {
		return SourceContext{}, err
	}
	defer conn.Close()

	request := &getSubChapterRequest{
		UserID:       userID,
		SubChapterID: subChapterID,
		MaxChars:     c.maxChars,
	}
	var response getSubChapterResponse
	if err := conn.Invoke(callCtx, "/document_content.v1.DocumentContentService/GetSubChapter", request, &response); err != nil {
		return SourceContext{}, err
	}

	subChapter := response.SubChapter
	return SourceContext{
		Status:              "retrieved",
		UserID:              userID,
		DBUserID:            subChapter.NormalizedUserID,
		SubChapterID:        subChapter.SubChapterID,
		DocumentID:          subChapter.DocumentID,
		ChapterID:           subChapter.ChapterID,
		SubChapterIndex:     subChapter.SubChapterIndex,
		SubChapterTitle:     subChapter.Title,
		StartLine:           subChapter.StartLine,
		EndLine:             subChapter.EndLine,
		ChunkIDs:            subChapter.SourceChunkIDs,
		ChunkCount:          subChapter.ChunkCount,
		CandidateChunkCount: subChapter.CandidateChunkCount,
		ChunkLookupStrategy: subChapter.ChunkLookupStrategy,
		MarkdownCacheHit:    subChapter.MarkdownCacheHit,
		MarkdownCacheKey:    subChapter.MarkdownCacheKey,
		SourceCharCount:     subChapter.SourceCharCount,
		SourceTruncated:     subChapter.SourceTruncated,
		SourceContentHash:   subChapter.SourceContentHash,
		SourceText:          subChapter.SourceText,
	}, nil
}
