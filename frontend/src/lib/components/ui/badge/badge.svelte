<script lang="ts">
	import { cn } from '$lib/utils';

	const statusColors: Record<string, string> = {
		pending:     'bg-sepia-200 text-sepia-700 dark:bg-sepia-800 dark:text-sepia-300',
		downloading: 'bg-blue-100 text-blue-800 border border-blue-300 dark:bg-blue-950 dark:text-blue-300 dark:border-blue-800',
		importing:   'bg-amber-100 text-amber-800 border border-amber-300 dark:bg-amber-950 dark:text-amber-300 dark:border-amber-800',
		done:        'bg-green-100 text-green-800 border border-green-300 dark:bg-green-950 dark:text-green-300 dark:border-green-800',
		failed:      'bg-red-100 text-red-800 border border-red-300 dark:bg-red-950 dark:text-red-300 dark:border-red-800',
	};

	interface Props {
		status?: string;
		class?: string;
		children?: import('svelte').Snippet;
	}

	let { status, class: className, children }: Props = $props();

	const colorClass = $derived(status ? (statusColors[status] ?? 'bg-sepia-200 text-sepia-600 dark:bg-sepia-800 dark:text-sepia-400') : '');
</script>

<span
	class={cn(
		'inline-block rounded-full px-2.5 py-0.5 text-xs font-medium',
		colorClass,
		className,
	)}
>
	{#if children}
		{@render children()}
	{:else}
		{status}
	{/if}
</span>
