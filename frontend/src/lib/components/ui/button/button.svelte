<script lang="ts">
	import type { HTMLButtonAttributes } from 'svelte/elements';
	import { tv, type VariantProps } from 'tailwind-variants';
	import { cn } from '$lib/utils';

	const buttonVariants = tv({
		base: 'inline-flex items-center justify-center rounded-lg text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-sepia-600 focus-visible:ring-offset-2 focus-visible:ring-offset-sepia-100 dark:focus-visible:ring-sepia-400 dark:focus-visible:ring-offset-sepia-900 disabled:pointer-events-none disabled:opacity-50',
		variants: {
			variant: {
				default: 'bg-sepia-700 text-sepia-50 hover:bg-sepia-600 dark:bg-sepia-300 dark:text-sepia-950 dark:hover:bg-sepia-200',
				outline: 'border border-sepia-500 bg-sepia-100 text-sepia-700 hover:bg-sepia-200 hover:text-sepia-900 dark:border-sepia-600 dark:bg-sepia-800 dark:text-sepia-300 dark:hover:bg-sepia-700 dark:hover:text-sepia-100',
				ghost: 'text-sepia-600 hover:text-sepia-900 hover:bg-sepia-200 dark:text-sepia-400 dark:hover:text-sepia-100 dark:hover:bg-sepia-800',
				destructive: 'bg-red-700 text-white hover:bg-red-600',
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
