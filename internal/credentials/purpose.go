package credentials

// Purpose is what an access to a repository is for, which is what a credential that is
// made for each operation — a GitHub App's installation token — is limited to.
type Purpose int

const (
	// Read fetches: prepare, update, checkout, probe.
	Read Purpose = iota
	// Write pushes a published branch.
	Write
	// PullRequests opens a pull request and asks for its reviewers and labels.
	PullRequests
)

func (purpose Purpose) String() string {
	switch purpose {
	case Write:
		return "write"
	case PullRequests:
		return "pull requests"
	default:
		return "read"
	}
}
