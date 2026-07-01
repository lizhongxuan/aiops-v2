package publicweb

import "context"

type Broker struct {
	backend SearchBackend
	fetcher Fetcher
}

func NewBroker(backend SearchBackend, fetcher Fetcher) *Broker {
	return &Broker{backend: backend, fetcher: fetcher}
}

func (b *Broker) Execute(ctx context.Context, req SearchRequest) (ResultEnvelope, error) {
	switch req.Operation {
	case OperationOpen:
		result, err := b.fetcher.Fetch(ctx, FetchRequest{
			URL:            req.URL,
			AllowedDomains: req.AllowedDomains,
			BlockedDomains: req.BlockedDomains,
			MaxBytes:       req.MaxBytes,
			Timeout:        req.Timeout,
		})
		if err != nil {
			return ResultEnvelope{}, err
		}
		return FormatOpenEnvelope(req, result, ResultMeta{Backend: "internal_fetch"}), nil
	case OperationSearch:
		results, err := b.backend.Search(ctx, req)
		if err != nil {
			return ResultEnvelope{}, err
		}
		meta := ResultMeta{Backend: b.backend.Name()}
		if req.FetchContent && b.fetcher != nil {
			limit := req.MaxContentResults
			if limit <= 0 || limit > len(results) {
				limit = len(results)
			}
			for i := 0; i < limit; i++ {
				fetched, fetchErr := b.fetcher.Fetch(ctx, FetchRequest{
					URL:            results[i].URL,
					AllowedDomains: req.AllowedDomains,
					BlockedDomains: req.BlockedDomains,
					MaxBytes:       req.MaxBytes,
					Timeout:        req.Timeout,
				})
				if fetchErr != nil {
					results[i].FetchError = fetchErr.Error()
					continue
				}
				results[i].Text = fetched.Text
				results[i].Markdown = fetched.Markdown
				results[i].Fetched = fetched.Fetched
				results[i].StatusCode = fetched.StatusCode
				results[i].ContentType = fetched.ContentType
				results[i].FetchedAt = fetched.FetchedAt
				meta.FetchedCount++
			}
		}
		return FormatSearchEnvelope(req, results, meta), nil
	default:
		return ResultEnvelope{}, ErrUnsupportedOperation(req.Operation)
	}
}

type ErrUnsupportedOperation string

func (e ErrUnsupportedOperation) Error() string {
	return "unsupported public web operation " + string(e)
}
