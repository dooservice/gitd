use tokio_stream::{Stream, StreamExt as _};

use crate::client::Gitd;
use crate::error::Error;
use crate::proto::{
    ChangesRequest, ChangesResponse, CompareRequest, CompareResponse, Conflict, ConflictsRequest,
    DiffRequest, DiffResponse, Side, WatchRequest, WatchResponse,
};
use crate::review_options::{CompareOptions, DiffOptions};

impl Gitd {
    /// What was changed in a workspace since gitd put it on its commit, committed or not:
    /// each file added, modified or deleted, with its lines.
    pub async fn changes(
        &self,
        path: impl Into<String>,
        include_ignored: bool,
    ) -> Result<ChangesResponse, Error> {
        let request = ChangesRequest {
            path: path.into(),
            include_ignored,
        };
        Ok(self.workspaces().changes(request).await?.into_inner())
    }

    /// The unified diff of a workspace's changes, of some files or all, cut short at a size.
    pub async fn diff(
        &self,
        path: impl Into<String>,
        options: DiffOptions,
    ) -> Result<DiffResponse, Error> {
        let request = DiffRequest {
            path: path.into(),
            files: options.files,
            max_bytes: options.max_bytes,
            include_ignored: options.include_ignored,
        };
        Ok(self.workspaces().diff(request).await?.into_inner())
    }

    /// How two works of one repository differ, from `first` to `second`, file by file.
    pub async fn compare(
        &self,
        first: Side,
        second: Side,
        options: CompareOptions,
    ) -> Result<CompareResponse, Error> {
        let request = CompareRequest {
            first: Some(first),
            second: Some(second),
            url: options.url,
            include_ignored: options.include_ignored,
            with_diff: options.with_diff,
            max_bytes: options.max_bytes,
        };
        Ok(self.workspaces().compare(request).await?.into_inner())
    }

    /// Which pairs of works do not merge cleanly, and on which files; a workspace stands for
    /// what it committed. `url` is the repository when every side is a branch.
    pub async fn conflicts(
        &self,
        sides: Vec<Side>,
        url: impl Into<String>,
    ) -> Result<Vec<Conflict>, Error> {
        let request = ConflictsRequest {
            sides,
            url: url.into(),
        };
        Ok(self
            .workspaces()
            .conflicts(request)
            .await?
            .into_inner()
            .conflicts)
    }

    /// What happens to the workspaces at `paths`, or to every one when empty, as it happens,
    /// until the stream is dropped.
    pub async fn watch(
        &self,
        paths: Vec<String>,
    ) -> Result<impl Stream<Item = Result<WatchResponse, Error>> + use<>, Error> {
        let stream = self
            .workspaces()
            .watch(WatchRequest { paths })
            .await?
            .into_inner();
        Ok(stream.map(|event| event.map_err(Error::from)))
    }
}
