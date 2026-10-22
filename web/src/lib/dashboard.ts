export interface DashboardRecentFile {
	id: string;
	vaultId: string;
	encryptedMetadata: string;
	encryptedFileKey: string;
	updatedAt: string;
}

export interface DashboardResponse {
	totalFiles: number;
	usedBytes: number;
	maxStorageBytes: number;
	recentFiles: DashboardRecentFile[];
}
