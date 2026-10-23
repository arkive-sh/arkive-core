export function formatBytes(bytes: number | null | undefined): string {
	if (bytes == null || !Number.isFinite(bytes)) return '—';
	if (bytes <= 0) return '0 B';

	const units = ['B', 'KB', 'MB', 'GB', 'TB'];
	const unit = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
	const decimals = unit === 0 ? 0 : unit >= 3 ? 2 : 1;
	return `${(bytes / 1024 ** unit).toFixed(decimals)} ${units[unit]}`;
}

export function formatDate(value: string | number | Date): string {
	const date = value instanceof Date ? value : new Date(value);
	return Number.isNaN(date.getTime()) ? '—' : new Intl.DateTimeFormat(undefined, { dateStyle: 'medium' }).format(date);
}

export function formatRelativeTime(value: string | Date): string {
	const date = value instanceof Date ? value : new Date(value);
	const difference = Date.now() - date.getTime();
	if (!Number.isFinite(difference)) return '';
	if (difference < 0) {
		return new Intl.DateTimeFormat('en-US', {
			month: 'short', day: 'numeric', year: 'numeric', hour: '2-digit', minute: '2-digit', hourCycle: 'h23'
		}).format(date);
	}
	if (difference < 60_000) return 'Just now';
	if (difference < 3_600_000) return `${Math.floor(difference / 60_000)}m ago`;
	if (difference < 86_400_000) return `${Math.floor(difference / 3_600_000)}h ago`;
	if (difference < 604_800_000) return `${Math.floor(difference / 86_400_000)}d ago`;
	if (difference < 2_592_000_000) return `${Math.floor(difference / 604_800_000)}w ago`;
	return new Intl.DateTimeFormat('en-US', { month: 'short', day: 'numeric', year: 'numeric' }).format(date);
}
