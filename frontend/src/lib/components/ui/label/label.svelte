<script lang="ts">
	import type { HTMLLabelAttributes } from 'svelte/elements';
	import { cn } from '$lib/utils';

	interface Props extends HTMLLabelAttributes {
		class?: string;
	}

	let { class: className, children, ...props }: Props = $props();
</script>

<label class={cn('block text-sm font-medium text-sepia-700 dark:text-sepia-300', className)} {...props}>
	{@render children?.()}
</label>
