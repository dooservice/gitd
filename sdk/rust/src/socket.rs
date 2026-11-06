//! gitd's Unix socket: tokio's on Unix; on Windows, where gitd serves AF_UNIX too but tokio
//! has no Unix sockets, one of its own.

#[cfg(unix)]
mod unix;
#[cfg(windows)]
mod windows;
#[cfg(windows)]
mod windows_stream;

#[cfg(unix)]
pub(crate) use unix::Socket;
#[cfg(windows)]
pub(crate) use windows::Socket;
