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

- **Prompts**: either of two kinds of speaker notes.
  - **A script split into `SLIDE 1:`, `SLIDE 2:`, …** gives one slide per
    section. Bold words in the PDF are drawn in the style's highlight colour.
  - **Free-form notes** (no `SLIDE` markers) are split by their own layout:
    - Each block of lines separated by blank space becomes a slide, and a
      larger section heading goes at the top of the slide that follows it.
    - Centred heading lines keep their line breaks. Wrapped paragraphs such
      as scripture flow as one paragraph.
    - Blocks too long for one slide are split across several at sentence
      ends, to stay under a soft limit of **7 lines** at the template's font
      size. The limit can be changed on the Convert page.
    - The author's own **colours and italics** are kept. Text on a highlight
      box (dark text on a dark page) becomes black on a white highlight.
    - Small print such as the page header and date is dropped.

    Expect to tidy a few slides by hand, e.g. merging two short ones.

  The Convert page's **Colours** option switches between the PDF's colours
  and the style's colours. **Automatic** uses the PDF's for free-form notes
  and the style's for `SLIDE` scripts.
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

- uses the system's Go if it's new enough (1.26+), otherwise installs the
  official Go release into `/usr/local/go`;
- builds the app into `/opt/propresenter-toolkit/pptoolkit`;
- creates a `propresenter-toolkit` system user and a systemd service of the same name;
- writes settings to `/etc/propresenter-toolkit.env` (first run only);
- keeps styles in `/var/lib/propresenter-toolkit/styles`, seeded with the
  built-in styles (below).

It prints the address when it's done, e.g. `http://192.168.1.50`. The app
listens on port 80, so no `:port` is needed.

### A friendly address

1. Give the container a fixed IP, either as a static IP in Proxmox or a
   DHCP reservation.
2. In your DNS server, add an **A record** for a name, e.g. `propresenter`,
   pointing at that IP. On Windows Server: DNS Manager → your zone → New
   Host (A).

Everyone using that DNS server can then open `http://propresenter.<your-domain>`.
Nothing else is needed; a PTR (reverse) record is optional.

With no DNS server of your own, install `avahi-daemon` in the container
instead. It then answers at `http://<container-hostname>.local` on Macs,
iPhones and Windows 10/11.

### HTTPS (recommended)

Browsers increasingly try `https://` first, and a site that only speaks
HTTP then works on some devices and not others. If the domain's DNS is on
Cloudflare, the app can get and renew its own free Let's Encrypt certificate.
It proves ownership with a temporary DNS record, so this works even though
the site is only reachable inside your network.

1. In Cloudflare: **My Profile → API Tokens → Create Token → Create Custom
   Token**.
   - Permissions: **Zone / Zone / Read** and **Zone / DNS / Edit**.
   - Zone Resources: **Include / Specific zone / your domain** (e.g.
     `rivers.church`).
2. In `/etc/propresenter-toolkit.env`, set:

   ```text
   PPT_DOMAIN=protoolkit.rivers.church
   PPT_CLOUDFLARE_API_TOKEN=<the token>
   ```

3. Restart the service:

   ```bash
   systemctl restart propresenter-toolkit
   ```

   Watch the first certificate arrive (it takes a minute or two):

   ```bash
   journalctl -u propresenter-toolkit -f
   ```

The app then serves `https://protoolkit.rivers.church` on port 443, and plain
HTTP on port 80 redirects there. Certificates are kept in
`/var/lib/propresenter-toolkit/certs` and renew automatically. The container
needs outbound internet access, to reach Let's Encrypt, the Cloudflare API and
public DNS (`1.1.1.1` / `8.8.8.8`) for checking the record.

The internal DNS record pointing the name at the container stays as it is.
Nothing about the site becomes public.

### Settings

Edit `/etc/propresenter-toolkit.env`, then run
`systemctl restart propresenter-toolkit`:

| Setting | Default | |
|---|---|---|
| `PPT_ADDR` | `:80` | address and port to listen on |
| `PPT_STYLES_DIR` | `/var/lib/propresenter-toolkit/styles` | style profiles folder |
| `PPT_AUTH` | *(empty)* | `user:password` to require a login |
| `PPT_DOMAIN` | *(empty)* | domain for HTTPS (see above); empty = plain HTTP |
| `PPT_CLOUDFLARE_API_TOKEN` | *(empty)* | Cloudflare token for the HTTPS certificate |
| `PPT_ACME_EMAIL` | *(empty)* | optional address for certificate expiry notices |

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

## Built-in styles

| Style | Kind | Result |
|---|---|---|
| Message (Prompts) | Prompts | white text; bold words from the PDF in yellow |
| Message (Prompts, all yellow) | Prompts | all text yellow |
| Message (Prompts, plain) | Prompts | all text white, nothing highlighted |
| 2026-09-27 (Slides) | Slides | Title / Point / Keyword / Scripture slides with Audience Looks |

The three Prompts styles use the same template slide, so the font, size and box
are identical; only the colours differ. Your own styles can do the same with the
**Highlight bold words** option in the Style Manager.

Built-in styles are copied into the styles folder when the app starts. A
built-in style added in a later version appears automatically. One you've
deleted doesn't come back, and one you've edited is never overwritten.

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
- **Line counts are estimates** for free-form notes, based on Arial/Helvetica
  Bold character widths. They're accurate for the Message template; with other
  fonts a slide may run a line over or under.
- **Underlines aren't carried over.** Colours, italics and highlight boxes are.

## Development

Needs Go 1.26 or newer.

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
  pdftext/       PDF -> visual lines of words, with bold/italic flags and colours
  parse/         lines -> Prompts slides (SLIDE markers or free-form) / Notes entries
  metrics/       estimates how text wraps in a slide's text box
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
