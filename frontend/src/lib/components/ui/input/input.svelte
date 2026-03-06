<script lang="ts">
	import type { HTMLInputAttributes } from 'svelte/elements';
	import { cn } from '$lib/utils';

	interface Props extends HTMLInputAttributes {
		class?: string;
		value?: HTMLInputAttributes['value'];
	}

	let { class: className, value = $bindable(), ...props }: Props = $props();
</script>

<input
	class={cn(
		'w-full rounded-lg border border-sepia-400 bg-sepia-50 px-3 py-2.5 text-sm text-sepia-900 placeholder:text-sepia-400 transition-colors focus:border-sepia-600 focus:outline-none focus:ring-1 focus:ring-sepia-600 disabled:cursor-not-allowed disabled:opacity-50 dark:border-sepia-700 dark:bg-sepia-800 dark:text-sepia-100 dark:placeholder:text-sepia-500 dark:focus:border-sepia-500 dark:focus:ring-sepia-500',
		className,
	)}
	bind:value
	{...props}
/>
