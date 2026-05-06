package appui

import (
	"context"
	"os"
	"strings"
	"time"

	"aiops-v2/internal/actionproposal"
	"aiops-v2/internal/runbooks"
)

type RunbookView = runbooks.Runbook
type RunbookCandidateView = runbooks.Candidate
type RunbookInstanceView = runbooks.RunbookInstance
type RunbookMatchCommand = runbooks.MatchRequest

type RunbookService interface {
	List(ctx context.Context) ([]RunbookView, error)
	Get(ctx context.Context, id string) (RunbookView, bool)
	Match(ctx context.Context, cmd RunbookMatchCommand) ([]RunbookCandidateView, error)
	Instances(ctx context.Context, status string) ([]RunbookInstanceView, error)
}

type defaultRunbookService struct {
	domain  *runbooks.Service
	loadErr error
}

func NewRunbookService(pattern string, signer *actionproposal.Signer) RunbookService {
	if strings.TrimSpace(pattern) == "" {
		pattern = "runbooks/erp/*.yaml"
	}
	catalog, err := runbooks.LoadCatalog(projectRelativePath(pattern))
	if err != nil {
		catalog = runbooks.NewCatalog(nil)
	}
	if signer == nil {
		secret := strings.TrimSpace(os.Getenv("AIOPS_ACTION_TOKEN_SECRET"))
		if secret != "" {
			signer = actionproposal.NewSigner([]byte(secret), time.Now)
		}
	}
	return &defaultRunbookService{
		domain:  runbooks.NewService(catalog, signer, runbooks.NewInMemoryInstanceStore(), time.Now),
		loadErr: err,
	}
}

func (s *defaultRunbookService) List(context.Context) ([]RunbookView, error) {
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	return s.domain.List(), nil
}

func (s *defaultRunbookService) Get(_ context.Context, id string) (RunbookView, bool) {
	if s.loadErr != nil {
		return RunbookView{}, false
	}
	return s.domain.GetRunbook(id)
}

func (s *defaultRunbookService) Match(_ context.Context, cmd RunbookMatchCommand) ([]RunbookCandidateView, error) {
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	return s.domain.Match(cmd), nil
}

func (s *defaultRunbookService) Instances(_ context.Context, status string) ([]RunbookInstanceView, error) {
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	return s.domain.Instances(status), nil
}
