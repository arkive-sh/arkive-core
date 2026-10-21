const encoder = new TextEncoder();
const hkdfSalt = encoder.encode("arkive:recovery-proof:v1");
const privateKeyInfoPrefix = Uint8Array.from([
  0x30, 0x2e, 0x02, 0x01, 0x00, 0x30, 0x05, 0x06,
  0x03, 0x2b, 0x65, 0x70, 0x04, 0x22, 0x04, 0x20,
]);

function decodeBase64URL(value) {
  const normalized = String(value || "").replace(/-/g, "+").replace(/_/g, "/");
  const binary = atob(normalized + "=".repeat((4 - normalized.length % 4) % 4));
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i += 1) {
    bytes[i] = binary.charCodeAt(i);
  }
  return bytes;
}

async function derivePrivateKey(recoveryKey, userID, extractable) {
  const subtle = globalThis.crypto && globalThis.crypto.subtle;
  if (!subtle) {
    throw new Error("Recovery proof requires a secure browser context");
  }

  const inputKey = await subtle.importKey("raw", recoveryKey, "HKDF", false, ["deriveBits"]);
  const seed = new Uint8Array(await subtle.deriveBits({
    name: "HKDF",
    hash: "SHA-256",
    salt: hkdfSalt,
    info: encoder.encode(`ed25519-seed\n${userID}`),
  }, inputKey, 256));
  // Web Crypto imports Ed25519 private seeds as PKCS#8.
  const pkcs8 = new Uint8Array(privateKeyInfoPrefix.length + seed.length);
  pkcs8.set(privateKeyInfoPrefix);
  pkcs8.set(seed, privateKeyInfoPrefix.length);

  try {
    try {
      return await subtle.importKey("pkcs8", pkcs8, { name: "Ed25519" }, extractable, ["sign"]);
    } catch (_) {
      throw new Error("This browser does not support Ed25519 recovery proofs");
    }
  } finally {
    seed.fill(0);
    pkcs8.fill(0);
  }
}

export async function deriveRecoveryProofPublicKey(recoveryKey, userID) {
  const privateKey = await derivePrivateKey(recoveryKey, userID, true);
  const jwk = await globalThis.crypto.subtle.exportKey("jwk", privateKey);
  const publicKey = decodeBase64URL(jwk.x);
  jwk.d = "";
  return publicKey;
}

export async function signRecoveryProof(recoveryKey, userID, resetToken) {
  const privateKey = await derivePrivateKey(recoveryKey, userID, false);
  const message = encoder.encode(
    `arkive:recovery-proof:v1\npassword-reset\n${userID}\n${resetToken}`,
  );
  return new Uint8Array(await globalThis.crypto.subtle.sign("Ed25519", privateKey, message));
}
