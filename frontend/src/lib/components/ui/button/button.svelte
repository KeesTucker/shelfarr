<script lang="ts">
	import type { HTMLButtonAttributes } from 'svelte/elements';
	import { tv, type VariantProps } from 'tailwind-variants';
	import { cn } from '$lib/utils';

	const buttonVariants = tv({
		base: 'inline-flex items-center justify-center rounded-lg text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-zinc-300 focus-visible:ring-offset-2 focus-visible:ring-offset-zinc-900 disabled:pointer-events-none disabled:opacity-50',
		variants: {
			variant: {
				default: 'bg-zinc-50 text-zinc-900 hover:bg-zinc-200',
				outline:
					'border border-zinc-600 bg-zinc-800 text-zinc-300 hover:bg-zinc-700 hover:text-zinc-100',
				ghost: 'text-zinc-400 hover:text-zinc-100 hover:bg-zinc-800',
				destructive: 'bg-red-600 text-white hover:bg-red-700',
			},
			size: {
				default: 'px-4 py-2',
				sm: 'px-3 py-1.5 text-xs',
				icon: 'h-9 w-9',
			},
		},
		defaultVariants: {
			variant: 'default',
			size: 'default',
		},
	});

	type ButtonVariants = VariantProps<typeof buttonVariants>;

	interface Props extends HTMLButtonAttributes {
		variant?: ButtonVariants['variant'];
		size?: ButtonVariants['size'];
		class?: string;
	}

	let { class: className, variant, size, children, ...props }: Props = $props();
</script>

<button class={cn(buttonVariants({ variant, size }), className)} {...props}>
	{@render children?.()}
</button>
