import { readonly, writable } from 'svelte/store';

const unlocked = writable(false);
export const vaultUnlocked = readonly(unlocked);

export function setVaultUnlocked(value: boolean): void {
	if (typeof window !== 'undefined') unlocked.set(value);
}
