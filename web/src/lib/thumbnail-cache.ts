const DATABASE_NAME = 'arkive-thumbnail-cache';
const STORE_NAME = 'thumbnails';
const DATABASE_VERSION = 1;
const MAX_ENTRY_AGE_MS = 60 * 24 * 60 * 60 * 1000;

interface ThumbnailRecord {
	cacheKey: string;
	fileId: string;
	lastAccessedAt: number;
	bytes: Uint8Array | ArrayBuffer;
}

let databasePromise: Promise<IDBDatabase | null> | null = null;

function cacheKey(fileId: string, thumbnailVersion: number, thumbnailSize: number): string {
	return [fileId, String(thumbnailVersion), String(thumbnailSize)].join(':');
}

function openDatabase(): Promise<IDBDatabase | null> {
	if (!databasePromise) {
		if (typeof indexedDB === 'undefined') return Promise.resolve(null);

		databasePromise = new Promise<IDBDatabase>((resolve, reject) => {
			const request = indexedDB.open(DATABASE_NAME, DATABASE_VERSION);
			request.onupgradeneeded = () => {
				const database = request.result;
				const store = database.objectStoreNames.contains(STORE_NAME)
					? request.transaction!.objectStore(STORE_NAME)
					: database.createObjectStore(STORE_NAME, { keyPath: 'cacheKey' });

				if (!store.indexNames.contains('lastAccessedAt')) {
					store.createIndex('lastAccessedAt', 'lastAccessedAt', { unique: false });
				}
			};
			request.onsuccess = () => {
				const database = request.result;
				database.onversionchange = () => {
					database.close();
					databasePromise = null;
				};
				resolve(database);
			};
			request.onerror = () => reject(request.error ?? new Error('Thumbnail cache unavailable'));
		}).catch(() => {
			databasePromise = null;

			return null;
		});
	}

	return databasePromise;
}

function requestResult<T>(request: IDBRequest<T>): Promise<T> {
	return new Promise((resolve, reject) => {
		request.onsuccess = () => resolve(request.result);
		request.onerror = () => reject(request.error ?? new Error('Thumbnail cache request failed'));
	});
}

function transactionDone(transaction: IDBTransaction): Promise<void> {
	return new Promise((resolve, reject) => {
		transaction.oncomplete = () => resolve();
		transaction.onabort = () =>
			reject(transaction.error ?? new Error('Thumbnail cache transaction aborted'));
		transaction.onerror = () =>
			reject(transaction.error ?? new Error('Thumbnail cache transaction failed'));
	});
}

async function evictExpired(database: IDBDatabase, now: number): Promise<void> {
	const transaction = database.transaction(STORE_NAME, 'readwrite');
	const done = transactionDone(transaction);
	const request = transaction
		.objectStore(STORE_NAME)
		.index('lastAccessedAt')
		.openCursor(IDBKeyRange.upperBound(now - MAX_ENTRY_AGE_MS));
	request.onsuccess = () => {
		const cursor = request.result;

		if (!cursor) return;

		cursor.delete();
		cursor.continue();
	};

	await done;
}

export async function getCachedThumbnail(
	fileId: string,
	thumbnailVersion: number,
	thumbnailSize: number,
): Promise<Uint8Array | null> {
	const database = await openDatabase();

	if (!database) return null;

	let transaction: IDBTransaction;

	try {
		transaction = database.transaction(STORE_NAME, 'readwrite');
	} catch {
		return null;
	}

	const done = transactionDone(transaction);

	try {
		const store = transaction.objectStore(STORE_NAME);
		const record = (await requestResult(
			store.get(cacheKey(fileId, thumbnailVersion, thumbnailSize)),
		)) as ThumbnailRecord | undefined;

		if (!record) {
			await done;

			return null;
		}

		const now = Date.now();

		if (now - record.lastAccessedAt > MAX_ENTRY_AGE_MS) {
			store.delete(record.cacheKey);
			await done;

			return null;
		}

		record.lastAccessedAt = now;
		store.put(record);
		await done;

		const bytes = record.bytes instanceof Uint8Array ? record.bytes : new Uint8Array(record.bytes);

		return bytes.slice();
	} catch {
		await done.catch(() => {});

		return null;
	}
}

export async function cacheThumbnail(
	fileId: string,
	thumbnailVersion: number,
	thumbnailSize: number,
	encryptedBytes: Uint8Array,
): Promise<void> {
	const database = await openDatabase();

	if (!database || !encryptedBytes.length) return;

	const now = Date.now();

	try {
		await evictExpired(database, now);

		const transaction = database.transaction(STORE_NAME, 'readwrite');
		const done = transactionDone(transaction);
		transaction.objectStore(STORE_NAME).put({
			cacheKey: cacheKey(fileId, thumbnailVersion, thumbnailSize),
			fileId,
			lastAccessedAt: now,
			bytes: encryptedBytes.slice(),
		} satisfies ThumbnailRecord);
		await done;
	} catch {
		// Thumbnail caching is optional; storage failures should not block previews.
	}
}

export async function deleteCachedThumbnails(fileIds: readonly string[]): Promise<void> {
	const database = await openDatabase();

	if (!database || !fileIds.length) return;

	const wanted = new Set(fileIds.map((fileId) => fileId.trim()).filter(Boolean));

	if (!wanted.size) return;

	let transaction: IDBTransaction;

	try {
		transaction = database.transaction(STORE_NAME, 'readwrite');
	} catch {
		return;
	}

	const done = transactionDone(transaction);

	try {
		const store = transaction.objectStore(STORE_NAME);
		const request = store.openCursor();
		request.onsuccess = () => {
			const cursor = request.result;

			if (!cursor) return;

			const record = cursor.value as ThumbnailRecord;

			if (wanted.has(record.fileId)) cursor.delete();
			cursor.continue();
		};

		await done;
	} catch {
		await done.catch(() => {});
		// Cache cleanup is best effort and must not block file actions.
	}
}
