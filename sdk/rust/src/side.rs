use crate::proto::Side;
use crate::proto::side::Of;

impl Side {
    /// A workspace, by its path: what it committed.
    pub fn workspace(path: impl Into<String>) -> Self {
        Self {
            of: Some(Of::Workspace(path.into())),
        }
    }

    /// A branch gitd published, or that the repository's pool holds.
    pub fn branch(name: impl Into<String>) -> Self {
        Self {
            of: Some(Of::Branch(name.into())),
        }
    }
}
