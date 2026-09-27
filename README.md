# ProPresenter Toolkit

![AI-assisted](https://img.shields.io/badge/AI--assisted-Claude-8A2BE2)

> **AI-assisted project:** this code was written with the help of an AI
> assistant (Anthropic's Claude), under human direction and review. Commits
> with AI involvement carry a `Co-Authored-By: Claude` trailer.

Turns sermon-notes PDFs into ready-to-open **ProPresenter 7 `.pro` files**, in a
style you define once from your own real show files.

It's a single Go program with a built-in web UI. Double-click it and it opens
in your browser; run it on a server and the whole team can use it. No Python,
no installer, and ProPresenter isn't needed to run the converter itself, only
to open the result.

## Two conversion modes

- **Prompts**: for a running script broken into `SLIDE 1:`, `SLIDE 2:`, …
  sections. Bold words (detected from the PDF's own font weight) are drawn in a
  highlight color; everything else keeps its original casing. One slide per
  `SLIDE N:` block.

- **Slides**: for a labelled outline:
  | In the PDF | Becomes |
  |---|---|
  | `Title: …` | a Title slide |
  | `POINT N: …` | a Point slide |
  | `CONDITION N: …`, `DRIVEN N: …` | Keyword slides |
  | `Ephesians 2:1-10 (NIV)` + the verse text under it | a Scripture slide (reference + verse boxes) |
  | `Back to Point 1 when I say: …` | a copy of that earlier slide, relabelled |
  | `Image: …` | flagged and skipped (add images by hand) |

  Lines that match none of these are listed as warnings rather than guessed at,
  so nothing silently goes missing.

Either way you get a review screen before the file is written, where you can
remove any slide you don't want.

## Getting it

**Download**: grab the file for your computer from the
[Releases](../../releases) page:

| Computer | File |
|---|---|
| Windows | `pptoolkit-windows-amd64.exe` |
| Mac (Apple silicon) | `pptoolkit-darwin-arm64` |
| Mac (Intel) | `pptoolkit-darwin-amd64` |
| Linux / Proxmox LXC | `pptoolkit-linux-amd64` |

**Or build it** (needs [Go](https://go.dev/dl/) 1.26+):

```bash
go build -o pptoolkit.exe ./cmd/pptoolkit
```

## Using it

### On your own computer

Double-click `pptoolkit.exe` (or run `pptoolkit` in a terminal). A small
console window opens and the app appears in your browser at
`http://127.0.0.1:5000`. Close the console window to stop it.

1. **Convert**: choose Prompts or Slides, pick a style, choose the PDF, then
   **Read PDF**. Check the list, remove anything you don't want, and
   **Download .pro**.
2. Open the downloaded file in ProPresenter.

### Style Manager

A *style* is a few real slides copied out of one of your own `.pro` files, plus
a couple of settings (text colour, highlight colour, whether to strip forced ALL
CAPS). Because new slides are made by cloning those slides and swapping only
the text, the font, box position and size, background and shadow all come
straight from your example. You never have to describe the look.

1. **Style Manager → New style from a .pro file**.
2. Choose *Prompts* or *Slides* and upload an example show file.
3. Every slide in the file is listed. Click the role buttons on the slide that
   should be copied for each role:
   - **Prompts**: *Body*, a normal single-text-box slide.
   - **Slides**: *Title*, *Point*, *Keyword* (used for Condition/Driven lines)
     and *Scripture* (a slide with a reference box and a verse box). Optionally,
     *"Back to" look* for the slides created by `Back to …` lines.

   Any **Audience Look** attached to the slide you pick is captured
   automatically. If the slide has none, that role simply gets no look action.
4. Name the style, pick colours, **Save style**.

Styles are plain JSON files, stored by default in
`%AppData%\propresenter-toolkit\styles` on Windows
(`~/.config/propresenter-toolkit/styles` on Linux,
`~/Library/Application Support/propresenter-toolkit/styles` on Mac).
The Style Manager page shows the exact folder. Back that folder up, or use each
style's **Download** button.

Two example styles are built in and copied into that folder on first run:
`Message (Prompts)` and `2026-09-27 (Slides)`. Style files from the earlier
Python version of this tool work as-is; just copy them into the folder.

#### Known gap: the "Point L3" look in the example Slides style

The sample file used to build `2026-09-27 (Slides)` never contained a slide
using the "Point L3" Audience Look, so that role has a **placeholder UUID** that
won't resolve in ProPresenter. Fix it once by making a new Slides style in the
Style Manager from a `.pro` that *does* have a slide set to "Point L3", clicking
**Point** on that slide.

### From the command line

```bash
pptoolkit convert -mode prompts -style "Message (Prompts)" "Sunday - Prompts.pdf"
```

```bash
pptoolkit convert -mode slides -name "The Calling" notes.pdf out.pro
```

```bash
pptoolkit cues "Some Show.pro"
```

`-style` can be left out when only one style of that kind exists. `cues` lists
every slide in a `.pro` with its text and Audience Look, which is handy for
checking a file.

## Hosting it for a team

`pptoolkit serve` listens on all interfaces, port 5000. Settings can be passed as flags or environment variables:

| Flag | Env var | Default |
|---|---|---|
| `-addr` | `PPT_ADDR` | `:5000` |
| `-styles` | `PPT_STYLES_DIR` | per-user config folder (above) |
| `-auth user:password` | `PPT_AUTH` | none: anyone who can reach it can use it |

In-progress conversions are kept in memory (for 6 hours), and styles are JSON files on
disk. There's no database.

### Docker

```bash
docker compose up -d --build
```

Then open `http://<host>:5000`. Styles live in the `styles-data` volume, so
they survive rebuilds. Uncomment `PPT_AUTH` in `docker-compose.yml` to require a
login. In a Proxmox LXC, Docker needs a privileged container with nesting
enabled. If you'd rather avoid that, use the plain LXC route below.

### Plain Proxmox LXC (no Docker)

In a Debian/Ubuntu LXC, as root:

1. Put the `pptoolkit-linux-amd64` release file in the `deploy/` folder,
   renamed to `pptoolkit`.
2. Copy the `deploy/` folder into the container.
3. Run `sh install-lxc.sh`, or `PPT_AUTH="team:change-me" sh install-lxc.sh`
   to require a login.

That installs a `propresenter-toolkit` systemd service running as an
unprivileged user, with styles in `/var/lib/propresenter-toolkit/styles`.
Check it with `systemctl status propresenter-toolkit`.

### Reaching it

- **Same network**: `http://<container-ip>:5000`.
- **From outside, privately**: [Tailscale](https://tailscale.com) inside the container.
- **Public URL**: put Caddy or nginx in front for HTTPS, and set `PPT_AUTH`.

## Honest limitations

- **Verse pacing isn't automatic.** A long passage lands on one Scripture
  slide. Split it in ProPresenter if you want a verse-by-verse reveal.
- **Bold detection is font-name based**: font names containing `Heavy`,
  `Black` or `Bold`. That matches every sample so far. If another PDF export
  names fonts differently, add to `BoldFontMarkers` in
  `internal/pdftext/pdftext.go`.
- **Only text is generated.** Backgrounds, media and props come from the
  template slide as-is; images marked `Image:` are left for you to add.

## Development

```text
cmd/pptoolkit/        entry point: desktop launcher, serve, convert, cues
internal/
  pdftext/            PDF -> visual lines of words, with bold flags
  parse/              lines -> Prompts slides / Notes entries
  rtf/                the RTF dialect ProPresenter writes (+ plain-text preview)
  style/              style profiles (JSON) and the styles folder; bundled defaults
  pro/                read .pro files; build new ones from a style + parsed PDF
  convert/            the PDF -> review -> .pro pipeline shared by web and CLI
  web/                HTTP handlers, templates and CSS (embedded in the binary)
  pb/                 Go code generated from proto/ - do not edit
  testpdf/            generates tiny PDFs so tests don't need real sermon files
proto/                ProPresenter 7 protobuf schema (see "Credits")
deploy/               LXC install script
```

```bash
go test ./...
```

```bash
go run ./cmd/pptoolkit
```

**Regenerating the protobuf code** (only after updating `proto/`):

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
```

```bash
go install github.com/bufbuild/buf/cmd/buf@latest
```

```bash
buf generate
```

**Releasing**: push a tag like `v1.0.0`. GitHub Actions runs the tests, builds
binaries for Windows, Mac and Linux, and attaches them to a GitHub Release.

### Changes from the Python version

This is a Go rewrite of an earlier Python tool (Tkinter desktop app + Flask web
app). The `.pro` output was checked against the Python version on the same
PDFs and styles and is identical apart from these fixes:

- Words sitting slightly off the baseline no longer break onto their own line
  (Python produced *"If we were / to / consider…"*).
- Words with accented letters or hyphens no longer split (*"poi ē ma"* →
  *"poiēma"*, *"well- adjusted"* → *"well-adjusted"*), and no stray space after
  opening quotes.
- Typographic ligatures (`ﬁ`, `ﬂ`) become plain letters.
- A `SLIDE 26` marker missing its colon is still recognised.
- Slide label colours always get full opacity.

Also new: optional login, custom colours per style, style download/delete,
and warnings about skipped images and unmatched "Back to" lines shown
*before* you download rather than after.

## Credits

The ProPresenter 7 protobuf definitions in `proto/` are from
[greyshirtguy/ProPresenter7-Proto](https://github.com/greyshirtguy/ProPresenter7-Proto)
(MIT licence, see `proto/LICENSE`). ProPresenter is a product of Renewed Vision;
this project is not affiliated with them.
