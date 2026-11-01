import * as opaque from '@serenity-kit/opaque';
import { api } from '#lib/api.ts';
import { fromBase64 } from '#lib/vault/bytes.ts';

interface LoginStartResponse {
	attemptId: string;
	loginResponse: string;
}

interface LoginFinishResponse {
	salt: string;
	encryptedMasterKey: string;
}

interface RegistrationStartResponse {
	registrationResponse: string;
}

interface RegistrationResult {
	registrationRecord: string;
	exportKey: Uint8Array<ArrayBuffer>;
}

export class OpaqueAuth {
	async login(email: string, password: string): Promise<LoginFinishResponse & { exportKey: Uint8Array<ArrayBuffer> }> {
		let clientState: ReturnType<typeof opaque.client.startLogin> | null = opaque.client.startLogin({ password });
		let login: ReturnType<typeof opaque.client.finishLogin> | null = null;
		let encodedExportKey: string | null = null;
		let exportKey: Uint8Array<ArrayBuffer> | null = null;
		try {
			const { data: start } = await api.post<LoginStartResponse>('/api/auth/opaque/login/start', {
				email,
				startLoginRequest: clientState.startLoginRequest
			});
			try {
				login = opaque.client.finishLogin({ clientLoginState: clientState.clientLoginState, loginResponse: start.loginResponse, password });
			} catch {
				throw new Error('Invalid email or password');
			}
			if (!login) throw new Error('Invalid email or password');
			const finishLoginRequest = login.finishLoginRequest;
			encodedExportKey = login.exportKey;
			exportKey = fromBase64(encodedExportKey);
			clientState = null;
			login = null;
			encodedExportKey = null;
			const { data } = await api.post<LoginFinishResponse>('/api/auth/opaque/login/finish', {
				attemptId: start.attemptId,
				finishLoginRequest
			});
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
				const { data } = await api.post<RegistrationStartResponse>('/api/setup/opaque/registration/start', {
					username,
					registrationRequest
				});
				return data.registrationResponse;
			});
		} finally {
			password = '';
		}
	}

	async registerPasswordReset(token: string, password: string): Promise<RegistrationResult> {
		try {
			return await this.register(password, async (registrationRequest) => {
				const { data } = await api.post<RegistrationStartResponse>('/api/auth/password-reset/opaque/start', {
					token,
					registrationRequest
				});
				return data.registrationResponse;
			});
		} finally {
			password = '';
		}
	}

	private async register(
		password: string,
		getRegistrationResponse: (request: string) => Promise<string>
	): Promise<RegistrationResult> {
    let clientState: ReturnType<typeof opaque.client.startRegistration> | null = opaque.client.startRegistration({ password });

    let registration: ReturnType<typeof opaque.client.finishRegistration> | null = null;

		let encodedExportKey: string | null = null;
		let exportKey: Uint8Array<ArrayBuffer> | null = null;
		try {
			const registrationResponse = await getRegistrationResponse(clientState.registrationRequest);
			registration = opaque.client.finishRegistration({
				clientRegistrationState: clientState.clientRegistrationState,
				registrationResponse,
				password
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
