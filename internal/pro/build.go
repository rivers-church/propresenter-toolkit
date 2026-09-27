package pro

import (
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

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

// withLabel sets a slide action's label text and color.
func withLabel(a *pb.Action, text string, color *pb.Color) {
	if a.Label == nil {
		a.Label = &pb.Action_Label{}
	}
	a.Label.Text = text
	a.Label.Color = proto.Clone(color).(*pb.Color)
}

// ---------------------------------------------------------------- prompts --

// BuildPrompts makes one slide per parsed prompts section, cloning the
// style's "body" template. Bold words get the emphasis color.
func BuildPrompts(st *style.Profile, slides []parse.PromptSlide, name string) ([]byte, error) {
	tmpl, err := st.TemplateCue(style.RoleBody)
	if err != nil {
		return nil, err
	}
	if _, _, err := slideParts(tmpl, 1); err != nil {
		return nil, fmt.Errorf("style %q body template: %w", st.Name, err)
	}
	colors := []string{st.Settings.Regular(), st.Settings.Emphasis()}

	b := newBuilder(name)
	for n, slide := range slides {
		cue := proto.Clone(tmpl).(*pb.Cue)
		cue.Name = fmt.Sprintf("Slide %d", n+1)
		_, els, _ := slideParts(cue, 1)
		el := els[0]
		attrs := el.GetText().GetAttributes()
		if st.Settings.CapsOff() {
			attrs.Capitalization = pb.Graphics_Text_Attributes_CAPITALIZATION_NONE
		}
		attrs.CustomAttributes = nil

		var lines []rtf.Line
		for _, line := range slide {
			var l rtf.Line
			for _, r := range line {
				color := 0
				if r.Bold {
					color = 1
				}
				l = append(l, rtf.Run{Text: r.Text, Color: color, SpaceBefore: r.SpaceBefore})
			}
			lines = append(lines, l)
		}
		data, err := rtf.Build(rtf.StyleFromElement(el), colors, lines)
		if err != nil {
			return nil, err
		}
		el.Text.RtfData = data
		b.add(cue)
	}
	return b.bytes()
}

// ----------------------------------------------------------------- slides --

// BuildSlides makes slides from a parsed notes outline, cloning the
// style's title / point / keyword / scripture templates.
func BuildSlides(st *style.Profile, entries []parse.Entry, name string) ([]byte, Report, error) {
	var rep Report
	color := st.Settings.Regular()
	looks := st.Settings.AudienceLooks
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
		withLabel(action, label, labelRed)
		els[0].Text.Attributes.CustomAttributes = nil
		data, err := rtf.Plain(rtf.StyleFromElement(els[0]), color, text)
		if err != nil {
			return 0, err
		}
		els[0].Text.RtfData = data
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
		withLabel(action, ref, labelWhite)
		for i, text := range []string{ref, verse} {
			els[i].Text.Attributes.CustomAttributes = nil
			data, err := rtf.Plain(rtf.StyleFromElement(els[i]), color, text)
			if err != nil {
				return 0, err
			}
			els[i].Text.RtfData = data
		}
		return finish(tmpl, action, style.RoleScripture), nil
	}

	duplicate := func(src int, label string) {
		cue := proto.Clone(b.p.Cues[src]).(*pb.Cue)
		cue.Name = label
		action := SlideAction(cue)
		withLabel(action, label, labelWhite)
		finish(cue, action, style.RoleBackTo)
	}

	for _, e := range entries {
		switch e.Kind {
		case parse.KindTitle:
			idx, err := addText(style.RoleTitle, e.Text, "Title")
			if err != nil {
				return nil, rep, err
			}
			byLabel["title"] = idx
		case parse.KindPoint:
			role := style.RoleKeyword
			if strings.HasPrefix(strings.ToLower(e.Label), "point") {
				role = style.RolePoint
			}
			idx, err := addText(role, e.Text, e.Label)
			if err != nil {
				return nil, rep, err
			}
			byLabel[strings.ToLower(e.Label)] = idx
		case parse.KindScripture:
			idx, err := addScripture(e.Reference, e.Verse)
			if err != nil {
				return nil, rep, err
			}
			byLabel[strings.ToLower(e.Reference)] = idx
		case parse.KindImage:
			rep.SkippedImages = append(rep.SkippedImages, e.Text)
		case parse.KindBackTo:
			if idx, ok := byLabel[strings.ToLower(e.Ref)]; ok {
				duplicate(idx, e.Trigger)
			} else {
				rep.UnresolvedBackTo = append(rep.UnresolvedBackTo, "Back to "+e.Ref)
			}
		}
	}
	data, err := b.bytes()
	return data, rep, err
}
