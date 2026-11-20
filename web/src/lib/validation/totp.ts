import * as v from 'valibot';

export const totpCodeSchema = v.pipe(
	v.string(),
	v.regex(/^\d{6}$/, 'Enter a valid six-digit authenticator code.'),
);

export const recoveryCodeSchema = v.pipe(
	v.string(),
	v.regex(/^[a-f\d]{8}(?:-[a-f\d]{8}){3}$/i, 'Enter a valid recovery code.'),
);

export const totpProofSchema = v.union([totpCodeSchema, recoveryCodeSchema]);
