use std::fs;
use std::path::{Path, PathBuf};
use std::time::Duration;

use crate::environment::Environment;
use crate::error::Error;
use crate::target::{Address, DEFAULT_SOCKET, Target};

/// Which gitd to call and how. What is left unset is read from the environment as gitd's
/// command line reads it, then defaults: the network address from `GITD_URL`, else the
/// socket from `GITD_SOCKET`, then [`DEFAULT_SOCKET`]; the key from the file `GITD_KEY_FILE`
/// names, then `GITD_KEY`; the certificate authority from `GITD_CA_FILE`.
///
/// ```no_run
/// # async fn run() -> Result<(), gitd::Error> {
/// let options = gitd::Options::new()
///     .url("https://gitd.internal:9420")
///     .key_file("/run/secrets/acme");
/// let gitd = gitd::Gitd::connect(options).await?;
/// # Ok(())
/// # }
/// ```
#[derive(Clone, Debug, Default)]
pub struct Options {
    socket: Option<PathBuf>,
    url: Option<String>,
    key: Option<Secret>,
    key_file: Option<PathBuf>,
    ca_file: Option<PathBuf>,
    timeout: Option<Duration>,
}

impl Options {
    /// Options that leave everything to the environment and the defaults.
    pub fn new() -> Self {
        Self::default()
    }

    /// gitd's Unix socket, called when neither these options nor `GITD_URL` give a network
    /// address, as the other clients do.
    pub fn socket(mut self, path: impl Into<PathBuf>) -> Self {
        self.socket = Some(path.into());
        self
    }

    /// A gitd on the network instead of the socket: `https://host:port`, or `http://` on a
    /// private network, where the key travels unencrypted.
    pub fn url(mut self, url: impl Into<String>) -> Self {
        self.url = Some(url.into());
        self
    }

    /// The key naming the caller's tenant, brought on every call.
    pub fn key(mut self, key: impl Into<String>) -> Self {
        self.key = Some(Secret(key.into()));
        self
    }

    /// The file holding the key naming the caller's tenant, read when connecting.
    pub fn key_file(mut self, path: impl Into<PathBuf>) -> Self {
        self.key_file = Some(path.into());
        self
    }

    /// The PEM of a certificate authority to trust besides the system's, for a gitd whose
    /// certificate the system does not trust.
    pub fn ca_file(mut self, path: impl Into<PathBuf>) -> Self {
        self.ca_file = Some(path.into());
        self
    }

    /// How long any call may take; gitd otherwise bounds its calls itself.
    pub fn timeout(mut self, timeout: Duration) -> Self {
        self.timeout = Some(timeout);
        self
    }

    pub(crate) fn duration(&self) -> Option<Duration> {
        self.timeout
    }

    /// Where gitd answers, with what was given or the environment says.
    pub(crate) fn target(&self) -> Result<Target, Error> {
        let url = self.url.clone().or_else(Environment::url);
        let Some(url) = url else {
            let socket = self.socket.clone().or_else(Environment::socket);
            return Ok(Target::Socket(
                socket.unwrap_or_else(|| PathBuf::from(DEFAULT_SOCKET)),
            ));
        };
        let authority = match self.ca_file.clone().or_else(Environment::ca_file) {
            Some(path) => Some(Self::read(&path)?),
            None => None,
        };
        Ok(Target::Network(Address::parse(&url)?, authority))
    }

    /// The caller's key, if there is one: the one given, else the file's, else the
    /// environment's.
    pub(crate) fn resolved_key(&self) -> Result<Option<String>, Error> {
        if let Some(Secret(key)) = &self.key {
            return Ok(Some(key.trim().to_owned()));
        }
        if let Some(path) = self.key_file.clone().or_else(Environment::key_file) {
            let key = String::from_utf8_lossy(&Self::read(&path)?)
                .trim()
                .to_owned();
            return Ok(Some(key));
        }
        Ok(Environment::key())
    }

    fn read(path: &Path) -> Result<Vec<u8>, Error> {
        fs::read(path).map_err(|source| Error::File {
            path: path.to_owned(),
            source,
        })
    }
}

/// A key, kept out of `Debug` output.
#[derive(Clone)]
struct Secret(String);

impl std::fmt::Debug for Secret {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter.write_str("Secret(…)")
    }
}
