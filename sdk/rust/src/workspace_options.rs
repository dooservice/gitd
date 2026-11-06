use crate::follow::OnProgress;
use crate::proto::Strategy;

/// How [`Gitd::prepare`](crate::Gitd::prepare) makes a workspace. By default: at the
/// repository's default branch, without submodules or history, the fastest way this
/// machine allows.
#[derive(Default)]
pub struct PrepareOptions {
    /// The branch to check out; ignored when `commit` is given.
    pub branch: Option<String>,
    /// The commit to check out.
    pub commit: Option<String>,
    /// Check out the submodules too.
    pub submodules: bool,
    /// The whole history, for `git log` and `git blame` to go back as far as in a clone.
    pub full_history: bool,
    /// A preference: gitd falls back to copy when it cannot make one that way here.
    pub strategy: Option<Strategy>,
    /// A warm layer to stand on, its dependencies installed already: the workspace is then
    /// made by overlay, at the commit the layer was sealed at, or not at all.
    pub warm: Option<String>,
    /// Told what git is doing while the workspace is made.
    pub on_progress: Option<OnProgress>,
}

/// How [`Gitd::update`](crate::Gitd::update) moves a workspace.
#[derive(Default)]
pub struct UpdateOptions {
    /// The branch to follow; by default the one it follows.
    pub branch: Option<String>,
    /// Bring the submodules along.
    pub submodules: bool,
    /// Told what git is doing meanwhile.
    pub on_progress: Option<OnProgress>,
}

/// How [`Gitd::checkout`](crate::Gitd::checkout) moves a workspace.
#[derive(Default)]
pub struct CheckoutOptions {
    /// Bring the submodules along.
    pub submodules: bool,
    /// Told what git is doing meanwhile.
    pub on_progress: Option<OnProgress>,
}
