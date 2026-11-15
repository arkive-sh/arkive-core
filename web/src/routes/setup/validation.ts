import * as v from 'valibot';

export interface SetupFormValues {
	name: string;
	username: string;
	email: string;
	password: string;
	confirm_password: string;
	storage_gb: string;
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
			storage_gb: v.pipe(
				v.string(),
				v.nonEmpty('storage limit is required'),
				v.regex(/^\d+$/, 'storage limit must be 0 or a positive number'),
			),
			s3_access_key_id: required('access key is required'),
			s3_secret_access_key: required('secret key is required'),
			s3_bucket: required('bucket is required'),
			s3_endpoint: required('endpoint is required'),
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
