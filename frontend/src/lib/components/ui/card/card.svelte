<script lang="ts">
	import { cn } from '$lib/utils';

	interface Props {
		class?: string;
		children?: import('svelte').Snippet;
	}

	let { class: className, children }: Props = $props();
</script>

<div
	class={cn('rounded-xl border border-zinc-800 bg-zinc-900 shadow-2xl', className)}
>
	{@render children?.()}
</div>
