package incidents

import "time"

type TimelineItem struct {
	At      time.Time `json:"at"`
	Summary string    `json:"summary"`
}

type PostmortemDraft struct {
	Timeline   []TimelineItem `json:"timeline,omitempty"`
	Impact     string         `json:"impact,omitempty"`
	RootCause  string         `json:"rootCause,omitempty"`
	Mitigation string         `json:"mitigation,omitempty"`
	FollowUps  []string       `json:"followUps,omitempty"`
}

type PostmortemService struct{}

func NewPostmortemService() *PostmortemService {
	return &PostmortemService{}
}

func (s *PostmortemService) Draft(incident IncidentCase, evidence []EvidenceRef, req CloseRequest, now time.Time) PostmortemDraft {
	impact := incident.BusinessCapability
	if impact == "" {
		impact = incident.Title
	}
	timeline := []TimelineItem{{At: incident.CreatedAt, Summary: "incident opened"}}
	for _, item := range evidence {
		timeline = append(timeline, TimelineItem{At: item.CreatedAt, Summary: item.Summary})
	}
	timeline = append(timeline, TimelineItem{At: now, Summary: "incident closed"})
	return PostmortemDraft{
		Timeline:   timeline,
		Impact:     impact,
		RootCause:  req.RootCause,
		Mitigation: req.Mitigation,
		FollowUps:  append([]string(nil), req.FollowUps...),
	}
}
