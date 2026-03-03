<script lang="ts">
	import { Dialog as DialogPrimitive } from 'bits-ui';

	interface Props {
		children?: import('svelte').Snippet;
	}

	let { children }: Props = $props();
</script>

<DialogPrimitive.Close>
	{@render children?.()}
</DialogPrimitive.Close>
