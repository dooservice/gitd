package workspace

import (
	"regexp"
	"strings"
)

// The configuration keys a workspace may set — the only ones gitd runs git under. Everything
// else is refused (see guard), so a key git adds tomorrow is refused until it is listed here.
//
// To allow a key, add it to the group it belongs to, with a reason if it opens a new group.
// Never list a key whose value git runs as a command or reads as a path to load: filters,
// hooks, editors, pagers, ssh or proxy commands, includes, upload or receive packs, diff and
// merge drivers, credential helpers, fsmonitor, signing programs.
//
// Keys are written as git lists them: section and name lowercased, `<subsection>` for any
// subsection (which may hold dots).

// keyGroup is a group of allowed keys and why they are harmless.
type keyGroup struct {
	why  string
	keys []string
}

var allowedKeyGroups = []keyGroup{
	{
		why: "written by git init and git clone; they describe the repository's format",
		keys: []string{
			"core.repositoryformatversion", "core.filemode", "core.bare", "core.logallrefupdates",
			"core.ignorecase", "core.precomposeunicode", "core.symlinks",
			"extensions.objectformat", "extensions.refstorage",
		},
	},
	{
		why: "written by gitd itself: what the workspace is, and how its files are compared",
		keys: []string{
			"gitd.url", "gitd.pool", "gitd.history", "gitd.strategy", "gitd.branch", "gitd.poolinside",
			"core.checkstat", "core.trustctime",
		},
	},
	{
		why: "remotes, branches and submodules, as git remote, git branch and git submodule write them; their URLs are read only by git fetch and push, which gitd limits to HTTPS, SSH and its own pools",
		keys: []string{
			"remote.<subsection>.url", "remote.<subsection>.pushurl", "remote.<subsection>.fetch",
			"remote.<subsection>.push", "remote.<subsection>.tagopt", "remote.<subsection>.prune",
			"branch.<subsection>.remote", "branch.<subsection>.merge", "branch.<subsection>.rebase",
			"branch.<subsection>.pushremote", "branch.<subsection>.description",
			"submodule.<subsection>.url", "submodule.<subsection>.active", "submodule.<subsection>.branch",
		},
	},
	{
		why: "what an agent sets while it works: who it commits as, and how it pulls and pushes",
		keys: []string{
			"user.name", "user.email", "init.defaultbranch",
			"pull.rebase", "pull.ff", "push.default", "push.autosetupremote", "fetch.prune",
		},
	},
	{
		why: "how files and text are handled, without running anything",
		keys: []string{
			"core.autocrlf", "core.eol", "core.safecrlf", "core.quotepath",
			"core.untrackedcache", "core.commitgraph", "core.multipackindex",
		},
	},
	{
		why:  "only what a person sees; gitd runs git's own commands, never an alias",
		keys: []string{"alias.<subsection>", "color.<subsection>", "advice.<subsection>"},
	},
}

// moduleWorkTree is the one key a submodule's git directory sets beyond those: where its
// checkout is, which must name a directory of the workspace (see insideCheckout).
const moduleWorkTree = "core.worktree"

// allowedKeys matches every key of allowedKeyGroups.
var allowedKeys = compileKeys(allowedKeyGroups)

func compileKeys(groups []keyGroup) *regexp.Regexp {
	var patterns []string
	for _, group := range groups {
		for _, key := range group.keys {
			pattern := regexp.QuoteMeta(key)
			pattern = strings.ReplaceAll(pattern, regexp.QuoteMeta("<subsection>"), ".+")
			patterns = append(patterns, pattern)
		}
	}
	return regexp.MustCompile(`^(` + strings.Join(patterns, "|") + `)$`)
}
