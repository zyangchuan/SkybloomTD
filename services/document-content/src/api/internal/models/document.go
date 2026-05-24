package models

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
