use std::path::PathBuf;
use std::sync::Arc;

use hyper_util::rt::TokioIo;
use tokio::net::UnixStream;
use tonic::transport::{Channel, Endpoint, Uri};
use tower::service_fn;

/// gitd's Unix socket, dialled with tokio.
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
            async move { UnixStream::connect(path.as_path()).await.map(TokioIo::new) }
        }))
    }
}
