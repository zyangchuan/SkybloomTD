package models

type LevelJob struct {
	TaskID       string `json:"task_id"`
	FetchTaskID  string `json:"fetch_task_id"`
	GenerateID   string `json:"generate_task_id"`
	UserID       string `json:"user_id"`
	SubChapterID string `json:"sub_chapter_id"`
}
