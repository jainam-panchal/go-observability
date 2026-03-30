package main

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jainam-panchal/go-observability/database"
	"github.com/jainam-panchal/go-observability/logger"
	"github.com/jainam-panchal/go-observability/telemetry"
	"github.com/jainam-panchal/go-observability/worker"
	"go.uber.org/zap"
)

func main() {
	cfg := telemetry.LoadConfigFromEnv()

	shutdown, err := telemetry.Init(cfg)
	if err != nil {
		panic(fmt.Errorf("init telemetry: %w", err))
	}
	defer func() {
		_ = shutdown(context.Background())
	}()

	appLogger, err := logger.New(cfg)
	if err != nil {
		panic(fmt.Errorf("init logger: %w", err))
	}
	zap.ReplaceGlobals(appLogger)

	db, err := database.OpenInstrumentedSQL("sqlite3", "file:worker-example?mode=memory&cache=shared")
	if err != nil {
		panic(fmt.Errorf("open instrumented sql db: %w", err))
	}
	defer func() {
		_ = db.Close()
	}()

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS jobs (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL)`); err != nil {
		panic(fmt.Errorf("create jobs table: %w", err))
	}

	if err := runJob(context.Background(), db, "thumbnail.render"); err != nil {
		logger.L(context.Background()).Fatal("job failed", zap.Error(err))
	}
}

func runJob(ctx context.Context, db *sql.DB, jobName string) (err error) {
	ctx, finish := worker.StartJob(ctx, jobName)
	defer func() {
		finish(err)
	}()

	logger.L(ctx).Info("starting job", zap.String("job_name", jobName))

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `INSERT INTO jobs (name) VALUES (?)`, jobName); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("insert job row: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	logger.L(ctx).Info("finished job", zap.String("job_name", jobName))
	return nil
}
