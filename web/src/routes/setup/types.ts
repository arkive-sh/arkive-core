export const SETUP_MASTER_KEY_STORAGE = 'arkive:setup-master-key:v1';

export interface SetupStatus {
	initialized: boolean;
	recoveryPending: boolean;
	userID?: string;
}

export type StorageProvider = 'local' | 's3';

export interface InitialSetupRequest {
	name: string;
	username: string;
	email: string;
	opaqueRegistrationRecord: string;
	vaultSalt: string;
	encryptedMasterKey: string;
	storage: {
		provider: StorageProvider;
		localPath: string;
		storageGB: string;
		s3AccessKeyID: string;
		s3SecretAccessKey: string;
		s3SessionToken: string;
		s3Bucket: string;
		s3Endpoint: string;
		s3Region: string;
		s3UsePathStyle: boolean;
	};
}

export interface InitialRecoveryRequest {
	confirmBackup: boolean;
	encryptedMasterKeyRecovery: string;
	recoveryProofPublicKey: string;
}
