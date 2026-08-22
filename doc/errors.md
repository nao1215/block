# Error codes

A coded error carries a name that can be searched for, linked to, and branched on, so the message beside it stays free to improve without breaking anyone.

A code is `BLK` followed by four digits, and the thousands digit says what kind of problem it is — which is to say, where the fix lives.

| Codes | Kind | Codes assigned |
|---|---|---|
| `BLK1xxx` | what was asked for — block.toml, block.lock, and the names typed on the command line | 10 |
| `BLK2xxx` | resolving a version against an upstream, which only `block lock` ever does | 10 |
| `BLK3xxx` | downloading an artifact and proving it is the one block.lock names | 3 |
| `BLK4xxx` | installing into the store under `$BLOCK_HOME` | 5 |
| `BLK5xxx` | running a command with the locked toolchain, through `block exec` or a shim | 4 |
| `BLK6xxx` | a refusal on security grounds — block declining, rather than failing | 6 |
| `BLK9xxx` | an internal error: a bug in block | 1 |

The digit is not the exit status. Every coded error exits 1. block's other non-zero exit, 2 from `block lock --check` and `block status`, is a result rather than an error — the lockfile would change, or the toolchain is not ready — and carries no code, because there is nothing to look up.

Codes are grouped by what you have to fix rather than by where inside block the error was raised, so one code can be reported from several places when the answer is the same in all of them.

Not every message carries one. A command line block cannot parse at all — an unknown command, an unknown flag, the wrong number of arguments — is reported by the parser, and there is nothing to look up: the message already names what you typed.

Look a code up from the terminal with `block explain BLK1001`, which prints this same text without a browser.

## Every code

| Code | Meaning | Since |
|---|---|---|
| [`BLK1001`](#blk1001--blocktoml-not-found) | block.toml not found | v0.1.0 |
| [`BLK1002`](#blk1002--blocktoml-is-not-valid) | block.toml is not valid | v0.1.0 |
| [`BLK1003`](#blk1003--blocklock-not-found) | block.lock not found | v0.1.0 |
| [`BLK1004`](#blk1004--blocklock-is-not-valid) | block.lock is not valid | v0.1.0 |
| [`BLK1005`](#blk1005--blocklock-does-not-match-blocktoml) | block.lock does not match block.toml | v0.1.0 |
| [`BLK1006`](#blk1006--blocklock-has-no-artifact-for-this-platform) | block.lock has no artifact for this platform | v0.1.0 |
| [`BLK1007`](#blk1007--the-tool-is-not-in-the-registry) | the tool is not in the registry | v0.1.0 |
| [`BLK1008`](#blk1008--the-named-tool-is-not-declared-in-blocktoml) | the named tool is not declared in block.toml | v0.1.0 |
| [`BLK1009`](#blk1009--two-executables-would-provide-one-command) | two executables would provide one command | v0.1.0 |
| [`BLK1010`](#blk1010--the-ecosystem-is-not-one-the-registry-knows) | the ecosystem is not one the registry knows | v0.1.0 |
| [`BLK2001`](#blk2001--no-upstream-version-matches-the-constraint) | no upstream version matches the constraint | v0.1.0 |
| [`BLK2002`](#blk2002--the-matching-tags-have-no-published-release) | the matching tags have no published release | v0.1.0 |
| [`BLK2003`](#blk2003--the-upstream-repository-tag-or-release-does-not-exist) | the upstream repository, tag or release does not exist | v0.1.0 |
| [`BLK2004`](#blk2004--the-release-does-not-carry-the-expected-asset) | the release does not carry the expected asset | v0.1.0 |
| [`BLK2005`](#blk2005--the-github-api-rate-limit-was-reached) | the GitHub API rate limit was reached | v0.1.0 |
| [`BLK2006`](#blk2006--the-upstream-could-not-be-reached-or-did-not-answer-usefully) | the upstream could not be reached or did not answer usefully | v0.1.0 |
| [`BLK2007`](#blk2007--the-upstream-ships-no-build-for-this-platform) | the upstream ships no build for this platform | v0.1.0 |
| [`BLK2008`](#blk2008--the-release-carries-the-asset-name-more-than-once) | the release carries the asset name more than once | v0.1.0 |
| [`BLK2009`](#blk2009--the-upstream-publishes-no-such-release-channel) | the upstream publishes no such release channel | v0.3.0 |
| [`BLK2010`](#blk2010--the-channel-cannot-be-pinned-to-anything-immutable) | the channel cannot be pinned to anything immutable | v0.3.0 |
| [`BLK3001`](#blk3001--an-artifact-could-not-be-downloaded) | an artifact could not be downloaded | v0.1.0 |
| [`BLK3002`](#blk3002--a-download-does-not-match-the-digest-in-blocklock) | a download does not match the digest in block.lock | v0.1.0 |
| [`BLK3003`](#blk3003--a-cached-artifact-is-corrupt-and-could-not-be-replaced) | a cached artifact is corrupt and could not be replaced | v0.1.0 |
| [`BLK4001`](#blk4001--an-artifact-could-not-be-unpacked) | an artifact could not be unpacked | v0.1.0 |
| [`BLK4002`](#blk4002--the-artifact-does-not-contain-a-declared-executable) | the artifact does not contain a declared executable | v0.1.0 |
| [`BLK4003`](#blk4003--an-install-in-the-store-is-incomplete-or-damaged) | an install in the store is incomplete or damaged | v0.1.0 |
| [`BLK4004`](#blk4004--a-locked-tool-is-not-installed) | a locked tool is not installed | v0.1.0 |
| [`BLK4005`](#blk4005--the-store-could-not-be-written) | the store could not be written | v0.1.0 |
| [`BLK5001`](#blk5001--the-command-is-in-neither-the-toolchain-nor-path) | the command is in neither the toolchain nor PATH | v0.1.0 |
| [`BLK5002`](#blk5002--the-command-could-not-be-started) | the command could not be started | v0.1.0 |
| [`BLK5003`](#blk5003--shims-are-calling-each-other-in-a-loop) | shims are calling each other in a loop | v0.1.0 |
| [`BLK5004`](#blk5004--a-shim-found-neither-a-project-nor-another-command-of-that-name) | a shim found neither a project nor another command of that name | v0.1.0 |
| [`BLK6001`](#blk6001--the-url-is-not-one-block-will-fetch) | the URL is not one block will fetch | v0.1.0 |
| [`BLK6002`](#blk6002--an-entry-would-be-written-outside-its-directory) | an entry would be written outside its directory | v0.1.0 |
| [`BLK6003`](#blk6003--an-archive-contains-an-entry-block-will-not-extract) | an archive contains an entry block will not extract | v0.1.0 |
| [`BLK6004`](#blk6004--an-archive-member-is-larger-than-block-will-extract) | an archive member is larger than block will extract | v0.1.0 |
| [`BLK6005`](#blk6005--a-name-or-version-from-blocklock-is-not-a-path-component) | a name or version from block.lock is not a path component | v0.1.0 |
| [`BLK6006`](#blk6006--an-archive-writes-the-same-file-twice) | an archive writes the same file twice | v0.1.0 |
| [`BLK9001`](#blk9001--an-internal-error) | an internal error | v0.1.0 |

## BLK1xxx — what was asked for — block.toml, block.lock, and the names typed on the command line

### BLK1001 — block.toml not found

block works on a project, and a project is the directory tree under a block.toml. block looked for that file in the working directory and every directory above it, and found none.

Fix: Write a block.toml in the repository root, or change into a directory inside a project that has one. The manifest is a few lines; see https://nao1215.github.io/block/getting-started/.

Exits 1. Since v0.1.0.

### BLK1002 — block.toml is not valid

block.toml was found but does not parse, or says something block cannot act on: an unknown key, a tool name that is not a tool name, a platform block does not support, or a version constraint that is not a dotted prefix. block refuses the whole file rather than acting on the part of it that made sense.

Fix: Fix the key the message names. Constraints are dotted prefixes only — "1", "1.7", "1.7.4" — with no operators or ranges; the format is at https://nao1215.github.io/block/reference/.

Exits 1. Since v0.1.0.

### BLK1003 — block.lock not found

The project declares a toolchain but has never resolved it, so there is nothing to install or run. block will not resolve one on the spot: which version a build uses is a decision that gets committed, not one made by whoever happened to run the command.

Fix: Run "block lock" to resolve block.toml into block.lock, and commit both files.

Exits 1. Since v0.1.0.

### BLK1004 — block.lock is not valid

block.lock does not parse, carries a format version this block does not write, or holds a value that is not one lock could have produced — a digest that is not a digest, a version with a path separator in it, a pin that does not satisfy the constraint it records. A lockfile arrives through pull requests and hand edits, so it is checked rather than trusted.

Fix: Do not hand-edit block.lock. Restore it from version control, or delete it and run "block lock" to write it again. A lockfile from a newer block needs a newer block.

Exits 1. Since v0.1.0.

### BLK1005 — block.lock does not match block.toml

The manifest and the lockfile describe different toolchains: a tool was added, removed, or re-pointed at a different constraint or source since the lock was written. sync and exec both refuse rather than installing something nobody resolved, and every disagreement is listed, not just the first.

Fix: Run "block lock" and commit the result. In CI, "block lock --check" reports the same disagreements before anything is installed.

Exits 1. Since v0.1.0.

### BLK1006 — block.lock has no artifact for this platform

The lockfile pins the tool, but not for the operating system and architecture this machine is. Either the pin was made on a machine of a different kind and block.toml does not declare the platforms the project supports, or block.toml declares a list that does not include this one — in which case re-running lock would write the same lockfile back, and what has to change is the manifest.

Fix: Add every platform the project builds on to the "platforms" list in block.toml and run "block lock" again, so one lockfile covers the whole team and CI.

Exits 1. Since v0.1.0.

### BLK1007 — the tool is not in the registry

block.toml names a tool with no `[tools.<name>.source]` of its own, and the registry compiled into this binary has no recipe for that name.

Fix: Run "block list" to see the names the registry carries, or define `[tools.<name>.source]` in block.toml to fetch the tool without waiting for a registry entry.

Exits 1. Since v0.1.0.

### BLK1008 — the named tool is not declared in block.toml

`block lock <tool>` re-resolves the tools you name and keeps every other pin. A name that is not in block.toml would silently do nothing, so it is refused instead.

Fix: Check the spelling against block.toml, or run "block lock" with no arguments to re-resolve every tool.

Exits 1. Since v0.1.0.

### BLK1009 — two executables would provide one command

Two tools in the toolchain — or two paths inside one tool — end in the same command name. Which of them runs would depend on how it was called: a shim resolves a command through the lockfile, PATH resolves it by directory order, and the two can disagree. Names are compared without regard to case on every platform, because Windows resolves PATH that way and a lockfile is committed and read everywhere.

Fix: Remove one of the two tools from block.toml, or pin the fork rather than the tool it forks.

Exits 1. Since v0.1.0.

### BLK1010 — the ecosystem is not one the registry knows

`block list <ecosystem>` narrows the catalogue to one blockchain system. The name given is not one any recipe declares, so there is nothing to narrow to; the message lists the names that exist.

Fix: Use one of the names in the message, or run `block list` with no argument to see every tool with the systems it serves.

Exits 1. Since v0.1.0.

## BLK2xxx — resolving a version against an upstream, which only `block lock` ever does

### BLK2001 — no upstream version matches the constraint

The upstream's tags were read, and none of them is a release version the constraint in block.toml allows. Pre-releases never satisfy a constraint, so a line that has only ever shipped release candidates matches nothing.

Fix: Widen the constraint in block.toml, or check the upstream's releases page for how the line is actually spelled.

Exits 1. Since v0.1.0.

### BLK2002 — the matching tags have no published release

Tags satisfying the constraint exist, but the newest of them are drafts, pre-releases, or tags pushed before their release was published. block only pins a published, non-draft, non-pre-release release.

Fix: Wait for the release to be published, or pin a version that already has one.

Exits 1. Since v0.1.0.

### BLK2003 — the upstream repository, tag or release does not exist

The upstream answered "not found". The repository has been renamed, deleted, or made private, or the tag the recipe renders for this version was never pushed.

Fix: Check the repo in the recipe or in `[tools.<name>.source]`. A repository that moved needs the new owner/name; a private one needs a GITHUB_TOKEN that can see it.

Exits 1. Since v0.1.0.

### BLK2004 — the release does not carry the expected asset

The release was found, but it publishes no file with the name the recipe renders for this version and platform. Upstreams rename their assets, and a recipe is a rule about those names. The message lists what the release does carry.

Fix: For a registry tool, report it at https://github.com/nao1215/block-registry/issues — the recipe needs updating. For a `[tools.<name>.source]` of your own, correct the asset template against the names in the message.

Exits 1. Since v0.1.0.

### BLK2005 — the GitHub API rate limit was reached

block lock reads tags and releases from the GitHub API, which allows 60 requests an hour to an unauthenticated client. sync and exec never call the API, so this can interrupt a re-lock but never a build.

Fix: Set GITHUB_TOKEN (or GH_TOKEN) to a token with public read access, which raises the limit to 5,000 requests an hour. On GitHub Actions, pass secrets.GITHUB_TOKEN. Otherwise wait until the reset time in the message.

Exits 1. Since v0.1.0.

### BLK2006 — the upstream could not be reached or did not answer usefully

A request to the GitHub API or to a vendor download server failed, timed out, or returned a status block cannot act on. This is about the network or the service, not about the project.

Fix: Retry. If it persists, check whether the service is up and whether a proxy is in the way; the message carries the URL and the status.

Exits 1. Since v0.1.0.

### BLK2007 — the upstream ships no build for this platform

The recipe records which operating systems and architectures the upstream publishes, and the platform asked for is not one of them. block will not substitute a build for a different platform, and will not build from source.

Fix: Use a platform the message lists, or drop the tool from the platforms your project declares. The full platform coverage of every tool is at https://github.com/nao1215/block/blob/main/doc/tools.md.

Exits 1. Since v0.1.0.

### BLK2008 — the release carries the asset name more than once

The release publishes several files with the name the recipe renders. They are different downloads, and which of them a lockfile pinned would depend on the order the API happened to answer in, so block refuses rather than choosing. The message lists their URLs.

Fix: For a registry tool, report it at https://github.com/nao1215/block-registry/issues — the asset template needs to be specific enough to name one file. For a `[tools.<name>.source]` of your own, make the template unambiguous.

Exits 1. Since v0.1.0.

### BLK2009 — the upstream publishes no such release channel

block.toml asks for a channel — a release line an upstream publishes under a tag that moves, such as Foundry's "nightly" — and the recipe declares none by that name. A channel has to be declared, because its assets are named after the channel rather than after a version, and block cannot guess that name.

Fix: Ask for a version instead, or use one of the channels in the message. For a registry tool whose upstream publishes a channel block does not carry yet, report it at https://github.com/nao1215/block-registry/issues.

Exits 1. Since v0.3.0.

### BLK2010 — the channel cannot be pinned to anything immutable

A channel is a tag the upstream moves. block pins one by dereferencing that tag and taking the release published for the commit under it, whose tag never moves again. This upstream moves the tag but publishes no such release, so the only thing to record would be a URL whose contents change — which is the one thing a lockfile may not hold.

Fix: Ask for a version instead. If the upstream has changed how it publishes this channel, report it at https://github.com/nao1215/block-registry/issues so the recipe can follow.

Exits 1. Since v0.3.0.

## BLK3xxx — downloading an artifact and proving it is the one block.lock names

### BLK3001 — an artifact could not be downloaded

The artifact block.lock names could not be fetched: the host refused, the transfer broke, or the request timed out. Nothing was installed, and no partial file was kept — a download is written to a temporary file and only named once its digest is known.

Fix: Retry. If it persists, check network access to the host in the message; a release CDN blocked by a proxy is the usual cause in CI.

Exits 1. Since v0.1.0.

### BLK3002 — a download does not match the digest in block.lock

The bytes at the locked URL hash to something other than the SHA-256 recorded when the pin was made. Nothing is extracted and nothing is installed; the mismatching download is discarded rather than cached. This is the check working.

Fix: Run it once more in case a proxy truncated the transfer. If it persists, the artifact at that URL is not the artifact that was locked, which is worth knowing before it is on your PATH — do not "fix" it by re-running block lock until you know why the upstream file changed.

Exits 1. Since v0.1.0.

### BLK3003 — a cached artifact is corrupt and could not be replaced

The download cache under $BLOCK_HOME holds a blob whose bytes do not hash to the name it is filed under — a truncated download, or a half-restored CI cache. block discards such a blob and fetches again, but this one could not be removed.

Fix: Check the permissions on $BLOCK_HOME, then delete the file the message names. Deleting the whole cache directory is always safe: it is rebuilt from block.lock.

Exits 1. Since v0.1.0.

## BLK4xxx — installing into the store under `$BLOCK_HOME`

### BLK4001 — an artifact could not be unpacked

The downloaded file is not an archive of a kind block extracts, or it is one but is damaged. block extracts .tar.gz, .tgz, .tar.bz2, .tbz2 and .zip; anything else is installed as a single raw executable.

Fix: For a `[tools.<name>.source]` of your own, check that the asset template names the file the upstream actually publishes. For a registry tool, report it at https://github.com/nao1215/block-registry/issues.

Exits 1. Since v0.1.0.

### BLK4002 — the artifact does not contain a declared executable

The archive unpacked, but one of the executables the recipe promises is not in it — often because the upstream wraps everything in a versioned directory that strip_components has to drop, or because a binary was renamed.

Fix: For a `[tools.<name>.source]` of your own, correct bin or strip_components against the archive's real layout. For a registry tool, report it at https://github.com/nao1215/block-registry/issues.

Exits 1. Since v0.1.0.

### BLK4003 — an install in the store is incomplete or damaged

The install directory exists but does not verify: its completion marker is missing, or one of its executables is gone or is not executable. block writes the marker last and renames the directory into place atomically, so this is an interrupted install, a half-restored CI cache, or something that deleted files under $BLOCK_HOME.

Fix: Run "block sync" to replace it. block never runs an install it cannot verify.

Exits 1. Since v0.1.0.

### BLK4004 — a locked tool is not installed

block.lock pins the tool and the pin is current, but nothing for it is in the store — a fresh clone, a cleared cache, or a runner without the store restored. exec and the shims never install: what they run is what sync put there.

Fix: Run "block sync".

Exits 1. Since v0.1.0.

### BLK4005 — the store could not be written

block could not create or replace a directory under $BLOCK_HOME. The store is a per-user directory; a shared or root-owned one, or a full disk, stops sync before anything is installed.

Fix: Check the permissions and free space on $BLOCK_HOME (~/.local/share/block by default, %LOCALAPPDATA%\block on Windows), or point BLOCK_HOME at a directory this user owns.

Exits 1. Since v0.1.0.

## BLK5xxx — running a command with the locked toolchain, through `block exec` or a shim

### BLK5001 — the command is in neither the toolchain nor PATH

block exec runs any command with the locked tools first on PATH, not only the locked tools themselves. This one is not a command the project locks, and PATH has no other executable by that name either.

Fix: Check the spelling, add the tool to block.toml and run "block lock" and "block sync", or install the command the usual way for your system.

Exits 1. Since v0.1.0.

### BLK5002 — the command could not be started

The executable was found but the operating system refused to run it: it is not executable, it is built for another architecture, or its interpreter is missing. Its own exit status, when it has one, is reported instead of this — a command that runs and fails is not a block error.

Fix: Run "block sync" to reinstall the tool. If it persists, the artifact is not one this machine can run; check the platform the pin covers.

Exits 1. Since v0.1.0.

### BLK5003 — shims are calling each other in a loop

A shim runs the next command of its name on PATH when the current directory is not a block project. With more than one block shim directory on PATH, each one can find the other and the command never reaches a real tool.

Fix: Remove the extra shim directory from PATH. There is one shim directory per store: $BLOCK_HOME/shims.

Exits 1. Since v0.1.0.

### BLK5004 — a shim found neither a project nor another command of that name

The command was run through a block shim, but the working directory is not inside a block project — or the project does not lock a tool providing this command — and PATH holds no other executable of that name to step aside to.

Fix: Change into a project that locks the tool, or install the command the usual way for your system so the shim has something to defer to.

Exits 1. Since v0.1.0.

## BLK6xxx — a refusal on security grounds — block declining, rather than failing

### BLK6001 — the URL is not one block will fetch

Artifacts are fetched over HTTPS and nothing else. Plain HTTP is accepted only for loopback addresses, so an offline test server can stand in for GitHub. The rule is held on every redirect hop too: an https URL that redirects to plain http is refused rather than followed.

Fix: Use an https URL. If an upstream publishes over plain http only, it is not a source block can verify the origin of, and mirroring it somewhere with TLS is the fix.

Exits 1. Since v0.1.0.

### BLK6002 — an entry would be written outside its directory

An archive member, an executable path, or a value from block.lock resolves to somewhere other than the directory it is allowed to touch — an absolute path, a ".." component, a drive letter or a UNC path. block refuses the whole artifact rather than the one entry, because an archive that tries this is not an archive to be picky about.

Fix: Do not hand-edit block.lock. For a `[tools.<name>.source]` of your own, bin entries are relative, slash-separated paths inside the archive. For a registry tool or a real upstream artifact, report it — an upstream shipping such an archive is a finding.

Exits 1. Since v0.1.0.

### BLK6003 — an archive contains an entry block will not extract

block extracts regular files and directories, and nothing else. Symbolic links, hard links, device nodes, sockets and FIFOs are refused: a link is a way to reach outside the install directory after the path check has already passed, and the rest have no place in a tool distribution.

Fix: For a `[tools.<name>.source]` of your own, take an artifact the upstream publishes without links. For a registry tool, report it at https://github.com/nao1215/block-registry/issues.

Exits 1. Since v0.1.0.

### BLK6004 — an archive member is larger than block will extract

A single member of the archive exceeds the size block unpacks. A compressed archive can be very much smaller than what it expands to, so the limit is applied while writing rather than trusted from the header.

Fix: Take the artifact apart yourself and check what is in it. No tool block installs comes close to this limit.

Exits 1. Since v0.1.0.

### BLK6005 — a name or version from block.lock is not a path component

A tool's name and version become a directory under $BLOCK_HOME, and that directory is one block creates, populates and removes. A value carrying a separator, a "..", a NUL or anything but the closed alphabet a version is written in is refused before it reaches the filesystem.

Fix: Do not hand-edit block.lock. Restore it from version control, or delete it and run "block lock" to write it again.

Exits 1. Since v0.1.0.

### BLK6006 — an archive writes the same file twice

Two members of the archive resolve to one file, so what ends up on disk would depend on which of them was extracted last. Names differing only in case are the usual cause: they are two files on Linux and one on macOS and Windows, and an archive that relies on the difference installs differently on different machines.

Fix: For a `[tools.<name>.source]` of your own, take an artifact whose members are distinct. For a registry tool, report it at https://github.com/nao1215/block-registry/issues.

Exits 1. Since v0.1.0.

## BLK9xxx — an internal error: a bug in block

### BLK9001 — an internal error

block reached a state it does not have an explanation for. This is a bug in block, not something wrong with the project, the network or the upstream.

Fix: Report it at https://github.com/nao1215/block/issues/new with the command you ran and the whole message.

Exits 1. Since v0.1.0.

