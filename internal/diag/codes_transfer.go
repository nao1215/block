package diag

// BLK3xxx: getting the bytes, and proving they are the bytes block.lock names.
var (
	// DownloadFailed is a transfer that did not complete.
	DownloadFailed = register(3001,
		"an artifact could not be downloaded",
		"The artifact block.lock names could not be fetched: the host refused, the transfer broke, or the request timed out. Nothing was installed, and no partial file was kept — a download is written to a temporary file and only named once its digest is known.",
		"Retry. If it persists, check network access to the host in the message; a release CDN blocked by a proxy is the usual cause in CI.",
		"v0.1.0")

	// ChecksumMismatch is a download that is not what was locked.
	ChecksumMismatch = register(3002,
		"a download does not match the digest in block.lock",
		"The bytes at the locked URL hash to something other than the SHA-256 recorded when the pin was made. Nothing is extracted and nothing is installed; the mismatching download is discarded rather than cached. This is the check working.",
		"Run it once more in case a proxy truncated the transfer. If it persists, the artifact at that URL is not the artifact that was locked, which is worth knowing before it is on your PATH — do not \"fix\" it by re-running block lock until you know why the upstream file changed.",
		"v0.1.0")

	// CacheUnusable is a cached blob that is corrupt and cannot be replaced.
	CacheUnusable = register(3003,
		"a cached artifact is corrupt and could not be replaced",
		"The download cache under $BLOCK_HOME holds a blob whose bytes do not hash to the name it is filed under — a truncated download, or a half-restored CI cache. block discards such a blob and fetches again, but this one could not be removed.",
		"Check the permissions on $BLOCK_HOME, then delete the file the message names. Deleting the whole cache directory is always safe: it is rebuilt from block.lock.",
		"v0.1.0")
)
