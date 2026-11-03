package router

import (
	"github.com/gin-gonic/gin"

	"arkive/core/config"
	"arkive/core/database"
	"arkive/core/handlers"
	"arkive/core/middleware"
	authrepo "arkive/core/repositories/auth"
	filerepo "arkive/core/repositories/files"
	sessionrepo "arkive/core/repositories/session"
	settingsrepo "arkive/core/repositories/settings"
	sharerepo "arkive/core/repositories/shares"
	syncrepo "arkive/core/repositories/sync"
	usersrepo "arkive/core/repositories/users"
	"arkive/core/services/auth"
	filessvc "arkive/core/services/files"
	folderssvc "arkive/core/services/folders"
	settingssvc "arkive/core/services/settings"
	"arkive/core/services/setup"
	"arkive/core/services/shares"
	syncsvc "arkive/core/services/sync"
	"arkive/core/services/uploads"
	"arkive/core/web"
	"arkive/pkg/storage/localclient"
)

func New(db database.PgPool, cfg config.Config, uploadService *uploads.Service, filesService *filessvc.Service, folderService *folderssvc.Service, localStorage *localclient.Client, corsMiddleware gin.HandlerFunc) *gin.Engine {
	r := gin.Default()
	r.Use(middleware.ErrorLogger())
	r.Use(middleware.SecurityHeaders())

	authRepo := authrepo.New()
	usersRepo := usersrepo.New()
	authService := auth.NewService(db, authRepo, sessionrepo.New(), usersRepo, auth.Config{
		SessionTTL:            cfg.SessionTTL,
		PasswordResetTokenTTL: cfg.PasswordResetTokenTTL,
		BaseURL:               cfg.BaseURL,
		OpaqueServerSetup:     cfg.OpaqueServerSetup,
	})
	settingsRepo := settingsrepo.New()
	settingsService := settingssvc.NewService(db, settingsRepo)
	setupService := setup.NewService(db, authRepo, usersRepo, settingsRepo, cfg.OpaqueServerSetup)
	shareService := shares.NewService(db, filerepo.New(), sharerepo.New())
	syncService := syncsvc.NewService(db, syncrepo.New())

	// Static assets or page: public
	r.StaticFS("/static", web.StaticFS("static"))
	r.StaticFS("/web/pages", web.StaticFS("pages"))
	r.GET("/sw.js", handlers.ServiceWorkerJS())
	r.GET("/favicon.ico", handlers.FaviconICO())
	r.GET("/robots.txt", handlers.RobotsTxt())
	r.GET("/sitemap.xml", handlers.SitemapXML())
	localStorageRoutes := r.Group("/local-storage")
	if corsMiddleware != nil {
		localStorageRoutes.Use(corsMiddleware)
		r.OPTIONS("/local-storage/*path", corsMiddleware)
	}
	localStorageRoutes.PUT("/upload/:token", handlers.LocalStorageUpload(localStorage))
	localStorageRoutes.GET("/download/:token", handlers.LocalStorageDownload(localStorage))
	r.GET("/s/:token", handlers.PublicShareView(shareService, filesService, settingsService, cfg.CookieSecret))
	r.POST("/s/:token", handlers.PublicShareUnlock(shareService, filesService, settingsService, cfg.CookieSecret))

	// Auth pages live in SvelteKit; the legacy Go forms submit plaintext passwords.
	// Protected
	protected := r.Group("/")
	protected.Use(middleware.RequireSessionRedirect(authService))
	protected.GET("/files", handlers.WebFiles(filesService, folderService))
	protected.GET("/folders/:id", handlers.WebFolder(filesService, folderService))
	protected.GET("/files/:id/view", handlers.WebFileView(filesService, settingsService))
	protected.GET("/shares", handlers.WebShares(shareService))
	protected.GET("/settings", handlers.WebSettings(settingsService))
	protected.POST("/settings/storage", handlers.WebSettingsStoragePost(settingsService))
	protected.POST("/settings/uploads", handlers.WebSettingsUploadPost(settingsService))
	protected.POST("/settings/previews", handlers.WebSettingsPreviewPost(settingsService))
	// Public api endpoints
	api := r.Group("/api")
	if corsMiddleware != nil {
		api.Use(corsMiddleware)
		r.OPTIONS("/api/*path", corsMiddleware)
	}
	publicAuth := api.Group("")
	// Bound auth and setup JSON before ShouldBindJSON reads it; uploads have their own larger limit.
	publicAuth.Use(middleware.LimitBody(16 << 10))
	{
		publicAuth.GET("/setup/status", handlers.APISetupStatus(setupService))
		publicAuth.POST("/setup/opaque/registration/start", handlers.APISetupOpaqueRegistrationStart(setupService))
		publicAuth.POST("/setup", handlers.APICreateInitialSetup(setupService))
		publicAuth.POST("/setup/recovery-key", handlers.APICompleteInitialSetup(setupService))
		publicAuth.POST("/auth/opaque/login/start", handlers.APIOpaqueLoginStart(authService))
		publicAuth.POST("/auth/opaque/login/finish", handlers.APIOpaqueLoginFinish(authService))
		publicAuth.POST("/auth/logout", handlers.APILogout(authService))
		publicAuth.POST("/auth/forgot-password", handlers.APIForgotPassword(authService))
		publicAuth.GET("/auth/password-reset/vault", handlers.APIPasswordResetVault(authService))
		publicAuth.POST("/auth/password-reset/opaque/start", handlers.APIStartPasswordResetRegistration(authService))
		publicAuth.POST("/auth/password-reset/complete", handlers.APICompletePasswordReset(authService))
	}
	{
		api.GET("/me", middleware.RequireSessionJSON(authService), handlers.APIMe(authService))
		api.GET("/health", handlers.Health(db))
		api.GET("/transfers/limits", handlers.APITransferLimits(settingsService))
		api.GET("/public/shares/:token", handlers.APIPublicShareRecord(shareService, filesService, cfg.CookieSecret))
		api.POST("/public/shares/:token/consume", handlers.APIPublicShareConsume(shareService, filesService, cfg.CookieSecret))
		api.POST("/search", middleware.RequireSessionJSON(authService), handlers.APISearch(filesService, folderService))
	}

	apiUploads := api.Group("/uploads")
	apiUploads.Use(middleware.RequireSessionJSON(authService))
	apiUploads.Use(middleware.LimitBody(2 << 20))
	apiUploads.Use(middleware.UploadTiming())
	{
		apiUploads.GET("/limits", handlers.APIUploadLimits(settingsService))
		apiUploads.POST("/start", handlers.APIUploadStart(uploadService))
		apiUploads.POST("/batch/start", handlers.APIUploadBatchStart(uploadService))
		apiUploads.POST("/batch/complete", handlers.APIUploadBatchComplete(uploadService))
		apiUploads.POST("/:id/parts", handlers.APIUploadPartRecord(uploadService))
		apiUploads.POST("/:id/parts/presign", handlers.APIUploadPartPresignBatch(uploadService))
		apiUploads.POST("/:id/parts/:part/presign", handlers.APIUploadPartPresign(uploadService))
		apiUploads.POST("/:id/presign", handlers.APIUploadSinglePresign(uploadService))
		apiUploads.POST("/:id/thumbnail/presign", handlers.APIThumbnailPresign(uploadService))
		apiUploads.POST("/:id/complete", handlers.APIUploadComplete(uploadService))
		apiUploads.POST("/:id/cancel", handlers.APIUploadCancel(uploadService))
	}

	apiFiles := api.Group("/files")
	apiFiles.Use(middleware.RequireSessionJSON(authService))
	{
		apiFiles.GET("/:id/record", handlers.APIFileRecord(filesService))
		apiFiles.GET("/:id/thumbnail/presign", handlers.APIThumbnailDownloadPresign(filesService))
		apiFiles.GET("/:id/share", handlers.APIGetShareForFile(shareService))
		apiFiles.POST("/:id/share", handlers.APICreateShare(shareService))
	}

	apiShares := api.Group("/shares")
	apiShares.Use(middleware.RequireSessionJSON(authService))
	{
		apiShares.PATCH("/:id", handlers.APIUpdateShare(shareService))
		apiShares.POST("/:id/revoke", handlers.APIRevokeShare(shareService))
		apiShares.POST("/:id/activate", handlers.APIActivateShare(shareService))
		apiShares.GET("/:id/crypto-record", handlers.APIGetShareCryptoRecord(shareService))
		apiShares.DELETE("/:id", handlers.APIDeleteShare(shareService))
	}

	apiFolders := api.Group("")
	apiFolders.Use(middleware.RequireSessionJSON(authService))
	{
		apiFolders.POST("/folders", handlers.APICreateFolder(folderService))
		apiFolders.GET("/folders/root/entries", handlers.APIListRootFolderEntries(folderService))
		apiFolders.GET("/folders/:id/entries", handlers.APIListFolderEntries(folderService))
		apiFolders.POST("/entries/delete", handlers.APIDeleteEntries(folderService))
		apiFolders.POST("/entries/move", handlers.APIMoveEntries(folderService))
		apiFolders.POST("/entries/rename", handlers.APIRenameEntry(folderService, filesService))
	}

	apiSync := api.Group("/sync")
	apiSync.Use(middleware.RequireSessionJSON(authService))
	{
		apiSync.GET("/entries", handlers.APIListSyncEntries(syncService))
	}

	r.NoRoute(handlers.WebNotFound())

	return r
}
