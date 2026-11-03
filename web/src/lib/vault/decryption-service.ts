import {
	AES_GCM_IV_BYTES,
	AES_GCM_KEY_BYTES,
	AES_GCM_TAG_BYTES,
	ENCRYPTED_ENVELOPE_OVERHEAD_BYTES,
	ENCRYPTION_VERSION,
} from './crypto-format';

const ENVELOPE_HEADER_BYTES = 1 + AES_GCM_IV_BYTES;

export class DecryptionService {
	async unwrapKey(
		envelope: Uint8Array,
		wrappingKey: Uint8Array,
		aad: Uint8Array,
	): Promise<Uint8Array> {
		const key = await this.decrypt(envelope, wrappingKey, aad);

		if (key.byteLength !== AES_GCM_KEY_BYTES) {
			key.fill(0);

			throw new Error('Invalid wrapped key.');
		}

		return key;
	}

	async decrypt(envelope: Uint8Array, rawKey: Uint8Array, aad: Uint8Array): Promise<Uint8Array> {
		if (
			rawKey.byteLength !== AES_GCM_KEY_BYTES ||
			envelope.byteLength < ENCRYPTED_ENVELOPE_OVERHEAD_BYTES ||
			envelope[0] !== ENCRYPTION_VERSION
		) {
			throw new Error('Invalid encrypted data.');
		}

		const key = await globalThis.crypto.subtle.importKey(
			'raw',
			rawKey as BufferSource,
			'AES-GCM',
			false,
			['decrypt'],
		);
		const iv = envelope.subarray(1, ENVELOPE_HEADER_BYTES);
		const sealed = envelope.subarray(ENVELOPE_HEADER_BYTES);
		const plaintext = await globalThis.crypto.subtle.decrypt(
			{
				name: 'AES-GCM',
				iv: iv as BufferSource,
				additionalData: aad as BufferSource,
				tagLength: AES_GCM_TAG_BYTES * 8,
			},
			key,
			sealed as BufferSource,
		);

		return new Uint8Array(plaintext);
	}
}
