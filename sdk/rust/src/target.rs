use http::Uri;
use tonic::transport::{Certificate, Channel, ClientTlsConfig, Endpoint};

use crate::error::Error;
use crate::socket::Socket;

/// Where gitd answers when the configuration says nowhere else, as gitd's own default.
#[cfg(unix)]
pub const DEFAULT_SOCKET: &str = "/run/gitd/gitd.sock";
/// Where gitd answers when the configuration says nowhere else, as gitd's own default.
#[cfg(windows)]
pub const DEFAULT_SOCKET: &str = r"C:\ProgramData\gitd\gitd.sock";

/// The origin calls on the socket are addressed to: no host is dialled there.
const SOCKET_ORIGIN: &str = "http://gitd";
const SECURE_SCHEME: &str = "https";
const PLAIN_SCHEME: &str = "http";

/// Where gitd answers: its Unix socket, or an address on the network with the PEM of a
/// certificate authority to trust besides the system's.
pub(crate) enum Target {
    Socket(std::path::PathBuf),
    Network(Address, Option<Vec<u8>>),
}

impl Target {
    /// A channel to gitd, dialled on the first call.
    pub(crate) fn channel(
        self,
        configure: impl FnOnce(Endpoint) -> Endpoint,
    ) -> Result<Channel, Error> {
        match self {
            Self::Socket(path) => {
                let endpoint = configure(Endpoint::from_static(SOCKET_ORIGIN));
                Ok(Socket::new(path).channel(&endpoint))
            }
            Self::Network(address, authority) => {
                let mut endpoint = configure(Endpoint::from(address.uri.clone()));
                if address.is_secure() {
                    let mut tls = ClientTlsConfig::new().with_native_roots();
                    if let Some(pem) = authority {
                        tls = tls.ca_certificate(Certificate::from_pem(pem));
                    }
                    endpoint = endpoint.tls_config(tls).map_err(Error::Tls)?;
                }
                Ok(endpoint.connect_lazy())
            }
        }
    }
}

/// A gitd's address on the network: `https://host:port`, or `http://host:port` on a
/// private network.
#[derive(Debug)]
pub(crate) struct Address {
    uri: Uri,
}

impl Address {
    pub(crate) fn parse(url: &str) -> Result<Self, Error> {
        let refused = || Error::Address(url.to_owned());
        let uri: Uri = url.trim_end_matches('/').parse().map_err(|_| refused())?;
        let known = matches!(uri.scheme_str(), Some(SECURE_SCHEME | PLAIN_SCHEME));
        let has_host = uri.host().is_some_and(|host| !host.is_empty());
        if known && has_host {
            Ok(Self { uri })
        } else {
            Err(refused())
        }
    }

    pub(crate) fn is_secure(&self) -> bool {
        self.uri.scheme_str() == Some(SECURE_SCHEME)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn takes_https_and_http_addresses() {
        assert!(
            Address::parse("https://gitd.internal:9420")
                .unwrap()
                .is_secure()
        );
        assert!(
            Address::parse("https://gitd.internal:9420/")
                .unwrap()
                .is_secure()
        );
        assert!(!Address::parse("http://10.0.0.5:9420").unwrap().is_secure());
    }

    #[test]
    fn refuses_what_is_not_an_address() {
        for url in [
            "gitd.internal:9420",
            "ftp://gitd.internal",
            "https://",
            "unix:/run/gitd/gitd.sock",
            "",
        ] {
            assert!(
                matches!(Address::parse(url), Err(Error::Address(_))),
                "{url} was taken"
            );
        }
    }
}
