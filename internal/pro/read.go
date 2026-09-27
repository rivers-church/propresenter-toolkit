// Package pro reads and writes ProPresenter 7 .pro files (a serialized
// rv.data.Presentation protobuf).
package pro

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"

	"github.com/rivers-church/propresenter-toolkit/internal/pb"
	"github.com/rivers-church/propresenter-toolkit/internal/rtf"
	"github.com/rivers-church/propresenter-toolkit/internal/style"
)

// Parse decodes a .pro file.
func Parse(data []byte) (*pb.Presentation, error) {
	p := &pb.Presentation{}
	if err := proto.Unmarshal(data, p); err != nil {
		return nil, fmt.Errorf("not a ProPresenter 7 .pro file: %w", err)
	}
	return p, nil
}

// CueInfo summarises one cue for the Style Manager's picker.
type CueInfo struct {
	Index    int
	Name     string
	Label    string
	Preview  string
	Elements int
	Look     *style.Look
}

// ListCues summarises every cue in a presentation.
func ListCues(p *pb.Presentation) []CueInfo {
	var out []CueInfo
	for i, c := range p.GetCues() {
		info := CueInfo{Index: i, Name: c.GetName(), Look: AudienceLook(c)}
		if a := SlideAction(c); a != nil {
			info.Label = a.GetLabel().GetText()
			els := a.GetSlide().GetPresentation().GetBaseSlide().GetElements()
			info.Elements = len(els)
			var parts []string
			for _, e := range els {
				if t := strings.TrimSpace(rtf.PlainText(e.GetElement().GetText().GetRtfData())); t != "" {
					parts = append(parts, t)
				}
			}
			info.Preview = truncate(strings.Join(parts, " | "), 120)
		}
		if info.Preview == "" {
			info.Preview = "(no text)"
		}
		out = append(out, info)
	}
	return out
}

// CueBytes returns cue i serialized, for storing as a style template.
func CueBytes(p *pb.Presentation, i int) ([]byte, error) {
	cues := p.GetCues()
	if i < 0 || i >= len(cues) {
		return nil, fmt.Errorf("cue %d out of range (file has %d)", i, len(cues))
	}
	return proto.Marshal(cues[i])
}

// SlideAction returns the cue's presentation-slide action, if any.
func SlideAction(c *pb.Cue) *pb.Action {
	for _, a := range c.GetActions() {
		if a.GetType() == pb.Action_ACTION_TYPE_PRESENTATION_SLIDE {
			return a
		}
	}
	return nil
}

// AudienceLook returns the Audience Look attached to a cue, if any.
func AudienceLook(c *pb.Cue) *style.Look {
	for _, a := range c.GetActions() {
		if a.GetType() == pb.Action_ACTION_TYPE_AUDIENCE_LOOK {
			id := a.GetAudienceLook().GetIdentification()
			return &style.Look{UUID: id.GetParameterUuid().GetString_(), Name: id.GetParameterName()}
		}
	}
	return nil
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
