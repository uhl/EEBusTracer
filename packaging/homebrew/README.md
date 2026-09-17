# Homebrew tap

EEBusTracer is distributed via a Homebrew tap at
[`uhl/homebrew-eebustracer`](https://github.com/uhl/homebrew-eebustracer).

```bash
brew tap uhl/eebustracer
brew install eebustracer
```

## How the tap is maintained

- `eebustracer.rb.tmpl` in this directory is the source formula template.
- On every `v*` tag push (or GitHub release creation), the release workflow
  waits for the archive artifacts to be uploaded, then the
  `homebrew-tap` job renders the template with the new version and the
  SHA256 of each platform archive, and pushes the resulting `eebustracer.rb`
  to the tap repo.

## One-time bootstrap of the tap repo

The tap repo must exist before the auto-update job can push to it. Create it:

```bash
gh repo create uhl/homebrew-eebustracer --public \
  --description "Homebrew tap for EEBusTracer"
git clone https://github.com/uhl/homebrew-eebustracer
cd homebrew-eebustracer
mkdir -p Formula
# copy the initial formula rendered from the current release into Formula/eebustracer.rb
git add Formula/eebustracer.rb
git commit -m "Initial formula"
git push
```

## Required secret

The release workflow needs `HOMEBREW_TAP_TOKEN` — a fine-grained PAT with
`contents: write` on the tap repo only. Add it under
`Settings → Secrets and variables → Actions` in the main
`uhl/EEBusTracer` repo.
