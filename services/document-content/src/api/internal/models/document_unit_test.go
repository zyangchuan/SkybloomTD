package models

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestDatabaseUUIDDeterministic(t *testing.T) {
	first := DatabaseUUID("user-1", "user")
	second := DatabaseUUID("user-1", "user")
	differentValue := DatabaseUUID("user-2", "user")
	differentNamespace := DatabaseUUID("user-1", "document")

	if first != second {
		t.Fatalf("expected stable UUID for same input, got %s and %s", first, second)
	}
	if first == differentValue {
		t.Fatalf("expected different UUID for different value")
	}
	if first == differentNamespace {
		t.Fatalf("expected different UUID for different namespace")
	}
}

func TestDatabaseUUIDPreservesUUIDInput(t *testing.T) {
	id := uuid.New()

	got := DatabaseUUID(id.String(), "user")

	if got != id {
		t.Fatalf("expected parsed UUID %s, got %s", id, got)
	}
}

func TestS3DirectoryPath(t *testing.T) {
	userID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	documentID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	got := S3DirectoryPath(userID, documentID)
	want := "11111111-1111-1111-1111-111111111111/22222222-2222-2222-2222-222222222222"

	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestNewDocumentSummaryMapsDocumentFields(t *testing.T) {
	createdAt := time.Date(2026, 7, 10, 1, 2, 3, 0, time.UTC)
	updatedAt := createdAt.Add(time.Hour)
	document := Document{
		ID:             uuid.New(),
		SourceFilename: "rulebook.pdf",
		GameName:       "SkybloomTD",
		IsReady:        true,
		TaskID:         "task-1",
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
	}

	got := NewDocumentSummary(document)

	if got.DocumentID != document.ID ||
		got.SourceFilename != document.SourceFilename ||
		got.GameName != document.GameName ||
		got.IsReady != document.IsReady ||
		got.IsPublic != document.IsPublic ||
		got.TaskID != document.TaskID ||
		!got.CreatedAt.Equal(createdAt) ||
		!got.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("summary did not preserve document fields: %#v", got)
	}
}

func TestTaskStatusSerialization(t *testing.T) {
	errorMessage := "processing failed"
	status := TaskStatus{
		TaskID:     "task-1",
		DocumentID: "document-1",
		Status:     TaskStatusFailed,
		Error:      &errorMessage,
		UpdatedAt:  time.Date(2026, 7, 10, 1, 2, 3, 0, time.UTC),
	}

	body, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("marshal task status: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal task status: %v", err)
	}

	if got["task_id"] != status.TaskID ||
		got["document_id"] != status.DocumentID ||
		got["status"] != status.Status ||
		got["error"] != errorMessage {
		t.Fatalf("unexpected task status JSON: %s", string(body))
	}
}

func TestNewQueuedDocumentBuildsUploadDocument(t *testing.T) {
	documentID := uuid.NewString()
	userID := uuid.NewString()
	source := SourceRef{S3Bucket: "documents", SourceFilename: "rulebook.pdf"}

	document, err := NewQueuedDocument(documentID, userID, "task-1", "SkybloomTD", source)
	if err != nil {
		t.Fatalf("NewQueuedDocument failed: %v", err)
	}

	if document.ID.String() != documentID ||
		document.UserID.String() != userID ||
		document.S3Bucket == nil ||
		*document.S3Bucket != source.S3Bucket ||
		document.SourceFilename != source.SourceFilename ||
		document.GameName != "SkybloomTD" ||
		document.TaskID != "task-1" ||
		document.IsReady ||
		!document.IsPublic {
		t.Fatalf("unexpected queued document: %#v", document)
	}
}
