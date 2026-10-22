import type { VaultAAD, VaultBytes } from './types';

export function toBase64(value?: VaultBytes | string): string {
	if (!value) return '';
	if (typeof value === 'string') return value;
	const bytes = value instanceof Uint8Array ? value : new Uint8Array(value);
	let binary = '';
	for (const byte of bytes) binary += String.fromCharCode(byte);
	return btoa(binary);
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
