package pages

import (
	"math"
	"strconv"

	lucide "github.com/eduardolat/gomponents-lucide"
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"

	"arkive/core/models"
	"arkive/core/web/components"
	"arkive/pkg/validation"
)

func storageSettingsForm(settings models.StorageSettings, storageGB string, errors validation.Errors) g.Node {
	isLocal := settings.Provider != "s3"
	generalError := validation.FieldError(errors, validation.GeneralKey)

	return h.Form(
		h.Class("settings-form"),
		g.Attr("method", "POST"),
		g.Attr("action", "/settings/storage"),
		g.If(
			generalError != "",
			h.Div(
				h.Class("form-error settings-form-error"),
				lucide.CircleAlert(h.Class("error-lucide"), g.Attr("aria-hidden", "true")),
				h.Span(g.Text(generalError)),
			),
		),
		h.Div(
			h.Class("form-section-head"),
			h.H4(g.Text("Select Storage Backend")),
			h.P(g.Text("Choose where encrypted zero-knowledge ciphertext blobs are stored.")),
		),
		h.Div(
			h.Class("provider-selector-grid"),
			h.Label(
				h.Class("provider-card-option"),
				h.Input(
					g.Attr("type", "radio"),
					g.Attr("name", "storage_provider"),
					g.Attr("value", "local"),
					g.If(isLocal, g.Attr("checked", "checked")),
				),
				h.Div(
					h.Class("provider-card-inner"),
					h.Div(
						h.Class("provider-card-top"),
						h.Span(
							h.Class("provider-card-icon-wrap provider-icon-local"),
							lucide.HardDrive(h.Class("provider-card-lucide"), g.Attr("aria-hidden", "true")),
						),
						h.Span(h.Class("provider-card-badge"), g.Text("Default")),
					),
					h.Strong(h.Class("provider-card-title"), g.Text("Local Disk")),
					h.P(h.Class("provider-card-desc"), g.Text("Persist encrypted chunks directly to server filesystem. Zero cloud network latency.")),
				),
			),
			h.Label(
				h.Class("provider-card-option"),
				h.Input(
					g.Attr("type", "radio"),
					g.Attr("name", "storage_provider"),
					g.Attr("value", "s3"),
					g.If(!isLocal, g.Attr("checked", "checked")),
				),
				h.Div(
					h.Class("provider-card-inner"),
					h.Div(
						h.Class("provider-card-top"),
						h.Span(
							h.Class("provider-card-icon-wrap provider-icon-s3"),
							lucide.Cloud(h.Class("provider-card-lucide"), g.Attr("aria-hidden", "true")),
						),
						h.Span(h.Class("provider-card-badge provider-badge-cloud"), g.Text("Cloud")),
					),
					h.Strong(h.Class("provider-card-title"), g.Text("S3 Compatible")),
					h.P(h.Class("provider-card-desc"), g.Text("AWS S3, Cloudflare R2, MinIO, Wasabi, Backblaze B2. Resilient cloud object storage.")),
				),
			),
		),
		components.InputField(components.InputProps{
			Label:       "Instance Storage Quota (GB)",
			Name:        "storage_gb",
			Type:        components.InputTypeNumber,
			Min:         "0",
			Value:       storageGB,
			Description: "Storage ceiling across your vault. Enter 0 for unlimited capacity.",
			HelperText:  validation.FieldError(errors, "storage_gb"),
			HasError:    validation.FieldError(errors, "storage_gb") != "",
		}),
		h.Div(
			h.Class("settings-provider-fields local-panel"),
			h.Div(
				h.Class("form-section-head"),
				h.H4(g.Text("Local Storage Parameters")),
				h.P(g.Text("Filesystem location where encrypted chunks are stored.")),
			),
			components.InputField(components.InputProps{
				Label:       "Host Directory Path",
				Name:        "local_path",
				Type:        components.InputTypeText,
				Value:       settings.LocalPath,
				Placeholder: "./data or /var/lib/arkive/files",
				Description: "Relative or absolute directory on server filesystem.",
				HelperText:  validation.FieldError(errors, "local_path"),
				HasError:    validation.FieldError(errors, "local_path") != "",
			}),
		),
		h.Div(
			h.Class("settings-provider-fields s3-panel"),
			h.Div(
				h.Class("form-section-head"),
				h.H4(g.Text("S3 API Credentials")),
				h.P(g.Text("Authentication keys for your S3-compatible bucket.")),
			),
			h.Div(
				h.Class("settings-form-grid-two"),
				components.InputField(components.InputProps{
					Label:       "S3 Access Key ID",
					Name:        "s3_access_key_id",
					Type:        components.InputTypeText,
					Value:       settings.S3AccessKeyID,
					Placeholder: "AKIAIOSFODNN7EXAMPLE",
					HelperText:  validation.FieldError(errors, "s3_access_key_id"),
					HasError:    validation.FieldError(errors, "s3_access_key_id") != "",
				}),
				components.InputField(components.InputProps{
					Label:       "S3 Secret Access Key",
					Name:        "s3_secret_access_key",
					Type:        components.InputTypePassword,
					Placeholder: "Leave blank to keep existing secret",
					HelperText:  validation.FieldError(errors, "s3_secret_access_key"),
					HasError:    validation.FieldError(errors, "s3_secret_access_key") != "",
				}),
			),
			components.InputField(components.InputProps{
				Label:       "S3 Session Token (Optional)",
				Name:        "s3_session_token",
				Type:        components.InputTypePassword,
				Value:       settings.S3SessionToken,
				Placeholder: "Optional STS session token",
				Description: "Only required when authenticating with temporary AWS STS credentials.",
			}),
			h.Div(
				h.Class("form-section-head"),
				h.H4(g.Text("Bucket & Location")),
				h.P(g.Text("Target bucket name and provider region.")),
			),
			h.Div(
				h.Class("settings-form-grid-two"),
				components.InputField(components.InputProps{
					Label:       "Bucket Name",
					Name:        "s3_bucket",
					Type:        components.InputTypeText,
					Value:       settings.S3Bucket,
					Placeholder: "arkive-vault-data",
					HelperText:  validation.FieldError(errors, "s3_bucket"),
					HasError:    validation.FieldError(errors, "s3_bucket") != "",
				}),
				components.InputField(components.InputProps{
					Label:       "Region",
					Name:        "s3_region",
					Type:        components.InputTypeText,
					Value:       settings.S3Region,
					Placeholder: "us-east-1 (or 'auto' for R2)",
				}),
			),
			h.Div(
				h.Class("form-section-head"),
				h.H4(g.Text("Endpoint & Addressing")),
				h.P(g.Text("Custom API endpoint and URL format.")),
			),
			components.InputField(components.InputProps{
				Label:       "Custom S3 Endpoint URL",
				Name:        "s3_endpoint",
				Type:        components.InputTypeText,
				Value:       settings.S3Endpoint,
				Placeholder: "https://<account-id>.r2.cloudflarestorage.com",
				Description: "Leave blank for standard AWS S3, or specify your provider endpoint (R2, Wasabi, MinIO, Backblaze B2).",
				HelperText:  validation.FieldError(errors, "s3_endpoint"),
				HasError:    validation.FieldError(errors, "s3_endpoint") != "",
			}),
			h.Label(
				h.Class("settings-checkbox-card"),
				h.Input(
					g.Attr("type", "checkbox"),
					g.Attr("name", "s3_use_path_style"),
					g.If(settings.S3UsePathStyle, g.Attr("checked", "checked")),
				),
				h.Div(
					h.Class("checkbox-card-content"),
					h.Strong(g.Text("Use path-style addressing")),
					h.Span(g.Text("Required for MinIO, Ceph, and self-hosted S3 gateways (https://endpoint/bucket).")),
				),
			),
		),
		h.Div(
			h.Class("settings-form-actions"),
			components.Button(components.ButtonProps{
				Text:     "Save Storage Settings",
				Type:     "submit",
				Variant:  "primary",
				Class:    "settings-submit-btn",
				BusyText: "Saving storage settings...",
			}),
		),
	)
}

func uploadSettingsForm(settings models.UploadSettings, errors validation.Errors) g.Node {
	generalError := validation.FieldError(errors, validation.GeneralKey)

	return h.Form(
		h.Class("settings-form"),
		g.Attr("method", "POST"),
		g.Attr("action", "/settings/uploads"),
		g.If(
			generalError != "",
			h.Div(
				h.Class("form-error settings-form-error"),
				lucide.CircleAlert(h.Class("error-lucide"), g.Attr("aria-hidden", "true")),
				h.Span(g.Text(generalError)),
			),
		),
		components.InputField(components.InputProps{
			Label:       "Max Queue Items",
			Name:        "max_queue_items",
			Type:        components.InputTypeNumber,
			Min:         "1",
			Max:         "1000",
			Value:       strconv.Itoa(settings.MaxQueueItems),
			Description: "Maximum files queued simultaneously in upload tray. Default: 300.",
			HelperText:  validation.FieldError(errors, "max_queue_items"),
			HasError:    validation.FieldError(errors, "max_queue_items") != "",
		}),
		components.InputField(components.InputProps{
			Label:       "Part Concurrency",
			Name:        "part_concurrency",
			Type:        components.InputTypeNumber,
			Min:         "1",
			Max:         "10",
			Value:       strconv.Itoa(settings.PartConcurrency),
			Description: "Parallel chunks per file. Encrypted in browser Web Workers before sending. Recommended: 3-5.",
			HelperText:  validation.FieldError(errors, "part_concurrency"),
			HasError:    validation.FieldError(errors, "part_concurrency") != "",
		}),
		components.InputField(components.InputProps{
			Label:       "Stale Upload Window (Hours)",
			Name:        "stale_upload_hours",
			Type:        components.InputTypeNumber,
			Min:         "1",
			Max:         "168",
			Value:       strconv.Itoa(settings.StaleUploadHours),
			Description: "Lifespan of unfinished multipart sessions and presigned URLs before automatic server cleanup. Default: 1.",
			HelperText:  validation.FieldError(errors, "stale_upload_hours"),
			HasError:    validation.FieldError(errors, "stale_upload_hours") != "",
		}),
		settingsSecurityNotice(
			"Client-Side Chunking Pipeline",
			"Arkive Core chunks large files and encrypts each chunk in browser Web Workers using AES-256-GCM. Higher concurrency increases throughput on high-bandwidth connections, while lower concurrency conserves client device CPU and memory.",
		),
		h.Div(
			h.Class("settings-form-actions"),
			components.Button(components.ButtonProps{
				Text:     "Save Upload Settings",
				Type:     "submit",
				Variant:  "primary",
				Class:    "settings-submit-btn",
				BusyText: "Saving upload settings...",
			}),
		),
	)
}

func previewSettingsForm(settings models.PreviewSettings, errors validation.Errors) g.Node {
	generalError := validation.FieldError(errors, validation.GeneralKey)

	return h.Form(
		h.Class("settings-form"),
		g.Attr("method", "POST"),
		g.Attr("action", "/settings/previews"),
		g.If(
			generalError != "",
			h.Div(
				h.Class("form-error settings-form-error"),
				lucide.CircleAlert(h.Class("error-lucide"), g.Attr("aria-hidden", "true")),
				h.Span(g.Text(generalError)),
			),
		),
		components.InputField(components.InputProps{
			Label:       "Image Preview Max (MB)",
			Name:        "image_max_mb",
			Type:        components.InputTypeNumber,
			Min:         "1",
			Max:         strconv.Itoa(models.MaxImagePreviewMB),
			Value:       strconv.FormatInt(settings.ImageMaxBytes/(1024*1024), 10),
			Description: "Largest image file rendered in lightbox view. Default: 50 MB (max: 100 MB).",
			HelperText:  validation.FieldError(errors, "image_max_mb"),
			HasError:    validation.FieldError(errors, "image_max_mb") != "",
		}),
		components.InputField(components.InputProps{
			Label:       "Video Fallback Max (MB)",
			Name:        "video_max_mb",
			Type:        components.InputTypeNumber,
			Min:         "1",
			Max:         strconv.Itoa(models.MaxVideoPreviewMB),
			Value:       strconv.FormatInt(settings.VideoMaxBytes/(1024*1024), 10),
			Description: "Fallback cap for legacy video playback. Range streaming is not limited by this value (max: 500 MB).",
			HelperText:  validation.FieldError(errors, "video_max_mb"),
			HasError:    validation.FieldError(errors, "video_max_mb") != "",
		}),
		components.InputField(components.InputProps{
			Label:       "Text Preview Max (MB)",
			Name:        "text_max_mb",
			Type:        components.InputTypeNumber,
			Min:         "1",
			Max:         strconv.Itoa(models.MaxTextPreviewMB),
			Value:       strconv.FormatInt(settings.TextMaxBytes/(1024*1024), 10),
			Description: "Maximum text or code document size loaded in syntax viewer. Default: 2 MB (max: 20 MB).",
			HelperText:  validation.FieldError(errors, "text_max_mb"),
			HasError:    validation.FieldError(errors, "text_max_mb") != "",
		}),
		settingsInfoCallout(
			lucide.Sparkles,
			"Adaptive Cryptographic Range Streaming",
			"Arkive Core includes an adaptive byte-range streaming video player. When video streaming is supported by the browser, video files of any size can be played on-demand by decrypting discrete cryptographic blocks in sequence.",
		),
		h.Div(
			h.Class("settings-form-actions"),
			components.Button(components.ButtonProps{
				Text:     "Save Preview Settings",
				Type:     "submit",
				Variant:  "primary",
				Class:    "settings-submit-btn",
				BusyText: "Saving preview settings...",
			}),
		),
	)
}

func settingsStorageGB(bytes int64) string {
	if bytes <= 0 || bytes == math.MaxInt64 {
		return "0"
	}
	return strconv.FormatInt(bytes/(1024*1024*1024), 10)
}
