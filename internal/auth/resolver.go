package auth

// Resolver exposes the current authoritative credential truth to other domains
// without leaking UI summary details.
type Resolver interface {
	Resolve() (CredentialTruth, bool)
}
