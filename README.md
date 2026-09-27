# ProPresenter Toolkit

![AI-assisted](https://img.shields.io/badge/AI--assisted-Claude-8A2BE2)

> **AI-assisted project:** this code was written with the help of an AI
> assistant (Anthropic's Claude), under human direction and review. Commits
> with AI involvement carry a `Co-Authored-By: Claude` trailer.

A small self-hosted web app that turns sermon-notes PDFs into ready-to-open
**ProPresenter 7 `.pro` files**, in a style learned from your own show files.
It's built to run as a service in a Debian LXC (e.g. on Proxmox), and the team
uses it from a browser.

## What it does

Upload a PDF, check the slides it found, and download a `.pro`. There are two
modes:

- **Prompts**: a running script split into `SLIDE 1:`, `SLIDE 2:`, …
  One slide per section; bold words in the PDF are drawn in a highlight colour.
- **Slides**: a labelled outline:

  | In the PDF | Becomes |
  |---|---|
  | `Title: …` | a Title slide |
  | `POINT N: …` | a Point slide |
  | `CONDITION N: …`, `DRIVEN N: …` | Keyword slides |
  | `Ephesians 2:1-10 (NIV)` + the verse text under it | a Scripture slide |
  | `Back to Point 1 when I say: …` | a copy of that earlier slide, relabelled |
  | `Image: …` | flagged and skipped (add images by hand) |

  Lines that match none of these are shown as warnings, never guessed at.

**Styles** are made in the **Style Manager**. Upload one of your real `.pro`
files and click which slide to copy for each role (Title, Point, Scripture, …).
Generated slides are clones of those slides with only the text swapped, so the
font, box, background and Audience Look all match your existing shows.

## Install in a Debian LXC

Needs a Debian 12 or 13 container with internet access (to fetch Go and the
dependencies at build time). 1 CPU, 1 GB RAM and 4 GB disk is plenty. The app
itself uses a few tens of MB; the extra is for compiling.

As root in the container:

1. Install git: `apt update && apt install -y git`
2. Clone the repo:
   `git clone https://github.com/rivers-church/propresenter-toolkit.git /opt/src/propresenter-toolkit`
3. Run the installer:
   `sh /opt/src/propresenter-toolkit/deploy/install.sh`

The installer:

- uses Debian's Go if it's new enough (Debian 13), otherwise installs the
  official Go release into `/usr/local/go`;
- builds the app into `/opt/propresenter-toolkit/pptoolkit`;
- creates a `propresenter-toolkit` system user and a systemd service of the same name;
- writes settings to `/etc/propresenter-toolkit.env` (first run only);
- keeps styles in `/var/lib/propresenter-toolkit/styles`, seeded with two example styles.

It prints the address when it's done, e.g. `http://192.168.1.50:5000`.

### Settings

Edit `/etc/propresenter-toolkit.env`, then run
`systemctl restart propresenter-toolkit`:

| Setting | Default | |
|---|---|---|
| `PPT_ADDR` | `:5000` | address and port to listen on |
| `PPT_STYLES_DIR` | `/var/lib/propresenter-toolkit/styles` | style profiles folder |
| `PPT_AUTH` | *(empty)* | `user:password` to require a login |

There's no login unless you set `PPT_AUTH`, so anyone who can reach the port
can use the app and edit styles. That's fine on a trusted LAN. Otherwise set
it, and for access from outside, use [Tailscale](https://tailscale.com) or a
reverse proxy (Caddy/nginx) with HTTPS rather than opening the port.

### Updating

```bash
git -C /opt/src/propresenter-toolkit pull
```

```bash
sh /opt/src/propresenter-toolkit/deploy/install.sh
```

Settings and styles are kept.

### Day-to-day

```bash
systemctl status propresenter-toolkit
```

```bash
journalctl -u propresenter-toolkit -f
```

Back up `/var/lib/propresenter-toolkit/styles`. The Style Manager page also has a
**Download** button per style. Style files from the earlier Python version of
this tool work as-is; copy them into that folder. In-progress conversions are
held in memory only and are dropped after 6 hours or on restart.

### Uninstalling

```bash
systemctl disable --now propresenter-toolkit
```

```bash
rm -rf /opt/propresenter-toolkit /etc/systemd/system/propresenter-toolkit.service /etc/propresenter-toolkit.env
```

Remove `/var/lib/propresenter-toolkit` too if you don't want the styles.

## Known gap: "Point L3" in the example Slides style

The sample file used to build the example `2026-09-27 (Slides)` style had no
slide using the "Point L3" Audience Look, so that role has a placeholder UUID
that won't resolve in ProPresenter. Fix it once by making a new Slides style in
the Style Manager from a `.pro` that has a slide set to "Point L3", and clicking
**Point** on that slide.

## Limitations

- **Verse pacing isn't automatic.** A passage lands on one Scripture slide;
  split it in ProPresenter for a verse-by-verse reveal.
- **Bold detection is font-name based**: fonts named `Heavy`, `Black` or `Bold`.
  If another PDF export names fonts differently, extend `BoldFontMarkers` in
  `internal/pdftext/pdftext.go`.
- **Only text is generated.** Backgrounds, media and props come from the
  template slide; `Image:` lines are left for you to add.

## Development

Needs Go 1.24.1 or newer.

```bash
go run ./cmd/pptoolkit
```

Then open http://localhost:5000. Styles go in `./styles`.

```bash
go test ./...
```

```text
cmd/pptoolkit/   the server binary
internal/
  web/           HTTP handlers, templates, CSS (embedded in the binary)
  convert/       PDF -> review -> .pro pipeline
  pdftext/       PDF -> visual lines of words, with bold flags
  parse/         lines -> Prompts slides / Notes entries
  rtf/           the RTF dialect ProPresenter writes (+ plain-text preview)
  style/         style profiles (JSON) and the styles folder; bundled examples
  pro/           read .pro files; build new ones from a style + parsed PDF
  pb/            Go code generated from proto/ - do not edit
  testpdf/       tiny generated PDFs so tests don't need real sermon files
proto/           ProPresenter 7 protobuf schema (see Credits)
deploy/          the LXC install script
```

To regenerate `internal/pb` after updating `proto/`, install the generators and
run `buf generate`:

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
```

```bash
go install github.com/bufbuild/buf/cmd/buf@latest
```

```bash
buf generate
```

### Changes from the Python version

This replaces an earlier Python tool. On the same PDFs and styles its `.pro`
output is identical, apart from these fixes:

- Words slightly off the baseline no longer break onto their own line.
- Accented and hyphenated words no longer split ("poi ē ma" → "poiēma").
- No stray space after opening quotes, and ligatures (`ﬁ`, `ﬂ`) become plain letters.
- A `SLIDE 26` marker missing its colon is still recognised.

It also adds an optional login, per-style colours, style download/delete, and
shows warnings before you download rather than after.

## Credits

The ProPresenter 7 protobuf definitions in `proto/` are from
[greyshirtguy/ProPresenter7-Proto](https://github.com/greyshirtguy/ProPresenter7-Proto)
(MIT licence, see `proto/LICENSE`). ProPresenter is a product of Renewed Vision;
this project is not affiliated with them.
