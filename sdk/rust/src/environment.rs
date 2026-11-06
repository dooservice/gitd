use std::env;
use std::path::PathBuf;

/// The variable naming gitd's Unix socket.
pub const SOCKET_VARIABLE: &str = "GITD_SOCKET";
/// The variable naming a gitd on the network, `https://host:port`.
pub const URL_VARIABLE: &str = "GITD_URL";
/// The variable holding the key naming the caller's tenant.
pub const KEY_VARIABLE: &str = "GITD_KEY";
/// The variable naming the file that holds the key; it wins over [`KEY_VARIABLE`].
pub const KEY_FILE_VARIABLE: &str = "GITD_KEY_FILE";
/// The variable naming the authority of a gitd's certificate the system does not trust.
pub const CA_FILE_VARIABLE: &str = "GITD_CA_FILE";

/// What the environment says about the gitd to call, as the command line reads it. An empty
/// variable says nothing.
pub(crate) struct Environment;

impl Environment {
    pub(crate) fn socket() -> Option<PathBuf> {
        Self::text(SOCKET_VARIABLE).map(PathBuf::from)
    }

    pub(crate) fn url() -> Option<String> {
        Self::text(URL_VARIABLE)
    }

    pub(crate) fn key() -> Option<String> {
        Self::text(KEY_VARIABLE).map(|key| key.trim().to_owned())
    }

    pub(crate) fn key_file() -> Option<PathBuf> {
        Self::text(KEY_FILE_VARIABLE).map(PathBuf::from)
    }

    pub(crate) fn ca_file() -> Option<PathBuf> {
        Self::text(CA_FILE_VARIABLE).map(PathBuf::from)
    }

    fn text(name: &str) -> Option<String> {
        env::var(name).ok().filter(|value| !value.is_empty())
    }
}
