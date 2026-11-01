export type StreamPurpose = 'preview' | 'download';

export interface StreamRecord {
	fileId: string;
}

export interface StreamMetadata {
	name?: string;
	mime?: string;
	size?: number;
}

export interface StreamRegistration {
	sessionId: string;
	purpose: StreamPurpose;
	fileId: string;
	plaintextSize: number;
	mimeType: string;
	concurrency: number;
}

export interface EncryptedStreamOptions {
	reader: StreamReader;
	record: StreamRecord;
	metadata: StreamMetadata;
	purpose?: StreamPurpose;
	concurrency?: number;
}

export interface StreamReader {
	readRange(start: number, end: number, signal?: AbortSignal): Promise<Uint8Array>;
}

export interface EncryptedStream {
	url: string;
	close(): void;
}

export interface StreamProvider {
	reader: StreamReader;
	registration: StreamRegistration;
	reads: Map<string, AbortController>;
}
