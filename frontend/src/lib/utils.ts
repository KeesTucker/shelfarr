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
// audio → blue  |  ebook → teal  |  metadata → purple  |  image → sepia  |  other → sepia (dimmer)
const FILE_TYPE_CLASSES: Record<string, string> = {
	audio:    'bg-blue-100 text-blue-800 border border-blue-300 dark:bg-blue-950 dark:text-blue-300 dark:border-blue-800',
	ebook:    'bg-teal-100 text-teal-800 border border-teal-300 dark:bg-teal-950 dark:text-teal-300 dark:border-teal-800',
	metadata: 'bg-purple-100 text-purple-800 border border-purple-300 dark:bg-purple-950 dark:text-purple-300 dark:border-purple-800',
	image:    'bg-sepia-200 text-sepia-600 border border-sepia-400 dark:bg-sepia-800 dark:text-sepia-400 dark:border-sepia-700',
	other:    'bg-sepia-200 text-sepia-500 border border-sepia-400 dark:bg-sepia-800 dark:text-sepia-500 dark:border-sepia-700',
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
