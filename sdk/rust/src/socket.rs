//! gitd's Unix socket.

#[cfg(unix)]
mod unix;

#[cfg(unix)]
pub(crate) use unix::Socket;
