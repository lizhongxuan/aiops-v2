package appui

import "context"

type defaultStateService struct {
	sessions SessionSource
	builder  *SnapshotBuilder
}

func NewStateService(sessions SessionSource, builder *SnapshotBuilder) StateService {
	return &defaultStateService{
		sessions: sessions,
		builder:  builder,
	}
}

func (s *defaultStateService) GetState(context.Context) (StateSnapshot, error) {
	if s.sessions == nil {
		return s.builder.BuildStateSnapshot(nil), nil
	}
	return s.builder.BuildStateSnapshot(latestUserVisibleSession(s.sessions)), nil
}
