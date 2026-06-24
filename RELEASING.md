# Releasing & Homebrew install

`hc` is distributed through a personal Homebrew **tap** (your own formula repo).
The formula builds from source, so there are no per-platform binaries to manage —
Homebrew just needs Go (declared as a build dependency) and a source tarball.

## One-time setup

1. **Push this project to GitHub**, e.g. `matt17r/healthchecks-cli` (public).

2. **Create a tap repo.** Homebrew taps must be named `homebrew-<something>`:

   ```sh
   brew tap-new matt17r/tap        # scaffolds a local homebrew-tap repo
   ```

   Then push that repo to `github.com/matt17r/homebrew-tap`.

## Cutting a release

```sh
git tag v0.1.0
git push origin v0.1.0
```

Get the tarball checksum Homebrew will verify:

```sh
curl -sL https://github.com/matt17r/healthchecks-cli/archive/refs/tags/v0.1.0.tar.gz | shasum -a 256
```

Copy `hc.rb` into the tap as `Formula/hc.rb`, then in it:
- replace `matt17r`,
- set `url` to the new tag,
- paste the `sha256` from above.

Commit and push the tap.

## Installing

```sh
brew install matt17r/tap/hc      # from the tagged release
brew install --HEAD matt17r/tap/hc   # build straight from main, no tag needed
```

`brew upgrade hc` picks up new releases once you bump the tag + sha in the tap.

## Notes

- The `head` line in the formula enables `--HEAD` installs from `main`, handy for
  trying changes before tagging.
- `brew audit --strict --new hc` checks the formula before you publish.
- A `LICENSE` file should exist in the repo to match the `license "MIT"` line
  (or change/remove that line).
