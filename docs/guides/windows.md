# Windows

gitd runs on Windows 10 and 11, amd64 and arm64, for agents on a workstation or a Windows
server. Windows has no overlay. On a [Dev Drive](https://learn.microsoft.com/windows/dev-drive/),
or any other ReFS volume, gitd clones each file of a template by its blocks, sharing them
until written. On NTFS it copies the files.

It needs [Git for Windows](https://git-scm.com/download/win) on the `PATH`, or its path as
`git` in `gitd.toml`.

## As a service

From an administrator's PowerShell:

```powershell
New-Item -ItemType Directory -Force C:\ProgramData\gitd, "C:\Program Files\gitd" | Out-Null
Invoke-WebRequest -OutFile "C:\Program Files\gitd\gitd.exe" https://github.com/dooservice/gitd/releases/latest/download/gitd-windows-amd64.exe
@'
state = "D:/gitd/state"
roots = ["D:/gitd/workspaces"]
'@ | Set-Content -Encoding utf8 C:\ProgramData\gitd\gitd.toml
& "C:\Program Files\gitd\gitd.exe" service install
& "C:\Program Files\gitd\gitd.exe" service start
```

`D:` is the Dev Drive here; without one, leave `state` and `roots` out and gitd keeps them
under `C:\ProgramData\gitd\state`.

`gitd service install` registers the service gitd, run as LocalSystem, started with Windows
and restarted when it fails. It also creates the local group `gitd` and adds you to it
(`--user` adds someone else). Sign out and back in for the membership to count. Members of
the group may:

- connect to the socket, `C:\ProgramData\gitd\gitd.sock`, which is where the command line
  looks by default;
- read the pools and templates their workspaces borrow from;
- change the workspaces, and nothing else gitd keeps.

The configuration, which holds the credentials, is left to SYSTEM and Administrators.

The service logs to `gitd.log` beside its configuration. `gitd service status`, `stop` and
`uninstall` do what they say. Uninstalling keeps the pools and workspaces.

## In a terminal

Without the service, gitd serves for as long as the terminal is open, and only you can reach
it:

```powershell
@'
socket = "D:/gitd/gitd.sock"
state = "D:/gitd/state"
roots = ["D:/gitd/workspaces"]
'@ | Set-Content -Encoding utf8 $HOME\gitd.toml
gitd serve --config $HOME\gitd.toml
# in another terminal
$env:GITD_SOCKET = "D:\gitd\gitd.sock"
gitd prepare https://github.com/acme/web D:\gitd\workspaces\agent-1
```

## What differs from Linux

- `gitd info` says `workspaces made by reflink` when templates are cloned. Keep the state and
  the roots on one ReFS volume.
- Files keep the line endings the repository has: gitd reads no system or global git
  configuration, so `core.autocrlf` is off unless a workspace sets it.
- Workspaces allow paths longer than 260 characters (`core.longpaths`).
- There is no `gitd-mount` and no CSI driver; those are Linux's.

The CI's Windows job measures it with [`benchmarks/windows.ps1`](../../benchmarks/windows.ps1)
on a Dev Drive.
