# appimg

A command-line package manager for [AppImages](https://appimage.org/) on Linux.

Installs AppImages from the [appimage.github.io catalog](https://appimage.github.io/) or directly from GitHub releases, with automatic desktop integration (`.desktop` file + icon extraction).

## Installation

```bash
go install github.com/sivel/appimg@latest
```

AppImages are installed to `~/Applications/`.

## Usage

```
appimg <command> [flags]
```

Run `appimg --help` or `appimg <command> --help` for full flag documentation.

### Sources

Packages can be installed from the catalog by name, directly from a GitHub repository, from a local file, or from a URL:

```bash
appimg install Bitwarden
appimg install github:FreeCAD/FreeCAD
appimg install github:localsend/localsend@1.17.0
appimg install ~/Downloads/SomeApp-x86_64.AppImage
appimg install https://example.com/SomeApp-x86_64.AppImage
```

Local and URL installs are stored with source `local:` and cannot be updated via `appimg update`.

### Desktop integration

On install, `appimg` extracts the `.desktop` file and icon embedded in the AppImage and installs them into `$XDG_DATA_HOME` (default `~/.local/share/`). The `Exec=` line is set to use `appimg exec <name>`, which handles sandbox requirements and surfaces error dialogs on launch failure.

If `appimg` itself is moved or upgraded (e.g. via a version manager like `mise`), run:

```bash
appimg desktop regenerate
```

This rewrites the `Exec=` line in all desktop files to reflect the current `appimg` location.

### Rolling releases

Some projects use a fixed tag name (e.g. `nightly`, `latest`, `continuous`) where only the assets change. `appimg` auto-detects these at install time and uses `published_at` timestamps for update comparisons instead of tag names. For non-standard rolling tag names, use `--rolling` at install time.

## Authentication

GitHub API requests are unauthenticated by default (60 requests/hour limit). Set `GITHUB_TOKEN` to increase this:

```bash
export GITHUB_TOKEN=<your token>
```

## License

MIT
