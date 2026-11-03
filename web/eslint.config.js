import js from '@eslint/js';
import eslintConfigPrettier from 'eslint-config-prettier/flat';
import globals from 'globals';
import stylistic from '@stylistic/eslint-plugin';
import svelte from 'eslint-plugin-svelte';
import tseslint from 'typescript-eslint';
import { defineConfig } from 'eslint/config';

export default defineConfig([
	{
		ignores: ['.svelte-kit/**', 'build/**', 'dist/**', 'node_modules/**', 'static/wasm/**'],
	},
	js.configs.recommended,
	...tseslint.configs.recommended,
	...svelte.configs.recommended,
	{
		languageOptions: {
			globals: {
				...globals.browser,
				...globals.node,
			},
			parserOptions: {
				extraFileExtensions: ['.svelte'],
			},
		},
	},
	{
		files: ['**/*.svelte', '**/*.svelte.js', '**/*.svelte.ts'],
		languageOptions: {
			parserOptions: {
				parser: tseslint.parser,
			},
		},
	},
	{
		rules: {
			// Clearing sensitive references in finally blocks is intentional cleanup.
			'no-useless-assignment': 'off',
			'no-empty': ['error', { allowEmptyCatch: true }],
		},
	},
	...svelte.configs.prettier,
	eslintConfigPrettier,
	{
		plugins: {
			'@stylistic': stylistic,
		},
		rules: {
			'@stylistic/padding-line-between-statements': [
				'error',
				{ blankLine: 'always', prev: '*', next: ['return', 'throw'] },
				{ blankLine: 'always', prev: 'import', next: '*' },
				{ blankLine: 'never', prev: 'import', next: 'import' },
				{
					blankLine: 'always',
					prev: ['const', 'let', 'var'],
					next: ['if', 'switch', 'for', 'while', 'try'],
				},
			],
			'@stylistic/lines-between-class-members': ['error', 'always'],
		},
	},
]);
