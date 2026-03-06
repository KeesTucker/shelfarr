<script lang="ts">
	import { cn } from '$lib/utils';

	interface Props {
		class?: string;
		children?: import('svelte').Snippet;
	}

	let { class: className, children }: Props = $props();
</script>

<div
	class={cn('rounded-xl border border-sepia-400 bg-sepia-100 shadow-md dark:border-sepia-700 dark:bg-sepia-900', className)}
>
	{@render children?.()}
</div>
