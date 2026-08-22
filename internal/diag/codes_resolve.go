package diag

// BLK2xxx: resolving a version against an upstream. Everything here happens
// inside `block lock`, which is the only command that talks to an upstream at
// all. sync and exec never reach these codes.
var (
	// NoMatchingVersion is a constraint no published tag satisfies.
	NoMatchingVersion = register(2001,
		"no upstream version matches the constraint",
		"The upstream's tags were read, and none of them is a release version the constraint in block.toml allows. Pre-releases never satisfy a constraint, so a line that has only ever shipped release candidates matches nothing.",
		"Widen the constraint in block.toml, or check the upstream's releases page for how the line is actually spelled.",
		"v0.1.0")

	// NoPublishedRelease is a matching tag with no release behind it.
	NoPublishedRelease = register(2002,
		"the matching tags have no published release",
		"Tags satisfying the constraint exist, but the newest of them are drafts, pre-releases, or tags pushed before their release was published. block only pins a published, non-draft, non-pre-release release.",
		"Wait for the release to be published, or pin a version that already has one.",
		"v0.1.0")

	// UpstreamNotFound is a repository, tag or release that is not there.
	UpstreamNotFound = register(2003,
		"the upstream repository, tag or release does not exist",
		"The upstream answered \"not found\". The repository has been renamed, deleted, or made private, or the tag the recipe renders for this version was never pushed.",
		"Check the repo in the recipe or in `[tools.<name>.source]`. A repository that moved needs the new owner/name; a private one needs a GITHUB_TOKEN that can see it.",
		"v0.1.0")

	// AssetMissing is a release that does not carry the file the recipe names.
	AssetMissing = register(2004,
		"the release does not carry the expected asset",
		"The release was found, but it publishes no file with the name the recipe renders for this version and platform. Upstreams rename their assets, and a recipe is a rule about those names. The message lists what the release does carry.",
		"For a registry tool, report it at https://github.com/nao1215/block-registry/issues — the recipe needs updating. For a `[tools.<name>.source]` of your own, correct the asset template against the names in the message.",
		"v0.1.0")

	// AmbiguousAsset is a release publishing the same file name twice.
	AmbiguousAsset = register(2008,
		"the release carries the asset name more than once",
		"The release publishes several files with the name the recipe renders. They are different downloads, and which of them a lockfile pinned would depend on the order the API happened to answer in, so block refuses rather than choosing. The message lists their URLs.",
		"For a registry tool, report it at https://github.com/nao1215/block-registry/issues — the asset template needs to be specific enough to name one file. For a `[tools.<name>.source]` of your own, make the template unambiguous.",
		"v0.1.0")

	// RateLimited is the GitHub API refusing to answer any more.
	RateLimited = register(2005,
		"the GitHub API rate limit was reached",
		"block lock reads tags and releases from the GitHub API, which allows 60 requests an hour to an unauthenticated client. sync and exec never call the API, so this can interrupt a re-lock but never a build.",
		"Set GITHUB_TOKEN (or GH_TOKEN) to a token with public read access, which raises the limit to 5,000 requests an hour. On GitHub Actions, pass secrets.GITHUB_TOKEN. Otherwise wait until the reset time in the message.",
		"v0.1.0")

	// UpstreamError is any other failure talking to an upstream.
	UpstreamError = register(2006,
		"the upstream could not be reached or did not answer usefully",
		"A request to the GitHub API or to a vendor download server failed, timed out, or returned a status block cannot act on. This is about the network or the service, not about the project.",
		"Retry. If it persists, check whether the service is up and whether a proxy is in the way; the message carries the URL and the status.",
		"v0.1.0")

	// NoSuchChannel is a constraint naming a release line the upstream does
	// not publish.
	NoSuchChannel = register(2009,
		"the upstream publishes no such release channel",
		"block.toml asks for a channel — a release line an upstream publishes under a tag that moves, such as Foundry's \"nightly\" — and the recipe declares none by that name. A channel has to be declared, because its assets are named after the channel rather than after a version, and block cannot guess that name.",
		"Ask for a version instead, or use one of the channels in the message. For a registry tool whose upstream publishes a channel block does not carry yet, report it at https://github.com/nao1215/block-registry/issues.",
		"v0.3.0")

	// ChannelNotPinnable is a channel whose moving tag leads to nothing that
	// stays put.
	ChannelNotPinnable = register(2010,
		"the channel cannot be pinned to anything immutable",
		"A channel is a tag the upstream moves. block pins one by dereferencing that tag and taking the release published for the commit under it, whose tag never moves again. This upstream moves the tag but publishes no such release, so the only thing to record would be a URL whose contents change — which is the one thing a lockfile may not hold.",
		"Ask for a version instead. If the upstream has changed how it publishes this channel, report it at https://github.com/nao1215/block-registry/issues so the recipe can follow.",
		"v0.3.0")

	// PlatformUnsupported is a source that ships nothing for a platform.
	PlatformUnsupported = register(2007,
		"the upstream ships no build for this platform",
		"The recipe records which operating systems and architectures the upstream publishes, and the platform asked for is not one of them. block will not substitute a build for a different platform, and will not build from source.",
		"Use a platform the message lists, or drop the tool from the platforms your project declares. The full platform coverage of every tool is at https://github.com/nao1215/block/blob/main/doc/tools.md.",
		"v0.1.0")
)
