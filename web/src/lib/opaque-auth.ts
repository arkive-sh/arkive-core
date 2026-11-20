import * as opaque from '@serenity-kit/opaque';
import axios from 'axios';
import { api } from '#lib/api.ts';
import { fromBase64 } from '#lib/vault/bytes.ts';

interface LoginStartResponse {
	attemptId: string;
	loginResponse: string;
}

export interface LoginFinishResponse {
	userID: string;
	salt: string;
	encryptedMasterKey: string;
	recoveryCodes?: string[];
}

export interface TOTPSetup {
	secret: string;
	qrCodeDataURL: string;
}

export interface MFAChallenge {
	challenge: string;
	totpSetup?: TOTPSetup;
}

export interface MFAProof {
	code?: string;
	recoveryCode?: string;
	rememberDevice?: boolean;
}

interface RegistrationStartResponse {
	registrationResponse: string;
}

interface RegistrationResult {
	registrationRecord: string;
	exportKey: Uint8Array<ArrayBuffer>;
}

export class OpaqueAuth {
	async login(
		email: string,
		password: string,
		requestMFA: (challenge: MFAChallenge, invalidCode: boolean) => Promise<MFAProof>,
	): Promise<LoginFinishResponse & { exportKey: Uint8Array<ArrayBuffer> }> {
		let clientState: ReturnType<typeof opaque.client.startLogin> | null = opaque.client.startLogin({
			password,
		});
		let login: ReturnType<typeof opaque.client.finishLogin> | null = null;
		let encodedExportKey: string | null = null;
		let exportKey: Uint8Array<ArrayBuffer> | null = null;

		try {
			const { data: start } = await api.post<LoginStartResponse>('/api/auth/opaque/login/start', {
				email,
				startLoginRequest: clientState.startLoginRequest,
			});

			try {
				login = opaque.client.finishLogin({
					clientLoginState: clientState.clientLoginState,
					loginResponse: start.loginResponse,
					password,
				});
			} catch {
				throw new Error('Invalid email or password');
			}
			if (!login) throw new Error('Invalid email or password');
			const finishLoginRequest = login.finishLoginRequest;
			encodedExportKey = login.exportKey;
			exportKey = fromBase64(encodedExportKey);
			password = '';
			clientState = null;
			login = null;
			encodedExportKey = null;
			const { data: finish } = await api.post<
				LoginFinishResponse | { mfaRequired: true; challenge: string; totpSetup?: TOTPSetup }
			>('/api/auth/opaque/login/finish', {
				attemptId: start.attemptId,
				finishLoginRequest,
			});
			let data: LoginFinishResponse;

			if ('mfaRequired' in finish) {
				const challenge = { challenge: finish.challenge, totpSetup: finish.totpSetup };
				let invalidCode = false;

				while (true) {
					const proof = await requestMFA(challenge, invalidCode);

					try {
						({ data } = await api.post<LoginFinishResponse>('/api/auth/totp/verify', {
							challenge: challenge.challenge,
							...proof,
						}));
						break;
					} catch (error) {
						if (!axios.isAxiosError(error) || error.response?.status !== 401) throw error;
						invalidCode = true;
					}
				}
			} else {
				data = finish;
			}
			const result = { ...data, exportKey };
			exportKey = null;

			return result;
		} finally {
			password = '';
			if (exportKey?.byteLength) exportKey.fill(0);
			exportKey = null;
			encodedExportKey = null;
			login = null;
			clientState = null;
		}
	}

	async registerSetup(username: string, password: string): Promise<RegistrationResult> {
		try {
			return await this.register(password, async (registrationRequest) => {
				const { data } = await api.post<RegistrationStartResponse>(
					'/api/setup/opaque/registration/start',
					{
						username,
						registrationRequest,
					},
				);

				return data.registrationResponse;
			});
		} finally {
			password = '';
		}
	}

	async registerPasswordReset(token: string, password: string): Promise<RegistrationResult> {
		try {
			return await this.register(password, async (registrationRequest) => {
				const { data } = await api.post<RegistrationStartResponse>(
					'/api/auth/password-reset/opaque/start',
					{
						token,
						registrationRequest,
					},
				);

				return data.registrationResponse;
			});
		} finally {
			password = '';
		}
	}

	async registerSharePassword(shareID: string, password: string): Promise<void> {
		let registration: RegistrationResult | null = null;

		try {
			registration = await this.register(password, async (registrationRequest) => {
				const path = `/api/shares/${encodeURIComponent(shareID)}/password/opaque/start`;
				const { data } = await api.post<RegistrationStartResponse>(path, { registrationRequest });

				return data.registrationResponse;
			});
			await api.post(`/api/shares/${encodeURIComponent(shareID)}/password/opaque/finish`, {
				registrationRecord: registration.registrationRecord,
			});
		} finally {
			password = '';
			registration?.exportKey.fill(0);
			registration = null;
		}
	}

	async createSharePasswordRegistration(token: string, password: string): Promise<string> {
		let registration: RegistrationResult | null = null;

		try {
			registration = await this.register(password, async (registrationRequest) => {
				const { data } = await api.post<RegistrationStartResponse>(
					'/api/shares/password/opaque/start',
					{ token, registrationRequest },
				);

				return data.registrationResponse;
			});

			return registration.registrationRecord;
		} finally {
			password = '';
			registration?.exportKey.fill(0);
			registration = null;
		}
	}

	async loginShare(token: string, password: string): Promise<void> {
		let clientState: ReturnType<typeof opaque.client.startLogin> | null = opaque.client.startLogin({
			password,
		});
		let login: ReturnType<typeof opaque.client.finishLogin> | null = null;
		let exportKey: Uint8Array<ArrayBuffer> | null = null;

		try {
			const base = `/api/public/shares/${encodeURIComponent(token)}/password/opaque`;
			const { data: start } = await api.post<LoginStartResponse>(`${base}/start`, {
				startLoginRequest: clientState.startLoginRequest,
			});

			try {
				login = opaque.client.finishLogin({
					clientLoginState: clientState.clientLoginState,
					loginResponse: start.loginResponse,
					password,
				});
			} catch {
				throw new Error('Password is incorrect.');
			}
			if (!login) throw new Error('Password is incorrect.');
			const finishLoginRequest = login.finishLoginRequest;
			exportKey = fromBase64(login.exportKey);
			clientState = null;
			login = null;
			await api.post(`${base}/finish`, { attemptId: start.attemptId, finishLoginRequest });
		} catch (cause) {
			if (axios.isAxiosError(cause) && cause.response?.status === 401) {
				throw new Error('Password is incorrect.', { cause });
			}

			throw cause;
		} finally {
			password = '';
			exportKey?.fill(0);
			exportKey = null;
			login = null;
			clientState = null;
		}
	}

	private async register(
		password: string,
		getRegistrationResponse: (request: string) => Promise<string>,
	): Promise<RegistrationResult> {
		let clientState: ReturnType<typeof opaque.client.startRegistration> | null =
			opaque.client.startRegistration({ password });

		let registration: ReturnType<typeof opaque.client.finishRegistration> | null = null;

		let encodedExportKey: string | null = null;
		let exportKey: Uint8Array<ArrayBuffer> | null = null;

		try {
			const registrationResponse = await getRegistrationResponse(clientState.registrationRequest);
			registration = opaque.client.finishRegistration({
				clientRegistrationState: clientState.clientRegistrationState,
				registrationResponse,
				password,
			});
			const registrationRecord = registration.registrationRecord;
			encodedExportKey = registration.exportKey;
			exportKey = fromBase64(encodedExportKey);
			registration = null;
			clientState = null;
			encodedExportKey = null;
			const result = { registrationRecord, exportKey };
			exportKey = null;

			return result;
		} finally {
			password = '';
			if (exportKey?.byteLength) exportKey.fill(0);
			exportKey = null;
			encodedExportKey = null;
			registration = null;
			clientState = null;
		}
	}
}

export const opaqueAuth = new OpaqueAuth();
