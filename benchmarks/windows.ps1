# N workspaces of one repository on Windows, where templates are cloned by blocks on a Dev
# Drive or another ReFS volume: how long gitd takes for each and the disk they take, as the
# volume's free space says — a clone shares its template's blocks — against N
# `git clone --depth 1`. ROOT must be on the Dev Drive for workspaces to be cloned.
#
#   go build -o bin/gitd.exe ./cmd/gitd; benchmarks/windows.ps1 [-Repo URL] [-Agents N] [-Root DIR]
param(
    [string]$Repo = "https://github.com/kubernetes/kubernetes",
    [int]$Agents = 10,
    [string]$Root = (Join-Path ([System.IO.Path]::GetTempPath()) "gitd-windows-$PID")
)
$ErrorActionPreference = "Stop"
$gitd = Join-Path $PWD "bin\gitd.exe"
New-Item -ItemType Directory -Force -Path "$Root\workspaces" | Out-Null
$socket = "$Root\gitd.sock"
$toml = ($Root -replace '\\', '/')
@"
socket = "$toml/gitd.sock"
state = "$toml/state"
roots = ["$toml/workspaces"]
"@ | Set-Content -Encoding utf8 "$Root\gitd.toml"

$serving = Start-Process -FilePath $gitd -ArgumentList "serve", "--config", "$Root\gitd.toml" -RedirectStandardOutput "$Root\gitd.log" -RedirectStandardError "$Root\gitd.err" -PassThru -NoNewWindow
try {
    $ready = $false
    for ($i = 0; $i -lt 150 -and -not $ready; $i++) {
        & $gitd info --socket $socket *> $null
        $ready = $LASTEXITCODE -eq 0
        if (-not $ready) { Start-Sleep -Milliseconds 200 }
    }
    if (-not $ready) { Get-Content "$Root\gitd.log", "$Root\gitd.err"; throw "gitd did not start" }

    $made = (& $gitd info --socket $socket | Select-Object -Last 1)
    $drive = (Get-Item $Root).PSDrive.Name
    function Free { (Get-Volume -DriveLetter $drive).SizeRemaining }

    $first = (Measure-Command { & $gitd prepare --socket $socket $Repo "$Root\workspaces\agent-1" | Out-Null }).TotalSeconds
    if ($LASTEXITCODE -ne 0) { throw "the first workspace failed" }
    $before = Free
    $rest = (Measure-Command {
        foreach ($i in 2..$Agents) {
            & $gitd prepare --socket $socket $Repo "$Root\workspaces\agent-$i" | Out-Null
            if ($LASTEXITCODE -ne 0) { throw "workspace $i failed" }
        }
    }).TotalSeconds
    $after = Free
    $each = $rest / ($Agents - 1)
    $perWorkspace = ($before - $after) / 1MB / ($Agents - 1)
    $clean = 0
    foreach ($i in 1..$Agents) {
        if (-not (git -C "$Root\workspaces\agent-$i" status --porcelain)) { $clean++ }
    }
    $clone = (Measure-Command { git clone -q --depth 1 $Repo "$Root\clone" }).TotalSeconds
    $cloneDisk = (Get-ChildItem -Recurse -File -Force "$Root\clone" | Measure-Object -Sum Length).Sum / 1MB
    $os = (Get-CimInstance Win32_OperatingSystem).Caption
    $filesystem = (Get-Volume -DriveLetter $drive).FileSystemType

    "## $Agents workspaces of $Repo on Windows"
    ""
    "$os, $filesystem, $env:PROCESSOR_ARCHITECTURE, $(git --version). $made"
    ""
    "| How | Time | Disk |"
    "|---|---|---|"
    "| ``gitd prepare`` | the first {0:N2}s, then {1:N3}s each | {2:N1} MB each, by the volume's free space |" -f $first, $each, $perWorkspace
    "| ``git clone --depth 1`` | {0:N2}s each | {1:N1} MB each |" -f $clone, $cloneDisk
    ""
    "- Clean checkouts as git sees them: $clean of $Agents."
    $served = Select-String -Path "$Root\gitd.log" -Pattern 'procedure=\S*Prepare took=(\S+)' | ForEach-Object { $_.Matches[0].Groups[1].Value }
    "- What gitd itself took for each prepare, as its log says: $($served -join ', ')."
    $largest = Get-ChildItem -Recurse -File "$Root\workspaces\agent-2" | Where-Object { $_.FullName -notmatch '\\.git\\' } | Sort-Object Length -Descending | Select-Object -First 1
    $counts = fsutil file queryExtentsAndRefCounts $largest.FullName 2>&1 | Select-String -Pattern 'Ref(erence)?\s*Count\s*:\s*(0x)?([0-9a-fA-F]+)' | ForEach-Object { [Convert]::ToInt64($_.Matches[0].Groups[3].Value, $(if ($_.Matches[0].Groups[2].Value) { 16 } else { 10 })) }
    $shared = ($counts | Measure-Object -Maximum).Maximum
    "- Blocks of the largest file of a workspace ({0:N0} KB) are shared by up to $shared files, as ReFS counts them." -f ($largest.Length / 1KB)
    $start = (Measure-Command { foreach ($i in 1..20) { git --version | Out-Null } }).TotalMilliseconds / 20
    "- One git process starts and ends in {0:N0} ms here." -f $start
}
finally {
    Stop-Process -Id $serving.Id -Force -ErrorAction SilentlyContinue
}
