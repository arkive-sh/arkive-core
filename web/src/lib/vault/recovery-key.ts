import type { HashService } from './hash-service';

const VERSION = 1;
const KEY_BYTES = 32;
const CHECKSUM_BYTES = 3;
const MAX_INPUT_LENGTH = 256;
const PREFIX = 'ARK-RK1';
const CHECKSUM_DOMAIN = new TextEncoder().encode('arkive-recovery-key-v1');

async function checksum(key: Uint8Array, hashes: HashService): Promise<Uint8Array> {
	const input = new Uint8Array(CHECKSUM_DOMAIN.length + 1 + key.length);
	input.set(CHECKSUM_DOMAIN);
	input[CHECKSUM_DOMAIN.length] = VERSION;
	input.set(key, CHECKSUM_DOMAIN.length + 1);
	let digest: Uint8Array | null = null;

	try {
		digest = await hashes.sha256(input);

		return digest.slice(0, CHECKSUM_BYTES);
	} finally {
		input.fill(0);
		digest?.fill(0);
	}
}

export async function formatRecoveryKey(key: Uint8Array, hashes: HashService): Promise<string> {
	if (key.byteLength !== KEY_BYTES) throw new Error('Invalid recovery key.');
	const payload = new Uint8Array(1 + KEY_BYTES + CHECKSUM_BYTES);
	let digest: Uint8Array | null = null;

	try {
		payload[0] = VERSION;
		payload.set(key, 1);
		digest = await checksum(key, hashes);
		payload.set(digest, 1 + KEY_BYTES);
		const hex = Array.from(payload, (byte) => byte.toString(16).padStart(2, '0'))
			.join('')
			.toUpperCase();
		const groups = hex.match(/.{1,8}/g)?.join('-') ?? '';

		return `${PREFIX}-${groups}`;
	} finally {
		payload.fill(0);
		digest?.fill(0);
	}
}

export async function parseRecoveryKey(value: string, hashes: HashService): Promise<Uint8Array> {
	if (new TextEncoder().encode(value).byteLength > MAX_INPUT_LENGTH)
		throw new Error('Invalid recovery key.');
	const compact = value.replace(/[-\t\n\v\f\r ]/g, '').toUpperCase();
	const compactPrefix = PREFIX.replace('-', '');
	const hex = compact.startsWith(compactPrefix) ? compact.slice(compactPrefix.length) : '';

	if (hex.length !== (1 + KEY_BYTES + CHECKSUM_BYTES) * 2 || !/^[0-9A-F]+$/.test(hex)) {
		throw new Error('Invalid recovery key.');
	}

	const payload = new Uint8Array(hex.length / 2);
	let digest: Uint8Array | null = null;

	try {
		for (let i = 0; i < payload.length; i += 1)
			payload[i] = Number.parseInt(hex.slice(i * 2, i * 2 + 2), 16);

		if (payload[0] !== VERSION) throw new Error('Invalid recovery key.');

		const key = payload.subarray(1, 1 + KEY_BYTES);
		digest = await checksum(key, hashes);
		let mismatch = 0;

		for (let i = 0; i < CHECKSUM_BYTES; i += 1) mismatch |= payload[1 + KEY_BYTES + i] ^ digest[i];

		if (mismatch) throw new Error('Invalid recovery key.');

		return key.slice();
	} catch {
		throw new Error('Invalid recovery key.');
	} finally {
		payload.fill(0);
		digest?.fill(0);
	}
}
