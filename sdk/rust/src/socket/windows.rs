use std::io;
use std::path::PathBuf;
use std::sync::Arc;

use async_io::Async;
use hyper_util::rt::TokioIo;
use tonic::transport::{Channel, Endpoint, Uri};
use tower::service_fn;
use uds_windows::UnixStream;

use super::windows_stream::WindowsStream;

/// gitd's AF_UNIX socket on Windows, which tokio cannot dial: connected by `uds_windows`
/// on a blocking thread, then read and written through `async-io`'s reactor.
pub(crate) struct Socket {
    path: Arc<PathBuf>,
}

impl Socket {
    pub(crate) fn new(path: PathBuf) -> Self {
        Self {
            path: Arc::new(path),
        }
    }

    /// A channel whose every connection is made on the socket, the first one on the first call.
    pub(crate) fn channel(&self, endpoint: &Endpoint) -> Channel {
        let path = Arc::clone(&self.path);
        endpoint.connect_with_connector_lazy(service_fn(move |_: Uri| {
            let path = Arc::clone(&path);
            async move {
                let stream =
                    tokio::task::spawn_blocking(move || UnixStream::connect(path.as_path()))
                        .await
                        .map_err(io::Error::other)??;
                Ok::<_, io::Error>(TokioIo::new(WindowsStream::new(Async::new(stream)?)))
            }
        }))
    }
}
