<script lang="ts">
	import {
		AppWindow,
		AudioLines,
		File as FileIcon,
		FileArchive,
		FileCode,
		FileImage,
		FileSpreadsheet,
		FileTerminal,
		FileText,
		FileType,
		FileVideoCamera,
		Presentation,
		ScrollText,
	} from '@lucide/svelte';
	import { isImageFile } from '#lib/utils/image.ts';

	type Props = {
		name: string;
		mime: string;
		className?: string;
		style?: string;
	};

	let { name, mime, className = '', style = '' }: Props = $props();

	function iconForFile(fileName: string, contentType: string) {
		const extension = fileName.includes('.')
			? fileName.slice(fileName.lastIndexOf('.')).toLowerCase()
			: '';
		const type = contentType.toLowerCase().split(';', 1)[0].trim();

		if (extension === '.pdf' || type === 'application/pdf')
			return { component: FileType, color: 'text-red-400' };
		if (
			['.doc', '.docx', '.odt', '.rtf'].includes(extension) ||
			[
				'application/msword',
				'application/rtf',
				'application/vnd.oasis.opendocument.text',
				'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
			].includes(type)
		)
			return { component: FileText, color: 'text-blue-400' };
		if (
			['.ppt', '.pptx', '.odp', '.key'].includes(extension) ||
			[
				'application/vnd.ms-powerpoint',
				'application/vnd.oasis.opendocument.presentation',
				'application/vnd.openxmlformats-officedocument.presentationml.presentation',
			].includes(type)
		)
			return { component: Presentation, color: 'text-orange-400' };
		if (
			['.csv', '.xls', '.xlsx', '.ods'].includes(extension) ||
			[
				'application/vnd.ms-excel',
				'application/vnd.oasis.opendocument.spreadsheet',
				'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
				'text/csv',
			].includes(type)
		)
			return { component: FileSpreadsheet, color: 'text-green-400' };
		if (
			['.sh', '.bash', '.zsh', '.fish', '.ps1', '.bat', '.cmd', '.ksh'].includes(extension) ||
			['application/x-sh', 'application/x-shellscript', 'text/x-shellscript'].includes(type)
		)
			return { component: FileTerminal, color: 'text-amber-400' };
		if (
			['.exe', '.msi', '.app', '.apk', '.deb', '.rpm', '.dmg', '.dll'].includes(extension) ||
			[
				'application/vnd.microsoft.portable-executable',
				'application/x-msdownload',
				'application/x-executable',
				'application/x-mach-binary',
				'application/vnd.android.package-archive',
			].includes(type)
		)
			return { component: AppWindow, color: 'text-purple-400' };
		if (
			['.zip', '.rar', '.7z', '.tar', '.gz', '.bz2', '.xz', '.zst'].includes(extension) ||
			type.includes('zip') ||
			type.includes('compressed') ||
			type.includes('archive')
		)
			return { component: FileArchive, color: 'text-yellow-400' };
		if (isImageFile(fileName, type)) return { component: FileImage, color: 'text-cyan-400' };
		if (type.startsWith('video/')) return { component: FileVideoCamera, color: 'text-pink-400' };
		if (type.startsWith('audio/')) return { component: AudioLines, color: 'text-indigo-400' };
		if (extension === '.log') return { component: ScrollText, color: 'text-amber-300' };
		if (
			[
				'.c',
				'.cc',
				'.cpp',
				'.cs',
				'.css',
				'.go',
				'.h',
				'.hpp',
				'.html',
				'.java',
				'.js',
				'.jsx',
				'.json',
				'.kt',
				'.md',
				'.php',
				'.py',
				'.rs',
				'.sql',
				'.swift',
				'.toml',
				'.ts',
				'.tsx',
				'.xml',
				'.yaml',
				'.yml',
			].includes(extension) ||
			[
				'application/json',
				'application/xml',
				'text/html',
				'text/markdown',
				'text/x-python',
			].includes(type)
		)
			return { component: FileCode, color: 'text-violet-400' };
		if (type.startsWith('text/')) return { component: FileText, color: 'text-sky-300' };

		return { component: FileIcon, color: 'text-(--muted)' };
	}

	const icon = $derived(iconForFile(name, mime));
	let Icon = $derived(icon.component);
</script>

<Icon class={`${className} ${icon.color}`} {style} aria-hidden="true" />
