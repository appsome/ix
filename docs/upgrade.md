# Upgrades

Two independent upgrade paths, by design (see [DESIGN.md](DESIGN.md) §2, §7).

## 1. Runtime module (imported) — trivial

```sh
go get -u github.com/appsome/ix/runtime@latest
go mod tidy
```

No merge. The API is stable and SemVer'd; breaking changes bump the major and
are called out in the runtime CHANGELOG. `ix upgrade` runs this automatically
when an installed block's required `runtime.version` advances.

## 2. Vendored blocks — 3-way merge

For files a block marks `managed: true`:

```
BASE   = last pristine render (from .ix/baseline/, written on every add/upgrade)
THEIRS = your current file on disk
OURS   = new pristine render at the new block version

merged, conflicts = diff3(OURS, BASE, THEIRS)
```

- `ix diff [block]` previews the merge — writes nothing.
- `ix upgrade [block]` writes the merge, updates `.ix/baseline/` and `ix.lock`,
  and reports any conflicts (standard `<<<<<<<` markers — resolve like a git
  merge).
- `managed: false` files (e.g. `seed_policy.csv`) are rendered once on `add`
  and never touched again.
- `once: true` files (e.g. a migration) are skipped if they already exist.

### Why `.ix/baseline/`

It is the committed, never-hand-edited pristine copy of every managed file. It
makes the merge deterministic and offline: `ix` does not need to embed every
historical template version in the binary to reconstruct BASE. Cost is a little
repo weight; the directory is excluded from editor search.

### Patches (anchors)

Blocks that inject into another block's file (e.g. `authz` wiring into
`cmd/api/main.go`) do so at named anchor comments like `// ix:wire-services`,
not line numbers. Re-applying is idempotent: if the insert text is already
present, the patch is skipped. If the anchor is gone (you deleted it), `ix
upgrade` reports it rather than guessing.
