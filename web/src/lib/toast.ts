export type ToastType = 'success' | 'warning' | 'error' | 'info';

export type ToastOptions = {
	title?: string;
	duration?: number;
};

export type ToastDetail = ToastOptions & {
	message: string;
	type: ToastType;
};

export const TOAST_EVENT = 'arkive:toast';

function show(message: string, type: ToastType, options: ToastOptions = {}): void {
	if (typeof window === 'undefined' || !message) return;
	window.dispatchEvent(
		new CustomEvent<ToastDetail>(TOAST_EVENT, {
			detail: { message, type, ...options },
		}),
	);
}

export const toast = {
	show: (message: string, type: ToastType = 'info', options?: ToastOptions) =>
		show(message, type, options),
	success: (message: string, options?: ToastOptions) => show(message, 'success', options),
	warning: (message: string, options?: ToastOptions) => show(message, 'warning', options),
	error: (message: string, options?: ToastOptions) => show(message, 'error', options),
	info: (message: string, options?: ToastOptions) => show(message, 'info', options),
};
