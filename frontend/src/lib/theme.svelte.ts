function createTheme() {
	let dark = $state(
		typeof document !== 'undefined'
			? document.documentElement.classList.contains('dark')
			: false
	);

	function init() {
		try {
			const stored = localStorage.getItem('theme');
			dark = stored === 'dark' || (!stored && window.matchMedia('(prefers-color-scheme: dark)').matches);
		} catch {}
		apply();
	}

	function toggle() {
		dark = !dark;
		try {
			localStorage.setItem('theme', dark ? 'dark' : 'light');
		} catch {}
		apply();
	}

	function apply() {
		document.documentElement.classList.toggle('dark', dark);
	}

	return {
		get dark() { return dark; },
		init,
		toggle,
	};
}

export const theme = createTheme();
