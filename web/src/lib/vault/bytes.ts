import type { VaultAAD, VaultBytes } from './types';

export function toBase64(value?: VaultBytes | string): string {
	if (!value) return '';
	if (typeof value === 'string') return value;
	const bytes = value instanceof Uint8Array ? value : new Uint8Array(value);
	return bytes.toBase64();
}

export function fromBase64Url(value: string): ArrayBuffer {
	const normalized = value.replace(/-/g, '+').replace(/_/g, '/');
	const binary = atob(normalized + '='.repeat((4 - (normalized.length % 4)) % 4));
	const bytes = new Uint8Array(binary.length);
	for (let index = 0; index < binary.length; index += 1) bytes[index] = binary.charCodeAt(index);
	return bytes.buffer;
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
