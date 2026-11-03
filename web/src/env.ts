import { defineEnvVars } from '@sveltejs/kit/env';

export const variables = defineEnvVars({
	CORE_API_ORIGIN: {
		public: true,
		schema: (value) => value || 'http://localhost:8080',
		description: 'Browser-accessible origin of the Arkive Core API',
	},
});
