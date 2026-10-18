package mounthelper

// Request is what gitd asks: Op "mount", with all four directories, or "unmount", with the
// target only. Lower is one template, or a warm layer and its template joined by a colon,
// the topmost first; each is checked on its own.
type Request struct {
	Op     string `json:"op"`
	Lower  string `json:"lower,omitempty"`
	Upper  string `json:"upper,omitempty"`
	Work   string `json:"work,omitempty"`
	Target string `json:"target"`
}

// Answer is the outcome: empty Error when done.
type Answer struct {
	Error string `json:"error,omitempty"`
}
