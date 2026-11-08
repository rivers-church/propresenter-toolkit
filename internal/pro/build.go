package pro

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf16"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	"github.com/rivers-church/propresenter-toolkit/internal/metrics"
	"github.com/rivers-church/propresenter-toolkit/internal/parse"
	"github.com/rivers-church/propresenter-toolkit/internal/pb"
	"github.com/rivers-church/propresenter-toolkit/internal/rtf"
	"github.com/rivers-church/propresenter-toolkit/internal/style"
)

const zeroUUID = "00000000-0000-0000-0000-000000000000"

var labelRed = &pb.Color{Red: 1, Green: 0, Blue: 0, Alpha: 1}
var labelWhite = &pb.Color{Red: 1, Green: 1, Blue: 1, Alpha: 1}

// Report lists things the user should know about after a build.
type Report struct {
	SkippedImages    []string // "Image:" lines - add these by hand in ProPresenter
	UnresolvedBackTo []string // "Back to X" whose X never appeared earlier
}

// Warnings renders the report as human-readable lines.
func (r Report) Warnings() []string {
	var w []string
	if len(r.SkippedImages) > 0 {
		w = append(w, "Images skipped (add these by hand in ProPresenter): "+strings.Join(r.SkippedImages, "; "))
	}
	if len(r.UnresolvedBackTo) > 0 {
		w = append(w, "\"Back to\" slides with no earlier match (not created): "+strings.Join(r.UnresolvedBackTo, "; "))
	}
	return w
}

func newUUID() *pb.UUID { return &pb.UUID{String_: uuid.NewString()} }

// builder accumulates cues into a fresh presentation with one cue group.
type builder struct {
	p     *pb.Presentation
	group *pb.Presentation_CueGroup
}

func newBuilder(name string) *builder {
	g := &pb.Presentation_CueGroup{Group: &pb.Group{Uuid: newUUID(), Name: "Slides"}}
	return &builder{
		p:     &pb.Presentation{Name: name, Uuid: newUUID(), CueGroups: []*pb.Presentation_CueGroup{g}},
		group: g,
	}
}

// add gives the cue fresh identities and appends it; returns its index.
func (b *builder) add(c *pb.Cue) int {
	freshUUIDs(c)
	b.p.Cues = append(b.p.Cues, c)
	b.group.CueIdentifiers = append(b.group.CueIdentifiers, &pb.UUID{String_: c.GetUuid().GetString_()})
	return len(b.p.Cues) - 1
}

func (b *builder) bytes() ([]byte, error) { return proto.Marshal(b.p) }

// freshUUIDs replaces every identity copied from the template, so
// ProPresenter treats each generated cue, slide and element as new.
func freshUUIDs(c *pb.Cue) {
	c.Uuid = newUUID()
	c.CompletionTargetUuid = &pb.UUID{String_: zeroUUID}
	c.CompletionActionUuid = &pb.UUID{String_: zeroUUID}
	for _, a := range c.GetActions() {
		a.Uuid = newUUID()
		if a.GetType() == pb.Action_ACTION_TYPE_PRESENTATION_SLIDE {
			base := a.GetSlide().GetPresentation().GetBaseSlide()
			if base != nil {
				base.Uuid = newUUID()
				for _, el := range base.GetElements() {
					if el.GetElement() != nil {
						el.Element.Uuid = newUUID()
					}
				}
			}
		}
	}
}

func audienceLookAction(l style.Look) *pb.Action {
	return &pb.Action{
		Uuid:      newUUID(),
		IsEnabled: true,
		Type:      pb.Action_ACTION_TYPE_AUDIENCE_LOOK,
		ActionTypeData: &pb.Action_AudienceLook{AudienceLook: &pb.Action_AudienceLookType{
			Identification: &pb.CollectionElementType{
				ParameterUuid: &pb.UUID{String_: l.UUID},
				ParameterName: l.Name,
			},
		}},
	}
}

// slideParts returns a template cue's slide action and its text elements.
func slideParts(c *pb.Cue, minElements int) (*pb.Action, []*pb.Graphics_Element, error) {
	a := SlideAction(c)
	if a == nil {
		return nil, nil, errors.New("template cue has no slide action")
	}
	var els []*pb.Graphics_Element
	for _, e := range a.GetSlide().GetPresentation().GetBaseSlide().GetElements() {
		if e.GetElement() != nil {
			els = append(els, e.GetElement())
		}
	}
	if len(els) < minElements {
		return nil, nil, fmt.Errorf("template cue needs %d text box(es), has %d", minElements, len(els))
	}
	for _, el := range els[:minElements] {
		if el.GetText().GetAttributes() == nil {
			return nil, nil, errors.New("template cue's text box has no text attributes")
		}
	}
	return a, els, nil
}

// fitCustomAttributes keeps the template's whole-text capitalisation
// attributes (the per-range setting ProPresenter actually applies ALL CAPS
// from) and stretches them over the new text. The per-range font is dropped,
// so the text takes the text box's font like the RTF does, and ranges that
// covered only part of the template's text belonged to that text.
func fitCustomAttributes(attrs *pb.Graphics_Text_Attributes, text string) {
	end := int32(len(utf16.Encode([]rune(text))))
	var full int32
	for _, ca := range attrs.GetCustomAttributes() {
		if r := ca.GetRange(); r != nil && r.Start == 0 && r.End > full {
			full = r.End
		}
	}
	var keep []*pb.Graphics_Text_Attributes_CustomAttribute
	for _, ca := range attrs.GetCustomAttributes() {
		r := ca.GetRange()
		if r == nil || r.Start != 0 || r.End != full || ca.GetCapitalization() == pb.Graphics_Text_Attributes_CustomAttribute_CAPITALIZATION_NONE {
			continue
		}
		c := proto.Clone(ca).(*pb.Graphics_Text_Attributes_CustomAttribute)
		c.ProtoReflect().SetUnknown(nil) // the per-range font
		c.Range.End = end
		keep = append(keep, c)
	}
	attrs.CustomAttributes = keep
}

// labelColor returns the colour of the slide action's own label, so a style
// captured from a .pro keeps the label colours its slides were given there.
// A label with no colour (ProPresenter stores those as fully transparent)
// gets fallback.
func labelColor(a *pb.Action, fallback *pb.Color) *pb.Color {
	if c := a.GetLabel().GetColor(); c != nil && c.GetAlpha() > 0 {
		return c
	}
	return fallback
}

// withLabel sets a slide action's label text and color.
func withLabel(a *pb.Action, text string, color *pb.Color) {
	if a.Label == nil {
		a.Label = &pb.Action_Label{}
	}
	a.Label.Text = text
	a.Label.Color = proto.Clone(color).(*pb.Color)
}

// ---------------------------------------------------------------- prompts --

// PromptOptions tune how Prompts slides are coloured.
type PromptOptions struct {
	// PDFColors keeps the PDF's own text colours and italics (and turns
	// dark-on-light-box text into black on a white highlight) instead of
	// the style's regular/highlight colours.
	PDFColors bool
}

// BuildPrompts makes one slide per parsed prompts section, cloning the
// style's "body" template. By default bold words get the emphasis colour;
// see PromptOptions for keeping the PDF's colours instead.
func BuildPrompts(st *style.Profile, slides []parse.PromptSlide, name string, opt PromptOptions) ([]byte, error) {
	tmpl, err := st.TemplateCue(style.RoleBody)
	if err != nil {
		return nil, err
	}
	if _, _, err := slideParts(tmpl, 1); err != nil {
		return nil, fmt.Errorf("style %q body template: %w", st.Name, err)
	}
	highlight := st.Settings.Highlights() && !opt.PDFColors

	b := newBuilder(name)
	for n, slide := range slides {
		cue := proto.Clone(tmpl).(*pb.Cue)
		cue.Name = fmt.Sprintf("Slide %d", n+1)
		_, els, _ := slideParts(cue, 1)
		el := els[0]
		attrs := el.GetText().GetAttributes()
		if st.Settings.CapsOff() {
			attrs.Capitalization = pb.Graphics_Text_Attributes_CAPITALIZATION_NONE
			attrs.CustomAttributes = nil
		}

		colors := []string{st.Settings.Regular()}
		if highlight {
			colors = append(colors, st.Settings.Emphasis())
		}
		colorIndex := func(hex string) int {
			for i, c := range colors {
				if strings.EqualFold(c, hex) {
					return i
				}
			}
			colors = append(colors, hex)
			return len(colors) - 1
		}

		var lines []rtf.Line
		for _, line := range slide {
			var l rtf.Line
			for _, r := range line {
				run := rtf.Run{Text: r.Text, SpaceBefore: r.SpaceBefore}
				switch {
				case opt.PDFColors && r.Highlight:
					// Dark text the author put on a light box: black on white.
					run.Color = colorIndex("#000000")
					run.Highlight = colorIndex("#FFFFFF") + 1
					run.Italic = r.Italic
				case opt.PDFColors:
					if r.Color != "" {
						run.Color = colorIndex(r.Color)
					}
					run.Italic = r.Italic
				case r.Bold && highlight:
					run.Color = 1
				}
				l = append(l, run)
			}
			lines = append(lines, l)
		}
		data, err := rtf.Build(rtf.StyleFromElement(el), colors, lines)
		if err != nil {
			return nil, err
		}
		el.Text.RtfData = data
		if !st.Settings.CapsOff() {
			fitCustomAttributes(attrs, rtf.PlainText(data))
		}
		b.add(cue)
	}
	return b.bytes()
}

// TextBox describes a style's Prompts text box, for estimating how many
// lines text will wrap to.
func TextBox(st *style.Profile) (metrics.Box, error) {
	tmpl, err := st.TemplateCue(style.RoleBody)
	if err != nil {
		return metrics.Box{}, err
	}
	_, els, err := slideParts(tmpl, 1)
	if err != nil {
		return metrics.Box{}, err
	}
	el := els[0]
	m := el.GetText().GetMargins()
	return metrics.Box{
		Width:    el.GetBounds().GetSize().GetWidth() - m.GetLeft() - m.GetRight(),
		FontSize: el.GetText().GetAttributes().GetFont().GetSize(),
		AllCaps: !st.Settings.CapsOff() &&
			el.GetText().GetAttributes().GetCapitalization() == pb.Graphics_Text_Attributes_CAPITALIZATION_ALL_CAPS,
	}, nil
}

// scriptureBox describes a style's scripture verse box, for estimating how
// many lines a verse wraps to.
func scriptureBox(st *style.Profile) (metrics.Box, error) {
	tmpl, err := st.TemplateCue(style.RoleScripture)
	if err != nil {
		return metrics.Box{}, err
	}
	_, els, err := slideParts(tmpl, 2)
	if err != nil {
		return metrics.Box{}, err
	}
	el := els[1]
	m := el.GetText().GetMargins()
	attrs := el.GetText().GetAttributes()
	return metrics.Box{
		Width:    el.GetBounds().GetSize().GetWidth() - m.GetLeft() - m.GetRight(),
		FontSize: attrs.GetFont().GetSize(),
		AllCaps:  !st.Settings.CapsOff() && attrs.GetCapitalization() == pb.Graphics_Text_Attributes_CAPITALIZATION_ALL_CAPS,
	}, nil
}

// ----------------------------------------------------------------- slides --

// SlideOptions tune BuildSlides.
type SlideOptions struct {
	// DefaultCopies adds, straight after every title, point and subpoint
	// slide, an identical copy that carries the style's Default Audience
	// Look (its "backto" look) instead of the role's own.
	DefaultCopies bool
	// DisableCopies marks those copies disabled, so ProPresenter greys
	// them out and skips them when stepping through the presentation.
	DisableCopies bool
	// CenterScriptures fits the verse box to its text and centres the
	// verse and reference boxes together, vertically, on the slide.
	CenterScriptures bool
}

// lineHeightFactor is Arial's line height as a multiple of the font size,
// used to turn an estimated line count into a box height.
const lineHeightFactor = 1.15

// verseBoxPadding is extra height (points) on a fitted verse box. Without
// it ProPresenter clips the text: five lines come out at 288pt but need
// about 303pt, which is also what the template's own five-line box has.
const verseBoxPadding = 15

// centerVertically sizes the verse box to the lines its text wraps to, then
// moves it and the reference box, keeping their order and the gap between
// them, so the pair sits in the vertical middle of the slide.
func centerVertically(st *style.Profile, a *pb.Action, ref, verse *pb.Graphics_Element, text string) {
	slideH := a.GetSlide().GetPresentation().GetBaseSlide().GetSize().GetHeight()
	rb, vb := ref.GetBounds(), verse.GetBounds()
	if slideH <= 0 || rb.GetOrigin() == nil || rb.GetSize() == nil || vb.GetOrigin() == nil || vb.GetSize() == nil {
		return
	}
	tx := verse.GetText()
	m := tx.GetMargins()
	attrs := tx.GetAttributes()
	box := metrics.Box{
		Width:    vb.Size.Width - m.GetLeft() - m.GetRight(),
		FontSize: attrs.GetFont().GetSize(),
		AllCaps:  !st.Settings.CapsOff() && attrs.GetCapitalization() == pb.Graphics_Text_Attributes_CAPITALIZATION_ALL_CAPS,
	}
	if box.Width <= 0 || box.FontSize <= 0 {
		return
	}
	multiple := attrs.GetParagraphStyle().GetLineHeightMultiple()
	if multiple <= 0 {
		multiple = 1
	}
	height := float64(box.Lines(text))*box.FontSize*lineHeightFactor*multiple + m.GetTop() + m.GetBottom() + verseBoxPadding

	// A passage too long for the slide can't be made to fit; just keep the
	// boxes on the slide.
	height = math.Min(height, slideH-rb.Size.Height)

	// A verse box above the reference keeps its bottom edge (and the gap);
	// one below keeps its top edge.
	if vb.Origin.Y < rb.Origin.Y {
		vb.Origin.Y += vb.Size.Height - height
	}
	vb.Size.Height = height

	top := math.Min(vb.Origin.Y, rb.Origin.Y)
	bottom := math.Max(vb.Origin.Y+vb.Size.Height, rb.Origin.Y+rb.Size.Height)
	dy := (slideH-(bottom-top))/2 - top
	vb.Origin.Y += dy
	rb.Origin.Y += dy
}

// BuildSlides makes slides from a parsed notes outline, cloning the
// style's title / point / keyword / scripture templates.
func BuildSlides(st *style.Profile, entries []parse.Entry, name string, opt SlideOptions) ([]byte, Report, error) {
	var rep Report
	color := st.Settings.Regular()
	looks := st.Settings.AudienceLooks
	if opt.DefaultCopies {
		if l, ok := looks[style.RoleBackTo]; !ok || l.UUID == "" {
			return nil, rep, fmt.Errorf("style %q has no Default Audience Look to put on the copies (set the “Back to” look in the Style Manager)", st.Name)
		}
	}
	b := newBuilder(name)
	byLabel := map[string]int{} // lower-cased label -> cue index, for "Back to"

	// finish drops any look the template carried, adds the role's look,
	// and appends the cue.
	finish := func(cue *pb.Cue, action *pb.Action, lookRole string) int {
		cue.Actions = []*pb.Action{action}
		if l, ok := looks[lookRole]; ok && l.UUID != "" {
			cue.Actions = append(cue.Actions, audienceLookAction(l))
		}
		return b.add(cue)
	}

	addText := func(role, text, label string) (int, error) {
		tmpl, err := st.TemplateCue(role)
		if err != nil {
			return 0, err
		}
		action, els, err := slideParts(tmpl, 1)
		if err != nil {
			return 0, fmt.Errorf("style %q %s template: %w", st.Name, role, err)
		}
		tmpl.Name = label
		withLabel(action, label, labelColor(action, labelRed))
		data, err := rtf.Plain(rtf.StyleFromElement(els[0]), color, text)
		if err != nil {
			return 0, err
		}
		els[0].Text.RtfData = data
		fitCustomAttributes(els[0].Text.Attributes, text)
		return finish(tmpl, action, role), nil
	}

	addScripture := func(ref, verse string) (int, error) {
		tmpl, err := st.TemplateCue(style.RoleScripture)
		if err != nil {
			return 0, err
		}
		action, els, err := slideParts(tmpl, 2)
		if err != nil {
			return 0, fmt.Errorf("style %q scripture template (needs a reference box and a verse box): %w", st.Name, err)
		}
		tmpl.Name = ref
		withLabel(action, ref, labelColor(action, labelWhite))
		if opt.CenterScriptures {
			centerVertically(st, action, els[0], els[1], verse)
		}
		for i, text := range []string{ref, verse} {
			data, err := rtf.Plain(rtf.StyleFromElement(els[i]), color, text)
			if err != nil {
				return 0, err
			}
			els[i].Text.RtfData = data
			fitCustomAttributes(els[i].Text.Attributes, text)
		}
		return finish(tmpl, action, style.RoleScripture), nil
	}

	duplicate := func(src int, label string) {
		cue := proto.Clone(b.p.Cues[src]).(*pb.Cue)
		cue.Name = label
		action := SlideAction(cue)
		withLabel(action, label, labelColor(action, labelWhite))
		finish(cue, action, style.RoleBackTo)
	}

	// defaultCopy appends a duplicate of cue src with the Default look.
	defaultCopy := func(src int) {
		if !opt.DefaultCopies {
			return
		}
		cue := proto.Clone(b.p.Cues[src]).(*pb.Cue)
		cue.IsEnabled = !opt.DisableCopies
		finish(cue, SlideAction(cue), style.RoleBackTo)
	}

	for _, e := range entries {
		switch e.Kind {
		case parse.KindTitle:
			idx, err := addText(style.RoleTitle, e.Text, "Title")
			if err != nil {
				return nil, rep, err
			}
			byLabel["title"] = idx
			defaultCopy(idx)
		case parse.KindPoint:
			role := style.RoleKeyword
			if l := strings.ToLower(e.Label); strings.HasPrefix(l, "point") || strings.HasPrefix(l, "subpoint") {
				role = style.RolePoint
			}
			idx, err := addText(role, e.Text, e.Label)
			if err != nil {
				return nil, rep, err
			}
			byLabel[strings.ToLower(e.Label)] = idx
			defaultCopy(idx)
		case parse.KindScripture:
			// A long passage becomes several slides, each carrying the
			// reference; "Back to" returns to the first.
			parts := []string{e.Verse}
			if box, err := scriptureBox(st); err == nil {
				parts = parse.SplitVerse(e.Verse, box, st.Settings.ScriptureLines())
			}
			for n, part := range parts {
				idx, err := addScripture(e.Reference, part)
				if err != nil {
					return nil, rep, err
				}
				if n == 0 {
					byLabel[strings.ToLower(e.Reference)] = idx
				}
			}
		case parse.KindImage:
			rep.SkippedImages = append(rep.SkippedImages, e.Text)
		case parse.KindBackTo:
			if idx, ok := byLabel[strings.ToLower(e.Ref)]; ok {
				label := e.Trigger
				if label == "" {
					label = "Back to " + e.Ref
				}
				duplicate(idx, label)
			} else {
				rep.UnresolvedBackTo = append(rep.UnresolvedBackTo, "Back to "+e.Ref)
			}
		}
	}
	data, err := b.bytes()
	return data, rep, err
}
