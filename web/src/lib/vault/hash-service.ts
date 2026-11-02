export class HashService {
  async sha256(bytes: Uint8Array): Promise<Uint8Array> {
    return new Uint8Array(
      await globalThis.crypto.subtle.digest("SHA-256", bytes as BufferSource),
    );
  }

  async hkdf(
    keyBytes: Uint8Array,
    salt: Uint8Array,
    info: Uint8Array,
    length = 32,
  ): Promise<Uint8Array> {
    const key = await globalThis.crypto.subtle.importKey(
      "raw",
      keyBytes as BufferSource,
      "HKDF",
      false,
      ["deriveBits"],
    );
    const bits = await globalThis.crypto.subtle.deriveBits(
      {
        name: "HKDF",
        hash: "SHA-256",
        salt: salt as BufferSource,
        info: info as BufferSource,
      },
      key,
      length * 8,
    );
    return new Uint8Array(bits);
  }

  async hmacSha256(
    keyBytes: Uint8Array,
    bytes: Uint8Array,
  ): Promise<Uint8Array> {
    const key = await globalThis.crypto.subtle.importKey(
      "raw",
      keyBytes as BufferSource,
      { name: "HMAC", hash: "SHA-256" },
      false,
      ["sign"],
    );
    return new Uint8Array(
      await globalThis.crypto.subtle.sign("HMAC", key, bytes as BufferSource),
    );
  }
}
