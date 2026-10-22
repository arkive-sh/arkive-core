package pages

import (
	"strconv"
	"strings"

	lucide "github.com/eduardolat/gomponents-lucide"
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"

	"arkive/core/models"
)

func settingsTabLink(target, label string, icon func(...g.Node) g.Node) g.Node {
	return h.A(
		h.Class("settings-tab"),
		h.Href("#"+target),
		h.Span(
			h.Class("settings-tab-icon-wrap"),
			icon(
				h.Class("settings-tab-lucide"),
				g.Attr("aria-hidden", "true"),
			),
		),
		h.Span(h.Class("settings-tab-label"), g.Text(label)),
		h.Span(h.Class("settings-tab-indicator"), g.Attr("aria-hidden", "true")),
	)
}

func settingsNotification(message string) g.Node {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return nil
	}

	displayText := trimmed
	switch trimmed {
	case "storage-updated":
		displayText = "Storage provider settings were saved successfully."
	case "upload-updated":
		displayText = "Upload queue limits were updated successfully."
	case "preview-updated":
		displayText = "In-browser preview thresholds were updated successfully."
	}

	return h.Div(
		h.Class("settings-banner settings-banner-success"),
		h.Span(
			h.Class("settings-banner-icon"),
			lucide.CircleCheckBig(
				h.Class("banner-lucide"),
				g.Attr("aria-hidden", "true"),
			),
		),
		h.Div(
			h.Class("settings-banner-content"),
			h.Strong(g.Text("Settings updated")),
			h.P(g.Text(displayText)),
		),
	)
}

func settingsAdminCard(user *models.User, brandName, email, memberSince, lastLogin string) g.Node {
	adminInitial := "A"
	if email != "" && email != "Unavailable" {
		adminInitial = strings.ToUpper(string(email[0]))
	} else if brandName != "" && brandName != "Unavailable" {
		adminInitial = strings.ToUpper(string(brandName[0]))
	}

	return h.Div(
		h.Class("settings-identity-card"),
		h.Div(
			h.Class("settings-identity-header"),
			h.Div(
				h.Class("settings-identity-avatar"),
				h.Span(h.Class("avatar-letter"), g.Text(adminInitial)),
				h.Span(
					h.Class("avatar-status-dot"),
					g.Attr("title", "Active Administrator Session"),
				),
			),
			h.Div(
				h.Class("settings-identity-details"),
				h.Div(
					h.Class("settings-identity-title-row"),
					h.H3(g.Text(displayOrDash(brandName))),
					h.Span(h.Class("identity-role-badge"), g.Text("Master Admin")),
				),
				h.P(h.Class("settings-identity-email"), g.Text(displayOrDash(email))),
			),
		),
		h.Div(
			h.Class("settings-meta-grid"),
			h.Div(
				h.Class("settings-meta-tile"),
				h.Div(
					h.Class("meta-tile-head"),
					lucide.Layers(h.Class("meta-tile-lucide"), g.Attr("aria-hidden", "true")),
					h.Span(h.Class("meta-tile-label"), g.Text("Instance")),
				),
				h.Strong(h.Class("meta-tile-value"), g.Text(displayOrDash(brandName))),
			),
			h.Div(
				h.Class("settings-meta-tile"),
				h.Div(
					h.Class("meta-tile-head"),
					lucide.Mail(h.Class("meta-tile-lucide"), g.Attr("aria-hidden", "true")),
					h.Span(h.Class("meta-tile-label"), g.Text("Owner Email")),
				),
				h.Strong(h.Class("meta-tile-value"), g.Text(displayOrDash(email))),
			),
			h.Div(
				h.Class("settings-meta-tile"),
				h.Div(
					h.Class("meta-tile-head"),
					lucide.Calendar(h.Class("meta-tile-lucide"), g.Attr("aria-hidden", "true")),
					h.Span(h.Class("meta-tile-label"), g.Text("Provisioned")),
				),
				h.Strong(h.Class("meta-tile-value"), g.Text(displayOrDash(memberSince))),
			),
			h.Div(
				h.Class("settings-meta-tile"),
				h.Div(
					h.Class("meta-tile-head"),
					lucide.Clock(h.Class("meta-tile-lucide"), g.Attr("aria-hidden", "true")),
					h.Span(h.Class("meta-tile-label"), g.Text("Last Active")),
				),
				h.Strong(h.Class("meta-tile-value"), g.Text(displayOrDash(lastLogin))),
			),
		),
	)
}

func settingsCapacityCard(usedStorage, quotaStorage string, usagePercent int, maxStorageBytes int64, providerLabel string) g.Node {
	isUnlimited := maxStorageBytes <= 0
	percentStr := formatPercent(usagePercent)

	return h.Div(
		h.Class("settings-capacity-card"),
		h.Div(
			h.Class("settings-capacity-head"),
			h.Div(
				h.Class("capacity-title-block"),
				h.Span(h.Class("settings-overline"), g.Text("Encrypted Storage Quota")),
				h.H3(g.Text("Storage Utilization")),
			),
			h.Div(
				h.Class("capacity-badge-wrap"),
				g.If(isUnlimited, h.Span(h.Class("capacity-status-chip chip-unlimited"), g.Text("Uncapped Capacity"))),
				g.If(!isUnlimited, h.Span(h.Class("capacity-status-chip"), g.Text(percentStr+" Utilized"))),
			),
		),
		h.Div(
			h.Class("settings-gauge-display"),
			h.Div(
				h.Class("gauge-numbers"),
				h.Span(h.Class("gauge-used"), g.Text(usedStorage)),
				h.Span(h.Class("gauge-separator"), g.Text("/")),
				h.Span(h.Class("gauge-total"), g.Text(quotaStorage)),
			),
			h.Span(
				h.Class("gauge-subtext"),
				g.If(isUnlimited, g.Text("No artificial limit configured. Constrained only by backend drive.")),
				g.If(!isUnlimited, g.Text("Enforced across user vaults prior to chunk upload.")),
			),
		),
		h.Div(
			h.Class("settings-progress-container"),
			h.Div(
				h.Class("settings-progress-track"),
				h.Span(
					h.Class("settings-progress-fill"),
					g.If(!isUnlimited, g.Attr("style", "width: "+percentStr)),
					g.If(isUnlimited, g.Attr("style", "width: 100%")),
					g.If(isUnlimited, h.Class("progress-fill-unlimited")),
				),
			),
		),
		h.Div(
			h.Class("settings-info-grid"),
			h.Div(
				h.Class("settings-info-tile"),
				h.Span(h.Class("settings-info-label"), g.Text("Deployment")),
				h.Strong(g.Text("Single-Tenant Core")),
				h.P(g.Text("Isolated personal file server")),
			),
			h.Div(
				h.Class("settings-info-tile"),
				h.Span(h.Class("settings-info-label"), g.Text("Active Backend")),
				h.Strong(g.Text(providerLabel)),
				h.A(
					h.Class("settings-tile-link"),
					h.Href("#settings-provider"),
					g.Text("Configure provider →"),
				),
			),
			h.Div(
				h.Class("settings-info-tile"),
				h.Span(h.Class("settings-info-label"), g.Text("Quota Policy")),
				h.Strong(g.If(isUnlimited, g.Text("Unlimited (0 GB)")), g.If(!isUnlimited, g.Text(quotaStorage))),
				h.P(g.Text("Modifiable in Storage Provider")),
			),
		),
		settingsInfoCallout(
			lucide.Info,
			"Storage Limit Controls",
			"You can adjust the total storage capacity or set it to 0 for unlimited storage in the Storage Provider tab.",
		),
	)
}

func storageProviderStatusCard(settings models.StorageSettings) g.Node {
	provider := strings.ToLower(strings.TrimSpace(settings.Provider))
	if provider == "" {
		provider = "local"
	}

	if provider == "s3" {
		targetSummary := "S3-compatible Object Storage"
		if settings.S3Bucket != "" {
			targetSummary = "Bucket: " + settings.S3Bucket
		}
		if settings.S3Endpoint != "" {
			targetSummary += " @ " + settings.S3Endpoint
		}

		return h.Div(
			h.Class("settings-status-card status-card-s3"),
			h.Div(
				h.Class("status-card-header"),
				h.Div(
					h.Class("status-card-icon-bubble bubble-s3"),
					lucide.Cloud(h.Class("status-card-lucide"), g.Attr("aria-hidden", "true")),
				),
				h.Div(
					h.Class("status-card-titles"),
					h.Div(
						h.Class("status-badge-row"),
						h.Span(h.Class("settings-badge badge-s3"), g.Text("S3 Compatible")),
						h.Span(h.Class("status-online-beacon"), h.Span(h.Class("beacon-dot")), g.Text("Operational")),
					),
					h.H4(g.Text("Cloud Object Storage is Active")),
				),
			),
			h.P(
				h.Class("status-card-desc"),
				g.Text("Encrypted chunk payloads are transferred directly to your configured S3-compatible bucket. Zero-knowledge encryption ensures the cloud host receives opaque ciphertext blobs only."),
			),
			h.Div(
				h.Class("status-target-pill"),
				lucide.Database(h.Class("target-lucide"), g.Attr("aria-hidden", "true")),
				h.Span(g.Text(targetSummary)),
			),
			settingsSecurityNotice(
				"Zero-Knowledge Cloud Isolation",
				"All files, filenames, and folder structures are client-side encrypted (AES-256-GCM) with your vault keys before transmission. S3 credentials remain securely on your self-hosted server.",
			),
		)
	}

	localPathSummary := settings.LocalPath
	if localPathSummary == "" {
		localPathSummary = "./data (Default storage path)"
	}

	return h.Div(
		h.Class("settings-status-card status-card-local"),
		h.Div(
			h.Class("status-card-header"),
			h.Div(
				h.Class("status-card-icon-bubble bubble-local"),
				lucide.HardDrive(h.Class("status-card-lucide"), g.Attr("aria-hidden", "true")),
			),
			h.Div(
				h.Class("status-card-titles"),
				h.Div(
					h.Class("status-badge-row"),
					h.Span(h.Class("settings-badge badge-local"), g.Text("Local Disk")),
					h.Span(h.Class("status-online-beacon"), h.Span(h.Class("beacon-dot")), g.Text("Operational")),
				),
				h.H4(g.Text("Local Disk Storage is Active")),
			),
		),
		h.P(
			h.Class("status-card-desc"),
			g.Text("Encrypted chunks are written directly to the host filesystem. This gives maximum I/O throughput with zero external network egress or third-party cloud fees."),
		),
		h.Div(
			h.Class("status-target-pill"),
			lucide.Folder(h.Class("target-lucide"), g.Attr("aria-hidden", "true")),
			h.Span(g.Text("Host Path: "+localPathSummary)),
		),
		settingsSecurityNotice(
			"Local Disk Cryptographic Safety",
			"Files written to disk are encrypted with distinct Data Encryption Keys (DEKs). Even root or filesystem administrators cannot decrypt files without your master password.",
		),
	)
}

func settingsStatCard(icon func(...g.Node) g.Node, metric, unit, label, hint string, badge string) g.Node {
	return h.Div(
		h.Class("settings-stat-card"),
		h.Div(
			h.Class("stat-card-head"),
			h.Span(
				h.Class("stat-icon-wrap"),
				icon(h.Class("stat-lucide"), g.Attr("aria-hidden", "true")),
			),
			g.If(badge != "", h.Span(h.Class("stat-badge"), g.Text(badge))),
		),
		h.Div(
			h.Class("stat-value-group"),
			h.Strong(h.Class("stat-number"), g.Text(metric)),
			g.If(unit != "", h.Span(h.Class("stat-unit"), g.Text(unit))),
		),
		h.Span(h.Class("stat-label"), g.Text(label)),
		h.P(h.Class("stat-hint"), g.Text(hint)),
	)
}

func settingsMediaCategoryCard(icon func(...g.Node) g.Node, capValue, unit, title string, badges []string, desc string) g.Node {
	badgeNodes := make([]g.Node, 0, len(badges))
	for _, b := range badges {
		badgeNodes = append(badgeNodes, h.Span(h.Class("media-format-pill"), g.Text(b)))
	}

	return h.Div(
		h.Class("settings-media-card"),
		h.Div(
			h.Class("media-card-top"),
			h.Span(
				h.Class("media-icon-bubble"),
				icon(h.Class("media-lucide"), g.Attr("aria-hidden", "true")),
			),
			h.Div(
				h.Class("media-cap-badge"),
				h.Strong(g.Text(capValue)),
				h.Span(g.Text(unit)),
			),
		),
		h.H4(h.Class("media-card-title"), g.Text(title)),
		h.P(h.Class("media-card-desc"), g.Text(desc)),
		h.Div(
			h.Class("media-formats-list"),
			g.Group(badgeNodes),
		),
	)
}

func settingsSecurityNotice(title, text string) g.Node {
	return h.Div(
		h.Class("settings-notice-box notice-security"),
		h.Span(
			h.Class("notice-icon-wrap"),
			lucide.Shield(h.Class("notice-lucide"), g.Attr("aria-hidden", "true")),
		),
		h.Div(
			h.Class("notice-body"),
			h.Strong(g.Text(title)),
			h.P(g.Text(text)),
		),
	)
}

func settingsInfoCallout(icon func(...g.Node) g.Node, title, text string) g.Node {
	return h.Div(
		h.Class("settings-notice-box notice-info"),
		h.Span(
			h.Class("notice-icon-wrap"),
			icon(h.Class("notice-lucide"), g.Attr("aria-hidden", "true")),
		),
		h.Div(
			h.Class("notice-body"),
			h.Strong(g.Text(title)),
			h.P(g.Text(text)),
		),
	)
}

func formatPercent(value int) string {
	if value <= 0 {
		return "0%"
	}
	if value >= 100 {
		return "100%"
	}
	return strconv.Itoa(value) + "%"
}

func displayOrDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "Unavailable"
	}
	return value
}
