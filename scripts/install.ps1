# Install only a verified Anchor executable; do not initialize/edit Agent state.
[CmdletBinding()]
param(
    [string] $Version = '',
    [string] $InstallDir = '',
    [switch] $Replace,
    [switch] $Help
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
if ($Help) {
    Write-Output @'
Usage: & .\install.ps1 [-Version VERSION] [-InstallDir DIR] [-Replace] [-Help]
Downloads one Windows AMD64 executable and verifies its release SHA256SUMS.
Default: most recently created published GitHub Release (INCLUDING prereleases).
Directory: %LOCALAPPDATA%\Anchor\bin. No admin privileges, PATH/profile or Codex edits.
-Version accepts an optional leading v. -InstallDir must be absolute.
-Replace explicitly replaces a regular file: stop Anchor and back up state first.
Only Windows x64 is currently published; other targets fail closed.
'@
    return
}
function Assert-Version([string] $Value) {
    if ($Value -cnotmatch '\A(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-(alpha|beta|rc)\.(0|[1-9][0-9]*))?\z') {
        throw 'Invalid/unsupported version. Use X.Y.Z or X.Y.Z-alpha/beta/rc.N.'
    }
}
if ($Version.StartsWith('v')) { $Version = $Version.Substring(1); Assert-Version $Version }
if ($Version) { Assert-Version $Version }
if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) { throw 'Windows only; Linux: use install.sh.' }
$nativeArch = $env:PROCESSOR_ARCHITEW6432
if (-not $nativeArch) { $nativeArch = $env:PROCESSOR_ARCHITECTURE }
if ($nativeArch -ne 'AMD64') { throw 'Unsupported architecture; Windows AMD64 only.' }
if (-not $InstallDir) {
    if (-not $env:LOCALAPPDATA) { throw 'LOCALAPPDATA is unset; specify -InstallDir.' }
    $InstallDir = Join-Path $env:LOCALAPPDATA 'Anchor\bin'
}
# Require fully qualified local drive paths, not drive-relative/UNC/network paths.
if ($InstallDir -notmatch '\A[A-Za-z]:[\\/]' -or $InstallDir -match '[\r\n;]') {
    throw '-InstallDir must be an absolute local drive path without newline/semicolon.'
}
$InstallDir = [IO.Path]::GetFullPath($InstallDir)
$destination = Join-Path $InstallDir 'anchor.exe'
function Assert-Destination {
    # -Force also inspects hidden/reparse items, including dangling symlinks.
    $item = Get-Item -LiteralPath $destination -Force -ErrorAction SilentlyContinue
    if ($item) {
        if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 -or $item.PSIsContainer) {
            throw 'Refusing symlink/reparse-point or non-file destination.'
        }
        if (-not $Replace) { throw 'anchor.exe already exists; stop it, back up state, then use -Replace.' }
    }
}
Assert-Destination
[IO.Directory]::CreateDirectory($InstallDir) | Out-Null
$lockPath = Join-Path $InstallDir '.anchor-install.lock'
$lock = $null
$workDir = $null
$oldTls = [Net.ServicePointManager]::SecurityProtocol
try {
    # Exclusive creation rejects existing files/reparse points. Windows removes
    # only our lock on close, atomically with releasing its non-shared handle.
    $lock = [IO.FileStream]::new($lockPath, [IO.FileMode]::CreateNew, [IO.FileAccess]::ReadWrite, [IO.FileShare]::None, 1, [IO.FileOptions]::DeleteOnClose)
    $workDir = Join-Path $InstallDir ('.anchor-install.' + [Guid]::NewGuid().ToString('N'))
    [IO.Directory]::CreateDirectory($workDir) | Out-Null
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
    function Get-HttpsFile([string] $Url, [string] $OutputPath) {
        $uri = [Uri] $Url
        for ($redirects = 0; $redirects -le 5; $redirects++) {
            if ($uri.Scheme -ne 'https') { throw 'Refusing a non-HTTPS download/redirect.' }
            $request = [Net.HttpWebRequest]::Create($uri)
            $request.AllowAutoRedirect = $false
            $request.Timeout = 30000
            $request.ReadWriteTimeout = 180000
            $request.UserAgent = 'Anchor-installer'
            $request.Accept = 'application/vnd.github+json'
            $response = $request.GetResponse()
            try {
                $status = [int] $response.StatusCode
                if ($status -ge 300 -and $status -lt 400) {
                    $location = $response.Headers['Location']
                    if (-not $location) { throw 'Redirect without a Location header.' }
                    $uri = [Uri]::new($uri, $location)
                    continue
                }
                if ($status -ne 200) { throw "Unexpected download status: $status" }
                $output = [IO.File]::Open($OutputPath, [IO.FileMode]::CreateNew)
                try { $response.GetResponseStream().CopyTo($output) } finally { $output.Dispose() }
                return
            } finally { $response.Dispose() }
        }
        throw 'Too many HTTPS redirects.'
    }
    $repo = 'candyflosssss/Anchor'
    if (-not $Version) {
        $metadata = Join-Path $workDir 'releases.json'
        Get-HttpsFile "https://api.github.com/repos/$repo/releases?per_page=1" $metadata
        $releases = @(Get-Content -LiteralPath $metadata -Raw | ConvertFrom-Json)
        if ($releases.Count -ne 1 -or -not $releases[0].tag_name.StartsWith('v') -or $releases[0].draft) {
            throw 'No supported published release; Actions artifacts/drafts cannot be installed here.'
        }
        $Version = $releases[0].tag_name.Substring(1)
        Assert-Version $Version
    }
    $asset = "anchor_${Version}_windows_amd64.exe"
    $base = "https://github.com/$repo/releases/download/v$Version"
    Write-Output "Installing Anchor $Version (Windows AMD64) into $InstallDir"
    $sums = Join-Path $workDir 'SHA256SUMS'
    Get-HttpsFile "$base/SHA256SUMS" $sums
    $matchesFound = @()
    foreach ($line in (Get-Content -LiteralPath $sums)) {
        $fields = @($line.Trim() -split '\s+')
        if ($fields.Count -ge 2 -and ($fields[1] -ceq $asset -or $fields[1] -ceq "*$asset")) {
            if ($fields.Count -ne 2 -or $fields[0] -cnotmatch '\A[0-9a-fA-F]{64}\z') { throw 'Malformed SHA256 entry.' }
            $matchesFound += $fields[0]
        }
    }
    if ($matchesFound.Count -ne 1) { throw 'Checksum entry is missing or duplicate.' }
    $staged = Join-Path $workDir 'anchor.exe'
    Get-HttpsFile "$base/$asset" $staged
    if ((Get-FileHash -LiteralPath $staged -Algorithm SHA256).Hash -ine $matchesFound[0]) {
        throw 'SHA256 mismatch; refusing installation (existing program unchanged).'
    }
    Assert-Destination
    if ([IO.File]::Exists($destination)) {
        # Same-filesystem atomic replacement. Windows refuses a running/locked EXE.
        [IO.File]::Replace($staged, $destination, (Join-Path $workDir 'previous-anchor.exe'))
    } else {
        [IO.File]::Move($staged, $destination)
    }
    Write-Output "Installed: $destination"
    Write-Output 'No state, PATH, shell profile or Codex configuration was modified.'
    if (@($env:Path -split ';') -notcontains $InstallDir) {
        $escapedDir = $InstallDir.Replace("'", "''")
        Write-Output "Add to PATH in this terminal: `$env:Path = '$escapedDir;' + `$env:Path"
    }
    Write-Output 'Next: anchor version (then anchor init for a new instance).'
} finally {
    [Net.ServicePointManager]::SecurityProtocol = $oldTls
    try {
        if ($workDir -and [IO.Directory]::Exists($workDir)) { Remove-Item -LiteralPath $workDir -Recurse -Force }
    } finally {
        if ($lock) { $lock.Dispose() }
    }
}
