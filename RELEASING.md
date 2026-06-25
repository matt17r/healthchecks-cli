# Releasing & Homebrew install

`hc` is distributed through a personal Homebrew **tap** (your own formula repo) as a
**cask** — Homebrew's format for prebuilt binaries (formulae are meant to compile
from source). [GoReleaser](https://goreleaser.com) cross-compiles the binaries,
attaches them to the GitHub release, and regenerates `Casks/hc.rb` to point at
them — so `brew install` just downloads the binary instead of compiling.

Releases are cut **locally** (no CI) via `scripts/release.sh`, which uses your `gh`
login (write access to both the source repo and the tap). GoReleaser generates the
cask but does **not** push it (`skip_upload`); the script runs `brew style --fix`
on it first — goreleaser's template emits a stray blank line that the tap's
`brew test-bot` CI would otherwise reject — then commits it to the tap.

## One-time setup

1. **Push this project to GitHub** as `matt17r/healthchecks-cli` (public). The
   `origin` remote is how GoReleaser knows where to publish the release.

2. **Create the tap repo.** Homebrew taps must be named `homebrew-<something>`:

   ```sh
   brew tap-new matt17r/tap        # scaffolds a local homebrew-tap repo
   ```

   Push it to `github.com/matt17r/homebrew-tap`. GoReleaser writes `Casks/hc.rb`
   into it on each release, so you never hand-edit the cask or compute checksums.

3. **Install the tooling:**

   ```sh
   brew install goreleaser
   gh auth login        # if you haven't already
   ```

## Cutting a release

```sh
git tag v0.1.0
git push origin v0.1.0

scripts/release.sh           # or: scripts/release.sh v0.1.0
```

`scripts/release.sh` builds every OS/arch, creates the GitHub release and uploads
the archives + checksums, lint-fixes the generated cask with `brew style --fix`,
then commits it to the tap. Bump the tag for each subsequent release.

Under the hood it's `goreleaser release --clean` followed by `brew style --fix` and
a `git push` to the tap; run those by hand if you need to.

### Dry runs

```sh
goreleaser check                       # validate .goreleaser.yaml
goreleaser release --snapshot --clean  # build everything into dist/, touch nothing remote
```

`--snapshot` doesn't need a tag or a token — handy for confirming the build matrix
and the generated `dist/homebrew/Casks/hc.rb` before doing it for real.

### Why the cask isn't pushed by GoReleaser

`homebrew_casks` has `skip_upload: "true"`, so GoReleaser writes the cask into
`dist/` but doesn't push it. That gap is deliberate: GoReleaser's generated cask
trips `brew test-bot`'s `Layout/EmptyLinesAroundBlockBody` audit (a stray blank
line before the closing `end`), so `scripts/release.sh` runs `brew style --fix` on
it before committing to the tap. Pushing straight from GoReleaser would land an
unlinted cask and a red CI run on the tap.

## Installing

```sh
brew install matt17r/tap/hc      # downloads the prebuilt binary from the release
brew upgrade hc                  # picks up new releases once you bump the tag
```

## Notes

- `brew audit --cask --new matt17r/tap/hc` checks the generated cask before you
  publish.
- The cask installs an **unsigned** binary, so the generated `postflight` strips the
  macOS quarantine xattr to avoid a Gatekeeper warning. Signing/notarising would
  remove the need for that, but isn't set up.
- Moving to CI later is mostly swapping the local `gh` token for a fine-grained PAT
  with write access to the tap repo (the Actions-provided `GITHUB_TOKEN` can't reach
  a second repo), stored as a secret and used by a tag-triggered workflow.
