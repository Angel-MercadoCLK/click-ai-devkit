/** Name shared with lib/storage-protector.ts, which reads globalThis[Symbol.for(STORAGE_PROTECTOR_KEY)]. */
export const STORAGE_PROTECTOR_KEY = "click-dot.storage-protector";

export type StorageProtector = {
  encrypt(text: string): Buffer;
  decrypt(data: Buffer): string;
};

export type SafeStorageLike = {
  isEncryptionAvailable(): boolean;
  encryptString(text: string): Buffer;
  decryptString(data: Buffer): string;
};

export function createStorageProtector(safeStorage: SafeStorageLike): StorageProtector {
  return {
    encrypt(text) {
      if (typeof text !== "string") throw new TypeError("Only text can be protected.");
      return safeStorage.encryptString(text);
    },
    decrypt(data) {
      if (!Buffer.isBuffer(data)) throw new TypeError("Protected data must be a Buffer.");
      return safeStorage.decryptString(data);
    },
  };
}

/**
 * Publishes the protector for the Next.js server code. When the OS cannot encrypt, nothing is
 * published and storage keeps failing closed instead of writing credentials unprotected.
 */
export function installStorageProtector(target: object, safeStorage: SafeStorageLike) {
  let available = false;
  try {
    available = safeStorage.isEncryptionAvailable();
  } catch {
    available = false;
  }
  if (!available) return false;
  Reflect.set(target, Symbol.for(STORAGE_PROTECTOR_KEY), createStorageProtector(safeStorage));
  return true;
}
