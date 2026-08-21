# Hero demo: two lines of block.toml in, a pinned toolchain out, a real tool
# running. Rendered to doc/img/demo.gif for the README and the front page.
#
# Regenerate with "make demo", which builds the binary and primes the demo
# store first. The recording is real: `block lock` calls the GitHub API, so it
# needs network and a GITHUB_TOKEN, and the versions it prints are whatever
# upstream has published. The store is primed beforehand so `block sync`
# extracts from cache instead of spending twenty seconds of the GIF on a
# progress-free download — everything it prints is what it would print anyway.

Output doc/img/demo.gif

Set Shell "bash"
Set FontSize 18
Set Width 1180
Set Height 500
Set Padding 24
Set Theme "Dracula"
Set TypingSpeed 45ms

Hide
Type "export PS1='$ ' PATH=$PWD/dist:$PATH BLOCK_HOME=/tmp/block-demo"
Enter
Type "cd doc/demo/project && rm -f block.lock && rm -rf /tmp/block-demo/tools && clear"
Enter
Show
Sleep 800ms

# Two tools. That is the whole manifest.
Type "cat block.toml"
Sleep 500ms
Enter
Sleep 2.2s

# lock resolves them and writes a URL and a SHA-256 per platform.
Type "block lock"
Sleep 500ms
Enter
Sleep 4s

# sync installs exactly what the lockfile names. It never resolves.
Type "block sync"
Sleep 500ms
Enter
Sleep 5s

# exec runs your command with the locked toolchain first on PATH.
Type "block exec forge --version"
Sleep 500ms
Enter
Sleep 3.5s
