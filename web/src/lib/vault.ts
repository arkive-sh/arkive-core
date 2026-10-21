interface LoginVaultModule {
	unlockVault(password: string, salt: string, encryptedMasterKey: string): Promise<unknown>;
	persistSessionUnlock(): Promise<unknown>;
	clearSessionUnlock(): void;
}

// shortcut: reuse the Go UI's vault runtime until its crypto worker is migrated too.
async function loadLoginVault(): Promise<LoginVaultModule> {
	return (await import('../../../core/web/static/features/vault.js')) as unknown as LoginVaultModule;
}

export async function unlockForLogin(password: string, salt: string, encryptedMasterKey: string): Promise<void> {
	const vault = await loadLoginVault();
	await vault.unlockVault(password, salt, encryptedMasterKey);
	await vault.persistSessionUnlock();
}

export async function clearSessionUnlock(): Promise<void> {
	const vault = await loadLoginVault();
	vault.clearSessionUnlock();
}
