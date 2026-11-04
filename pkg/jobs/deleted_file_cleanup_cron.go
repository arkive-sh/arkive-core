package jobs

import (
	"context"
	"log"
	"os"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	filessvc "arkive/core/services/files"
	folderssvc "arkive/core/services/folders"
)

const deletedFileCleanupSchedule = "@every 5m"

func StartDeletedFileCleanup(filesService *filessvc.Service, folderService *folderssvc.Service) (*cron.Cron, error) {
	logger := log.New(os.Stdout, "cron ", log.LstdFlags)
	cleanupCron := cron.New(
		cron.WithChain(cron.Recover(cron.DefaultLogger)),
		cron.WithLogger(cron.VerbosePrintfLogger(logger)),
	)
	var runMu sync.Mutex
	runCleanup := func() {
		if !runMu.TryLock() {
			log.Printf("trash cleanup skipped: previous run is still active")
			return
		}
		defer runMu.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		filesPurged, cleanupErr := filesService.PurgeDeletedFiles(ctx)
		if cleanupErr != nil {
			log.Printf("deleted file cleanup failed: %v", cleanupErr)
			return
		}
		foldersPurged, cleanupErr := folderService.PurgeExpiredTrashFolders(ctx)
		if cleanupErr != nil {
			log.Printf("deleted folder cleanup failed: %v", cleanupErr)
			return
		}
		log.Printf("trash cleanup completed: %d purged files, %d purged folders", filesPurged, foldersPurged)
	}
	_, err := cleanupCron.AddFunc(deletedFileCleanupSchedule, runCleanup)
	if err != nil {
		return nil, err
	}
	cleanupCron.Start()
	go runCleanup()
	return cleanupCron, nil
}
