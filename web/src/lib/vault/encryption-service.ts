import {
  AES_GCM_IV_BYTES,
  AES_GCM_KEY_BYTES,
  AES_GCM_TAG_BYTES,
  ENCRYPTED_ENVELOPE_OVERHEAD_BYTES,
  ENCRYPTION_VERSION,
} from "./crypto-format";

export class EncryptionService {
  wrapKey(
    keyToWrap: Uint8Array,
    wrappingKey: Uint8Array,
    aad: Uint8Array,
  ): Promise<Uint8Array> {
    if (keyToWrap.byteLength !== AES_GCM_KEY_BYTES)
      throw new Error("Invalid key to wrap.");
    return this.encrypt(keyToWrap, wrappingKey, aad);
  }

  async encrypt(
    plaintext: Uint8Array,
    rawKey: Uint8Array,
    aad: Uint8Array,
  ): Promise<Uint8Array> {
    if (rawKey.byteLength !== AES_GCM_KEY_BYTES)
      throw new Error("Invalid encryption key.");

    const iv = globalThis.crypto.getRandomValues(
      new Uint8Array(AES_GCM_IV_BYTES),
    );
    let sealed: Uint8Array | null = null;
    try {
      const key = await globalThis.crypto.subtle.importKey(
        "raw",
        rawKey as BufferSource,
        "AES-GCM",
        false,
        ["encrypt"],
      );
      sealed = new Uint8Array(
        await globalThis.crypto.subtle.encrypt(
          {
            name: "AES-GCM",
            iv,
            additionalData: aad as BufferSource,
            tagLength: AES_GCM_TAG_BYTES * 8,
          },
          key,
          plaintext as BufferSource,
        ),
      );

      const envelope = new Uint8Array(
        ENCRYPTED_ENVELOPE_OVERHEAD_BYTES + plaintext.byteLength,
      );
      envelope[0] = ENCRYPTION_VERSION;
      envelope.set(iv, 1);
      envelope.set(sealed, 1 + AES_GCM_IV_BYTES);
      return envelope;
    } finally {
      iv.fill(0);
      sealed?.fill(0);
    }
  }
}
