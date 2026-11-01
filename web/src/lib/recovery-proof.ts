import { fromBase64 } from './vault/bytes';

const encoder = new TextEncoder();
const hkdfSalt = encoder.encode('arkive:recovery-proof:v1');
const privateKeyInfoPrefix = Uint8Array.from([
	0x30, 0x2e, 0x02, 0x01, 0x00, 0x30, 0x05, 0x06,
	0x03, 0x2b, 0x65, 0x70, 0x04, 0x22, 0x04, 0x20
]);

async function derivePrivateKey(recoveryKey: Uint8Array, userID: string, extractable: boolean): Promise<CryptoKey> {
	const subtle = globalThis.crypto?.subtle;
	if (!subtle) throw new Error('Recovery proof requires a secure browser context');

	let rawRecoveryKey: Uint8Array<ArrayBuffer> | null = new Uint8Array(recoveryKey.byteLength);
	rawRecoveryKey.set(recoveryKey);
	let inputKey: CryptoKey | null = null;
	try {
		inputKey = await subtle.importKey('raw', rawRecoveryKey, 'HKDF', false, ['deriveBits']);
	} finally {
		rawRecoveryKey.fill(0);
		rawRecoveryKey = null;
	}
	if (!inputKey) throw new Error('Recovery proof key derivation failed');
	let seed: Uint8Array<ArrayBuffer> | null = null;
	let pkcs8: Uint8Array<ArrayBuffer> | null = null;

	try {
		seed = new Uint8Array(await subtle.deriveBits({
			name: 'HKDF',
			hash: 'SHA-256',
			salt: hkdfSalt,
			info: encoder.encode(`ed25519-seed\n${userID}`)
		}, inputKey, 256));
		pkcs8 = new Uint8Array(privateKeyInfoPrefix.length + seed.length);
		pkcs8.set(privateKeyInfoPrefix);
		pkcs8.set(seed, privateKeyInfoPrefix.length);
		try {
			return await subtle.importKey('pkcs8', pkcs8, { name: 'Ed25519' }, extractable, ['sign']);
		} catch {
			throw new Error('This browser does not support Ed25519 recovery proofs');
		}
	} finally {
		seed?.fill(0);
		seed = null;
		pkcs8?.fill(0);
		pkcs8 = null;
		inputKey = null;
	}
}

export async function deriveRecoveryProofPublicKey(recoveryKey: Uint8Array, userID: string): Promise<Uint8Array> {
	let privateKey: CryptoKey | null = await derivePrivateKey(recoveryKey, userID, true);
	let jwk: JsonWebKey | null = null;
	try {
		jwk = await globalThis.crypto.subtle.exportKey('jwk', privateKey);
		return fromBase64(jwk.x ?? '');
	} finally {
		if (jwk) jwk.d = '';
		jwk = null;
		privateKey = null;
	}
}

export async function signRecoveryProof(recoveryKey: Uint8Array, userID: string, resetToken: string): Promise<Uint8Array> {
	let privateKey: CryptoKey | null = await derivePrivateKey(recoveryKey, userID, false);
	let message: Uint8Array<ArrayBuffer> | null = encoder.encode(`arkive:recovery-proof:v1\npassword-reset\n${userID}\n${resetToken}`);
	try {
		if (!privateKey || !message) throw new Error('Recovery proof signing key is unavailable');
		return new Uint8Array(await globalThis.crypto.subtle.sign('Ed25519', privateKey, message));
	} finally {
		message?.fill(0);
		message = null;
		privateKey = null;
	}
}
