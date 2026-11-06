use crate::proto::{Provenance, PullRequestOptions};

/// How [`Gitd::publish`](crate::Gitd::publish) publishes a workspace. By default: to a new
/// branch, its pull request opened only when the publishing settings say so.
#[derive(Clone, Debug, Default)]
pub struct PublishOptions {
    /// Who asked for the work, as their account on the host.
    pub requested_by: String,
    /// The pull request to open or update.
    pub pull_request: Option<PullRequestOptions>,
    /// Which agent, model and session did the work, and for which task.
    pub provenance: Option<Provenance>,
    /// With the workspace's path empty: the sandbox whose pushed branch to publish.
    pub sandbox: String,
    /// Move an earlier publish's branch forward, only ever as a fast-forward, and update
    /// its pull request rather than open another.
    pub update: bool,
}
