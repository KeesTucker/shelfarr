import { clsx, type ClassValue } from 'clsx';
import { twMerge } from 'tailwind-merge';

export function cn(...inputs: ClassValue[]) {
	return twMerge(clsx(inputs));
}

export function formatSize(bytes: number): string {
	if (!bytes || bytes <= 0) return '—';
	if (bytes >= 1_073_741_824) return `${(bytes / 1_073_741_824).toFixed(1)} GB`;
	if (bytes >= 1_048_576) return `${(bytes / 1_048_576).toFixed(0)} MB`;
	return `${Math.round(bytes / 1024)} KB`;
}

export function formatDate(iso: string): string {
	return new Date(iso).toLocaleDateString(undefined, {
		year: 'numeric',
		month: 'short',
		day: 'numeric',
	});
}

// Colour classes per file/media category.
// audio → blue  |  ebook → teal  |  metadata → purple  |  image → zinc  |  other → zinc (dimmer)
const FILE_TYPE_CLASSES: Record<string, string> = {
	audio:    'bg-blue-950 text-blue-300 border border-blue-800',
	ebook:    'bg-teal-950 text-teal-300 border border-teal-800',
	metadata: 'bg-purple-950 text-purple-300 border border-purple-800',
	image:    'bg-zinc-800 text-zinc-400 border border-zinc-700',
	other:    'bg-zinc-800 text-zinc-500 border border-zinc-700',
};

/** Return the Tailwind classes for a known file category. */
export function fileTypeClass(category: 'audio' | 'ebook' | 'metadata' | 'image' | 'other'): string {
	return FILE_TYPE_CLASSES[category] ?? FILE_TYPE_CLASSES.other;
}

const EBOOK_FORMATS = new Set(['EPUB', 'PDF', 'MOBI', 'AZW3', 'AZW', 'LIT']);
const AUDIO_FORMATS = new Set(['MP3', 'M4B', 'M4A', 'FLAC', 'AAC', 'OGG', 'OPUS', 'WAV']);

/** Infer a file category from a raw torrent tag string (e.g. "EPUB", "MP3 / ENG") and return its colour classes. */
export function tagColorFromLabel(tag: string): string {
	const first = tag.split(/[\s/]/)[0].toUpperCase();
	if (EBOOK_FORMATS.has(first)) return FILE_TYPE_CLASSES.ebook;
	if (AUDIO_FORMATS.has(first)) return FILE_TYPE_CLASSES.audio;
	return FILE_TYPE_CLASSES.other;
}
