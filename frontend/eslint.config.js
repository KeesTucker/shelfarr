import js from '@eslint/js';
import ts from '@typescript-eslint/eslint-plugin';
import tsParser from '@typescript-eslint/parser';
import svelte from 'eslint-plugin-svelte';
import svelteParser from 'svelte-eslint-parser';
import globals from 'globals';

const svelteRunes = {
	$state: 'readonly',
	$derived: 'readonly',
	$effect: 'readonly',
	$props: 'readonly',
	$bindable: 'readonly',
	$inspect: 'readonly',
	$host: 'readonly',
};

/** @type {import('eslint').Linter.Config[]} */
export default [
	js.configs.recommended,
	{
		files: ['**/*.ts', '**/*.svelte.ts'],
		languageOptions: {
			parser: tsParser,
			globals: { ...globals.browser, ...svelteRunes },
		},
		plugins: { '@typescript-eslint': ts },
		rules: {
			...ts.configs.recommended.rules,
			'no-undef': 'off', // TypeScript handles this
			'no-empty': ['error', { allowEmptyCatch: true }],
		},
	},
	{
		files: ['**/*.svelte'],
		languageOptions: {
			parser: svelteParser,
			parserOptions: {
				parser: tsParser,
			},
			globals: { ...globals.browser, ...svelteRunes },
		},
		plugins: { svelte, '@typescript-eslint': ts },
		rules: {
			...svelte.configs.recommended.rules,
			...ts.configs.recommended.rules,
			'no-undef': 'off',
			'no-empty': ['error', { allowEmptyCatch: true }],
		},
	},
	{
		ignores: ['.svelte-kit/**', 'build/**', 'node_modules/**'],
	},
];
