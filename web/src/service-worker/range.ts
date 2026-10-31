export interface ByteRange {
	start: number;
	end: number;
}

// TODO: Add tests for valid, suffix, out-of-bounds, and malformed byte ranges.
export function parseByteRange(header: string | null, size: number, maxBytes: number): ByteRange | null {
	if (!Number.isSafeInteger(size) || size <= 0 || !Number.isSafeInteger(maxBytes) || maxBytes <= 0) return null;
	if (header === null) return { start: 0, end: Math.min(size, maxBytes) - 1 };

	const match = /^bytes=(\d*)-(\d*)$/i.exec(header.trim());
	if (!match || (!match[1] && !match[2])) return null;

	if (!match[1]) {
		const suffixSize = Number(match[2]);
		if (suffixSize <= 0) return null;
		const length = Math.min(size, maxBytes, suffixSize);
		return { start: size - length, end: size - 1 };
	}

	const start = Number(match[1]);
	const requestedEnd = match[2] ? Number(match[2]) : size - 1;
	if (!Number.isSafeInteger(start) || start >= size || Number.isNaN(requestedEnd) || requestedEnd < start) {
		return null;
	}

	return {
		start,
		end: Math.min(requestedEnd, size - 1, start + maxBytes - 1)
	};
}
