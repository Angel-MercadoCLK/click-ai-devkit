// Protected storage for native Windows.
//
// POSIX storage relies on owner-only modes, uid checks and O_NOFOLLOW, none of which can be verified
// on native Windows. There, the desktop shell (Electron main) injects a protector backed by
// `safeStorage` (DPAPI, bound to the Windows user) before the Next server loads, by setting
// `globalThis[Symbol.for("click-dot.storage-protector")]`. The stores then keep their files encrypted
// instead of relying on file modes. Without a protector, native Windows keeps failing closed.

export type StorageProtector = {
  encrypt(text: string): Buffer;
  decrypt(data: Buffer): string;
};

export const STORAGE_PROTECTOR_KEY = Symbol.for("click-dot.storage-protector");

// Version marker so a plaintext or foreign file is rejected before it reaches the protector.
const MAGIC = Buffer.from("CLICKDOT-PROTECTED-1\n", "utf8");
// Allowance for the marker plus the protector's own framing when bounding the size of a stored file.
export const PROTECTED_OVERHEAD_BYTES = 64 * 1024;

const PROTECTED_ERROR = "Protected local storage could not be read.";

// Test seam: lets tests exercise the Windows code path on a POSIX host. Production code never calls
// it; without an override the real `process.platform` is used.
let platformOverride: NodeJS.Platform | undefined;

export function setStoragePlatformForTests(platform: NodeJS.Platform | undefined) {
  platformOverride = platform;
}

export function storagePlatform(): NodeJS.Platform {
  return platformOverride ?? process.platform;
}

export function getStorageProtector(): StorageProtector | undefined {
  const candidate = (globalThis as Record<symbol, unknown>)[STORAGE_PROTECTOR_KEY];
  if (!candidate || typeof candidate !== "object") return undefined;
  const { encrypt, decrypt } = candidate as Partial<StorageProtector>;
  if (typeof encrypt !== "function" || typeof decrypt !== "function") return undefined;
  return candidate as StorageProtector;
}

// The protector in effect for the current platform: only native Windows uses one. Returning
// undefined on Windows means no protector was injected, and callers must keep failing closed.
export function windowsStorageProtector(): StorageProtector | undefined {
  return storagePlatform() === "win32" ? getStorageProtector() : undefined;
}

export function sealText(protector: StorageProtector, text: string): Buffer {
  let encrypted: unknown;
  try {
    encrypted = protector.encrypt(text);
  } catch {
    throw new Error("Protected local storage could not be written.");
  }
  if (!Buffer.isBuffer(encrypted) || encrypted.length === 0) throw new Error("Protected local storage could not be written.");
  return Buffer.concat([MAGIC, encrypted]);
}

// Decrypts and parses a sealed file. Any failure is reported generically so file contents never leak.
export function openJson(protector: StorageProtector, data: Buffer): unknown {
  try {
    if (data.length <= MAGIC.length || !data.subarray(0, MAGIC.length).equals(MAGIC)) throw new Error();
    const text = protector.decrypt(data.subarray(MAGIC.length));
    if (typeof text !== "string") throw new Error();
    return JSON.parse(text);
  } catch {
    throw new Error(PROTECTED_ERROR);
  }
}
