import { PDFDataRangeTransport } from 'pdfjs-dist';

export interface PdfRangeSource {
	readRange(begin: number, end: number, signal?: AbortSignal): Promise<Uint8Array>;
}

export class PdfRangeTransport extends PDFDataRangeTransport {
	private readonly requests = new Set<AbortController>();
	private readonly source: PdfRangeSource;
	private readonly onReadError: (error: unknown, begin: number, end: number) => void;

	constructor(
		length: number,
		source: PdfRangeSource,
		onReadError: (error: unknown, begin: number, end: number) => void
	) {
		super(length, new Uint8Array(0));
		this.source = source;
		this.onReadError = onReadError;
	}

	requestDataRange(begin: number, end: number): void {
		const request = new AbortController();
		this.requests.add(request);
		void this.source.readRange(begin, end, request.signal).then((bytes) => {
			if (request.signal.aborted) {
				bytes.fill(0);
				return;
			}
			this.onDataRange(begin, bytes);
		}).catch((error: unknown) => {
			if (!request.signal.aborted) this.onReadError(error, begin, end);
		}).finally(() => this.requests.delete(request));
	}

	abort(): void {
		for (const request of this.requests) request.abort();
		this.requests.clear();
	}
}
