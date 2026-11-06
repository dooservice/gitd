use std::io;
use std::pin::Pin;
use std::task::{Context, Poll, ready};

use async_io::Async;
use futures_io::{AsyncRead as _, AsyncWrite as _};
use tokio::io::{AsyncRead, AsyncWrite, ReadBuf};
use uds_windows::UnixStream;

/// A Windows AF_UNIX connection that tokio reads and writes: `async-io` waits for it to be
/// ready, and the reads and writes go through a shared reference, as `async-io` allows for
/// any socket that reads and writes through one.
pub(crate) struct WindowsStream {
    inner: Async<UnixStream>,
}

impl WindowsStream {
    pub(crate) fn new(inner: Async<UnixStream>) -> Self {
        Self { inner }
    }
}

impl AsyncRead for WindowsStream {
    fn poll_read(
        self: Pin<&mut Self>,
        context: &mut Context<'_>,
        buffer: &mut ReadBuf<'_>,
    ) -> Poll<io::Result<()>> {
        let mut shared = &self.inner;
        let read = ready!(Pin::new(&mut shared).poll_read(context, buffer.initialize_unfilled()))?;
        buffer.advance(read);
        Poll::Ready(Ok(()))
    }
}

impl AsyncWrite for WindowsStream {
    fn poll_write(
        self: Pin<&mut Self>,
        context: &mut Context<'_>,
        buffer: &[u8],
    ) -> Poll<io::Result<usize>> {
        let mut shared = &self.inner;
        Pin::new(&mut shared).poll_write(context, buffer)
    }

    fn poll_flush(self: Pin<&mut Self>, context: &mut Context<'_>) -> Poll<io::Result<()>> {
        let mut shared = &self.inner;
        Pin::new(&mut shared).poll_flush(context)
    }

    fn poll_shutdown(self: Pin<&mut Self>, context: &mut Context<'_>) -> Poll<io::Result<()>> {
        let mut shared = &self.inner;
        Pin::new(&mut shared).poll_close(context)
    }
}
