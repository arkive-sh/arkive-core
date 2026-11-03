export function fileLocationHref(fileId: string, folderId: string | null): string {
	const params = new URLSearchParams({ focus_file: fileId });

	if (folderId) params.set('folder_id', folderId);

	return `/?${params}`;
}
