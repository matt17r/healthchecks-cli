# Releasing & Homebrew install

`hc` is distributed through a personal Homebrew **tap** (your own formula repo) as a
**cask** — Homebrew's format for prebuilt binaries (formulae are meant to compile
from source). [GoReleaser](https://goreleaser.com) cross-compiles the binaries,
attaches them to the GitHub release, and regenerates `Casks/hc.rb` in the tap to
point at them — so `brew install` just downloads the binary instead of compiling.

Releases are cut **locally** (no CI). GoReleaser uses your `gh` login, which has
write access to both the source repo and the tap, so one command does everything.

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

GITHUB_TOKEN=$(gh auth token) goreleaser release --clean
```

That one `goreleaser` command builds every OS/arch, creates the GitHub release and
uploads the archives + checksums, then commits the updated cask to the tap. Bump
the tag for each subsequent release.

### Dry runs

```sh
goreleaser check                       # validate .goreleaser.yaml
goreleaser release --snapshot --clean  # build everything into dist/, touch nothing remote
```

`--snapshot` doesn't need a tag or a token — handy for confirming the build matrix
and the generated `dist/homebrew/Casks/hc.rb` before doing it for real.

### Committing the cask by hand instead

If you'd rather not let GoReleaser push to the tap, set `skip_upload: "true"` under
`homebrew_casks` in `.goreleaser.yaml`. GoReleaser then only writes the cask into
`dist/`, and you copy it into your local tap clone and commit it yourself.

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
