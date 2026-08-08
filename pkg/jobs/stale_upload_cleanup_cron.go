package jobs

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/robfig/cron/v3"

	"arkive/core/services/uploads"
)

const staleUploadCleanupSchedule = "@every 10m"

func StartStaleUploadCleanup(uploadService *uploads.Service) (*cron.Cron, error) {
	logger := log.New(os.Stdout, "cron ", log.LstdFlags)
	cleanupCron := cron.New(
		cron.WithChain(cron.Recover(cron.DefaultLogger)),
		cron.WithLogger(cron.VerbosePrintfLogger(logger)),
	)
	_, err := cleanupCron.AddFunc(staleUploadCleanupSchedule, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		count, cleanupErr := uploadService.CleanupExpiredUploads(ctx)
		if cleanupErr != nil {
			log.Printf("stale upload cleanup failed: %v", cleanupErr)
			return
		}
		log.Printf("stale upload cleanup completed: %d uploads", count)
	})
	if err != nil {
		return nil, err
	}
	cleanupCron.Start()
	return cleanupCron, nil
}
