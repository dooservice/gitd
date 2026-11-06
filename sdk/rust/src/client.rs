use prost_types::Timestamp;
use tonic::service::interceptor::InterceptedService;
use tonic::transport::Channel;

use crate::error::Error;
use crate::http_refusal::HttpRefusal;
use crate::key::Key;
use crate::options::Options;
use crate::proto::connection_service_client::ConnectionServiceClient;
use crate::proto::endpoint_service_client::EndpointServiceClient;
use crate::proto::publishing_service_client::PublishingServiceClient;
use crate::proto::repository_service_client::RepositoryServiceClient;
use crate::proto::server_service_client::ServerServiceClient;
use crate::proto::workspace_service_client::WorkspaceServiceClient;
use crate::proto::{
    AuditEntry, AuditRequest, InfoRequest, InfoResponse, ProbeRequest, ProbeResponse,
};
use crate::review_options::AuditOptions;

/// What the contract's clients call through: the channel to gitd, with the caller's key.
pub type Transport = InterceptedService<HttpRefusal<Channel>, Key>;

/// A gitd on its Unix socket, or on the network with a key naming the caller's tenant.
///
/// No call on workspaces takes a credential: gitd reads and publishes with the ones it
/// keeps. The calls a platform makes to manage those credentials, the publishing settings
/// and the endpoint's tokens are on the contract's own clients: [`Gitd::connections`],
/// [`Gitd::publishing`] and [`Gitd::endpoint`].
///
/// A `Gitd` is cheap to clone, and its clones share one connection.
#[derive(Clone, Debug)]
pub struct Gitd {
    channel: HttpRefusal<Channel>,
    key: Key,
}

impl Gitd {
    /// The gitd `options` name. Nothing is dialled until the first call, which fails if
    /// gitd cannot be reached.
    pub async fn connect(options: Options) -> Result<Self, Error> {
        let key = Key::new(options.resolved_key()?)?;
        let timeout = options.duration();
        let channel = options.target()?.channel(|endpoint| match timeout {
            Some(timeout) => endpoint.timeout(timeout),
            None => endpoint,
        })?;
        Ok(Self {
            channel: HttpRefusal::new(channel),
            key,
        })
    }

    /// The contract's client of `WorkspaceService`, for what the methods here leave out.
    pub fn workspaces(&self) -> WorkspaceServiceClient<Transport> {
        WorkspaceServiceClient::with_interceptor(self.channel.clone(), self.key.clone())
    }

    /// The contract's client of `RepositoryService`.
    pub fn repositories(&self) -> RepositoryServiceClient<Transport> {
        RepositoryServiceClient::with_interceptor(self.channel.clone(), self.key.clone())
    }

    /// The contract's client of `ServerService`.
    pub fn server(&self) -> ServerServiceClient<Transport> {
        ServerServiceClient::with_interceptor(self.channel.clone(), self.key.clone())
    }

    /// The contract's client of `ConnectionService`: the credentials gitd keeps, and
    /// connecting GitHub.
    pub fn connections(&self) -> ConnectionServiceClient<Transport> {
        ConnectionServiceClient::with_interceptor(self.channel.clone(), self.key.clone())
    }

    /// The contract's client of `PublishingService`: the rules publishing follows.
    pub fn publishing(&self) -> PublishingServiceClient<Transport> {
        PublishingServiceClient::with_interceptor(self.channel.clone(), self.key.clone())
    }

    /// The contract's client of `EndpointService`: tokens for sandboxes elsewhere.
    pub fn endpoint(&self) -> EndpointServiceClient<Transport> {
        EndpointServiceClient::with_interceptor(self.channel.clone(), self.key.clone())
    }

    /// Which gitd answers, and how it makes workspaces here and why.
    pub async fn info(&self) -> Result<InfoResponse, Error> {
        Ok(self.server().info(InfoRequest {}).await?.into_inner())
    }

    /// Whether a repository can be read with the credentials gitd keeps, and its branches.
    pub async fn probe(&self, url: impl Into<String>) -> Result<ProbeResponse, Error> {
        let request = ProbeRequest {
            url: url.into(),
            credentials: Vec::new(),
        };
        Ok(self.repositories().probe(request).await?.into_inner())
    }

    /// The audit log: every publish, done or refused, and every change to connections and
    /// publishing settings, newest first.
    pub async fn audit(&self, options: AuditOptions) -> Result<Vec<AuditEntry>, Error> {
        let request = AuditRequest {
            since: options.since.map(Timestamp::from),
            url: options.url,
            limit: options.limit,
        };
        Ok(self.server().audit(request).await?.into_inner().entries)
    }
}
