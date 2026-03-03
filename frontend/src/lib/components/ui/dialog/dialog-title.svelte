<script lang="ts">
	import { Dialog as DialogPrimitive } from 'bits-ui';
	import { cn } from '$lib/utils';

	interface Props {
		class?: string;
		children?: import('svelte').Snippet;
	}

	let { class: className, children }: Props = $props();
</script>

<DialogPrimitive.Title class={cn('text-lg font-semibold text-zinc-50', className)}>
	{@render children?.()}
</DialogPrimitive.Title>
