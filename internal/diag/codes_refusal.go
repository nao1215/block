package diag

// BLK6xxx: refusals on security grounds. These are not failures to do
// something; they are block declining to. Each one is a boundary block holds
// whatever an upstream, a recipe or a hand-edited lockfile asks for.
var (
	// InsecureURL is a URL block will not fetch.
	InsecureURL = register(6001,
		"the URL is not one block will fetch",
		"Artifacts are fetched over HTTPS and nothing else. Plain HTTP is accepted only for loopback addresses, so an offline test server can stand in for GitHub. The rule is held on every redirect hop too: an https URL that redirects to plain http is refused rather than followed.",
		"Use an https URL. If an upstream publishes over plain http only, it is not a source block can verify the origin of, and mirroring it somewhere with TLS is the fix.",
		"v0.1.0")

	// PathEscape is an archive member, or a lockfile value, that would be
	// written outside where it belongs.
	PathEscape = register(6002,
		"an entry would be written outside its directory",
		"An archive member, an executable path, or a value from block.lock resolves to somewhere other than the directory it is allowed to touch — an absolute path, a \"..\" component, a drive letter or a UNC path. block refuses the whole artifact rather than the one entry, because an archive that tries this is not an archive to be picky about.",
		"Do not hand-edit block.lock. For a `[tools.<name>.source]` of your own, bin entries are relative, slash-separated paths inside the archive. For a registry tool or a real upstream artifact, report it — an upstream shipping such an archive is a finding.",
		"v0.1.0")

	// UnsupportedEntry is an archive member that is not a file or a directory.
	UnsupportedEntry = register(6003,
		"an archive contains an entry block will not extract",
		"block extracts regular files and directories, and nothing else. Symbolic links, hard links, device nodes, sockets and FIFOs are refused: a link is a way to reach outside the install directory after the path check has already passed, and the rest have no place in a tool distribution.",
		"For a `[tools.<name>.source]` of your own, take an artifact the upstream publishes without links. For a registry tool, report it at https://github.com/nao1215/block-registry/issues.",
		"v0.1.0")

	// DuplicateEntry is an archive writing one file twice.
	DuplicateEntry = register(6006,
		"an archive writes the same file twice",
		"Two members of the archive resolve to one file, so what ends up on disk would depend on which of them was extracted last. Names differing only in case are the usual cause: they are two files on Linux and one on macOS and Windows, and an archive that relies on the difference installs differently on different machines.",
		"For a `[tools.<name>.source]` of your own, take an artifact whose members are distinct. For a registry tool, report it at https://github.com/nao1215/block-registry/issues.",
		"v0.1.0")

	// ArchiveTooLarge is a member that would fill the disk.
	ArchiveTooLarge = register(6004,
		"an archive member is larger than block will extract",
		"A single member of the archive exceeds the size block unpacks. A compressed archive can be very much smaller than what it expands to, so the limit is applied while writing rather than trusted from the header.",
		"Take the artifact apart yourself and check what is in it. No tool block installs comes close to this limit.",
		"v0.1.0")

	// UnsafeStorePath is a name or version that would escape the store.
	UnsafeStorePath = register(6005,
		"a name or version from block.lock is not a path component",
		"A tool's name and version become a directory under $BLOCK_HOME, and that directory is one block creates, populates and removes. A value carrying a separator, a \"..\", a NUL or anything but the closed alphabet a version is written in is refused before it reaches the filesystem.",
		"Do not hand-edit block.lock. Restore it from version control, or delete it and run \"block lock\" to write it again.",
		"v0.1.0")
)
