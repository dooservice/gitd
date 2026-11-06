use crate::client::Gitd;
use crate::error::Error;
use crate::follow::Follow;
use crate::proto::revision::Target as RevisionTarget;
use crate::proto::{
    CheckoutRequest, DescribeRequest, DescribeResponse, ListWarmRequest, PrepareRequest,
    PublishRequest, PublishResponse, RemoveRequest, Revision, SealRequest, UpdateRequest,
    WarmLayer, WorkspaceResult,
};
use crate::publish_options::PublishOptions;
use crate::workspace_options::{CheckoutOptions, PrepareOptions, UpdateOptions};

impl Gitd {
    /// Makes a workspace of the repository at `url`, at `path` under one of gitd's roots,
    /// from the repository's pool: in milliseconds by overlay or reflink where the machine
    /// allows, by copy elsewhere.
    pub async fn prepare(
        &self,
        url: impl Into<String>,
        path: impl Into<String>,
        options: PrepareOptions,
    ) -> Result<WorkspaceResult, Error> {
        let target = match (options.commit, options.branch) {
            (Some(commit), _) => Some(RevisionTarget::Commit(commit)),
            (None, Some(branch)) => Some(RevisionTarget::Branch(branch)),
            (None, None) => None,
        };
        let request = PrepareRequest {
            url: url.into(),
            path: path.into(),
            revision: target.map(|target| Revision {
                target: Some(target),
            }),
            with_submodules: options.submodules,
            full_history: options.full_history,
            strategy: options.strategy.unwrap_or_default() as i32,
            warm: options.warm.unwrap_or_default(),
            new_branch: options.new_branch.unwrap_or_default(),
            ..PrepareRequest::default()
        };
        let stream = self.workspaces().prepare(request).await?.into_inner();
        Follow::new(options.on_progress).result(stream).await
    }

    /// Brings a workspace to the tip of its branch, or of the branch given; with `merge`,
    /// merges that branch into the one it is on.
    pub async fn update(
        &self,
        path: impl Into<String>,
        options: UpdateOptions,
    ) -> Result<WorkspaceResult, Error> {
        let request = UpdateRequest {
            path: path.into(),
            branch: options.branch.unwrap_or_default(),
            with_submodules: options.submodules,
            credentials: Vec::new(),
            merge: options.merge.unwrap_or_default(),
        };
        let stream = self.workspaces().update(request).await?.into_inner();
        Follow::new(options.on_progress).result(stream).await
    }

    /// Moves a workspace to a commit, as a rollback does.
    pub async fn checkout(
        &self,
        path: impl Into<String>,
        commit: impl Into<String>,
        options: CheckoutOptions,
    ) -> Result<WorkspaceResult, Error> {
        let request = CheckoutRequest {
            path: path.into(),
            commit: commit.into(),
            with_submodules: options.submodules,
            credentials: Vec::new(),
        };
        let stream = self.workspaces().checkout(request).await?.into_inner();
        Follow::new(options.on_progress).result(stream).await
    }

    /// What a workspace is on disk right now, without the network.
    pub async fn describe(&self, path: impl Into<String>) -> Result<DescribeResponse, Error> {
        Ok(self
            .workspaces()
            .describe(DescribeRequest { path: path.into() })
            .await?
            .into_inner())
    }

    /// Pushes what was committed in a workspace to a new branch, or to the branch it is on
    /// when `branch` is empty, with the credential gitd keeps, and opens its pull request
    /// when the publishing settings or the options say so; with `update`, or on a branch
    /// this workspace published before, moves that branch forward and updates its pull
    /// request.
    pub async fn publish(
        &self,
        path: impl Into<String>,
        branch: impl Into<String>,
        options: PublishOptions,
    ) -> Result<PublishResponse, Error> {
        let request = PublishRequest {
            path: path.into(),
            branch: branch.into(),
            credentials: Vec::new(),
            requested_by: options.requested_by,
            pull_request: options.pull_request,
            provenance: options.provenance,
            sandbox: options.sandbox,
            update: options.update,
        };
        Ok(self.workspaces().publish(request).await?.into_inner())
    }

    /// Deletes a workspace; its pool stays.
    pub async fn remove(&self, path: impl Into<String>) -> Result<(), Error> {
        self.workspaces()
            .remove(RemoveRequest { path: path.into() })
            .await?;
        Ok(())
    }

    /// Seals what was installed in an overlay workspace as the warm layer `name` of its
    /// repository, for workspaces made later at the same commit to stand on. The layer is
    /// moved, not copied: the workspace is removed.
    pub async fn seal(
        &self,
        path: impl Into<String>,
        name: impl Into<String>,
    ) -> Result<WarmLayer, Error> {
        let request = SealRequest {
            path: path.into(),
            name: name.into(),
        };
        let sealed = self.workspaces().seal(request).await?.into_inner();
        sealed.warm.ok_or(Error::NoResult)
    }

    /// The warm layers of a repository.
    pub async fn warm_layers(&self, url: impl Into<String>) -> Result<Vec<WarmLayer>, Error> {
        Ok(self
            .workspaces()
            .list_warm(ListWarmRequest { url: url.into() })
            .await?
            .into_inner()
            .warm)
    }
}
