import * as v from 'valibot';
import type { StorageProvider } from './types';

export interface SetupFormValues {
	name: string;
	username: string;
	email: string;
	password: string;
	confirm_password: string;
	storage_provider: StorageProvider;
	storage_gb: string;
	local_path: string;
	s3_access_key_id: string;
	s3_secret_access_key: string;
	s3_bucket: string;
	s3_endpoint: string;
}

const required = (message: string) => v.pipe(v.string(), v.nonEmpty(message));

const password = v.pipe(
	v.string(),
	v.nonEmpty('password is required'),
	v.minBytes(8, 'password must be at least 8 characters'),
	v.regex(/\p{Ll}/u, 'password must include a lowercase letter'),
	v.regex(/\p{Lu}/u, 'password must include an uppercase letter'),
	v.regex(/[\p{P}\p{S}]/u, 'password must include a symbol'),
);

export function validateSetup(values: SetupFormValues) {
	const schema = v.pipe(
		v.object({
			name: v.pipe(
				v.string(),
				v.nonEmpty('name is required'),
				v.minCodePoints(3, 'name must be at least 3 characters'),
			),
			username: v.pipe(
				v.string(),
				v.nonEmpty('username is required'),
				v.minCodePoints(3, 'username must be at least 3 characters'),
				v.maxCodePoints(20, 'username must be at most 20 characters'),
				v.regex(/^[A-Za-z0-9]+$/, 'username may only contain letters and numbers'),
			),
			email: v.pipe(v.string(), v.nonEmpty('email is required'), v.email('email must be valid')),
			password,
			confirm_password: required('confirm password is required'),
			storage_provider: v.picklist(['local', 's3']),
			storage_gb: v.pipe(
				v.string(),
				v.nonEmpty('storage limit is required'),
				v.regex(/^\d+$/, 'storage limit must be 0 or a positive number'),
			),
			local_path:
				values.storage_provider === 'local'
					? required('local storage path is required')
					: v.string(),
			s3_access_key_id:
				values.storage_provider === 's3' ? required('access key is required') : v.string(),
			s3_secret_access_key:
				values.storage_provider === 's3' ? required('secret key is required') : v.string(),
			s3_bucket: values.storage_provider === 's3' ? required('bucket is required') : v.string(),
			s3_endpoint: values.storage_provider === 's3' ? required('endpoint is required') : v.string(),
		}),
		v.forward(
			v.partialCheck(
				[['password'], ['confirm_password']],
				(input) => input.password === input.confirm_password,
				'passwords do not match',
			),
			['confirm_password'],
		),
	);

	return v.safeParse(schema, values);
}
