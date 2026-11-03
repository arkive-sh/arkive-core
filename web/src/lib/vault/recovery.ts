import type { VaultWorkerCalls } from './types';
import { callVaultWorker } from './worker-client';

export function preparePasswordReset(
	input: VaultWorkerCalls['preparePasswordReset']['params'],
): Promise<VaultWorkerCalls['preparePasswordReset']['result']> {
	return callVaultWorker('preparePasswordReset', input);
}
