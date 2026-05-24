package database

import (
	"context"
	"database/sql"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func Open(ctx context.Context, databaseURL string) (*sql.DB, func() error, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, nil, err
	}
	return db, db.Close, nil
}

func Migrate(ctx context.Context, db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS levels (
			id UUID PRIMARY KEY,
			user_id UUID,
			document_id UUID NOT NULL,
			chapter_id UUID NOT NULL,
			sub_chapter_id UUID NOT NULL,
			summary_markdown TEXT NOT NULL,
			source_chunk_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
			source_metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
			model TEXT,
			created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS quizzes (
			id UUID PRIMARY KEY,
			level_id UUID NOT NULL REFERENCES levels(id) ON DELETE CASCADE,
			quiz_index INTEGER NOT NULL,
			quiz_type TEXT NOT NULL
				CONSTRAINT quizzes_quiz_type_check
				CHECK (quiz_type IN ('mcq', 'true_false')),
			question_markdown TEXT NOT NULL,
			options_markdown JSONB NOT NULL,
			answer_index INTEGER NOT NULL,
			created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT quizzes_level_id_quiz_index_key UNIQUE (level_id, quiz_index)
		)`,
		`ALTER TABLE quizzes ADD COLUMN IF NOT EXISTS answer_index INTEGER`,
		`DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint
				WHERE conname = 'quizzes_answer_index_nonnegative_check'
			) THEN
				ALTER TABLE quizzes
				ADD CONSTRAINT quizzes_answer_index_nonnegative_check
				CHECK (answer_index >= 0);
			END IF;
		}
		$$`,
		`CREATE INDEX IF NOT EXISTS levels_user_id_idx ON levels(user_id)`,
		`CREATE INDEX IF NOT EXISTS levels_sub_chapter_id_idx ON levels(sub_chapter_id)`,
		`CREATE INDEX IF NOT EXISTS levels_document_id_idx ON levels(document_id)`,
		`CREATE INDEX IF NOT EXISTS quizzes_level_id_idx ON quizzes(level_id)`,
		`DO $$
		DECLARE
			constraint_name text;
		BEGIN
			FOR constraint_name IN
				SELECT conname
				FROM pg_constraint
				WHERE conrelid = 'levels'::regclass
				  AND contype = 'f'
			LOOP
				EXECUTE format('ALTER TABLE levels DROP CONSTRAINT IF EXISTS %I', constraint_name);
			END LOOP;
		END
		$$`,
		`UPDATE quizzes SET answer_index = 0 WHERE answer_index IS NULL`,
		`ALTER TABLE quizzes ALTER COLUMN answer_index SET NOT NULL`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}
