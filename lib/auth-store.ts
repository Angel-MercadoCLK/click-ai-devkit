import { randomUUID } from "node:crypto";
import { constants } from "node:fs";
import { chmod, lstat, mkdir, open, rename, rmdir, unlink } from "node:fs/promises";
import { homedir } from "node:os";
import { dirname, isAbsolute, join, parse, sep } from "node:path";

export type AuthAttempt = {
  state: string;
  nonce: string;
  verifier: string;
  expiresAt: number;
  redirectUri: string;
  expectedClientId: string;
  expectedSubject?: string;
  generation: number;
};

export type Connection = {
  clientId: string;
  issuer: string;
  subject: string;
  accessToken: string;
  refreshToken?: string;
  idToken: string;
  email?: string;
  scopes: string[];
  expiresAt: number;
};

type AuthData = {
  hostId: string;
  generation: number;
  attempts: Record<string, AuthAttempt>;
  issuedClientId?: string;
  connection?: Connection;
};

const locks = new Map<string, Promise<void>>();
const filePath = () => process.env.CLICK_DOT_AUTH_FILE ?? join(homedir(), ".config", "click-dot", "auth.json");
const emptyData = (): AuthData => ({ hostId: `urn:uuid:${randomUUID()}`, generation: 0, attempts: {} });

type LockIdentity = { device: number; inode: number };

async function acquireProcessLock(path: string): Promise<LockIdentity> {
  const lockPath = `${path}.lock`;
  const configured = Number(process.env.CLICK_DOT_AUTH_LOCK_TIMEOUT_MS);
  const timeoutMs = Number.isFinite(configured) && configured > 0 ? Math.min(configured, 30_000) : 5_000;
  const deadline = Date.now() + timeoutMs;
  while (true) {
    try {
      await mkdir(lockPath, { mode: 0o700 });
      const info = await lstat(lockPath);
      if (!info.isDirectory() || info.isSymbolicLink() || info.uid !== uid() || (info.mode & 0o777) !== 0o700) {
        throw new Error("Local account lock is not private.");
      }
      return { device: info.dev, inode: info.ino };
    } catch (error) {
      if (!isAlreadyExists(error)) throw error;
      const info = await lstat(lockPath).catch((statError) => {
        if (isMissing(statError)) return undefined;
        throw statError;
      });
      if (info && (!info.isDirectory() || info.isSymbolicLink() || info.uid !== uid() || (info.mode & 0o777) !== 0o700)) {
        throw new Error("An unsafe local account lock exists; refusing to modify auth state.");
      }
      if (Date.now() >= deadline) {
        throw new Error("Local account lock acquisition timed out. An existing lock was left untouched.");
      }
      await new Promise((resolve) => setTimeout(resolve, 20));
    }
  }
}

function isAlreadyExists(error: unknown) {
  return (error as NodeJS.ErrnoException).code === "EEXIST";
}

async function releaseProcessLock(path: string, expected: LockIdentity) {
  const lockPath = `${path}.lock`;
  const info = await lstat(lockPath);
  if (!info.isDirectory() || info.isSymbolicLink() || info.uid !== uid()
    || info.dev !== expected.device || info.ino !== expected.inode || (info.mode & 0o777) !== 0o700) {
    throw new Error("Local account lock identity changed; refusing to remove it.");
  }
  await rmdir(lockPath);
}

async function withFileLock<T>(path: string, action: () => Promise<T>): Promise<T> {
  const previous = locks.get(path) ?? Promise.resolve();
  let release!: () => void;
  const current = new Promise<void>((resolve) => { release = resolve; });
  locks.set(path, current);
  await previous;
  try {
    await ensurePrivateDirectory(path, true);
    const processLock = await acquireProcessLock(path);
    try {
      return await action();
    } finally {
      await releaseProcessLock(path, processLock);
    }
  } finally {
    release();
    if (locks.get(path) === current) locks.delete(path);
  }
}

function uid() {
  const current = process.getuid?.();
  if (current === undefined) throw new Error("Private local account storage is unavailable on this platform.");
  return current;
}

async function ensurePrivateDirectory(path: string, create: boolean) {
  if (process.platform === "win32") {
    throw new Error("Local account storage is not supported on native Windows until its ACL can be verified.");
  }
  if (!isAbsolute(path)) throw new Error("Local account storage path must be absolute.");
  const dir = dirname(path);
  const absolute = dir.split(sep).filter(Boolean);
  let current = parse(dir).root;
  for (const [index, part] of absolute.entries()) {
    current = join(current, part);
    let info;
    try {
      info = await lstat(current);
    } catch (error) {
      if (!isMissing(error) || !create) {
        if (isMissing(error)) return false;
        throw error;
      }
      await mkdir(current, { mode: 0o700 }).catch((mkdirError) => {
        if (!isAlreadyExists(mkdirError)) throw mkdirError;
      });
      info = await lstat(current);
    }
    if (!info.isDirectory() || info.isSymbolicLink()) throw new Error("Local account storage path contains an unsafe directory.");
    const isFinal = index === absolute.length - 1;
    const stickyTempDirectory = (current === "/tmp" || current === "/var/tmp") && (info.mode & 0o1000) !== 0;
    if ((info.mode & 0o022) !== 0 && !stickyTempDirectory) throw new Error("Local account storage path is writable by another user.");
    if (isFinal && (info.uid !== uid() || (info.mode & 0o777) !== 0o700)) {
      throw new Error("Local account storage directory is not private.");
    }
  }
  return true;
}

function isMissing(error: unknown) {
  return (error as NodeJS.ErrnoException).code === "ENOENT";
}

async function readData(path: string, create: boolean): Promise<AuthData> {
  if (constants.O_NOFOLLOW === undefined) throw new Error("Safe local file access is unavailable on this platform.");
  if (!await ensurePrivateDirectory(path, create)) return emptyData();
  let handle;
  try {
    handle = await open(path, constants.O_RDONLY | (constants.O_NOFOLLOW ?? 0));
  } catch (error) {
    if (isMissing(error)) return emptyData();
    throw error;
  }
  try {
    const info = await handle.stat();
    const pathInfo = await lstat(path);
    if (!info.isFile() || pathInfo.isSymbolicLink() || info.dev !== pathInfo.dev || info.ino !== pathInfo.ino
      || info.nlink !== 1 || info.uid !== uid() || (info.mode & 0o777) !== 0o600) {
      throw new Error("Local account storage file is not private.");
    }
    const data = JSON.parse(await handle.readFile("utf8")) as Partial<AuthData>;
    if (typeof data.hostId !== "string" || !data.attempts || typeof data.attempts !== "object") {
      throw new Error("Local account storage is invalid.");
    }
    return { ...emptyData(), ...data, generation: Number.isSafeInteger(data.generation) ? data.generation! : 0 };
  } finally {
    await handle.close();
  }
}

async function writeData(path: string, data: AuthData) {
  if (constants.O_NOFOLLOW === undefined) throw new Error("Safe local file access is unavailable on this platform.");
  await ensurePrivateDirectory(path, true);
  const temp = `${path}.${randomUUID()}.tmp`;
  const handle = await open(temp, constants.O_WRONLY | constants.O_CREAT | constants.O_EXCL | (constants.O_NOFOLLOW ?? 0), 0o600);
  try {
    const info = await handle.stat();
    if (!info.isFile() || info.nlink !== 1 || info.uid !== uid() || (info.mode & 0o777) !== 0o600) throw new Error("Private local account storage could not be created.");
    await handle.writeFile(JSON.stringify(data), "utf8");
    await handle.sync();
  } finally {
    await handle.close();
  }
  try {
    try {
      const existing = await lstat(path);
      if (!existing.isFile() || existing.isSymbolicLink() || existing.nlink !== 1 || existing.uid !== uid() || (existing.mode & 0o777) !== 0o600) {
        throw new Error("Local account storage file is not private.");
      }
    } catch (error) {
      if (!isMissing(error)) throw error;
    }
    await rename(temp, path);
    await chmod(path, 0o600);
  } catch (error) {
    await unlink(temp).catch(() => undefined);
    throw error;
  }
}

async function transact<T>(action: (data: AuthData) => Promise<T> | T): Promise<T> {
  const path = filePath();
  return withFileLock(path, async () => {
    const data = await readData(path, true);
    const result = await action(data);
    await writeData(path, data);
    return result;
  });
}

export async function createAttempt(id: string, attempt: Omit<AuthAttempt, "expectedClientId" | "expectedSubject" | "generation">, initialClientId: string) {
  return transact((data) => {
    const now = Date.now();
    data.attempts = Object.fromEntries(Object.entries(data.attempts).filter(([, value]) => value.expiresAt > now));
    const expectedClientId = data.issuedClientId ?? initialClientId;
    const fullAttempt: AuthAttempt = {
      ...attempt,
      expectedClientId,
      generation: data.generation,
    };
    if (data.connection?.subject) fullAttempt.expectedSubject = data.connection.subject;
    data.attempts[id] = fullAttempt;
    return { hostId: data.hostId, issuedClientId: data.issuedClientId, attempt: fullAttempt };
  });
}

export async function consumeAttempt(id: string) {
  return transact((data) => {
    const attempt = data.attempts[id];
    delete data.attempts[id];
    return attempt && attempt.expiresAt > Date.now() ? attempt : undefined;
  });
}

export async function saveConnection(connection: Connection, attempt: AuthAttempt) {
  return transact((data) => {
    if (data.generation !== attempt.generation || !data.issuedClientId || data.issuedClientId !== connection.clientId
      || (attempt.expectedClientId !== "dynamic_agent_client" && attempt.expectedClientId !== connection.clientId)
      || (attempt.expectedSubject && attempt.expectedSubject !== connection.subject)
      || (data.connection && (data.connection.clientId !== connection.clientId || data.connection.subject !== connection.subject))) {
      return false;
    }
    data.connection = connection;
    data.generation += 1;
    data.attempts = {};
    return true;
  });
}

export async function saveIssuedClientId(clientId: string, expectedGeneration: number) {
  if (!clientId || clientId !== clientId.trim() || clientId === "dynamic_agent_client") throw new Error("The issued local application ID is invalid.");
  return transact((data) => {
    if (data.generation !== expectedGeneration) return false;
    if (data.issuedClientId && data.issuedClientId !== clientId) return false;
    data.issuedClientId = clientId;
    return true;
  });
}

export async function getIssuedClientId() {
  return (await readData(filePath(), false)).issuedClientId;
}

export async function getConnection() {
  return (await readData(filePath(), false)).connection;
}

export async function clearConnection() {
  return transact((data) => {
    data.connection = undefined;
    data.generation += 1;
    data.attempts = {};
  });
}

export async function getStoragePathForTests() {
  return filePath();
}
