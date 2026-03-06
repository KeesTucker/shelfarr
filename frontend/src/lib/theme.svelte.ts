function createTheme() {
	let dark = $state(false);

	function init() {
		dark = localStorage.getItem('theme') === 'dark';
		apply();
	}

	function toggle() {
		dark = !dark;
		localStorage.setItem('theme', dark ? 'dark' : 'light');
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
