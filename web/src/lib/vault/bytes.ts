import type { VaultAAD, VaultBytes } from './types';

export function toBase64(value?: VaultBytes | string): string {
	if (!value) return '';
	if (typeof value === 'string') return value;
	const bytes = value instanceof Uint8Array ? value : new Uint8Array(value);

	return bytes.toBase64();
}

export function toBase64Url(value: VaultBytes): string {
	return toBase64(value).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '');
}

export function fromBase64(value: string): Uint8Array<ArrayBuffer> {
	const normalized = value.trim().replace(/-/g, '+').replace(/_/g, '/');
	const binary = atob(normalized + '='.repeat((4 - (normalized.length % 4)) % 4));
	const bytes = new Uint8Array(binary.length);

	for (let index = 0; index < binary.length; index += 1) bytes[index] = binary.charCodeAt(index);

	return bytes;
}

export function encodeJSON(value: unknown): ArrayBuffer {
	return new TextEncoder().encode(JSON.stringify(value)).buffer as ArrayBuffer;
}

export function decodeJSON<T>(bytes: Uint8Array): T {
	try {
		return JSON.parse(new TextDecoder().decode(bytes)) as T;
	} finally {
		bytes.fill(0);
	}
}

export function toWorkerBytes(value: VaultBytes): ArrayBuffer {
	const source = value instanceof Uint8Array ? value : new Uint8Array(value);
	const copy = new Uint8Array(source.byteLength);
	copy.set(source);

	return copy.buffer;
}

export function normalizeAAD(value?: VaultAAD): VaultAAD {
	return value || '';
}
