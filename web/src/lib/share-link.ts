import { toBase64Url } from '#lib/vault/bytes.ts';
import { openShareKey } from '#lib/vault/shares.ts';

export function buildShareURL(origin: string, token: string, secret: Uint8Array): string {
	// The fragment is kept in the browser and is not sent with the HTTP request.
	return `${origin}/share/${encodeURIComponent(token)}#key=${toBase64Url(secret)}`;
}

export async function decryptShareURL(
	origin: string,
	token: string,
	encryptedShareKey: string,
): Promise<string> {
	const { shareSecret } = await openShareKey(encryptedShareKey, token);

	try {
		return buildShareURL(origin, token, shareSecret);
	} finally {
		shareSecret.fill(0);
	}
}
