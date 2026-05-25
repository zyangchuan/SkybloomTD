package models

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	TaskStatusQueued     = "queued"
	TaskStatusProcessing = "processing"
	TaskStatusSuccessful = "successful"
	TaskStatusFailed     = "failed"
)

var ErrTaskStatusNotFound = errors.New("task status not found")
var ErrDocumentNotFound = errors.New("document not found")
var ErrChapterNotFound = errors.New("chapter not found")

type SourceRef struct {
	Type        string `json:"type"`
	Bucket      string `json:"bucket"`
	Key         string `json:"key"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type,omitempty"`
}

type DocumentJob struct {
	JobType      string `json:"job_type"`
	TaskID       string `json:"task_id"`
	OCRTaskID    string `json:"ocr_task_id"`
	UploadTaskID string `json:"upload_task_id"`
	IndexTaskID  string `json:"index_task_id"`
	UserID       string `json:"user_id"`
	DocumentID   string `json:"document_id"`
	Filename     string `json:"filename"`
	Source       any    `json:"source"`
}

type Document struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey" json:"document_id"`
	UserID            uuid.UUID `gorm:"type:uuid;index" json:"user_id"`
	S3Bucket          *string   `gorm:"type:text" json:"s3_bucket,omitempty"`
	S3Key             *string   `gorm:"type:text" json:"s3_key,omitempty"`
	Filename          string    `gorm:"type:text" json:"filename"`
	GameName          string    `gorm:"type:text;not null;default:'Untitled Game'" json:"game_name"`
	TaskID            string    `gorm:"type:text;index" json:"task_id"`
	IsReady           bool      `gorm:"not null;default:false" json:"is_ready"`
	SourceType        string    `gorm:"type:text" json:"source_type"`
	SourceBucket      *string   `gorm:"type:text" json:"source_bucket,omitempty"`
	SourceKey         *string   `gorm:"type:text" json:"source_key,omitempty"`
	SourcePath        *string   `gorm:"type:text" json:"source_path,omitempty"`
	SourceContentType *string   `gorm:"type:text" json:"source_content_type,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type DocumentSummary struct {
	DocumentID uuid.UUID `json:"document_id"`
	Filename   string    `json:"filename"`
	GameName   string    `json:"game_name"`
	IsReady    bool      `json:"is_ready"`
	TaskID     string    `json:"task_id"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type ListDocumentsResponse struct {
	Documents []DocumentSummary `json:"documents"`
}

type ChapterSummary struct {
	ChapterID    uuid.UUID `json:"chapter_id"`
	DocumentID   uuid.UUID `json:"document_id"`
	ChapterIndex *int      `json:"chapter_index"`
	Title        *string   `json:"title"`
	StartLine    *int      `json:"start_line"`
	EndLine      *int      `json:"end_line"`
	CreatedAt    time.Time `json:"created_at"`
}

type ListChaptersResponse struct {
	Chapters []ChapterSummary `json:"chapters"`
}

type SubChapterSummary struct {
	SubChapterID    uuid.UUID `json:"sub_chapter_id"`
	DocumentID      uuid.UUID `json:"document_id"`
	ChapterID       uuid.UUID `json:"chapter_id"`
	SubChapterIndex *int      `json:"sub_chapter_index"`
	Title           *string   `json:"title"`
	StartLine       *int      `json:"start_line"`
	EndLine         *int      `json:"end_line"`
	CreatedAt       time.Time `json:"created_at"`
}

type ListSubChaptersResponse struct {
	SubChapters []SubChapterSummary `json:"sub_chapters"`
}

type TaskStatus struct {
	TaskID     string    `json:"task_id"`
	DocumentID string    `json:"document_id"`
	Status     string    `json:"status"`
	Error      *string   `json:"error"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func NewQueuedDocument(documentID string, userID string, taskID string, filename string, gameName string, source any) (Document, error) {
	parsedDocumentID, err := uuid.Parse(documentID)
	if err != nil {
		return Document{}, fmt.Errorf("parse document_id: %w", err)
	}

	document := Document{
		ID:       parsedDocumentID,
		UserID:   DatabaseUUID(userID, "user"),
		Filename: filename,
		GameName: gameName,
		TaskID:   taskID,
		IsReady:  false,
	}

	switch value := source.(type) {
	case SourceRef:
		document.SourceType = value.Type
		document.SourceBucket = stringPointer(value.Bucket)
		document.SourceKey = stringPointer(value.Key)
		document.SourceContentType = stringPointer(value.ContentType)
	case string:
		document.SourceType = "local"
		document.SourcePath = stringPointer(value)
	default:
		return Document{}, fmt.Errorf("unsupported document source type %T", source)
	}

	return document, nil
}

func NewTaskStatus(taskID string, documentID string, status string, errorMessage *string) TaskStatus {
	return TaskStatus{
		TaskID:     taskID,
		DocumentID: documentID,
		Status:     status,
		Error:      errorMessage,
		UpdatedAt:  time.Now().UTC(),
	}
}

func NewDocumentSummary(document Document) DocumentSummary {
	return DocumentSummary{
		DocumentID: document.ID,
		Filename:   document.Filename,
		GameName:   document.GameName,
		IsReady:    document.IsReady,
		TaskID:     document.TaskID,
		CreatedAt:  document.CreatedAt,
		UpdatedAt:  document.UpdatedAt,
	}
}

func DatabaseUUID(value string, namespace string) uuid.UUID {
	if parsed, err := uuid.Parse(value); err == nil {
		return parsed
	}
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(fmt.Sprintf("ocr:%s:%s", namespace, value)))
}

func stringPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
