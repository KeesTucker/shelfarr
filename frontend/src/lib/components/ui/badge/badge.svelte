<script lang="ts">
	import { cn } from '$lib/utils';

	const statusColors: Record<string, string> = {
		pending: 'bg-zinc-800 text-zinc-300',
		downloading: 'bg-blue-950 text-blue-300 border border-blue-800',
		moving: 'bg-amber-950 text-amber-300 border border-amber-800',
		done: 'bg-green-950 text-green-300 border border-green-800',
		failed: 'bg-red-950 text-red-300 border border-red-800',
	};

	interface Props {
		status?: string;
		class?: string;
		children?: import('svelte').Snippet;
	}

	let { status, class: className, children }: Props = $props();

	const colorClass = $derived(status ? (statusColors[status] ?? 'bg-zinc-800 text-zinc-400') : '');
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
