package pages

import (
	"strconv"
	"strings"
	"time"

	lucide "github.com/eduardolat/gomponents-lucide"
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"

	"arkive/core/models"
	"arkive/core/web"
	"arkive/core/web/components"
	"arkive/pkg/format"
	"arkive/pkg/validation"
)

type SettingsPageProps struct {
	Ctx             PageContext
	StorageSettings models.StorageSettings
	UploadSettings  models.UploadSettings
	PreviewSettings models.PreviewSettings
	StorageGB       string
	Errors          validation.Errors
	Message         string
	ActiveTab       string
}

func SettingsPage(props SettingsPageProps) web.Page {
	user := props.Ctx.User
	brandName := ""
	email := ""
	memberSince := "Unavailable"
	lastLogin := "Unavailable"
	usedStorage := "0 B"
	quotaStorage := "Unlimited"
	usagePercent := 0
	storageSettings := props.StorageSettings
	uploadSettings := props.UploadSettings
	previewSettings := props.PreviewSettings

	if storageSettings.Provider == "" {
		storageSettings.Provider = "local"
	}
	if uploadSettings.MaxQueueItems == 0 {
		uploadSettings.MaxQueueItems = 300
	}
	if uploadSettings.PartConcurrency == 0 {
		uploadSettings.PartConcurrency = 3
	}
	if uploadSettings.StaleUploadHours == 0 {
		uploadSettings.StaleUploadHours = 1
	}
	if previewSettings.ImageMaxBytes == 0 {
		previewSettings.ImageMaxBytes = 50 * 1024 * 1024
	}
	if previewSettings.VideoMaxBytes == 0 {
		previewSettings.VideoMaxBytes = 128 * 1024 * 1024
	}
	if previewSettings.TextMaxBytes == 0 {
		previewSettings.TextMaxBytes = 2 * 1024 * 1024
	}

	storageProviderLabel := strings.ToUpper(storageSettings.Provider)
	storageGB := props.StorageGB
	if storageGB == "" {
		storageGB = settingsStorageGB(storageSettings.MaxStorageBytes)
	}

	if user != nil {
		brandName = strings.TrimSpace(user.BrandName)
		email = strings.TrimSpace(user.Email)
		if !user.CreatedAt.IsZero() {
			memberSince = user.CreatedAt.Format("Jan 2, 2006")
		}
		if user.LastLoginAt != nil {
			lastLogin = user.LastLoginAt.Format(time.RFC1123)
		}
		usedStorage = format.Bytes(user.UsedBytes)
		if storageSettings.MaxStorageBytes > 0 {
			quotaStorage = format.Bytes(storageSettings.MaxStorageBytes)
			if user.UsedBytes > 0 {
				usagePercent = int((float64(user.UsedBytes) / float64(storageSettings.MaxStorageBytes)) * 100)
				if usagePercent > 100 {
					usagePercent = 100
				}
			}
		}
	}

	activeTab := props.ActiveTab
	if activeTab == "" {
		if uploadFormHasErrors(props.Errors) {
			activeTab = "settings-upload"
		} else if previewFormHasErrors(props.Errors) {
			activeTab = "settings-preview"
		} else if storageFormHasErrors(props.Errors) {
			activeTab = "settings-provider"
		} else {
			activeTab = "settings-account"
		}
	}

	contentClasses := "settings-content"
	if activeTab != "" {
		contentClasses += " active-tab-" + activeTab
	}

	return web.Page{
		Title:              "Arkive · Settings",
		Robots:             RobotsNoIndex,
		CSS:                []string{"/web/pages/settings.css"},
		AuthLayout:         true,
		RequireVaultUnlock: true,
		User:               user,
		ActiveNav:          "settings",
		Body: h.Main(
			h.Class("settings-page"),
			h.Div(
				h.Class("container"),
				h.Div(
					h.Class("page-header settings-header"),
					h.Div(
						h.Class("page-title"),
						h.Div(
							h.Class("settings-badge-row"),
							h.Span(
								h.Class("settings-node-badge"),
								h.Span(h.Class("node-badge-dot")),
								g.Text("Core Single-Tenant Node"),
							),
							h.Span(h.Class("settings-encryption-chip"), g.Text("Zero-Knowledge Enforced")),
						),
						h.H1(g.Text("Settings")),
						h.P(g.Text("Manage node identity, storage backend engines, upload concurrency, and media viewer limits.")),
					),
				),
				g.If(props.Message != "", settingsNotification(props.Message)),
				h.Div(
					h.Class("settings-grid"),
					h.Aside(
						h.Class("settings-tabs"),
						settingsTabLink("settings-account", "Instance", lucide.Server),
						settingsTabLink("settings-provider", "Storage Provider", lucide.HardDrive),
						settingsTabLink("settings-upload", "Uploads", lucide.CloudUpload),
						settingsTabLink("settings-preview", "Previews", lucide.Eye),
						settingsTabLink("settings-security", "Security", lucide.ShieldCheck),
					),
					h.Div(
						h.Class(contentClasses),
						// Tab 1: Instance
						h.Section(
							h.Class("settings-panel settings-panel-default"+panelActiveClass("settings-account", activeTab)),
							g.Attr("id", "settings-account"),
							h.Div(
								h.Class("settings-panel-header"),
								h.Div(
									h.Class("settings-panel-title"),
									h.Span(h.Class("panel-overline"), g.Text("CORE NODE · IDENTITY & QUOTA")),
									h.H2(g.Text("Instance Overview")),
									h.P(g.Text("Administrator identity, deployment details, and encrypted vault storage quota.")),
								),
							),
							h.Div(
								h.Class("settings-stack"),
								components.Card(components.CardProps{
									Title:    "Admin Profile",
									Subtitle: "Primary instance administrator credentials and activity.",
									Class:    "settings-card",
									Body: []g.Node{
										settingsAdminCard(user, brandName, email, memberSince, lastLogin),
									},
								}),
								components.Card(components.CardProps{
									Title:    "Storage Capacity",
									Subtitle: "Total storage usage across encrypted vaults against configured limit.",
									Class:    "settings-card",
									Body: []g.Node{
										settingsCapacityCard(usedStorage, quotaStorage, usagePercent, storageSettings.MaxStorageBytes, storageProviderLabel),
									},
								}),
							),
						),
						// Tab 2: Storage Provider
						h.Section(
							h.Class("settings-panel"+panelActiveClass("settings-provider", activeTab)),
							g.Attr("id", "settings-provider"),
							h.Div(
								h.Class("settings-panel-header"),
								h.Div(
									h.Class("settings-panel-title"),
									h.Span(h.Class("panel-overline"), g.Text("PERSISTENCE BACKEND · ENCRYPTED BLOBS")),
									h.H2(g.Text("Storage Provider")),
									h.P(g.Text("Configure storage location for encrypted blobs. Supports local host filesystem and any S3-compatible cloud object store.")),
								),
							),
							h.Div(
								h.Class("settings-stack"),
								components.Card(components.CardProps{
									Title:    "Active Engine Status",
									Subtitle: "Current persistence backend and cryptographic security status.",
									Class:    "settings-card",
									Body: []g.Node{
										storageProviderStatusCard(storageSettings),
									},
								}),
								components.Card(components.CardProps{
									Title:    "Backend Configuration",
									Subtitle: "Select storage provider and configure connection credentials.",
									Class:    "settings-card",
									Body: []g.Node{
										storageSettingsForm(storageSettings, storageGB, props.Errors),
									},
								}),
							),
						),
						// Tab 3: Uploads
						h.Section(
							h.Class("settings-panel"+panelActiveClass("settings-upload", activeTab)),
							g.Attr("id", "settings-upload"),
							h.Div(
								h.Class("settings-panel-header"),
								h.Div(
									h.Class("settings-panel-title"),
									h.Span(h.Class("panel-overline"), g.Text("CLIENT CONCURRENCY · WORKER PIPELINE")),
									h.H2(g.Text("Uploads")),
									h.P(g.Text("Configure upload queue behavior and concurrency limits for this instance.")),
								),
							),
							h.Div(
								h.Class("settings-stack"),
								h.Div(
									h.Class("settings-stats-grid"),
									settingsStatCard(
										lucide.Layers,
										strconv.Itoa(uploadSettings.MaxQueueItems),
										"items",
										"Queue Depth",
										"Maximum files queued concurrently in the upload tray",
										"Staging Limit",
									),
									settingsStatCard(
										lucide.Zap,
										strconv.Itoa(uploadSettings.PartConcurrency),
										"threads",
										"Chunk Concurrency",
										"Simultaneous chunk transfers per file via Web Workers",
										"Parallelism",
									),
									settingsStatCard(
										lucide.Timer,
										strconv.Itoa(uploadSettings.StaleUploadHours),
										"hour(s)",
										"Session Window",
										"Validity duration for unfinished multipart uploads",
										"Cleanup TTL",
									),
								),
								h.Details(
									h.Class("settings-advanced"),
									g.If(uploadFormHasErrors(props.Errors), g.Attr("open", "open")),
									h.Summary(
										g.Text("Advanced — edit upload queue limits"),
									),
									components.Card(components.CardProps{
										Title:    "Configuration",
										Subtitle: "Update upload queue settings for this Core instance.",
										Class:    "settings-card",
										Body: []g.Node{
											uploadSettingsForm(uploadSettings, props.Errors),
										},
									}),
								),
							),
						),
						// Tab 4: Previews
						h.Section(
							h.Class("settings-panel"+panelActiveClass("settings-preview", activeTab)),
							g.Attr("id", "settings-preview"),
							h.Div(
								h.Class("settings-panel-header"),
								h.Div(
									h.Class("settings-panel-title"),
									h.Span(h.Class("panel-overline"), g.Text("CLIENT DECRYPTION · MEDIA VIEWER")),
									h.H2(g.Text("Previews")),
									h.P(g.Text("Configure browser preview limits for image, video, and text files.")),
								),
							),
							h.Div(
								h.Class("settings-stack"),
								h.Div(
									h.Class("settings-media-grid"),
									settingsMediaCategoryCard(
										lucide.Image,
										strconv.FormatInt(previewSettings.ImageMaxBytes/(1024*1024), 10),
										"MB",
										"Image Previews",
										[]string{"PNG", "JPEG", "WEBP", "GIF", "SVG"},
										"Decrypted directly in browser memory for instant lightbox rendering.",
									),
									settingsMediaCategoryCard(
										lucide.Film,
										strconv.FormatInt(previewSettings.VideoMaxBytes/(1024*1024), 10),
										"MB",
										"Video Fallback",
										[]string{"MP4", "WEBM", "Streaming"},
										"Fallback cap for non-streaming players. Native range streaming is uncapped.",
									),
									settingsMediaCategoryCard(
										lucide.FileText,
										strconv.FormatInt(previewSettings.TextMaxBytes/(1024*1024), 10),
										"MB",
										"Text & Code Viewer",
										[]string{"TXT", "MD", "JSON", "Code"},
										"High-performance syntax viewer cap to prevent browser thread locking.",
									),
								),
								h.Details(
									h.Class("settings-advanced"),
									g.If(previewFormHasErrors(props.Errors), g.Attr("open", "open")),
									h.Summary(
										g.Text("Advanced — edit preview limits"),
									),
									components.Card(components.CardProps{
										Title:    "Configuration",
										Subtitle: "Update preview-size limits without changing encrypted file formats. Video limit applies only when browser streaming preview falls back to full-file playback.",
										Class:    "settings-card",
										Body: []g.Node{
											previewSettingsForm(previewSettings, props.Errors),
										},
									}),
								),
							),
						),
						// Tab 5: Security
						h.Section(
							h.Class("settings-panel"+panelActiveClass("settings-security", activeTab)),
							g.Attr("id", "settings-security"),
							h.Div(
								h.Class("settings-panel-header"),
								h.Div(
									h.Class("settings-panel-title"),
									h.Span(h.Class("panel-overline"), g.Text("CRYPTOGRAPHY · ACCESS HARDENING")),
									h.H2(g.Text("Security Controls")),
									h.P(g.Text("Core security posture, master vault credentials, and instance protection.")),
								),
							),
							components.Card(components.CardProps{
								Title:    "Upcoming Hardening Controls",
								Subtitle: "Planned self-host security features in upcoming Core milestones.",
								Class:    "settings-card",
								Body: []g.Node{
									h.Ul(
										h.Class("settings-list"),
										h.Li(g.Text("Master password rotation and vault re-wrapping")),
										h.Li(g.Text("Session idle-timeout and biometric WebAuthn unlock")),
										h.Li(g.Text("Immutable audit trail for file creation and share access")),
									),
								},
							}),
						),
					),
				),
			),
		),
	}
}

func panelActiveClass(panelId, activeTab string) string {
	if activeTab == panelId {
		return " settings-panel-active"
	}
	return ""
}

func uploadFormHasErrors(errors validation.Errors) bool {
	return validation.FieldError(errors, "max_queue_items") != "" ||
		validation.FieldError(errors, "part_concurrency") != "" ||
		validation.FieldError(errors, "stale_upload_hours") != ""
}

func previewFormHasErrors(errors validation.Errors) bool {
	return validation.FieldError(errors, "image_max_mb") != "" ||
		validation.FieldError(errors, "video_max_mb") != "" ||
		validation.FieldError(errors, "text_max_mb") != ""
}

func storageFormHasErrors(errors validation.Errors) bool {
	return validation.FieldError(errors, "storage_gb") != "" ||
		validation.FieldError(errors, "local_path") != "" ||
		validation.FieldError(errors, "s3_access_key_id") != "" ||
		validation.FieldError(errors, "s3_secret_access_key") != "" ||
		validation.FieldError(errors, "s3_bucket") != "" ||
		validation.FieldError(errors, "s3_endpoint") != ""
}
