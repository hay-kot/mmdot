# mmdot

A tiny and terrible dotfiles utility for managing my machines. Probably don't use this.

## Installation

```bash
go install github.com/hay-kot/mmdot
```

## Usage

```bash
mmdot generate          # render configured templates
mmdot encrypt           # create/update managed .age files
mmdot encrypt --dry-run # report files that need encryption or recipient rotation
mmdot encrypt --force   # re-encrypt all managed .age files with current recipients
mmdot decrypt           # restore plaintext copies while keeping .age files
mmdot clean             # remove plaintext copies when .age counterparts exist
mmdot clean --dry-run   # preview plaintext files that would be removed
```

### Secret file lifecycle

Vault variable files are configured with `?vault=true` in `variables.var_files`.
Running `mmdot encrypt` writes `<file>.age` and removes the plaintext vault file.
Running `mmdot decrypt` restores the plaintext copy and leaves the `.age` file in
place so it remains ready to commit.

Files configured under `age.files` use explicit encrypted/plaintext paths:

```yaml
age:
  identity_file: path/to/private-key.txt
  recipients:
    - age1...
  files:
    - src: secrets/example.txt.age # encrypted file managed by mmdot
      dest: ~/.config/example.txt  # plaintext destination
      perm: "0600"                 # optional permission after decrypt
```

For `age.files`, `encrypt` reads `dest` and writes `src` while keeping `dest`.
`decrypt` reads `src` and writes `dest` while keeping `src`.

When the configured recipient count changes, `encrypt` rotates existing managed
`.age` files so they can be decrypted by the current recipients. Use
`encrypt --force` when recipients changed but the count stayed the same.

